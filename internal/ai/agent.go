package ai

import (
	"context"
	"fmt"
	"strings"
)

type AgentToolDefinition struct {
	Name             string
	Provider         string
	ProviderKind     string
	Description      string
	InputJSONExample string
}

type AgentToolResult struct {
	ToolName  string
	ToolInput string
	Output    string
}

type AgentStepDecision struct {
	Action    string `json:"action"`
	Answer    string `json:"answer"`
	ToolName  string `json:"tool_name"`
	ToolInput string `json:"tool_input"`
}

func (s *Service) DecideAgentStep(ctx context.Context, task string, history []ConversationMessage, tools []AgentToolDefinition, results []AgentToolResult) (AgentStepDecision, error) {
	cfg, err := s.requireConfig(ctx)
	if err != nil {
		return AgentStepDecision{}, err
	}

	schema := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"action": map[string]any{
				"type": "string",
				"enum": []string{"answer", "tool"},
			},
			"answer": map[string]any{
				"type": "string",
			},
			"tool_name": map[string]any{
				"type": "string",
			},
			"tool_input": map[string]any{
				"type": "string",
			},
		},
		"required": []string{"action", "answer", "tool_name", "tool_input"},
	}

	var prompt strings.Builder
	prompt.WriteString("用户任务：\n")
	prompt.WriteString(strings.TrimSpace(task))
	prompt.WriteString("\n\n")

	if normalizedHistory := NormalizeConversationMessages(history); len(normalizedHistory) > 0 {
		prompt.WriteString("对话历史：\n")
		for _, item := range normalizedHistory {
			prompt.WriteString("- ")
			prompt.WriteString(item.Role)
			prompt.WriteString(": ")
			prompt.WriteString(item.Content)
			prompt.WriteString("\n")
		}
		prompt.WriteString("\n")
	}

	prompt.WriteString("可用工具：\n")
	for _, tool := range tools {
		prompt.WriteString("- ")
		prompt.WriteString(tool.Name)
		if provider := strings.TrimSpace(tool.Provider); provider != "" {
			prompt.WriteString(" [provider=")
			prompt.WriteString(provider)
			if kind := strings.TrimSpace(tool.ProviderKind); kind != "" {
				prompt.WriteString(", kind=")
				prompt.WriteString(kind)
			}
			prompt.WriteString("]")
		}
		prompt.WriteString(": ")
		prompt.WriteString(strings.TrimSpace(tool.Description))
		if example := strings.TrimSpace(tool.InputJSONExample); example != "" {
			prompt.WriteString(" | input json example: ")
			prompt.WriteString(example)
		}
		prompt.WriteString("\n")
	}

	if len(results) > 0 {
		prompt.WriteString("\n已经执行过的工具结果：\n")
		for index, result := range results {
			prompt.WriteString(fmt.Sprintf("%d. tool=%s input=%s\n%s\n\n",
				index+1,
				strings.TrimSpace(result.ToolName),
				strings.TrimSpace(result.ToolInput),
				strings.TrimSpace(result.Output),
			))
		}
	}

	instructions := strings.TrimSpace(`
You are orchestrating tool use for myclaw's agent mode.
At each step, decide exactly one of:
- answer: provide the final assistant reply to the user
- tool: call exactly one available tool

Rules:
- Use tools when the user is asking to inspect or modify local state, and when tool output is needed before answering.
- If prior tool results already contain enough information, choose answer.
- tool_name must match one of the available tools exactly, including any provider prefix such as local::search or mcp.docs::lookup.
- tool_input must be a JSON object string such as {"query":"macOS"}.
- If a tool does not need arguments, return {}.
- Keep answers concise and practical.
- Respond only with JSON that matches the schema.
`)

	var decision AgentStepDecision
	if err := s.generateJSON(ctx, cfg, instructions, prompt.String(), "agent_step", schema, &decision); err != nil {
		return AgentStepDecision{}, err
	}

	decision.Action = strings.TrimSpace(strings.ToLower(decision.Action))
	decision.Answer = strings.TrimSpace(decision.Answer)
	decision.ToolName = strings.TrimSpace(decision.ToolName)
	decision.ToolInput = strings.TrimSpace(decision.ToolInput)
	if decision.Action == "" {
		return AgentStepDecision{}, fmt.Errorf("model returned empty agent action")
	}
	return decision, nil
}
