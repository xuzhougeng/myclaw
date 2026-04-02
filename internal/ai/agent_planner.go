package ai

import (
	"context"
	"fmt"
	"strings"
)

const (
	maxPlannerAttemptRawLines = 8
	maxPlannerAttemptRawRunes = 600
)

type loopDecisionRaw struct {
	Action    string `json:"action"`
	ToolName  string `json:"tool_name"`
	ToolInput string `json:"tool_input"`
	Answer    string `json:"answer"`
	Question  string `json:"question"`
	Reason    string `json:"reason"`
}

var validLoopActions = map[string]LoopAction{
	"continue": LoopContinue,
	"answer":   LoopAnswer,
	"ask":      LoopAsk,
	"stop":     LoopStop,
}

// appendAttemptListToPrompt writes the numbered tool-attempt history block into b.
// Planner prompts are summary-first: only fall back to trimmed raw output when no summary exists.
func appendAttemptListToPrompt(b *strings.Builder, attempts []ToolAttempt) {
	if len(attempts) == 0 {
		return
	}
	b.WriteString("\n已执行的工具调用：\n")
	for index, attempt := range attempts {
		status := "成功"
		if !attempt.Succeeded {
			status = "失败"
		}
		summary := plannerFacingAttemptSummary(attempt)
		fmt.Fprintf(b, "%d. tool=%s input=%s [%s]\n",
			index+1,
			strings.TrimSpace(attempt.ToolName),
			strings.TrimSpace(attempt.ToolInput),
			status,
		)
		if !attempt.Succeeded && summary != "" {
			b.WriteString("   失败原因: ")
			b.WriteString(summary)
			b.WriteString("\n")
		}
		if attempt.Succeeded && summary != "" {
			b.WriteString("   结果摘要: ")
			b.WriteString(summary)
			b.WriteString("\n")
		}
	}
}

func plannerFacingAttemptSummary(attempt ToolAttempt) string {
	if !attempt.Succeeded {
		if reason := strings.TrimSpace(attempt.FailureReason); reason != "" {
			return reason
		}
	}
	if summary := strings.TrimSpace(attempt.OutputSummary); summary != "" {
		return summary
	}
	return truncatePlannerAttemptRawOutput(attempt.RawOutput)
}

func truncatePlannerAttemptRawOutput(raw string) string {
	raw = strings.TrimSpace(strings.ReplaceAll(raw, "\r\n", "\n"))
	if raw == "" {
		return ""
	}

	lines := strings.Split(raw, "\n")
	truncated := false
	if len(lines) > maxPlannerAttemptRawLines {
		lines = lines[:maxPlannerAttemptRawLines]
		truncated = true
	}
	raw = strings.TrimSpace(strings.Join(lines, "\n"))
	if raw == "" {
		return ""
	}

	if len([]rune(raw)) > maxPlannerAttemptRawRunes {
		raw = previewRunes(raw, maxPlannerAttemptRawRunes)
		truncated = true
	}
	if truncated {
		raw = strings.TrimSpace(raw) + "\n[truncated]"
	}
	return raw
}

func previewRunes(text string, limit int) string {
	text = strings.TrimSpace(text)
	if text == "" || limit <= 0 {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit])
}

func (s *Service) PlanAgentLoopStep(ctx context.Context, history []ConversationMessage, state AgentTaskState) (LoopDecision, error) {
	cfg, err := s.requireConfig(ctx)
	if err != nil {
		return LoopDecision{}, err
	}

	schema := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"action":     map[string]any{"type": "string", "enum": []string{"continue", "answer", "ask", "stop"}},
			"tool_name":  map[string]any{"type": "string"},
			"tool_input": map[string]any{"type": "string"},
			"answer":     map[string]any{"type": "string"},
			"question":   map[string]any{"type": "string"},
			"reason":     map[string]any{"type": "string"},
		},
		"required": []string{"action", "tool_name", "tool_input", "answer", "question", "reason"},
	}

	var prompt strings.Builder

	prompt.WriteString("用户目标：\n")
	prompt.WriteString(strings.TrimSpace(state.Goal))
	prompt.WriteString("\n\n")

	if input := strings.TrimSpace(state.UserInput); input != "" {
		prompt.WriteString("用户原始输入：\n")
		prompt.WriteString(input)
		prompt.WriteString("\n\n")
	}

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

	if summary := strings.TrimSpace(state.WorkingSummary); summary != "" {
		prompt.WriteString("当前进展摘要：\n")
		prompt.WriteString(summary)
		prompt.WriteString("\n\n")
	}

	appendToolListToPrompt(&prompt, state.CandidateTools)
	appendAttemptListToPrompt(&prompt, state.ToolAttempts)

	instructions := strings.TrimSpace(`
你是 baize 代理模式的循环调度器。
每次决策恰好选择以下四个动作之一：
- continue: 调用一个可用工具，继续收集信息或执行操作
- answer: 已有足够信息，输出最终答复给用户
- ask: 需要向用户确认或询问以消除歧义
- stop: 无法继续，停止并说明原因

工具调用规则：
- side_effect=read_only 的工具可以自由调用
- side_effect=soft_write 的工具需要对用户意图有较强把握才可调用
- side_effect=destructive 的工具仅在用户意图绝对明确时才可调用，否则使用 ask 向用户确认
- tool_name 必须与可用工具列表中的名称完全一致，包含前缀如 local:: 或 mcp.xxx::
- tool_input 必须是 JSON 对象字符串，如 {"query":"macOS"}；无参数时返回 {}

决策规则：
- 已有足够信息时选择 answer
- 陷入困境或存在歧义时选择 ask 或 stop
- 仅返回符合 schema 的 JSON

Respond only with JSON that matches the schema.
`)

	var raw loopDecisionRaw
	if err := s.generateJSON(ctx, cfg, instructions, prompt.String(), "agent_loop_step", schema, &raw); err != nil {
		return LoopDecision{}, err
	}

	raw.Action = strings.TrimSpace(strings.ToLower(raw.Action))
	action, ok := validLoopActions[raw.Action]
	if !ok {
		return LoopDecision{}, fmt.Errorf("model returned invalid loop action %q", raw.Action)
	}

	return LoopDecision{
		Action:    action,
		ToolName:  strings.TrimSpace(raw.ToolName),
		ToolInput: strings.TrimSpace(raw.ToolInput),
		Answer:    strings.TrimSpace(raw.Answer),
		Question:  strings.TrimSpace(raw.Question),
		Reason:    strings.TrimSpace(raw.Reason),
	}, nil
}

