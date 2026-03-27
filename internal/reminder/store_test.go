package reminder

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestStoreAddAndList(t *testing.T) {
	t.Parallel()

	store := NewStore(filepath.Join(t.TempDir(), "reminders.json"))
	first, err := store.Add(context.Background(), Reminder{
		Target:    Target{Interface: "terminal", UserID: "u1"},
		Message:   "喝水",
		Frequency: FrequencyOnce,
		NextRunAt: time.Now().Add(2 * time.Hour),
	})
	if err != nil {
		t.Fatalf("add first: %v", err)
	}
	if first.ID == "" {
		t.Fatal("expected reminder id")
	}

	items, err := store.List(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 reminder, got %d", len(items))
	}
	if items[0].Message != "喝水" {
		t.Fatalf("unexpected message: %#v", items[0])
	}
}
