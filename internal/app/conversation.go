package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"baize/internal/ai"
	"baize/internal/knowledge"
)

func (s *Service) conversationMode(ctx context.Context, mc MessageContext) (Mode, error) {
	key := conversationSessionKey(mc)

	s.modeMu.RLock()
	mode, ok := s.modeMap[key]
	s.modeMu.RUnlock()
	if ok && mode != "" {
		return mode, nil
	}

	if s.sessionStore != nil {
		snapshot, ok, err := s.sessionStore.Load(ctx, key)
		if err != nil {
			return "", err
		}
		if ok {
			mode = normalizeMode(snapshot.Mode)
			if mode == "" {
				mode = defaultMode()
				snapshot.Mode = string(mode)
				_ = s.saveSessionSnapshot(ctx, snapshot)
			}
			s.rememberConversationMode(key, mode)
			return mode, nil
		}
	}

	mode = defaultMode()
	s.rememberConversationMode(key, mode)
	return mode, nil
}

func (s *Service) setConversationMode(ctx context.Context, mc MessageContext, mode Mode) (Mode, error) {
	mode = normalizeMode(string(mode))
	if mode == "" {
		return "", fmt.Errorf("unsupported mode")
	}

	key := conversationSessionKey(mc)
	s.rememberConversationMode(key, mode)

	if s.sessionStore != nil {
		snapshot, err := s.sessionSnapshotByKey(ctx, key)
		if err != nil {
			return "", err
		}
		snapshot.Mode = string(mode)
		if err := s.saveSessionSnapshot(ctx, snapshot); err != nil {
			return "", err
		}
	}
	return mode, nil
}

func (s *Service) rememberConversationMode(key string, mode Mode) {
	s.modeMu.Lock()
	defer s.modeMu.Unlock()
	s.modeMap[key] = mode
}

func (s *Service) GetMode(ctx context.Context, mc MessageContext) (Mode, error) {
	return s.conversationMode(ctx, mc)
}

func (s *Service) SetMode(ctx context.Context, mc MessageContext, mode Mode) (Mode, error) {
	return s.setConversationMode(ctx, mc, mode)
}

func (s *Service) handleConversationMessageStream(ctx context.Context, mc MessageContext, text string, onDelta func(string)) (string, error) {
	mode, stripped, overridden, err := s.resolveConversationMode(ctx, mc, text)
	if err != nil {
		return "", err
	}
	if overridden {
		text = stripped
		if text == "" {
			reply := "请在 `@ai`、`@kb` 或 `@agent` 后输入具体内容。"
			emitIfPresent(onDelta, reply)
			return reply, nil
		}
		if normalized := normalizeSlash(text); isSlashCommand(normalized) {
			reply, err := s.handleCommand(ctx, mc, normalized)
			if err == nil {
				emitIfPresent(onDelta, reply)
			}
			return reply, err
		}
	}

	if mode == ModeAsk {
		return s.handleAskConversationStream(ctx, mc, text, onDelta)
	}

	return s.handleAIDecisionInternal(ctx, mc, text, mode, onDelta)
}

func (s *Service) resolveConversationMode(ctx context.Context, mc MessageContext, text string) (Mode, string, bool, error) {
	if mode, stripped, ok := parseModeOverride(text); ok {
		return mode, stripped, true, nil
	}

	mode, err := s.conversationMode(ctx, mc)
	if err != nil {
		return "", "", false, err
	}
	return mode, text, false, nil
}

func emitIfPresent(onDelta func(string), reply string) {
	if onDelta != nil && reply != "" {
		onDelta(reply)
	}
}

func emitChunkedIfPresent(onDelta func(string), reply string) {
	if onDelta == nil || reply == "" {
		return
	}
	for _, chunk := range splitReplyIntoChunks(reply, 24) {
		onDelta(chunk)
	}
}

func splitReplyIntoChunks(reply string, maxRunes int) []string {
	if reply == "" {
		return nil
	}
	if maxRunes <= 0 {
		return []string{reply}
	}

	var chunks []string
	current := make([]rune, 0, maxRunes)
	flush := func() {
		if len(current) == 0 {
			return
		}
		chunks = append(chunks, string(current))
		current = current[:0]
	}

	for _, r := range []rune(reply) {
		current = append(current, r)
		if r == '\n' || len(current) >= maxRunes {
			flush()
		}
	}
	flush()
	return chunks
}

func (s *Service) handleAIDecision(ctx context.Context, mc MessageContext, text string, mode Mode) (string, error) {
	return s.handleAIDecisionInternal(ctx, mc, text, mode, nil)
}

func (s *Service) handleAIDecisionStream(ctx context.Context, mc MessageContext, text string, mode Mode, onDelta func(string)) (string, error) {
	return s.handleAIDecisionInternal(ctx, mc, text, mode, onDelta)
}

