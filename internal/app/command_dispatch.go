package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"baize/internal/filesearch"
	"baize/internal/knowledge"
)

type commandHandler func(*Service, context.Context, MessageContext, string, []string) (string, error)

var serviceCommandHandlers = map[string]commandHandler{
	"/help":         handleHelpCommand,
	"/new":          handleNewConversationCommand,
	"/find":         handleFindCommand,
	"/send":         handleSendCommand,
	"/translate":    handleTranslateCommand,
	"/debug-search": handleDebugSearchCommand,
	"/skill":        handleSkillCommandDispatch,
	"/prompt":       handlePromptCommandDispatch,
	"/kb":           handleKnowledgeCommandDispatch,
	"/notice":       handleNoticeCommand,
}

const helpCommandText = "可用命令:\n" +
	"/new — 开启新对话（terminal / desktop）\n" +
	"/find <关键词> — 搜索本地文件\n" +
	"/send <序号> — 微信里发送上一轮 /find 结果中的文件\n" +
	"/skill — 查看当前会话已加载技能\n" +
	"/skill list — 查看可用技能和加载状态\n" +
	"/skill show <技能名> — 查看某个技能内容\n" +
	"/skill load <技能名> — 手动为当前会话加载一个技能\n" +
	"/skill unload <技能名> — 从当前会话卸载一个技能\n" +
	"/skill clear — 清空当前会话已加载技能\n" +
	"/prompt — 查看当前 Prompt profile\n" +
	"/prompt list — 查看可用 Prompt profiles\n" +
	"/prompt use <PromptID前缀> — 为当前会话启用 Prompt profile\n" +
	"/prompt clear — 清除当前会话 Prompt profile\n" +
	"/kb remember <内容> 或 记住：<内容> — 保存一条知识\n" +
	"/kb remember-file <路径> — 总结图片/PDF并存入知识库\n" +
	"/kb append <ID前缀> <内容> — 追加到已有知识\n" +
	"/kb forget <ID前缀> — 删除一条知识\n" +
	"/kb new <名称> — 新建并切换到一个知识库\n" +
	"/kb switch <名称> — 切换当前知识库\n" +
	"/kb — 查看当前知识库和可用知识库\n" +
	"/kb list — 查看全部知识\n" +
	"/kb stats — 查看知识库状态\n" +
	"/kb clear — 清空知识库\n" +
	"/translate <内容> — 翻译成中文\n" +
	"/debug-search <问题> — 查看关键词检索和候选复核过程\n" +
	"/notice — 创建、查看、删除提醒\n" +
	"/cron — 与 /notice 等价\n" +
	"/help — 查看帮助\n\n" +
	"普通问题默认走 agent 模式；可以在单条消息前加 `@ai`、`@kb` 或 `@agent` 临时覆盖当前执行方式。"

func (s *Service) handleCommand(ctx context.Context, mc MessageContext, input string) (string, error) {
	input = CanonicalizeCommandInput(input)
	fields := strings.Fields(input)
	if len(fields) == 0 {
		return "", nil
	}

	handler, ok := serviceCommandHandlers[strings.ToLower(fields[0])]
	if !ok {
		return s.handleConversationMessageStream(ctx, mc, input, nil)
	}
	addProcessTrace(ctx, "命令分发", "command="+strings.ToLower(fields[0]))
	return handler(s, ctx, mc, input, fields)
}

func handleHelpCommand(_ *Service, _ context.Context, _ MessageContext, _ string, _ []string) (string, error) {
	return helpCommandText, nil
}

func handleNewConversationCommand(_ *Service, _ context.Context, _ MessageContext, _ string, _ []string) (string, error) {
	return "当前客户端会在收到 `/new` 时切换到一个新的会话。", nil
}

func handleRememberCommand(s *Service, ctx context.Context, mc MessageContext, input string, fields []string) (string, error) {
	if len(fields) < 2 {
		return "用法: /kb remember <内容>", nil
	}
	body := strings.TrimSpace(strings.TrimPrefix(input, fields[0]))
	entry, err := s.store.Add(ctx, knowledge.Entry{
		Text:       body,
		Source:     sourceLabel(mc),
		RecordedAt: time.Now(),
	})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("已记住 #%s\n%s", shortID(entry.ID), preview(entry.Text, maxReplyPreviewRunes)), nil
}

