package app

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"myclaw/internal/ai"
	"myclaw/internal/fileingest"
	"myclaw/internal/filesearch"
	"myclaw/internal/knowledge"
	"myclaw/internal/promptlib"
	"myclaw/internal/sessionstate"
	"myclaw/internal/skilllib"
)

const (
	maxReplyPreviewRunes      = 240
	retrievalCandidateLimit   = 8
	retrievalAnswerLimit      = 4
	retrievalPreviewItemLimit = 3
)

var (
	forgetIntentPattern    = regexp.MustCompile(`^(?:请)?(?:帮我)?(?:遗忘|忘掉|删除|删掉)\s*#?([0-9a-fA-F]{4,})\s*$`)
	appendByIDPattern      = regexp.MustCompile(`^(?:请)?(?:给|把)\s*#?([0-9a-fA-F]{4,})\s*(?:这条|这一条)?(?:再)?(?:补充|追加|补记|加上)(?:一点|一下|一条|一句|笔记)?(?:\s*[:：]\s*|\s+)(.+)$`)
	appendByIDShortPattern = regexp.MustCompile(`^#?([0-9a-fA-F]{4,})\s*(?:追加|补充|补记)(?:\s*[:：]\s*|\s+)(.+)$`)
	appendLastPattern      = regexp.MustCompile(`^(?:再)?(?:补充|追加|补记)(?:一点|一下|一条|一句|笔记)?(?:\s*[:：]\s*|\s+)(.+)$`)
	appendLastRefPattern   = regexp.MustCompile(`^(?:请)?(?:给|把)(?:上一条|上条|刚才那条|刚刚那条|这条)\s*(?:再)?(?:补充|追加|补记|加上)(?:一点|一下|一条|一句|笔记)?(?:\s*[:：]\s*|\s+)(.+)$`)
	resolveFileInput       = fileingest.Resolve
	extractPDFText         = fileingest.ExtractPDFText
)

type MessageContext struct {
	UserID    string
	Interface string
	SessionID string
	Project   string
}

type Service struct {
	store           *knowledge.Store
	aiService       aiBackend
	reminders       reminderBackend
	skillLoader     *skilllib.Loader
	promptStore     promptBackend
	sessionStore    *sessionstate.Store
	toolProviders   *agentToolProviders
	settingsMu      sync.RWMutex
	weixinHistory   conversationHistoryLimits
	fileSearchPath  string
	fileSearchExec  filesearch.SearchExecutor
	modeMu          sync.RWMutex
	modeMap         map[string]Mode
	loadedSkillsMu  sync.RWMutex
	loadedSkillsMap map[string]map[string]skilllib.Skill
}

type retrievalPlan struct {
	Queries          []string
	Keywords         []string
	FallbackKeywords []string
	AIStatus         string
	CanReview        bool
}

type aiBackend interface {
	IsConfigured(ctx context.Context) (bool, error)
	RouteCommand(ctx context.Context, input string) (ai.RouteDecision, error)
	BuildSearchPlan(ctx context.Context, question string) (ai.SearchPlan, error)
	DetectToolOpportunities(ctx context.Context, task string, tools []ai.ToolCapability) ([]ai.ToolOpportunity, error)
	PlanToolUse(ctx context.Context, task string, tool ai.ToolCapability, prior []ai.ToolExecution) (ai.ToolPlanDecision, error)
	ReviewAnswerCandidates(ctx context.Context, question string, entries []knowledge.Entry) ([]string, error)
	Answer(ctx context.Context, question string, entries []knowledge.Entry) (string, error)
	Chat(ctx context.Context, input string, history []ai.ConversationMessage) (string, error)
	DecideAgentStep(ctx context.Context, task string, history []ai.ConversationMessage, tools []ai.AgentToolDefinition, results []ai.AgentToolResult) (ai.AgentStepDecision, error)
	PlanNext(ctx context.Context, task string, history []ai.ConversationMessage, tools []ai.AgentToolDefinition, state ai.AgentTaskState) (ai.LoopDecision, error)
	SummarizeWorkingState(ctx context.Context, state ai.AgentTaskState) (string, error)
	SummarizeFinal(ctx context.Context, state ai.AgentTaskState, finalAnswer string) (string, error)
	TranslateToChinese(ctx context.Context, input string) (string, error)
	SummarizePDFText(ctx context.Context, fileName, extractedText string) (string, error)
	SummarizeImageFile(ctx context.Context, fileName, imageURL string) (string, error)
}