func (s *Service) handleAskConversationStream(ctx context.Context, mc MessageContext, text string, onDelta func(string)) (string, error) {
	if reply, err := s.ensureAIAvailable(ctx); reply != "" || err != nil {
		emitIfPresent(onDelta, reply)
		return reply, err
	}
	ctx = s.withSkillContext(ctx, mc)

	history := s.chatHistoryWithRuntimeState(ctx, mc)
	addProcessTrace(ctx, "执行模式", "mode=ask\nhistory="+fmt.Sprintf("%d", len(history)))
	reply, err := s.streamOrChat(ctx, text, history, onDelta)
	if err == nil {
		s.maybeAppendConversationHistory(ctx, mc, text, reply)
	}
	return reply, err
}

func (s *Service) handleAIDecisionInternal(ctx context.Context, mc MessageContext, text string, mode Mode, onDelta func(string)) (string, error) {
	if reply, err := s.ensureAIAvailable(ctx); reply != "" || err != nil {
		emitIfPresent(onDelta, reply)
		return reply, err
	}
	ctx = s.withSkillContext(ctx, mc)

	decision, err := s.aiService.RouteCommand(ctx, text)
	if err != nil {
		addProcessTrace(ctx, "AI 路由失败", err.Error())
		return "", err
	}
	rawCommand := strings.TrimSpace(decision.Command)
	decision = normalizeConversationRouteDecision(text, decision)
	if !strings.EqualFold(rawCommand, strings.TrimSpace(decision.Command)) {
		addProcessTrace(ctx, "AI 路由修正", "from="+rawCommand+"\nto="+strings.TrimSpace(decision.Command))
	}
	addProcessTrace(ctx, "AI 路由", "command="+strings.TrimSpace(decision.Command)+"\nmode="+string(mode))

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
		reply := fmt.Sprintf("已记住 #%s\n%s", shortID(entry.ID), preview(entry.Text, maxReplyPreviewRunes))
		emitIfPresent(onDelta, reply)
		return reply, nil
	case "append":
		if decision.KnowledgeID == "" {
			reply := "请提供要补充的知识 ID。"
			emitIfPresent(onDelta, reply)
			return reply, nil
		}
		if decision.AppendText == "" {
			reply := "请提供要补充的内容。"
			emitIfPresent(onDelta, reply)
			return reply, nil
		}
		reply, err := s.appendKnowledge(ctx, decision.KnowledgeID, decision.AppendText)
		if err == nil {
			emitIfPresent(onDelta, reply)
		}
		return reply, err
	case "append_last":
		if decision.AppendText == "" {
			reply := "请提供要补充的内容。"
			emitIfPresent(onDelta, reply)
			return reply, nil
		}
		reply, err := s.appendLatestKnowledge(ctx, mc, decision.AppendText)
		if err == nil {
			emitIfPresent(onDelta, reply)
		}
		return reply, err
	case "forget":
		if decision.KnowledgeID == "" {
			reply := "请提供要遗忘的知识 ID。"
			emitIfPresent(onDelta, reply)
			return reply, nil
		}
		reply, err := s.forgetKnowledge(ctx, decision.KnowledgeID)
		if err == nil {
			emitIfPresent(onDelta, reply)
		}
		return reply, err
	case "notice_add":
		if decision.ReminderSpec == "" {
			reply := "请提供提醒时间和内容。"
			emitIfPresent(onDelta, reply)
			return reply, nil
		}
		reply, err := s.handleReminderCommand(ctx, mc, "/notice "+decision.ReminderSpec)
		if err == nil {
			emitIfPresent(onDelta, reply)
		}
		return reply, err
	case "notice_list":
		reply, err := s.handleReminderCommand(ctx, mc, "/notice list")
		if err == nil {
			emitIfPresent(onDelta, reply)
		}
		return reply, err
	case "notice_remove":
		if decision.ReminderID == "" {
			reply := "请提供要删除的提醒 ID。"
			emitIfPresent(onDelta, reply)
			return reply, nil
		}
		reply, err := s.handleReminderCommand(ctx, mc, "/notice remove "+decision.ReminderID)
		if err == nil {
			emitIfPresent(onDelta, reply)
		}
		return reply, err
	case "list":
		reply, err := s.handleCommand(ctx, mc, "/kb list")
		if err == nil {
			emitIfPresent(onDelta, reply)
		}
		return reply, err
	case "stats":
		reply, err := s.handleCommand(ctx, mc, "/kb stats")
		if err == nil {
			emitIfPresent(onDelta, reply)
		}
		return reply, err
	case "help":
		reply, err := s.handleCommand(ctx, mc, "/help")
		if err == nil {
			emitIfPresent(onDelta, reply)
		}
		return reply, err
	case "answer":
		question := strings.TrimSpace(decision.Question)
		if question == "" {
			question = text
		}
		switch mode {
		case modeKnowledgeOverride:
			entries, err := s.selectKnowledgeForAnswer(ctx, question)
			if err != nil {
				return "", err
			}
			addProcessTrace(ctx, "执行模式", "mode=knowledge\nentries="+fmt.Sprintf("%d", len(entries)))
			reply, err := s.streamOrAnswer(ctx, question, entries, onDelta)
			if err != nil {
				addProcessTrace(ctx, "执行失败", err.Error())
			}
			if err == nil {
				s.maybeAppendConversationHistory(ctx, mc, question, reply)
			}
			return reply, err
		case ModeAgent:
			addProcessTrace(ctx, "执行模式", "mode=agent")
			reply, err := s.handleAgentQuestion(ctx, mc, question)
			if err == nil {
				emitChunkedIfPresent(onDelta, reply)
			}
			return reply, err
		case ModeAsk:
			fallthrough
		default:
			history := s.chatHistoryWithRuntimeState(ctx, mc)
			addProcessTrace(ctx, "执行模式", "mode=ask\nhistory="+fmt.Sprintf("%d", len(history)))
			reply, err := s.streamOrChat(ctx, question, history, onDelta)
			if err != nil {
				addProcessTrace(ctx, "执行失败", err.Error())
			}
			if err == nil {
				s.maybeAppendConversationHistory(ctx, mc, question, reply)
			}
			return reply, err
		}
	default:
		reply := fmt.Sprintf("无法识别命令路由: %s", decision.Command)
		emitIfPresent(onDelta, reply)
		return reply, nil
	}
}

