package main

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"myclaw/internal/ai"
	appsvc "myclaw/internal/app"
	"myclaw/internal/knowledge"
	"myclaw/internal/projectstate"
	"myclaw/internal/promptlib"
	"myclaw/internal/reminder"
	"myclaw/internal/sessionstate"
	"myclaw/internal/weixin"
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

func TestDesktopSendMessageReturnsAndPersistsUsage(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store := knowledge.NewStore(filepath.Join(root, "knowledge.json"))
	projectStore := projectstate.NewStore(filepath.Join(root, "project.json"))
	promptStore := promptlib.NewStore(filepath.Join(root, "prompts.json"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(root, "reminders.json")))
	sessionStore := sessionstate.NewStore(filepath.Join(root, "sessions.json"))
	service := appsvc.NewServiceWithRuntime(store, desktopTestAI{
		route: ai.RouteDecision{
			Command:  "answer",
			Question: "测试 usage",
		},
		chatFunc: func(ctx context.Context, input string, history []ai.ConversationMessage) string {
			if input != "测试 usage" {
				t.Fatalf("unexpected chat input: %q", input)
			}
			ai.AddUsage(ctx, ai.TokenUsage{
				InputTokens:  140,
				OutputTokens: 28,
				CachedTokens: 36,
				TotalTokens:  168,
			})
			return "已返回 usage"
		},
	}, reminders, nil, sessionStore, promptStore)
	app := NewDesktopApp(root, store, promptStore, projectStore, nil, nil, service, sessionStore, reminders, nil)

	result, err := app.SendMessage("测试 usage")
	if err != nil {
		t.Fatalf("send message: %v", err)
	}
	if result.Usage == nil {
		t.Fatalf("expected usage payload, got %#v", result)
	}
	if result.Usage.InputTokens != 140 || result.Usage.OutputTokens != 28 || result.Usage.CachedTokens != 36 || result.Usage.TotalTokens != 168 {
		t.Fatalf("unexpected response usage: %#v", result.Usage)
	}

	state, err := app.GetChatState()
	if err != nil {
		t.Fatalf("get chat state: %v", err)
	}
	if len(state.Messages) != 2 {
		t.Fatalf("expected user+assistant messages, got %#v", state.Messages)
	}
	last := state.Messages[len(state.Messages)-1]
	if last.Usage == nil {
		t.Fatalf("expected persisted usage on assistant message, got %#v", last)
	}
	if last.Usage.InputTokens != 140 || last.Usage.OutputTokens != 28 || last.Usage.CachedTokens != 36 || last.Usage.TotalTokens != 168 {
		t.Fatalf("unexpected persisted usage: %#v", last.Usage)
	}
}

func TestDesktopSendMessageReusesCurrentSession(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store := knowledge.NewStore(filepath.Join(root, "knowledge.json"))
	projectStore := projectstate.NewStore(filepath.Join(root, "project.json"))
	promptStore := promptlib.NewStore(filepath.Join(root, "prompts.json"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(root, "reminders.json")))
	sessionStore := sessionstate.NewStore(filepath.Join(root, "sessions.json"))
	var histories [][]ai.ConversationMessage
	service := appsvc.NewServiceWithRuntime(store, desktopTestAI{
		route: ai.RouteDecision{Command: "answer"},
		chatFunc: func(_ context.Context, input string, history []ai.ConversationMessage) string {
			histories = append(histories, append([]ai.ConversationMessage(nil), history...))
			return "reply:" + input
		},
	}, reminders, nil, sessionStore, promptStore)
	app := NewDesktopApp(root, store, promptStore, projectStore, nil, nil, service, sessionStore, reminders, nil)

	first, err := app.SendMessage("first")
	if err != nil {
		t.Fatalf("send first message: %v", err)
	}
	second, err := app.SendMessage("second")
	if err != nil {
		t.Fatalf("send second message: %v", err)
	}

	if first.SessionID == "" || second.SessionID == "" {
		t.Fatalf("expected session ids, got first=%#v second=%#v", first, second)
	}
	if first.SessionID != second.SessionID {
		t.Fatalf("expected same session, got first=%q second=%q", first.SessionID, second.SessionID)
	}
	if len(histories) != 2 {
		t.Fatalf("expected 2 chat calls, got %d", len(histories))
	}
	if len(histories[0]) != 0 {
		t.Fatalf("expected empty history for first message, got %#v", histories[0])
	}
	if len(histories[1]) != 2 || histories[1][0].Content != "first" || histories[1][1].Content != "reply:first" {
		t.Fatalf("expected second message to reuse prior conversation, got %#v", histories[1])
	}
}

