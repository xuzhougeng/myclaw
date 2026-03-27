package app

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"myclaw/internal/ai"
	"myclaw/internal/knowledge"
)

const maxReplyPreviewRunes = 240

var (
	forgetIntentPattern    = regexp.MustCompile(`^(?:请)?(?:帮我)?(?:遗忘|忘掉|删除|删掉)\s*#?([0-9a-fA-F]{4,})\s*$`)
	appendByIDPattern      = regexp.MustCompile(`^(?:请)?(?:给|把)\s*#?([0-9a-fA-F]{4,})\s*(?:这条|这一条)?(?:再)?(?:补充|追加|补记|加上)(?:一点|一下|一条|一句|笔记)?(?:\s*[:：]\s*|\s+)(.+)$`)
	appendByIDShortPattern = regexp.MustCompile(`^#?([0-9a-fA-F]{4,})\s*(?:追加|补充|补记)(?:\s*[:：]\s*|\s+)(.+)$`)
	appendLastPattern      = regexp.MustCompile(`^(?:再)?(?:补充|追加|补记)(?:一点|一下|一条|一句|笔记)?(?:\s*[:：]\s*|\s+)(.+)$`)
	appendLastRefPattern   = regexp.MustCompile(`^(?:请)?(?:给|把)(?:上一条|上条|刚才那条|刚刚那条|这条)\s*(?:再)?(?:补充|追加|补记|加上)(?:一点|一下|一条|一句|笔记)?(?:\s*[:：]\s*|\s+)(.+)$`)
)

type MessageContext struct {
	UserID    string
	Interface string
}

type Service struct {
	store     *knowledge.Store
	aiService aiBackend
	reminders reminderBackend
}

type aiBackend interface {
	IsConfigured(ctx context.Context) (bool, error)
	RouteCommand(ctx context.Context, input string) (ai.RouteDecision, error)
	Answer(ctx context.Context, question string, entries []knowledge.Entry) (string, error)
	TranslateToChinese(ctx context.Context, input string) (string, error)
}

func NewService(store *knowledge.Store, aiService aiBackend, reminders reminderBackend) *Service {
	return &Service{
		store:     store,
		aiService: aiService,
		reminders: reminders,
	}
}

func (s *Service) HandleMessage(ctx context.Context, mc MessageContext, input string) (string, error) {
	text := strings.TrimSpace(input)
	if text == "" {
		return "我没有收到有效内容。发送“记住：xxx”保存知识，或直接问问题。", nil
	}

	if normalized := normalizeSlash(text); strings.HasPrefix(normalized, "/") {
		return s.handleCommand(ctx, mc, normalized)
	}

	if reply, ok, err := s.tryHandleNaturalAppend(ctx, mc, text); ok || err != nil {
		return reply, err
	}

	if reply, ok, err := s.tryHandleNaturalReminder(ctx, mc, text); ok || err != nil {
		return reply, err
	}

	if reply, ok, err := s.tryHandleNaturalForget(ctx, text); ok || err != nil {
		return reply, err
	}

	if memoryText, ok := parseRememberIntent(text); ok {
		entry, err := s.store.Add(ctx, knowledge.Entry{
			Text:       memoryText,
			Source:     sourceLabel(mc),
			RecordedAt: time.Now(),
		})
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("已记住 #%s\n%s", shortID(entry.ID), preview(entry.Text, maxReplyPreviewRunes)), nil
	}

	return s.handleAIMessage(ctx, mc, text)
}

