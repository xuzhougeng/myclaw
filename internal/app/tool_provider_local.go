package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"myclaw/internal/knowledge"
	"myclaw/internal/reminder"
)

type localAgentToolProvider struct {
	service *Service
}

func newLocalAgentToolProvider(service *Service) AgentToolProvider {
	return &localAgentToolProvider{service: service}
}

func (p *localAgentToolProvider) ProviderKind() AgentToolProviderKind {
	return AgentToolProviderLocal
}

func (p *localAgentToolProvider) ProviderKey() string {
	return string(AgentToolProviderLocal)
}

func (p *localAgentToolProvider) ListAgentTools(context.Context, MessageContext) ([]AgentToolSpec, error) {
	return []AgentToolSpec{
		{
			Name:             "knowledge_search",
			Description:      "Search the current project's knowledge base for relevant entries.",
			InputJSONExample: `{"query":"macOS 计划"}`,
		},
		{
			Name:             "remember",
			Description:      "Save a new knowledge entry in the current project.",
			InputJSONExample: `{"text":"未来要支持 macOS"}`,
		},
		{
			Name:             "append_knowledge",
			Description:      "Append content to an existing knowledge entry by ID or prefix.",
			InputJSONExample: `{"id":"6d2d7724","text":"补充说明"}`,
		},
		{
			Name:             "forget_knowledge",
			Description:      "Delete a knowledge entry by ID or prefix.",
			InputJSONExample: `{"id":"6d2d7724"}`,
		},
		{
			Name:             "reminder_list",
			Description:      "List reminders for the current interface and user.",
			InputJSONExample: `{}`,
		},
		{
			Name:             "reminder_add",
			Description:      "Create a reminder using the same syntax as /notice.",
			InputJSONExample: `{"spec":"2小时后 喝水"}`,
		},
		{
			Name:             "reminder_remove",
			Description:      "Delete a reminder by ID or prefix.",
			InputJSONExample: `{"id":"abcd1234"}`,
		},
	}, nil
}

func (p *localAgentToolProvider) ExecuteAgentTool(ctx context.Context, mc MessageContext, toolName, rawInput string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(toolName)) {
	case "knowledge_search":
		var args struct {
			Query string `json:"query"`
		}
		if err := decodeAgentToolInput(rawInput, &args); err != nil {
			return "", err
		}
		args.Query = strings.TrimSpace(args.Query)
		if args.Query == "" {
			return "", fmt.Errorf("knowledge_search requires query")
		}
		results, err := p.service.searchCandidates(ctx, []string{args.Query}, nil, retrievalCandidateLimit)
		if err != nil {
			return "", err
		}
		if len(results) == 0 {
			return "没有命中的知识。", nil
		}
		var builder strings.Builder
		builder.WriteString(fmt.Sprintf("命中 %d 条知识：\n", len(results)))
		for index, result := range results {
			builder.WriteString(fmt.Sprintf("%d. #%s score=%d %s\n",
				index+1,
				shortID(result.Entry.ID),
				result.Score,
				preview(result.Entry.Text, maxReplyPreviewRunes),
			))
		}
		return strings.TrimSpace(builder.String()), nil
	case "remember":
		var args struct {
			Text string `json:"text"`
		}
		if err := decodeAgentToolInput(rawInput, &args); err != nil {
			return "", err
		}
		args.Text = strings.TrimSpace(args.Text)
		if args.Text == "" {
			return "", fmt.Errorf("remember requires text")
		}
		entry, err := p.service.store.Add(ctx, knowledge.Entry{
			Text:       args.Text,
			Source:     sourceLabel(mc),
			RecordedAt: time.Now(),
		})
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("已记住 #%s\n%s", shortID(entry.ID), preview(entry.Text, maxReplyPreviewRunes)), nil
	case "append_knowledge":
		var args struct {
			ID   string `json:"id"`
			Text string `json:"text"`
		}
		if err := decodeAgentToolInput(rawInput, &args); err != nil {
			return "", err
		}
		return p.service.appendKnowledge(ctx, args.ID, args.Text)
	case "forget_knowledge":
		var args struct {
			ID string `json:"id"`
		}
		if err := decodeAgentToolInput(rawInput, &args); err != nil {
			return "", err
		}
		return p.service.forgetKnowledge(ctx, args.ID)
	case "reminder_list":
		var args map[string]any
		if err := decodeAgentToolInput(rawInput, &args); err != nil {
			return "", err
		}
		if p.service.reminders == nil {
			return "提醒功能未启用。", nil
		}
		items, err := p.service.reminders.List(ctx, reminder.Target{Interface: mc.Interface, UserID: mc.UserID})
		if err != nil {
			return "", err
		}
		return formatReminderList(items), nil
	case "reminder_add":
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
	case "reminder_remove":
		var args struct {
			ID string `json:"id"`
		}
		if err := decodeAgentToolInput(rawInput, &args); err != nil {
			return "", err
		}
		if p.service.reminders == nil {
			return "提醒功能未启用。", nil
		}
		item, ok, err := p.service.reminders.Remove(ctx, reminder.Target{Interface: mc.Interface, UserID: mc.UserID}, args.ID)
		if err != nil {
			return "", err
		}
		if !ok {
			return fmt.Sprintf("没有找到提醒 %q。", args.ID), nil
		}
		return fmt.Sprintf("已删除提醒 #%s\n内容: %s", shortID(item.ID), item.Message), nil
	default:
		return "", fmt.Errorf("unknown local tool %q", toolName)
	}
}

func decodeAgentToolInput(raw string, out any) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		raw = "{}"
	}
	if !strings.HasPrefix(raw, "{") {
		return fmt.Errorf("tool input must be a JSON object")
	}
	if err := json.Unmarshal([]byte(raw), out); err != nil {
		return fmt.Errorf("decode tool input: %w", err)
	}
	return nil
}
