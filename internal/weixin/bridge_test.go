package weixin

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	aicore "myclaw/internal/ai"
	appsvc "myclaw/internal/app"
	"myclaw/internal/filesearch"
	"myclaw/internal/knowledge"
	"myclaw/internal/reminder"
	"myclaw/internal/sessionstate"
)

func TestExtractTextSupportsVoiceFallback(t *testing.T) {
	t.Parallel()

	text := extractText(WeixinMessage{
		ItemList: []MessageItem{
			{Type: ItemTypeVoice, VoiceItem: &VoiceItem{Text: "语音转写内容"}},
		},
	})
	if text != "语音转写内容" {
		t.Fatalf("unexpected text: %q", text)
	}
}

func TestSplitByRunes(t *testing.T) {
	t.Parallel()

	chunks := splitByRunes("123456789", 4)
	if len(chunks) != 3 {
		t.Fatalf("expected 3 chunks, got %d", len(chunks))
	}
	if chunks[0] != "1234" || chunks[1] != "5678" || chunks[2] != "9" {
		t.Fatalf("unexpected chunks: %#v", chunks)
	}
}

func TestSendTextMessageIncludesClientIDAndBaseInfo(t *testing.T) {
	t.Parallel()

	var got SendMessageRequest
	client := newTestClient(t, &got)

	err := client.SendTextMessage(context.Background(), "user-1", "hello", "ctx-1")
	if err != nil {
		t.Fatalf("send text: %v", err)
	}
	if got.Msg.ClientID == "" {
		t.Fatal("expected client id")
	}
	if got.BaseInfo.ChannelVersion != ChannelVersion {
		t.Fatalf("unexpected channel version: %q", got.BaseInfo.ChannelVersion)
	}
	if got.Msg.ContextToken != "ctx-1" {
		t.Fatalf("unexpected context token: %q", got.Msg.ContextToken)
	}
}

func TestFinalizeLoginPersistsAccount(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	bridge := NewBridge(NewClient("https://unit.test", ""), nil, nil, BridgeConfig{DataDir: dataDir})

	account, err := bridge.finalizeLogin(&QRCodeStatusResponse{
		Status:      "confirmed",
		BotToken:    "bot-token",
		BaseURL:     "https://weixin.example",
		ILinkBotID:  "bot-123",
		ILinkUserID: "user-456",
	})
	if err != nil {
		t.Fatalf("finalize login: %v", err)
	}
	if account.AccountID != "bot-123" {
		t.Fatalf("unexpected account id: %q", account.AccountID)
	}

	saved, ok := bridge.ReadSavedAccount()
	if !ok {
		t.Fatal("expected saved account")
	}
	if saved.Token != "bot-token" || saved.BaseURL != "https://weixin.example" {
		t.Fatalf("unexpected saved account: %#v", saved)
	}
}