func (s *Service) handleCommand(ctx context.Context, mc MessageContext, input string) (string, error) {
	fields := strings.Fields(input)
	if len(fields) == 0 {
		return "", nil
	}

	switch strings.ToLower(fields[0]) {
	case "/help", "/h":
		return "可用命令:\n" +
			"/remember <内容> 或 记住：<内容> — 保存一条知识\n" +
			"/append <ID前缀> <内容> — 追加到已有知识\n" +
			"/translate <内容> — 翻译成中文\n" +
			"/forget <ID前缀> — 删除一条知识\n" +
			"/list — 查看全部知识\n" +
			"/stats — 查看知识库状态\n" +
			"/notice — 创建、查看、删除提醒\n" +
			"/cron — 与 /notice 等价\n" +
			"/clear — 清空知识库\n" +
			"/help — 查看帮助\n\n" +
			"当前版本不会做复杂推理；收到普通问题时，会读取全部知识后直接回复。", nil
	case "/remember", "/r":
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
	case "/append":
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
	case "/translate":
		if len(fields) < 2 {
			return "用法: /translate <待翻译内容>", nil
		}
		body := strings.TrimSpace(strings.TrimPrefix(input, fields[0]))
		reply, err := s.ensureAIAvailable(ctx)
		if reply != "" || err != nil {
			return reply, err
		}
		return s.aiService.TranslateToChinese(ctx, body)
	case "/forget", "/delete":
		if len(fields) < 2 {
			return "用法: /forget <知识ID前缀>", nil
		}
		return s.forgetKnowledge(ctx, fields[1])
	case "/list", "/ls":
		entries, err := s.store.List(ctx)
		if err != nil {
			return "", err
		}
		return formatKnowledgeDump(entries, "")
	case "/stats":
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
	case "/clear":
		if err := s.store.Clear(ctx); err != nil {
			return "", err
		}
		return "知识库已清空。", nil
	case "/notice", "/cron":
		return s.handleReminderCommand(ctx, mc, input)
	default:
		return s.handleAIMessage(ctx, mc, input)
	}
}

func (s *Service) tryHandleNaturalAppend(ctx context.Context, mc MessageContext, input string) (string, bool, error) {
	text := strings.TrimSpace(input)

	if matches := appendByIDPattern.FindStringSubmatch(text); len(matches) == 3 {
		reply, err := s.appendKnowledge(ctx, matches[1], matches[2])
		return reply, true, err
	}
	if matches := appendByIDShortPattern.FindStringSubmatch(text); len(matches) == 3 {
		reply, err := s.appendKnowledge(ctx, matches[1], matches[2])
		return reply, true, err
	}
	if matches := appendLastRefPattern.FindStringSubmatch(text); len(matches) == 2 {
		reply, err := s.appendLatestKnowledge(ctx, mc, matches[1])
		return reply, true, err
	}
	if matches := appendLastPattern.FindStringSubmatch(text); len(matches) == 2 {
		reply, err := s.appendLatestKnowledge(ctx, mc, matches[1])
		return reply, true, err
	}
	return "", false, nil
}

func (s *Service) tryHandleNaturalForget(ctx context.Context, input string) (string, bool, error) {
	matches := forgetIntentPattern.FindStringSubmatch(strings.TrimSpace(input))
	if len(matches) != 2 {
		return "", false, nil
	}
	reply, err := s.forgetKnowledge(ctx, matches[1])
	return reply, true, err
}

func (s *Service) appendKnowledge(ctx context.Context, idOrPrefix, appendText string) (string, error) {
	entry, ok, err := s.store.Append(ctx, idOrPrefix, appendText)
	if err != nil {
		return "", err
	}
	if !ok {
		return fmt.Sprintf("没有找到知识 #%s。", strings.TrimPrefix(strings.TrimSpace(idOrPrefix), "#")), nil
	}
	return fmt.Sprintf("已补充 #%s\n%s", shortID(entry.ID), preview(entry.Text, maxReplyPreviewRunes)), nil
}

func (s *Service) appendLatestKnowledge(ctx context.Context, mc MessageContext, appendText string) (string, error) {
	entry, ok, err := s.store.AppendLatest(ctx, sourceLabel(mc), appendText)
	if err != nil {
		return "", err
	}
	if !ok {
		return "当前没有可补充的最近知识。先记住一条内容，再说“再补充一点：...”。", nil
	}
	return fmt.Sprintf("已补充 #%s\n%s", shortID(entry.ID), preview(entry.Text, maxReplyPreviewRunes)), nil
}

func (s *Service) forgetKnowledge(ctx context.Context, idOrPrefix string) (string, error) {
	entry, ok, err := s.store.Remove(ctx, idOrPrefix)
	if err != nil {
		return "", err
	}
	if !ok {
		return fmt.Sprintf("没有找到知识 #%s。", strings.TrimPrefix(strings.TrimSpace(idOrPrefix), "#")), nil
	}
	return fmt.Sprintf("已遗忘 #%s\n%s", shortID(entry.ID), preview(entry.Text, maxReplyPreviewRunes)), nil
}

