package sessionstate

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestStoreListReturnsNewestSnapshotsFirst(t *testing.T) {
	t.Parallel()

	store := NewStore(filepath.Join(t.TempDir(), "sessions.json"))

	if _, err := store.Save(context.Background(), Snapshot{Key: "session:oldest"}); err != nil {
		t.Fatalf("save oldest: %v", err)
	}
	time.Sleep(2 * time.Millisecond)
	if _, err := store.Save(context.Background(), Snapshot{Key: "session:middle"}); err != nil {
		t.Fatalf("save middle: %v", err)
	}
	time.Sleep(2 * time.Millisecond)
	if _, err := store.Save(context.Background(), Snapshot{Key: "session:newest"}); err != nil {
		t.Fatalf("save newest: %v", err)
	}

	items, err := store.List(context.Background())
	if err != nil {
		t.Fatalf("list snapshots: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("expected 3 snapshots, got %#v", items)
	}
	if items[0].Key != "session:newest" || items[1].Key != "session:middle" || items[2].Key != "session:oldest" {
		t.Fatalf("unexpected order: %#v", items)
	}
}
