package promptlib

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestStoreAddListRemoveAndClear(t *testing.T) {
	t.Parallel()

	store := NewStore(filepath.Join(t.TempDir(), "prompts.json"))

	first, err := store.Add(context.Background(), Prompt{
		Title:      "日报整理",
		Content:    "请把下面内容整理成日报。",
		RecordedAt: time.Date(2026, 3, 28, 10, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("add first prompt: %v", err)
	}
	if first.ID == "" {
		t.Fatalf("expected generated id")
	}

	if _, err := store.Add(context.Background(), Prompt{
		Title:      "代码解释",
		Content:    "请解释这段代码。",
		RecordedAt: time.Date(2026, 3, 28, 11, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("add second prompt: %v", err)
	}

	items, err := store.List(context.Background())
	if err != nil {
		t.Fatalf("list prompts: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 prompts, got %d", len(items))
	}
	if items[0].Title != "日报整理" || items[1].Title != "代码解释" {
		t.Fatalf("unexpected order: %#v", items)
	}

	removed, ok, err := store.Remove(context.Background(), first.ID[:8])
	if err != nil {
		t.Fatalf("remove prompt: %v", err)
	}
	if !ok || removed.ID != first.ID {
		t.Fatalf("unexpected removed prompt: %#v %v", removed, ok)
	}

	items, err = store.List(context.Background())
	if err != nil {
		t.Fatalf("list after remove: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 prompt after remove, got %d", len(items))
	}

	if err := store.Clear(context.Background()); err != nil {
		t.Fatalf("clear prompts: %v", err)
	}
	items, err = store.List(context.Background())
	if err != nil {
		t.Fatalf("list after clear: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected no prompts after clear, got %d", len(items))
	}
}
