package auth

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type Role string

const (
	RoleUser  Role = "user"
	RoleAdmin Role = "admin"
)

var (
	ErrInvalidCredentials = errors.New("invalid username or password")
	ErrUserExists         = errors.New("user already exists")
	ErrUserNotFound       = errors.New("user not found")
	ErrInactiveUser       = errors.New("user is inactive")
	ErrLastAdmin          = errors.New("cannot remove the last admin")
)

type User struct {
	Username       string    `json:"username"`
	PasswordHash   string    `json:"password_hash"`
	Role           Role      `json:"role"`
	Active         bool      `json:"active"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	CheckInPoints  int       `json:"check_in_points"`
	LastCheckInAt  string    `json:"last_check_in_at"` // format: "2006-01-02"
}

type Store struct {
	path  string
	mu    sync.RWMutex
	users map[string]User
}

func OpenStore(path string) (*Store, error) {
	store := &Store{
		path:  path,
		users: map[string]User{},
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return store, nil
		}
		return nil, err
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return store, nil
	}

	if err := json.Unmarshal(data, &store.users); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *Store) CreateUser(username, password string, role Role) error {
	username = normalizeUsername(username)
	if err := validateUsername(username); err != nil {
		return err
	}
	if len(password) < 8 {
		return errors.New("password must be at least 8 characters")
	}
	if role != RoleAdmin {
		role = RoleUser
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.users[username]; exists {
		return ErrUserExists
	}

	now := time.Now().UTC()
	s.users[username] = User{
		Username:     username,
		PasswordHash: string(hash),
		Role:         role,
		Active:       true,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	return s.saveLocked()
}

func (s *Store) Authenticate(username, password string) (User, error) {
	username = normalizeUsername(username)

	s.mu.RLock()
	user, ok := s.users[username]
	s.mu.RUnlock()
	if !ok {
		return User{}, ErrInvalidCredentials
	}
	if !user.Active {
		return User{}, ErrInactiveUser
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) != nil {
		return User{}, ErrInvalidCredentials
	}
	return user, nil
}

func (s *Store) ListUsers() []User {
	s.mu.RLock()
	defer s.mu.RUnlock()

	users := make([]User, 0, len(s.users))
	for _, user := range s.users {
		users = append(users, user)
	}
	sort.Slice(users, func(i, j int) bool {
		return users[i].Username < users[j].Username
	})
	return users
}

func (s *Store) SetRole(username string, role Role) error {
	username = normalizeUsername(username)
	if role != RoleAdmin {
		role = RoleUser
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	user, ok := s.users[username]
	if !ok {
		return ErrUserNotFound
	}
	if user.Role == RoleAdmin && role != RoleAdmin && s.adminCountLocked() == 1 {
		return ErrLastAdmin
	}
	user.Role = role
	user.UpdatedAt = time.Now().UTC()
	s.users[username] = user
	return s.saveLocked()
}

func (s *Store) SetActive(username string, active bool) error {
	username = normalizeUsername(username)

	s.mu.Lock()
	defer s.mu.Unlock()

	user, ok := s.users[username]
	if !ok {
		return ErrUserNotFound
	}
	if user.Role == RoleAdmin && !active && s.adminCountLocked() == 1 {
		return ErrLastAdmin
	}
	user.Active = active
	user.UpdatedAt = time.Now().UTC()
	s.users[username] = user
	return s.saveLocked()
}

func (s *Store) ResetPassword(username, password string) error {
	username = normalizeUsername(username)
	if len(password) < 8 {
		return errors.New("password must be at least 8 characters")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	user, ok := s.users[username]
	if !ok {
		return ErrUserNotFound
	}
	user.PasswordHash = string(hash)
	user.UpdatedAt = time.Now().UTC()
	s.users[username] = user
	return s.saveLocked()
}

func (s *Store) DeleteUser(username string) error {
	username = normalizeUsername(username)

	s.mu.Lock()
	defer s.mu.Unlock()

	user, ok := s.users[username]
	if !ok {
		return ErrUserNotFound
	}
	if user.Role == RoleAdmin && s.adminCountLocked() == 1 {
		return ErrLastAdmin
	}
	delete(s.users, username)
	return s.saveLocked()
}

func (s *Store) HasAdmin() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.adminCountLocked() > 0
}

func (s *Store) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}

	data, err := json.MarshalIndent(s.users, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o600)
}

func (s *Store) adminCountLocked() int {
	count := 0
	for _, user := range s.users {
		if user.Role == RoleAdmin {
			count++
		}
	}
	return count
}

func normalizeUsername(username string) string {
	return strings.ToLower(strings.TrimSpace(username))
}

func validateUsername(username string) error {
	if len(username) < 3 || len(username) > 32 {
		return errors.New("username must be 3-32 characters")
	}
	for _, ch := range username {
		if ch >= 'a' && ch <= 'z' {
			continue
		}
		if ch >= '0' && ch <= '9' {
			continue
		}
		if ch == '_' || ch == '-' {
			continue
		}
		return fmt.Errorf("username contains invalid character %q", ch)
	}
	return nil
}

func RandomPassword(length int) string {
	if length < 12 {
		length = 12
	}
	buf := make([]byte, length)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("admin-%d", time.Now().UnixNano())
	}
	return base64.RawURLEncoding.EncodeToString(buf)[:length]
}

type CheckInResult struct {
	Success       bool
	PointsEarned  int
	TotalPoints   int
	ConsecutiveDays int
	Message       string
}

func (s *Store) CheckIn(username string) (CheckInResult, error) {
	username = normalizeUsername(username)

	s.mu.Lock()
	defer s.mu.Unlock()

	user, ok := s.users[username]
	if !ok {
		return CheckInResult{}, ErrUserNotFound
	}

	today := time.Now().Format("2006-01-02")
	if user.LastCheckInAt == today {
		return CheckInResult{
			Success:      false,
			TotalPoints:  user.CheckInPoints,
			Message:      "你今天已经签到过了",
		}, nil
	}

	// Calculate consecutive days
	var consecutiveDays int
	if user.LastCheckInAt != "" {
		yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
		if user.LastCheckInAt == yesterday {
			consecutiveDays = 1
		}
	}

	// Points calculation: base 10, bonus for consecutive days
	points := 10 + consecutiveDays*2
	if points > 30 {
		points = 30
	}

	user.CheckInPoints += points
	user.LastCheckInAt = today
	user.UpdatedAt = time.Now().UTC()
	s.users[username] = user

	if err := s.saveLocked(); err != nil {
		return CheckInResult{}, err
	}

	return CheckInResult{
		Success:         true,
		PointsEarned:    points,
		TotalPoints:     user.CheckInPoints,
		ConsecutiveDays: consecutiveDays + 1,
		Message:         fmt.Sprintf("签到成功！获得 %d 积分", points),
	}, nil
}

type RankingEntry struct {
	Username string
	Points   int
	Rank     int
}

func (s *Store) GetCheckInRanking(limit int) []RankingEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entries := make([]RankingEntry, 0, len(s.users))
	for _, user := range s.users {
		entries = append(entries, RankingEntry{
			Username: user.Username,
			Points:   user.CheckInPoints,
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Points != entries[j].Points {
			return entries[i].Points > entries[j].Points
		}
		return entries[i].Username < entries[j].Username
	})

	if limit > 0 && len(entries) > limit {
		entries = entries[:limit]
	}

	for i := range entries {
		entries[i].Rank = i + 1
	}

	return entries
}

func (s *Store) GetUser(username string) (User, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	user, ok := s.users[normalizeUsername(username)]
	return user, ok
}