func TestDesktopChatStatePersistsAcrossAppRestart(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store := knowledge.NewStore(filepath.Join(root, "knowledge.json"))
	projectStore := projectstate.NewStore(filepath.Join(root, "project.json"))
	promptStore := promptlib.NewStore(filepath.Join(root, "prompts.json"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(root, "reminders.json")))
	sessionStore := sessionstate.NewStore(filepath.Join(root, "sessions.json"))
	service := appsvc.NewServiceWithRuntime(store, desktopTestAI{
		route: ai.RouteDecision{Command: "answer"},
		chatFunc: func(_ context.Context, input string, history []ai.ConversationMessage) string {
			return "reply:" + input
		},
	}, reminders, nil, sessionStore, promptStore)
	app := NewDesktopApp(root, store, promptStore, projectStore, nil, nil, service, sessionStore, reminders, nil)

	first, err := app.SendMessage("first")
	if err != nil {
		t.Fatalf("send first message: %v", err)
	}

	reloadedService := appsvc.NewServiceWithRuntime(store, nil, reminders, nil, sessionStore, promptStore)
	reloadedApp := NewDesktopApp(root, store, promptStore, projectStore, nil, nil, reloadedService, sessionStore, reminders, nil)
	state, err := reloadedApp.GetChatState()
	if err != nil {
		t.Fatalf("reload chat state: %v", err)
	}

	if state.SessionID != first.SessionID {
		t.Fatalf("expected reload to reuse session %q, got %#v", first.SessionID, state)
	}
	if len(state.Messages) != 2 || state.Messages[0].Text != "first" || state.Messages[1].Text != "reply:first" {
		t.Fatalf("expected reloaded chat history, got %#v", state.Messages)
	}
}

func TestDesktopAppRestartRestoresLastSelectedDesktopSession(t *testing.T) {
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
		t.Fatalf("get first state: %v", err)
	}
	second, err := app.NewChatSession()
	if err != nil {
		t.Fatalf("create second session: %v", err)
	}
	if _, err := app.SwitchChatSession(first.SessionID); err != nil {
		t.Fatalf("switch back to first session: %v", err)
	}

	reloadedService := appsvc.NewServiceWithRuntime(store, nil, reminders, nil, sessionStore, promptStore)
	reloadedApp := NewDesktopApp(root, store, promptStore, projectStore, nil, nil, reloadedService, sessionStore, reminders, nil)
	state, err := reloadedApp.GetChatState()
	if err != nil {
		t.Fatalf("reload chat state: %v", err)
	}

	if state.SessionID != first.SessionID {
		t.Fatalf("expected restart to restore selected session %q, got %#v", first.SessionID, state)
	}
	if state.SessionID == second.SessionID {
		t.Fatalf("expected restart not to jump to newer session %q", second.SessionID)
	}
}

