package knowledge

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

type Entry struct {
	ID         string    `json:"id"`
	Text       string    `json:"text"`
	Source     string    `json:"source,omitempty"`
	RecordedAt time.Time `json:"recorded_at"`
}

type Store struct {
	path string
	mu   sync.Mutex
}

func NewStore(path string) *Store {
	return &Store{path: path}
}

func (s *Store) Add(_ context.Context, entry Entry) (Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entries, err := s.readAllLocked()
	if err != nil {
		return Entry{}, err
	}

	if entry.ID == "" {
		entry.ID = newID()
	}
	if entry.RecordedAt.IsZero() {
		entry.RecordedAt = time.Now()
	}

	entries = append(entries, entry)
	if err := s.writeAllLocked(entries); err != nil {
		return Entry{}, err
	}
	return entry, nil
}

func (s *Store) List(_ context.Context) ([]Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entries, err := s.readAllLocked()
	if err != nil {
		return nil, err
	}

	slices.SortFunc(entries, func(a, b Entry) int {
		switch {
		case a.RecordedAt.Before(b.RecordedAt):
			return -1
		case a.RecordedAt.After(b.RecordedAt):
			return 1
		default:
			return 0
		}
	})
	return entries, nil
}

func (s *Store) Clear(_ context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writeAllLocked(nil)
}

func (s *Store) Remove(_ context.Context, idOrPrefix string) (Entry, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entries, err := s.readAllLocked()
	if err != nil {
		return Entry{}, false, err
	}

	match := normalizeEntryID(idOrPrefix)
	for index, entry := range entries {
		if strings.HasPrefix(normalizeEntryID(entry.ID), match) {
			removed := entry
			entries = append(entries[:index], entries[index+1:]...)
			if err := s.writeAllLocked(entries); err != nil {
				return Entry{}, false, err
			}
			return removed, true, nil
		}
	}
	return Entry{}, false, nil
}

func (s *Store) Append(_ context.Context, idOrPrefix, addition string) (Entry, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entries, err := s.readAllLocked()
	if err != nil {
		return Entry{}, false, err
	}

	match := normalizeEntryID(idOrPrefix)
	for index, entry := range entries {
		if strings.HasPrefix(normalizeEntryID(entry.ID), match) {
			entry.Text = mergeEntryText(entry.Text, addition)
			entries[index] = entry
			if err := s.writeAllLocked(entries); err != nil {
				return Entry{}, false, err
			}
			return entry, true, nil
		}
	}
	return Entry{}, false, nil
}

func (s *Store) AppendLatest(_ context.Context, source, addition string) (Entry, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entries, err := s.readAllLocked()
	if err != nil {
		return Entry{}, false, err
	}

	selectedIndex := -1
	for index, entry := range entries {
		if strings.TrimSpace(source) != "" && entry.Source != source {
			continue
		}
		if selectedIndex == -1 || entry.RecordedAt.After(entries[selectedIndex].RecordedAt) {
			selectedIndex = index
		}
	}

	if selectedIndex == -1 {
		return Entry{}, false, nil
	}

	entry := entries[selectedIndex]
	entry.Text = mergeEntryText(entry.Text, addition)
	entries[selectedIndex] = entry
	if err := s.writeAllLocked(entries); err != nil {
		return Entry{}, false, err
	}
	return entry, true, nil
}

func (s *Store) readAllLocked() ([]Entry, error) {
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
	var entries []Entry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

func (s *Store) writeAllLocked(entries []Entry) error {
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(entries, "", "  ")
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

func normalizeEntryID(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "#")
	return strings.ToLower(value)
}

func mergeEntryText(base, addition string) string {
	base = strings.TrimSpace(base)
	addition = strings.TrimSpace(addition)

	switch {
	case base == "":
		return addition
	case addition == "":
		return base
	default:
		return base + "\n" + addition
	}
}
