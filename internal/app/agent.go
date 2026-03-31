package app

import (
	"context"
	"fmt"
	"strings"

	"myclaw/internal/ai"
)

const maxAgentToolSteps = 3

func (s *Service) handleAgentQuestion(ctx context.Context, mc MessageContext, question string) (string, error) {
	if s.toolProviders == nil {
		return "", fmt.Errorf("agent tool providers are not configured")
	}

	history := s.conversationHistory(ctx, mc)
	results := make([]ai.AgentToolResult, 0, maxAgentToolSteps)

	for step := 0; step < maxAgentToolSteps; step++ {
		definitions, err := s.toolProviders.Definitions(ctx, mc)
		if err != nil {
			return "", err
		}

		decision, err := s.aiService.DecideAgentStep(ctx, question, history, definitions, results)
		if err != nil {
			return "", err
		}

		switch decision.Action {
		case "answer":
			reply := strings.TrimSpace(decision.Answer)
			if reply == "" {
				return "", fmt.Errorf("agent returned empty answer")
			}
			addProcessTrace(ctx, fmt.Sprintf("Agent 决策 %d", step+1), "action=answer\nreply="+preview(reply, maxReplyPreviewRunes))
			s.maybeAppendConversationHistory(ctx, mc, question, reply)
			return reply, nil
		case "tool":
			addProcessTrace(ctx, fmt.Sprintf("Agent 决策 %d", step+1), "action=tool\ntool="+strings.TrimSpace(decision.ToolName)+"\ninput="+strings.TrimSpace(decision.ToolInput))
			output, err := s.toolProviders.Execute(ctx, mc, decision.ToolName, decision.ToolInput)
			if err != nil {
				output = "工具执行失败: " + err.Error()
			}
			addProcessTrace(ctx, fmt.Sprintf("Agent 工具结果 %d", step+1), preview(output, maxReplyPreviewRunes))
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