type streamingAIBackend interface {
	AnswerStream(ctx context.Context, question string, entries []knowledge.Entry, onDelta func(string)) (string, error)
	ChatStream(ctx context.Context, input string, history []ai.ConversationMessage, onDelta func(string)) (string, error)
}

type promptBackend interface {
	List(ctx context.Context) ([]promptlib.Prompt, error)
	Resolve(ctx context.Context, idOrPrefix string) (promptlib.Prompt, bool, error)
}

func NewService(store *knowledge.Store, aiService aiBackend, reminders reminderBackend) *Service {
	return NewServiceWithSkills(store, aiService, reminders, nil)
}

func NewServiceWithSkills(store *knowledge.Store, aiService aiBackend, reminders reminderBackend, skillLoader *skilllib.Loader) *Service {
	return NewServiceWithSkillsAndSessions(store, aiService, reminders, skillLoader, nil)
}

func NewServiceWithSkillsAndSessions(store *knowledge.Store, aiService aiBackend, reminders reminderBackend, skillLoader *skilllib.Loader, sessionStore *sessionstate.Store) *Service {
	return NewServiceWithRuntime(store, aiService, reminders, skillLoader, sessionStore, nil)
}

func NewServiceWithRuntime(store *knowledge.Store, aiService aiBackend, reminders reminderBackend, skillLoader *skilllib.Loader, sessionStore *sessionstate.Store, promptStore promptBackend) *Service {
	if skillLoader == nil {
		skillLoader = skilllib.NewLoader()
	}
	service := &Service{
		store:           store,
		aiService:       aiService,
		reminders:       reminders,
		skillLoader:     skillLoader,
		promptStore:     promptStore,
		sessionStore:    sessionStore,
		weixinHistory:   defaultWeixinConversationHistoryLimits(),
		fileSearchExec:  filesearch.ExecuteWithEverything,
		modeMap:         make(map[string]Mode),
		loadedSkillsMap: make(map[string]map[string]skilllib.Skill),
	}
	service.toolProviders = newAgentToolProviders()
	service.toolProviders.Register(newLocalAgentToolProvider(service))
	return service
}

func (s *Service) WeixinHistoryLimits() (messages int, runes int) {
	s.settingsMu.RLock()
	defer s.settingsMu.RUnlock()
	return s.weixinHistory.Messages, s.weixinHistory.Runes
}

func (s *Service) SetWeixinHistoryLimits(messages int, runes int) {
	if messages < 0 {
		messages = 0
	}
	if runes < 0 {
		runes = 0
	}
	s.settingsMu.Lock()
	s.weixinHistory = conversationHistoryLimits{
		Messages: messages,
		Runes:    runes,
	}
	s.settingsMu.Unlock()
}

func (s *Service) HandleMessage(ctx context.Context, mc MessageContext, input string) (string, error) {
	ctx = withTaskContext(withKnowledgeContext(ctx, mc))
	text := strings.TrimSpace(input)
	if text == "" {
		return "我没有收到有效内容。发送\u201c记住：xxx\u201d保存知识，或直接问问题。", nil
	}
	return s.handleMessageDispatch(ctx, mc, text, nil)
}

func (s *Service) HandleMessageStream(ctx context.Context, mc MessageContext, input string, onDelta func(string)) (string, error) {
	ctx = withTaskContext(withKnowledgeContext(ctx, mc))
	text := strings.TrimSpace(input)
	if text == "" {
		reply := "我没有收到有效内容。发送\u201c记住：xxx\u201d保存知识，或直接问问题。"
		emitIfPresent(onDelta, reply)
		return reply, nil
	}
	return s.handleMessageDispatch(ctx, mc, text, onDelta)
}