func TestDesktopChatStateIncludesWeixinConversation(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store := knowledge.NewStore(filepath.Join(root, "knowledge.json"))
	projectStore := projectstate.NewStore(filepath.Join(root, "project.json"))
	promptStore := promptlib.NewStore(filepath.Join(root, "prompts.json"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(root, "reminders.json")))
	sessionStore := sessionstate.NewStore(filepath.Join(root, "sessions.json"))
	service := appsvc.NewServiceWithRuntime(store, nil, reminders, nil, sessionStore, promptStore)
	app := NewDesktopApp(root, store, promptStore, projectStore, nil, nil, service, sessionStore, reminders, nil)

	if _, err := app.GetChatState(); err != nil {
		t.Fatalf("prime desktop chat state: %v", err)
	}
	if _, err := sessionStore.Save(context.Background(), sessionstate.Snapshot{
		Key: "source:weixin:user-1|session:weixin:ctx-1",
		History: []sessionstate.Message{
			{Role: "user", Content: "帮我找单细胞 pdf"},
			{Role: "assistant", Content: "找到 2 个文件，回复序号即可发送给你。"},
		},
	}); err != nil {
		t.Fatalf("save weixin snapshot: %v", err)
	}

	state, err := app.GetChatState()
	if err != nil {
		t.Fatalf("get chat state: %v", err)
	}

	found := false
	for _, item := range state.Conversations {
		if item.SessionID != "weixin:ctx-1" {
			continue
		}
		found = true
		if item.ReadOnly {
			t.Fatalf("expected weixin conversation to stay writable in desktop chat: %#v", item)
		}
		if item.Source != "weixin:user-1" || item.SourceLabel != "微信" {
			t.Fatalf("unexpected weixin source metadata: %#v", item)
		}
		if item.Active {
			t.Fatalf("expected desktop conversation to remain active by default: %#v", item)
		}
	}
	if !found {
		t.Fatalf("expected weixin conversation in chat state, got %#v", state.Conversations)
	}

	switched, err := app.SwitchChatSession("weixin:ctx-1")
	if err != nil {
		t.Fatalf("switch to weixin conversation: %v", err)
	}
	if switched.SessionID != "weixin:ctx-1" {
		t.Fatalf("unexpected active session after switch: %#v", switched)
	}
	if len(switched.Messages) != 2 {
		t.Fatalf("expected weixin history to be loaded, got %#v", switched.Messages)
	}
	if switched.Messages[0].Text != "帮我找单细胞 pdf" || switched.Messages[1].Text != "找到 2 个文件，回复序号即可发送给你。" {
		t.Fatalf("unexpected weixin message history: %#v", switched.Messages)
	}
}

func TestDesktopEmitChatChangedActivatesWeixinConversation(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store := knowledge.NewStore(filepath.Join(root, "knowledge.json"))
	projectStore := projectstate.NewStore(filepath.Join(root, "project.json"))
	promptStore := promptlib.NewStore(filepath.Join(root, "prompts.json"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(root, "reminders.json")))
	sessionStore := sessionstate.NewStore(filepath.Join(root, "sessions.json"))
	service := appsvc.NewServiceWithRuntime(store, nil, reminders, nil, sessionStore, promptStore)
	app := NewDesktopApp(root, store, promptStore, projectStore, nil, nil, service, sessionStore, reminders, nil)

	if _, err := app.GetChatState(); err != nil {
		t.Fatalf("prime desktop chat state: %v", err)
	}
	if _, err := sessionStore.Save(context.Background(), sessionstate.Snapshot{
		Key: "source:weixin:user-1|session:weixin:ctx-1",
		History: []sessionstate.Message{
			{Role: "user", Content: "你好"},
			{Role: "assistant", Content: "你好，我在。"},
		},
	}); err != nil {
		t.Fatalf("save weixin snapshot: %v", err)
	}

	app.emitChatChanged(weixin.ConversationUpdate{SessionID: "weixin:ctx-1", Activate: true})

	state, err := app.GetChatState()
	if err != nil {
		t.Fatalf("get chat state: %v", err)
	}
	if state.SessionID != "weixin:ctx-1" {
		t.Fatalf("expected active weixin session, got %#v", state)
	}
	if len(state.Messages) != 2 || state.Messages[0].Text != "你好" || state.Messages[1].Text != "你好，我在。" {
		t.Fatalf("expected activated weixin history, got %#v", state.Messages)
	}
}

