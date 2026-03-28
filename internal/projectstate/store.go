package projectstate

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"myclaw/internal/knowledge"
)

type Snapshot struct {
	ActiveProject string `json:"activeProject"`
}

type Store struct {
	path string
	mu   sync.Mutex
}

func NewStore(path string) *Store {
	return &Store{path: path}
}

func (s *Store) Load(_ context.Context) (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadLocked()
}

func (s *Store) Save(_ context.Context, project string) (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	snapshot := Snapshot{
		ActiveProject: canonicalProject(project),
	}
	if err := s.writeLocked(snapshot); err != nil {
		return Snapshot{}, err
	}
	return snapshot, nil
}

func (s *Store) loadLocked() (Snapshot, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return Snapshot{ActiveProject: knowledge.DefaultProjectName}, nil
		}
		return Snapshot{}, err
	}
	if len(data) == 0 {
		return Snapshot{ActiveProject: knowledge.DefaultProjectName}, nil
	}

	var snapshot Snapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return Snapshot{}, err
	}
	snapshot.ActiveProject = canonicalProject(snapshot.ActiveProject)
	return snapshot, nil
}

func (s *Store) writeLocked(snapshot Snapshot) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return err
	}

	tmpPath := s.path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpPath, s.path)
}

func canonicalProject(project string) string {
	project = strings.TrimSpace(project)
	if project == "" {
		return knowledge.DefaultProjectName
	}
	return project
}