func (s *Service) handleMessageDispatch(ctx context.Context, mc MessageContext, text string, onDelta func(string)) (string, error) {
	if normalized := normalizeSlash(text); isSlashCommand(normalized) {
		reply, err := s.handleCommand(ctx, mc, normalized)
		if err == nil {
			emitIfPresent(onDelta, reply)
		}
		return reply, err
	}

	if reply, ok, err := s.tryHandleNaturalAppend(ctx, mc, text); ok || err != nil {
		if err == nil {
			emitIfPresent(onDelta, reply)
		}
		return reply, err
	}

	if reply, ok, err := s.tryHandleNaturalReminder(ctx, mc, text); ok || err != nil {
		if err == nil {
			emitIfPresent(onDelta, reply)
		}
		return reply, err
	}

	if reply, ok, err := s.tryHandleNaturalForget(ctx, text); ok || err != nil {
		if err == nil {
			emitIfPresent(onDelta, reply)
		}
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
		reply := fmt.Sprintf("已记住 #%s\n%s", shortID(entry.ID), preview(entry.Text, maxReplyPreviewRunes))
		emitIfPresent(onDelta, reply)
		return reply, nil
	}

	if reply, ok, err := s.tryHandleDirectFileIngest(ctx, mc, text); ok || err != nil {
		if err == nil {
			emitIfPresent(onDelta, reply)
		}
		return reply, err
	}

	if reply, ok, err := s.tryHandleFileSearch(ctx, mc, text); ok || err != nil {
		if err == nil {
			emitIfPresent(onDelta, reply)
		}
		return reply, err
	}

	return s.handleConversationMessageStream(ctx, mc, text, onDelta)
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
		return "当前没有可补充的最近知识。先记住一条内容，再说\u201c再补充一点：...\u201d。", nil
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

func (s *Service) tryHandleDirectFileIngest(ctx context.Context, mc MessageContext, input string) (string, bool, error) {
	if _, ok, err := resolveFileInput(input); err != nil {
		return "", true, err
	} else if !ok {
		return "", false, nil
	}
	reply, err := s.ingestFilePath(ctx, mc, input)
	return reply, true, err
}

func (s *Service) ingestFilePath(ctx context.Context, mc MessageContext, rawPath string) (string, error) {
	input, ok, err := resolveFileInput(rawPath)
	if err != nil {
		return "", err
	}
	if !ok {
		return "只支持直接输入现有的图片或 PDF 文件路径。", nil
	}

	reply, err := s.ensureAIAvailable(ctx)
	if reply != "" || err != nil {
		return reply, err
	}
	ctx = s.withSkillContext(ctx, mc)

	var summary string
	switch input.Kind {
	case fileingest.KindPDF:
		extractedText, extractErr := extractPDFText(input.Path)
		if extractErr != nil {
			if errors.Is(extractErr, fileingest.ErrPDFExtractorUnavailable) {
				return "当前这个构建不包含 PDF 文本提取能力。请使用启用 CGO 的本机构建来开启 go-fitz PDF 总结。", nil
			}
			return "", extractErr
		}
		summary, err = s.aiService.SummarizePDFText(ctx, input.Name, extractedText)
	case fileingest.KindImage:
		summary, err = s.aiService.SummarizeImageFile(ctx, input.Name, input.DataURL)
	default:
		return "暂不支持这个文件类型。", nil
	}
	if err != nil {
		return "", err
	}

	entry, err := s.store.Add(ctx, knowledge.Entry{
		Text:       fileingest.FormatKnowledgeText(input, summary),
		Source:     sourceLabel(mc),
		RecordedAt: time.Now(),
	})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("已记住 #%s\n%s", shortID(entry.ID), preview(entry.Text, maxReplyPreviewRunes)), nil
}

func (s *Service) ensureAIAvailable(ctx context.Context) (string, error) {
	if s.aiService == nil {
		return "模型尚未启用。请先配置模型，或使用 `/remember` / `记住：` 明确保存内容。", nil
	}

	configured, err := s.aiService.IsConfigured(ctx)
	if err != nil {
		return "", err
	}
	if !configured {
		return "模型还没有配置完成。请先在桌面端模型页保存 Provider、Base URL、API Key 和 Model，或设置对应环境变量。", nil
	}
	return "", nil
}

func (s *Service) selectKnowledgeForAnswer(ctx context.Context, question string) ([]knowledge.Entry, error) {
	plan, err := s.buildRetrievalPlan(ctx, question)
	if err != nil {
		return nil, err
	}
	results, err := s.searchCandidates(ctx, plan.Queries, plan.Keywords, retrievalCandidateLimit)
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		entries, err := s.store.List(ctx)
		if err != nil {
			return nil, err
		}
		return latestEntries(entries, retrievalAnswerLimit), nil
	}

	candidates := make([]knowledge.Entry, 0, len(results))
	for _, result := range results {
		candidates = append(candidates, result.Entry)
	}

	selected := candidates
	if plan.CanReview {
		if reviewedIDs, err := s.aiService.ReviewAnswerCandidates(ctx, question, candidates); err == nil {
			reviewed := pickEntriesByID(candidates, reviewedIDs)
			if len(reviewed) > 0 {
				selected = reviewed
			}
		}
	}

	if len(selected) > retrievalAnswerLimit {
		selected = selected[:retrievalAnswerLimit]
	}
	return selected, nil
}

