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

func TestMaybeHandleFileFindSearchAndSelection(t *testing.T) {
	t.Parallel()

	bridge := NewBridge(NewClient("https://unit.test", ""), nil, nil, BridgeConfig{
		DataDir:        t.TempDir(),
		EverythingPath: "es.exe",
	})

	paths := []string{
		`E:\xwechat_files\a.pdf`,
		`E:\xwechat_files\b.pdf`,
	}
	bridge.searchFiles = func(_ context.Context, everythingPath string, input filesearch.ToolInput) (filesearch.ToolResult, error) {
		if everythingPath != "es.exe" {
			t.Fatalf("unexpected everything path: %q", everythingPath)
		}
		if input.Query != "单细胞" {
			t.Fatalf("unexpected query: %q", input.Query)
		}
		if input.Limit != findResultLimit {
			t.Fatalf("unexpected limit: %d", input.Limit)
		}
		return filesearch.ToolResult{
			Tool:  filesearch.ToolName,
			Query: input.Query,
			Limit: input.Limit,
			Count: len(paths),
			Items: []filesearch.ResultItem{
				{Index: 1, Name: "a.pdf", Path: paths[0]},
				{Index: 2, Name: "b.pdf", Path: paths[1]},
			},
		}, nil
	}

	var sentTo, sentToken, sentPath string
	bridge.sendFile = func(_ context.Context, toUserID, contextToken, filePath string) error {
		sentTo = toUserID
		sentToken = contextToken
		sentPath = filePath
		return nil
	}

	msg := WeixinMessage{FromUserID: "user-1", ContextToken: "ctx-1"}
	mc := bridge.conversationContext(msg, weixinSessionID(msg))
	reply, handled, err := bridge.maybeHandleFileFind(context.Background(), msg, mc, "/find 单细胞")
	if err != nil {
		t.Fatalf("search file: %v", err)
	}
	if !handled {
		t.Fatal("expected /find to be handled")
	}
	if !strings.Contains(reply, "找到 2 个文件") || !strings.Contains(reply, `E:\xwechat_files\b.pdf`) {
		t.Fatalf("unexpected search reply: %q", reply)
	}

	reply, handled, err = bridge.maybeHandleFileFind(context.Background(), msg, mc, "2")
	if err != nil {
		t.Fatalf("select file: %v", err)
	}
	if !handled {
		t.Fatal("expected selection to be handled")
	}
	if sentTo != "user-1" || sentToken != "ctx-1" || sentPath != paths[1] {
		t.Fatalf("unexpected send target: to=%q token=%q path=%q", sentTo, sentToken, sentPath)
	}
	if !strings.Contains(reply, "已通过 ClawBot 发送文件 2") {
		t.Fatalf("unexpected selection reply: %q", reply)
	}
	if _, ok := bridge.pendingFileSelection(bridge.conversationSlotKey(msg)); ok {
		t.Fatal("expected pending selection to be cleared")
	}
}

func TestMaybeHandleNaturalFileFindUsesAIIntent(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store := knowledge.NewStore(filepath.Join(root, "entries.json"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(root, "reminders.json")))
	service := appsvc.NewService(store, bridgeTestAI{
		intent: aicore.FileSearchIntent{
			Enabled: true,
			Query:   "d: ext:pdf 单细胞",
		},
	}, reminders)

	bridge := NewBridge(NewClient("https://unit.test", ""), service, reminders, BridgeConfig{
		DataDir:        root,
		EverythingPath: "es.exe",
	})
	bridge.searchFiles = func(_ context.Context, everythingPath string, input filesearch.ToolInput) (filesearch.ToolResult, error) {
		if everythingPath != "es.exe" {
			t.Fatalf("unexpected everything path: %q", everythingPath)
		}
		if input.Query != "d: ext:pdf 单细胞" {
			t.Fatalf("unexpected query: %q", input.Query)
		}
		if input.Limit != findResultLimit {
			t.Fatalf("unexpected limit: %d", input.Limit)
		}
		return filesearch.ToolResult{
			Tool:  filesearch.ToolName,
			Query: input.Query,
			Limit: input.Limit,
			Count: 1,
			Items: []filesearch.ResultItem{
				{Index: 1, Name: "单细胞报告.pdf", Path: `D:\docs\单细胞报告.pdf`},
			},
		}, nil
	}
	msg := WeixinMessage{FromUserID: "user-1", ContextToken: "ctx-1"}
	mc := bridge.conversationContext(msg, weixinSessionID(msg))

	reply, handled, err := bridge.maybeHandleFileFind(context.Background(), msg, mc, "查找 D 盘单细胞相关的PDF文件")
	if err != nil {
		t.Fatalf("natural search: %v", err)
	}
	if !handled {
		t.Fatal("expected natural language file find to be handled")
	}
	if !strings.Contains(reply, "检索式: d: ext:pdf 单细胞") {
		t.Fatalf("unexpected reply: %q", reply)
	}
}

