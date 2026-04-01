package app

import (
	"context"
	"path/filepath"
	"testing"

	"myclaw/internal/knowledge"
	"myclaw/internal/reminder"
	"myclaw/internal/sessionstate"
)

func TestAppendWithExplicitFinalSummary(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store := knowledge.NewStore(filepath.Join(root, "entries.json"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(root, "reminders.json")))
	stateStore := sessionstate.NewStore(filepath.Join(root, "sessions.json"))
	service := NewServiceWithSkillsAndSessions(store, nil, reminders, nil, stateStore)
	mc := MessageContext{Interface: "terminal", UserID: "u1", SessionID: "s1"}

	ctx := context.Background()
	service.appendConversationHistoryWithSummary(ctx, mc, "hello", "world", "explicit summary")

	snapshot, ok, err := stateStore.Load(ctx, conversationSessionKey(mc))
	if err != nil {
		t.Fatalf("load snapshot: %v", err)
	}
	if !ok {
		t.Fatal("expected saved snapshot")
	}
	if len(snapshot.History) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(snapshot.History))
	}
	assistant := snapshot.History[1]
	if assistant.ContextSummary != "explicit summary" {
		t.Fatalf("expected ContextSummary = %q, got %q", "explicit summary", assistant.ContextSummary)
	}
	if assistant.Content != "world" {
		t.Fatalf("expected Content = %q, got %q", "world", assistant.Content)
	}
}

func TestAppendFallsBackToReplyWhenSummaryEmpty(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store := knowledge.NewStore(filepath.Join(root, "entries.json"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(root, "reminders.json")))
	stateStore := sessionstate.NewStore(filepath.Join(root, "sessions.json"))
	service := NewServiceWithSkillsAndSessions(store, nil, reminders, nil, stateStore)
	mc := MessageContext{Interface: "terminal", UserID: "u2", SessionID: "s2"}

	ctx := context.Background()
	service.appendConversationHistoryWithSummary(ctx, mc, "ping", "pong", "")

	snapshot, ok, err := stateStore.Load(ctx, conversationSessionKey(mc))
	if err != nil {
		t.Fatalf("load snapshot: %v", err)
	}
	if !ok {
		t.Fatal("expected saved snapshot")
	}
	if len(snapshot.History) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(snapshot.History))
	}
	assistant := snapshot.History[1]
	if assistant.ContextSummary != "pong" {
		t.Fatalf("expected ContextSummary to fall back to reply %q, got %q", "pong", assistant.ContextSummary)
	}
}
