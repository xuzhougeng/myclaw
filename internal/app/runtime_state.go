package app

import (
	"context"
	"os"
	"slices"
	"strconv"
	"strings"

	"myclaw/internal/ai"
	"myclaw/internal/sessionstate"
)

const (
	defaultConversationHistoryMessages       = 24
	defaultConversationHistoryRunes          = 2400
	defaultWeixinConversationHistoryMessages = 12
	defaultWeixinConversationHistoryRunes    = 360
	envConversationHistoryMessages           = "MYCLAW_HISTORY_MESSAGES"
	envConversationHistoryRunes              = "MYCLAW_HISTORY_RUNES"
	envWeixinHistoryMessages                 = "MYCLAW_WEIXIN_HISTORY_MESSAGES"
	envWeixinHistoryRunes                    = "MYCLAW_WEIXIN_HISTORY_RUNES"
)

type conversationHistoryLimits struct {
	Messages int
	Runes    int
}

func (s *Service) sessionSnapshot(ctx context.Context, mc MessageContext) (sessionstate.Snapshot, error) {
	return s.sessionSnapshotByKey(ctx, conversationSessionKey(mc))
}

func (s *Service) sessionSnapshotByKey(ctx context.Context, key string) (sessionstate.Snapshot, error) {
	snapshot := sessionstate.Snapshot{Key: strings.TrimSpace(key)}
	if s.sessionStore == nil || snapshot.Key == "" {
		return snapshot, nil
	}

	saved, ok, err := s.sessionStore.Load(ctx, snapshot.Key)
	if err != nil {
		return sessionstate.Snapshot{}, err
	}
	if ok {
		return saved, nil
	}
	return snapshot, nil
}

func (s *Service) saveSessionSnapshot(ctx context.Context, snapshot sessionstate.Snapshot) error {
	if s.sessionStore == nil {
		return nil
	}
	snapshot.Key = strings.TrimSpace(snapshot.Key)
	if snapshot.Key == "" {
		return nil
	}
	_, err := s.sessionStore.Save(ctx, snapshot)
	return err
}

func (s *Service) conversationHistory(ctx context.Context, mc MessageContext) []ai.ConversationMessage {
	snapshot, err := s.sessionSnapshot(ctx, mc)
	if err != nil {
		return nil
	}

	history := make([]ai.ConversationMessage, 0, len(snapshot.History))
	for _, item := range snapshot.History {
		content := strings.TrimSpace(item.Content)
		if strings.EqualFold(strings.TrimSpace(item.Role), "assistant") && strings.TrimSpace(item.ContextSummary) != "" {
			content = strings.TrimSpace(item.ContextSummary)
		}
		history = append(history, ai.ConversationMessage{
			Role:    item.Role,
			Content: content,
		})
	}
	return trimConversationHistory(history, s.conversationHistoryLimitsFor(mc))
}

func (s *Service) appendConversationHistory(ctx context.Context, mc MessageContext, userInput, assistantReply string) {
	s.appendConversationHistoryWithSummary(ctx, mc, userInput, assistantReply, turnSummaryFromContext(ctx))
}

func (s *Service) maybeAppendConversationHistory(ctx context.Context, mc MessageContext, userInput, assistantReply string) {
	if !conversationPersistenceEnabled(ctx) {
		return
	}
	s.appendConversationHistory(ctx, mc, userInput, assistantReply)
}

// appendConversationHistoryWithSummary persists a turn with an explicit final summary.
// If finalSummary is empty, falls back to assistantReply for ContextSummary.
func (s *Service) appendConversationHistoryWithSummary(ctx context.Context, mc MessageContext, userInput, assistantReply, finalSummary string) {
	snapshot, err := s.sessionSnapshot(ctx, mc)
	if err != nil {
		return
	}

	limits := s.conversationHistoryLimitsFor(mc)
	usage := ai.UsageFromContext(ctx)
	process := ai.CallTraceFromContext(ctx)
	history := append([]sessionstate.Message(nil), snapshot.History...)
	var assistantUsage *ai.TokenUsage
	if !usage.IsZero() {
		usageCopy := usage
		assistantUsage = &usageCopy
	}
	var assistantProcess []ai.CallTraceStep
	if len(process) > 0 {
		assistantProcess = append([]ai.CallTraceStep(nil), process...)
	}
	assistantContextSummary := strings.TrimSpace(finalSummary)
	if assistantContextSummary == "" {
		assistantContextSummary = assistantReply
	}
	history = append(history,
		sessionstate.Message{
			Role:    "user",
			Content: trimConversationHistoryText(userInput, limits.Runes),
		},
		sessionstate.Message{
			Role:           "assistant",
			Content:        trimConversationHistoryText(assistantReply, limits.Runes),
			ContextSummary: trimConversationHistoryText(assistantContextSummary, limits.Runes),
			Usage:          assistantUsage,
			Process:        assistantProcess,
		},
	)
	snapshot.History = trimSessionHistory(history, limits)
	_ = s.saveSessionSnapshot(ctx, snapshot)
}

