package main

import (
	"path/filepath"
	"testing"

	appsvc "myclaw/internal/app"
	"myclaw/internal/knowledge"
	"myclaw/internal/projectstate"
	"myclaw/internal/promptlib"
	"myclaw/internal/reminder"
	"myclaw/internal/sessionstate"
)

func TestDesktopChatSessionsCanBeCreatedAndSwitched(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store := knowledge.NewStore(filepath.Join(root, "knowledge.json"))
	projectStore := projectstate.NewStore(filepath.Join(root, "project.json"))
	promptStore := promptlib.NewStore(filepath.Join(root, "prompts.json"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(root, "reminders.json")))
	sessionStore := sessionstate.NewStore(filepath.Join(root, "sessions.json"))
	service := appsvc.NewServiceWithRuntime(store, nil, reminders, nil, sessionStore, promptStore)
	app := NewDesktopApp(root, store, promptStore, projectStore, nil, nil, service, sessionStore, reminders, nil)

	first, err := app.GetChatState()
	if err != nil {
		t.Fatalf("get first chat state: %v", err)
	}
	if first.SessionID == "" {
		t.Fatal("expected initial session id")
	}
	if len(first.Conversations) != 1 || !first.Conversations[0].Active {
		t.Fatalf("unexpected initial conversations: %#v", first.Conversations)
	}

	second, err := app.NewChatSession()
	if err != nil {
		t.Fatalf("new chat session: %v", err)
	}
	if second.SessionID == "" || second.SessionID == first.SessionID {
		t.Fatalf("expected distinct session ids, got first=%q second=%q", first.SessionID, second.SessionID)
	}
	if len(second.Conversations) != 2 {
		t.Fatalf("expected 2 conversations after creating new session, got %#v", second.Conversations)
	}

	switched, err := app.SwitchChatSession(first.SessionID)
	if err != nil {
		t.Fatalf("switch back to first session: %v", err)
	}
	if switched.SessionID != first.SessionID {
		t.Fatalf("expected switched session %q, got %#v", first.SessionID, switched)
	}
	activeCount := 0
	for _, conversation := range switched.Conversations {
		if conversation.Active {
			activeCount++
		}
	}
	if activeCount != 1 {
		t.Fatalf("expected exactly one active conversation, got %#v", switched.Conversations)
	}
}

func TestDesktopSendMessageNewConversationReturnsSessionChanged(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store := knowledge.NewStore(filepath.Join(root, "knowledge.json"))
	projectStore := projectstate.NewStore(filepath.Join(root, "project.json"))
	promptStore := promptlib.NewStore(filepath.Join(root, "prompts.json"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(root, "reminders.json")))
	sessionStore := sessionstate.NewStore(filepath.Join(root, "sessions.json"))
	service := appsvc.NewServiceWithRuntime(store, nil, reminders, nil, sessionStore, promptStore)
	app := NewDesktopApp(root, store, promptStore, projectStore, nil, nil, service, sessionStore, reminders, nil)

	first, err := app.GetChatState()
	if err != nil {
		t.Fatalf("get chat state: %v", err)
	}

	result, err := app.SendMessage("/new")
	if err != nil {
		t.Fatalf("send /new: %v", err)
	}
	if !result.SessionChanged {
		t.Fatalf("expected session changed response, got %#v", result)
	}
	if result.SessionID == "" || result.SessionID == first.SessionID {
		t.Fatalf("expected a new session id, got first=%q result=%#v", first.SessionID, result)
	}
}