func TestDesktopSendMessageContinuesWeixinConversation(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store := knowledge.NewStore(filepath.Join(root, "knowledge.json"))
	projectStore := projectstate.NewStore(filepath.Join(root, "project.json"))
	promptStore := promptlib.NewStore(filepath.Join(root, "prompts.json"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(root, "reminders.json")))
	sessionStore := sessionstate.NewStore(filepath.Join(root, "sessions.json"))
	service := appsvc.NewServiceWithRuntime(store, desktopTestAI{
		route: ai.RouteDecision{Command: "answer"},
		chatFunc: func(_ context.Context, input string, history []ai.ConversationMessage) string {
			if input != "继续说" {
				t.Fatalf("unexpected continued input: %q", input)
			}
			if len(history) != 2 || history[0].Content != "你好" || history[1].Content != "你好，我在。" {
				t.Fatalf("unexpected continued history: %#v", history)
			}
			return "继续聊"
		},
	}, reminders, nil, sessionStore, promptStore)
	app := NewDesktopApp(root, store, promptStore, projectStore, nil, nil, service, sessionStore, reminders, nil)

	if _, err := app.GetChatState(); err != nil {
		t.Fatalf("prime desktop chat state: %v", err)
	}
	if _, err := sessionStore.Save(context.Background(), sessionstate.Snapshot{
		Key: "source:weixin:user-1|session:weixin:ctx-1",
		History: []sessionstate.Message{
			{Role: "user", Content: "你好"},
			{Role: "assistant", Content: "你好，我在。"},
		},
	}); err != nil {
		t.Fatalf("save weixin snapshot: %v", err)
	}
	if _, err := app.SwitchChatSession("weixin:ctx-1"); err != nil {
		t.Fatalf("switch to weixin conversation: %v", err)
	}

	result, err := app.SendMessage("继续说")
	if err != nil {
		t.Fatalf("continue weixin conversation: %v", err)
	}
	if result.SessionID != "weixin:ctx-1" || result.Reply != "继续聊" {
		t.Fatalf("unexpected continued result: %#v", result)
	}

	state, err := app.GetChatState()
	if err != nil {
		t.Fatalf("reload chat state: %v", err)
	}
	if len(state.Messages) != 4 || state.Messages[2].Text != "继续说" || state.Messages[3].Text != "继续聊" {
		t.Fatalf("expected continued weixin history, got %#v", state.Messages)
	}
}

func TestDesktopChatSessionCanBeRenamed(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store := knowledge.NewStore(filepath.Join(root, "knowledge.json"))
	projectStore := projectstate.NewStore(filepath.Join(root, "project.json"))
	promptStore := promptlib.NewStore(filepath.Join(root, "prompts.json"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(root, "reminders.json")))
	sessionStore := sessionstate.NewStore(filepath.Join(root, "sessions.json"))
	service := appsvc.NewServiceWithRuntime(store, nil, reminders, nil, sessionStore, promptStore)
	app := NewDesktopApp(root, store, promptStore, projectStore, nil, nil, service, sessionStore, reminders, nil)

	initial, err := app.GetChatState()
	if err != nil {
		t.Fatalf("get chat state: %v", err)
	}

	renamed, err := app.RenameChatSession(initial.SessionID, "架构讨论")
	if err != nil {
		t.Fatalf("rename chat session: %v", err)
	}
	if renamed.Conversations[0].Title != "架构讨论" || !renamed.Conversations[0].CustomTitle {
		t.Fatalf("unexpected renamed conversation: %#v", renamed.Conversations[0])
	}

	reloaded, err := app.GetChatState()
	if err != nil {
		t.Fatalf("reload chat state: %v", err)
	}
	if reloaded.Conversations[0].Title != "架构讨论" || !reloaded.Conversations[0].CustomTitle {
		t.Fatalf("expected persisted custom title, got %#v", reloaded.Conversations[0])
	}
}