func (s *Service) RecordConversationTurn(ctx context.Context, mc MessageContext, userInput, assistantReply string) {
	s.appendConversationHistory(ctx, mc, userInput, assistantReply)
}

func (s *Service) ConversationExists(ctx context.Context, mc MessageContext) (bool, error) {
	if s.sessionStore == nil {
		return false, nil
	}
	_, ok, err := s.sessionStore.Load(ctx, conversationSessionKey(mc))
	return ok, err
}

func (s *Service) EnsureConversation(ctx context.Context, mc MessageContext) error {
	if s.sessionStore == nil {
		return nil
	}

	key := conversationSessionKey(mc)
	if key == "" {
		return nil
	}
	if _, ok, err := s.sessionStore.Load(ctx, key); err != nil {
		return err
	} else if ok {
		return nil
	}
	return s.saveSessionSnapshot(ctx, sessionstate.Snapshot{Key: key})
}

func (s *Service) persistedLoadedSkillNames(mc MessageContext) []string {
	snapshot, err := s.sessionSnapshot(context.Background(), mc)
	if err != nil {
		return nil
	}
	out := append([]string(nil), snapshot.LoadedSkills...)
	slices.Sort(out)
	return out
}

func (s *Service) setPersistedLoadedSkillNames(mc MessageContext, names []string) {
	snapshot, err := s.sessionSnapshot(context.Background(), mc)
	if err != nil {
		return
	}
	snapshot.LoadedSkills = normalizeStringList(names)
	_ = s.saveSessionSnapshot(context.Background(), snapshot)
}

func (s *Service) selectedPromptID(ctx context.Context, mc MessageContext) string {
	snapshot, err := s.sessionSnapshot(ctx, mc)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(snapshot.PromptID)
}

func (s *Service) setSelectedPromptID(ctx context.Context, mc MessageContext, promptID string) error {
	snapshot, err := s.sessionSnapshot(ctx, mc)
	if err != nil {
		return err
	}
	snapshot.PromptID = strings.TrimSpace(promptID)
	return s.saveSessionSnapshot(ctx, snapshot)
}

func trimConversationHistory(history []ai.ConversationMessage, limits conversationHistoryLimits) []ai.ConversationMessage {
	history = ai.NormalizeConversationMessages(history)
	if limits.Messages <= 0 || len(history) <= limits.Messages {
		return history
	}
	return history[len(history)-limits.Messages:]
}

func trimSessionHistory(history []sessionstate.Message, limits conversationHistoryLimits) []sessionstate.Message {
	out := make([]sessionstate.Message, 0, len(history))
	for _, item := range history {
		role := strings.ToLower(strings.TrimSpace(item.Role))
		if role != "user" && role != "assistant" {
			continue
		}
		content := trimConversationHistoryText(item.Content, limits.Runes)
		if content == "" {
			continue
		}
		out = append(out, sessionstate.Message{
			Role:           role,
			Content:        content,
			ContextSummary: trimConversationHistoryText(item.ContextSummary, limits.Runes),
			Usage:          item.Usage,
			Process:        item.Process,
		})
	}
	if limits.Messages <= 0 || len(out) <= limits.Messages {
		return out
	}
	return out[len(out)-limits.Messages:]
}

func trimConversationHistoryText(text string, maxRunes int) string {
	text = strings.TrimSpace(text)
	if maxRunes <= 0 {
		return text
	}
	return preview(text, maxRunes)
}

func defaultWeixinConversationHistoryLimits() conversationHistoryLimits {
	return conversationHistoryLimits{
		Messages: envIntOrDefault(envWeixinHistoryMessages, defaultWeixinConversationHistoryMessages),
		Runes:    envIntOrDefault(envWeixinHistoryRunes, defaultWeixinConversationHistoryRunes),
	}
}

func defaultConversationHistoryLimits() conversationHistoryLimits {
	return conversationHistoryLimits{
		Messages: envIntOrDefault(envConversationHistoryMessages, defaultConversationHistoryMessages),
		Runes:    envIntOrDefault(envConversationHistoryRunes, defaultConversationHistoryRunes),
	}
}

func (s *Service) conversationHistoryLimitsFor(mc MessageContext) conversationHistoryLimits {
	if strings.EqualFold(strings.TrimSpace(mc.Interface), "weixin") {
		messages, runes := s.WeixinHistoryLimits()
		return conversationHistoryLimits{Messages: messages, Runes: runes}
	}
	return defaultConversationHistoryLimits()
}

func envIntOrDefault(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		return fallback
	}
	return parsed
}

func normalizeStringList(values []string) []string {
	var out []string
	seen := make(map[string]struct{})
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	slices.Sort(out)
	return out
}
