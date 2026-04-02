package main

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"baize/internal/reminder"
)

func TestListRemindersAggregatesSourcesAndFormatsOutput(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	root := t.TempDir()
	store := reminder.NewStore(filepath.Join(root, "app.db"))
	manager := reminder.NewManager(store)

	createdAt := time.Date(2026, 3, 28, 9, 0, 0, 0, time.Local)
	if _, err := store.Add(ctx, reminder.Reminder{
		Target:    reminder.Target{Interface: desktopInterface, UserID: desktopUserID},
		Message:   "交房租",
		Frequency: reminder.FrequencyOnce,
		NextRunAt: time.Date(2026, 3, 30, 14, 0, 0, 0, time.Local),
		CreatedAt: createdAt,
	}); err != nil {
		t.Fatalf("seed once reminder: %v", err)
	}
	if _, err := store.Add(ctx, reminder.Reminder{
		Target:      reminder.Target{Interface: desktopInterface, UserID: desktopUserID},
		Message:     "写日报",
		Frequency:   reminder.FrequencyDaily,
		NextRunAt:   time.Date(2026, 3, 31, 9, 30, 0, 0, time.Local),
		DailyHour:   9,
		DailyMinute: 30,
		CreatedAt:   createdAt.Add(10 * time.Minute),
	}); err != nil {
		t.Fatalf("seed daily reminder: %v", err)
	}
	if _, err := store.Add(ctx, reminder.Reminder{
		Target:    reminder.Target{Interface: "weixin", UserID: "user-a"},
		Message:   "做下行程管理",
		Frequency: reminder.FrequencyOnce,
		NextRunAt: time.Date(2026, 3, 29, 9, 0, 0, 0, time.Local),
		CreatedAt: createdAt,
	}); err != nil {
		t.Fatalf("seed weixin reminder: %v", err)
	}

	app := &DesktopApp{reminders: manager}
	items, err := app.ListReminders()
	if err != nil {
		t.Fatalf("list reminders: %v", err)
	}

	if len(items) != 3 {
		t.Fatalf("expected 3 reminders, got %d", len(items))
	}

	if items[0].Message != "做下行程管理" || items[0].Source != "weixin:user-a" || items[0].SourceLabel != "微信" {
		t.Fatalf("unexpected first reminder: %#v", items[0])
	}
	if items[0].Frequency != string(reminder.FrequencyOnce) || items[0].FrequencyLabel != "单次" || items[0].ScheduleLabel != "单次" {
		t.Fatalf("unexpected weixin reminder formatting: %#v", items[0])
	}

	if items[1].Message != "交房租" || items[1].Source != "desktop:primary" || items[1].SourceLabel != "桌面" {
		t.Fatalf("unexpected second reminder: %#v", items[1])
	}
	if items[1].NextRunAt != "2026-03-30 14:00:00" {
		t.Fatalf("unexpected once next run: %#v", items[1])
	}

	if items[2].Message != "写日报" || items[2].SourceLabel != "桌面" {
		t.Fatalf("unexpected third reminder: %#v", items[2])
	}
	if items[2].Frequency != string(reminder.FrequencyDaily) || items[2].FrequencyLabel != "每天" || items[2].ScheduleLabel != "每天 09:30" {
		t.Fatalf("unexpected daily reminder formatting: %#v", items[2])
	}
	if items[2].CreatedAt != "2026-03-28 09:10:00" {
		t.Fatalf("unexpected daily created at: %#v", items[2])
	}
}

func TestListRemindersWithoutManagerReturnsEmptySlice(t *testing.T) {
	t.Parallel()

	items, err := (&DesktopApp{}).ListReminders()
	if err != nil {
		t.Fatalf("list reminders without manager: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected empty reminders, got %#v", items)
	}
}
