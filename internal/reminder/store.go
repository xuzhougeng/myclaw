package reminder

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"
)

type Frequency string

const (
	FrequencyOnce  Frequency = "once"
	FrequencyDaily Frequency = "daily"
)

type Target struct {
	Interface string `json:"interface"`
	UserID    string `json:"user_id"`
}

type Reminder struct {
	ID          string    `json:"id"`
	Target      Target    `json:"target"`
	Message     string    `json:"message"`
	Frequency   Frequency `json:"frequency"`
	NextRunAt   time.Time `json:"next_run_at"`
	DailyHour   int       `json:"daily_hour,omitempty"`
	DailyMinute int       `json:"daily_minute,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Store struct {
	path string
	mu   sync.Mutex
}

func NewStore(path string) *Store {
	return &Store{path: path}
}

func (s *Store) List(_ context.Context) ([]Reminder, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	items, err := s.readAllLocked()
	if err != nil {
		return nil, err
	}
	slices.SortFunc(items, func(a, b Reminder) int {
		switch {
		case a.NextRunAt.Before(b.NextRunAt):
			return -1
		case a.NextRunAt.After(b.NextRunAt):
			return 1
		default:
			return 0
		}
	})
	return items, nil
}

func (s *Store) SaveAll(_ context.Context, items []Reminder) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writeAllLocked(items)
}

func (s *Store) Add(ctx context.Context, item Reminder) (Reminder, error) {
	items, err := s.List(ctx)
	if err != nil {
		return Reminder{}, err
	}
	if item.ID == "" {
		item.ID = newID()
	}
	now := time.Now()
	if item.CreatedAt.IsZero() {
		item.CreatedAt = now
	}
	item.UpdatedAt = now
	items = append(items, item)
	if err := s.SaveAll(ctx, items); err != nil {
		return Reminder{}, err
	}
	return item, nil
}

func (s *Store) readAllLocked() ([]Reminder, error) {
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
	var items []Reminder
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, err
	}
	return items, nil
}

func (s *Store) writeAllLocked(items []Reminder) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func newID() string {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return time.Now().Format("20060102150405")
	}
	return hex.EncodeToString(buf[:])
}