func TestDesktopDeleteChatSessionFallsBackToRemainingConversation(t *testing.T) {
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
	second, err := app.NewChatSession()
	if err != nil {
		t.Fatalf("new chat session: %v", err)
	}

	next, err := app.DeleteChatSession(second.SessionID)
	if err != nil {
		t.Fatalf("delete chat session: %v", err)
	}
	if next.SessionID != first.SessionID {
		t.Fatalf("expected fallback to first session %q, got %#v", first.SessionID, next)
	}
	if len(next.Conversations) != 1 {
		t.Fatalf("expected one remaining conversation, got %#v", next.Conversations)
	}
	if next.Conversations[0].SessionID != first.SessionID || !next.Conversations[0].Active {
		t.Fatalf("unexpected remaining conversation: %#v", next.Conversations[0])
	}
}

func TestDesktopRefreshChatResponseReplacesLatestAssistantReply(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store := knowledge.NewStore(filepath.Join(root, "knowledge.json"))
	projectStore := projectstate.NewStore(filepath.Join(root, "project.json"))
	promptStore := promptlib.NewStore(filepath.Join(root, "prompts.json"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(root, "reminders.json")))
	sessionStore := sessionstate.NewStore(filepath.Join(root, "sessions.json"))
	callCount := 0
	service := appsvc.NewServiceWithRuntime(store, desktopTestAI{
		route: ai.RouteDecision{
			Command:  "answer",
			Question: "重新生成这个回答",
		},
		chatFunc: func(_ context.Context, input string, history []ai.ConversationMessage) string {
			callCount++
			if input != "重新生成这个回答" {
				t.Fatalf("unexpected chat input: %q", input)
			}
			if len(history) != 0 {
				t.Fatalf("expected refresh to replay from the previous turn, got history %#v", history)
			}
			if callCount == 1 {
				return "第一次回答"
			}
			return "第二次回答"
		},
	}, reminders, nil, sessionStore, promptStore)
	app := NewDesktopApp(root, store, promptStore, projectStore, nil, nil, service, sessionStore, reminders, nil)

	if _, err := app.SendMessage("重新生成这个回答"); err != nil {
		t.Fatalf("send initial message: %v", err)
	}

	result, err := app.RefreshChatResponse()
	if err != nil {
		t.Fatalf("refresh chat response: %v", err)
	}
	if result.Reply != "第二次回答" {
		t.Fatalf("unexpected refreshed reply: %#v", result)
	}

	state, err := app.GetChatState()
	if err != nil {
		t.Fatalf("get chat state: %v", err)
	}
	if len(state.Messages) != 2 {
		t.Fatalf("expected refreshed conversation to keep one user/assistant pair, got %#v", state.Messages)
	}
	if state.Messages[0].Role != "user" || state.Messages[0].Text != "重新生成这个回答" {
		t.Fatalf("unexpected user message after refresh: %#v", state.Messages[0])
	}
	if state.Messages[1].Role != "assistant" || state.Messages[1].Text != "第二次回答" {
		t.Fatalf("unexpected assistant message after refresh: %#v", state.Messages[1])
	}
}