func TestLoadAccountUsesSavedCredentials(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	accountPath := filepath.Join(dataDir, "weixin-bridge", "account.json")
	if err := os.MkdirAll(filepath.Dir(accountPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(accountPath, []byte(`{"token":"saved-token","base_url":"https://saved.example","account_id":"bot-1"}`), 0o644); err != nil {
		t.Fatalf("write account: %v", err)
	}

	bridge := NewBridge(NewClient("https://unit.test", ""), nil, nil, BridgeConfig{DataDir: dataDir})
	if !bridge.LoadAccount() {
		t.Fatal("expected saved account to load")
	}
	if bridge.client.token != "saved-token" {
		t.Fatalf("unexpected token: %q", bridge.client.token)
	}
	if bridge.client.BaseURL() != "https://saved.example" {
		t.Fatalf("unexpected base URL: %q", bridge.client.BaseURL())
	}
}

func TestHandleMessageFileFindDoesNotCreateConversation(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store := knowledge.NewStore(filepath.Join(root, "entries.json"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(root, "reminders.json")))
	sessionStore := sessionstate.NewStore(filepath.Join(root, "sessions.json"))
	service := appsvc.NewServiceWithRuntime(store, bridgeTestAI{
		toolOpportunities: []aicore.ToolOpportunity{{ToolName: filesearch.ToolName, Goal: "查找 D 盘 pdf 文件"}},
		toolPlanDecision: aicore.ToolPlanDecision{
			Action:    "tool",
			ToolName:  filesearch.ToolName,
			ToolInput: `{"query":"d: ext:pdf 单细胞"}`,
		},
	}, reminders, nil, sessionStore, nil)
	service.SetFileSearchEverythingPath("es.exe")
	service.SetFileSearchExecutor(func(_ context.Context, everythingPath string, input filesearch.ToolInput) (filesearch.ToolResult, error) {
		query := filesearch.CompileQuery(input)
		return filesearch.ToolResult{
			Tool:  filesearch.ToolName,
			Query: query,
			Limit: input.Limit,
			Count: 1,
			Items: []filesearch.ResultItem{
				{Index: 1, Name: "单细胞报告.pdf", Path: `D:\docs\单细胞报告.pdf`},
			},
		}, nil
	})

	var sent SendMessageRequest
	bridge := NewBridge(newTestClient(t, &sent), service, reminders, BridgeConfig{
		DataDir:        root,
		EverythingPath: "es.exe",
	})
	var sentFiles []string
	bridge.fileSender.SetSendFunc(func(_ context.Context, _ string, _ string, filePath string) error {
		sentFiles = append(sentFiles, filePath)
		return nil
	})
	var updates []ConversationUpdate
	bridge.SetConversationUpdatedHook(func(update ConversationUpdate) {
		updates = append(updates, update)
	})

	msg := WeixinMessage{
		FromUserID:   "user-1",
		ContextToken: "ctx-1",
		MessageType:  MessageTypeUser,
		MessageState: MessageStateFinish,
		ItemList:     []MessageItem{{Type: ItemTypeText, TextItem: &TextItem{Text: "帮我找单细胞 pdf"}}},
		ClientID:     "client-1",
		ToUserID:     "bot",
	}
	bridge.handleMessage(context.Background(), msg)

	items, err := sessionStore.List(context.Background())
	if err != nil {
		t.Fatalf("list recorded snapshots: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected file find search to avoid creating sessions, got %#v", items)
	}
	if len(bridge.conversationSessions) != 0 {
		t.Fatalf("expected file find search to avoid binding sessions, got %#v", bridge.conversationSessions)
	}
	if len(updates) != 0 {
		t.Fatalf("expected file find search to avoid chat activation, got %#v", updates)
	}
	if sent.Msg.ToUserID != "user-1" || sent.Msg.ContextToken != "ctx-1" {
		t.Fatalf("unexpected send target: %#v", sent.Msg)
	}
	replyText := extractText(sent.Msg)
	if !strings.Contains(replyText, "找到 1 个文件") || !strings.Contains(replyText, "单细胞报告.pdf") {
		t.Fatalf("unexpected search reply: %q", replyText)
	}

	msg.ItemList = []MessageItem{{Type: ItemTypeText, TextItem: &TextItem{Text: "/send 1"}}}
	bridge.handleMessage(context.Background(), msg)

	items, err = sessionStore.List(context.Background())
	if err != nil {
		t.Fatalf("list snapshots after selection: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected file selection to avoid creating sessions, got %#v", items)
	}
	if len(bridge.conversationSessions) != 0 {
		t.Fatalf("expected file selection to avoid binding sessions, got %#v", bridge.conversationSessions)
	}
	if len(updates) != 0 {
		t.Fatalf("expected file selection to avoid chat activation, got %#v", updates)
	}
	if len(sentFiles) != 1 || sentFiles[0] != `D:\docs\单细胞报告.pdf` {
		t.Fatalf("unexpected sent files: %#v", sentFiles)
	}
	if !strings.Contains(extractText(sent.Msg), "已通过 ClawBot 发送文件 1") {
		t.Fatalf("unexpected selection reply: %#v", sent.Msg)
	}
}

func TestHandleMessageRecordsSlashCommandConversation(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store := knowledge.NewStore(filepath.Join(root, "entries.json"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(root, "reminders.json")))
	sessionStore := sessionstate.NewStore(filepath.Join(root, "sessions.json"))
	service := appsvc.NewServiceWithRuntime(store, nil, reminders, nil, sessionStore, nil)
	if _, err := store.Add(context.Background(), knowledge.Entry{Text: "第一条知识"}); err != nil {
		t.Fatalf("add knowledge: %v", err)
	}

	var sent SendMessageRequest
	bridge := NewBridge(newTestClient(t, &sent), service, reminders, BridgeConfig{DataDir: root})
	msg := WeixinMessage{
		FromUserID:   "user-1",
		ContextToken: "ctx-1",
		MessageType:  MessageTypeUser,
		MessageState: MessageStateFinish,
		ItemList:     []MessageItem{{Type: ItemTypeText, TextItem: &TextItem{Text: "/list"}}},
	}

	bridge.handleMessage(context.Background(), msg)

	snapshot, ok, err := sessionStore.Load(context.Background(), "source:weixin:user-1|session:weixin-user:user-1")
	if err != nil {
		t.Fatalf("load slash snapshot: %v", err)
	}
	if !ok {
		t.Fatal("expected slash command conversation to be recorded")
	}
	if len(snapshot.History) != 2 {
		t.Fatalf("expected slash history, got %#v", snapshot.History)
	}
	if snapshot.History[0].Content != "/list" {
		t.Fatalf("unexpected recorded command: %#v", snapshot.History[0])
	}
	if !strings.Contains(snapshot.History[1].Content, "当前对话已开始。") {
		t.Fatalf("expected new conversation notice, got %#v", snapshot.History[1])
	}
	if !strings.Contains(snapshot.History[1].Content, "第一条知识") {
		t.Fatalf("expected slash reply to be recorded, got %#v", snapshot.History[1])
	}
	if !strings.Contains(extractText(sent.Msg), "当前对话已开始。") {
		t.Fatalf("expected outbound notice, got %#v", sent.Msg)
	}
}

func TestHandleMessageStatelessCommandsDoNotCreateConversation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		command         string
		seedKnowledge   bool
		wantReplySubstr string
	}{
		{
			name:            "help",
			command:         "/help",
			wantReplySubstr: "可用命令:",
		},
		{
			name:            "stats",
			command:         "/stats",
			seedKnowledge:   true,
			wantReplySubstr: "知识条数: 1",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			store := knowledge.NewStore(filepath.Join(root, "entries.json"))
			reminders := reminder.NewManager(reminder.NewStore(filepath.Join(root, "reminders.json")))
			sessionStore := sessionstate.NewStore(filepath.Join(root, "sessions.json"))
			service := appsvc.NewServiceWithRuntime(store, nil, reminders, nil, sessionStore, nil)
			if tc.seedKnowledge {
				if _, err := store.Add(context.Background(), knowledge.Entry{Text: "第一条知识"}); err != nil {
					t.Fatalf("add knowledge: %v", err)
				}
			}

			var sent SendMessageRequest
			bridge := NewBridge(newTestClient(t, &sent), service, reminders, BridgeConfig{DataDir: root})
			var updates []ConversationUpdate
			bridge.SetConversationUpdatedHook(func(update ConversationUpdate) {
				updates = append(updates, update)
			})

			msg := WeixinMessage{
				FromUserID:   "user-1",
				ContextToken: "ctx-1",
				MessageType:  MessageTypeUser,
				MessageState: MessageStateFinish,
				ItemList:     []MessageItem{{Type: ItemTypeText, TextItem: &TextItem{Text: tc.command}}},
			}

			bridge.handleMessage(context.Background(), msg)

			items, err := sessionStore.List(context.Background())
			if err != nil {
				t.Fatalf("list sessions: %v", err)
			}
			if len(items) != 0 {
				t.Fatalf("expected stateless command to avoid creating sessions, got %#v", items)
			}
			if len(bridge.conversationSessions) != 0 {
				t.Fatalf("expected stateless command to avoid binding weixin sessions, got %#v", bridge.conversationSessions)
			}
			if len(updates) != 0 {
				t.Fatalf("expected stateless command to avoid chat activation, got %#v", updates)
			}
			if !strings.Contains(extractText(sent.Msg), tc.wantReplySubstr) {
				t.Fatalf("unexpected outbound reply: %#v", sent.Msg)
			}
		})
	}
}

