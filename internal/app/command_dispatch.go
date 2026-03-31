package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"myclaw/internal/filesearch"
	"myclaw/internal/knowledge"
)

type commandHandler func(*Service, context.Context, MessageContext, string, []string) (string, error)

var serviceCommandHandlers = map[string]commandHandler{
	"/help":          handleHelpCommand,
	"/new":           handleNewConversationCommand,
	"/remember":      handleRememberCommand,
	"/remember-file": handleRememberFileCommand,
	"/find":          handleFindCommand,
	"/send":          handleSendCommand,
	"/append":        handleAppendCommand,
	"/translate":     handleTranslateCommand,
	"/debug-search":  handleDebugSearchCommand,
	"/mode":          handleModeCommand,
	"/skills":        handleSkillsCommand,
	"/show-skill":    handleShowSkillCommand,
	"/load-skill":    handleLoadSkillCommand,
	"/unload-skill":  handleUnloadSkillCommand,
	"/page-skills":   handlePageSkillsCommand,
	"/prompt":        handlePromptCommandDispatch,
	"/forget":        handleForgetCommand,
	"/list":          handleListCommand,
	"/stats":         handleStatsCommand,
	"/clear":         handleClearCommand,
	"/notice":        handleNoticeCommand,
}

const helpCommandText = "可用命令:\n" +
	"/new — 开启新对话（terminal / desktop）\n" +
	"/remember <内容> 或 记住：<内容> — 保存一条知识\n" +
	"/remember-file <路径> — 总结图片/PDF并存入知识库\n" +
	"/find <关键词> — 搜索本地文件\n" +
	"/send <序号> — 微信里发送上一轮 /find 结果中的文件\n" +
	"/append <ID前缀> <内容> — 追加到已有知识\n" +
	"/skills — 查看技能库和当前会话已加载技能\n" +
	"/show-skill <技能名> — 查看某个技能内容\n" +
	"/load-skill <技能名> — 手动为当前会话加载一个技能\n" +
	"/unload-skill <技能名> — 从当前会话卸载一个技能\n" +
	"/page-skills — 查看当前会话已加载技能\n" +
	"/prompt — 查看当前 Prompt profile\n" +
	"/prompt list — 查看可用 Prompt profiles\n" +
	"/prompt use <PromptID前缀> — 为当前会话启用 Prompt profile\n" +
	"/prompt clear — 清除当前会话 Prompt profile\n" +
	"/translate <内容> — 翻译成中文\n" +
	"/debug-search <问题> — 查看关键词检索和候选复核过程\n" +
	"/mode [direct|knowledge|agent] — 查看或切换普通对话模式\n" +
	"/forget <ID前缀> — 删除一条知识\n" +
	"/list — 查看全部知识\n" +
	"/stats — 查看知识库状态\n" +
	"/notice — 创建、查看、删除提醒\n" +
	"/cron — 与 /notice 等价\n" +
	"/clear — 清空知识库\n" +
	"/help — 查看帮助\n\n" +
	"普通问题默认走 direct 模式；可以用 `/mode knowledge` 切到知识库检索，或在单条消息前加 `@kb` 临时覆盖。"

func (s *Service) handleCommand(ctx context.Context, mc MessageContext, input string) (string, error) {
	input = CanonicalizeCommandInput(input)
	fields := strings.Fields(input)
	if len(fields) == 0 {
		return "", nil
	}

	handler, ok := serviceCommandHandlers[strings.ToLower(fields[0])]
	if !ok {
		return s.handleConversationMessage(ctx, mc, input)
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
		return "用法: /remember <内容>", nil
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
		return "用法: /remember-file <图片或PDF路径>", nil
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
		return "用法: /append <知识ID前缀> <补充内容>", nil
	}
	body := strings.TrimSpace(strings.TrimPrefix(input, fields[0]))
	bodyFields := strings.Fields(body)
	if len(bodyFields) < 2 {
		return "用法: /append <知识ID前缀> <补充内容>", nil
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

func handleModeCommand(s *Service, ctx context.Context, mc MessageContext, _ string, fields []string) (string, error) {
	if len(fields) == 1 {
		mode, err := s.GetMode(ctx, mc)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("当前模式: %s\n%s\n\n%s", mode, modeDescription(mode), modeUsage()), nil
	}
	if len(fields) != 2 {
		return modeUsage(), nil
	}
	mode := normalizeMode(fields[1])
	if mode == "" {
		return modeUsage(), nil
	}
	if _, err := s.SetMode(ctx, mc, mode); err != nil {
		return "", err
	}
	return fmt.Sprintf("已切换到 %s 模式。\n%s", mode, modeDescription(mode)), nil
}

func handleSkillsCommand(s *Service, _ context.Context, mc MessageContext, _ string, _ []string) (string, error) {
	return s.listSkills(mc)
}

func handleShowSkillCommand(s *Service, _ context.Context, _ MessageContext, _ string, fields []string) (string, error) {
	if len(fields) < 2 {
		return "用法: /show-skill <技能名>", nil
	}
	return s.showSkill(fields[1])
}

func handleLoadSkillCommand(s *Service, _ context.Context, mc MessageContext, _ string, fields []string) (string, error) {
	if len(fields) < 2 {
		return "用法: /load-skill <技能名>", nil
	}
	return s.loadSkill(mc, fields[1])
}

func handleUnloadSkillCommand(s *Service, _ context.Context, mc MessageContext, _ string, fields []string) (string, error) {
	if len(fields) < 2 {
		return "用法: /unload-skill <技能名>", nil
	}
	return s.unloadSkill(mc, fields[1])
}

func handlePageSkillsCommand(s *Service, _ context.Context, mc MessageContext, _ string, _ []string) (string, error) {
	return s.listLoadedSkills(mc), nil
}

func handlePromptCommandDispatch(s *Service, ctx context.Context, mc MessageContext, input string, _ []string) (string, error) {
	return s.handlePromptCommand(ctx, mc, input)
}

func handleForgetCommand(s *Service, ctx context.Context, _ MessageContext, _ string, fields []string) (string, error) {
	if len(fields) < 2 {
		return "用法: /forget <知识ID前缀>", nil
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
