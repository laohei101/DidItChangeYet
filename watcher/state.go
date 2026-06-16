package watcher

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

// WatchState holds the persisted state for a single watch.
type WatchState struct {
	LastValue string    `json:"last_value"`
	LastCheck time.Time `json:"last_check"`
	LastError string    `json:"last_error,omitempty"`
	Triggered bool      `json:"triggered"`
	CheckCount int      `json:"check_count"`
}

type stateFile struct {
	Watches map[string]*WatchState `json:"watches"`
}

// StateStore persists watch states to a JSON file with mutex protection.
type StateStore struct {
	mu   sync.Mutex
	path string
	data stateFile
}

// NewStateStore opens (or creates) the state file at path.
func NewStateStore(path string) (*StateStore, error) {
	s := &StateStore{
		path: path,
		data: stateFile{Watches: make(map[string]*WatchState)},
	}
	if err := s.load(); err != nil {
		return nil, fmt.Errorf("loading state from %q: %w", path, err)
	}
	return s, nil
}

func (s *StateStore) load() error {
	data, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(data, &s.data)
}

// Get returns the WatchState for the given watch ID (nil if not yet seen).
func (s *StateStore) Get(id string) *WatchState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.data.Watches[id]
}

// Set persists updated state for a watch ID.
func (s *StateStore) Set(id string, ws *WatchState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.Watches[id] = ws
	return s.save()
}

// All returns a copy of all watch states (for the dashboard).
func (s *StateStore) All() map[string]WatchState {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]WatchState, len(s.data.Watches))
	for k, v := range s.data.Watches {
		out[k] = *v
	}
	return out
}

func (s *StateStore) save() error {
	data, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	// Write atomically via temp file + rename
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}
