package eval

import (
	"context"
	"encoding/json"
	"fmt"

	"baize/internal/ai"
)

func runRouteCommand(ctx context.Context, svc *ai.Service, tc TestCase) (map[string]any, error) {
	decision, err := svc.RouteCommand(ctx, tc.Input)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"command":       decision.Command,
		"memory_text":   decision.MemoryText,
		"append_text":   decision.AppendText,
		"knowledge_id":  decision.KnowledgeID,
		"reminder_spec": decision.ReminderSpec,
		"reminder_id":   decision.ReminderID,
		"question":      decision.Question,
	}, nil
}

func runSearchPlan(ctx context.Context, svc *ai.Service, tc TestCase) (map[string]any, error) {
	question := tc.Question
	if question == "" {
		question = tc.Input
	}
	plan, err := svc.BuildSearchPlan(ctx, question)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"queries":  plan.Queries,
		"keywords": plan.Keywords,
	}, nil
}

func runToolOpportunity(ctx context.Context, svc *ai.Service, tc TestCase, demoTools map[string]ai.ToolCapability) (map[string]any, error) {
	var tools []ai.ToolCapability
	for _, name := range tc.Tools {
		if tool, ok := demoTools[name]; ok {
			tools = append(tools, tool)
		}
	}
	opps, err := svc.DetectToolOpportunities(ctx, tc.Task, tools)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, opp := range opps {
		names = append(names, opp.ToolName)
	}
	return map[string]any{"tool_names": names}, nil
}

func runToolPlan(ctx context.Context, svc *ai.Service, tc TestCase, demoTools map[string]ai.ToolCapability) (map[string]any, error) {
	tool, ok := demoTools[tc.ToolName]
	if !ok {
		return nil, nil
	}
	var prior []ai.ToolExecution
	if len(tc.Prior) > 0 {
		if err := json.Unmarshal(tc.Prior, &prior); err != nil {
			return nil, err
		}
	}
	decision, err := svc.PlanToolUse(ctx, tc.Task, tool, prior)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"action":     decision.Action,
		"tool_name":  decision.ToolName,
		"tool_input": decision.ToolInput,
	}, nil
}

func runAgentStep(ctx context.Context, svc *ai.Service, tc TestCase, demoTools map[string]ai.ToolCapability) (map[string]any, error) {
	var history []ai.ConversationMessage
	if len(tc.History) > 0 {
		if err := json.Unmarshal(tc.History, &history); err != nil {
			return nil, err
		}
	}
	tools := toolsFromCaps(tc.Tools, demoTools)
	var results []ai.AgentToolResult
	if len(tc.Results) > 0 {
		if err := json.Unmarshal(tc.Results, &results); err != nil {
			return nil, err
		}
	}
	decision, err := svc.DecideAgentStep(ctx, tc.Task, history, tools, results)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"action":     decision.Action,
		"answer":     decision.Answer,
		"tool_name":  decision.ToolName,
		"tool_input": decision.ToolInput,
	}, nil
}

// toolsFromCaps converts named ToolCapability entries from demoTools into
// AgentToolDefinition values for use by agent stage handlers.
func toolsFromCaps(names []string, demoTools map[string]ai.ToolCapability) []ai.AgentToolDefinition {
	var tools []ai.AgentToolDefinition
	for _, name := range names {
		if cap, ok := demoTools[name]; ok {
			tools = append(tools, ai.AgentToolDefinition{
				Name:              cap.Name,
				Purpose:           cap.Purpose,
				Description:       cap.Description,
				InputContract:     cap.InputContract,
				OutputContract:    cap.OutputContract,
				Usage:             cap.Usage,
				InputJSONExample:  cap.InputJSONExample,
				OutputJSONExample: cap.OutputJSONExample,
			})
		}
	}
	return tools
}

// runAgentLoop handles the "agent_loop" stage.
// TODO: implement once ai.AgentTaskState and svc.PlanAgentLoopStep exist:
//
//	var state ai.AgentTaskState
//	if len(tc.State) > 0 { json.Unmarshal(tc.State, &state) }
//	history := unmarshalHistory(tc.History)
//	tools := toolsFromCaps(tc.Tools, demoTools)
//	decision, err := svc.PlanAgentLoopStep(ctx, tc.Task, history, tools, state)
//	return map[string]any{"action": string(decision.Action), "tool_name": decision.ToolName,
//	    "answer": decision.Answer, "reason": decision.Reason}, err
func runAgentLoop(_ context.Context, _ *ai.Service, _ TestCase, _ map[string]ai.ToolCapability) (map[string]any, error) {
	return nil, fmt.Errorf("agent_loop stage requires PlanAgentLoopStep — not yet implemented")
}
