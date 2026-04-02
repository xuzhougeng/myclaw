package app

import (
	"context"
	"fmt"
	"strings"

	"baize/internal/ai"
	"baize/internal/reminder"
)

const reminderChatSummaryLimit = 6

func (s *Service) ListVisibleReminders(ctx context.Context, mc MessageContext) ([]reminder.Reminder, error) {
	if s.reminders == nil {
		return nil, nil
	}
	if usesGlobalReminderScope(mc) {
		return s.reminders.ListAll(ctx)
	}
	return s.reminders.List(ctx, reminderTargetForContext(mc))
}

func (s *Service) RemoveVisibleReminder(ctx context.Context, mc MessageContext, idOrPrefix string) (reminder.Reminder, bool, error) {
	if s.reminders == nil {
		return reminder.Reminder{}, false, nil
	}
	if usesGlobalReminderScope(mc) {
		return s.reminders.RemoveAny(ctx, idOrPrefix)
	}
	return s.reminders.Remove(ctx, reminderTargetForContext(mc), idOrPrefix)
}

func (s *Service) chatHistoryWithRuntimeState(ctx context.Context, mc MessageContext) []ai.ConversationMessage {
	history := s.conversationHistory(ctx, mc)
	summary := s.reminderRuntimeSummary(ctx, mc)
	if summary == "" {
		return history
	}
	return append(history, ai.ConversationMessage{
		Role:    "assistant",
		Content: summary,
	})
}

func (s *Service) reminderRuntimeSummary(ctx context.Context, mc MessageContext) string {
	items, err := s.ListVisibleReminders(ctx, mc)
	if err != nil || len(items) == 0 {
		return ""
	}
	return formatReminderRuntimeSummary(items, usesGlobalReminderScope(mc))
}

func reminderTargetForContext(mc MessageContext) reminder.Target {
	return reminder.Target{
		Interface: strings.TrimSpace(mc.Interface),
		UserID:    strings.TrimSpace(mc.UserID),
	}
}

func usesGlobalReminderScope(mc MessageContext) bool {
	return strings.EqualFold(strings.TrimSpace(mc.Interface), "desktop") &&
		strings.EqualFold(strings.TrimSpace(mc.UserID), "primary")
}

func formatReminderListForContext(mc MessageContext, items []reminder.Reminder) string {
	if usesGlobalReminderScope(mc) {
		return formatReminderListWithSources(items)
	}
	return formatReminderList(items)
}

func formatReminderRuntimeSummary(items []reminder.Reminder, includeSource bool) string {
	if len(items) == 0 {
		return ""
	}

	limit := min(len(items), reminderChatSummaryLimit)
	var builder strings.Builder
	builder.WriteString("系统同步的当前提醒摘要：\n")
	for index := 0; index < limit; index++ {
		builder.WriteString(formatReminderLine(index+1, items[index], includeSource))
		builder.WriteByte('\n')
	}
	if len(items) > limit {
		builder.WriteString(fmt.Sprintf("... 另有 %d 条提醒未展开。\n", len(items)-limit))
	}
	builder.WriteString("仅当这些提醒与当前问题相关时再使用它们。")
	return builder.String()
}

func formatReminderListWithSources(items []reminder.Reminder) string {
	return formatReminderListInternal(items, true)
}

func formatReminderListInternal(items []reminder.Reminder, includeSource bool) string {
	if len(items) == 0 {
		return "当前没有提醒。"
	}
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("当前提醒（共 %d 条）:\n", len(items)))
	for index, item := range items {
		builder.WriteString(formatReminderLine(index+1, item, includeSource))
		builder.WriteByte('\n')
	}
	return strings.TrimSpace(builder.String())
}

func formatReminderLine(index int, item reminder.Reminder, includeSource bool) string {
	schedule := reminderScheduleLabel(item)
	if includeSource {
		return fmt.Sprintf("%d. #%s [%s] %s (来源: %s)",
			index,
			shortID(item.ID),
			schedule,
			item.Message,
			reminderSourceLabel(item.Target),
		)
	}
	return fmt.Sprintf("%d. #%s [%s] %s",
		index,
		shortID(item.ID),
		schedule,
		item.Message,
	)
}

func reminderScheduleLabel(item reminder.Reminder) string {
	if item.Frequency == reminder.FrequencyDaily {
		return fmt.Sprintf("每天 %02d:%02d", item.DailyHour, item.DailyMinute)
	}
	return item.NextRunAt.Local().Format("2006-01-02 15:04:05")
}

func reminderSourceLabel(target reminder.Target) string {
	name := strings.TrimSpace(target.Interface)
	userID := strings.TrimSpace(target.UserID)
	switch {
	case strings.EqualFold(name, "weixin"):
		return "微信"
	case strings.EqualFold(name, "desktop"):
		return "桌面"
	case userID != "":
		return strings.TrimSpace(name + " · " + userID)
	default:
		return name
	}
}
