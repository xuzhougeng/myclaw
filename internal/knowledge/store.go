package knowledge

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"baize/internal/sqliteutil"
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
	path     string
	db       *sql.DB
	initOnce sync.Once
	initErr  error
}

func NewStore(path string) *Store {
	return &Store{path: path}
}

func (s *Store) Add(ctx context.Context, entry Entry) (Entry, error) {
	if err := s.ensureReady(); err != nil {
		return Entry{}, err
	}

	if entry.ID == "" {
		entry.ID = newID()
	}
	if entry.RecordedAt.IsZero() {
		entry.RecordedAt = time.Now()
	}
	entry.Source = strings.TrimSpace(entry.Source)
	entry.Project = canonicalEntryProject(ctx, entry.Project)
	entry.Keywords = MergeKeywords(entry.Keywords, GenerateKeywords(entry.Text))
	if err := s.ensureProject(ctx, entry.Project); err != nil {
		return Entry{}, err
	}

	keywordsJSON, err := json.Marshal(entry.Keywords)
	if err != nil {
		return Entry{}, err
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO knowledge_entries (id, text, keywords_json, source, project, recorded_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, entry.ID, entry.Text, string(keywordsJSON), entry.Source, entry.Project, formatSQLiteTime(entry.RecordedAt))
	if err != nil {
		return Entry{}, err
	}
	return entry, nil
}

func (s *Store) EnsureProject(ctx context.Context, project string) (ProjectInfo, error) {
	if err := s.ensureReady(); err != nil {
		return ProjectInfo{}, err
	}
	project = CanonicalProjectName(project)
	if err := s.ensureProject(ctx, project); err != nil {
		return ProjectInfo{}, err
	}
	return s.projectInfo(ctx, project)
}

func (s *Store) List(ctx context.Context) ([]Entry, error) {
	if err := s.ensureReady(); err != nil {
		return nil, err
	}
	return s.listEntries(ctx, ProjectFromContext(ctx))
}

func (s *Store) Clear(ctx context.Context) error {
	if err := s.ensureReady(); err != nil {
		return err
	}

	project := strings.TrimSpace(ProjectFromContext(ctx))
	if project == "" {
		_, err := s.db.ExecContext(ctx, `DELETE FROM knowledge_entries`)
		return err
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM knowledge_entries WHERE project = ?`, CanonicalProjectName(project))
	return err
}

func (s *Store) Search(ctx context.Context, query string, extraKeywords []string, limit int) ([]SearchResult, error) {
	if err := s.ensureReady(); err != nil {
		return nil, err
	}
	entries, err := s.listEntries(ctx, ProjectFromContext(ctx))
	if err != nil {
		return nil, err
	}
	return RankEntries(entries, query, extraKeywords, limit), nil
}

func (s *Store) BackfillKeywords(ctx context.Context) (int, error) {
	if err := s.ensureReady(); err != nil {
		return 0, err
	}

	entries, err := s.listEntries(ctx, "")
	if err != nil {
		return 0, err
	}

	updated := 0
	err = sqliteutil.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		for _, entry := range entries {
			keywords := MergeKeywords(entry.Keywords, GenerateKeywords(entry.Text))
			if equalStringSlices(entry.Keywords, keywords) {
				continue
			}
			keywordsJSON, err := json.Marshal(keywords)
			if err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `UPDATE knowledge_entries SET keywords_json = ? WHERE id = ?`, string(keywordsJSON), entry.ID); err != nil {
				return err
			}
			updated++
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return updated, nil
}

func (s *Store) Remove(ctx context.Context, idOrPrefix string) (Entry, bool, error) {
	if err := s.ensureReady(); err != nil {
		return Entry{}, false, err
	}

	entry, ok, err := s.findByPrefix(ctx, idOrPrefix, ProjectFromContext(ctx), false)
	if err != nil || !ok {
		return Entry{}, ok, err
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM knowledge_entries WHERE id = ?`, entry.ID); err != nil {
		return Entry{}, false, err
	}
	return entry, true, nil
}

func (s *Store) Append(ctx context.Context, idOrPrefix, addition string) (Entry, bool, error) {
	if err := s.ensureReady(); err != nil {
		return Entry{}, false, err
	}

	entry, ok, err := s.findByPrefix(ctx, idOrPrefix, ProjectFromContext(ctx), false)
	if err != nil || !ok {
		return Entry{}, ok, err
	}
	entry.Text = mergeEntryText(entry.Text, addition)
	entry.Keywords = GenerateKeywords(entry.Text)
	keywordsJSON, err := json.Marshal(entry.Keywords)
	if err != nil {
		return Entry{}, false, err
	}
	if _, err := s.db.ExecContext(ctx, `
		UPDATE knowledge_entries
		SET text = ?, keywords_json = ?, source = ?, project = ?, recorded_at = ?
		WHERE id = ?
	`, entry.Text, string(keywordsJSON), entry.Source, entry.Project, formatSQLiteTime(entry.RecordedAt), entry.ID); err != nil {
		return Entry{}, false, err
	}
	return entry, true, nil
}

func (s *Store) AppendLatest(ctx context.Context, source, addition string) (Entry, bool, error) {
	if err := s.ensureReady(); err != nil {
		return Entry{}, false, err
	}

	project := strings.TrimSpace(ProjectFromContext(ctx))
	source = strings.TrimSpace(source)
	query := `
		SELECT id, text, keywords_json, source, project, recorded_at
		FROM knowledge_entries
		WHERE (? = '' OR source = ?)
		  AND (? = '' OR project = ?)
		ORDER BY recorded_at DESC, id DESC
		LIMIT 1
	`
	row := s.db.QueryRowContext(ctx, query, source, source, canonicalProjectFilter(project), canonicalProjectFilter(project))
	entry, ok, err := scanKnowledgeEntry(row)
	if err != nil || !ok {
		return Entry{}, ok, err
	}

	entry.Text = mergeEntryText(entry.Text, addition)
	entry.Keywords = GenerateKeywords(entry.Text)
	keywordsJSON, err := json.Marshal(entry.Keywords)
	if err != nil {
		return Entry{}, false, err
	}
	if _, err := s.db.ExecContext(ctx, `
		UPDATE knowledge_entries
		SET text = ?, keywords_json = ?
		WHERE id = ?
	`, entry.Text, string(keywordsJSON), entry.ID); err != nil {
		return Entry{}, false, err
	}
	return entry, true, nil
}

func (s *Store) ListProjects(ctx context.Context) ([]ProjectInfo, error) {
	if err := s.ensureReady(); err != nil {
		return nil, err
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT
			p.name,
			COUNT(e.id),
			COALESCE(MAX(e.recorded_at), p.created_at)
		FROM knowledge_projects p
		LEFT JOIN knowledge_entries e ON e.project = p.name
		GROUP BY p.name, p.created_at
		ORDER BY COALESCE(MAX(e.recorded_at), p.created_at) DESC, LOWER(p.name) ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ProjectInfo
	for rows.Next() {
		var (
			project  string
			count    int
			recorded string
		)
		if err := rows.Scan(&project, &count, &recorded); err != nil {
			return nil, err
		}
		recordedAt, err := parseSQLiteTime(recorded)
		if err != nil {
			return nil, err
		}
		out = append(out, ProjectInfo{
			Name:             CanonicalProjectName(project),
			KnowledgeCount:   count,
			LatestRecordedAt: recordedAt,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

func (s *Store) ensureReady() error {
	s.initOnce.Do(func() {
		s.db, s.initErr = sqliteutil.Open(s.path)
		if s.initErr != nil {
			return
		}
		_, s.initErr = s.db.Exec(`
			CREATE TABLE IF NOT EXISTS knowledge_projects (
				name TEXT PRIMARY KEY,
				created_at TEXT NOT NULL
			);
			CREATE TABLE IF NOT EXISTS knowledge_entries (
				id TEXT PRIMARY KEY,
				text TEXT NOT NULL,
				keywords_json TEXT NOT NULL DEFAULT '[]',
				source TEXT NOT NULL DEFAULT '',
				project TEXT NOT NULL DEFAULT 'default',
				recorded_at TEXT NOT NULL
			);
			CREATE INDEX IF NOT EXISTS knowledge_entries_project_recorded_idx
				ON knowledge_entries (project, recorded_at);
			CREATE INDEX IF NOT EXISTS knowledge_entries_source_recorded_idx
				ON knowledge_entries (source, recorded_at);
		`)
		if s.initErr != nil {
			return
		}
		s.initErr = s.ensureProject(context.Background(), DefaultProjectName)
	})
	return s.initErr
}

func (s *Store) listEntries(ctx context.Context, project string) ([]Entry, error) {
	query := `
		SELECT id, text, keywords_json, source, project, recorded_at
		FROM knowledge_entries
		WHERE (? = '' OR project = ?)
		ORDER BY recorded_at ASC, id ASC
	`
	project = canonicalProjectFilter(project)
	rows, err := s.db.QueryContext(ctx, query, project, project)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Entry
	for rows.Next() {
		entry, err := scanKnowledgeRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) findByPrefix(ctx context.Context, idOrPrefix, project string, newest bool) (Entry, bool, error) {
	prefix := normalizeEntryID(idOrPrefix)
	if prefix == "" {
		return Entry{}, false, nil
	}

	order := "ASC"
	if newest {
		order = "DESC"
	}
	query := fmt.Sprintf(`
		SELECT id, text, keywords_json, source, project, recorded_at
		FROM knowledge_entries
		WHERE LOWER(id) LIKE ?
		  AND (? = '' OR project = ?)
		ORDER BY recorded_at %s, id %s
		LIMIT 1
	`, order, order)
	project = canonicalProjectFilter(project)
	row := s.db.QueryRowContext(ctx, query, prefix+"%", project, project)
	return scanKnowledgeEntry(row)
}

func (s *Store) ensureProject(ctx context.Context, project string) error {
	project = CanonicalProjectName(project)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO knowledge_projects (name, created_at)
		VALUES (?, ?)
		ON CONFLICT(name) DO NOTHING
	`, project, formatSQLiteTime(time.Now()))
	return err
}

func (s *Store) projectInfo(ctx context.Context, project string) (ProjectInfo, error) {
	project = CanonicalProjectName(project)
	row := s.db.QueryRowContext(ctx, `
		SELECT
			p.name,
			COUNT(e.id),
			COALESCE(MAX(e.recorded_at), p.created_at)
		FROM knowledge_projects p
		LEFT JOIN knowledge_entries e ON e.project = p.name
		WHERE p.name = ?
		GROUP BY p.name, p.created_at
	`, project)

	var (
		name     string
		count    int
		recorded string
	)
	if err := row.Scan(&name, &count, &recorded); err != nil {
		return ProjectInfo{}, err
	}
	recordedAt, err := parseSQLiteTime(recorded)
	if err != nil {
		return ProjectInfo{}, err
	}
	return ProjectInfo{
		Name:             CanonicalProjectName(name),
		KnowledgeCount:   count,
		LatestRecordedAt: recordedAt,
	}, nil
}

func scanKnowledgeRows(scanner interface{ Scan(...any) error }) (Entry, error) {
	var (
		entry        Entry
		keywordsJSON string
		recordedAt   string
	)
	if err := scanner.Scan(&entry.ID, &entry.Text, &keywordsJSON, &entry.Source, &entry.Project, &recordedAt); err != nil {
		return Entry{}, err
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(keywordsJSON)), &entry.Keywords); err != nil {
		return Entry{}, err
	}
	t, err := parseSQLiteTime(recordedAt)
	if err != nil {
		return Entry{}, err
	}
	entry.RecordedAt = t
	entry.Project = CanonicalProjectName(entry.Project)
	return entry, nil
}

func scanKnowledgeEntry(row *sql.Row) (Entry, bool, error) {
	entry, err := scanKnowledgeRows(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return Entry{}, false, nil
		}
		return Entry{}, false, err
	}
	return entry, true, nil
}

func canonicalEntryProject(ctx context.Context, project string) string {
	switch {
	case strings.TrimSpace(ProjectFromContext(ctx)) != "":
		return CanonicalProjectName(ProjectFromContext(ctx))
	case strings.TrimSpace(project) != "":
		return CanonicalProjectName(project)
	default:
		return CanonicalProjectName("")
	}
}

func canonicalProjectFilter(project string) string {
	project = strings.TrimSpace(project)
	if project == "" {
		return ""
	}
	return CanonicalProjectName(project)
}

func formatSQLiteTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}

func parseSQLiteTime(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339Nano, value)
}

func equalStringSlices(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
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
