package ai

import (
	"context"
	"fmt"
	"strings"
)

func (s *Service) DetectToolOpportunities(ctx context.Context, task string, tools []ToolCapability) ([]ToolOpportunity, error) {
	cfg, err := s.requireConfig(ctx)
	if err != nil {
		return nil, err
	}
	if len(tools) == 0 {
		return nil, nil
	}

	schema := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"matches": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type":                 "object",
					"additionalProperties": false,
					"properties": map[string]any{
						"tool_name": map[string]any{
							"type": "string",
						},
						"goal": map[string]any{
							"type": "string",
						},
					},
					"required": []string{"tool_name", "goal"},
				},
			},
		},
		"required": []string{"matches"},
	}

	var prompt strings.Builder
	prompt.WriteString("用户任务：\n")
	prompt.WriteString(strings.TrimSpace(task))
	prompt.WriteString("\n\n可用工具：\n")
	for _, tool := range tools {
		name := strings.TrimSpace(tool.Name)
		if name == "" {
			continue
		}
		prompt.WriteString("- ")
		prompt.WriteString(name)
		prompt.WriteString(": ")
		prompt.WriteString(strings.TrimSpace(tool.Description))
		if purpose := strings.TrimSpace(tool.Purpose); purpose != "" && purpose != strings.TrimSpace(tool.Description) {
			prompt.WriteString(" | purpose: ")
			prompt.WriteString(purpose)
		}
		if inputContract := strings.TrimSpace(tool.InputContract); inputContract != "" {
			prompt.WriteString(" | input contract: ")
			prompt.WriteString(inputContract)
		}
		if outputContract := strings.TrimSpace(tool.OutputContract); outputContract != "" {
			prompt.WriteString(" | output contract: ")
			prompt.WriteString(outputContract)
		}
		if usage := strings.TrimSpace(tool.Usage); usage != "" {
			prompt.WriteString(" | usage: ")
			prompt.WriteString(usage)
		}
		if example := strings.TrimSpace(tool.InputJSONExample); example != "" {
			prompt.WriteString(" | input example: ")
			prompt.WriteString(example)
		}
		if example := strings.TrimSpace(tool.OutputJSONExample); example != "" {
			prompt.WriteString(" | output example: ")
			prompt.WriteString(example)
		}
		prompt.WriteString("\n")
	}

	instructions := strings.TrimSpace(`
You are the generic tool opportunity detector for baize.
Your job is:
1. understand the user's current need
2. decide whether any available tool should be considered
3. return zero or more candidate tools in best-first order

Rules:
- Only return tools that materially help complete the user's task.
- Prefer an empty list over a weak guess.
- tool_name must exactly match one available tool name.
- goal should be a short Chinese restatement of what the tool should accomplish.
- Do not invent tools.
- Return only JSON that matches the schema.
`)

	var payload struct {
		Matches []ToolOpportunity `json:"matches"`
	}
	if err := s.generateJSON(ctx, cfg, instructions, prompt.String(), "tool_opportunity_detection", schema, &payload); err != nil {
		return nil, err
	}

	allowed := make(map[string]struct{}, len(tools))
	for _, tool := range tools {
		name := strings.TrimSpace(tool.Name)
		if name == "" {
			continue
		}
		allowed[name] = struct{}{}
	}

	out := make([]ToolOpportunity, 0, len(payload.Matches))
	seen := make(map[string]struct{})
	for _, item := range payload.Matches {
		item.ToolName = strings.TrimSpace(item.ToolName)
		item.Goal = strings.TrimSpace(item.Goal)
		if item.ToolName == "" {
			continue
		}
		if _, ok := allowed[item.ToolName]; !ok {
			continue
		}
		if _, ok := seen[item.ToolName]; ok {
			continue
		}
		seen[item.ToolName] = struct{}{}
		out = append(out, item)
	}
	return out, nil
}

