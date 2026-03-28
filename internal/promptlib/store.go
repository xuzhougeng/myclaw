package promptlib

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

type Prompt struct {
	ID         string    `json:"id"`
	Title      string    `json:"title"`
	Content    string    `json:"content"`
	RecordedAt time.Time `json:"recorded_at"`
}

type Store struct {
	path string
	mu   sync.Mutex
}

func NewStore(path string) *Store {
	return &Store{path: path}
}

func (s *Store) Add(_ context.Context, prompt Prompt) (Prompt, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	prompts, err := s.readAllLocked()
	if err != nil {
		return Prompt{}, err
	}

	if prompt.ID == "" {
		prompt.ID = newID()
	}
	if prompt.RecordedAt.IsZero() {
		prompt.RecordedAt = time.Now()
	}
	prompt.Title = strings.TrimSpace(prompt.Title)
	prompt.Content = strings.TrimSpace(prompt.Content)

	prompts = append(prompts, prompt)
	if err := s.writeAllLocked(prompts); err != nil {
		return Prompt{}, err
	}
	return prompt, nil
}

func (s *Store) List(_ context.Context) ([]Prompt, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	prompts, err := s.readAllLocked()
	if err != nil {
		return nil, err
	}

	slices.SortFunc(prompts, func(a, b Prompt) int {
		switch {
		case a.RecordedAt.Before(b.RecordedAt):
			return -1
		case a.RecordedAt.After(b.RecordedAt):
			return 1
		default:
			return 0
		}
	})
	return prompts, nil
}

func (s *Store) Remove(_ context.Context, idOrPrefix string) (Prompt, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	prompts, err := s.readAllLocked()
	if err != nil {
		return Prompt{}, false, err
	}

	match := normalizeID(idOrPrefix)
	for index, prompt := range prompts {
		if strings.HasPrefix(normalizeID(prompt.ID), match) {
			removed := prompt
			prompts = append(prompts[:index], prompts[index+1:]...)
			if err := s.writeAllLocked(prompts); err != nil {
				return Prompt{}, false, err
			}
			return removed, true, nil
		}
	}
	return Prompt{}, false, nil
}

func (s *Store) Clear(_ context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writeAllLocked(nil)
}

func (s *Store) readAllLocked() ([]Prompt, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if len(data) == 0 {
		return nil, nil
	}
	var prompts []Prompt
	if err := json.Unmarshal(data, &prompts); err != nil {
		return nil, err
	}
	return prompts, nil
}

func (s *Store) writeAllLocked(prompts []Prompt) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(prompts, "", "  ")
	if err != nil {
		return err
	}

	tmpPath := s.path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpPath, s.path)
}

func newID() string {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return time.Now().Format("20060102150405")
	}
	return hex.EncodeToString(buf[:])
}

func normalizeID(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "#")
	return strings.ToLower(value)
}