func (s *Service) debugSearch(ctx context.Context, question string) (string, error) {
	question = strings.TrimSpace(question)
	if question == "" {
		return "用法: /debug-search <问题>", nil
	}

	plan, err := s.buildRetrievalPlan(ctx, question)
	if err != nil {
		return "", err
	}
	results, err := s.searchCandidates(ctx, plan.Queries, plan.Keywords, retrievalCandidateLimit)
	if err != nil {
		return "", err
	}

	candidates := make([]knowledge.Entry, 0, len(results))
	for _, result := range results {
		candidates = append(candidates, result.Entry)
	}

	selectedEntries := candidates
	selectedIDs := []string(nil)
	reviewStatus := "未执行"
	if !plan.CanReview {
		reviewStatus = "模型未启用或未配置"
	}
	if len(candidates) > 0 && plan.CanReview {
		reviewedIDs, err := s.aiService.ReviewAnswerCandidates(ctx, question, candidates)
		if err != nil {
			reviewStatus = "复核失败: " + err.Error()
		} else {
			reviewStatus = "已复核"
			selectedIDs = reviewedIDs
			reviewed := pickEntriesByID(candidates, reviewedIDs)
			if len(reviewed) > 0 {
				selectedEntries = reviewed
			}
		}
	}

	if len(results) == 0 {
		entries, err := s.store.List(ctx)
		if err != nil {
			return "", err
		}
		selectedEntries = latestEntries(entries, retrievalAnswerLimit)
		if reviewStatus == "未执行" {
			reviewStatus = "无候选，回退到最近知识"
		}
	}

	var builder strings.Builder
	builder.WriteString("检索调试\n")
	builder.WriteString("问题：")
	builder.WriteString(question)
	builder.WriteString("\n")
	builder.WriteString("检索问句：")
	builder.WriteString(formatKeywordList(plan.Queries))
	builder.WriteString("\n")
	builder.WriteString("AI关键词：")
	builder.WriteString(formatKeywordList(plan.Keywords))
	builder.WriteString("\n")
	builder.WriteString("模型状态：")
	builder.WriteString(plan.AIStatus)
	builder.WriteString("\n")
	if len(plan.FallbackKeywords) > 0 {
		builder.WriteString("本地兜底关键词：")
		builder.WriteString(formatKeywordList(plan.FallbackKeywords))
		builder.WriteString("\n")
	}
	builder.WriteString("复核状态：")
	builder.WriteString(reviewStatus)
	builder.WriteString("\n\n")

	if len(results) == 0 {
		builder.WriteString("候选：没有关键词命中，已回退到最近知识。\n")
	} else {
		builder.WriteString("候选：\n")
		for index, result := range results {
			builder.WriteString(fmt.Sprintf("%d. #%s score=%d matches=%s\n%s\n",
				index+1,
				shortID(result.Entry.ID),
				result.Score,
				formatKeywordList(result.Matches),
				preview(result.Entry.Text, maxReplyPreviewRunes),
			))
		}
	}

	builder.WriteString("\n最终会送去回答的知识：\n")
	if len(selectedEntries) == 0 {
		builder.WriteString("(空)")
		return builder.String(), nil
	}
	selectedSet := make(map[string]struct{}, len(selectedIDs))
	for _, id := range selectedIDs {
		selectedSet[id] = struct{}{}
	}
	for index, entry := range selectedEntries {
		prefix := ""
		if len(selectedSet) > 0 {
			if _, ok := selectedSet[entry.ID]; ok {
				prefix = "[review] "
			}
		}
		builder.WriteString(fmt.Sprintf("%d. %s#%s %s\n",
			index+1,
			prefix,
			shortID(entry.ID),
			preview(entry.Text, maxReplyPreviewRunes),
		))
	}
	return strings.TrimSpace(builder.String()), nil
}