func normalizeConversationRouteDecision(input string, decision ai.RouteDecision) ai.RouteDecision {
	command := strings.TrimSpace(strings.ToLower(decision.Command))
	if command == "" {
		return decision
	}

	if command == "help" && !looksLikeConversationHelpIntent(input) {
		return ai.RouteDecision{
			Command:  "answer",
			Question: strings.TrimSpace(input),
		}
	}

	if command == "answer" && strings.TrimSpace(decision.Question) == "" {
		decision.Question = strings.TrimSpace(input)
	}
	return decision
}

func looksLikeConversationHelpIntent(input string) bool {
	text := strings.ToLower(strings.TrimSpace(normalizeSlash(input)))
	if text == "" {
		return false
	}

	exactPhrases := []string{
		"help",
		"usage",
		"show help",
		"show usage",
		"帮助",
		"查看帮助",
		"需要帮助",
		"用法",
	}
	for _, phrase := range exactPhrases {
		if text == phrase {
			return true
		}
	}

	containsPhrases := []string{
		"how to use",
		"how do i use",
		"what can you do",
		"available commands",
		"command list",
		"show commands",
		"怎么用",
		"如何用",
		"可以做什么",
		"能做什么",
		"有哪些功能",
		"可用命令",
		"命令列表",
		"有哪些命令",
		"可用指令",
		"指令列表",
		"有哪些指令",
	}
	for _, phrase := range containsPhrases {
		if strings.Contains(text, phrase) {
			return true
		}
	}
	return false
}

func (s *Service) streamOrAnswer(ctx context.Context, question string, entries []knowledge.Entry, onDelta func(string)) (string, error) {
	if streamer, ok := s.aiService.(streamingAIBackend); ok {
		return streamer.AnswerStream(ctx, question, entries, onDelta)
	}
	reply, err := s.aiService.Answer(ctx, question, entries)
	if err == nil && onDelta != nil && reply != "" {
		onDelta(reply)
	}
	return reply, err
}

func (s *Service) streamOrChat(ctx context.Context, question string, history []ai.ConversationMessage, onDelta func(string)) (string, error) {
	if streamer, ok := s.aiService.(streamingAIBackend); ok {
		return streamer.ChatStream(ctx, question, history, onDelta)
	}
	reply, err := s.aiService.Chat(ctx, question, history)
	if err == nil && onDelta != nil && reply != "" {
		onDelta(reply)
	}
	return reply, err
}

func conversationSessionKey(mc MessageContext) string {
	parts := make([]string, 0, 4)
	if source := strings.TrimSpace(sourceLabel(mc)); source != "" {
		parts = append(parts, "source:"+source)
	}
	if sessionID := strings.TrimSpace(mc.SessionID); sessionID != "" {
		parts = append(parts, "session:"+sessionID)
	}
	if project := strings.TrimSpace(mc.Project); project != "" {
		parts = append(parts, "project:"+strings.ToLower(project))
	}
	if len(parts) == 0 {
		return "default"
	}
	return strings.Join(parts, "|")
}
