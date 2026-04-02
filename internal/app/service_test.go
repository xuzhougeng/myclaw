package app

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"baize/internal/ai"
	"baize/internal/fileingest"
	"baize/internal/filesearch"
	"baize/internal/knowledge"
	"baize/internal/projectstate"
	"baize/internal/promptlib"
	"baize/internal/reminder"
	"baize/internal/sessionstate"
	"baize/internal/skilllib"
)

func TestHandleMessageRememberAndList(t *testing.T) {
	t.Parallel()

	store := knowledge.NewStore(filepath.Join(t.TempDir(), "app.db"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(t.TempDir(), "app.db")))
	service := NewService(store, nil, reminders)
	ctx := context.Background()

	reply, err := service.HandleMessage(ctx, MessageContext{UserID: "u1", Interface: "weixin"}, "记住：Windows 版本先做微信接口")
	if err != nil {
		t.Fatalf("remember failed: %v", err)
	}
	if !strings.Contains(reply, "已记住") {
		t.Fatalf("unexpected remember reply: %q", reply)
	}

	reply, err = service.HandleMessage(ctx, MessageContext{}, "/kb list")
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if !strings.Contains(reply, "Windows 版本先做微信接口") {
		t.Fatalf("list reply missing entry: %q", reply)
	}
}

func TestKnowledgeBaseCommandsCreateSwitchAndListScopes(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	appDBPath := filepath.Join(root, "app.db")
	store := knowledge.NewStore(appDBPath)
	projectStore := projectstate.NewStore(appDBPath)
	reminders := reminder.NewManager(reminder.NewStore(appDBPath))
	service := NewService(store, nil, reminders)
	service.SetProjectStore(projectStore)
	ctx := context.Background()
	mc := MessageContext{Interface: "terminal", UserID: "terminal"}

	reply, err := service.HandleMessage(ctx, mc, "/kb")
	if err != nil {
		t.Fatalf("show kb state: %v", err)
	}
	if !strings.Contains(reply, "当前知识库：default") {
		t.Fatalf("unexpected default kb reply: %q", reply)
	}

	reply, err = service.HandleMessage(ctx, mc, "/kb new Alpha")
	if err != nil {
		t.Fatalf("create alpha kb: %v", err)
	}
	if !strings.Contains(reply, "Alpha") {
		t.Fatalf("unexpected create alpha reply: %q", reply)
	}

	if _, err := service.HandleMessage(ctx, mc, "/kb remember alpha note"); err != nil {
		t.Fatalf("remember alpha note: %v", err)
	}

	if _, err := service.HandleMessage(ctx, mc, "/kb switch Beta"); err != nil {
		t.Fatalf("switch beta kb: %v", err)
	}
	if _, err := service.HandleMessage(ctx, mc, "/kb remember beta note"); err != nil {
		t.Fatalf("remember beta note: %v", err)
	}

	reply, err = service.HandleMessage(ctx, mc, "/kb")
	if err != nil {
		t.Fatalf("show kb summary: %v", err)
	}
	for _, expected := range []string{"当前知识库：Beta", "Alpha (1 条)", "Beta [当前] (1 条)", "default (0 条)"} {
		if !strings.Contains(reply, expected) {
			t.Fatalf("expected %q in kb summary, got %q", expected, reply)
		}
	}

	reply, err = service.HandleMessage(ctx, mc, "/kb switch Alpha")
	if err != nil {
		t.Fatalf("switch alpha kb: %v", err)
	}
	if !strings.Contains(reply, "已切换到知识库 Alpha") {
		t.Fatalf("unexpected switch alpha reply: %q", reply)
	}

	reply, err = service.HandleMessage(ctx, mc, "/kb list")
	if err != nil {
		t.Fatalf("list alpha kb: %v", err)
	}
	if !strings.Contains(reply, "alpha note") || strings.Contains(reply, "beta note") {
		t.Fatalf("expected alpha-only kb list, got %q", reply)
	}
}

