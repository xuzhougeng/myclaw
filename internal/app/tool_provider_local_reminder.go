package app

import (
	"context"
	"fmt"

	"baize/internal/reminder"
)

func (p *localAgentToolProvider) reminderToolSet() localToolSet {
	return newLocalToolSet(
		reminder.AgentToolContracts(),
		map[string]ToolSideEffectLevel{
			reminder.ListToolName:   ToolSideEffectReadOnly,
			reminder.AddToolName:    ToolSideEffectSoftWrite,
			reminder.RemoveToolName: ToolSideEffectDestructive,
		},
		map[string]localToolHandler{
			reminder.ListToolName:   p.executeReminderList,
			reminder.AddToolName:    p.executeReminderAdd,
			reminder.RemoveToolName: p.executeReminderRemove,
		},
		nil,
	)
}

func (p *localAgentToolProvider) executeReminderList(ctx context.Context, mc MessageContext, rawInput string) (string, error) {
	if err := decodeAgentToolInput(rawInput, &struct{}{}); err != nil {
		return "", err
	}
	if p.service.reminders == nil {
		return "提醒功能未启用。", nil
	}
	items, err := p.service.ListVisibleReminders(ctx, mc)
	if err != nil {
		return "", err
	}
	return formatReminderListForContext(mc, items), nil
}

func (p *localAgentToolProvider) executeReminderAdd(ctx context.Context, mc MessageContext, rawInput string) (string, error) {
	var args struct {
		Spec string `json:"spec"`
	}
	if err := decodeAgentToolInput(rawInput, &args); err != nil {
		return "", err
	}
	if p.service.reminders == nil {
		return "提醒功能未启用。", nil
	}
	item, err := p.service.scheduleReminderSpec(ctx, reminder.Target{
		Interface: mc.Interface,
		UserID:    mc.UserID,
	}, args.Spec)
	if err != nil {
		return "", err
	}
	return formatReminderCreated(item), nil
}

func (p *localAgentToolProvider) executeReminderRemove(ctx context.Context, mc MessageContext, rawInput string) (string, error) {
	var args struct {
		ID string `json:"id"`
	}
	if err := decodeAgentToolInput(rawInput, &args); err != nil {
		return "", err
	}
	if p.service.reminders == nil {
		return "提醒功能未启用。", nil
	}
	item, ok, err := p.service.RemoveVisibleReminder(ctx, mc, args.ID)
	if err != nil {
		return "", err
	}
	if !ok {
		return fmt.Sprintf("没有找到提醒 %q。", args.ID), nil
	}
	return fmt.Sprintf("已删除提醒 #%s\n内容: %s", shortID(item.ID), item.Message), nil
}
