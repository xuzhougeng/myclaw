package main

import (
	"path/filepath"
	"testing"
	"time"

	appsvc "baize/internal/app"
	"baize/internal/knowledge"
	"baize/internal/projectstate"
	"baize/internal/promptlib"
	"baize/internal/reminder"
	"baize/internal/screentrace"
	"baize/internal/sessionstate"
)

func TestDesktopScreenTraceAPIsExposeRecordsAndDigests(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	appDBPath := filepath.Join(root, "app.db")
	store := knowledge.NewStore(appDBPath)
	projectStore := projectstate.NewStore(appDBPath)
	promptStore := promptlib.NewStore(appDBPath)
	reminders := reminder.NewManager(reminder.NewStore(appDBPath))
	sessionStore := sessionstate.NewStore(appDBPath)
	service := appsvc.NewServiceWithRuntime(store, nil, reminders, nil, sessionStore, promptStore)

	screenStore := screentrace.NewStore(appDBPath)
	capturedAt := time.Date(2026, 4, 2, 10, 30, 0, 0, time.UTC)
	record, err := screenStore.AddRecord(t.Context(), screentrace.Record{
		CapturedAt:     capturedAt,
		ImagePath:      filepath.Join(root, "screentrace", "shot.jpg"),
		ImageHash:      "1234567890abcdef",
		Width:          1440,
		Height:         900,
		DisplayIndex:   0,
		SceneSummary:   "current_user 正在阅读文档并修改代码",
		VisibleText:    []string{"README", "ScreenTrace"},
		Apps:           []string{"VS Code"},
		TaskGuess:      "实现 screentrace",
		Keywords:       []string{"screentrace", "docs"},
		SensitiveLevel: "low",
		Confidence:     0.88,
	})
	if err != nil {
		t.Fatalf("add screentrace record: %v", err)
	}
	_, err = screenStore.UpsertDigest(t.Context(), screentrace.Digest{
		BucketStart:      capturedAt.Truncate(15 * time.Minute),
		BucketEnd:        capturedAt.Truncate(15 * time.Minute).Add(15 * time.Minute),
		RecordCount:      1,
		Summary:          "这一段时间主要在实现 ScreenTrace。",
		Keywords:         []string{"screentrace"},
		DominantApps:     []string{"VS Code"},
		DominantTasks:    []string{"实现"},
		WrittenToKB:      true,
		KnowledgeEntryID: "kb1234",
	})
	if err != nil {
		t.Fatalf("upsert screentrace digest: %v", err)
	}

	app := NewDesktopApp(root, store, promptStore, projectStore, nil, nil, service, sessionStore, reminders, nil)

	status, err := app.GetScreenTraceStatus()
	if err != nil {
		t.Fatalf("get screentrace status: %v", err)
	}
	if status.TotalRecords != 1 {
		t.Fatalf("expected totalRecords=1, got %#v", status)
	}

	records, err := app.ListScreenTraceRecords(10)
	if err != nil {
		t.Fatalf("list screentrace records: %v", err)
	}
	if len(records) != 1 || records[0].ID != record.ID {
		t.Fatalf("unexpected screentrace records: %#v", records)
	}

	digests, err := app.ListScreenTraceDigests(10)
	if err != nil {
		t.Fatalf("list screentrace digests: %v", err)
	}
	if len(digests) != 1 || !digests[0].WrittenToKB {
		t.Fatalf("unexpected screentrace digests: %#v", digests)
	}
}
