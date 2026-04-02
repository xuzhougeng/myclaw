package main

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"baize/internal/ai"
	appsvc "baize/internal/app"
	"baize/internal/filesearch"
	"baize/internal/knowledge"
	"baize/internal/projectstate"
	"baize/internal/promptlib"
	"baize/internal/reminder"
	"baize/internal/sessionstate"
	"baize/internal/weixin"
)

func TestDesktopChatSessionsCanBeCreatedAndSwitched(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store := knowledge.NewStore(filepath.Join(root, "app.db"))
	projectStore := projectstate.NewStore(filepath.Join(root, "app.db"))
	promptStore := promptlib.NewStore(filepath.Join(root, "app.db"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(root, "app.db")))
	sessionStore := sessionstate.NewStore(filepath.Join(root, "app.db"))
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

	second, err := app.NewChatSession("")
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

func TestDesktopChatStateIncludesConversationMode(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store := knowledge.NewStore(filepath.Join(root, "app.db"))
	projectStore := projectstate.NewStore(filepath.Join(root, "app.db"))
	promptStore := promptlib.NewStore(filepath.Join(root, "app.db"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(root, "app.db")))
	sessionStore := sessionstate.NewStore(filepath.Join(root, "app.db"))
	service := appsvc.NewServiceWithRuntime(store, nil, reminders, nil, sessionStore, promptStore)
	app := NewDesktopApp(root, store, promptStore, projectStore, nil, nil, service, sessionStore, reminders, nil)

	initial, err := app.GetChatState()
	if err != nil {
		t.Fatalf("get initial chat state: %v", err)
	}
	if len(initial.Conversations) == 0 {
		t.Fatalf("expected initial conversation, got %#v", initial)
	}
	if initial.Conversations[0].Mode != "agent" {
		t.Fatalf("expected default desktop mode=agent, got %#v", initial.Conversations[0])
	}

	askState, err := app.NewChatSession("ask")
	if err != nil {
		t.Fatalf("new ask session: %v", err)
	}
	active := askState.Conversations[0]
	for _, conversation := range askState.Conversations {
		if conversation.Active {
			active = conversation
			break
		}
	}
	if active.Mode != "ask" {
		t.Fatalf("expected active ask conversation mode, got %#v", active)
	}
}

func TestDesktopSendMessageNewConversationReturnsSessionChanged(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store := knowledge.NewStore(filepath.Join(root, "app.db"))
	projectStore := projectstate.NewStore(filepath.Join(root, "app.db"))
	promptStore := promptlib.NewStore(filepath.Join(root, "app.db"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(root, "app.db")))
	sessionStore := sessionstate.NewStore(filepath.Join(root, "app.db"))
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
	if result.HistoryPersisted {
		t.Fatalf("expected /new to avoid chat history persistence, got %#v", result)
	}
}

func TestDesktopSendMessageHelpDoesNotPersistHistory(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store := knowledge.NewStore(filepath.Join(root, "app.db"))
	projectStore := projectstate.NewStore(filepath.Join(root, "app.db"))
	promptStore := promptlib.NewStore(filepath.Join(root, "app.db"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(root, "app.db")))
	sessionStore := sessionstate.NewStore(filepath.Join(root, "app.db"))
	service := appsvc.NewServiceWithRuntime(store, nil, reminders, nil, sessionStore, promptStore)
	app := NewDesktopApp(root, store, promptStore, projectStore, nil, nil, service, sessionStore, reminders, nil)

	result, err := app.SendMessage("/help")
	if err != nil {
		t.Fatalf("send /help: %v", err)
	}
	if result.HistoryPersisted {
		t.Fatalf("expected /help to avoid chat history persistence, got %#v", result)
	}
	if !strings.Contains(result.Reply, "/help") {
		t.Fatalf("expected help reply, got %#v", result)
	}

	state, err := app.GetChatState()
	if err != nil {
		t.Fatalf("get chat state: %v", err)
	}
	if len(state.Messages) != 0 {
		t.Fatalf("expected /help to avoid persisted chat messages, got %#v", state.Messages)
	}
}

func TestDesktopSendMessageKBListPersistsHistory(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store := knowledge.NewStore(filepath.Join(root, "app.db"))
	projectStore := projectstate.NewStore(filepath.Join(root, "app.db"))
	promptStore := promptlib.NewStore(filepath.Join(root, "app.db"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(root, "app.db")))
	sessionStore := sessionstate.NewStore(filepath.Join(root, "app.db"))
	service := appsvc.NewServiceWithRuntime(store, nil, reminders, nil, sessionStore, promptStore)
	app := NewDesktopApp(root, store, promptStore, projectStore, nil, nil, service, sessionStore, reminders, nil)

	result, err := app.SendMessage("/kb list")
	if err != nil {
		t.Fatalf("send /kb list: %v", err)
	}
	if !result.HistoryPersisted {
		t.Fatalf("expected /kb list to persist history, got %#v", result)
	}
	if !strings.Contains(result.Reply, "知识库为空") {
		t.Fatalf("expected list reply, got %#v", result)
	}

	state, err := app.GetChatState()
	if err != nil {
		t.Fatalf("get chat state: %v", err)
	}
	if len(state.Messages) != 2 {
		t.Fatalf("expected /kb list exchange in chat history, got %#v", state.Messages)
	}
	if state.Messages[0].Role != "user" || state.Messages[0].Text != "/kb list" {
		t.Fatalf("unexpected persisted user message: %#v", state.Messages[0])
	}
	if state.Messages[1].Role != "assistant" || !strings.Contains(state.Messages[1].Text, "知识库为空") {
		t.Fatalf("unexpected persisted assistant message: %#v", state.Messages[1])
	}
}

func TestDesktopSendMessageUnavailableAIPersistsHistory(t *testing.T) {
	t.Parallel()

	for _, mode := range []string{"agent", "ask"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			store := knowledge.NewStore(filepath.Join(root, "app.db"))
			projectStore := projectstate.NewStore(filepath.Join(root, "app.db"))
			promptStore := promptlib.NewStore(filepath.Join(root, "app.db"))
			reminders := reminder.NewManager(reminder.NewStore(filepath.Join(root, "app.db")))
			sessionStore := sessionstate.NewStore(filepath.Join(root, "app.db"))
			service := appsvc.NewServiceWithRuntime(store, nil, reminders, nil, sessionStore, promptStore)
			app := NewDesktopApp(root, store, promptStore, projectStore, nil, nil, service, sessionStore, reminders, nil)

			if mode == "ask" {
				state, err := app.NewChatSession("ask")
				if err != nil {
					t.Fatalf("new ask session: %v", err)
				}
				if state.SessionID == "" {
					t.Fatalf("expected ask session id, got %#v", state)
				}
			}

			result, err := app.SendMessage("hello")
			if err != nil {
				t.Fatalf("send message: %v", err)
			}
			if !result.HistoryPersisted {
				t.Fatalf("expected unavailable-ai reply to persist history, got %#v", result)
			}
			if !strings.Contains(result.Reply, "模型尚未启用") {
				t.Fatalf("expected unavailable-ai reply, got %#v", result)
			}

			state, err := app.GetChatState()
			if err != nil {
				t.Fatalf("get chat state: %v", err)
			}
			if len(state.Messages) != 2 {
				t.Fatalf("expected persisted unavailable-ai exchange, got %#v", state.Messages)
			}
			if state.Messages[0].Role != "user" || state.Messages[0].Text != "hello" {
				t.Fatalf("unexpected persisted user message: %#v", state.Messages[0])
			}
			if state.Messages[1].Role != "assistant" || !strings.Contains(state.Messages[1].Text, "模型尚未启用") {
				t.Fatalf("unexpected persisted assistant message: %#v", state.Messages[1])
			}
		})
	}
}

