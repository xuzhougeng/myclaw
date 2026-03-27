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
		intent: ai.IntentDecision{
			Intent:   "answer",
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
		intent: ai.IntentDecision{
			Intent:     "remember",
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

type fakeAI struct {
	configured bool
	intent     ai.IntentDecision
	answer     string
}

func (f fakeAI) IsConfigured(context.Context) (bool, error) {
	return f.configured, nil
}

func (f fakeAI) RecognizeIntent(context.Context, string) (ai.IntentDecision, error) {
	return f.intent, nil
}

func (f fakeAI) Answer(context.Context, string, []knowledge.Entry) (string, error) {
	return f.answer, nil
}
