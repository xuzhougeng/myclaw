package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"baize/internal/modelconfig"
)

func newTestAgentService(t *testing.T, handler func(*http.Request) (*http.Response, error)) *Service {
	t.Helper()
	store := newConfiguredStore(t, modelconfig.Config{
		Provider: modelconfig.ProviderOpenAI,
		APIType:  modelconfig.APITypeResponses,
		BaseURL:  "http://example.invalid/v1",
		APIKey:   "secret",
		Model:    "gpt-test",
	})
	service := NewService(store)
	service.httpClient = newTestClient(t, handler)
	return service
}

func TestPlanAgentLoopStep_FirstCallContinue(t *testing.T) {
	service := newTestAgentService(t, func(r *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, `{"output":[{"type":"message","content":[{"type":"output_text","text":"{\"action\":\"continue\",\"tool_name\":\"local::knowledge_search\",\"tool_input\":\"{\\\"query\\\":\\\"test\\\"}\",\"answer\":\"\",\"question\":\"\",\"reason\":\"\"}"}]}]}`), nil
	})

	state := AgentTaskState{
		Goal: "搜索知识库",
		CandidateTools: []AgentToolDefinition{
			{
				Name:            "local::knowledge_search",
				Description:     "搜索本地知识库",
				SideEffectLevel: "read_only",
			},
		},
	}

	decision, err := service.PlanAgentLoopStep(context.Background(), nil, state)
	if err != nil {
		t.Fatalf("PlanAgentLoopStep: %v", err)
	}
	if decision.Action != LoopContinue {
		t.Fatalf("expected LoopContinue, got %q", decision.Action)
	}
	if decision.ToolName != "local::knowledge_search" {
		t.Fatalf("unexpected tool_name: %q", decision.ToolName)
	}
}

func TestPlanAgentLoopStep_ConvergeToAnswer(t *testing.T) {
	service := newTestAgentService(t, func(r *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, `{"output":[{"type":"message","content":[{"type":"output_text","text":"{\"action\":\"answer\",\"tool_name\":\"\",\"tool_input\":\"\",\"answer\":\"result found\",\"question\":\"\",\"reason\":\"\"}"}]}]}`), nil
	})

	state := AgentTaskState{
		Goal: "查找结果",
		ToolAttempts: []ToolAttempt{
			{
				ToolName:      "local::knowledge_search",
				ToolInput:     `{"query":"test"}`,
				OutputSummary: "找到相关内容",
				Succeeded:     true,
			},
		},
		CandidateTools: []AgentToolDefinition{
			{
				Name:        "local::knowledge_search",
				Description: "搜索本地知识库",
			},
		},
	}

	decision, err := service.PlanAgentLoopStep(context.Background(), nil, state)
	if err != nil {
		t.Fatalf("PlanAgentLoopStep: %v", err)
	}
	if decision.Action != LoopAnswer {
		t.Fatalf("expected LoopAnswer, got %q", decision.Action)
	}
	if decision.Answer != "result found" {
		t.Fatalf("unexpected answer: %q", decision.Answer)
	}
}

func TestPlanAgentLoopStep_StopAction(t *testing.T) {
	service := newTestAgentService(t, func(r *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, `{"output":[{"type":"message","content":[{"type":"output_text","text":"{\"action\":\"stop\",\"tool_name\":\"\",\"tool_input\":\"\",\"answer\":\"\",\"question\":\"\",\"reason\":\"no more options\"}"}]}]}`), nil
	})

	state := AgentTaskState{
		Goal: "执行某个任务",
		CandidateTools: []AgentToolDefinition{
			{
				Name:        "local::knowledge_search",
				Description: "搜索本地知识库",
			},
		},
	}

	decision, err := service.PlanAgentLoopStep(context.Background(), nil, state)
	if err != nil {
		t.Fatalf("PlanAgentLoopStep: %v", err)
	}
	if decision.Action != LoopStop {
		t.Fatalf("expected LoopStop, got %q", decision.Action)
	}
	if decision.Reason != "no more options" {
		t.Fatalf("unexpected reason: %q", decision.Reason)
	}
}

func TestSummarizeWorkingState_Empty(t *testing.T) {
	// Handler should NOT be called when ToolAttempts is empty.
	called := false
	service := newTestAgentService(t, func(r *http.Request) (*http.Response, error) {
		called = true
		return jsonResponse(http.StatusOK, `{"output":[{"type":"message","content":[{"type":"output_text","text":"some summary"}]}]}`), nil
	})

	state := AgentTaskState{
		Goal:         "查找资料",
		ToolAttempts: nil,
	}

	result, err := service.SummarizeAgentWorkingState(context.Background(), state)
	if err != nil {
		t.Fatalf("SummarizeAgentWorkingState: %v", err)
	}
	if result != "" {
		t.Fatalf("expected empty result for no tool attempts, got %q", result)
	}
	if called {
		t.Fatal("expected no HTTP call when ToolAttempts is empty")
	}
}