func TestMaybeHandleFindHelpReturnsModuleHelp(t *testing.T) {
	t.Parallel()

	bridge := NewBridge(NewClient("https://unit.test", ""), nil, nil, BridgeConfig{
		DataDir:        t.TempDir(),
		EverythingPath: "es.exe",
	})
	msg := WeixinMessage{FromUserID: "user-1", ContextToken: "ctx-1"}
	mc := bridge.conversationContext(msg, weixinSessionID(msg))

	reply, handled, err := bridge.maybeHandleFileFind(context.Background(), msg, mc, "/find help")
	if err != nil {
		t.Fatalf("find help: %v", err)
	}
	if !handled {
		t.Fatal("expected /find help to be handled")
	}
	if !strings.Contains(reply, filesearch.ToolName) || !strings.Contains(reply, "/find help") {
		t.Fatalf("unexpected help reply: %q", reply)
	}
}

func TestMaybeHandleSlashFindUsesAIIntentForNaturalLanguageQuery(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store := knowledge.NewStore(filepath.Join(root, "entries.json"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(root, "reminders.json")))
	service := appsvc.NewService(store, bridgeTestAI{
		intent: aicore.FileSearchIntent{
			Enabled: true,
			Query:   "file: shell:Downloads *.pdf",
		},
	}, reminders)

	bridge := NewBridge(NewClient("https://unit.test", ""), service, reminders, BridgeConfig{
		DataDir:        root,
		EverythingPath: "es.exe",
	})
	bridge.searchFiles = func(_ context.Context, everythingPath string, input filesearch.ToolInput) (filesearch.ToolResult, error) {
		if input.Query != "file: shell:Downloads *.pdf" {
			t.Fatalf("unexpected query: %q", input.Query)
		}
		return filesearch.ToolResult{
			Tool:  filesearch.ToolName,
			Query: input.Query,
			Limit: input.Limit,
			Count: 1,
			Items: []filesearch.ResultItem{
				{Index: 1, Name: "单细胞.pdf", Path: `C:\Users\demo\Downloads\单细胞.pdf`},
			},
		}, nil
	}
	msg := WeixinMessage{FromUserID: "user-1", ContextToken: "ctx-1"}
	mc := bridge.conversationContext(msg, weixinSessionID(msg))

	reply, handled, err := bridge.maybeHandleFileFind(context.Background(), msg, mc, "/find 查找下载目录下的pdf文件")
	if err != nil {
		t.Fatalf("slash natural search: %v", err)
	}
	if !handled {
		t.Fatal("expected /find natural language file find to be handled")
	}
	if !strings.Contains(reply, "检索式: file: shell:Downloads *.pdf") {
		t.Fatalf("unexpected reply: %q", reply)
	}
}