func (s *Service) answerWithAllKnowledge(ctx context.Context, question string) (string, error) {
	entries, err := s.store.List(ctx)
	if err != nil {
		return "", err
	}
	return formatKnowledgeDump(entries, question)
}

func (s *Service) ensureAIAvailable(ctx context.Context) (string, error) {
	if s.aiService == nil {
		return "模型尚未启用。请先在本地环境变量中配置模型，或使用 `/remember` / `记住：` 明确保存内容。", nil
	}

	configured, err := s.aiService.IsConfigured(ctx)
	if err != nil {
		return "", err
	}
	if !configured {
		return "模型还没有配置完成。请先设置本地环境变量 `MYCLAW_MODEL_PROVIDER`、`MYCLAW_MODEL_BASE_URL`、`MYCLAW_MODEL_API_KEY` 和 `MYCLAW_MODEL_NAME`。", nil
	}
	return "", nil
}

func (s *Service) handleAIMessage(ctx context.Context, mc MessageContext, text string) (string, error) {
	if reply, err := s.ensureAIAvailable(ctx); reply != "" || err != nil {
		return reply, err
	}

	decision, err := s.aiService.RouteCommand(ctx, text)
	if err != nil {
		return "", err
	}

	switch decision.Command {
	case "remember":
		memoryText := strings.TrimSpace(decision.MemoryText)
		if memoryText == "" {
			memoryText = text
		}
		entry, err := s.store.Add(ctx, knowledge.Entry{
			Text:       memoryText,
			Source:     sourceLabel(mc),
			RecordedAt: time.Now(),
		})
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("已记住 #%s\n%s", shortID(entry.ID), preview(entry.Text, maxReplyPreviewRunes)), nil
	case "append":
		if decision.KnowledgeID == "" {
			return "请提供要补充的知识 ID。", nil
		}
		if decision.AppendText == "" {
			return "请提供要补充的内容。", nil
		}
		return s.appendKnowledge(ctx, decision.KnowledgeID, decision.AppendText)
	case "append_last":
		if decision.AppendText == "" {
			return "请提供要补充的内容。", nil
		}
		return s.appendLatestKnowledge(ctx, mc, decision.AppendText)
	case "forget":
		if decision.KnowledgeID == "" {
			return "请提供要遗忘的知识 ID。", nil
		}
		return s.forgetKnowledge(ctx, decision.KnowledgeID)
	case "notice_add":
		if decision.ReminderSpec == "" {
			return "请提供提醒时间和内容。", nil
		}
		return s.handleReminderCommand(ctx, mc, "/notice "+decision.ReminderSpec)
	case "notice_list":
		return s.handleReminderCommand(ctx, mc, "/notice list")
	case "notice_remove":
		if decision.ReminderID == "" {
			return "请提供要删除的提醒 ID。", nil
		}
		return s.handleReminderCommand(ctx, mc, "/notice remove "+decision.ReminderID)
	case "list":
		return s.handleCommand(ctx, mc, "/list")
	case "stats":
		return s.handleCommand(ctx, mc, "/stats")
	case "help":
		return s.handleCommand(ctx, mc, "/help")
	case "answer":
		entries, err := s.store.List(ctx)
		if err != nil {
			return "", err
		}
		question := strings.TrimSpace(decision.Question)
		if question == "" {
			question = text
		}
		return s.aiService.Answer(ctx, question, entries)
	default:
		return fmt.Sprintf("无法识别命令路由: %s", decision.Command), nil
	}
}

func parseRememberIntent(text string) (string, bool) {
	prefixes := []string{
		"记住：",
		"记住:",
		"记住 ",
		"记一下：",
		"记一下:",
		"记一下 ",
		"帮我记住：",
		"帮我记住:",
		"帮我记住 ",
		"保存：",
		"保存:",
		"保存 ",
	}
	for _, prefix := range prefixes {
		if strings.HasPrefix(text, prefix) {
			body := strings.TrimSpace(strings.TrimPrefix(text, prefix))
			return body, body != ""
		}
	}
	return "", false
}

