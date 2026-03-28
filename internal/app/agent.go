package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"myclaw/internal/ai"
	"myclaw/internal/knowledge"
	"myclaw/internal/reminder"
)

const maxAgentToolSteps = 3

type agentTool struct {
	Name             string
	Description      string
	InputJSONExample string
	Run              func(ctx context.Context, mc MessageContext, rawInput string) (string, error)
}

type agentToolRegistry struct {
	tools map[string]agentTool
	order []string
}

func newAgentToolRegistry(s *Service) agentToolRegistry {
	tools := []agentTool{
		{
			Name:             "knowledge_search",
			Description:      "Search the current project's knowledge base for relevant entries.",
			InputJSONExample: `{"query":"macOS 计划"}`,
			Run: func(ctx context.Context, _ MessageContext, rawInput string) (string, error) {
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
				results, err := s.searchCandidates(ctx, []string{args.Query}, nil, retrievalCandidateLimit)
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
			},
		},
		{
			Name:             "remember",
			Description:      "Save a new knowledge entry in the current project.",
			InputJSONExample: `{"text":"未来要支持 macOS"}`,
			Run: func(ctx context.Context, mc MessageContext, rawInput string) (string, error) {
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
				entry, err := s.store.Add(ctx, knowledge.Entry{
					Text:       args.Text,
					Source:     sourceLabel(mc),
					RecordedAt: time.Now(),
				})
				if err != nil {
					return "", err
				}
				return fmt.Sprintf("已记住 #%s\n%s", shortID(entry.ID), preview(entry.Text, maxReplyPreviewRunes)), nil
			},
		},
		{
			Name:             "append_knowledge",
			Description:      "Append content to an existing knowledge entry by ID or prefix.",
			InputJSONExample: `{"id":"6d2d7724","text":"补充说明"}`,
			Run: func(ctx context.Context, _ MessageContext, rawInput string) (string, error) {
				var args struct {
					ID   string `json:"id"`
					Text string `json:"text"`
				}
				if err := decodeAgentToolInput(rawInput, &args); err != nil {
					return "", err
				}
				return s.appendKnowledge(ctx, args.ID, args.Text)
			},
		},
		{
			Name:             "forget_knowledge",
			Description:      "Delete a knowledge entry by ID or prefix.",
			InputJSONExample: `{"id":"6d2d7724"}`,
			Run: func(ctx context.Context, _ MessageContext, rawInput string) (string, error) {
				var args struct {
					ID string `json:"id"`
				}
				if err := decodeAgentToolInput(rawInput, &args); err != nil {
					return "", err
				}
				return s.forgetKnowledge(ctx, args.ID)
			},
		},
		{
			Name:             "reminder_list",
			Description:      "List reminders for the current interface and user.",
			InputJSONExample: `{}`,
			Run: func(ctx context.Context, mc MessageContext, rawInput string) (string, error) {
				var args map[string]any
				if err := decodeAgentToolInput(rawInput, &args); err != nil {
					return "", err
				}
				if s.reminders == nil {
					return "提醒功能未启用。", nil
				}
				items, err := s.reminders.List(ctx, reminder.Target{Interface: mc.Interface, UserID: mc.UserID})
				if err != nil {
					return "", err
				}
				return formatReminderList(items), nil
			},
		},
		{
			Name:             "reminder_add",
			Description:      "Create a reminder using the same syntax as /notice.",
			InputJSONExample: `{"spec":"2小时后 喝水"}`,
			Run: func(ctx context.Context, mc MessageContext, rawInput string) (string, error) {
				var args struct {
					Spec string `json:"spec"`
				}
				if err := decodeAgentToolInput(rawInput, &args); err != nil {
					return "", err
				}
				if s.reminders == nil {
					return "提醒功能未启用。", nil
				}
				item, err := s.scheduleReminderSpec(ctx, reminder.Target{
					Interface: mc.Interface,
					UserID:    mc.UserID,
				}, args.Spec)
				if err != nil {
					return "", err
				}
				return formatReminderCreated(item), nil
			},
		},
		{
			Name:             "reminder_remove",
			Description:      "Delete a reminder by ID or prefix.",
			InputJSONExample: `{"id":"abcd1234"}`,
			Run: func(ctx context.Context, mc MessageContext, rawInput string) (string, error) {
				var args struct {
					ID string `json:"id"`
				}
				if err := decodeAgentToolInput(rawInput, &args); err != nil {
					return "", err
				}
				if s.reminders == nil {
					return "提醒功能未启用。", nil
				}
				item, ok, err := s.reminders.Remove(ctx, reminder.Target{Interface: mc.Interface, UserID: mc.UserID}, args.ID)
				if err != nil {
					return "", err
				}
				if !ok {
					return fmt.Sprintf("没有找到提醒 %q。", args.ID), nil
				}
				return fmt.Sprintf("已删除提醒 #%s\n内容: %s", shortID(item.ID), item.Message), nil
			},
		},
	}

	registry := agentToolRegistry{
		tools: make(map[string]agentTool, len(tools)),
		order: make([]string, 0, len(tools)),
	}
	for _, tool := range tools {
		key := strings.ToLower(strings.TrimSpace(tool.Name))
		registry.tools[key] = tool
		registry.order = append(registry.order, key)
	}
	return registry
}

func (r agentToolRegistry) Definitions() []ai.AgentToolDefinition {
	out := make([]ai.AgentToolDefinition, 0, len(r.order))
	for _, name := range r.order {
		tool := r.tools[name]
		out = append(out, ai.AgentToolDefinition{
			Name:             tool.Name,
			Description:      tool.Description,
			InputJSONExample: tool.InputJSONExample,
		})
	}
	return out
}

func (r agentToolRegistry) Execute(ctx context.Context, mc MessageContext, name, rawInput string) (string, error) {
	tool, ok := r.tools[strings.ToLower(strings.TrimSpace(name))]
	if !ok {
		return "", fmt.Errorf("unknown agent tool %q", name)
	}
	return tool.Run(ctx, mc, rawInput)
}

func (s *Service) handleAgentQuestion(ctx context.Context, mc MessageContext, question string) (string, error) {
	registry := newAgentToolRegistry(s)
	history := s.conversationHistory(ctx, mc)
	results := make([]ai.AgentToolResult, 0, maxAgentToolSteps)

	for step := 0; step < maxAgentToolSteps; step++ {
		decision, err := s.aiService.DecideAgentStep(ctx, question, history, registry.Definitions(), results)
		if err != nil {
			return "", err
		}

		switch decision.Action {
		case "answer":
			reply := strings.TrimSpace(decision.Answer)
			if reply == "" {
				return "", fmt.Errorf("agent returned empty answer")
			}
			s.appendConversationHistory(ctx, mc, question, reply)
			return reply, nil
		case "tool":
			output, err := registry.Execute(ctx, mc, decision.ToolName, decision.ToolInput)
			if err != nil {
				output = "工具执行失败: " + err.Error()
			}
			results = append(results, ai.AgentToolResult{
				ToolName:  strings.TrimSpace(decision.ToolName),
				ToolInput: strings.TrimSpace(decision.ToolInput),
				Output:    strings.TrimSpace(output),
			})
		default:
			return "", fmt.Errorf("unsupported agent action %q", decision.Action)
		}
	}

	return "", fmt.Errorf("agent reached the maximum tool step limit")
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
