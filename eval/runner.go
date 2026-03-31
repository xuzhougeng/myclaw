package eval

import (
	"context"
	"fmt"
	"time"

	"myclaw/internal/ai"
)

type CaseResult struct {
	ID         string                 `json:"id"`
	Pass       bool                   `json:"pass"`
	DurationMs int64                  `json:"duration_ms"`
	RawOutput  map[string]any         `json:"raw_output"`
	Judge      JudgeResult            `json:"judge_result"`
	Error      string                 `json:"error,omitempty"`
}

type RunReport struct {
	Dataset   string       `json:"dataset"`
	Provider  string       `json:"provider"`
	Model     string       `json:"model"`
	APIType   string       `json:"api_type"`
	StartedAt string       `json:"started_at"`
	Cases     []CaseResult `json:"cases"`
}

func RunStage(ctx context.Context, svc *ai.Service, tc TestCase, demoTools map[string]ai.ToolCapability) (CaseResult, error) {
	start := time.Now()
	result := CaseResult{ID: tc.ID}

	var rawOutput map[string]any
	var err error

	switch tc.Stage {
	case "route_command":
		rawOutput, err = runRouteCommand(ctx, svc, tc)
	case "search_plan":
		rawOutput, err = runSearchPlan(ctx, svc, tc)
	case "tool_opportunity":
		rawOutput, err = runToolOpportunity(ctx, svc, tc, demoTools)
	case "tool_plan":
		rawOutput, err = runToolPlan(ctx, svc, tc, demoTools)
	case "agent_step":
		rawOutput, err = runAgentStep(ctx, svc, tc, demoTools)
	case "agent_loop":
		rawOutput, err = runAgentLoop(ctx, svc, tc, demoTools)
	default:
		return result, fmt.Errorf("unknown stage: %s", tc.Stage)
	}

	result.DurationMs = time.Since(start).Milliseconds()
	if err != nil {
		result.Error = err.Error()
		return result, nil
	}

	result.RawOutput = rawOutput
	judgeResult, err := Judge(tc.Expect, tc.Judge, rawOutput)
	if err != nil {
		result.Error = err.Error()
		return result, nil
	}
	result.Judge = judgeResult
	result.Pass = judgeResult.Pass
	return result, nil
}
