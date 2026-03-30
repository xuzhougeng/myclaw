package sessionstate

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

type Snapshot struct {
	Key          string    `json:"key"`
	Mode         string    `json:"mode,omitempty"`
	LoadedSkills []string  `json:"loaded_skills,omitempty"`
	PromptID     string    `json:"prompt_id,omitempty"`
	History      []Message `json:"history,omitempty"`
	UpdatedAt    time.Time `json:"updated_at,omitempty"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Store struct {
	path string
	mu   sync.Mutex
}

func NewStore(path string) *Store {
	return &Store{path: path}
}

func (s *Store) Load(_ context.Context, key string) (Snapshot, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	items, err := s.readAllLocked()
	if err != nil {
		return Snapshot{}, false, err
	}

	snapshot, ok := items[normalizeKey(key)]
	if !ok {
		return Snapshot{}, false, nil
	}
	return snapshot, true, nil
}

func (s *Store) Save(_ context.Context, snapshot Snapshot) (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	items, err := s.readAllLocked()
	if err != nil {
		return Snapshot{}, err
	}

	snapshot.Key = normalizeKey(snapshot.Key)
	snapshot.Mode = strings.TrimSpace(snapshot.Mode)
	snapshot.UpdatedAt = time.Now()
	items[snapshot.Key] = snapshot

	if err := s.writeAllLocked(items); err != nil {
		return Snapshot{}, err
	}
	return snapshot, nil
}

func (s *Store) List(_ context.Context) ([]Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	items, err := s.readAllLocked()
	if err != nil {
		return nil, err
	}

	out := make([]Snapshot, 0, len(items))
	for _, snapshot := range items {
		out = append(out, snapshot)
	}
	slices.SortFunc(out, func(left, right Snapshot) int {
		switch {
		case left.UpdatedAt.After(right.UpdatedAt):
			return -1
		case left.UpdatedAt.Before(right.UpdatedAt):
			return 1
		default:
			return strings.Compare(left.Key, right.Key)
		}
	})
	return out, nil
}

func (s *Store) readAllLocked() (map[string]Snapshot, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]Snapshot{}, nil
		}
		return nil, err
	}
	if len(data) == 0 {
		return map[string]Snapshot{}, nil
	}

	var items map[string]Snapshot
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, err
	}
	if items == nil {
		items = map[string]Snapshot{}
	}
	return items, nil
}

func (s *Store) writeAllLocked(items map[string]Snapshot) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return err
	}

	tmpPath := s.path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpPath, s.path)
}

func normalizeKey(key string) string {
	return strings.TrimSpace(key)
}
