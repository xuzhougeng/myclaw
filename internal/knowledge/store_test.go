package knowledge

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
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

func TestStoreScopesEntriesByProject(t *testing.T) {
	t.Parallel()

	store := NewStore(filepath.Join(t.TempDir(), "entries.json"))
	ctx := context.Background()
	alphaCtx := WithProject(ctx, "Alpha")
	betaCtx := WithProject(ctx, "Beta")

	if _, err := store.Add(alphaCtx, Entry{
		ID:         "alpha111aaaa1111",
		Text:       "alpha memory",
		RecordedAt: time.Date(2026, 3, 27, 9, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("add alpha entry: %v", err)
	}
	if _, err := store.Add(betaCtx, Entry{
		ID:         "beta2222bbbb2222",
		Text:       "beta memory",
		RecordedAt: time.Date(2026, 3, 27, 10, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("add beta entry: %v", err)
	}

	alphaEntries, err := store.List(alphaCtx)
	if err != nil {
		t.Fatalf("list alpha: %v", err)
	}
	if len(alphaEntries) != 1 || alphaEntries[0].Text != "alpha memory" {
		t.Fatalf("unexpected alpha entries: %#v", alphaEntries)
	}

	betaEntries, err := store.List(betaCtx)
	if err != nil {
		t.Fatalf("list beta: %v", err)
	}
	if len(betaEntries) != 1 || betaEntries[0].Text != "beta memory" {
		t.Fatalf("unexpected beta entries: %#v", betaEntries)
	}

	allEntries, err := store.List(ctx)
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	if len(allEntries) != 2 {
		t.Fatalf("expected 2 global entries, got %d", len(allEntries))
	}

	if _, ok, err := store.Remove(alphaCtx, "#beta2222"); err != nil {
		t.Fatalf("remove beta from alpha scope: %v", err)
	} else if ok {
		t.Fatalf("expected beta entry to stay invisible from alpha scope")
	}
}

func TestStoreAppendLatestAndProjectsRespectProjectScope(t *testing.T) {
	t.Parallel()

	store := NewStore(filepath.Join(t.TempDir(), "entries.json"))
	ctx := context.Background()
	alphaCtx := WithProject(ctx, "Alpha")
	betaCtx := WithProject(ctx, "Beta")

	if _, err := store.Add(alphaCtx, Entry{
		ID:         "11111111aaaa1111",
		Text:       "alpha old",
		Source:     "desktop:primary",
		RecordedAt: time.Date(2026, 3, 27, 9, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("add alpha old: %v", err)
	}
	if _, err := store.Add(alphaCtx, Entry{
		ID:         "22222222aaaa2222",
		Text:       "alpha latest",
		Source:     "desktop:primary",
		RecordedAt: time.Date(2026, 3, 27, 11, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("add alpha latest: %v", err)
	}
	if _, err := store.Add(betaCtx, Entry{
		ID:         "33333333bbbb3333",
		Text:       "beta latest",
		Source:     "desktop:primary",
		RecordedAt: time.Date(2026, 3, 27, 12, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("add beta latest: %v", err)
	}
	if _, err := store.Add(ctx, Entry{
		ID:         "44444444cccc4444",
		Text:       "legacy default",
		RecordedAt: time.Date(2026, 3, 27, 8, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("add legacy default: %v", err)
	}

	updated, ok, err := store.AppendLatest(alphaCtx, "desktop:primary", "alpha extra")
	if err != nil {
		t.Fatalf("append latest alpha: %v", err)
	}
	if !ok {
		t.Fatalf("expected alpha latest entry to be appended")
	}
	if updated.ID != "22222222aaaa2222" {
		t.Fatalf("expected alpha latest entry, got %#v", updated)
	}

	projects, err := store.ListProjects(ctx)
	if err != nil {
		t.Fatalf("list projects: %v", err)
	}
	if len(projects) != 3 {
		t.Fatalf("expected 3 projects, got %#v", projects)
	}

	projectNames := []string{projects[0].Name, projects[1].Name, projects[2].Name}
	if !slices.Contains(projectNames, "Alpha") || !slices.Contains(projectNames, "Beta") || !slices.Contains(projectNames, DefaultProjectName) {
		t.Fatalf("unexpected project names: %#v", projects)
	}
}

func TestGenerateKeywordsAvoidsBrokenChineseFragments(t *testing.T) {
	t.Parallel()

	keywords := GenerateKeywords("怎么解除服务器ip被封")
	for _, expected := range []string{"解除", "服务器", "ip", "被封"} {
		if !slices.Contains(keywords, expected) {
			t.Fatalf("expected keyword %q in %#v", expected, keywords)
		}
	}
	for _, unexpected := range []string{"么解", "除服", "务器"} {
		if slices.Contains(keywords, unexpected) {
			t.Fatalf("did not expect keyword %q in %#v", unexpected, keywords)
		}
	}
}

func TestStoreSearchFindsRelevantEntryFromMixedQuery(t *testing.T) {
	t.Parallel()

	store := NewStore(filepath.Join(t.TempDir(), "entries.json"))
	ctx := context.Background()

	macEntry, err := store.Add(ctx, Entry{
		ID:         "11111111aaaa1111",
		Text:       "未来需要支持 macOS。",
		RecordedAt: time.Date(2026, 3, 27, 10, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("add mac entry: %v", err)
	}
	if _, err := store.Add(ctx, Entry{
		ID:         "22222222bbbb2222",
		Text:       "微信接口先做。",
		RecordedAt: time.Date(2026, 3, 27, 11, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("add weixin entry: %v", err)
	}

	results, err := store.Search(ctx, "macOS什么时候做", nil, 5)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results) == 0 {
		t.Fatalf("expected search results")
	}
	if results[0].Entry.ID != macEntry.ID {
		t.Fatalf("expected macOS entry first, got %#v", results)
	}
	if results[0].Score == 0 {
		t.Fatalf("expected positive score, got %#v", results[0])
	}
}

func TestStoreBackfillKeywords(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "entries.json")
	data := []byte(`[
  {
    "id": "11111111aaaa1111",
    "text": "未来需要支持 macOS。",
    "recorded_at": "2026-03-27T10:00:00Z"
  }
]`)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("seed entries: %v", err)
	}

	store := NewStore(path)
	updated, err := store.BackfillKeywords(context.Background())
	if err != nil {
		t.Fatalf("backfill keywords: %v", err)
	}
	if updated != 1 {
		t.Fatalf("expected 1 updated entry, got %d", updated)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read entries: %v", err)
	}
	var entries []Entry
	if err := json.Unmarshal(raw, &entries); err != nil {
		t.Fatalf("decode entries: %v", err)
	}
	if len(entries) != 1 || !slices.Contains(entries[0].Keywords, "macos") {
		t.Fatalf("expected backfilled keywords, got %#v", entries)
	}
}