func TestSummarizeWorkingState_WithAttempts(t *testing.T) {
	service := newTestAgentService(t, func(r *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, `{"output":[{"type":"message","content":[{"type":"output_text","text":"已搜索知识库并找到相关内容，任务进展顺利。"}]}]}`), nil
	})

	state := AgentTaskState{
		Goal: "查找资料",
		ToolAttempts: []ToolAttempt{
			{
				ToolName:      "local::knowledge_search",
				ToolInput:     `{"query":"test"}`,
				OutputSummary: "找到 3 条相关记录",
				Succeeded:     true,
			},
		},
	}

	result, err := service.SummarizeAgentWorkingState(context.Background(), state)
	if err != nil {
		t.Fatalf("SummarizeAgentWorkingState: %v", err)
	}
	if result == "" {
		t.Fatal("expected non-empty summary")
	}
}

func TestPlanAgentLoopStep_PrefersOutputSummaryOverRawOutput(t *testing.T) {
	var prompt string
	service := newTestAgentService(t, func(r *http.Request) (*http.Response, error) {
		var req responsesRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		prompt = req.Input[0].Content[0].Text
		return jsonResponse(http.StatusOK, `{"output":[{"type":"message","content":[{"type":"output_text","text":"{\"action\":\"answer\",\"tool_name\":\"\",\"tool_input\":\"\",\"answer\":\"done\",\"question\":\"\",\"reason\":\"\"}"}]}]}`), nil
	})

	state := AgentTaskState{
		Goal: "读取工具结果",
		ToolAttempts: []ToolAttempt{
			{
				ToolName:      "local::knowledge_search",
				ToolInput:     `{"query":"test"}`,
				RawOutput:     "RAW-OUTPUT-SHOULD-NOT-BE-IN-PROMPT",
				OutputSummary: "SUMMARY-OUTPUT-SHOULD-BE-IN-PROMPT",
				Succeeded:     true,
			},
		},
		CandidateTools: []AgentToolDefinition{{Name: "local::knowledge_search", Description: "搜索本地知识库"}},
	}

	if _, err := service.PlanAgentLoopStep(context.Background(), nil, state); err != nil {
		t.Fatalf("PlanAgentLoopStep: %v", err)
	}
	if !strings.Contains(prompt, "SUMMARY-OUTPUT-SHOULD-BE-IN-PROMPT") {
		t.Fatalf("expected prompt to include output summary, got:\n%s", prompt)
	}
	if strings.Contains(prompt, "RAW-OUTPUT-SHOULD-NOT-BE-IN-PROMPT") {
		t.Fatalf("expected prompt to omit raw output when summary exists, got:\n%s", prompt)
	}
}

func TestPlanAgentLoopStep_FallsBackToTrimmedRawOutput(t *testing.T) {
	var prompt string
	service := newTestAgentService(t, func(r *http.Request) (*http.Response, error) {
		var req responsesRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		prompt = req.Input[0].Content[0].Text
		return jsonResponse(http.StatusOK, `{"output":[{"type":"message","content":[{"type":"output_text","text":"{\"action\":\"answer\",\"tool_name\":\"\",\"tool_input\":\"\",\"answer\":\"done\",\"question\":\"\",\"reason\":\"\"}"}]}]}`), nil
	})

	rawOutput := strings.Join([]string{
		"line-1",
		"line-2",
		"line-3",
		"line-4",
		"line-5",
		"line-6",
		"line-7",
		"line-8",
		"line-9-hidden",
	}, "\n")

	state := AgentTaskState{
		Goal: "读取工具结果",
		ToolAttempts: []ToolAttempt{
			{
				ToolName:  "local::knowledge_search",
				ToolInput: `{"query":"test"}`,
				RawOutput: rawOutput,
				Succeeded: true,
			},
		},
		CandidateTools: []AgentToolDefinition{{Name: "local::knowledge_search", Description: "搜索本地知识库"}},
	}

	if _, err := service.PlanAgentLoopStep(context.Background(), nil, state); err != nil {
		t.Fatalf("PlanAgentLoopStep: %v", err)
	}
	if !strings.Contains(prompt, "line-8") {
		t.Fatalf("expected prompt to retain trimmed raw output, got:\n%s", prompt)
	}
	if !strings.Contains(prompt, "[truncated]") {
		t.Fatalf("expected prompt to mark raw output truncation, got:\n%s", prompt)
	}
	if strings.Contains(prompt, "line-9-hidden") {
		t.Fatalf("expected prompt to omit lines beyond fallback limit, got:\n%s", prompt)
	}
}
