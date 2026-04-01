package app

import (
	"context"
	"fmt"

	"myclaw/internal/ai"
)

const maxAgentToolSteps = 3

// serviceAgentRuntime implements ai.AgentLoopRuntime using the Service.
type serviceAgentRuntime struct {
	service *Service
}

func (r *serviceAgentRuntime) LoadHistory(ctx context.Context, mc any) []ai.ConversationMessage {
	return r.service.conversationHistory(ctx, mc.(MessageContext))
}

func (r *serviceAgentRuntime) ListTools(ctx context.Context, mc any) ([]ai.AgentToolDefinition, error) {
	return r.service.toolProviders.Definitions(ctx, mc.(MessageContext))
}

func (r *serviceAgentRuntime) ExecuteTool(ctx context.Context, mc any, toolName, toolInput string) (string, error) {
	output, err := r.service.toolProviders.Execute(ctx, mc.(MessageContext), toolName, toolInput)
	if err != nil {
		return "", err
	}
	summary := summarizeToolOutputForModel(output)
	recordToolArtifact(ctx, toolName, toolInput, output, summary)
	addProcessTrace(ctx, "tool:"+toolName, toolInput+"\n→ "+summary)
	return output, nil
}

func (r *serviceAgentRuntime) PersistTurn(ctx context.Context, mc any, userInput, assistantReply, finalSummary string) {
	setTurnSummary(ctx, finalSummary)
	r.service.maybeAppendConversationHistory(ctx, mc.(MessageContext), userInput, assistantReply)
}

// serviceAgentPlanner implements ai.AgentPlanner using the aiBackend.
type serviceAgentPlanner struct {
	svc aiBackend
}

func (p *serviceAgentPlanner) PlanNext(ctx context.Context, task string, history []ai.ConversationMessage, tools []ai.AgentToolDefinition, state ai.AgentTaskState) (ai.LoopDecision, error) {
	return p.svc.PlanNext(ctx, task, history, tools, state)
}

func (p *serviceAgentPlanner) SummarizeWorkingState(ctx context.Context, state ai.AgentTaskState) (string, error) {
	return p.svc.SummarizeWorkingState(ctx, state)
}

func (p *serviceAgentPlanner) SummarizeFinal(ctx context.Context, state ai.AgentTaskState, finalAnswer string) (string, error) {
	return p.svc.SummarizeFinal(ctx, state, finalAnswer)
}

func (s *Service) handleAgentQuestion(ctx context.Context, mc MessageContext, question string) (string, error) {
	if s.toolProviders == nil {
		return "", fmt.Errorf("agent tool providers not configured")
	}
	runtime := &serviceAgentRuntime{service: s}
	planner := &serviceAgentPlanner{svc: s.aiService}
	answer, _, err := ai.RunAgentLoop(ctx, runtime, planner, mc, question, maxAgentToolSteps)
	return answer, err
}