func (s *Service) buildRetrievalPlan(ctx context.Context, question string) (retrievalPlan, error) {
	question = strings.TrimSpace(question)
	plan := retrievalPlan{
		Queries:   []string{question},
		AIStatus:  "未启用",
		CanReview: false,
	}

	if s.aiService == nil {
		plan.Keywords = knowledge.GenerateKeywords(question)
		plan.FallbackKeywords = plan.Keywords
		return plan, nil
	}

	configured, err := s.aiService.IsConfigured(ctx)
	if err != nil {
		return retrievalPlan{}, err
	}
	if !configured {
		plan.AIStatus = "未配置"
		plan.Keywords = knowledge.GenerateKeywords(question)
		plan.FallbackKeywords = plan.Keywords
		return plan, nil
	}

	plan.CanReview = true
	searchPlan, err := s.aiService.BuildSearchPlan(ctx, question)
	if err != nil {
		plan.AIStatus = "检索计划生成失败: " + err.Error()
		plan.Keywords = knowledge.GenerateKeywords(question)
		plan.FallbackKeywords = plan.Keywords
		return plan, nil
	}

	plan.AIStatus = "已生成检索计划"
	if len(searchPlan.Queries) > 0 {
		plan.Queries = searchPlan.Queries
	}
	plan.Keywords = searchPlan.Keywords
	return plan, nil
}

func (s *Service) searchCandidates(ctx context.Context, queries []string, keywords []string, limit int) ([]knowledge.SearchResult, error) {
	queries = normalizeSearchInputs(queries)
	if len(queries) == 0 {
		queries = []string{""}
	}

	combined := make(map[string]knowledge.SearchResult)
	for _, query := range queries {
		results, err := s.store.Search(ctx, query, keywords, limit)
		if err != nil {
			return nil, err
		}
		for _, result := range results {
			current, ok := combined[result.Entry.ID]
			if !ok {
				combined[result.Entry.ID] = result
				continue
			}
			current.Score += result.Score
			current.Matches = knowledge.MergeKeywords(current.Matches, result.Matches)
			combined[result.Entry.ID] = current
		}
	}

	out := make([]knowledge.SearchResult, 0, len(combined))
	for _, result := range combined {
		out = append(out, result)
	}
	slices.SortFunc(out, func(a, b knowledge.SearchResult) int {
		if a.Score != b.Score {
			return b.Score - a.Score
		}
		switch {
		case a.Entry.RecordedAt.After(b.Entry.RecordedAt):
			return -1
		case a.Entry.RecordedAt.Before(b.Entry.RecordedAt):
			return 1
		default:
			return strings.Compare(a.Entry.ID, b.Entry.ID)
		}
	})
	if limit > 0 && len(out) > limit {
		return out[:limit], nil
	}
	return out, nil
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
			return "知识库为空。发送\u201c记住：xxx\u201d或 `/remember xxx` 添加内容。", nil
		}
		return fmt.Sprintf("我已读取知识库，但当前为空。\n\n你的问题：%s\n\n先用\u201c记住：xxx\u201d保存内容，再来问我。", question), nil
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

	scored := knowledge.RankEntries(entries, question, nil, retrievalPreviewItemLimit)
	if strings.TrimSpace(question) != "" && len(scored) > 0 && scored[0].Score > 0 {
		builder.WriteString("可能更相关的内容：\n")
		for index, result := range scored {
			entry := result.Entry
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

	builder.WriteString("\n当前版本默认使用关键词检索和候选复核来回答问题，不做向量检索、权限隔离或多租户隔离。")
	return strings.TrimSpace(builder.String()), nil
}

func latestEntries(entries []knowledge.Entry, limit int) []knowledge.Entry {
	if limit <= 0 || len(entries) == 0 {
		return nil
	}
	if len(entries) <= limit {
		out := append([]knowledge.Entry(nil), entries...)
		reverseEntries(out)
		return out
	}
	out := append([]knowledge.Entry(nil), entries[len(entries)-limit:]...)
	reverseEntries(out)
	return out
}

func reverseEntries(entries []knowledge.Entry) {
	for left, right := 0, len(entries)-1; left < right; left, right = left+1, right-1 {
		entries[left], entries[right] = entries[right], entries[left]
	}
}

func pickEntriesByID(entries []knowledge.Entry, ids []string) []knowledge.Entry {
	if len(entries) == 0 || len(ids) == 0 {
		return nil
	}
	index := make(map[string]knowledge.Entry, len(entries))
	for _, entry := range entries {
		index[entry.ID] = entry
	}

	out := make([]knowledge.Entry, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		entry, ok := index[id]
		if !ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, entry)
	}
	return out
}

