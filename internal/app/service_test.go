package app

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"myclaw/internal/ai"
	"myclaw/internal/knowledge"
	"myclaw/internal/reminder"
)

func TestHandleMessageRememberAndList(t *testing.T) {
	t.Parallel()

	store := knowledge.NewStore(filepath.Join(t.TempDir(), "entries.json"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(t.TempDir(), "reminders.json")))
	service := NewService(store, nil, reminders)
	ctx := context.Background()

	reply, err := service.HandleMessage(ctx, MessageContext{UserID: "u1", Interface: "weixin"}, "记住：Windows 版本先做微信接口")
	if err != nil {
		t.Fatalf("remember failed: %v", err)
	}
	if !strings.Contains(reply, "已记住") {
		t.Fatalf("unexpected remember reply: %q", reply)
	}

	reply, err = service.HandleMessage(ctx, MessageContext{}, "/list")
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if !strings.Contains(reply, "Windows 版本先做微信接口") {
		t.Fatalf("list reply missing entry: %q", reply)
	}
}

func TestHandleMessageQuestionReturnsAllKnowledge(t *testing.T) {
	t.Parallel()

	store := knowledge.NewStore(filepath.Join(t.TempDir(), "entries.json"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(t.TempDir(), "reminders.json")))
	service := NewService(store, fakeAI{
		configured: true,
		route: ai.RouteDecision{
			Command:  "answer",
			Question: "macOS 什么时候做？",
		},
		answer: "知识库里提到未来需要支持 macOS，目前还没有实现时间表。",
	}, reminders)
	ctx := context.Background()

	if _, err := service.HandleMessage(ctx, MessageContext{}, "/remember 未来需要支持 macOS"); err != nil {
		t.Fatalf("remember macos: %v", err)
	}
	if _, err := service.HandleMessage(ctx, MessageContext{}, "/remember 现在只做最小知识库检索"); err != nil {
		t.Fatalf("remember retrieval: %v", err)
	}

	reply, err := service.HandleMessage(ctx, MessageContext{}, "macOS 什么时候做？")
	if err != nil {
		t.Fatalf("question failed: %v", err)
	}
	if !strings.Contains(reply, "未来需要支持 macOS") {
		t.Fatalf("unexpected answer reply: %q", reply)
	}
}

func TestHandleMessageUsesAIIntentRecognition(t *testing.T) {
	t.Parallel()

	store := knowledge.NewStore(filepath.Join(t.TempDir(), "entries.json"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(t.TempDir(), "reminders.json")))
	service := NewService(store, fakeAI{
		configured: true,
		route: ai.RouteDecision{
			Command:    "remember",
			MemoryText: "## 整理后的记忆\n- 未来要支持 macOS",
		},
	}, reminders)

	reply, err := service.HandleMessage(context.Background(), MessageContext{}, "请帮我记住这个东西：未来要支持 macOS")
	if err != nil {
		t.Fatalf("handle message: %v", err)
	}
	if !strings.Contains(reply, "已记住") {
		t.Fatalf("unexpected reply: %q", reply)
	}

	entries, err := store.List(context.Background())
	if err != nil {
		t.Fatalf("list entries: %v", err)
	}
	if len(entries) != 1 || !strings.Contains(entries[0].Text, "整理后的记忆") {
		t.Fatalf("unexpected entries: %#v", entries)
	}
}

