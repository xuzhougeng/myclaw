package projectstate

import (
	"context"
	"database/sql"
	"strings"
	"sync"

	"baize/internal/knowledge"
	"baize/internal/sqliteutil"
)

const projectStateRowID = "primary"

type Snapshot struct {
	ActiveProject string `json:"activeProject"`
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

func (s *Store) Load(ctx context.Context) (Snapshot, error) {
	return s.LoadScope(ctx, projectStateRowID)
}

func (s *Store) LoadScope(ctx context.Context, scopeID string) (Snapshot, error) {
	if err := s.ensureReady(); err != nil {
		return Snapshot{}, err
	}

	row := s.db.QueryRowContext(ctx, `SELECT active_project FROM project_state WHERE id = ?`, canonicalScopeID(scopeID))
	var project string
	if err := row.Scan(&project); err != nil {
		if err == sql.ErrNoRows {
			return Snapshot{ActiveProject: knowledge.DefaultProjectName}, nil
		}
		return Snapshot{}, err
	}
	return Snapshot{ActiveProject: canonicalProject(project)}, nil
}

func (s *Store) Save(ctx context.Context, project string) (Snapshot, error) {
	return s.SaveScope(ctx, projectStateRowID, project)
}

func (s *Store) SaveScope(ctx context.Context, scopeID string, project string) (Snapshot, error) {
	if err := s.ensureReady(); err != nil {
		return Snapshot{}, err
	}

	snapshot := Snapshot{ActiveProject: canonicalProject(project)}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO project_state (id, active_project)
		VALUES (?, ?)
		ON CONFLICT(id) DO UPDATE SET active_project = excluded.active_project
	`, canonicalScopeID(scopeID), snapshot.ActiveProject)
	if err != nil {
		return Snapshot{}, err
	}
	return snapshot, nil
}

func (s *Store) ensureReady() error {
	s.initOnce.Do(func() {
		s.db, s.initErr = sqliteutil.Open(s.path)
		if s.initErr != nil {
			return
		}
		_, s.initErr = s.db.Exec(`
			CREATE TABLE IF NOT EXISTS project_state (
				id TEXT PRIMARY KEY,
				active_project TEXT NOT NULL
			)
		`)
	})
	return s.initErr
}

func canonicalProject(project string) string {
	project = strings.TrimSpace(project)
	if project == "" {
		return knowledge.DefaultProjectName
	}
	return project
}

func canonicalScopeID(scopeID string) string {
	scopeID = strings.TrimSpace(scopeID)
	if scopeID == "" {
		return projectStateRowID
	}
	return scopeID
}