func TestDesktopSendMessageReturnsAndPersistsUsage(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store := knowledge.NewStore(filepath.Join(root, "app.db"))
	projectStore := projectstate.NewStore(filepath.Join(root, "app.db"))
	promptStore := promptlib.NewStore(filepath.Join(root, "app.db"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(root, "app.db")))
	sessionStore := sessionstate.NewStore(filepath.Join(root, "app.db"))
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
	if len(result.Process) == 0 {
		t.Fatalf("expected debug process payload, got %#v", result)
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
	if len(last.Process) == 0 {
		t.Fatalf("expected persisted debug process on assistant message, got %#v", last)
	}
}

func TestDesktopSendMessageUsesFileSearchTool(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store := knowledge.NewStore(filepath.Join(root, "app.db"))
	projectStore := projectstate.NewStore(filepath.Join(root, "app.db"))
	promptStore := promptlib.NewStore(filepath.Join(root, "app.db"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(root, "app.db")))
	sessionStore := sessionstate.NewStore(filepath.Join(root, "app.db"))
	service := appsvc.NewServiceWithRuntime(store, desktopTestAI{
		route:             ai.RouteDecision{Command: "answer"},
		toolOpportunities: []ai.ToolOpportunity{{ToolName: filesearch.ToolName, Goal: "查找 D 盘 csv 文件"}},
		toolPlanDecision: ai.ToolPlanDecision{
			Action:    "tool",
			ToolName:  filesearch.ToolName,
			ToolInput: `{"query":"d: *.csv"}`,
		},
	}, reminders, nil, sessionStore, promptStore)
	service.SetFileSearchEverythingPath("es.exe")
	service.SetFileSearchExecutor(func(_ context.Context, everythingPath string, input filesearch.ToolInput) (filesearch.ToolResult, error) {
		if everythingPath != "es.exe" {
			t.Fatalf("unexpected everything path: %q", everythingPath)
		}
		if input.Query != "d: *.csv" {
			t.Fatalf("unexpected query: %#v", input)
		}
		return filesearch.ToolResult{
			Tool:  filesearch.ToolName,
			Query: input.Query,
			Items: []filesearch.ResultItem{
				{Index: 1, Name: "report.csv", Path: `D:\exports\report.csv`},
			},
		}, nil
	})
	app := NewDesktopApp(root, store, promptStore, projectStore, nil, nil, service, sessionStore, reminders, nil)

	result, err := app.SendMessage("查找D盘的csv文件")
	if err != nil {
		t.Fatalf("send file search message: %v", err)
	}
	if !strings.Contains(result.Reply, "找到 1 个文件") || !strings.Contains(result.Reply, `D:\exports\report.csv`) {
		t.Fatalf("unexpected file search reply: %#v", result)
	}
	if len(result.Process) < 3 {
		t.Fatalf("expected debug process on file search reply, got %#v", result.Process)
	}
	if result.Process[0].Title == "" || result.Process[0].Detail == "" {
		t.Fatalf("expected structured debug steps, got %#v", result.Process)
	}

	state, err := app.GetChatState()
	if err != nil {
		t.Fatalf("get chat state: %v", err)
	}
	if len(state.Messages) == 0 || len(state.Messages[len(state.Messages)-1].Process) < 3 {
		t.Fatalf("expected persisted debug process in chat state, got %#v", state.Messages)
	}
}

func TestDesktopSendMessageStreamsProcessStepsBeforeError(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store := knowledge.NewStore(filepath.Join(root, "app.db"))
	projectStore := projectstate.NewStore(filepath.Join(root, "app.db"))
	promptStore := promptlib.NewStore(filepath.Join(root, "app.db"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(root, "app.db")))
	sessionStore := sessionstate.NewStore(filepath.Join(root, "app.db"))
	service := appsvc.NewServiceWithRuntime(store, desktopTestAI{
		route:       ai.RouteDecision{Command: "answer"},
		planNextErr: errors.New("planning step 0: decode structured response: invalid character '{' after top-level value"),
	}, reminders, nil, sessionStore, promptStore)
	app := NewDesktopApp(root, store, promptStore, projectStore, nil, nil, service, sessionStore, reminders, nil)

	var streamed []ai.CallTraceStep
	result, err := app.sendMessage(context.Background(), "你好", nil, func(step ai.CallTraceStep) {
		streamed = append(streamed, step)
	})
	if err == nil {
		t.Fatalf("expected planning error, got result=%#v", result)
	}
	if len(streamed) < 3 {
		t.Fatalf("expected streamed process steps before error, got %#v", streamed)
	}
	if streamed[0].Title != "AI 路由" || streamed[1].Title != "执行模式" {
		t.Fatalf("expected route+mode steps first, got %#v", streamed)
	}
	last := streamed[len(streamed)-1]
	if last.Title != "Agent 执行失败" || !strings.Contains(last.Detail, "planning step 0") {
		t.Fatalf("expected final streamed error step, got %#v", streamed)
	}
}

func TestDesktopSendMessageStreamsAgentReply(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store := knowledge.NewStore(filepath.Join(root, "app.db"))
	projectStore := projectstate.NewStore(filepath.Join(root, "app.db"))
	promptStore := promptlib.NewStore(filepath.Join(root, "app.db"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(root, "app.db")))
	sessionStore := sessionstate.NewStore(filepath.Join(root, "app.db"))
	wantReply := "这是一个用于验证 desktop agent 最终回答会分段流到前端占位消息中的长回复。"
	service := appsvc.NewServiceWithRuntime(store, desktopTestAI{
		route: ai.RouteDecision{Command: "answer"},
		chatFunc: func(_ context.Context, input string, history []ai.ConversationMessage) string {
			if input != "请给我一个长回答" {
				t.Fatalf("unexpected input: %q", input)
			}
			if len(history) != 0 {
				t.Fatalf("expected empty history, got %#v", history)
			}
			return wantReply
		},
	}, reminders, nil, sessionStore, promptStore)
	app := NewDesktopApp(root, store, promptStore, projectStore, nil, nil, service, sessionStore, reminders, nil)

	var deltas []string
	var process []ai.CallTraceStep
	result, err := app.sendMessage(context.Background(), "请给我一个长回答", func(delta string) {
		deltas = append(deltas, delta)
	}, func(step ai.CallTraceStep) {
		process = append(process, step)
	})
	if err != nil {
		t.Fatalf("send streaming agent message: %v", err)
	}
	if result.Reply != wantReply {
		t.Fatalf("unexpected reply: %#v", result)
	}
	if strings.Join(deltas, "") != wantReply {
		t.Fatalf("unexpected streamed deltas: %#v", deltas)
	}
	if len(deltas) < 2 {
		t.Fatalf("expected multiple streamed deltas, got %#v", deltas)
	}
	if len(process) < 2 {
		t.Fatalf("expected streamed process steps, got %#v", process)
	}
	if process[0].Title != "AI 路由" || process[1].Title != "执行模式" {
		t.Fatalf("unexpected process steps: %#v", process)
	}
}

func TestDesktopSendMessageReusesCurrentSession(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store := knowledge.NewStore(filepath.Join(root, "app.db"))
	projectStore := projectstate.NewStore(filepath.Join(root, "app.db"))
	promptStore := promptlib.NewStore(filepath.Join(root, "app.db"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(root, "app.db")))
	sessionStore := sessionstate.NewStore(filepath.Join(root, "app.db"))
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
	store := knowledge.NewStore(filepath.Join(root, "app.db"))
	projectStore := projectstate.NewStore(filepath.Join(root, "app.db"))
	promptStore := promptlib.NewStore(filepath.Join(root, "app.db"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(root, "app.db")))
	sessionStore := sessionstate.NewStore(filepath.Join(root, "app.db"))
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
	store := knowledge.NewStore(filepath.Join(root, "app.db"))
	projectStore := projectstate.NewStore(filepath.Join(root, "app.db"))
	promptStore := promptlib.NewStore(filepath.Join(root, "app.db"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(root, "app.db")))
	sessionStore := sessionstate.NewStore(filepath.Join(root, "app.db"))
	service := appsvc.NewServiceWithRuntime(store, nil, reminders, nil, sessionStore, promptStore)
	app := NewDesktopApp(root, store, promptStore, projectStore, nil, nil, service, sessionStore, reminders, nil)

	first, err := app.GetChatState()
	if err != nil {
		t.Fatalf("get first state: %v", err)
	}
	second, err := app.NewChatSession("")
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
	store := knowledge.NewStore(filepath.Join(root, "app.db"))
	projectStore := projectstate.NewStore(filepath.Join(root, "app.db"))
	promptStore := promptlib.NewStore(filepath.Join(root, "app.db"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(root, "app.db")))
	sessionStore := sessionstate.NewStore(filepath.Join(root, "app.db"))
	service := appsvc.NewServiceWithRuntime(store, nil, reminders, nil, sessionStore, promptStore)
	app := NewDesktopApp(root, store, promptStore, projectStore, nil, nil, service, sessionStore, reminders, nil)

	if _, err := app.GetChatState(); err != nil {
		t.Fatalf("prime desktop chat state: %v", err)
	}
	if _, err := sessionStore.Save(context.Background(), sessionstate.Snapshot{
		Key: "source:weixin:user-1|session:weixin-user:user-1",
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
		if item.SessionID != "weixin-user:user-1" {
			continue
		}
		found = true
		if item.ReadOnly {
			t.Fatalf("expected weixin conversation to stay writable in desktop chat: %#v", item)
		}
		if item.Source != "weixin:user-1" || item.SourceLabel != "微信" {
			t.Fatalf("unexpected weixin source metadata: %#v", item)
		}
		if item.Mode != "agent" {
			t.Fatalf("expected weixin conversation default mode=agent, got %#v", item)
		}
		if item.Active {
			t.Fatalf("expected desktop conversation to remain active by default: %#v", item)
		}
	}
	if !found {
		t.Fatalf("expected weixin conversation in chat state, got %#v", state.Conversations)
	}

	switched, err := app.SwitchChatSession("weixin-user:user-1")
	if err != nil {
		t.Fatalf("switch to weixin conversation: %v", err)
	}
	if switched.SessionID != "weixin-user:user-1" {
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
	store := knowledge.NewStore(filepath.Join(root, "app.db"))
	projectStore := projectstate.NewStore(filepath.Join(root, "app.db"))
	promptStore := promptlib.NewStore(filepath.Join(root, "app.db"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(root, "app.db")))
	sessionStore := sessionstate.NewStore(filepath.Join(root, "app.db"))
	service := appsvc.NewServiceWithRuntime(store, nil, reminders, nil, sessionStore, promptStore)
	app := NewDesktopApp(root, store, promptStore, projectStore, nil, nil, service, sessionStore, reminders, nil)

	if _, err := app.GetChatState(); err != nil {
		t.Fatalf("prime desktop chat state: %v", err)
	}
	if _, err := sessionStore.Save(context.Background(), sessionstate.Snapshot{
		Key: "source:weixin:user-1|session:weixin-user:user-1",
		History: []sessionstate.Message{
			{Role: "user", Content: "你好"},
			{Role: "assistant", Content: "你好，我在。"},
		},
	}); err != nil {
		t.Fatalf("save weixin snapshot: %v", err)
	}

	app.emitChatChanged(weixin.ConversationUpdate{SessionID: "weixin-user:user-1", Activate: true})

	state, err := app.GetChatState()
	if err != nil {
		t.Fatalf("get chat state: %v", err)
	}
	if state.SessionID != "weixin-user:user-1" {
		t.Fatalf("expected active weixin session, got %#v", state)
	}
	if len(state.Messages) != 2 || state.Messages[0].Text != "你好" || state.Messages[1].Text != "你好，我在。" {
		t.Fatalf("expected activated weixin history, got %#v", state.Messages)
	}
}

func TestDesktopSendMessageContinuesWeixinConversation(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store := knowledge.NewStore(filepath.Join(root, "app.db"))
	projectStore := projectstate.NewStore(filepath.Join(root, "app.db"))
	promptStore := promptlib.NewStore(filepath.Join(root, "app.db"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(root, "app.db")))
	sessionStore := sessionstate.NewStore(filepath.Join(root, "app.db"))
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
		Key: "source:weixin:user-1|session:weixin-user:user-1",
		History: []sessionstate.Message{
			{Role: "user", Content: "你好"},
			{Role: "assistant", Content: "你好，我在。"},
		},
	}); err != nil {
		t.Fatalf("save weixin snapshot: %v", err)
	}
	if _, err := app.SwitchChatSession("weixin-user:user-1"); err != nil {
		t.Fatalf("switch to weixin conversation: %v", err)
	}

	result, err := app.SendMessage("继续说")
	if err != nil {
		t.Fatalf("continue weixin conversation: %v", err)
	}
	if result.SessionID != "weixin-user:user-1" || result.Reply != "继续聊" {
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
	store := knowledge.NewStore(filepath.Join(root, "app.db"))
	projectStore := projectstate.NewStore(filepath.Join(root, "app.db"))
	promptStore := promptlib.NewStore(filepath.Join(root, "app.db"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(root, "app.db")))
	sessionStore := sessionstate.NewStore(filepath.Join(root, "app.db"))
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
	store := knowledge.NewStore(filepath.Join(root, "app.db"))
	projectStore := projectstate.NewStore(filepath.Join(root, "app.db"))
	promptStore := promptlib.NewStore(filepath.Join(root, "app.db"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(root, "app.db")))
	sessionStore := sessionstate.NewStore(filepath.Join(root, "app.db"))
	service := appsvc.NewServiceWithRuntime(store, nil, reminders, nil, sessionStore, promptStore)
	app := NewDesktopApp(root, store, promptStore, projectStore, nil, nil, service, sessionStore, reminders, nil)

	first, err := app.GetChatState()
	if err != nil {
		t.Fatalf("get first chat state: %v", err)
	}
	second, err := app.NewChatSession("")
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
	store := knowledge.NewStore(filepath.Join(root, "app.db"))
	projectStore := projectstate.NewStore(filepath.Join(root, "app.db"))
	promptStore := promptlib.NewStore(filepath.Join(root, "app.db"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(root, "app.db")))
	sessionStore := sessionstate.NewStore(filepath.Join(root, "app.db"))
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
	store := knowledge.NewStore(filepath.Join(root, "app.db"))
	projectStore := projectstate.NewStore(filepath.Join(root, "app.db"))
	promptStore := promptlib.NewStore(filepath.Join(root, "app.db"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(root, "app.db")))
	sessionStore := sessionstate.NewStore(filepath.Join(root, "app.db"))
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

func TestDesktopChatStateSurvivesNewSessionSwitchAndRestart(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store := knowledge.NewStore(filepath.Join(root, "app.db"))
	projectStore := projectstate.NewStore(filepath.Join(root, "app.db"))
	promptStore := promptlib.NewStore(filepath.Join(root, "app.db"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(root, "app.db")))
	sessionStore := sessionstate.NewStore(filepath.Join(root, "app.db"))
	service := appsvc.NewServiceWithRuntime(store, desktopTestAI{
		route: ai.RouteDecision{Command: "answer"},
		chatFunc: func(_ context.Context, input string, history []ai.ConversationMessage) string {
			return "reply:" + input
		},
	}, reminders, nil, sessionStore, promptStore)
	app := NewDesktopApp(root, store, promptStore, projectStore, nil, nil, service, sessionStore, reminders, nil)

	firstReply, err := app.SendMessage("first")
	if err != nil {
		t.Fatalf("send first message: %v", err)
	}
	firstSessionID := firstReply.SessionID
	if firstSessionID == "" {
		t.Fatalf("expected first session id, got %#v", firstReply)
	}

	secondState, err := app.NewChatSession("ask")
	if err != nil {
		t.Fatalf("new second session: %v", err)
	}
	secondSessionID := secondState.SessionID
	if secondSessionID == "" || secondSessionID == firstSessionID {
		t.Fatalf("expected distinct second session id, got first=%q second=%#v", firstSessionID, secondState)
	}

	secondReply, err := app.SendMessage("second")
	if err != nil {
		t.Fatalf("send second message: %v", err)
	}
	if secondReply.SessionID != secondSessionID {
		t.Fatalf("expected second reply to stay in session %q, got %#v", secondSessionID, secondReply)
	}

	firstState, err := app.SwitchChatSession(firstSessionID)
	if err != nil {
		t.Fatalf("switch back to first session: %v", err)
	}
	if firstState.SessionID != firstSessionID {
		t.Fatalf("expected first session to be active, got %#v", firstState)
	}
	if len(firstState.Messages) != 2 || firstState.Messages[0].Text != "first" || firstState.Messages[1].Text != "reply:first" {
		t.Fatalf("expected first session history before restart, got %#v", firstState.Messages)
	}

	reloadedService := appsvc.NewServiceWithRuntime(store, nil, reminders, nil, sessionStore, promptStore)
	reloadedApp := NewDesktopApp(root, store, promptStore, projectStore, nil, nil, reloadedService, sessionStore, reminders, nil)

	reloadedState, err := reloadedApp.GetChatState()
	if err != nil {
		t.Fatalf("reload chat state: %v", err)
	}
	if reloadedState.SessionID != firstSessionID {
		t.Fatalf("expected restart to restore first session %q, got %#v", firstSessionID, reloadedState)
	}
	if len(reloadedState.Messages) != 2 || reloadedState.Messages[0].Text != "first" || reloadedState.Messages[1].Text != "reply:first" {
		t.Fatalf("expected first session history after restart, got %#v", reloadedState.Messages)
	}

	secondReloadedState, err := reloadedApp.SwitchChatSession(secondSessionID)
	if err != nil {
		t.Fatalf("switch to second session after restart: %v", err)
	}
	if secondReloadedState.SessionID != secondSessionID {
		t.Fatalf("expected second session after restart, got %#v", secondReloadedState)
	}
	if len(secondReloadedState.Messages) != 2 || secondReloadedState.Messages[0].Text != "second" || secondReloadedState.Messages[1].Text != "reply:second" {
		t.Fatalf("expected second session history after restart, got %#v", secondReloadedState.Messages)
	}

	secondSnapshot, ok, err := sessionStore.Load(context.Background(), desktopConversationSnapshotKey(knowledge.DefaultProjectName, secondSessionID))
	if err != nil {
		t.Fatalf("load second session snapshot: %v", err)
	}
	if !ok {
		t.Fatal("expected second session snapshot to persist")
	}
	if secondSnapshot.Mode != "ask" {
		t.Fatalf("expected ask mode on second session snapshot, got %#v", secondSnapshot)
	}
}

func TestBuildCurrentChatMarkdownExportFormatsConversation(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store := knowledge.NewStore(filepath.Join(root, "app.db"))
	projectStore := projectstate.NewStore(filepath.Join(root, "app.db"))
	promptStore := promptlib.NewStore(filepath.Join(root, "app.db"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(root, "app.db")))
	sessionStore := sessionstate.NewStore(filepath.Join(root, "app.db"))
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
	store := knowledge.NewStore(filepath.Join(root, "app.db"))
	projectStore := projectstate.NewStore(filepath.Join(root, "app.db"))
	promptStore := promptlib.NewStore(filepath.Join(root, "app.db"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(root, "app.db")))
	sessionStore := sessionstate.NewStore(filepath.Join(root, "app.db"))
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
	store := knowledge.NewStore(filepath.Join(root, "app.db"))
	projectStore := projectstate.NewStore(filepath.Join(root, "app.db"))
	promptStore := promptlib.NewStore(filepath.Join(root, "app.db"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(root, "app.db")))
	sessionStore := sessionstate.NewStore(filepath.Join(root, "app.db"))
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
	store := knowledge.NewStore(filepath.Join(root, "app.db"))
	projectStore := projectstate.NewStore(filepath.Join(root, "app.db"))
	promptStore := promptlib.NewStore(filepath.Join(root, "app.db"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(root, "app.db")))
	sessionStore := sessionstate.NewStore(filepath.Join(root, "app.db"))
	service := appsvc.NewServiceWithRuntime(store, nil, reminders, nil, sessionStore, promptStore)
	app := NewDesktopApp(root, store, promptStore, projectStore, nil, nil, service, sessionStore, reminders, nil)

	if _, err := app.buildCurrentChatMarkdownExport(context.Background()); err == nil || !strings.Contains(err.Error(), "没有可导出的消息") {
		t.Fatalf("expected empty conversation error, got %v", err)
	}
}

type desktopTestAI struct {
	route             ai.RouteDecision
	toolOpportunities []ai.ToolOpportunity
	toolPlanDecision  ai.ToolPlanDecision
	planNextErr       error
	chatFunc          func(context.Context, string, []ai.ConversationMessage) string
	chatResultFunc    func(context.Context, string, []ai.ConversationMessage) (string, error)
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

func (f desktopTestAI) DetectToolOpportunities(context.Context, string, []ai.ToolCapability) ([]ai.ToolOpportunity, error) {
	return f.toolOpportunities, nil
}

func (f desktopTestAI) PlanToolUse(context.Context, string, ai.ToolCapability, []ai.ToolExecution) (ai.ToolPlanDecision, error) {
	return f.toolPlanDecision, nil
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

func (f desktopTestAI) PlanNext(ctx context.Context, task string, history []ai.ConversationMessage, _ []ai.AgentToolDefinition, state ai.AgentTaskState) (ai.LoopDecision, error) {
	if f.planNextErr != nil {
		return ai.LoopDecision{}, f.planNextErr
	}
	if len(state.ToolAttempts) == 0 && strings.EqualFold(strings.TrimSpace(f.toolPlanDecision.Action), "tool") {
		toolName := strings.TrimSpace(f.toolPlanDecision.ToolName)
		if toolName != "" && !strings.Contains(toolName, "::") {
			toolName = "local::" + toolName
		}
		return ai.LoopDecision{Action: ai.LoopContinue, ToolName: toolName, ToolInput: f.toolPlanDecision.ToolInput}, nil
	}
	if len(state.ToolAttempts) > 0 {
		return ai.LoopDecision{Action: ai.LoopAnswer, Answer: strings.TrimSpace(state.ToolAttempts[len(state.ToolAttempts)-1].RawOutput)}, nil
	}
	reply, err := f.Chat(ctx, task, history)
	if err != nil {
		return ai.LoopDecision{}, err
	}
	return ai.LoopDecision{Action: ai.LoopAnswer, Answer: reply}, nil
}

func (f desktopTestAI) SummarizeWorkingState(_ context.Context, _ ai.AgentTaskState) (string, error) {
	return "", nil
}

func (f desktopTestAI) SummarizeFinal(_ context.Context, _ ai.AgentTaskState, finalAnswer string) (string, error) {
	return finalAnswer, nil
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