func TestDesktopRefreshChatResponseRestoresPreviousReplyOnFailure(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store := knowledge.NewStore(filepath.Join(root, "knowledge.json"))
	projectStore := projectstate.NewStore(filepath.Join(root, "project.json"))
	promptStore := promptlib.NewStore(filepath.Join(root, "prompts.json"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(root, "reminders.json")))
	sessionStore := sessionstate.NewStore(filepath.Join(root, "sessions.json"))
	callCount := 0
	service := appsvc.NewServiceWithRuntime(store, desktopTestAI{
		route: ai.RouteDecision{
			Command:  "answer",
			Question: "保留旧答案",
		},
		chatResultFunc: func(_ context.Context, input string, history []ai.ConversationMessage) (string, error) {
			callCount++
			if input != "保留旧答案" {
				t.Fatalf("unexpected chat input: %q", input)
			}
			if len(history) != 0 {
				t.Fatalf("expected refresh to replay from the previous turn, got history %#v", history)
			}
			if callCount == 1 {
				return "原始回答", nil
			}
			return "", errors.New("刷新失败")
		},
	}, reminders, nil, sessionStore, promptStore)
	app := NewDesktopApp(root, store, promptStore, projectStore, nil, nil, service, sessionStore, reminders, nil)

	if _, err := app.SendMessage("保留旧答案"); err != nil {
		t.Fatalf("send initial message: %v", err)
	}

	if _, err := app.RefreshChatResponse(); err == nil || !strings.Contains(err.Error(), "刷新失败") {
		t.Fatalf("expected refresh failure, got %v", err)
	}

	state, err := app.GetChatState()
	if err != nil {
		t.Fatalf("get chat state: %v", err)
	}
	if len(state.Messages) != 2 {
		t.Fatalf("expected original conversation to be restored, got %#v", state.Messages)
	}
	if state.Messages[1].Role != "assistant" || state.Messages[1].Text != "原始回答" {
		t.Fatalf("expected original assistant reply to be restored, got %#v", state.Messages[1])
	}
}

func TestBuildCurrentChatMarkdownExportFormatsConversation(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store := knowledge.NewStore(filepath.Join(root, "knowledge.json"))
	projectStore := projectstate.NewStore(filepath.Join(root, "project.json"))
	promptStore := promptlib.NewStore(filepath.Join(root, "prompts.json"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(root, "reminders.json")))
	sessionStore := sessionstate.NewStore(filepath.Join(root, "sessions.json"))
	service := appsvc.NewServiceWithRuntime(store, nil, reminders, nil, sessionStore, promptStore)
	app := NewDesktopApp(root, store, promptStore, projectStore, nil, nil, service, sessionStore, reminders, nil)

	state, err := app.GetChatState()
	if err != nil {
		t.Fatalf("get chat state: %v", err)
	}
	if _, err := sessionStore.Save(context.Background(), sessionstate.Snapshot{
		Key: desktopConversationSnapshotKey("default", state.SessionID),
		History: []sessionstate.Message{
			{Role: "user", Content: "导出/测试当前讨论"},
			{Role: "assistant", Content: `{"question":"接下来您想怎么走？","questiontype":"singleselect","options":[{"value":"continue","label":"继续探讨"},{"value":"wrap","label":"结束讨论"}]}`},
			{Role: "user", Content: "我想继续探讨。"},
		},
	}); err != nil {
		t.Fatalf("save chat snapshot: %v", err)
	}

	export, err := app.buildCurrentChatMarkdownExport(context.Background())
	if err != nil {
		t.Fatalf("build markdown export: %v", err)
	}
	if !strings.HasSuffix(export.Filename, ".md") {
		t.Fatalf("expected markdown filename, got %q", export.Filename)
	}
	if strings.Contains(export.Filename, "/") {
		t.Fatalf("expected sanitized filename, got %q", export.Filename)
	}
	for _, want := range []string{
		"# 导出/测试当前讨论",
		"- 项目：default",
		"## 用户",
		"## 助手",
		"接下来您想怎么走？",
		"- 继续探讨 (`continue`)",
		"- 结束讨论 (`wrap`)",
		"我想继续探讨。",
	} {
		if !strings.Contains(export.Markdown, want) {
			t.Fatalf("expected export markdown to contain %q, got:\n%s", want, export.Markdown)
		}
	}
	if strings.Contains(export.Markdown, `"questiontype":"singleselect"`) {
		t.Fatalf("expected option payload to be rendered, got:\n%s", export.Markdown)
	}
}