func TestHandleMessageQuestionUsesReviewedKnowledgeSubset(t *testing.T) {
	t.Parallel()

	store := knowledge.NewStore(filepath.Join(t.TempDir(), "app.db"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(t.TempDir(), "app.db")))
	service := NewService(store, fakeAI{
		configured: true,
		route: ai.RouteDecision{
			Command:  "answer",
			Question: "macOS 什么时候做？",
		},
	}, reminders)
	ctx := context.Background()

	macEntry, err := store.Add(ctx, knowledge.Entry{
		ID:         "11111111aaaa1111",
		Text:       "未来需要支持 macOS。",
		RecordedAt: time.Date(2026, 3, 27, 10, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("remember macos: %v", err)
	}
	if _, err := store.Add(ctx, knowledge.Entry{
		ID:         "22222222bbbb2222",
		Text:       "现在只做最小知识库检索。",
		RecordedAt: time.Date(2026, 3, 27, 11, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("remember retrieval: %v", err)
	}
	service.aiService = fakeAI{
		configured: true,
		route: ai.RouteDecision{
			Command:  "answer",
			Question: "macOS 什么时候做？",
		},
		searchPlan: ai.SearchPlan{
			Queries:  []string{"macOS 支持计划"},
			Keywords: []string{"macos", "支持"},
		},
		reviewIDs: []string{macEntry.ID},
		answerFunc: func(_ string, entries []knowledge.Entry) string {
			if len(entries) != 1 {
				t.Fatalf("expected 1 reviewed entry, got %#v", entries)
			}
			if entries[0].ID != macEntry.ID {
				t.Fatalf("expected macOS entry, got %#v", entries)
			}
			return "知识库里提到未来需要支持 macOS，目前还没有实现时间表。"
		},
	}
	reply, err := service.HandleMessage(ctx, MessageContext{}, "@kb macOS 什么时候做？")
	if err != nil {
		t.Fatalf("question failed: %v", err)
	}
	if !strings.Contains(reply, "未来需要支持 macOS") {
		t.Fatalf("unexpected answer reply: %q", reply)
	}
}

func TestSkillsCanBeListedLoadedAndUnloaded(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	skillDir := filepath.Join(root, "skills", "writer")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir skill dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(strings.TrimSpace(`
---
name: writer
description: 帮助输出更清晰的中文写作
---
# Writer
Use concise Chinese writing.
`)), 0o644); err != nil {
		t.Fatalf("write skill file: %v", err)
	}

	store := knowledge.NewStore(filepath.Join(root, "app.db"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(root, "app.db")))
	service := NewServiceWithSkills(store, nil, reminders, skilllib.NewLoader(filepath.Join(root, "skills")))
	mc := MessageContext{UserID: "u1", Interface: "terminal"}

	reply, err := service.HandleMessage(context.Background(), mc, "/skill list")
	if err != nil {
		t.Fatalf("list skills: %v", err)
	}
	if !strings.Contains(reply, "writer") {
		t.Fatalf("expected listed skill, got %q", reply)
	}

	reply, err = service.HandleMessage(context.Background(), mc, "/skill show writer")
	if err != nil {
		t.Fatalf("show skill: %v", err)
	}
	if !strings.Contains(reply, "# Writer") {
		t.Fatalf("expected skill content, got %q", reply)
	}

	reply, err = service.HandleMessage(context.Background(), mc, "/skill load writer")
	if err != nil {
		t.Fatalf("load skill: %v", err)
	}
	if !strings.Contains(reply, "已加载技能 writer") {
		t.Fatalf("unexpected load reply: %q", reply)
	}

	reply, err = service.HandleMessage(context.Background(), mc, "/skill list")
	if err != nil {
		t.Fatalf("list skills after load: %v", err)
	}
	if !strings.Contains(reply, "[已加载]") {
		t.Fatalf("expected loaded marker, got %q", reply)
	}

	reply, err = service.HandleMessage(context.Background(), mc, "/skill")
	if err != nil {
		t.Fatalf("page skills: %v", err)
	}
	if !strings.Contains(reply, "writer") {
		t.Fatalf("expected loaded page skill, got %q", reply)
	}

	reply, err = service.HandleMessage(context.Background(), mc, "/skill unload writer")
	if err != nil {
		t.Fatalf("unload skill: %v", err)
	}
	if !strings.Contains(reply, "已卸载技能 writer") {
		t.Fatalf("unexpected unload reply: %q", reply)
	}
}

func TestLoadedSkillIsInjectedIntoAIContext(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	skillDir := filepath.Join(root, "skills", "translator")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir skill dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(strings.TrimSpace(`
---
name: translator
description: 翻译时优先保留技术术语
---
# Translator
Preserve technical terms whenever possible.
`)), 0o644); err != nil {
		t.Fatalf("write skill file: %v", err)
	}

	store := knowledge.NewStore(filepath.Join(root, "app.db"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(root, "app.db")))
	service := NewServiceWithSkills(store, fakeAI{
		configured: true,
		translationFunc: func(ctx context.Context, input string) string {
			if input != "hello" {
				t.Fatalf("unexpected translation input: %q", input)
			}
			skillContext := ai.SkillContextFromContext(ctx)
			if !strings.Contains(skillContext, "translator") || !strings.Contains(skillContext, "Preserve technical terms") {
				t.Fatalf("skill context missing from request: %q", skillContext)
			}
			return "你好"
		},
	}, reminders, skilllib.NewLoader(filepath.Join(root, "skills")))
	mc := MessageContext{UserID: "u1", Interface: "terminal"}

	if _, err := service.HandleMessage(context.Background(), mc, "/skill load translator"); err != nil {
		t.Fatalf("load skill: %v", err)
	}

	reply, err := service.HandleMessage(context.Background(), mc, "/translate hello")
	if err != nil {
		t.Fatalf("translate with skill: %v", err)
	}
	if reply != "你好" {
		t.Fatalf("unexpected translate reply: %q", reply)
	}
}

func TestLoadedSkillsAreScopedBySessionID(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	skillDir := filepath.Join(root, "skills", "writer")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir skill dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(strings.TrimSpace(`
---
name: writer
description: 帮助输出更清晰的中文写作
---
# Writer
Use concise Chinese writing.
`)), 0o644); err != nil {
		t.Fatalf("write skill file: %v", err)
	}

	store := knowledge.NewStore(filepath.Join(root, "app.db"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(root, "app.db")))
	service := NewServiceWithSkills(store, nil, reminders, skilllib.NewLoader(filepath.Join(root, "skills")))

	mcA := MessageContext{UserID: "u1", Interface: "desktop", SessionID: "page-a"}
	mcB := MessageContext{UserID: "u1", Interface: "desktop", SessionID: "page-b"}

	reply, err := service.HandleMessage(context.Background(), mcA, "/skill load writer")
	if err != nil {
		t.Fatalf("load skill for page a: %v", err)
	}
	if !strings.Contains(reply, "已加载技能 writer") {
		t.Fatalf("unexpected load reply: %q", reply)
	}

	reply, err = service.HandleMessage(context.Background(), mcA, "/skill")
	if err != nil {
		t.Fatalf("page skills for page a: %v", err)
	}
	if !strings.Contains(reply, "writer") {
		t.Fatalf("expected loaded skill in page a, got %q", reply)
	}

	reply, err = service.HandleMessage(context.Background(), mcB, "/skill")
	if err != nil {
		t.Fatalf("page skills for page b: %v", err)
	}
	if !strings.Contains(reply, "还没有加载技能") {
		t.Fatalf("expected no loaded skills in page b, got %q", reply)
	}
}

func TestHandleMessageDefaultsToAgentMode(t *testing.T) {
	t.Parallel()

	store := knowledge.NewStore(filepath.Join(t.TempDir(), "app.db"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(t.TempDir(), "app.db")))
	service := NewService(store, fakeAI{
		configured: true,
		route: ai.RouteDecision{
			Command:  "answer",
			Question: "macOS 什么时候做？",
		},
		agentStepFunc: func(_ context.Context, task string, history []ai.ConversationMessage, tools []ai.AgentToolDefinition, results []ai.AgentToolResult) ai.AgentStepDecision {
			if task != "macOS 什么时候做？" {
				t.Fatalf("unexpected agent task: %q", task)
			}
			if len(history) != 0 {
				t.Fatalf("expected empty agent history, got %#v", history)
			}
			return ai.AgentStepDecision{
				Action: "answer",
				Answer: "这是 agent 默认模式下的回复。",
			}
		},
	}, reminders)

	reply, err := service.HandleMessage(context.Background(), MessageContext{}, "macOS 什么时候做？")
	if err != nil {
		t.Fatalf("handle message: %v", err)
	}
	if reply != "这是 agent 默认模式下的回复。" {
		t.Fatalf("unexpected agent reply: %q", reply)
	}
}

func TestHandleMessageAskModeUsesChat(t *testing.T) {
	t.Parallel()

	store := knowledge.NewStore(filepath.Join(t.TempDir(), "app.db"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(t.TempDir(), "app.db")))
	service := NewService(store, fakeAI{
		configured: true,
		route: ai.RouteDecision{
			Command:  "answer",
			Question: "macOS 什么时候做？",
		},
		answerFunc: func(string, []knowledge.Entry) string {
			t.Fatalf("knowledge answer should not be used in ask mode")
			return ""
		},
		chatFunc: func(_ context.Context, input string, history []ai.ConversationMessage) string {
			if input != "macOS 什么时候做？" {
				t.Fatalf("unexpected chat input: %q", input)
			}
			if len(history) != 0 {
				t.Fatalf("expected empty history, got %#v", history)
			}
			return "这是 ask 模式下的普通 AI 回复。"
		},
	}, reminders)
	if _, err := service.SetMode(context.Background(), MessageContext{}, ModeAsk); err != nil {
		t.Fatalf("set mode: %v", err)
	}

	reply, err := service.HandleMessage(context.Background(), MessageContext{}, "macOS 什么时候做？")
	if err != nil {
		t.Fatalf("handle message: %v", err)
	}
	if reply != "这是 ask 模式下的普通 AI 回复。" {
		t.Fatalf("unexpected ask reply: %q", reply)
	}
}

func TestHandleMessageAskModeBypassesRouteCommand(t *testing.T) {
	t.Parallel()

	store := knowledge.NewStore(filepath.Join(t.TempDir(), "app.db"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(t.TempDir(), "app.db")))
	service := NewService(store, fakeAI{
		configured: true,
		routeFunc: func(_ context.Context, input string) ai.RouteDecision {
			t.Fatalf("route command should not be called in ask mode, input=%q", input)
			return ai.RouteDecision{}
		},
		chatFunc: func(_ context.Context, input string, history []ai.ConversationMessage) string {
			if input != "你好" {
				t.Fatalf("unexpected chat input: %q", input)
			}
			if len(history) != 0 {
				t.Fatalf("expected empty history, got %#v", history)
			}
			return "普通聊天"
		},
	}, reminders)
	if _, err := service.SetMode(context.Background(), MessageContext{}, ModeAsk); err != nil {
		t.Fatalf("set mode: %v", err)
	}

	reply, err := service.HandleMessage(context.Background(), MessageContext{}, "你好")
	if err != nil {
		t.Fatalf("handle message: %v", err)
	}
	if reply != "普通聊天" {
		t.Fatalf("unexpected ask reply: %q", reply)
	}
}

func TestHandleMessageAskModeSkipsAutomaticFileSearchPlanning(t *testing.T) {
	t.Parallel()

	store := knowledge.NewStore(filepath.Join(t.TempDir(), "app.db"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(t.TempDir(), "app.db")))
	service := NewService(store, fakeAI{
		configured: true,
		toolOpportunityFunc: func(_ context.Context, task string, tools []ai.ToolCapability) []ai.ToolOpportunity {
			t.Fatalf("tool opportunity detection should not run in ask mode, task=%q tools=%#v", task, tools)
			return nil
		},
		chatFunc: func(_ context.Context, input string, history []ai.ConversationMessage) string {
			if input != "查找D盘的csv文件" {
				t.Fatalf("unexpected chat input: %q", input)
			}
			if len(history) != 0 {
				t.Fatalf("expected empty history, got %#v", history)
			}
			return "这是普通聊天，不自动触发文件检索。"
		},
	}, reminders)
	if _, err := service.SetMode(context.Background(), MessageContext{}, ModeAsk); err != nil {
		t.Fatalf("set mode: %v", err)
	}

	reply, err := service.HandleMessage(context.Background(), MessageContext{}, "查找D盘的csv文件")
	if err != nil {
		t.Fatalf("handle message: %v", err)
	}
	if reply != "这是普通聊天，不自动触发文件检索。" {
		t.Fatalf("unexpected ask reply: %q", reply)
	}
}

func TestHandleMessageScopesAnswerByProject(t *testing.T) {
	t.Parallel()

	store := knowledge.NewStore(filepath.Join(t.TempDir(), "app.db"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(t.TempDir(), "app.db")))

	if _, err := store.Add(knowledge.WithProject(context.Background(), "Alpha"), knowledge.Entry{
		ID:         "11111111aaaa1111",
		Text:       "Alpha 项目的发布计划是先做桌面端。",
		RecordedAt: time.Date(2026, 3, 27, 10, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("seed alpha entry: %v", err)
	}
	if _, err := store.Add(knowledge.WithProject(context.Background(), "Beta"), knowledge.Entry{
		ID:         "22222222bbbb2222",
		Text:       "Beta 项目的发布计划是先做接口层。",
		RecordedAt: time.Date(2026, 3, 27, 11, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("seed beta entry: %v", err)
	}

	service := NewService(store, fakeAI{
		configured: true,
		route: ai.RouteDecision{
			Command:  "answer",
			Question: "项目发布计划是什么？",
		},
		searchPlan: ai.SearchPlan{
			Queries:  []string{"项目发布计划"},
			Keywords: []string{"项目", "计划"},
		},
		answerFunc: func(_ string, entries []knowledge.Entry) string {
			if len(entries) != 1 {
				t.Fatalf("expected 1 scoped entry, got %#v", entries)
			}
			if entries[0].Project != "Alpha" {
				t.Fatalf("expected alpha project entry, got %#v", entries[0])
			}
			return entries[0].Text
		},
	}, reminders)
	reply, err := service.HandleMessage(context.Background(), MessageContext{Project: "Alpha"}, "@kb 项目发布计划是什么？")
	if err != nil {
		t.Fatalf("answer failed: %v", err)
	}
	if !strings.Contains(reply, "Alpha 项目的发布计划") {
		t.Fatalf("unexpected scoped reply: %q", reply)
	}
}

func TestDebugSearchShowsKeywordsCandidatesAndReviewedSelection(t *testing.T) {
	t.Parallel()

	store := knowledge.NewStore(filepath.Join(t.TempDir(), "app.db"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(t.TempDir(), "app.db")))

	macEntry, err := store.Add(context.Background(), knowledge.Entry{
		ID:         "11111111aaaa1111",
		Text:       "未来需要支持 macOS。",
		RecordedAt: time.Date(2026, 3, 27, 10, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("seed mac entry: %v", err)
	}
	if _, err := store.Add(context.Background(), knowledge.Entry{
		ID:         "22222222bbbb2222",
		Text:       "微信接口先做。",
		RecordedAt: time.Date(2026, 3, 27, 11, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("seed weixin entry: %v", err)
	}

	service := NewService(store, fakeAI{
		configured: true,
		searchPlan: ai.SearchPlan{
			Queries:  []string{"macOS 支持计划", "macOS 什么时候做"},
			Keywords: []string{"macos", "支持"},
		},
		reviewIDs: []string{macEntry.ID},
	}, reminders)

	reply, err := service.HandleMessage(context.Background(), MessageContext{}, "/debug-search macOS什么时候做")
	if err != nil {
		t.Fatalf("debug search: %v", err)
	}
	for _, expected := range []string{"检索调试", "检索问句：macOS 支持计划, macOS 什么时候做", "AI关键词：macos, 支持", "score=", "[review] #11111111"} {
		if !strings.Contains(reply, expected) {
			t.Fatalf("expected %q in reply: %q", expected, reply)
		}
	}
}

func TestHandleMessageUsesAIIntentRecognition(t *testing.T) {
	t.Parallel()

	store := knowledge.NewStore(filepath.Join(t.TempDir(), "app.db"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(t.TempDir(), "app.db")))
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

func TestModeOverrideUsesKnowledgeWithoutPersisting(t *testing.T) {
	t.Parallel()

	store := knowledge.NewStore(filepath.Join(t.TempDir(), "app.db"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(t.TempDir(), "app.db")))
	if _, err := store.Add(context.Background(), knowledge.Entry{
		ID:         "11111111aaaa1111",
		Text:       "未来需要支持 macOS。",
		RecordedAt: time.Date(2026, 3, 27, 10, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("seed entry: %v", err)
	}

	service := NewService(store, fakeAI{
		configured: true,
		route: ai.RouteDecision{
			Command:  "answer",
			Question: "macOS 什么时候做？",
		},
		searchPlan: ai.SearchPlan{
			Queries:  []string{"macOS 支持计划"},
			Keywords: []string{"macos", "支持"},
		},
		answerFunc: func(_ string, entries []knowledge.Entry) string {
			if len(entries) != 1 {
				t.Fatalf("expected 1 knowledge entry, got %#v", entries)
			}
			return "这是 @kb 临时切到知识库后的回复。"
		},
		chatFunc: func(_ context.Context, _ string, history []ai.ConversationMessage) string {
			if len(history) != 0 {
				t.Fatalf("expected empty history, got %#v", history)
			}
			return "这是 ask 模式下的普通回复。"
		},
	}, reminders)

	reply, err := service.HandleMessage(context.Background(), MessageContext{}, "@kb macOS 什么时候做？")
	if err != nil {
		t.Fatalf("knowledge override: %v", err)
	}
	if reply != "这是 @kb 临时切到知识库后的回复。" {
		t.Fatalf("unexpected knowledge override reply: %q", reply)
	}

	reply, err = service.HandleMessage(context.Background(), MessageContext{}, "macOS 什么时候做？")
	if err != nil {
		t.Fatalf("ask follow-up: %v", err)
	}
	if reply != "这是 ask 模式下的普通回复。" {
		t.Fatalf("unexpected ask follow-up reply: %q", reply)
	}
}

func TestAppendCommandUpdatesExistingKnowledge(t *testing.T) {
	t.Parallel()

	store := knowledge.NewStore(filepath.Join(t.TempDir(), "app.db"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(t.TempDir(), "app.db")))
	service := NewService(store, nil, reminders)

	if _, err := store.Add(context.Background(), knowledge.Entry{
		ID:         "6d2d7724abcd1234",
		Text:       "Puppeteer 是一个浏览器自动化工具。",
		RecordedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed entry: %v", err)
	}

	reply, err := service.HandleMessage(context.Background(), MessageContext{}, "/kb append 6d2d7724 它是 Google 出品的一个工具。")
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

	store := knowledge.NewStore(filepath.Join(t.TempDir(), "app.db"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(t.TempDir(), "app.db")))
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

	store := knowledge.NewStore(filepath.Join(t.TempDir(), "app.db"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(t.TempDir(), "app.db")))
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

func TestRememberFileCommandStoresImageSummary(t *testing.T) {
	t.Parallel()

	store := knowledge.NewStore(filepath.Join(t.TempDir(), "app.db"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(t.TempDir(), "app.db")))
	service := NewService(store, fakeAI{
		configured:   true,
		imageSummary: "- 这是一张 Puppeteer 截图",
	}, reminders)

	imagePath := filepath.Join(t.TempDir(), "puppeteer.png")
	imageData, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO6p3xkAAAAASUVORK5CYII=")
	if err != nil {
		t.Fatalf("decode image fixture: %v", err)
	}
	if err := os.WriteFile(imagePath, imageData, 0o644); err != nil {
		t.Fatalf("write image: %v", err)
	}

	reply, err := service.HandleMessage(context.Background(), MessageContext{
		UserID:    "terminal",
		Interface: "terminal",
	}, "/kb remember-file "+imagePath)
	if err != nil {
		t.Fatalf("remember-file: %v", err)
	}
	if !strings.Contains(reply, "已记住") {
		t.Fatalf("unexpected reply: %q", reply)
	}

	entries, err := store.List(context.Background())
	if err != nil {
		t.Fatalf("list entries: %v", err)
	}
	if len(entries) != 1 || !strings.Contains(entries[0].Text, "Puppeteer 截图") {
		t.Fatalf("unexpected entries: %#v", entries)
	}
}

func TestDirectPDFPathStoresSummary(t *testing.T) {
	store := knowledge.NewStore(filepath.Join(t.TempDir(), "app.db"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(t.TempDir(), "app.db")))
	service := NewService(store, fakeAI{
		configured: true,
		pdfSummary: "- 这份 PDF 讲了 Puppeteer 基础用法",
	}, reminders)

	originalExtractPDFText := extractPDFText
	extractPDFText = func(string) (string, error) {
		return "Puppeteer PDF full text", nil
	}
	t.Cleanup(func() {
		extractPDFText = originalExtractPDFText
	})

	pdfPath := filepath.Join(t.TempDir(), "puppeteer.pdf")
	if err := os.WriteFile(pdfPath, []byte("%PDF-1.4 dummy"), 0o644); err != nil {
		t.Fatalf("write pdf: %v", err)
	}

	reply, err := service.HandleMessage(context.Background(), MessageContext{
		UserID:    "terminal",
		Interface: "terminal",
	}, pdfPath)
	if err != nil {
		t.Fatalf("direct path ingest: %v", err)
	}
	if !strings.Contains(reply, "已记住") {
		t.Fatalf("unexpected reply: %q", reply)
	}

	entries, err := store.List(context.Background())
	if err != nil {
		t.Fatalf("list entries: %v", err)
	}
	if len(entries) != 1 || !strings.Contains(entries[0].Text, "Puppeteer 基础用法") {
		t.Fatalf("unexpected entries: %#v", entries)
	}
}

func TestNaturalRememberMessageDoesNotTripDirectFileDetect(t *testing.T) {
	t.Parallel()

	store := knowledge.NewStore(filepath.Join(t.TempDir(), "app.db"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(t.TempDir(), "app.db")))
	service := NewService(store, nil, reminders)

	input := "记住 brwap的安装方法：git clone https://github.com/containers/bubblewrap.git\ncd bubblewrap\nmeson _builddir\nmeson compile -C _builddir\nmeson test -C _builddir\nmeson install -C _builddir"
	reply, err := service.HandleMessage(context.Background(), MessageContext{
		UserID:    "u1",
		Interface: "weixin",
	}, input)
	if err != nil {
		t.Fatalf("handle remember message: %v", err)
	}
	if !strings.Contains(reply, "已记住") {
		t.Fatalf("unexpected reply: %q", reply)
	}

	entries, err := store.List(context.Background())
	if err != nil {
		t.Fatalf("list entries: %v", err)
	}
	if len(entries) != 1 || !strings.Contains(entries[0].Text, "git clone https://github.com/containers/bubblewrap.git") {
		t.Fatalf("unexpected entries: %#v", entries)
	}
}

func TestRememberFileReturnsFriendlyMessageWhenPDFUnavailable(t *testing.T) {
	store := knowledge.NewStore(filepath.Join(t.TempDir(), "app.db"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(t.TempDir(), "app.db")))
	service := NewService(store, fakeAI{configured: true}, reminders)

	originalExtractPDFText := extractPDFText
	extractPDFText = func(string) (string, error) {
		return "", errors.Join(fileingest.ErrPDFExtractorUnavailable, errors.New("no cgo in this build"))
	}
	t.Cleanup(func() {
		extractPDFText = originalExtractPDFText
	})

	pdfPath := filepath.Join(t.TempDir(), "puppeteer.pdf")
	if err := os.WriteFile(pdfPath, []byte("%PDF-1.4 dummy"), 0o644); err != nil {
		t.Fatalf("write pdf: %v", err)
	}

	reply, err := service.HandleMessage(context.Background(), MessageContext{
		UserID:    "terminal",
		Interface: "terminal",
	}, "/kb remember-file "+pdfPath)
	if err != nil {
		t.Fatalf("remember-file unavailable pdf: %v", err)
	}
	if !strings.Contains(reply, "当前这个构建不包含 PDF 文本提取能力") {
		t.Fatalf("unexpected reply: %q", reply)
	}

	entries, err := store.List(context.Background())
	if err != nil {
		t.Fatalf("list entries: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected no stored entry, got %#v", entries)
	}
}

func TestHandleMessageRequiresConfiguredModelForNaturalLanguage(t *testing.T) {
	t.Parallel()

	store := knowledge.NewStore(filepath.Join(t.TempDir(), "app.db"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(t.TempDir(), "app.db")))
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

	store := knowledge.NewStore(filepath.Join(t.TempDir(), "app.db"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(t.TempDir(), "app.db")))
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

	store := knowledge.NewStore(filepath.Join(t.TempDir(), "app.db"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(t.TempDir(), "app.db")))
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

	store := knowledge.NewStore(filepath.Join(t.TempDir(), "app.db"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(t.TempDir(), "app.db")))
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

	store := knowledge.NewStore(filepath.Join(t.TempDir(), "app.db"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(t.TempDir(), "app.db")))
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

func TestDesktopPrimaryNoticeListAggregatesReminderSources(t *testing.T) {
	t.Parallel()

	store := knowledge.NewStore(filepath.Join(t.TempDir(), "app.db"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(t.TempDir(), "app.db")))
	service := NewService(store, nil, reminders)

	if _, err := reminders.ScheduleAt(context.Background(), reminder.Target{
		Interface: "weixin",
		UserID:    "user-a",
	}, time.Date(2099, 4, 1, 11, 29, 16, 0, time.Local), "做下行程管理"); err != nil {
		t.Fatalf("seed weixin reminder: %v", err)
	}
	if _, err := reminders.ScheduleDaily(context.Background(), reminder.Target{
		Interface: "desktop",
		UserID:    "primary",
	}, 20, 0, "完成三代的全长数据处理"); err != nil {
		t.Fatalf("seed desktop reminder: %v", err)
	}

	listReply, err := service.HandleMessage(context.Background(), MessageContext{
		UserID:    "primary",
		Interface: "desktop",
	}, "/notice list")
	if err != nil {
		t.Fatalf("list reminders: %v", err)
	}
	if !strings.Contains(listReply, "做下行程管理") || !strings.Contains(listReply, "完成三代的全长数据处理") {
		t.Fatalf("expected aggregated reminders, got %q", listReply)
	}
	if !strings.Contains(listReply, "来源: 微信") || !strings.Contains(listReply, "来源: 桌面") {
		t.Fatalf("expected source labels in aggregated list, got %q", listReply)
	}
}

func TestDesktopPrimaryNoticeRemoveCanDeleteRemoteReminder(t *testing.T) {
	t.Parallel()

	store := knowledge.NewStore(filepath.Join(t.TempDir(), "app.db"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(t.TempDir(), "app.db")))
	service := NewService(store, nil, reminders)

	item, err := reminders.ScheduleDaily(context.Background(), reminder.Target{
		Interface: "weixin",
		UserID:    "user-a",
	}, 20, 0, "完成三代的全长数据处理")
	if err != nil {
		t.Fatalf("seed weixin reminder: %v", err)
	}

	removeReply, err := service.HandleMessage(context.Background(), MessageContext{
		UserID:    "primary",
		Interface: "desktop",
	}, "/notice remove "+item.ID[:8])
	if err != nil {
		t.Fatalf("remove reminder: %v", err)
	}
	if !strings.Contains(removeReply, "已删除提醒") {
		t.Fatalf("unexpected remove reply: %q", removeReply)
	}

	items, err := reminders.List(context.Background(), reminder.Target{Interface: "weixin", UserID: "user-a"})
	if err != nil {
		t.Fatalf("list weixin reminders: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected remote reminder to be removed, got %#v", items)
	}
}

func TestCronAliasCreatesDateReminder(t *testing.T) {
	t.Parallel()

	store := knowledge.NewStore(filepath.Join(t.TempDir(), "app.db"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(t.TempDir(), "app.db")))
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

	store := knowledge.NewStore(filepath.Join(t.TempDir(), "app.db"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(t.TempDir(), "app.db")))
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

	store := knowledge.NewStore(filepath.Join(t.TempDir(), "app.db"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(t.TempDir(), "app.db")))
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

	store := knowledge.NewStore(filepath.Join(t.TempDir(), "app.db"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(t.TempDir(), "app.db")))
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

func TestAgentModeCanUseKnowledgeSearchTool(t *testing.T) {
	t.Parallel()

	store := knowledge.NewStore(filepath.Join(t.TempDir(), "app.db"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(t.TempDir(), "app.db")))
	if _, err := store.Add(context.Background(), knowledge.Entry{
		ID:         "11111111aaaa1111",
		Text:       "未来需要支持 macOS。",
		RecordedAt: time.Date(2026, 3, 27, 10, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("seed entry: %v", err)
	}

	service := NewService(store, fakeAI{
		configured: true,
		route: ai.RouteDecision{
			Command:  "answer",
			Question: "我之前记过 macOS 计划吗？",
		},
		agentStepFunc: func(_ context.Context, task string, history []ai.ConversationMessage, tools []ai.AgentToolDefinition, results []ai.AgentToolResult) ai.AgentStepDecision {
			if task != "我之前记过 macOS 计划吗？" {
				t.Fatalf("unexpected task: %q", task)
			}
			if len(history) != 0 {
				t.Fatalf("expected empty history, got %#v", history)
			}
			var found bool
			for _, tool := range tools {
				if tool.Name == "local::knowledge_search" {
					found = true
					if tool.Provider != "local" || tool.ProviderKind != "local" {
						t.Fatalf("unexpected tool provider metadata: %#v", tool)
					}
				}
			}
			if !found {
				t.Fatalf("expected local knowledge_search tool, got %#v", tools)
			}
			if len(results) == 0 {
				return ai.AgentStepDecision{
					Action:    "tool",
					ToolName:  "local::knowledge_search",
					ToolInput: `{"query":"macOS 计划"}`,
				}
			}
			if len(results) != 1 || !strings.Contains(results[0].Output, "macOS") {
				t.Fatalf("unexpected tool results: %#v", results)
			}
			return ai.AgentStepDecision{
				Action: "answer",
				Answer: "你之前记过，知识里提到未来需要支持 macOS。",
			}
		},
	}, reminders)
	if _, err := service.SetMode(context.Background(), MessageContext{}, ModeAgent); err != nil {
		t.Fatalf("set mode: %v", err)
	}

	reply, err := service.HandleMessage(context.Background(), MessageContext{}, "我之前记过 macOS 计划吗？")
	if err != nil {
		t.Fatalf("handle message: %v", err)
	}
	if !strings.Contains(reply, "未来需要支持 macOS") {
		t.Fatalf("unexpected agent reply: %q", reply)
	}
}

func TestAgentToolProvidersExposeProtocolTools(t *testing.T) {
	t.Parallel()

	store := knowledge.NewStore(filepath.Join(t.TempDir(), "app.db"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(t.TempDir(), "app.db")))
	service := NewService(store, fakeAI{configured: true}, reminders)
	service.RegisterMCPToolProvider("docs", fakeProtocolToolClient{
		tools: []ProtocolToolSpec{{
			Name:             "lookup",
			Description:      "Search MCP docs.",
			InputJSONExample: `{"query":"tool calls"}`,
		}},
	})
	service.RegisterNCPToolProvider("desktop", fakeProtocolToolClient{
		tools: []ProtocolToolSpec{{
			Name:             "open_app",
			Description:      "Open an app on the local desktop.",
			InputJSONExample: `{"name":"WeChat"}`,
		}},
	})
	service.RegisterACPToolProvider("wechat", fakeProtocolToolClient{
		tools: []ProtocolToolSpec{{
			Name:             "send_message",
			Description:      "Send a WeChat message through ACP.",
			InputJSONExample: `{"to":"filehelper","text":"hello"}`,
		}},
	})

	definitions, err := service.ListAgentToolDefinitions(context.Background(), MessageContext{})
	if err != nil {
		t.Fatalf("list agent tool definitions: %v", err)
	}

	expected := map[string]struct {
		provider string
		kind     string
	}{
		"local::knowledge_search":  {provider: "local", kind: "local"},
		"mcp.docs::lookup":         {provider: "mcp.docs", kind: "mcp"},
		"ncp.desktop::open_app":    {provider: "ncp.desktop", kind: "ncp"},
		"acp.wechat::send_message": {provider: "acp.wechat", kind: "acp"},
	}

	for name, want := range expected {
		var found *ai.AgentToolDefinition
		for index := range definitions {
			if definitions[index].Name == name {
				found = &definitions[index]
				break
			}
		}
		if found == nil {
			t.Fatalf("missing tool definition %q in %#v", name, definitions)
		}
		if found.Provider != want.provider || found.ProviderKind != want.kind {
			t.Fatalf("unexpected provider metadata for %q: %#v", name, *found)
		}
	}
}

func TestAgentToolDefinitionsCarryUnifiedContracts(t *testing.T) {
	t.Parallel()

	store := knowledge.NewStore(filepath.Join(t.TempDir(), "app.db"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(t.TempDir(), "app.db")))
	service := NewService(store, fakeAI{configured: true}, reminders)
	service.RegisterMCPToolProvider("docs", fakeProtocolToolClient{
		tools: []ProtocolToolSpec{{
			Name:              "lookup",
			Purpose:           "Search MCP docs.",
			Description:       "Search MCP docs.",
			InputContract:     `Provide {"query":"..."}.`,
			OutputContract:    "Returns matching documentation passages.",
			Usage:             "Use when external MCP documentation is needed.",
			InputJSONExample:  `{"query":"tool calls"}`,
			OutputJSONExample: `{"items":[{"title":"tool calls"}]}`,
		}},
	})

	definitions, err := service.ListAgentToolDefinitions(context.Background(), MessageContext{})
	if err != nil {
		t.Fatalf("list agent tool definitions: %v", err)
	}

	var fileSearchDef *ai.AgentToolDefinition
	var protocolDef *ai.AgentToolDefinition
	for index := range definitions {
		switch definitions[index].Name {
		case "local::everything_file_search":
			fileSearchDef = &definitions[index]
		case "mcp.docs::lookup":
			protocolDef = &definitions[index]
		}
	}
	if fileSearchDef == nil || protocolDef == nil {
		t.Fatalf("missing expected definitions in %#v", definitions)
	}
	if fileSearchDef.Usage == "" || fileSearchDef.InputContract == "" || fileSearchDef.OutputContract == "" {
		t.Fatalf("expected unified contract fields on local tool, got %#v", *fileSearchDef)
	}
	if protocolDef.Usage == "" || protocolDef.InputContract == "" || protocolDef.OutputContract == "" || protocolDef.OutputJSONExample == "" {
		t.Fatalf("expected unified contract fields on protocol tool, got %#v", *protocolDef)
	}
}

func TestAgentModeCanUseMCPToolProvider(t *testing.T) {
	t.Parallel()

	store := knowledge.NewStore(filepath.Join(t.TempDir(), "app.db"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(t.TempDir(), "app.db")))

	var executed bool
	service := NewService(store, fakeAI{
		configured: true,
		route: ai.RouteDecision{
			Command:  "answer",
			Question: "帮我查一下 MCP 文档里怎么描述 tool calls",
		},
		agentStepFunc: func(_ context.Context, task string, _ []ai.ConversationMessage, tools []ai.AgentToolDefinition, results []ai.AgentToolResult) ai.AgentStepDecision {
			if task != "帮我查一下 MCP 文档里怎么描述 tool calls" {
				t.Fatalf("unexpected task: %q", task)
			}
			var found bool
			for _, tool := range tools {
				if tool.Name == "mcp.docs::lookup" {
					found = true
				}
			}
			if !found {
				t.Fatalf("expected mcp tool, got %#v", tools)
			}
			if len(results) == 0 {
				return ai.AgentStepDecision{
					Action:    "tool",
					ToolName:  "mcp.docs::lookup",
					ToolInput: `{"query":"tool calls"}`,
				}
			}
			if len(results) != 1 || !strings.Contains(results[0].Output, "MCP tool calls let the model") {
				t.Fatalf("unexpected tool results: %#v", results)
			}
			return ai.AgentStepDecision{
				Action: "answer",
				Answer: "MCP 文档提到 tool calls 让模型通过协议调用外部能力。",
			}
		},
	}, reminders)
	service.RegisterMCPToolProvider("docs", fakeProtocolToolClient{
		tools: []ProtocolToolSpec{{
			Name:             "lookup",
			Description:      "Search MCP docs.",
			InputJSONExample: `{"query":"tool calls"}`,
		}},
		execute: func(_ context.Context, _ MessageContext, toolName, rawInput string) (string, error) {
			executed = true
			if toolName != "lookup" {
				t.Fatalf("unexpected tool name: %q", toolName)
			}
			if rawInput != `{"query":"tool calls"}` {
				t.Fatalf("unexpected tool input: %q", rawInput)
			}
			return "MCP tool calls let the model invoke external capabilities through the protocol.", nil
		},
	})
	if _, err := service.SetMode(context.Background(), MessageContext{}, ModeAgent); err != nil {
		t.Fatalf("set mode: %v", err)
	}

	reply, err := service.HandleMessage(context.Background(), MessageContext{}, "帮我查一下 MCP 文档里怎么描述 tool calls")
	if err != nil {
		t.Fatalf("handle message: %v", err)
	}
	if !executed {
		t.Fatalf("expected mcp provider to execute")
	}
	if !strings.Contains(reply, "tool calls") {
		t.Fatalf("unexpected agent reply: %q", reply)
	}
}

func TestModePersistsAcrossServiceRestarts(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store := knowledge.NewStore(filepath.Join(root, "app.db"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(root, "app.db")))
	stateStore := sessionstate.NewStore(filepath.Join(root, "app.db"))
	service := NewServiceWithSkillsAndSessions(store, fakeAI{configured: true}, reminders, nil, stateStore)
	mc := MessageContext{Interface: "terminal", UserID: "u1", SessionID: "s1", Project: "alpha"}

	if _, err := service.SetMode(context.Background(), mc, ModeAsk); err != nil {
		t.Fatalf("set mode: %v", err)
	}

	restarted := NewServiceWithSkillsAndSessions(store, fakeAI{configured: true}, reminders, nil, stateStore)
	mode, err := restarted.GetMode(context.Background(), mc)
	if err != nil {
		t.Fatalf("get mode: %v", err)
	}
	if mode != ModeAsk {
		t.Fatalf("expected persisted ask mode, got %q", mode)
	}
}

func TestSetModePreservesConversationHistory(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store := knowledge.NewStore(filepath.Join(root, "app.db"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(root, "app.db")))
	stateStore := sessionstate.NewStore(filepath.Join(root, "app.db"))
	mc := MessageContext{Interface: "desktop", UserID: "u1", SessionID: "s1", Project: "alpha"}

	service := NewServiceWithSkillsAndSessions(store, fakeAI{
		configured: true,
		route: ai.RouteDecision{
			Command:  "answer",
			Question: "第一轮",
		},
		chatFunc: func(_ context.Context, input string, history []ai.ConversationMessage) string {
			if input != "第一轮" {
				t.Fatalf("unexpected chat input: %q", input)
			}
			if len(history) != 0 {
				t.Fatalf("expected empty first history, got %#v", history)
			}
			return "第一轮回复"
		},
	}, reminders, nil, stateStore)

	if _, err := service.HandleMessage(context.Background(), mc, "第一轮"); err != nil {
		t.Fatalf("handle first message: %v", err)
	}
	if _, err := service.SetMode(context.Background(), mc, ModeAsk); err != nil {
		t.Fatalf("set mode: %v", err)
	}

	snapshot, ok, err := stateStore.Load(context.Background(), conversationSessionKey(mc))
	if err != nil {
		t.Fatalf("load session snapshot: %v", err)
	}
	if !ok {
		t.Fatal("expected persisted session snapshot")
	}
	if snapshot.Mode != string(ModeAsk) {
		t.Fatalf("expected persisted mode %q, got %#v", ModeAsk, snapshot)
	}
	if len(snapshot.History) != 2 {
		t.Fatalf("expected history to be preserved, got %#v", snapshot.History)
	}
	if snapshot.History[0].Role != "user" || snapshot.History[0].Content != "第一轮" {
		t.Fatalf("unexpected preserved user message: %#v", snapshot.History[0])
	}
	if snapshot.History[1].Role != "assistant" || snapshot.History[1].Content != "第一轮回复" {
		t.Fatalf("unexpected preserved assistant message: %#v", snapshot.History[1])
	}
}

func TestEnsureConversationPersistsDefaultMode(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store := knowledge.NewStore(filepath.Join(root, "app.db"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(root, "app.db")))
	stateStore := sessionstate.NewStore(filepath.Join(root, "app.db"))
	service := NewServiceWithSkillsAndSessions(store, nil, reminders, nil, stateStore)
	mc := MessageContext{Interface: "weixin", UserID: "u1", SessionID: "session-1"}

	if err := service.EnsureConversation(context.Background(), mc); err != nil {
		t.Fatalf("ensure conversation: %v", err)
	}

	snapshot, ok, err := stateStore.Load(context.Background(), conversationSessionKey(mc))
	if err != nil {
		t.Fatalf("load session snapshot: %v", err)
	}
	if !ok {
		t.Fatal("expected persisted session snapshot")
	}
	if snapshot.Mode != string(ModeAgent) {
		t.Fatalf("expected persisted mode %q, got %#v", ModeAgent, snapshot)
	}
}

func TestGetModeBackfillsDefaultModeForLegacySnapshot(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store := knowledge.NewStore(filepath.Join(root, "app.db"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(root, "app.db")))
	stateStore := sessionstate.NewStore(filepath.Join(root, "app.db"))
	service := NewServiceWithSkillsAndSessions(store, nil, reminders, nil, stateStore)
	mc := MessageContext{Interface: "weixin", UserID: "u1", SessionID: "legacy-session"}
	key := conversationSessionKey(mc)

	if _, err := stateStore.Save(context.Background(), sessionstate.Snapshot{Key: key}); err != nil {
		t.Fatalf("save legacy session snapshot: %v", err)
	}

	mode, err := service.GetMode(context.Background(), mc)
	if err != nil {
		t.Fatalf("get mode: %v", err)
	}
	if mode != ModeAgent {
		t.Fatalf("expected mode %q, got %q", ModeAgent, mode)
	}

	snapshot, ok, err := stateStore.Load(context.Background(), key)
	if err != nil {
		t.Fatalf("reload session snapshot: %v", err)
	}
	if !ok {
		t.Fatal("expected persisted legacy snapshot")
	}
	if snapshot.Mode != string(ModeAgent) {
		t.Fatalf("expected backfilled mode %q, got %#v", ModeAgent, snapshot)
	}
}

func TestHandleMessageTreatsSpuriousHelpRouteAsAnswer(t *testing.T) {
	t.Parallel()

	for _, mode := range []Mode{ModeAsk, ModeAgent} {
		t.Run(string(mode), func(t *testing.T) {
			store := knowledge.NewStore(filepath.Join(t.TempDir(), "app.db"))
			reminders := reminder.NewManager(reminder.NewStore(filepath.Join(t.TempDir(), "reminder.db")))
			service := NewService(store, fakeAI{
				configured: true,
				route: ai.RouteDecision{
					Command: "help",
				},
				chatFunc: func(_ context.Context, input string, history []ai.ConversationMessage) string {
					if input != "你好" {
						t.Fatalf("unexpected chat input: %q", input)
					}
					if len(history) != 0 {
						t.Fatalf("expected empty history, got %#v", history)
					}
					return "正常对话"
				},
			}, reminders)
			mc := MessageContext{Interface: "desktop", SessionID: "s1"}
			if _, err := service.SetMode(context.Background(), mc, mode); err != nil {
				t.Fatalf("set mode: %v", err)
			}

			reply, err := service.HandleMessage(context.Background(), mc, "你好")
			if err != nil {
				t.Fatalf("handle message: %v", err)
			}
			if reply != "正常对话" {
				t.Fatalf("expected normal conversation reply, got %q", reply)
			}
		})
	}
}

func TestHandleMessageAgentModePreservesExplicitHelpRoute(t *testing.T) {
	t.Parallel()

	store := knowledge.NewStore(filepath.Join(t.TempDir(), "app.db"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(t.TempDir(), "reminder.db")))
	service := NewService(store, fakeAI{
		configured: true,
		route: ai.RouteDecision{
			Command: "help",
		},
		chatFunc: func(_ context.Context, input string, history []ai.ConversationMessage) string {
			t.Fatalf("chat should not be called for explicit help, input=%q history=%#v", input, history)
			return ""
		},
	}, reminders)
	mc := MessageContext{Interface: "desktop", SessionID: "s1"}
	if _, err := service.SetMode(context.Background(), mc, ModeAgent); err != nil {
		t.Fatalf("set mode: %v", err)
	}

	reply, err := service.HandleMessage(context.Background(), mc, "怎么用")
	if err != nil {
		t.Fatalf("handle help message: %v", err)
	}
	if !strings.Contains(reply, "可用命令") {
		t.Fatalf("expected help reply, got %q", reply)
	}
}

func TestLoadedSkillsPersistAcrossServiceRestarts(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	skillDir := filepath.Join(root, "skills", "writer")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir skill dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(strings.TrimSpace(`
---
name: writer
description: 帮助输出更清晰的中文写作
---
# Writer
Use concise Chinese writing.
`)), 0o644); err != nil {
		t.Fatalf("write skill file: %v", err)
	}

	store := knowledge.NewStore(filepath.Join(root, "app.db"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(root, "app.db")))
	stateStore := sessionstate.NewStore(filepath.Join(root, "app.db"))
	loader := skilllib.NewLoader(filepath.Join(root, "skills"))
	mc := MessageContext{Interface: "terminal", UserID: "u1", SessionID: "s1"}

	service := NewServiceWithSkillsAndSessions(store, fakeAI{configured: true}, reminders, loader, stateStore)
	if _, err := service.LoadSkillForSession(mc, "writer"); err != nil {
		t.Fatalf("load skill: %v", err)
	}

	restarted := NewServiceWithSkillsAndSessions(store, fakeAI{configured: true}, reminders, loader, stateStore)
	loaded := restarted.ListLoadedSkills(mc)
	if len(loaded) != 1 || loaded[0].Name != "writer" {
		t.Fatalf("expected persisted writer skill, got %#v", loaded)
	}
}

func TestPromptProfilePersistsAndInjectsIntoConversation(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store := knowledge.NewStore(filepath.Join(root, "app.db"))
	promptStore := promptlib.NewStore(filepath.Join(root, "app.db"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(root, "app.db")))
	stateStore := sessionstate.NewStore(filepath.Join(root, "app.db"))
	profile, err := promptStore.Add(context.Background(), promptlib.Prompt{
		Title:      "Architect",
		Content:    "Always answer with architecture-first tradeoff analysis.",
		RecordedAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("add prompt: %v", err)
	}

	mc := MessageContext{Interface: "terminal", UserID: "u1", SessionID: "s1"}
	service := NewServiceWithRuntime(store, fakeAI{configured: true}, reminders, nil, stateStore, promptStore)
	if _, err := service.SetPromptProfile(context.Background(), mc, profile.ID[:8]); err != nil {
		t.Fatalf("set prompt profile: %v", err)
	}

	restarted := NewServiceWithRuntime(store, fakeAI{
		configured: true,
		route: ai.RouteDecision{
			Command:  "answer",
			Question: "帮我分析一下这个架构",
		},
		chatFunc: func(ctx context.Context, _ string, history []ai.ConversationMessage) string {
			if len(history) != 0 {
				t.Fatalf("expected empty history, got %#v", history)
			}
			instructions := ai.SkillContextFromContext(ctx)
			if !strings.Contains(instructions, "Architect") || !strings.Contains(instructions, "architecture-first tradeoff analysis") {
				t.Fatalf("prompt profile missing from conversation context: %q", instructions)
			}
			return "这是带 prompt profile 的回复。"
		},
	}, reminders, nil, stateStore, promptStore)

	reply, err := restarted.HandleMessage(context.Background(), mc, "帮我分析一下这个架构")
	if err != nil {
		t.Fatalf("handle message: %v", err)
	}
	if reply != "这是带 prompt profile 的回复。" {
		t.Fatalf("unexpected prompt profile reply: %q", reply)
	}
}

func TestDirectModeConversationHistoryPersistsAcrossRestarts(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store := knowledge.NewStore(filepath.Join(root, "app.db"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(root, "app.db")))
	stateStore := sessionstate.NewStore(filepath.Join(root, "app.db"))
	mc := MessageContext{Interface: "terminal", UserID: "u1", SessionID: "s1"}

	service := NewServiceWithSkillsAndSessions(store, fakeAI{
		configured: true,
		route: ai.RouteDecision{
			Command:  "answer",
			Question: "我叫小张",
		},
		chatFunc: func(_ context.Context, input string, history []ai.ConversationMessage) string {
			if input != "我叫小张" {
				t.Fatalf("unexpected first chat input: %q", input)
			}
			if len(history) != 0 {
				t.Fatalf("expected empty first history, got %#v", history)
			}
			return "记住了，你叫小张。"
		},
	}, reminders, nil, stateStore)

	if _, err := service.HandleMessage(context.Background(), mc, "我叫小张"); err != nil {
		t.Fatalf("first message: %v", err)
	}

	restarted := NewServiceWithSkillsAndSessions(store, fakeAI{
		configured: true,
		route: ai.RouteDecision{
			Command:  "answer",
			Question: "我刚才叫什么名字？",
		},
		chatFunc: func(_ context.Context, input string, history []ai.ConversationMessage) string {
			if input != "我刚才叫什么名字？" {
				t.Fatalf("unexpected second chat input: %q", input)
			}
			if len(history) != 2 {
				t.Fatalf("expected persisted 2-message history, got %#v", history)
			}
			if history[0].Role != "user" || history[0].Content != "我叫小张" {
				t.Fatalf("unexpected first history item: %#v", history[0])
			}
			if history[1].Role != "assistant" || history[1].Content != "记住了，你叫小张。" {
				t.Fatalf("unexpected second history item: %#v", history[1])
			}
			return "你刚才说你叫小张。"
		},
	}, reminders, nil, stateStore)

	reply, err := restarted.HandleMessage(context.Background(), mc, "我刚才叫什么名字？")
	if err != nil {
		t.Fatalf("second message: %v", err)
	}
	if reply != "你刚才说你叫小张。" {
		t.Fatalf("unexpected second reply: %q", reply)
	}
}

func TestDesktopPrimaryDirectChatIncludesReminderRuntimeSummary(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store := knowledge.NewStore(filepath.Join(root, "app.db"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(root, "app.db")))
	stateStore := sessionstate.NewStore(filepath.Join(root, "app.db"))
	mc := MessageContext{Interface: "desktop", UserID: "primary", SessionID: "desktop-1"}

	if _, err := reminders.ScheduleAt(context.Background(), reminder.Target{
		Interface: "weixin",
		UserID:    "user-a",
	}, time.Date(2099, 4, 1, 11, 29, 16, 0, time.Local), "做下行程管理"); err != nil {
		t.Fatalf("seed weixin reminder: %v", err)
	}

	service := NewServiceWithSkillsAndSessions(store, fakeAI{
		configured: true,
		route: ai.RouteDecision{
			Command:  "answer",
			Question: "我今天要做什么事情？",
		},
		chatFunc: func(_ context.Context, input string, history []ai.ConversationMessage) string {
			if input != "我今天要做什么事情？" {
				t.Fatalf("unexpected chat input: %q", input)
			}
			if len(history) != 1 {
				t.Fatalf("expected runtime reminder summary history, got %#v", history)
			}
			if history[0].Role != "assistant" ||
				!strings.Contains(history[0].Content, "系统同步的当前提醒摘要") ||
				!strings.Contains(history[0].Content, "做下行程管理") ||
				!strings.Contains(history[0].Content, "来源: 微信") {
				t.Fatalf("unexpected reminder summary: %#v", history)
			}
			return "你今天要做下行程管理。"
		},
	}, reminders, nil, stateStore)

	reply, err := service.HandleMessage(context.Background(), mc, "@ai 我今天要做什么事情？")
	if err != nil {
		t.Fatalf("handle message: %v", err)
	}
	if reply != "你今天要做下行程管理。" {
		t.Fatalf("unexpected reply: %q", reply)
	}
}

func TestWeixinConversationHistoryUsesEnvLimits(t *testing.T) {
	t.Setenv(envWeixinHistoryMessages, "4")
	t.Setenv(envWeixinHistoryRunes, "5")

	root := t.TempDir()
	store := knowledge.NewStore(filepath.Join(root, "app.db"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(root, "app.db")))
	stateStore := sessionstate.NewStore(filepath.Join(root, "app.db"))
	service := NewServiceWithSkillsAndSessions(store, nil, reminders, nil, stateStore)
	mc := MessageContext{Interface: "weixin", UserID: "u1", SessionID: "s1"}

	for index := 0; index < 3; index++ {
		service.appendConversationHistory(context.Background(), mc,
			fmt.Sprintf("user%d-abcdef", index),
			fmt.Sprintf("assistant%d-uvwxyz", index),
		)
	}

	snapshot, ok, err := stateStore.Load(context.Background(), conversationSessionKey(mc))
	if err != nil {
		t.Fatalf("load session snapshot: %v", err)
	}
	if !ok {
		t.Fatal("expected saved session snapshot")
	}
	if len(snapshot.History) != 4 {
		t.Fatalf("expected 4 saved messages, got %#v", snapshot.History)
	}

	expected := []string{
		trimConversationHistoryText("user1-abcdef", 5),
		trimConversationHistoryText("assistant1-uvwxyz", 5),
		trimConversationHistoryText("user2-abcdef", 5),
		trimConversationHistoryText("assistant2-uvwxyz", 5),
	}
	for index, item := range snapshot.History {
		if item.Content != expected[index] {
			t.Fatalf("unexpected saved message %d: %#v", index, snapshot.History)
		}
	}

	history := service.conversationHistory(context.Background(), mc)
	if len(history) != 4 {
		t.Fatalf("expected 4 trimmed history messages, got %#v", history)
	}
}

func TestDesktopConversationHistoryIgnoresWeixinEnvLimits(t *testing.T) {
	t.Setenv(envWeixinHistoryMessages, "2")
	t.Setenv(envWeixinHistoryRunes, "5")

	root := t.TempDir()
	store := knowledge.NewStore(filepath.Join(root, "app.db"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(root, "app.db")))
	stateStore := sessionstate.NewStore(filepath.Join(root, "app.db"))
	service := NewServiceWithSkillsAndSessions(store, nil, reminders, nil, stateStore)
	mc := MessageContext{Interface: "desktop", UserID: "u1", SessionID: "s1"}

	for index := 0; index < 3; index++ {
		service.appendConversationHistory(context.Background(), mc,
			fmt.Sprintf("desktop-user-%d-abcdef", index),
			fmt.Sprintf("desktop-assistant-%d-uvwxyz", index),
		)
	}

	snapshot, ok, err := stateStore.Load(context.Background(), conversationSessionKey(mc))
	if err != nil {
		t.Fatalf("load session snapshot: %v", err)
	}
	if !ok {
		t.Fatal("expected saved session snapshot")
	}
	if len(snapshot.History) != 6 {
		t.Fatalf("expected full desktop history, got %#v", snapshot.History)
	}
	if snapshot.History[0].Content != "desktop-user-0-abcdef" {
		t.Fatalf("expected desktop content to remain untrimmed, got %#v", snapshot.History)
	}
	if snapshot.History[5].Content != "desktop-assistant-2-uvwxyz" {
		t.Fatalf("expected desktop content to remain untrimmed, got %#v", snapshot.History)
	}

	history := service.conversationHistory(context.Background(), mc)
	if len(history) != 6 {
		t.Fatalf("expected full desktop conversation history, got %#v", history)
	}
}

func TestDesktopConversationHistoryUsesGenericEnvLimits(t *testing.T) {
	t.Setenv(envConversationHistoryMessages, "4")
	t.Setenv(envConversationHistoryRunes, "5")

	root := t.TempDir()
	store := knowledge.NewStore(filepath.Join(root, "app.db"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(root, "app.db")))
	stateStore := sessionstate.NewStore(filepath.Join(root, "app.db"))
	service := NewServiceWithSkillsAndSessions(store, nil, reminders, nil, stateStore)
	mc := MessageContext{Interface: "desktop", UserID: "u1", SessionID: "s1"}

	for index := 0; index < 3; index++ {
		service.appendConversationHistory(context.Background(), mc,
			fmt.Sprintf("desktop-user-%d-abcdef", index),
			fmt.Sprintf("desktop-assistant-%d-uvwxyz", index),
		)
	}

	snapshot, ok, err := stateStore.Load(context.Background(), conversationSessionKey(mc))
	if err != nil {
		t.Fatalf("load session snapshot: %v", err)
	}
	if !ok {
		t.Fatal("expected saved session snapshot")
	}
	if len(snapshot.History) != 4 {
		t.Fatalf("expected trimmed desktop history, got %#v", snapshot.History)
	}

	expected := []string{
		trimConversationHistoryText("desktop-user-1-abcdef", 5),
		trimConversationHistoryText("desktop-assistant-1-uvwxyz", 5),
		trimConversationHistoryText("desktop-user-2-abcdef", 5),
		trimConversationHistoryText("desktop-assistant-2-uvwxyz", 5),
	}
	for index, item := range snapshot.History {
		if item.Content != expected[index] {
			t.Fatalf("unexpected saved desktop message %d: %#v", index, snapshot.History)
		}
	}
}

func TestConversationHistoryUsesAssistantContextSummary(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store := knowledge.NewStore(filepath.Join(root, "app.db"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(root, "app.db")))
	stateStore := sessionstate.NewStore(filepath.Join(root, "app.db"))
	service := NewServiceWithSkillsAndSessions(store, nil, reminders, nil, stateStore)
	mc := MessageContext{Interface: "desktop", UserID: "u1", SessionID: "s1"}

	ctx := withTaskContext(context.Background())
	setTurnSummary(ctx, "摘要结论")
	service.appendConversationHistory(ctx, mc, "帮我查文件", "这里是很长的原始结果，包含很多不应该进入下一轮上下文的细节。")

	snapshot, ok, err := stateStore.Load(context.Background(), conversationSessionKey(mc))
	if err != nil {
		t.Fatalf("load session snapshot: %v", err)
	}
	if !ok || len(snapshot.History) != 2 {
		t.Fatalf("expected saved history, got %#v", snapshot)
	}
	if snapshot.History[1].ContextSummary != "摘要结论" {
		t.Fatalf("expected persisted context summary, got %#v", snapshot.History[1])
	}

	history := service.conversationHistory(context.Background(), mc)
	if len(history) != 2 {
		t.Fatalf("expected 2 conversation history items, got %#v", history)
	}
	if history[1].Content != "摘要结论" {
		t.Fatalf("expected assistant context summary to be used, got %#v", history)
	}
}

func TestHandleMessageStreamUsesStreamingChat(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store := knowledge.NewStore(filepath.Join(root, "app.db"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(root, "app.db")))
	stateStore := sessionstate.NewStore(filepath.Join(root, "app.db"))
	mc := MessageContext{Interface: "desktop", UserID: "u1", SessionID: "s1"}

	service := NewServiceWithSkillsAndSessions(store, fakeStreamingAI{
		fakeAI: fakeAI{
			configured: true,
			route: ai.RouteDecision{
				Command:  "answer",
				Question: "给我结果",
			},
		},
		chatStreamFunc: func(_ context.Context, input string, history []ai.ConversationMessage, onDelta func(string)) string {
			if input != "给我结果" {
				t.Fatalf("unexpected chat input: %q", input)
			}
			if len(history) != 0 {
				t.Fatalf("expected empty history, got %#v", history)
			}
			onDelta("分")
			onDelta("段")
			return "分段"
		},
	}, reminders, nil, stateStore)

	var chunks []string
	reply, err := service.HandleMessageStream(context.Background(), mc, "@ai 给我结果", func(delta string) {
		chunks = append(chunks, delta)
	})
	if err != nil {
		t.Fatalf("handle message stream: %v", err)
	}
	if reply != "分段" {
		t.Fatalf("unexpected reply: %q", reply)
	}
	if strings.Join(chunks, "") != "分段" {
		t.Fatalf("unexpected chunks: %#v", chunks)
	}

	snapshot, ok, err := stateStore.Load(context.Background(), conversationSessionKey(mc))
	if err != nil {
		t.Fatalf("load session snapshot: %v", err)
	}
	if !ok || len(snapshot.History) != 2 {
		t.Fatalf("unexpected history snapshot: %#v / ok=%v", snapshot, ok)
	}
	if snapshot.History[1].Content != "分段" {
		t.Fatalf("expected streamed reply in history, got %#v", snapshot.History)
	}
}

func TestHandleMessageStreamChunksAgentReply(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store := knowledge.NewStore(filepath.Join(root, "app.db"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(root, "app.db")))
	stateStore := sessionstate.NewStore(filepath.Join(root, "app.db"))
	mc := MessageContext{Interface: "desktop", UserID: "u1", SessionID: "s1"}
	wantReply := "这是一个用于验证 desktop agent 最终回答分块流式输出的较长结果，应该通过多个 delta 返回给前端。"

	service := NewServiceWithSkillsAndSessions(store, fakeAI{
		configured: true,
		route: ai.RouteDecision{
			Command:  "answer",
			Question: "给我一个长回答",
		},
		chat: wantReply,
	}, reminders, nil, stateStore)

	var chunks []string
	reply, err := service.HandleMessageStream(context.Background(), mc, "@agent 给我一个长回答", func(delta string) {
		chunks = append(chunks, delta)
	})
	if err != nil {
		t.Fatalf("handle message stream: %v", err)
	}
	if reply != wantReply {
		t.Fatalf("unexpected reply: %q", reply)
	}
	if strings.Join(chunks, "") != wantReply {
		t.Fatalf("unexpected chunks: %#v", chunks)
	}
	if len(chunks) < 2 {
		t.Fatalf("expected multiple agent chunks, got %#v", chunks)
	}

	snapshot, ok, err := stateStore.Load(context.Background(), conversationSessionKey(mc))
	if err != nil {
		t.Fatalf("load session snapshot: %v", err)
	}
	if !ok || len(snapshot.History) != 2 {
		t.Fatalf("unexpected history snapshot: %#v / ok=%v", snapshot, ok)
	}
	if snapshot.History[1].Content != wantReply {
		t.Fatalf("expected persisted streamed agent reply, got %#v", snapshot.History)
	}
}

func TestResolveFileSearchIgnoresNaturalLanguageInput(t *testing.T) {
	t.Parallel()

	store := knowledge.NewStore(filepath.Join(t.TempDir(), "app.db"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(t.TempDir(), "app.db")))
	service := NewService(store, fakeAI{configured: true}, reminders)

	result, reply, handled, err := service.ResolveFileSearch(context.Background(), MessageContext{
		UserID:    "u1",
		Interface: "weixin",
		SessionID: "s1",
	}, "查找 D 盘单细胞相关的PDF文件")
	if err != nil {
		t.Fatalf("resolve file search: %v", err)
	}
	if handled {
		t.Fatalf("expected natural language input to bypass explicit file search, got result=%#v reply=%q", result, reply)
	}
	if reply != "" || strings.TrimSpace(result.Query) != "" || len(result.Items) != 0 {
		t.Fatalf("unexpected explicit file search output: %#v / %q", result, reply)
	}
}

func TestHandleMessageNaturalFileSearchFollowsAgentConversationFlow(t *testing.T) {
	t.Parallel()

	store := knowledge.NewStore(filepath.Join(t.TempDir(), "app.db"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(t.TempDir(), "app.db")))
	service := NewService(store, fakeAI{
		configured: true,
		route:      ai.RouteDecision{Command: "answer"},
		agentStep: ai.AgentStepDecision{
			Action: "answer",
			Answer: "普通聊天",
		},
		toolOpportunityFunc: func(_ context.Context, _ string, _ []ai.ToolCapability) []ai.ToolOpportunity {
			t.Fatal("legacy file search detector should not run for natural language input")
			return nil
		},
	}, reminders)
	service.SetFileSearchExecutor(func(_ context.Context, _ string, _ filesearch.ToolInput) (filesearch.ToolResult, error) {
		t.Fatal("file search executor should not run outside the agent loop")
		return filesearch.ToolResult{}, nil
	})

	reply, err := service.HandleMessage(context.Background(), MessageContext{Interface: "desktop", SessionID: "desktop-1"}, "查找D盘的csv文件")
	if err != nil {
		t.Fatalf("handle natural file search through agent flow: %v", err)
	}
	if reply != "普通聊天" {
		t.Fatalf("unexpected reply: %q", reply)
	}
}

func TestHandleMessageSlashFindExecutesToolOutsideWeixin(t *testing.T) {
	t.Parallel()

	store := knowledge.NewStore(filepath.Join(t.TempDir(), "app.db"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(t.TempDir(), "app.db")))
	service := NewService(store, nil, reminders)
	service.SetFileSearchEverythingPath("es.exe")
	service.SetFileSearchExecutor(func(_ context.Context, everythingPath string, input filesearch.ToolInput) (filesearch.ToolResult, error) {
		if everythingPath != "es.exe" {
			t.Fatalf("unexpected everything path: %q", everythingPath)
		}
		if input.Query != "output.csv" {
			t.Fatalf("unexpected query: %#v", input)
		}
		return filesearch.ToolResult{
			Tool:  filesearch.ToolName,
			Query: input.Query,
			Items: []filesearch.ResultItem{
				{Index: 1, Name: "output.csv", Path: `D:\exports\output.csv`},
			},
		}, nil
	})

	reply, err := service.HandleMessage(context.Background(), MessageContext{Interface: "desktop", SessionID: "desktop-1"}, "/find output.csv")
	if err != nil {
		t.Fatalf("handle slash find: %v", err)
	}
	if !strings.Contains(reply, "找到 1 个文件") || !strings.Contains(reply, `D:\exports\output.csv`) {
		t.Fatalf("unexpected reply: %q", reply)
	}
}

func TestHandleMessageSlashFindReturnsDisabledReplyWhenToolDisabled(t *testing.T) {
	t.Parallel()

	store := knowledge.NewStore(filepath.Join(t.TempDir(), "app.db"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(t.TempDir(), "app.db")))
	service := NewService(store, nil, reminders)
	service.SetDisabledAgentTools([]string{"local::everything_file_search"})
	service.SetFileSearchExecutor(func(_ context.Context, _ string, _ filesearch.ToolInput) (filesearch.ToolResult, error) {
		t.Fatal("file search executor should not run when tool is disabled")
		return filesearch.ToolResult{}, nil
	})

	reply, err := service.HandleMessage(context.Background(), MessageContext{Interface: "desktop", SessionID: "desktop-1"}, "/find output.csv")
	if err != nil {
		t.Fatalf("handle slash find with disabled tool: %v", err)
	}
	if reply != fileSearchDisabledReply {
		t.Fatalf("unexpected reply: %q", reply)
	}
}

func TestHandleMessageNaturalFileSearchSkipsDisabledTool(t *testing.T) {
	t.Parallel()

	store := knowledge.NewStore(filepath.Join(t.TempDir(), "app.db"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(t.TempDir(), "app.db")))
	service := NewService(store, fakeAI{
		configured: true,
		route:      ai.RouteDecision{Command: "answer"},
		agentStep: ai.AgentStepDecision{
			Action: "answer",
			Answer: "普通聊天",
		},
		toolOpportunityFunc: func(_ context.Context, _ string, _ []ai.ToolCapability) []ai.ToolOpportunity {
			t.Fatal("natural file search detector should not run when tool is disabled")
			return nil
		},
	}, reminders)
	service.SetDisabledAgentTools([]string{"local::everything_file_search"})
	service.SetFileSearchExecutor(func(_ context.Context, _ string, _ filesearch.ToolInput) (filesearch.ToolResult, error) {
		t.Fatal("file search executor should not run when tool is disabled")
		return filesearch.ToolResult{}, nil
	})

	reply, err := service.HandleMessage(context.Background(), MessageContext{Interface: "desktop", SessionID: "desktop-1"}, "查找D盘的csv文件")
	if err != nil {
		t.Fatalf("handle natural file search with disabled tool: %v", err)
	}
	if reply != "普通聊天" {
		t.Fatalf("unexpected reply: %q", reply)
	}
}

type fakeAI struct {
	configured          bool
	route               ai.RouteDecision
	routeFunc           func(context.Context, string) ai.RouteDecision
	searchPlan          ai.SearchPlan
	toolOpportunities   []ai.ToolOpportunity
	toolOpportunityFunc func(context.Context, string, []ai.ToolCapability) []ai.ToolOpportunity
	toolPlanDecision    ai.ToolPlanDecision
	toolPlanFunc        func(context.Context, string, ai.ToolCapability, []ai.ToolExecution) ai.ToolPlanDecision
	reviewIDs           []string
	answer              string
	answerFunc          func(string, []knowledge.Entry) string
	chat                string
	chatFunc            func(context.Context, string, []ai.ConversationMessage) string
	agentStep           ai.AgentStepDecision
	agentStepFunc       func(context.Context, string, []ai.ConversationMessage, []ai.AgentToolDefinition, []ai.AgentToolResult) ai.AgentStepDecision
	translation         string
	translationFunc     func(context.Context, string) string
	pdfSummary          string
	imageSummary        string
}

func (f fakeAI) IsConfigured(context.Context) (bool, error) {
	return f.configured, nil
}

func (f fakeAI) RouteCommand(ctx context.Context, input string) (ai.RouteDecision, error) {
	if f.routeFunc != nil {
		return f.routeFunc(ctx, input), nil
	}
	return f.route, nil
}

func (f fakeAI) BuildSearchPlan(context.Context, string) (ai.SearchPlan, error) {
	return f.searchPlan, nil
}

func (f fakeAI) DetectToolOpportunities(ctx context.Context, task string, tools []ai.ToolCapability) ([]ai.ToolOpportunity, error) {
	if f.toolOpportunityFunc != nil {
		return f.toolOpportunityFunc(ctx, task, tools), nil
	}
	return f.toolOpportunities, nil
}

func (f fakeAI) PlanToolUse(ctx context.Context, task string, tool ai.ToolCapability, prior []ai.ToolExecution) (ai.ToolPlanDecision, error) {
	if f.toolPlanFunc != nil {
		return f.toolPlanFunc(ctx, task, tool, prior), nil
	}
	return f.toolPlanDecision, nil
}

func (f fakeAI) ReviewAnswerCandidates(context.Context, string, []knowledge.Entry) ([]string, error) {
	return f.reviewIDs, nil
}

func (f fakeAI) Answer(_ context.Context, question string, entries []knowledge.Entry) (string, error) {
	if f.answerFunc != nil {
		return f.answerFunc(question, entries), nil
	}
	return f.answer, nil
}

func (f fakeAI) Chat(ctx context.Context, input string, history []ai.ConversationMessage) (string, error) {
	if f.chatFunc != nil {
		return f.chatFunc(ctx, input, history), nil
	}
	return f.chat, nil
}

func (f fakeAI) DecideAgentStep(ctx context.Context, task string, history []ai.ConversationMessage, tools []ai.AgentToolDefinition, results []ai.AgentToolResult) (ai.AgentStepDecision, error) {
	if f.agentStepFunc != nil {
		return f.agentStepFunc(ctx, task, history, tools, results), nil
	}
	return f.agentStep, nil
}

func (f fakeAI) PlanNext(ctx context.Context, task string, history []ai.ConversationMessage, tools []ai.AgentToolDefinition, state ai.AgentTaskState) (ai.LoopDecision, error) {
	results := make([]ai.AgentToolResult, 0, len(state.ToolAttempts))
	for _, a := range state.ToolAttempts {
		results = append(results, ai.AgentToolResult{ToolName: a.ToolName, ToolInput: a.ToolInput, Output: a.RawOutput})
	}
	step, err := f.DecideAgentStep(ctx, task, history, tools, results)
	if err != nil {
		return ai.LoopDecision{}, err
	}
	if strings.TrimSpace(step.Action) == "" {
		if len(results) == 0 {
			decision, err := f.PlanToolUse(ctx, task, ai.ToolCapability{Name: filesearch.ToolName}, nil)
			if err == nil && strings.EqualFold(strings.TrimSpace(decision.Action), "tool") {
				toolName := strings.TrimSpace(decision.ToolName)
				if toolName != "" && !strings.Contains(toolName, "::") {
					toolName = "local::" + toolName
				}
				return ai.LoopDecision{Action: ai.LoopContinue, ToolName: toolName, ToolInput: decision.ToolInput}, nil
			}
		}
		if len(results) > 0 {
			return ai.LoopDecision{Action: ai.LoopAnswer, Answer: strings.TrimSpace(results[len(results)-1].Output)}, nil
		}
		reply, err := f.Chat(ctx, task, history)
		if err != nil {
			return ai.LoopDecision{}, err
		}
		return ai.LoopDecision{Action: ai.LoopAnswer, Answer: reply}, nil
	}
	switch step.Action {
	case "answer":
		return ai.LoopDecision{Action: ai.LoopAnswer, Answer: step.Answer}, nil
	case "tool":
		return ai.LoopDecision{Action: ai.LoopContinue, ToolName: step.ToolName, ToolInput: step.ToolInput}, nil
	default:
		return ai.LoopDecision{}, fmt.Errorf("unsupported agent action %q", step.Action)
	}
}

func (f fakeAI) SummarizeWorkingState(_ context.Context, _ ai.AgentTaskState) (string, error) {
	return "", nil
}

func (f fakeAI) SummarizeFinal(_ context.Context, _ ai.AgentTaskState, finalAnswer string) (string, error) {
	return finalAnswer, nil
}

func (f fakeAI) TranslateToChinese(ctx context.Context, input string) (string, error) {
	if f.translationFunc != nil {
		return f.translationFunc(ctx, input), nil
	}
	return f.translation, nil
}

func (f fakeAI) SummarizePDFText(context.Context, string, string) (string, error) {
	return f.pdfSummary, nil
}

func (f fakeAI) SummarizeImageFile(context.Context, string, string) (string, error) {
	return f.imageSummary, nil
}

type fakeStreamingAI struct {
	fakeAI
	chatStreamFunc   func(context.Context, string, []ai.ConversationMessage, func(string)) string
	answerStreamFunc func(context.Context, string, []knowledge.Entry, func(string)) string
}

func (f fakeStreamingAI) ChatStream(ctx context.Context, input string, history []ai.ConversationMessage, onDelta func(string)) (string, error) {
	if f.chatStreamFunc != nil {
		return f.chatStreamFunc(ctx, input, history, onDelta), nil
	}
	return f.fakeAI.Chat(ctx, input, history)
}

func (f fakeStreamingAI) AnswerStream(ctx context.Context, question string, entries []knowledge.Entry, onDelta func(string)) (string, error) {
	if f.answerStreamFunc != nil {
		return f.answerStreamFunc(ctx, question, entries, onDelta), nil
	}
	return f.fakeAI.Answer(ctx, question, entries)
}

type fakeProtocolToolClient struct {
	tools   []ProtocolToolSpec
	execute func(context.Context, MessageContext, string, string) (string, error)
}

func (f fakeProtocolToolClient) ListProtocolTools(context.Context, MessageContext) ([]ProtocolToolSpec, error) {
	return append([]ProtocolToolSpec(nil), f.tools...), nil
}

func (f fakeProtocolToolClient) ExecuteProtocolTool(ctx context.Context, mc MessageContext, toolName, rawInput string) (string, error) {
	if f.execute != nil {
		return f.execute(ctx, mc, toolName, rawInput)
	}
	return "", nil
}