func (s *Service) PlanToolUse(ctx context.Context, task string, tool ToolCapability, prior []ToolExecution) (ToolPlanDecision, error) {
	cfg, err := s.requireConfig(ctx)
	if err != nil {
		return ToolPlanDecision{}, err
	}
	tool.Name = strings.TrimSpace(tool.Name)
	if tool.Name == "" {
		return ToolPlanDecision{}, fmt.Errorf("tool capability is missing name")
	}

	schema := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"action": map[string]any{
				"type": "string",
				"enum": []string{"tool", "stop"},
			},
			"tool_name": map[string]any{
				"type": "string",
			},
			"tool_input": map[string]any{
				"type": "string",
			},
			"user_message": map[string]any{
				"type": "string",
			},
		},
		"required": []string{"action", "tool_name", "tool_input", "user_message"},
	}

	var prompt strings.Builder
	prompt.WriteString("用户任务：\n")
	prompt.WriteString(strings.TrimSpace(task))
	prompt.WriteString("\n\n当前工具：\n")
	prompt.WriteString("name: ")
	prompt.WriteString(tool.Name)
	if purpose := strings.TrimSpace(tool.Purpose); purpose != "" {
		prompt.WriteString("\npurpose: ")
		prompt.WriteString(purpose)
	}
	prompt.WriteString("\ndescription: ")
	prompt.WriteString(strings.TrimSpace(tool.Description))
	if inputContract := strings.TrimSpace(tool.InputContract); inputContract != "" {
		prompt.WriteString("\ninput contract: ")
		prompt.WriteString(inputContract)
	}
	if outputContract := strings.TrimSpace(tool.OutputContract); outputContract != "" {
		prompt.WriteString("\noutput contract: ")
		prompt.WriteString(outputContract)
	}
	if usage := strings.TrimSpace(tool.Usage); usage != "" {
		prompt.WriteString("\nusage:\n")
		prompt.WriteString(usage)
	}
	if example := strings.TrimSpace(tool.InputJSONExample); example != "" {
		prompt.WriteString("\ninput example: ")
		prompt.WriteString(example)
	}
	if example := strings.TrimSpace(tool.OutputJSONExample); example != "" {
		prompt.WriteString("\noutput example: ")
		prompt.WriteString(example)
	}

	if len(prior) > 0 {
		prompt.WriteString("\n\n已有执行结果：\n")
		for index, item := range prior {
			prompt.WriteString(fmt.Sprintf("%d. tool=%s\ninput=%s\noutput=%s\n\n",
				index+1,
				strings.TrimSpace(item.ToolName),
				strings.TrimSpace(item.ToolInput),
				strings.TrimSpace(item.ToolOutput),
			))
		}
	}

	instructions := strings.TrimSpace(`
You are the generic tool planner for baize.
For the selected tool, decide the next step in this loop:
1. understand the user's goal
2. use the tool's contract and usage notes
3. if there are prior executions, refine based on those results
4. either produce the next tool call or stop

Rules:
- action=tool means call the current tool once more.
- action=stop means no more useful tool call should be made.
- tool_name must be empty or equal to the current tool name; prefer returning the exact current tool name when action=tool.
- tool_input must be a JSON object string such as {"query":"macOS"}.
- If prior results were empty, broaden or simplify constraints when that is reasonable.
- Do not repeat the exact same ineffective call unless there is a strong reason.
- user_message should be a short Chinese explanation only when action=stop or when the next action needs clarification.
- Return only JSON that matches the schema.
`)

	var decision ToolPlanDecision
	if err := s.generateJSON(ctx, cfg, instructions, prompt.String(), "tool_use_plan", schema, &decision); err != nil {
		return ToolPlanDecision{}, err
	}

	decision.Action = strings.TrimSpace(strings.ToLower(decision.Action))
	decision.ToolName = strings.TrimSpace(decision.ToolName)
	decision.ToolInput = strings.TrimSpace(decision.ToolInput)
	decision.UserMessage = strings.TrimSpace(decision.UserMessage)
	if decision.Action == "" {
		return ToolPlanDecision{}, fmt.Errorf("model returned empty tool plan action")
	}
	if decision.Action == "tool" && decision.ToolName == "" {
		decision.ToolName = tool.Name
	}
	return decision, nil
}