func formatKnowledgeDump(entries []knowledge.Entry, question string) (string, error) {
	if len(entries) == 0 {
		if strings.TrimSpace(question) == "" {
			return "知识库为空。发送“记住：xxx”或 `/remember xxx` 添加内容。", nil
		}
		return fmt.Sprintf("我已读取知识库，但当前为空。\n\n你的问题：%s\n\n先用“记住：xxx”保存内容，再来问我。", question), nil
	}

	var builder strings.Builder
	if strings.TrimSpace(question) != "" {
		builder.WriteString("我已读取当前知识库的全部内容。\n")
		builder.WriteString("当前问题：")
		builder.WriteString(question)
		builder.WriteString("\n\n")
	} else {
		builder.WriteString("当前知识库的全部内容如下。\n\n")
	}

	scored := rankEntries(entries, question)
	if strings.TrimSpace(question) != "" && len(scored) > 0 && scored[0].score > 0 {
		builder.WriteString("可能更相关的内容：\n")
		limit := min(3, len(scored))
		for index := range limit {
			entry := scored[index].entry
			builder.WriteString(fmt.Sprintf("%d. #%s %s\n", index+1, shortID(entry.ID), preview(entry.Text, maxReplyPreviewRunes)))
		}
		builder.WriteString("\n")
	}

	builder.WriteString(fmt.Sprintf("完整知识库（共 %d 条）：\n", len(entries)))
	for index, entry := range entries {
		builder.WriteString(fmt.Sprintf("%d. #%s [%s] %s\n",
			index+1,
			shortID(entry.ID),
			entry.RecordedAt.Local().Format("2006-01-02 15:04:05"),
			entry.Text,
		))
	}

	builder.WriteString("\n当前版本是最小实现：每次回复都会直接读取完整知识库，不做向量检索、权限隔离或模型总结。")
	return strings.TrimSpace(builder.String()), nil
}

type rankedEntry struct {
	entry knowledge.Entry
	score int
}

func rankEntries(entries []knowledge.Entry, question string) []rankedEntry {
	question = strings.ToLower(strings.TrimSpace(question))
	if question == "" {
		return nil
	}
	tokens := splitTokens(question)
	if len(tokens) == 0 {
		return nil
	}

	ranked := make([]rankedEntry, 0, len(entries))
	for _, entry := range entries {
		score := 0
		lower := strings.ToLower(entry.Text)
		for _, token := range tokens {
			if strings.Contains(lower, token) {
				score++
			}
		}
		ranked = append(ranked, rankedEntry{entry: entry, score: score})
	}

	slices.SortFunc(ranked, func(a, b rankedEntry) int {
		if a.score != b.score {
			return b.score - a.score
		}
		switch {
		case a.entry.RecordedAt.After(b.entry.RecordedAt):
			return -1
		case a.entry.RecordedAt.Before(b.entry.RecordedAt):
			return 1
		default:
			return strings.Compare(a.entry.ID, b.entry.ID)
		}
	})
	return ranked
}

func splitTokens(text string) []string {
	parts := strings.FieldsFunc(text, func(r rune) bool {
		switch {
		case r >= 'a' && r <= 'z':
			return false
		case r >= '0' && r <= '9':
			return false
		case r >= 0x4e00 && r <= 0x9fff:
			return false
		default:
			return true
		}
	})
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if utf8.RuneCountInString(part) < 2 {
			continue
		}
		out = append(out, part)
	}
	return out
}

func preview(text string, maxRunes int) string {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) <= maxRunes {
		return string(runes)
	}
	return string(runes[:maxRunes]) + "..."
}

func normalizeSlash(text string) string {
	if strings.HasPrefix(text, "／") {
		return "/" + strings.TrimPrefix(text, "／")
	}
	return text
}

func sourceLabel(mc MessageContext) string {
	if strings.TrimSpace(mc.Interface) == "" && strings.TrimSpace(mc.UserID) == "" {
		return ""
	}
	if strings.TrimSpace(mc.UserID) == "" {
		return mc.Interface
	}
	if strings.TrimSpace(mc.Interface) == "" {
		return mc.UserID
	}
	return mc.Interface + ":" + mc.UserID
}

func shortID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8]
}
