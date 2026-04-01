package reminder

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestManagerRunDueOnceReminder(t *testing.T) {
	t.Parallel()

	store := NewStore(filepath.Join(t.TempDir(), "app.db"))
	manager := NewManager(store)
	now := time.Date(2026, 3, 27, 10, 0, 0, 0, time.Local)
	manager.now = func() time.Time { return now }

	target := Target{Interface: "terminal", UserID: "terminal"}
	notified := false
	manager.RegisterNotifier(target, NotifierFunc(func(ctx context.Context, item Reminder) error {
		notified = true
		return nil
	}))

	if _, err := manager.ScheduleAfter(context.Background(), target, 2*time.Hour, "开会"); err != nil {
		t.Fatalf("schedule after: %v", err)
	}

	if err := manager.runDue(context.Background(), now.Add(3*time.Hour)); err != nil {
		t.Fatalf("run due: %v", err)
	}
	if !notified {
		t.Fatal("expected notifier to be called")
	}
	items, err := manager.List(context.Background(), target)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected once reminder removed, got %d", len(items))
	}
}

func TestManagerRunDueDailyReminder(t *testing.T) {
	t.Parallel()

	store := NewStore(filepath.Join(t.TempDir(), "app.db"))
	manager := NewManager(store)
	now := time.Date(2026, 3, 27, 8, 0, 0, 0, time.Local)
	manager.now = func() time.Time { return now }

	target := Target{Interface: "terminal", UserID: "terminal"}
	count := 0
	manager.RegisterNotifier(target, NotifierFunc(func(ctx context.Context, item Reminder) error {
		count++
		return nil
	}))

	item, err := manager.ScheduleDaily(context.Background(), target, 9, 0, "写日报")
	if err != nil {
		t.Fatalf("schedule daily: %v", err)
	}
	if item.NextRunAt.Hour() != 9 {
		t.Fatalf("unexpected first next run: %v", item.NextRunAt)
	}

	if err := manager.runDue(context.Background(), time.Date(2026, 3, 27, 9, 1, 0, 0, time.Local)); err != nil {
		t.Fatalf("run due: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected notifier once, got %d", count)
	}

	items, err := manager.List(context.Background(), target)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected daily reminder preserved, got %d", len(items))
	}
	if !items[0].NextRunAt.After(time.Date(2026, 3, 27, 9, 1, 0, 0, time.Local)) {
		t.Fatalf("expected next run advanced, got %v", items[0].NextRunAt)
	}
}

func TestManagerRunDueMultipleOnceReminders(t *testing.T) {
	t.Parallel()

	store := NewStore(filepath.Join(t.TempDir(), "app.db"))
	manager := NewManager(store)
	now := time.Date(2026, 3, 27, 10, 0, 0, 0, time.Local)
	manager.now = func() time.Time { return now }

	target := Target{Interface: "weixin", UserID: "user-1"}
	var notified []string
	manager.RegisterNotifier(target, NotifierFunc(func(ctx context.Context, item Reminder) error {
		notified = append(notified, item.Message)
		return nil
	}))

	if _, err := store.Add(context.Background(), Reminder{
		Target:    target,
		Message:   "喝水",
		Frequency: FrequencyOnce,
		NextRunAt: now.Add(-2 * time.Minute),
	}); err != nil {
		t.Fatalf("add first overdue reminder: %v", err)
	}
	if _, err := store.Add(context.Background(), Reminder{
		Target:    target,
		Message:   "站起来活动",
		Frequency: FrequencyOnce,
		NextRunAt: now.Add(-1 * time.Minute),
	}); err != nil {
		t.Fatalf("add second overdue reminder: %v", err)
	}

	if err := manager.runDue(context.Background(), now); err != nil {
		t.Fatalf("run due: %v", err)
	}
	if len(notified) != 2 {
		t.Fatalf("expected two notifications, got %#v", notified)
	}

	items, err := manager.List(context.Background(), target)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected once reminders removed, got %#v", items)
	}
}