func TestAppendCommandUpdatesExistingKnowledge(t *testing.T) {
	t.Parallel()

	store := knowledge.NewStore(filepath.Join(t.TempDir(), "entries.json"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(t.TempDir(), "reminders.json")))
	service := NewService(store, nil, reminders)

	if _, err := store.Add(context.Background(), knowledge.Entry{
		ID:         "6d2d7724abcd1234",
		Text:       "Puppeteer 是一个浏览器自动化工具。",
		RecordedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed entry: %v", err)
	}

	reply, err := service.HandleMessage(context.Background(), MessageContext{}, "/append 6d2d7724 它是 Google 出品的一个工具。")
	if err != nil {
		t.Fatalf("append command: %v", err)
	}
	if !strings.Contains(reply, "已补充 #6d2d7724") {
		t.Fatalf("unexpected reply: %q", reply)
	}

	entries, err := store.List(context.Background())
	if err != nil {
		t.Fatalf("list entries: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if !strings.Contains(entries[0].Text, "Google 出品") {
		t.Fatalf("expected appended text, got %q", entries[0].Text)
	}
}

func TestNaturalAppendByIDUpdatesExistingKnowledge(t *testing.T) {
	t.Parallel()

	store := knowledge.NewStore(filepath.Join(t.TempDir(), "entries.json"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(t.TempDir(), "reminders.json")))
	service := NewService(store, nil, reminders)

	if _, err := store.Add(context.Background(), knowledge.Entry{
		ID:         "6d2d7724abcd1234",
		Text:       "Puppeteer 是一个浏览器自动化工具。",
		RecordedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed entry: %v", err)
	}

	reply, err := service.HandleMessage(context.Background(), MessageContext{}, "给 #6d2d7724 补充：它是 Google 出品的一个工具。")
	if err != nil {
		t.Fatalf("natural append by id: %v", err)
	}
	if !strings.Contains(reply, "已补充 #6d2d7724") {
		t.Fatalf("unexpected reply: %q", reply)
	}

	entries, err := store.List(context.Background())
	if err != nil {
		t.Fatalf("list entries: %v", err)
	}
	if len(entries) != 1 || !strings.Contains(entries[0].Text, "Google 出品") {
		t.Fatalf("unexpected entries: %#v", entries)
	}
}

func TestTranslateCommandUsesAITranslator(t *testing.T) {
	t.Parallel()

	store := knowledge.NewStore(filepath.Join(t.TempDir(), "entries.json"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(t.TempDir(), "reminders.json")))
	service := NewService(store, fakeAI{
		configured:  true,
		translation: "Puppeteer 是一个浏览器自动化工具。",
	}, reminders)

	reply, err := service.HandleMessage(context.Background(), MessageContext{}, "/translate Puppeteer is a browser automation tool.")
	if err != nil {
		t.Fatalf("translate command: %v", err)
	}
	if !strings.Contains(reply, "浏览器自动化工具") {
		t.Fatalf("unexpected reply: %q", reply)
	}
}

func TestHandleMessageRequiresConfiguredModelForNaturalLanguage(t *testing.T) {
	t.Parallel()

	store := knowledge.NewStore(filepath.Join(t.TempDir(), "entries.json"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(t.TempDir(), "reminders.json")))
	service := NewService(store, fakeAI{configured: false}, reminders)

	reply, err := service.HandleMessage(context.Background(), MessageContext{}, "帮我看看知识库里有什么")
	if err != nil {
		t.Fatalf("handle message: %v", err)
	}
	if !strings.Contains(reply, "模型还没有配置完成") {
		t.Fatalf("unexpected reply: %q", reply)
	}
}

func TestNoticeCreatesReminder(t *testing.T) {
	t.Parallel()

	store := knowledge.NewStore(filepath.Join(t.TempDir(), "entries.json"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(t.TempDir(), "reminders.json")))
	service := NewService(store, nil, reminders)

	reply, err := service.HandleMessage(context.Background(), MessageContext{
		UserID:    "u1",
		Interface: "terminal",
	}, "/notice 2小时后 喝水")
	if err != nil {
		t.Fatalf("create reminder: %v", err)
	}
	if !strings.Contains(reply, "已创建提醒") {
		t.Fatalf("unexpected reply: %q", reply)
	}

	items, err := reminders.List(context.Background(), reminder.Target{Interface: "terminal", UserID: "u1"})
	if err != nil {
		t.Fatalf("list reminders: %v", err)
	}
	if len(items) != 1 || items[0].Message != "喝水" {
		t.Fatalf("unexpected reminders: %#v", items)
	}
}

func TestNaturalAppendLastUpdatesLatestKnowledgeFromSameSource(t *testing.T) {
	t.Parallel()

	store := knowledge.NewStore(filepath.Join(t.TempDir(), "entries.json"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(t.TempDir(), "reminders.json")))
	service := NewService(store, nil, reminders)

	if _, err := store.Add(context.Background(), knowledge.Entry{
		ID:         "11111111aaaa1111",
		Text:       "old same source",
		Source:     "weixin:u1",
		RecordedAt: time.Date(2026, 3, 27, 10, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("seed old same source: %v", err)
	}
	if _, err := store.Add(context.Background(), knowledge.Entry{
		ID:         "22222222bbbb2222",
		Text:       "other source latest",
		Source:     "weixin:u2",
		RecordedAt: time.Date(2026, 3, 27, 12, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("seed other source: %v", err)
	}
	if _, err := store.Add(context.Background(), knowledge.Entry{
		ID:         "33333333cccc3333",
		Text:       "Puppeteer 是一个浏览器自动化工具。",
		Source:     "weixin:u1",
		RecordedAt: time.Date(2026, 3, 27, 11, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("seed latest same source: %v", err)
	}

	reply, err := service.HandleMessage(context.Background(), MessageContext{
		UserID:    "u1",
		Interface: "weixin",
	}, "再补充一点：它是 Google 出品的一个工具。")
	if err != nil {
		t.Fatalf("natural append last: %v", err)
	}
	if !strings.Contains(reply, "已补充 #33333333") {
		t.Fatalf("unexpected reply: %q", reply)
	}

	entries, err := store.List(context.Background())
	if err != nil {
		t.Fatalf("list entries: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
	if !strings.Contains(entries[1].Text, "Google 出品") {
		t.Fatalf("expected latest same-source entry to be appended, got %#v", entries[1])
	}
	if strings.Contains(entries[2].Text, "Google 出品") {
		t.Fatalf("should not append to another source entry: %#v", entries[2])
	}
}

func TestNaturalReminderCreatesReminder(t *testing.T) {
	t.Parallel()

	store := knowledge.NewStore(filepath.Join(t.TempDir(), "entries.json"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(t.TempDir(), "reminders.json")))
	service := NewService(store, nil, reminders)

	reply, err := service.HandleMessage(context.Background(), MessageContext{
		UserID:    "u1",
		Interface: "terminal",
	}, "一分钟后提醒我喝水")
	if err != nil {
		t.Fatalf("create natural reminder: %v", err)
	}
	if !strings.Contains(reply, "已创建提醒") {
		t.Fatalf("unexpected reply: %q", reply)
	}

	items, err := reminders.List(context.Background(), reminder.Target{Interface: "terminal", UserID: "u1"})
	if err != nil {
		t.Fatalf("list reminders: %v", err)
	}
	if len(items) != 1 || items[0].Message != "喝水" {
		t.Fatalf("unexpected reminders: %#v", items)
	}

	entries, err := store.List(context.Background())
	if err != nil {
		t.Fatalf("list entries: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected reminder not to be stored as knowledge: %#v", entries)
	}
}

func TestNoticeListAndRemove(t *testing.T) {
	t.Parallel()

	store := knowledge.NewStore(filepath.Join(t.TempDir(), "entries.json"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(t.TempDir(), "reminders.json")))
	service := NewService(store, nil, reminders)

	item, err := reminders.ScheduleDaily(context.Background(), reminder.Target{
		Interface: "terminal",
		UserID:    "u1",
	}, 9, 0, "写日报")
	if err != nil {
		t.Fatalf("seed daily reminder: %v", err)
	}

	listReply, err := service.HandleMessage(context.Background(), MessageContext{
		UserID:    "u1",
		Interface: "terminal",
	}, "/notice list")
	if err != nil {
		t.Fatalf("list reminders: %v", err)
	}
	if !strings.Contains(listReply, "写日报") {
		t.Fatalf("unexpected list reply: %q", listReply)
	}

	removeReply, err := service.HandleMessage(context.Background(), MessageContext{
		UserID:    "u1",
		Interface: "terminal",
	}, "/notice remove "+item.ID[:8])
	if err != nil {
		t.Fatalf("remove reminder: %v", err)
	}
	if !strings.Contains(removeReply, "已删除提醒") {
		t.Fatalf("unexpected remove reply: %q", removeReply)
	}
}

func TestCronAliasCreatesDateReminder(t *testing.T) {
	t.Parallel()

	store := knowledge.NewStore(filepath.Join(t.TempDir(), "entries.json"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(t.TempDir(), "reminders.json")))
	service := NewService(store, nil, reminders)

	reply, err := service.HandleMessage(context.Background(), MessageContext{
		UserID:    "u1",
		Interface: "terminal",
	}, "/cron 2099-03-30 14:00 交房租")
	if err != nil {
		t.Fatalf("create cron reminder: %v", err)
	}
	if !strings.Contains(reply, "交房租") {
		t.Fatalf("unexpected reply: %q", reply)
	}

	items, err := reminders.List(context.Background(), reminder.Target{Interface: "terminal", UserID: "u1"})
	if err != nil {
		t.Fatalf("list reminders: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 reminder, got %d", len(items))
	}
	if !items[0].NextRunAt.After(time.Now()) {
		t.Fatalf("expected future reminder, got %v", items[0].NextRunAt)
	}
}

func TestForgetRemovesKnowledge(t *testing.T) {
	t.Parallel()

	store := knowledge.NewStore(filepath.Join(t.TempDir(), "entries.json"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(t.TempDir(), "reminders.json")))
	service := NewService(store, nil, reminders)

	_, err := store.Add(context.Background(), knowledge.Entry{
		ID:         "0015f908abcd1234",
		Text:       "喝水提醒偏好",
		RecordedAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("seed entry: %v", err)
	}

	reply, err := service.HandleMessage(context.Background(), MessageContext{}, "删掉 #0015f908")
	if err != nil {
		t.Fatalf("forget entry: %v", err)
	}
	if !strings.Contains(reply, "已遗忘 #0015f908") {
		t.Fatalf("unexpected reply: %q", reply)
	}

	entries, err := store.List(context.Background())
	if err != nil {
		t.Fatalf("list entries: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected empty store, got %#v", entries)
	}
}

func TestHandleMessageUsesAIRouteForReminderList(t *testing.T) {
	t.Parallel()

	store := knowledge.NewStore(filepath.Join(t.TempDir(), "entries.json"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(t.TempDir(), "reminders.json")))
	service := NewService(store, fakeAI{
		configured: true,
		route: ai.RouteDecision{
			Command: "notice_list",
		},
	}, reminders)

	reply, err := service.HandleMessage(context.Background(), MessageContext{
		UserID:    "u1",
		Interface: "terminal",
	}, "帮我看看当前有哪些提醒")
	if err != nil {
		t.Fatalf("handle message: %v", err)
	}
	if !strings.Contains(reply, "当前没有提醒") {
		t.Fatalf("unexpected reply: %q", reply)
	}
}

func TestHandleMessageUsesAIRouteForAppendLast(t *testing.T) {
	t.Parallel()

	store := knowledge.NewStore(filepath.Join(t.TempDir(), "entries.json"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(t.TempDir(), "reminders.json")))
	service := NewService(store, fakeAI{
		configured: true,
		route: ai.RouteDecision{
			Command:    "append_last",
			AppendText: "它是 Google 出品的一个工具。",
		},
	}, reminders)

	if _, err := store.Add(context.Background(), knowledge.Entry{
		ID:         "6d2d7724abcd1234",
		Text:       "Puppeteer 是一个浏览器自动化工具。",
		Source:     "terminal:u1",
		RecordedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed entry: %v", err)
	}

	reply, err := service.HandleMessage(context.Background(), MessageContext{
		UserID:    "u1",
		Interface: "terminal",
	}, "我看了这个 Puppeteer，想再追加一点笔记")
	if err != nil {
		t.Fatalf("handle message: %v", err)
	}
	if !strings.Contains(reply, "已补充 #6d2d7724") {
		t.Fatalf("unexpected reply: %q", reply)
	}

	entries, err := store.List(context.Background())
	if err != nil {
		t.Fatalf("list entries: %v", err)
	}
	if len(entries) != 1 || !strings.Contains(entries[0].Text, "Google 出品") {
		t.Fatalf("unexpected entries: %#v", entries)
	}
}

type fakeAI struct {
	configured  bool
	route       ai.RouteDecision
	answer      string
	translation string
}

func (f fakeAI) IsConfigured(context.Context) (bool, error) {
	return f.configured, nil
}

func (f fakeAI) RouteCommand(context.Context, string) (ai.RouteDecision, error) {
	return f.route, nil
}

func (f fakeAI) Answer(context.Context, string, []knowledge.Entry) (string, error) {
	return f.answer, nil
}

func (f fakeAI) TranslateToChinese(context.Context, string) (string, error) {
	return f.translation, nil
}
