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
	Keywords   []string  `json:"keywords,omitempty"`
	Source     string    `json:"source,omitempty"`
	Project    string    `json:"project,omitempty"`
	RecordedAt time.Time `json:"recorded_at"`
}

type ProjectInfo struct {
	Name             string
	KnowledgeCount   int
	LatestRecordedAt time.Time
}

type Store struct {
	path string
	mu   sync.Mutex
}

func NewStore(path string) *Store {
	return &Store{path: path}
}

func (s *Store) Add(ctx context.Context, entry Entry) (Entry, error) {
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
	if project := ProjectFromContext(ctx); project != "" {
		entry.Project = CanonicalProjectName(project)
	} else if strings.TrimSpace(entry.Project) != "" {
		entry.Project = CanonicalProjectName(entry.Project)
	}
	entry.Keywords = MergeKeywords(entry.Keywords, GenerateKeywords(entry.Text))

	entries = append(entries, entry)
	if err := s.writeAllLocked(entries); err != nil {
		return Entry{}, err
	}
	return entry, nil
}

func (s *Store) List(ctx context.Context) ([]Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entries, err := s.readAllLocked()
	if err != nil {
		return nil, err
	}
	entries = filterEntriesByProject(entries, ProjectFromContext(ctx))

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

func (s *Store) Clear(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	project := ProjectFromContext(ctx)
	if project == "" {
		return s.writeAllLocked(nil)
	}

	entries, err := s.readAllLocked()
	if err != nil {
		return err
	}

	filtered := make([]Entry, 0, len(entries))
	for _, entry := range entries {
		if sameProject(entry.Project, project) {
			continue
		}
		filtered = append(filtered, entry)
	}
	return s.writeAllLocked(filtered)
}

func (s *Store) Search(ctx context.Context, query string, extraKeywords []string, limit int) ([]SearchResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entries, err := s.readAllLocked()
	if err != nil {
		return nil, err
	}
	entries = filterEntriesByProject(entries, ProjectFromContext(ctx))
	return RankEntries(entries, query, extraKeywords, limit), nil
}

func (s *Store) BackfillKeywords(_ context.Context) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entries, err := s.readAllLocked()
	if err != nil {
		return 0, err
	}

	updated := 0
	for index, entry := range entries {
		keywords := MergeKeywords(entry.Keywords, GenerateKeywords(entry.Text))
		if slices.Equal(entry.Keywords, keywords) {
			continue
		}
		entry.Keywords = keywords
		entries[index] = entry
		updated++
	}

	if updated == 0 {
		return 0, nil
	}
	if err := s.writeAllLocked(entries); err != nil {
		return 0, err
	}
	return updated, nil
}

func (s *Store) Remove(ctx context.Context, idOrPrefix string) (Entry, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entries, err := s.readAllLocked()
	if err != nil {
		return Entry{}, false, err
	}

	match := normalizeEntryID(idOrPrefix)
	project := ProjectFromContext(ctx)
	for index, entry := range entries {
		if project != "" && !sameProject(entry.Project, project) {
			continue
		}
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

func (s *Store) Append(ctx context.Context, idOrPrefix, addition string) (Entry, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entries, err := s.readAllLocked()
	if err != nil {
		return Entry{}, false, err
	}

	match := normalizeEntryID(idOrPrefix)
	project := ProjectFromContext(ctx)
	for index, entry := range entries {
		if project != "" && !sameProject(entry.Project, project) {
			continue
		}
		if strings.HasPrefix(normalizeEntryID(entry.ID), match) {
			entry.Text = mergeEntryText(entry.Text, addition)
			entry.Keywords = GenerateKeywords(entry.Text)
			entries[index] = entry
			if err := s.writeAllLocked(entries); err != nil {
				return Entry{}, false, err
			}
			return entry, true, nil
		}
	}
	return Entry{}, false, nil
}

func (s *Store) AppendLatest(ctx context.Context, source, addition string) (Entry, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entries, err := s.readAllLocked()
	if err != nil {
		return Entry{}, false, err
	}

	selectedIndex := -1
	project := ProjectFromContext(ctx)
	for index, entry := range entries {
		if strings.TrimSpace(source) != "" && entry.Source != source {
			continue
		}
		if project != "" && !sameProject(entry.Project, project) {
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
	entry.Keywords = GenerateKeywords(entry.Text)
	entries[selectedIndex] = entry
	if err := s.writeAllLocked(entries); err != nil {
		return Entry{}, false, err
	}
	return entry, true, nil
}

func (s *Store) ListProjects(_ context.Context) ([]ProjectInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entries, err := s.readAllLocked()
	if err != nil {
		return nil, err
	}

	projects := make(map[string]ProjectInfo)
	for _, entry := range entries {
		key := normalizedProjectKey(entry.Project)
		info := projects[key]
		if info.Name == "" {
			info.Name = CanonicalProjectName(entry.Project)
		}
		info.KnowledgeCount++
		if entry.RecordedAt.After(info.LatestRecordedAt) {
			info.LatestRecordedAt = entry.RecordedAt
		}
		projects[key] = info
	}

	if len(projects) == 0 {
		return nil, nil
	}

	out := make([]ProjectInfo, 0, len(projects))
	for _, info := range projects {
		out = append(out, info)
	}

	slices.SortFunc(out, func(a, b ProjectInfo) int {
		switch {
		case a.LatestRecordedAt.After(b.LatestRecordedAt):
			return -1
		case a.LatestRecordedAt.Before(b.LatestRecordedAt):
			return 1
		default:
			return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
		}
	})
	return out, nil
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

func filterEntriesByProject(entries []Entry, project string) []Entry {
	project = strings.TrimSpace(project)
	if project == "" {
		return append([]Entry(nil), entries...)
	}

	filtered := make([]Entry, 0, len(entries))
	for _, entry := range entries {
		if sameProject(entry.Project, project) {
			filtered = append(filtered, entry)
		}
	}
	return filtered
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