func handleRememberFileCommand(s *Service, ctx context.Context, mc MessageContext, input string, fields []string) (string, error) {
	if len(fields) < 2 {
		return "用法: /kb remember-file <图片或PDF路径>", nil
	}
	body := strings.TrimSpace(strings.TrimPrefix(input, fields[0]))
	return s.ingestFilePath(ctx, mc, body)
}

func handleFindCommand(s *Service, ctx context.Context, mc MessageContext, input string, _ []string) (string, error) {
	reply, handled, err := s.tryHandleFileSearch(ctx, mc, input)
	if handled || err != nil {
		return reply, err
	}
	return filesearch.ShortcutUsageText(), nil
}

func handleSendCommand(_ *Service, _ context.Context, mc MessageContext, _ string, _ []string) (string, error) {
	if !strings.EqualFold(strings.TrimSpace(mc.Interface), "weixin") {
		return "当前只支持在微信中使用 /send。先使用 /find 查看候选文件。", nil
	}
	return "请先使用 `/find` 查找文件，再使用 `/send <序号>` 发送。", nil
}

func handleAppendCommand(s *Service, ctx context.Context, mc MessageContext, input string, fields []string) (string, error) {
	if len(fields) < 3 {
		return "用法: /kb append <知识ID前缀> <补充内容>", nil
	}
	body := strings.TrimSpace(strings.TrimPrefix(input, fields[0]))
	bodyFields := strings.Fields(body)
	if len(bodyFields) < 2 {
		return "用法: /kb append <知识ID前缀> <补充内容>", nil
	}
	target := bodyFields[0]
	appendText := strings.TrimSpace(strings.TrimPrefix(body, target))
	if strings.EqualFold(target, "last") || strings.EqualFold(target, "latest") {
		return s.appendLatestKnowledge(ctx, mc, appendText)
	}
	return s.appendKnowledge(ctx, target, appendText)
}

func handleTranslateCommand(s *Service, ctx context.Context, mc MessageContext, input string, fields []string) (string, error) {
	if len(fields) < 2 {
		return "用法: /translate <待翻译内容>", nil
	}
	body := strings.TrimSpace(strings.TrimPrefix(input, fields[0]))
	reply, err := s.ensureAIAvailable(ctx)
	if reply != "" || err != nil {
		return reply, err
	}
	return s.aiService.TranslateToChinese(s.withSkillContext(ctx, mc), body)
}

func handleDebugSearchCommand(s *Service, ctx context.Context, mc MessageContext, input string, fields []string) (string, error) {
	if len(fields) < 2 {
		return "用法: /debug-search <问题>", nil
	}
	body := strings.TrimSpace(strings.TrimPrefix(input, fields[0]))
	return s.debugSearch(s.withSkillContext(ctx, mc), body)
}

func handleSkillCommandDispatch(s *Service, _ context.Context, mc MessageContext, input string, fields []string) (string, error) {
	if len(fields) == 1 {
		return s.formatCurrentSkills(mc), nil
	}

	switch strings.ToLower(fields[1]) {
	case "current":
		return s.formatCurrentSkills(mc), nil
	case "list":
		return s.listSkills(mc)
	case "show":
		if len(fields) < 3 {
			return skillCommandUsage(), nil
		}
		return s.showSkill(fields[2])
	case "load":
		if len(fields) < 3 {
			return skillCommandUsage(), nil
		}
		return s.loadSkill(mc, fields[2])
	case "unload":
		if len(fields) < 3 {
			return skillCommandUsage(), nil
		}
		return s.unloadSkill(mc, fields[2])
	case "clear":
		return s.clearLoadedSkills(mc)
	default:
		return skillCommandUsage(), nil
	}
}

func handlePromptCommandDispatch(s *Service, ctx context.Context, mc MessageContext, input string, _ []string) (string, error) {
	return s.handlePromptCommand(ctx, mc, input)
}