func TestHandleMessageReusesConversationAcrossContextTokens(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store := knowledge.NewStore(filepath.Join(root, "entries.json"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(root, "reminders.json")))
	sessionStore := sessionstate.NewStore(filepath.Join(root, "sessions.json"))
	service := appsvc.NewServiceWithRuntime(store, bridgeTestAI{
		route: aicore.RouteDecision{Command: "answer"},
		chatFunc: func(_ context.Context, input string, history []aicore.ConversationMessage) string {
			return "收到: " + input
		},
	}, reminders, nil, sessionStore, nil)

	var sent SendMessageRequest
	bridge := NewBridge(newTestClient(t, &sent), service, reminders, BridgeConfig{DataDir: root})

	first := WeixinMessage{
		FromUserID:   "user-1",
		ContextToken: "ctx-1",
		MessageType:  MessageTypeUser,
		MessageState: MessageStateFinish,
		ItemList:     []MessageItem{{Type: ItemTypeText, TextItem: &TextItem{Text: "第一个问题"}}},
	}
	second := first
	second.ContextToken = "ctx-2"
	second.ItemList = []MessageItem{{Type: ItemTypeText, TextItem: &TextItem{Text: "第二个问题"}}}

	bridge.handleMessage(context.Background(), first)
	bridge.handleMessage(context.Background(), second)

	items, err := sessionStore.List(context.Background())
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one stable weixin conversation, got %#v", items)
	}
	if items[0].Key != "source:weixin:user-1|session:weixin-user:user-1" {
		t.Fatalf("unexpected session key: %#v", items)
	}
	if len(bridge.conversationSessions) != 1 {
		t.Fatalf("expected one bound weixin slot, got %#v", bridge.conversationSessions)
	}
	if sessionID := strings.TrimSpace(bridge.conversationSessions["user:user-1"]); sessionID != "weixin-user:user-1" {
		t.Fatalf("unexpected bound session id: %#v", bridge.conversationSessions)
	}
}