func TestBuildCurrentChatMarkdownExportFormatsWrappedOptionPayload(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store := knowledge.NewStore(filepath.Join(root, "knowledge.json"))
	projectStore := projectstate.NewStore(filepath.Join(root, "project.json"))
	promptStore := promptlib.NewStore(filepath.Join(root, "prompts.json"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(root, "reminders.json")))
	sessionStore := sessionstate.NewStore(filepath.Join(root, "sessions.json"))
	service := appsvc.NewServiceWithRuntime(store, nil, reminders, nil, sessionStore, promptStore)
	app := NewDesktopApp(root, store, promptStore, projectStore, nil, nil, service, sessionStore, reminders, nil)

	state, err := app.GetChatState()
	if err != nil {
		t.Fatalf("get chat state: %v", err)
	}
	if _, err := sessionStore.Save(context.Background(), sessionstate.Snapshot{
		Key: desktopConversationSnapshotKey("default", state.SessionID),
		History: []sessionstate.Message{
			{Role: "user", Content: "导出/测试包裹选项"},
			{Role: "assistant", Content: "先看下面这个选择题：\n\n```json\n{\"question\":\"接下来您想怎么走？\",\"questiontype\":\"singleselect\",\"options\":[{\"value\":\"continue\",\"label\":\"继续探讨\"},{\"value\":\"mode\",\"label\":\"切换讨论模式\"}]}\n```\n\n你也可以直接输入自己的想法。"},
		},
	}); err != nil {
		t.Fatalf("save chat snapshot: %v", err)
	}

	export, err := app.buildCurrentChatMarkdownExport(context.Background())
	if err != nil {
		t.Fatalf("build markdown export: %v", err)
	}
	for _, want := range []string{
		"# 导出/测试包裹选项",
		"接下来您想怎么走？",
		"- 继续探讨 (`continue`)",
		"- 切换讨论模式 (`mode`)",
	} {
		if !strings.Contains(export.Markdown, want) {
			t.Fatalf("expected export markdown to contain %q, got:\n%s", want, export.Markdown)
		}
	}
	if strings.Contains(export.Markdown, "\"questiontype\":\"singleselect\"") {
		t.Fatalf("expected wrapped option payload to be rendered, got:\n%s", export.Markdown)
	}
}

func TestBuildCurrentChatMarkdownExportFormatsAskUserInputPayload(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store := knowledge.NewStore(filepath.Join(root, "knowledge.json"))
	projectStore := projectstate.NewStore(filepath.Join(root, "project.json"))
	promptStore := promptlib.NewStore(filepath.Join(root, "prompts.json"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(root, "reminders.json")))
	sessionStore := sessionstate.NewStore(filepath.Join(root, "sessions.json"))
	service := appsvc.NewServiceWithRuntime(store, nil, reminders, nil, sessionStore, promptStore)
	app := NewDesktopApp(root, store, promptStore, projectStore, nil, nil, service, sessionStore, reminders, nil)

	state, err := app.GetChatState()
	if err != nil {
		t.Fatalf("get chat state: %v", err)
	}
	if _, err := sessionStore.Save(context.Background(), sessionstate.Snapshot{
		Key: desktopConversationSnapshotKey("default", state.SessionID),
		History: []sessionstate.Message{
			{Role: "user", Content: "导出/测试 ask_user_input 选项"},
			{Role: "assistant", Content: `<details>
<summary>📚 讨论控制面板</summary>

您可以随时输入自己的观点加入讨论，也可以选择以下操作：

</details>

{ask_user_input: single_select, question: "接下来您想怎么走？", options: ["继续探讨（进入下一轮：HVG数量对哪个下游环节影响最大）", "深挖这个问题（继续讨论HVG的定义与目的）", "切换讨论模式", "结束讨论"]}`},
		},
	}); err != nil {
		t.Fatalf("save chat snapshot: %v", err)
	}

	export, err := app.buildCurrentChatMarkdownExport(context.Background())
	if err != nil {
		t.Fatalf("build markdown export: %v", err)
	}
	for _, want := range []string{
		"# 导出/测试 ask_user_input 选项",
		"**📚 讨论控制面板**",
		"您可以随时输入自己的观点加入讨论，也可以选择以下操作：",
		"接下来您想怎么走？",
		"- 继续探讨（进入下一轮：HVG数量对哪个下游环节影响最大）",
		"- 深挖这个问题（继续讨论HVG的定义与目的）",
		"- 切换讨论模式",
		"- 结束讨论",
	} {
		if !strings.Contains(export.Markdown, want) {
			t.Fatalf("expected export markdown to contain %q, got:\n%s", want, export.Markdown)
		}
	}
	if strings.Contains(export.Markdown, "{ask_user_input:") {
		t.Fatalf("expected ask_user_input payload to be rendered, got:\n%s", export.Markdown)
	}
}

