package knowledge

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestStoreAddListAndClear(t *testing.T) {
	t.Parallel()

	store := NewStore(filepath.Join(t.TempDir(), "entries.json"))
	ctx := context.Background()

	if _, err := store.Add(ctx, Entry{
		Text:       "first",
		RecordedAt: time.Date(2026, 3, 27, 10, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("add first: %v", err)
	}
	if _, err := store.Add(ctx, Entry{
		Text:       "second",
		RecordedAt: time.Date(2026, 3, 27, 11, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("add second: %v", err)
	}

	entries, err := store.List(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Text != "first" || entries[1].Text != "second" {
		t.Fatalf("unexpected order: %#v", entries)
	}

	if err := store.Clear(ctx); err != nil {
		t.Fatalf("clear: %v", err)
	}
	entries, err = store.List(ctx)
	if err != nil {
		t.Fatalf("list after clear: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected empty store after clear, got %d entries", len(entries))
	}
}

func TestStoreRemoveByPrefix(t *testing.T) {
	t.Parallel()

	store := NewStore(filepath.Join(t.TempDir(), "entries.json"))
	ctx := context.Background()

	entry, err := store.Add(ctx, Entry{
		ID:         "0015f908abcd1234",
		Text:       "drink water",
		RecordedAt: time.Date(2026, 3, 27, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("add entry: %v", err)
	}

	removed, ok, err := store.Remove(ctx, "#0015f908")
	if err != nil {
		t.Fatalf("remove: %v", err)
	}
	if !ok {
		t.Fatalf("expected entry to be removed")
	}
	if removed.ID != entry.ID {
		t.Fatalf("unexpected removed entry: %#v", removed)
	}

	entries, err := store.List(ctx)
	if err != nil {
		t.Fatalf("list after remove: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected empty store after remove, got %d entries", len(entries))
	}
}

func TestStoreAppendByPrefix(t *testing.T) {
	t.Parallel()

	store := NewStore(filepath.Join(t.TempDir(), "entries.json"))
	ctx := context.Background()

	if _, err := store.Add(ctx, Entry{
		ID:         "6d2d7724abcd1234",
		Text:       "Puppeteer 是一个浏览器自动化工具。",
		RecordedAt: time.Date(2026, 3, 27, 12, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("add entry: %v", err)
	}

	updated, ok, err := store.Append(ctx, "#6d2d7724", "它是 Google 出品的一个工具。")
	if err != nil {
		t.Fatalf("append: %v", err)
	}
	if !ok {
		t.Fatalf("expected entry to be appended")
	}
	if updated.Text != "Puppeteer 是一个浏览器自动化工具。\n它是 Google 出品的一个工具。" {
		t.Fatalf("unexpected updated text: %q", updated.Text)
	}
}

func TestStoreAppendLatestBySource(t *testing.T) {
	t.Parallel()

	store := NewStore(filepath.Join(t.TempDir(), "entries.json"))
	ctx := context.Background()

	if _, err := store.Add(ctx, Entry{
		ID:         "11111111aaaa1111",
		Text:       "old same source",
		Source:     "weixin:u1",
		RecordedAt: time.Date(2026, 3, 27, 10, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("add old same source: %v", err)
	}
	if _, err := store.Add(ctx, Entry{
		ID:         "22222222bbbb2222",
		Text:       "latest other source",
		Source:     "weixin:u2",
		RecordedAt: time.Date(2026, 3, 27, 12, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("add other source: %v", err)
	}
	if _, err := store.Add(ctx, Entry{
		ID:         "33333333cccc3333",
		Text:       "latest same source",
		Source:     "weixin:u1",
		RecordedAt: time.Date(2026, 3, 27, 11, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("add latest same source: %v", err)
	}

	updated, ok, err := store.AppendLatest(ctx, "weixin:u1", "补充内容")
	if err != nil {
		t.Fatalf("append latest: %v", err)
	}
	if !ok {
		t.Fatalf("expected entry to be appended")
	}
	if updated.ID != "33333333cccc3333" {
		t.Fatalf("expected latest same-source entry, got %#v", updated)
	}
	if updated.Text != "latest same source\n补充内容" {
		t.Fatalf("unexpected updated text: %q", updated.Text)
	}
}