func TestHandleMessageNewCommandStartsDistinctConversation(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store := knowledge.NewStore(filepath.Join(root, "entries.json"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(root, "reminders.json")))
	sessionStore := sessionstate.NewStore(filepath.Join(root, "sessions.json"))
	service := appsvc.NewServiceWithRuntime(store, nil, reminders, nil, sessionStore, nil)

	var sent SendMessageRequest
	bridge := NewBridge(newTestClient(t, &sent), service, reminders, BridgeConfig{DataDir: root})
	msg := WeixinMessage{
		FromUserID:   "user-1",
		ContextToken: "ctx-1",
		MessageType:  MessageTypeUser,
		MessageState: MessageStateFinish,
		ItemList:     []MessageItem{{Type: ItemTypeText, TextItem: &TextItem{Text: "/new"}}},
	}

	bridge.handleMessage(context.Background(), msg)

	slot := bridge.conversationSlotKey(msg)
	sessionID := strings.TrimSpace(bridge.conversationSessions[slot])
	if sessionID == "" || sessionID == weixinSessionID(msg) {
		t.Fatalf("expected explicit /new to allocate a distinct session id, got %q", sessionID)
	}
	snapshot, ok, err := sessionStore.Load(context.Background(), "source:weixin:user-1|session:"+sessionID)
	if err != nil {
		t.Fatalf("load new snapshot: %v", err)
	}
	if !ok {
		t.Fatalf("expected new session snapshot for %q", sessionID)
	}
	if len(snapshot.History) != 2 || snapshot.History[0].Content != "/new" || snapshot.History[1].Content != "已进入新对话。" {
		t.Fatalf("unexpected /new history: %#v", snapshot.History)
	}
	if extractText(sent.Msg) != "已进入新对话。" {
		t.Fatalf("unexpected /new outbound reply: %#v", sent.Msg)
	}
}