func (s *Service) SummarizeAgentWorkingState(ctx context.Context, state AgentTaskState) (string, error) {
	if len(state.ToolAttempts) == 0 {
		return "", nil
	}

	cfg, err := s.requireConfig(ctx)
	if err != nil {
		return "", err
	}

	var prompt strings.Builder
	prompt.WriteString("目标：\n")
	prompt.WriteString(strings.TrimSpace(state.Goal))
	appendAttemptListToPrompt(&prompt, state.ToolAttempts)

	instructions := strings.TrimSpace(`
你是 baize 代理工作状态摘要生成器。
用 2-3 句简洁的中文描述目前已找到或已完成的内容。
不要重复工具名称列表，聚焦于关键发现和进展。
`)

	return s.generateText(ctx, cfg, instructions, prompt.String())
}

func (s *Service) SummarizeAgentFinalState(ctx context.Context, state AgentTaskState, finalAnswer string) (string, error) {
	cfg, err := s.requireConfig(ctx)
	if err != nil {
		return finalAnswer, nil
	}

	var prompt strings.Builder
	prompt.WriteString("目标：\n")
	prompt.WriteString(strings.TrimSpace(state.Goal))
	prompt.WriteString("\n\n最终答复：\n")
	prompt.WriteString(strings.TrimSpace(finalAnswer))

	if len(state.ToolAttempts) > 0 {
		prompt.WriteString("\n\n使用的工具：\n")
		for index, attempt := range state.ToolAttempts {
			status := "成功"
			if !attempt.Succeeded {
				status = "失败"
			}
			fmt.Fprintf(&prompt, "%d. %s [%s]\n", index+1, strings.TrimSpace(attempt.ToolName), status)
		}
	}

	if len(state.SideEffects) > 0 {
		prompt.WriteString("\n产生的副作用：\n")
		for _, effect := range state.SideEffects {
			prompt.WriteString("- ")
			prompt.WriteString(strings.TrimSpace(effect))
			prompt.WriteString("\n")
		}
	}

	instructions := strings.TrimSpace(`
你是 baize 代理最终状态摘要生成器。
生成一段简洁的中文摘要，供对话历史存档使用。
涵盖：完成了什么任务、关键发现、执行了哪些有副作用的操作（若有）。
保持简洁，不超过 5 句话。
`)

	result, err := s.generateText(ctx, cfg, instructions, prompt.String())
	if err != nil {
		return finalAnswer, nil
	}
	result = strings.TrimSpace(result)
	if result == "" {
		return finalAnswer, nil
	}
	return result, nil
}

// PlanNext implements AgentPlanner by delegating to PlanAgentLoopStep.
func (s *Service) PlanNext(ctx context.Context, task string, history []ConversationMessage, tools []AgentToolDefinition, state AgentTaskState) (LoopDecision, error) {
	if state.Goal == "" {
		state.Goal = task
	}
	if state.UserInput == "" {
		state.UserInput = task
	}
	if len(state.CandidateTools) == 0 {
		state.CandidateTools = tools
	}
	return s.PlanAgentLoopStep(ctx, history, state)
}

// SummarizeWorkingState implements AgentPlanner by delegating to SummarizeAgentWorkingState.
func (s *Service) SummarizeWorkingState(ctx context.Context, state AgentTaskState) (string, error) {
	return s.SummarizeAgentWorkingState(ctx, state)
}

// SummarizeFinal implements AgentPlanner by delegating to SummarizeAgentFinalState.
func (s *Service) SummarizeFinal(ctx context.Context, state AgentTaskState, finalAnswer string) (string, error) {
	return s.SummarizeAgentFinalState(ctx, state, finalAnswer)
}
