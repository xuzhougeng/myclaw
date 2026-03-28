package projectstate

import (
	"context"
	"path/filepath"
	"testing"
)

func TestStoreLoadDefaultsToDefaultProject(t *testing.T) {
	t.Parallel()

	store := NewStore(filepath.Join(t.TempDir(), "active.json"))
	snapshot, err := store.Load(context.Background())
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if snapshot.ActiveProject != "default" {
		t.Fatalf("expected default project, got %#v", snapshot)
	}
}

func TestStoreSaveAndLoad(t *testing.T) {
	t.Parallel()

	store := NewStore(filepath.Join(t.TempDir(), "active.json"))
	if _, err := store.Save(context.Background(), "FastClaw"); err != nil {
		t.Fatalf("save: %v", err)
	}

	snapshot, err := store.Load(context.Background())
	if err != nil {
		t.Fatalf("load after save: %v", err)
	}
	if snapshot.ActiveProject != "FastClaw" {
		t.Fatalf("unexpected snapshot: %#v", snapshot)
	}
}