func formatKeywordList(values []string) string {
	if len(values) == 0 {
		return "(空)"
	}
	return strings.Join(values, ", ")
}

func normalizeSearchInputs(values []string) []string {
	var out []string
	seen := make(map[string]struct{})
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
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

func (s *Service) listSkills(mc MessageContext) (string, error) {
	skills, err := s.skillLoader.List()
	if err != nil {
		return "", err
	}

	active := s.loadedSkillsFor(mc)
	activeSet := make(map[string]struct{}, len(active))
	for _, skill := range active {
		activeSet[strings.ToLower(skill.Name)] = struct{}{}
	}

	if len(skills) == 0 {
		var builder strings.Builder
		builder.WriteString("当前没有可用技能。")
		dirs := s.skillLoader.Dirs()
		if len(dirs) > 0 {
			builder.WriteString("\n请把技能放到以下目录之一：\n")
			for _, dir := range dirs {
				builder.WriteString("- ")
				builder.WriteString(filepath.Join(dir, "<技能名>", "SKILL.md"))
				builder.WriteString("\n")
			}
		}
		return strings.TrimSpace(builder.String()), nil
	}

	var builder strings.Builder
	builder.WriteString("可用技能：\n")
	for _, skill := range skills {
		builder.WriteString("- ")
		builder.WriteString(skill.Name)
		if _, ok := activeSet[strings.ToLower(skill.Name)]; ok {
			builder.WriteString(" [已加载]")
		}
		if desc := strings.TrimSpace(skill.Description); desc != "" {
			builder.WriteString("：")
			builder.WriteString(desc)
		}
		builder.WriteString("\n")
	}
	return strings.TrimSpace(builder.String()), nil
}

func (s *Service) showSkill(name string) (string, error) {
	skill, ok, err := s.skillLoader.Load(name)
	if err != nil {
		return "", err
	}
	if !ok {
		return fmt.Sprintf("没有找到技能 %q。先用 /skills 查看可用技能。", strings.TrimSpace(name)), nil
	}
	return skill.Content, nil
}

func (s *Service) loadSkill(mc MessageContext, name string) (string, error) {
	skill, ok, err := s.skillLoader.Load(name)
	if err != nil {
		return "", err
	}
	if !ok {
		return fmt.Sprintf("没有找到技能 %q。先用 /skills 查看可用技能。", strings.TrimSpace(name)), nil
	}

	_ = s.loadedSkillsFor(mc)
	key := skillSessionKey(mc)
	s.loadedSkillsMu.Lock()
	if s.loadedSkillsMap[key] == nil {
		s.loadedSkillsMap[key] = make(map[string]skilllib.Skill)
	}
	s.loadedSkillsMap[key][strings.ToLower(skill.Name)] = skill
	names := make([]string, 0, len(s.loadedSkillsMap[key]))
	for _, loadedSkill := range s.loadedSkillsMap[key] {
		names = append(names, loadedSkill.Name)
	}
	s.loadedSkillsMu.Unlock()
	s.setPersistedLoadedSkillNames(mc, names)

	if strings.TrimSpace(skill.Description) == "" {
		return fmt.Sprintf("已加载技能 %s。", skill.Name), nil
	}
	return fmt.Sprintf("已加载技能 %s：%s", skill.Name, skill.Description), nil
}

func (s *Service) unloadSkill(mc MessageContext, name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "请提供要卸载的技能名。", nil
	}

	_ = s.loadedSkillsFor(mc)
	key := skillSessionKey(mc)
	normalized := strings.ToLower(name)

	s.loadedSkillsMu.Lock()
	defer s.loadedSkillsMu.Unlock()

	if s.loadedSkillsMap[key] == nil {
		return fmt.Sprintf("当前会话没有加载技能 %q。", name), nil
	}
	if _, ok := s.loadedSkillsMap[key][normalized]; !ok {
		return fmt.Sprintf("当前会话没有加载技能 %q。", name), nil
	}
	delete(s.loadedSkillsMap[key], normalized)
	names := make([]string, 0, len(s.loadedSkillsMap[key]))
	for _, loadedSkill := range s.loadedSkillsMap[key] {
		names = append(names, loadedSkill.Name)
	}
	if len(s.loadedSkillsMap[key]) == 0 {
		delete(s.loadedSkillsMap, key)
	}
	s.setPersistedLoadedSkillNames(mc, names)
	return fmt.Sprintf("已卸载技能 %s。", name), nil
}

