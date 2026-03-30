package main

import (
	"context"
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
			{Role: "user", Content: "导出/测试 askuserinput 选项"},
			{Role: "assistant", Content: `<details>
<summary>📚 讨论控制面板</summary>

您可以随时输入自己的观点加入讨论，也可以选择以下操作：

</details>

{askuserinput: single_select, question: "接下来您想怎么走？", options: ["继续探讨（进入下一轮：HVG数量对哪个下游环节影响最大）", "深挖这个问题（继续讨论HVG的定义与目的）", "切换讨论模式", "结束讨论"]}`},
		},
	}); err != nil {
		t.Fatalf("save chat snapshot: %v", err)
	}

	export, err := app.buildCurrentChatMarkdownExport(context.Background())
	if err != nil {
		t.Fatalf("build markdown export: %v", err)
	}
	for _, want := range []string{
		"# 导出/测试 askuserinput 选项",
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
	if strings.Contains(export.Markdown, "{askuserinput:") {
		t.Fatalf("expected askuserinput payload to be rendered, got:\n%s", export.Markdown)
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
	route    ai.RouteDecision
	chatFunc func(context.Context, string, []ai.ConversationMessage) string
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

func (f desktopTestAI) ReviewAnswerCandidates(context.Context, string, []knowledge.Entry) ([]string, error) {
	return nil, nil
}

func (f desktopTestAI) Answer(context.Context, string, []knowledge.Entry) (string, error) {
	return "", nil
}

func (f desktopTestAI) Chat(ctx context.Context, input string, history []ai.ConversationMessage) (string, error) {
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
