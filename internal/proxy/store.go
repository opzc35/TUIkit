package proxy

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

var (
	ErrRouteExists   = errors.New("route already exists")
	ErrRouteNotFound = errors.New("route not found")
	ErrInvalidRoute  = errors.New("route name must be 2-32 characters")
	ErrInvalidURL    = errors.New("upstream URL is required and must start with http:// or https://")
)

type Route struct {
	Name       string    `json:"name"`
	Upstream   string    `json:"upstream"`
	PathPrefix string    `json:"path_prefix"`
	APIKey     string    `json:"api_key"`
	KeyHeader  string    `json:"key_header"`
	Enabled    bool      `json:"enabled"`
	CreatedBy  string    `json:"created_by"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type Store struct {
	path   string
	mu     sync.RWMutex
	routes map[string]Route
}

func OpenStore(path string) (*Store, error) {
	store := &Store{
		path:   path,
		routes: map[string]Route{},
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

	if err := json.Unmarshal(data, &store.routes); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *Store) CreateRoute(name, upstream, pathPrefix, apiKey, keyHeader, createdBy string) error {
	name = normalizeName(name)
	if err := validateName(name); err != nil {
		return err
	}
	upstream = strings.TrimSpace(upstream)
	if upstream == "" || (!strings.HasPrefix(upstream, "http://") && !strings.HasPrefix(upstream, "https://")) {
		return ErrInvalidURL
	}
	upstream = strings.TrimRight(upstream, "/")
	pathPrefix = "/" + strings.Trim(strings.TrimSpace(pathPrefix), "/")
	if pathPrefix == "/" {
		pathPrefix = ""
	}
	if keyHeader == "" {
		keyHeader = "Authorization"
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.routes[name]; exists {
		return ErrRouteExists
	}

	now := time.Now().UTC()
	s.routes[name] = Route{
		Name:       name,
		Upstream:   upstream,
		PathPrefix: pathPrefix,
		APIKey:     strings.TrimSpace(apiKey),
		KeyHeader:  keyHeader,
		Enabled:    true,
		CreatedBy:  strings.TrimSpace(createdBy),
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	return s.saveLocked()
}

func (s *Store) DeleteRoute(name string) error {
	name = normalizeName(name)

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.routes[name]; !ok {
		return ErrRouteNotFound
	}
	delete(s.routes, name)
	return s.saveLocked()
}

func (s *Store) SetEnabled(name string, enabled bool) error {
	name = normalizeName(name)

	s.mu.Lock()
	defer s.mu.Unlock()

	route, ok := s.routes[name]
	if !ok {
		return ErrRouteNotFound
	}
	route.Enabled = enabled
	route.UpdatedAt = time.Now().UTC()
	s.routes[name] = route
	return s.saveLocked()
}

func (s *Store) ListRoutes() []Route {
	s.mu.RLock()
	defer s.mu.RUnlock()

	routes := make([]Route, 0, len(s.routes))
	for _, r := range s.routes {
		routes = append(routes, r)
	}
	sort.Slice(routes, func(i, j int) bool {
		return routes[i].Name < routes[j].Name
	})
	return routes
}

func (s *Store) EnabledRoutes() []Route {
	s.mu.RLock()
	defer s.mu.RUnlock()

	routes := make([]Route, 0, len(s.routes))
	for _, r := range s.routes {
		if r.Enabled {
			routes = append(routes, r)
		}
	}
	sort.Slice(routes, func(i, j int) bool {
		return routes[i].Name < routes[j].Name
	})
	return routes
}

func (s *Store) MatchRoute(path string) (Route, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, r := range s.routes {
		if !r.Enabled {
			continue
		}
		prefix := r.PathPrefix
		if prefix == "" {
			prefix = "/"
		}
		if strings.HasPrefix(path, prefix) {
			return r, true
		}
	}
	return Route{}, false
}

func (s *Store) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s.routes, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o600)
}

func normalizeName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func validateName(name string) error {
	if len(name) < 2 || len(name) > 32 {
		return ErrInvalidRoute
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
		return fmt.Errorf("route name contains invalid character %q", ch)
	}
	return nil
}