func (s *Service) listLoadedSkills(mc MessageContext) string {
	active := s.loadedSkillsFor(mc)
	if len(active) == 0 {
		return "当前会话还没有加载技能。"
	}

	var builder strings.Builder
	builder.WriteString("当前会话已加载技能：\n")
	for _, skill := range active {
		builder.WriteString("- ")
		builder.WriteString(skill.Name)
		if desc := strings.TrimSpace(skill.Description); desc != "" {
			builder.WriteString("：")
			builder.WriteString(desc)
		}
		builder.WriteString("\n")
	}
	return strings.TrimSpace(builder.String())
}

func (s *Service) loadedSkillsFor(mc MessageContext) []skilllib.Skill {
	key := skillSessionKey(mc)

	s.loadedSkillsMu.RLock()
	current := s.loadedSkillsMap[key]
	s.loadedSkillsMu.RUnlock()
	if len(current) == 0 {
		restored := make(map[string]skilllib.Skill)
		for _, name := range s.persistedLoadedSkillNames(mc) {
			skill, ok, err := s.skillLoader.Load(name)
			if err != nil || !ok {
				continue
			}
			restored[strings.ToLower(skill.Name)] = skill
		}
		if len(restored) > 0 {
			s.loadedSkillsMu.Lock()
			if len(s.loadedSkillsMap[key]) == 0 {
				s.loadedSkillsMap[key] = restored
			}
			current = s.loadedSkillsMap[key]
			s.loadedSkillsMu.Unlock()
		}
	}
	if len(current) == 0 {
		return nil
	}

	names := make([]string, 0, len(current))
	for name := range current {
		names = append(names, name)
	}
	slices.Sort(names)

	out := make([]skilllib.Skill, 0, len(names))
	for _, name := range names {
		out = append(out, current[name])
	}
	return out
}

func (s *Service) ListAvailableSkills() ([]skilllib.Skill, error) {
	return s.skillLoader.List()
}

func (s *Service) ListLoadedSkills(mc MessageContext) []skilllib.Skill {
	return s.loadedSkillsFor(mc)
}

func (s *Service) LoadSkillForSession(mc MessageContext, name string) (string, error) {
	return s.loadSkill(mc, name)
}

func (s *Service) UnloadSkillForSession(mc MessageContext, name string) (string, error) {
	return s.unloadSkill(mc, name)
}

func skillSessionKey(mc MessageContext) string {
	return conversationSessionKey(mc)
}

func withKnowledgeContext(ctx context.Context, mc MessageContext) context.Context {
	if project := strings.TrimSpace(mc.Project); project != "" {
		return knowledge.WithProject(ctx, project)
	}
	return ctx
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
