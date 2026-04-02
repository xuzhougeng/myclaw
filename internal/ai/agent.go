package ai

import (
	"context"
	"fmt"
	"strings"
)

type AgentToolDefinition struct {
	Name              string
	FamilyKey         string
	FamilyTitle       string
	DisplayTitle      string
	DisplayOrder      int
	Provider          string
	ProviderKind      string
	Purpose           string
	Description       string
	InputContract     string
	OutputContract    string
	Usage             string
	InputJSONExample  string
	OutputJSONExample string
	SideEffectLevel   string
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

// appendToolListToPrompt writes the canonical tool-listing block into b.
func appendToolListToPrompt(b *strings.Builder, tools []AgentToolDefinition) {
	b.WriteString("可用工具：\n")
	for _, tool := range tools {
		b.WriteString("- ")
		b.WriteString(tool.Name)
		if family := strings.TrimSpace(tool.FamilyKey); family != "" {
			b.WriteString(" [family=")
			b.WriteString(family)
			if title := strings.TrimSpace(tool.FamilyTitle); title != "" {
				b.WriteString(", title=")
				b.WriteString(title)
			}
			b.WriteString("]")
		}
		if provider := strings.TrimSpace(tool.Provider); provider != "" {
			b.WriteString(" [provider=")
			b.WriteString(provider)
			if kind := strings.TrimSpace(tool.ProviderKind); kind != "" {
				b.WriteString(", kind=")
				b.WriteString(kind)
			}
			b.WriteString("]")
		}
		if level := strings.TrimSpace(tool.SideEffectLevel); level != "" {
			b.WriteString(" [side_effect=")
			b.WriteString(level)
			b.WriteString("]")
		}
		b.WriteString(": ")
		b.WriteString(strings.TrimSpace(tool.Description))
		if purpose := strings.TrimSpace(tool.Purpose); purpose != "" && purpose != strings.TrimSpace(tool.Description) {
			b.WriteString(" | purpose: ")
			b.WriteString(purpose)
		}
		if inputContract := strings.TrimSpace(tool.InputContract); inputContract != "" {
			b.WriteString(" | input contract: ")
			b.WriteString(inputContract)
		}
		if outputContract := strings.TrimSpace(tool.OutputContract); outputContract != "" {
			b.WriteString(" | output contract: ")
			b.WriteString(outputContract)
		}
		if usage := strings.TrimSpace(tool.Usage); usage != "" {
			b.WriteString(" | usage: ")
			b.WriteString(usage)
		}
		if example := strings.TrimSpace(tool.InputJSONExample); example != "" {
			b.WriteString(" | input json example: ")
			b.WriteString(example)
		}
		if example := strings.TrimSpace(tool.OutputJSONExample); example != "" {
			b.WriteString(" | output json example: ")
			b.WriteString(example)
		}
		b.WriteString("\n")
	}
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

	appendToolListToPrompt(&prompt, tools)

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
You are orchestrating tool use for baize's agent mode.
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
