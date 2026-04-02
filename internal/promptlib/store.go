package promptlib

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"strings"
	"sync"
	"time"

	"baize/internal/sqliteutil"
)

type Prompt struct {
	ID         string    `json:"id"`
	Title      string    `json:"title"`
	Content    string    `json:"content"`
	RecordedAt time.Time `json:"recorded_at"`
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

func (s *Store) Add(ctx context.Context, prompt Prompt) (Prompt, error) {
	if err := s.ensureReady(); err != nil {
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

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO prompts (id, title, content, recorded_at)
		VALUES (?, ?, ?, ?)
	`, prompt.ID, prompt.Title, prompt.Content, formatSQLiteTime(prompt.RecordedAt))
	if err != nil {
		return Prompt{}, err
	}
	return prompt, nil
}

func (s *Store) List(ctx context.Context) ([]Prompt, error) {
	if err := s.ensureReady(); err != nil {
		return nil, err
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, title, content, recorded_at
		FROM prompts
		ORDER BY recorded_at ASC, id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Prompt
	for rows.Next() {
		item, err := scanPrompt(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) Resolve(ctx context.Context, idOrPrefix string) (Prompt, bool, error) {
	if err := s.ensureReady(); err != nil {
		return Prompt{}, false, err
	}

	match := normalizeID(idOrPrefix)
	row := s.db.QueryRowContext(ctx, `
		SELECT id, title, content, recorded_at
		FROM prompts
		WHERE LOWER(id) LIKE ?
		ORDER BY recorded_at ASC, id ASC
		LIMIT 1
	`, match+"%")
	item, ok, err := scanPromptRow(row)
	if err != nil || !ok {
		return Prompt{}, ok, err
	}
	return item, true, nil
}

func (s *Store) Remove(ctx context.Context, idOrPrefix string) (Prompt, bool, error) {
	if err := s.ensureReady(); err != nil {
		return Prompt{}, false, err
	}

	item, ok, err := s.Resolve(ctx, idOrPrefix)
	if err != nil || !ok {
		return Prompt{}, ok, err
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM prompts WHERE id = ?`, item.ID); err != nil {
		return Prompt{}, false, err
	}
	return item, true, nil
}

func (s *Store) Clear(ctx context.Context) error {
	if err := s.ensureReady(); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM prompts`)
	return err
}

func (s *Store) ensureReady() error {
	s.initOnce.Do(func() {
		s.db, s.initErr = sqliteutil.Open(s.path)
		if s.initErr != nil {
			return
		}
		_, s.initErr = s.db.Exec(`
			CREATE TABLE IF NOT EXISTS prompts (
				id TEXT PRIMARY KEY,
				title TEXT NOT NULL,
				content TEXT NOT NULL,
				recorded_at TEXT NOT NULL
			);
			CREATE INDEX IF NOT EXISTS prompts_recorded_idx
				ON prompts (recorded_at, id);
		`)
	})
	return s.initErr
}

func scanPrompt(scanner interface{ Scan(...any) error }) (Prompt, error) {
	var (
		item       Prompt
		recordedAt string
	)
	if err := scanner.Scan(&item.ID, &item.Title, &item.Content, &recordedAt); err != nil {
		return Prompt{}, err
	}
	parsed, err := parseSQLiteTime(recordedAt)
	if err != nil {
		return Prompt{}, err
	}
	item.RecordedAt = parsed
	return item, nil
}

func scanPromptRow(row *sql.Row) (Prompt, bool, error) {
	item, err := scanPrompt(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return Prompt{}, false, nil
		}
		return Prompt{}, false, err
	}
	return item, true, nil
}

func formatSQLiteTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}

func parseSQLiteTime(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339Nano, value)
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