func TestHandleMessageRecordsFileFindConversation(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store := knowledge.NewStore(filepath.Join(root, "entries.json"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(root, "reminders.json")))
	sessionStore := sessionstate.NewStore(filepath.Join(root, "sessions.json"))
	service := appsvc.NewServiceWithRuntime(store, bridgeTestAI{
		intent: aicore.FileSearchIntent{
			Enabled: true,
			Query:   "d: ext:pdf 单细胞",
		},
	}, reminders, nil, sessionStore, nil)

	var sent SendMessageRequest
	bridge := NewBridge(newTestClient(t, &sent), service, reminders, BridgeConfig{
		DataDir:        root,
		EverythingPath: "es.exe",
	})
	bridge.searchFiles = func(_ context.Context, everythingPath string, input filesearch.ToolInput) (filesearch.ToolResult, error) {
		return filesearch.ToolResult{
			Tool:  filesearch.ToolName,
			Query: input.Query,
			Limit: input.Limit,
			Count: 1,
			Items: []filesearch.ResultItem{
				{Index: 1, Name: "单细胞报告.pdf", Path: `D:\docs\单细胞报告.pdf`},
			},
		}, nil
	}
	bridge.sendFile = func(context.Context, string, string, string) error { return nil }

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

	snapshot, ok, err := sessionStore.Load(context.Background(), "source:weixin:user-1|session:weixin:ctx-1")
	if err != nil {
		t.Fatalf("load recorded snapshot: %v", err)
	}
	if !ok {
		t.Fatal("expected file find conversation to be recorded")
	}
	if len(snapshot.History) != 2 {
		t.Fatalf("expected user+assistant history, got %#v", snapshot.History)
	}
	if snapshot.History[0].Content != "帮我找单细胞 pdf" {
		t.Fatalf("unexpected recorded user message: %#v", snapshot.History[0])
	}
	if !strings.Contains(snapshot.History[1].Content, "找到 1 个文件") || !strings.Contains(snapshot.History[1].Content, "检索式: d: ext:pdf 单细胞") {
		t.Fatalf("unexpected recorded assistant reply: %#v", snapshot.History[1])
	}
	if sent.Msg.ToUserID != "user-1" || sent.Msg.ContextToken != "ctx-1" {
		t.Fatalf("unexpected send target: %#v", sent.Msg)
	}

	msg.ItemList = []MessageItem{{Type: ItemTypeText, TextItem: &TextItem{Text: "1"}}}
	bridge.handleMessage(context.Background(), msg)

	snapshot, ok, err = sessionStore.Load(context.Background(), "source:weixin:user-1|session:weixin:ctx-1")
	if err != nil {
		t.Fatalf("reload recorded snapshot: %v", err)
	}
	if !ok || len(snapshot.History) != 4 {
		t.Fatalf("expected selection turn to be recorded, got %#v", snapshot.History)
	}
	if snapshot.History[2].Content != "1" {
		t.Fatalf("unexpected recorded selection: %#v", snapshot.History[2])
	}
	if !strings.Contains(snapshot.History[3].Content, "已通过 ClawBot 发送文件 1") {
		t.Fatalf("unexpected recorded selection reply: %#v", snapshot.History[3])
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

	snapshot, ok, err := sessionStore.Load(context.Background(), "source:weixin:user-1|session:weixin:ctx-1")
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
	if err := sessionStore.Delete(context.Background(), "source:weixin:user-1|session:weixin:ctx-1"); err != nil {
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
	intent aicore.FileSearchIntent
}

func (f bridgeTestAI) IsConfigured(context.Context) (bool, error) {
	return true, nil
}

func (f bridgeTestAI) RouteCommand(context.Context, string) (aicore.RouteDecision, error) {
	return aicore.RouteDecision{}, nil
}

func (f bridgeTestAI) BuildSearchPlan(context.Context, string) (aicore.SearchPlan, error) {
	return aicore.SearchPlan{}, nil
}

func (f bridgeTestAI) BuildFileSearchIntent(context.Context, string) (aicore.FileSearchIntent, error) {
	return f.intent, nil
}

func (f bridgeTestAI) ReviewAnswerCandidates(context.Context, string, []knowledge.Entry) ([]string, error) {
	return nil, nil
}

func (f bridgeTestAI) Answer(context.Context, string, []knowledge.Entry) (string, error) {
	return "", nil
}

func (f bridgeTestAI) Chat(context.Context, string, []aicore.ConversationMessage) (string, error) {
	return "", nil
}

func (f bridgeTestAI) DecideAgentStep(context.Context, string, []aicore.ConversationMessage, []aicore.AgentToolDefinition, []aicore.AgentToolResult) (aicore.AgentStepDecision, error) {
	return aicore.AgentStepDecision{}, nil
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
