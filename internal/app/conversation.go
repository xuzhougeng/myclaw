package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"myclaw/internal/knowledge"
	"myclaw/internal/sessionstate"
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
		if _, err := s.sessionStore.Save(ctx, sessionstate.Snapshot{
			Key:  key,
			Mode: string(mode),
		}); err != nil {
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

func (s *Service) handleConversationMessage(ctx context.Context, mc MessageContext, text string) (string, error) {
	mode, stripped, overridden, err := s.resolveConversationMode(ctx, mc, text)
	if err != nil {
		return "", err
	}
	if overridden {
		text = stripped
		if text == "" {
			return "请在 `@ai`、`@kb` 或 `@agent` 后输入具体内容。", nil
		}
		if normalized := normalizeSlash(text); isSlashCommand(normalized) {
			return s.handleCommand(ctx, mc, normalized)
		}
	}

	return s.handleAIDecision(ctx, mc, text, mode)
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

func (s *Service) handleAIDecision(ctx context.Context, mc MessageContext, text string, mode Mode) (string, error) {
	if reply, err := s.ensureAIAvailable(ctx); reply != "" || err != nil {
		return reply, err
	}
	ctx = s.withSkillContext(ctx, mc)

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
		question := strings.TrimSpace(decision.Question)
		if question == "" {
			question = text
		}
		switch mode {
		case ModeKnowledge:
			entries, err := s.selectKnowledgeForAnswer(ctx, question)
			if err != nil {
				return "", err
			}
			reply, err := s.aiService.Answer(ctx, question, entries)
			if err == nil {
				s.appendConversationHistory(ctx, mc, question, reply)
			}
			return reply, err
		case ModeAgent:
			return s.handleAgentQuestion(ctx, mc, question)
		case ModeDirect:
			fallthrough
		default:
			reply, err := s.aiService.Chat(ctx, question, s.conversationHistory(ctx, mc))
			if err == nil {
				s.appendConversationHistory(ctx, mc, question, reply)
			}
			return reply, err
		}
	default:
		return fmt.Sprintf("无法识别命令路由: %s", decision.Command), nil
	}
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