func TestBuildCurrentChatMarkdownExportRejectsEmptyConversation(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store := knowledge.NewStore(filepath.Join(root, "knowledge.json"))
	projectStore := projectstate.NewStore(filepath.Join(root, "project.json"))
	promptStore := promptlib.NewStore(filepath.Join(root, "prompts.json"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(root, "reminders.json")))
	sessionStore := sessionstate.NewStore(filepath.Join(root, "sessions.json"))
	service := appsvc.NewServiceWithRuntime(store, nil, reminders, nil, sessionStore, promptStore)
	app := NewDesktopApp(root, store, promptStore, projectStore, nil, nil, service, sessionStore, reminders, nil)

	if _, err := app.buildCurrentChatMarkdownExport(context.Background()); err == nil || !strings.Contains(err.Error(), "没有可导出的消息") {
		t.Fatalf("expected empty conversation error, got %v", err)
	}
}

type desktopTestAI struct {
	route          ai.RouteDecision
	chatFunc       func(context.Context, string, []ai.ConversationMessage) string
	chatResultFunc func(context.Context, string, []ai.ConversationMessage) (string, error)
}

func (f desktopTestAI) IsConfigured(context.Context) (bool, error) {
	return true, nil
}

func (f desktopTestAI) RouteCommand(context.Context, string) (ai.RouteDecision, error) {
	return f.route, nil
}

func (f desktopTestAI) BuildSearchPlan(context.Context, string) (ai.SearchPlan, error) {
	return ai.SearchPlan{}, nil
}

func (f desktopTestAI) BuildFileSearchIntent(context.Context, string) (ai.FileSearchIntent, error) {
	return ai.FileSearchIntent{}, nil
}

func (f desktopTestAI) ReviewAnswerCandidates(context.Context, string, []knowledge.Entry) ([]string, error) {
	return nil, nil
}

func (f desktopTestAI) Answer(context.Context, string, []knowledge.Entry) (string, error) {
	return "", nil
}

func (f desktopTestAI) Chat(ctx context.Context, input string, history []ai.ConversationMessage) (string, error) {
	if f.chatResultFunc != nil {
		return f.chatResultFunc(ctx, input, history)
	}
	if f.chatFunc != nil {
		return f.chatFunc(ctx, input, history), nil
	}
	return "", nil
}

func (f desktopTestAI) DecideAgentStep(context.Context, string, []ai.ConversationMessage, []ai.AgentToolDefinition, []ai.AgentToolResult) (ai.AgentStepDecision, error) {
	return ai.AgentStepDecision{}, nil
}

func (f desktopTestAI) TranslateToChinese(context.Context, string) (string, error) {
	return "", nil
}

func (f desktopTestAI) SummarizePDFText(context.Context, string, string) (string, error) {
	return "", nil
}

func (f desktopTestAI) SummarizeImageFile(context.Context, string, string) (string, error) {
	return "", nil
}