func TestHandleMessageAfterDeletedConversationStartsNewSession(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store := knowledge.NewStore(filepath.Join(root, "entries.json"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(root, "reminders.json")))
	sessionStore := sessionstate.NewStore(filepath.Join(root, "sessions.json"))
	service := appsvc.NewServiceWithRuntime(store, nil, reminders, nil, sessionStore, nil)
	if _, err := store.Add(context.Background(), knowledge.Entry{Text: "第一条知识"}); err != nil {
		t.Fatalf("add knowledge: %v", err)
	}

	var sent SendMessageRequest
	bridge := NewBridge(newTestClient(t, &sent), service, reminders, BridgeConfig{DataDir: root})
	msg := WeixinMessage{
		FromUserID:   "user-1",
		ContextToken: "ctx-1",
		MessageType:  MessageTypeUser,
		MessageState: MessageStateFinish,
		ItemList:     []MessageItem{{Type: ItemTypeText, TextItem: &TextItem{Text: "/list"}}},
	}

	bridge.handleMessage(context.Background(), msg)
	if err := sessionStore.Delete(context.Background(), "source:weixin:user-1|session:weixin-user:user-1"); err != nil {
		t.Fatalf("delete original snapshot: %v", err)
	}

	bridge.handleMessage(context.Background(), msg)

	sessionID := strings.TrimSpace(bridge.conversationSessions[bridge.conversationSlotKey(msg)])
	if sessionID == "" || sessionID == weixinSessionID(msg) {
		t.Fatalf("expected deleted conversation to be replaced, got %q", sessionID)
	}
	snapshot, ok, err := sessionStore.Load(context.Background(), "source:weixin:user-1|session:"+sessionID)
	if err != nil {
		t.Fatalf("load recreated snapshot: %v", err)
	}
	if !ok {
		t.Fatalf("expected recreated snapshot for %q", sessionID)
	}
	if !strings.Contains(snapshot.History[1].Content, "之前对话已丢失，已进入新对话。") {
		t.Fatalf("expected recreated session notice, got %#v", snapshot.History)
	}
}

type bridgeTestAI struct {
	toolOpportunities []aicore.ToolOpportunity
	toolPlanDecision  aicore.ToolPlanDecision
	route             aicore.RouteDecision
	chatFunc          func(context.Context, string, []aicore.ConversationMessage) string
}

func (f bridgeTestAI) IsConfigured(context.Context) (bool, error) {
	return true, nil
}

func (f bridgeTestAI) RouteCommand(context.Context, string) (aicore.RouteDecision, error) {
	return f.route, nil
}

func (f bridgeTestAI) BuildSearchPlan(context.Context, string) (aicore.SearchPlan, error) {
	return aicore.SearchPlan{}, nil
}

func (f bridgeTestAI) DetectToolOpportunities(context.Context, string, []aicore.ToolCapability) ([]aicore.ToolOpportunity, error) {
	return f.toolOpportunities, nil
}

func (f bridgeTestAI) PlanToolUse(context.Context, string, aicore.ToolCapability, []aicore.ToolExecution) (aicore.ToolPlanDecision, error) {
	return f.toolPlanDecision, nil
}

func (f bridgeTestAI) ReviewAnswerCandidates(context.Context, string, []knowledge.Entry) ([]string, error) {
	return nil, nil
}

func (f bridgeTestAI) Answer(context.Context, string, []knowledge.Entry) (string, error) {
	return "", nil
}

func (f bridgeTestAI) Chat(ctx context.Context, input string, history []aicore.ConversationMessage) (string, error) {
	if f.chatFunc != nil {
		return f.chatFunc(ctx, input, history), nil
	}
	return "", nil
}

func (f bridgeTestAI) DecideAgentStep(context.Context, string, []aicore.ConversationMessage, []aicore.AgentToolDefinition, []aicore.AgentToolResult) (aicore.AgentStepDecision, error) {
	return aicore.AgentStepDecision{}, nil
}

func (f bridgeTestAI) PlanNext(_ context.Context, _ string, _ []aicore.ConversationMessage, _ []aicore.AgentToolDefinition, _ aicore.AgentTaskState) (aicore.LoopDecision, error) {
	return aicore.LoopDecision{Action: aicore.LoopAnswer, Answer: ""}, nil
}

func (f bridgeTestAI) SummarizeWorkingState(_ context.Context, _ aicore.AgentTaskState) (string, error) {
	return "", nil
}

func (f bridgeTestAI) SummarizeFinal(_ context.Context, _ aicore.AgentTaskState, finalAnswer string) (string, error) {
	return finalAnswer, nil
}

func (f bridgeTestAI) TranslateToChinese(context.Context, string) (string, error) {
	return "", nil
}

func (f bridgeTestAI) SummarizePDFText(context.Context, string, string) (string, error) {
	return "", nil
}

func (f bridgeTestAI) SummarizeImageFile(context.Context, string, string) (string, error) {
	return "", nil
}
