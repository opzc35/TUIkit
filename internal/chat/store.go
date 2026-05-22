package chat

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const MaxMessageLength = 1000

var (
	ErrChannelExists    = errors.New("channel already exists")
	ErrChannelNotFound  = errors.New("channel not found")
	ErrMessageNotFound  = errors.New("message not found")
	ErrMutedUser        = errors.New("user is muted")
	ErrInvalidChannel   = errors.New("channel name must be 2-32 characters")
	ErrInvalidMessage   = errors.New("message cannot be empty")
	ErrInvalidMuteHours = errors.New("mute hours must be greater than zero")
)

type Channel struct {
	Name      string    `json:"name"`
	Topic     string    `json:"topic"`
	CreatedBy string    `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
}

type Message struct {
	ID        int64     `json:"id"`
	Channel   string    `json:"channel"`
	Author    string    `json:"author"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
	Deleted   bool      `json:"deleted"`
	DeletedBy string    `json:"deleted_by,omitempty"`
	DeletedAt time.Time `json:"deleted_at,omitempty"`
}

type Mute struct {
	Username  string    `json:"username"`
	Reason    string    `json:"reason"`
	Until     time.Time `json:"until"`
	CreatedBy string    `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
}

type EventType string

const (
	EventMessage      EventType = "message"
	EventDelete       EventType = "delete"
	EventClear        EventType = "clear"
	EventMuteChanged  EventType = "mute_changed"
	EventChannelAdded EventType = "channel_added"
)

type Event struct {
	Type    EventType
	Channel string
	Message Message
	Text    string
	Time    time.Time
}

type dataFile struct {
	NextMessageID int64              `json:"next_message_id"`
	Channels      map[string]Channel `json:"channels"`
	Messages      []Message          `json:"messages"`
	Mutes         map[string]Mute    `json:"mutes"`
}

type Store struct {
	path        string
	mu          sync.RWMutex
	data        dataFile
	subscribers map[string]map[chan Event]struct{}
}

func OpenStore(path string) (*Store, error) {
	store := &Store{
		path: path,
		data: dataFile{
			NextMessageID: 1,
			Channels:      map[string]Channel{},
			Messages:      []Message{},
			Mutes:         map[string]Mute{},
		},
		subscribers: map[string]map[chan Event]struct{}{},
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			store.normalizeLocked()
			if err := store.saveLocked(); err != nil {
				return nil, err
			}
			return store, nil
		}
		return nil, err
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		store.normalizeLocked()
		if err := store.saveLocked(); err != nil {
			return nil, err
		}
		return store, nil
	}
	if err := json.Unmarshal(raw, &store.data); err != nil {
		return nil, err
	}
	store.normalizeLocked()
	return store, nil
}

func (s *Store) CreateChannel(name, topic, createdBy string) error {
	name = normalizeChannel(name)
	if err := validateChannel(name); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.normalizeLocked()
	if _, exists := s.data.Channels[name]; exists {
		return ErrChannelExists
	}

	s.data.Channels[name] = Channel{
		Name:      name,
		Topic:     strings.TrimSpace(topic),
		CreatedBy: strings.TrimSpace(createdBy),
		CreatedAt: time.Now().UTC(),
	}
	if err := s.saveLocked(); err != nil {
		return err
	}
	s.publishLocked(Event{
		Type:    EventChannelAdded,
		Channel: name,
		Text:    fmt.Sprintf("channel #%s created", name),
		Time:    time.Now().UTC(),
	})
	return nil
}

func (s *Store) ListChannels() []Channel {
	s.mu.RLock()
	defer s.mu.RUnlock()

	channels := make([]Channel, 0, len(s.data.Channels))
	for _, channel := range s.data.Channels {
		channels = append(channels, channel)
	}
	sort.Slice(channels, func(i, j int) bool {
		return channels[i].Name < channels[j].Name
	})
	return channels
}

func (s *Store) Channel(name string) (Channel, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	channel, ok := s.data.Channels[normalizeChannel(name)]
	return channel, ok
}

func (s *Store) PostMessage(channel, author, body string) (Message, error) {
	channel = normalizeChannel(channel)
	body = strings.TrimSpace(body)
	if body == "" {
		return Message{}, ErrInvalidMessage
	}
	if len(body) > MaxMessageLength {
		return Message{}, fmt.Errorf("message must be at most %d characters", MaxMessageLength)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.normalizeLocked()
	if _, ok := s.data.Channels[channel]; !ok {
		return Message{}, ErrChannelNotFound
	}
	if mute, ok := s.activeMuteLocked(author); ok {
		return Message{}, fmt.Errorf("%w until %s: %s", ErrMutedUser, mute.Until.Format(time.RFC3339), mute.Reason)
	}

	message := Message{
		ID:        s.data.NextMessageID,
		Channel:   channel,
		Author:    strings.TrimSpace(author),
		Body:      body,
		CreatedAt: time.Now().UTC(),
	}
	s.data.NextMessageID++
	s.data.Messages = append(s.data.Messages, message)
	if err := s.saveLocked(); err != nil {
		return Message{}, err
	}
	s.publishLocked(Event{
		Type:    EventMessage,
		Channel: channel,
		Message: message,
		Time:    message.CreatedAt,
	})
	return message, nil
}

func (s *Store) RecentMessages(channel string, limit int, includeDeleted bool) []Message {
	channel = normalizeChannel(channel)
	if limit <= 0 {
		limit = 20
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	messages := make([]Message, 0, limit)
	for i := len(s.data.Messages) - 1; i >= 0 && len(messages) < limit; i-- {
		message := s.data.Messages[i]
		if message.Channel != channel {
			continue
		}
		if message.Deleted && !includeDeleted {
			continue
		}
		messages = append(messages, message)
	}
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}
	return messages
}

func (s *Store) DeleteMessage(id int64, deletedBy string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.data.Messages {
		if s.data.Messages[i].ID == id {
			s.data.Messages[i].Deleted = true
			s.data.Messages[i].DeletedBy = strings.TrimSpace(deletedBy)
			s.data.Messages[i].DeletedAt = time.Now().UTC()
			if err := s.saveLocked(); err != nil {
				return err
			}
			s.publishLocked(Event{
				Type:    EventDelete,
				Channel: s.data.Messages[i].Channel,
				Message: s.data.Messages[i],
				Text:    fmt.Sprintf("message %d deleted by %s", id, deletedBy),
				Time:    s.data.Messages[i].DeletedAt,
			})
			return nil
		}
	}
	return ErrMessageNotFound
}

func (s *Store) ClearChannel(channel, deletedBy string) (int, error) {
	channel = normalizeChannel(channel)

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.data.Channels[channel]; !ok {
		return 0, ErrChannelNotFound
	}

	count := 0
	now := time.Now().UTC()
	for i := range s.data.Messages {
		if s.data.Messages[i].Channel == channel && !s.data.Messages[i].Deleted {
			s.data.Messages[i].Deleted = true
			s.data.Messages[i].DeletedBy = strings.TrimSpace(deletedBy)
			s.data.Messages[i].DeletedAt = now
			count++
		}
	}
	if err := s.saveLocked(); err != nil {
		return 0, err
	}
	s.publishLocked(Event{
		Type:    EventClear,
		Channel: channel,
		Text:    fmt.Sprintf("%d messages cleared by %s", count, deletedBy),
		Time:    now,
	})
	return count, nil
}

func (s *Store) MuteUser(username string, hours int, reason, createdBy string) error {
	username = normalizeUser(username)
	if username == "" {
		return errors.New("username is required")
	}
	if hours <= 0 {
		return ErrInvalidMuteHours
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.normalizeLocked()
	now := time.Now().UTC()
	s.data.Mutes[username] = Mute{
		Username:  username,
		Reason:    strings.TrimSpace(reason),
		Until:     now.Add(time.Duration(hours) * time.Hour),
		CreatedBy: strings.TrimSpace(createdBy),
		CreatedAt: now,
	}
	if err := s.saveLocked(); err != nil {
		return err
	}
	s.publishLocked(Event{
		Type: EventMuteChanged,
		Text: fmt.Sprintf("%s muted until %s", username, s.data.Mutes[username].Until.Format(time.RFC3339)),
		Time: now,
	})
	return nil
}

func (s *Store) UnmuteUser(username string) error {
	username = normalizeUser(username)

	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.data.Mutes, username)
	if err := s.saveLocked(); err != nil {
		return err
	}
	s.publishLocked(Event{
		Type: EventMuteChanged,
		Text: fmt.Sprintf("%s unmuted", username),
		Time: time.Now().UTC(),
	})
	return nil
}

func (s *Store) ListMutes() []Mute {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.purgeExpiredMutesLocked(time.Now().UTC())
	mutes := make([]Mute, 0, len(s.data.Mutes))
	for _, mute := range s.data.Mutes {
		mutes = append(mutes, mute)
	}
	sort.Slice(mutes, func(i, j int) bool {
		return mutes[i].Username < mutes[j].Username
	})
	return mutes
}

func (s *Store) IsMuted(username string) (Mute, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.activeMuteLocked(username)
}

func (s *Store) Subscribe(channel string) (<-chan Event, func()) {
	channel = normalizeChannel(channel)
	events := make(chan Event, 32)

	s.mu.Lock()
	if s.subscribers == nil {
		s.subscribers = map[string]map[chan Event]struct{}{}
	}
	if s.subscribers[channel] == nil {
		s.subscribers[channel] = map[chan Event]struct{}{}
	}
	s.subscribers[channel][events] = struct{}{}
	s.mu.Unlock()

	unsubscribe := func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if subscribers, ok := s.subscribers[channel]; ok {
			delete(subscribers, events)
			if len(subscribers) == 0 {
				delete(s.subscribers, channel)
			}
		}
		close(events)
	}
	return events, unsubscribe
}

func (s *Store) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, raw, 0o600)
}

func (s *Store) normalizeLocked() {
	if s.data.NextMessageID < 1 {
		s.data.NextMessageID = 1
	}
	if s.data.Channels == nil {
		s.data.Channels = map[string]Channel{}
	}
	if s.data.Messages == nil {
		s.data.Messages = []Message{}
	}
	if s.data.Mutes == nil {
		s.data.Mutes = map[string]Mute{}
	}
	if _, ok := s.data.Channels["general"]; !ok {
		now := time.Now().UTC()
		s.data.Channels["general"] = Channel{
			Name:      "general",
			Topic:     "Default chat channel",
			CreatedBy: "system",
			CreatedAt: now,
		}
	}
}

func (s *Store) activeMuteLocked(username string) (Mute, bool) {
	username = normalizeUser(username)
	mute, ok := s.data.Mutes[username]
	if !ok {
		return Mute{}, false
	}
	if !mute.Until.After(time.Now().UTC()) {
		delete(s.data.Mutes, username)
		return Mute{}, false
	}
	return mute, true
}

func (s *Store) purgeExpiredMutesLocked(now time.Time) {
	for username, mute := range s.data.Mutes {
		if !mute.Until.After(now) {
			delete(s.data.Mutes, username)
		}
	}
}

func (s *Store) publish(event Event) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	s.publishLocked(event)
}

func (s *Store) publishLocked(event Event) {
	if event.Time.IsZero() {
		event.Time = time.Now().UTC()
	}

	targets := make([]chan Event, 0)
	if event.Channel != "" {
		for ch := range s.subscribers[event.Channel] {
			targets = append(targets, ch)
		}
	}
	for ch := range s.subscribers[""] {
		targets = append(targets, ch)
	}

	for _, ch := range targets {
		select {
		case ch <- event:
		default:
		}
	}
}

func normalizeChannel(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func normalizeUser(username string) string {
	return strings.ToLower(strings.TrimSpace(username))
}

func validateChannel(name string) error {
	if len(name) < 2 || len(name) > 32 {
		return ErrInvalidChannel
	}
	for _, ch := range name {
		if ch >= 'a' && ch <= 'z' {
			continue
		}
		if ch >= '0' && ch <= '9' {
			continue
		}
		if ch == '_' || ch == '-' {
			continue
		}
		return fmt.Errorf("channel contains invalid character %q", ch)
	}
	return nil
}
