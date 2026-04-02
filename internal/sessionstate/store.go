package sessionstate

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"baize/internal/ai"
	"baize/internal/sqliteutil"
)

type Snapshot struct {
	Key          string    `json:"key"`
	Title        string    `json:"title,omitempty"`
	Mode         string    `json:"mode,omitempty"`
	LoadedSkills []string  `json:"loaded_skills,omitempty"`
	PromptID     string    `json:"prompt_id,omitempty"`
	History      []Message `json:"history,omitempty"`
	UpdatedAt    time.Time `json:"updated_at,omitempty"`
}

type Message struct {
	Role           string             `json:"role"`
	Content        string             `json:"content"`
	ContextSummary string             `json:"context_summary,omitempty"`
	Usage          *ai.TokenUsage     `json:"usage,omitempty"`
	Process        []ai.CallTraceStep `json:"process,omitempty"`
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

func (s *Store) Load(ctx context.Context, key string) (Snapshot, bool, error) {
	if err := s.ensureReady(); err != nil {
		return Snapshot{}, false, err
	}

	row := s.db.QueryRowContext(ctx, `
		SELECT key, title, mode, loaded_skills_json, prompt_id, history_json, updated_at
		FROM session_snapshots
		WHERE key = ?
	`, normalizeKey(key))
	snapshot, ok, err := scanSnapshot(row)
	if err != nil || !ok {
		return Snapshot{}, ok, err
	}
	return snapshot, true, nil
}

func (s *Store) Save(ctx context.Context, snapshot Snapshot) (Snapshot, error) {
	if err := s.ensureReady(); err != nil {
		return Snapshot{}, err
	}

	snapshot.Key = normalizeKey(snapshot.Key)
	snapshot.Title = strings.TrimSpace(snapshot.Title)
	snapshot.Mode = strings.TrimSpace(snapshot.Mode)
	snapshot.PromptID = strings.TrimSpace(snapshot.PromptID)
	snapshot.UpdatedAt = time.Now()

	loadedSkillsJSON, err := json.Marshal(snapshot.LoadedSkills)
	if err != nil {
		return Snapshot{}, err
	}
	historyJSON, err := json.Marshal(snapshot.History)
	if err != nil {
		return Snapshot{}, err
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO session_snapshots (key, title, mode, loaded_skills_json, prompt_id, history_json, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET
			title = excluded.title,
			mode = excluded.mode,
			loaded_skills_json = excluded.loaded_skills_json,
			prompt_id = excluded.prompt_id,
			history_json = excluded.history_json,
			updated_at = excluded.updated_at
	`, snapshot.Key, snapshot.Title, snapshot.Mode, string(loadedSkillsJSON), snapshot.PromptID, string(historyJSON), formatSQLiteTime(snapshot.UpdatedAt))
	if err != nil {
		return Snapshot{}, err
	}
	return snapshot, nil
}

func (s *Store) List(ctx context.Context) ([]Snapshot, error) {
	if err := s.ensureReady(); err != nil {
		return nil, err
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT key, title, mode, loaded_skills_json, prompt_id, history_json, updated_at
		FROM session_snapshots
		ORDER BY updated_at DESC, key ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Snapshot
	for rows.Next() {
		snapshot, _, err := scanSnapshot(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, snapshot)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) Delete(ctx context.Context, key string) error {
	if err := s.ensureReady(); err != nil {
		return err
	}

	_, err := s.db.ExecContext(ctx, `DELETE FROM session_snapshots WHERE key = ?`, normalizeKey(key))
	return err
}

func (s *Store) ensureReady() error {
	s.initOnce.Do(func() {
		s.db, s.initErr = sqliteutil.Open(s.path)
		if s.initErr != nil {
			return
		}
		_, s.initErr = s.db.Exec(`
			CREATE TABLE IF NOT EXISTS session_snapshots (
				key TEXT PRIMARY KEY,
				title TEXT NOT NULL DEFAULT '',
				mode TEXT NOT NULL DEFAULT '',
				loaded_skills_json TEXT NOT NULL DEFAULT '[]',
				prompt_id TEXT NOT NULL DEFAULT '',
				history_json TEXT NOT NULL DEFAULT '[]',
				updated_at TEXT NOT NULL
			);
			CREATE INDEX IF NOT EXISTS session_snapshots_updated_idx
				ON session_snapshots (updated_at DESC, key ASC);
		`)
	})
	return s.initErr
}

func scanSnapshot(scanner interface{ Scan(...any) error }) (Snapshot, bool, error) {
	var (
		snapshot         Snapshot
		loadedSkillsJSON string
		historyJSON      string
		updatedAt        string
	)
	if err := scanner.Scan(&snapshot.Key, &snapshot.Title, &snapshot.Mode, &loadedSkillsJSON, &snapshot.PromptID, &historyJSON, &updatedAt); err != nil {
		if err == sql.ErrNoRows {
			return Snapshot{}, false, nil
		}
		return Snapshot{}, false, err
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(loadedSkillsJSON)), &snapshot.LoadedSkills); err != nil {
		return Snapshot{}, false, err
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(historyJSON)), &snapshot.History); err != nil {
		return Snapshot{}, false, err
	}
	parsed, err := parseSQLiteTime(updatedAt)
	if err != nil {
		return Snapshot{}, false, err
	}
	snapshot.UpdatedAt = parsed
	return snapshot, true, nil
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

func normalizeKey(key string) string {
	return strings.TrimSpace(key)
}
