package ai

import (
	"context"
	"fmt"
	"strings"
)

type LoopAction string

const (
	LoopContinue LoopAction = "continue"
	LoopAnswer   LoopAction = "answer"
	LoopAsk      LoopAction = "ask"
	LoopStop     LoopAction = "stop"
)

type LoopDecision struct {
	Action    LoopAction
	ToolName  string
	ToolInput string
	Answer    string
	Question  string
	Reason    string
}

type ToolAttempt struct {
	ToolName      string
	ToolInput     string
	RawOutput     string
	OutputSummary string
	Succeeded     bool
	FailureReason string
}

type AgentTaskState struct {
	Goal            string
	UserInput       string
	HistorySummary  string
	WorkingSummary  string
	FinalSummary    string
	Observations    []string
	ToolAttempts    []ToolAttempt
	CandidateTools  []AgentToolDefinition
	PendingQuestion string
	LastAnswerDraft string
	SideEffects     []string
}

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
		summary := strings.TrimSpace(attempt.OutputSummary)
		if summary == "" {
			summary = strings.TrimSpace(attempt.RawOutput)
		}
		fmt.Fprintf(b, "%d. tool=%s input=%s [%s]\n",
			index+1,
			strings.TrimSpace(attempt.ToolName),
			strings.TrimSpace(attempt.ToolInput),
			status,
		)
		if attempt.Succeeded && summary != "" {
			b.WriteString("   输出摘要: ")
			b.WriteString(summary)
			b.WriteString("\n")
		}
		if !attempt.Succeeded && strings.TrimSpace(attempt.FailureReason) != "" {
			b.WriteString("   失败原因: ")
			b.WriteString(strings.TrimSpace(attempt.FailureReason))
			b.WriteString("\n")
		}
	}
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
你是 myclaw 代理模式的循环调度器。
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
你是 myclaw 代理工作状态摘要生成器。
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
你是 myclaw 代理最终状态摘要生成器。
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