func handleKnowledgeCommandDispatch(s *Service, ctx context.Context, mc MessageContext, input string, fields []string) (string, error) {
	if len(fields) == 1 {
		return s.currentKnowledgeBaseReply(ctx, mc)
	}

	switch strings.ToLower(fields[1]) {
	case "remember":
		rewritten, rewrittenFields := rewriteNamespacedCommand(input, fields, 2, "/remember")
		return handleRememberCommand(s, ctx, mc, rewritten, rewrittenFields)
	case "remember-file":
		rewritten, rewrittenFields := rewriteNamespacedCommand(input, fields, 2, "/remember-file")
		return handleRememberFileCommand(s, ctx, mc, rewritten, rewrittenFields)
	case "append":
		rewritten, rewrittenFields := rewriteNamespacedCommand(input, fields, 2, "/append")
		return handleAppendCommand(s, ctx, mc, rewritten, rewrittenFields)
	case "forget":
		rewritten, rewrittenFields := rewriteNamespacedCommand(input, fields, 2, "/forget")
		return handleForgetCommand(s, ctx, mc, rewritten, rewrittenFields)
	case "new":
		if len(fields) < 3 {
			return "用法: /kb new <知识库名称>", nil
		}
		name := strings.TrimSpace(stripLeadingFields(input, fields, 2))
		info, err := s.switchKnowledgeBase(ctx, mc, name)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("已新建并切换到知识库 %s。\n当前条数: %d", info.Name, info.KnowledgeCount), nil
	case "switch":
		if len(fields) < 3 {
			return "用法: /kb switch <知识库名称>", nil
		}
		name := strings.TrimSpace(stripLeadingFields(input, fields, 2))
		info, err := s.switchKnowledgeBase(ctx, mc, name)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("已切换到知识库 %s。\n当前条数: %d", info.Name, info.KnowledgeCount), nil
	case "list":
		return handleListCommand(s, ctx, mc, "/kb list", []string{"/kb", "list"})
	case "stats":
		return handleStatsCommand(s, ctx, mc, "/kb stats", []string{"/kb", "stats"})
	case "clear":
		return handleClearCommand(s, ctx, mc, "/kb clear", []string{"/kb", "clear"})
	default:
		return knowledgeCommandUsage(), nil
	}
}

func handleForgetCommand(s *Service, ctx context.Context, _ MessageContext, _ string, fields []string) (string, error) {
	if len(fields) < 2 {
		return "用法: /kb forget <知识ID前缀>", nil
	}
	return s.forgetKnowledge(ctx, fields[1])
}

func handleListCommand(s *Service, ctx context.Context, _ MessageContext, _ string, _ []string) (string, error) {
	entries, err := s.store.List(ctx)
	if err != nil {
		return "", err
	}
	return formatKnowledgeDump(entries, "")
}

func handleStatsCommand(s *Service, ctx context.Context, _ MessageContext, _ string, _ []string) (string, error) {
	entries, err := s.store.List(ctx)
	if err != nil {
		return "", err
	}
	if len(entries) == 0 {
		return "知识库为空。", nil
	}
	first := entries[0].RecordedAt.Local().Format("2006-01-02 15:04:05")
	last := entries[len(entries)-1].RecordedAt.Local().Format("2006-01-02 15:04:05")
	return fmt.Sprintf("知识条数: %d\n首条时间: %s\n最新时间: %s", len(entries), first, last), nil
}

func handleClearCommand(s *Service, ctx context.Context, _ MessageContext, _ string, _ []string) (string, error) {
	if err := s.store.Clear(ctx); err != nil {
		return "", err
	}
	return "知识库已清空。", nil
}

func handleNoticeCommand(s *Service, ctx context.Context, mc MessageContext, input string, _ []string) (string, error) {
	return s.handleReminderCommand(ctx, mc, input)
}

func rewriteNamespacedCommand(input string, fields []string, consumedFields int, command string) (string, []string) {
	body := stripLeadingFields(input, fields, consumedFields)
	rewritten := strings.TrimSpace(command)
	if body != "" {
		rewritten += " " + body
	}
	return rewritten, strings.Fields(rewritten)
}

func stripLeadingFields(input string, fields []string, count int) string {
	remaining := strings.TrimSpace(input)
	for i := 0; i < count && i < len(fields); i++ {
		remaining = strings.TrimSpace(strings.TrimPrefix(remaining, fields[i]))
	}
	return remaining
}

func skillCommandUsage() string {
	return "用法:\n" +
		"/skill\n" +
		"/skill current\n" +
		"/skill list\n" +
		"/skill show <技能名>\n" +
		"/skill load <技能名>\n" +
		"/skill unload <技能名>\n" +
		"/skill clear"
}

func knowledgeCommandUsage() string {
	return "用法:\n" +
		"/kb\n" +
		"/kb new <知识库名称>\n" +
		"/kb switch <知识库名称>\n" +
		"/kb remember <内容>\n" +
		"/kb remember-file <图片或PDF路径>\n" +
		"/kb append <知识ID前缀> <补充内容>\n" +
		"/kb forget <知识ID前缀>\n" +
		"/kb list\n" +
		"/kb stats\n" +
		"/kb clear"
}
