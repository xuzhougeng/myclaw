package app

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"baize/internal/ai"
	"baize/internal/knowledge"
	"baize/internal/reminder"
)

// agentFakeAI wraps fakeAI and overrides PlanNext with a custom function so each
// test can script exactly which decisions are returned.
type agentFakeAI struct {
	fakeAI
	planNextFn              func(ctx context.Context, task string, history []ai.ConversationMessage, tools []ai.AgentToolDefinition, state ai.AgentTaskState) (ai.LoopDecision, error)
	summarizeWorkingStateFn func(ctx context.Context, state ai.AgentTaskState) (string, error)
}

func (f agentFakeAI) PlanNext(ctx context.Context, task string, history []ai.ConversationMessage, tools []ai.AgentToolDefinition, state ai.AgentTaskState) (ai.LoopDecision, error) {
	if f.planNextFn != nil {
		return f.planNextFn(ctx, task, history, tools, state)
	}
	return ai.LoopDecision{Action: ai.LoopAnswer, Answer: "fake answer"}, nil
}

func (f agentFakeAI) SummarizeWorkingState(ctx context.Context, state ai.AgentTaskState) (string, error) {
	if f.summarizeWorkingStateFn != nil {
		return f.summarizeWorkingStateFn(ctx, state)
	}
	return f.fakeAI.SummarizeWorkingState(ctx, state)
}

func newAgentTestService(t *testing.T, backend aiBackend) *Service {
	t.Helper()
	store := knowledge.NewStore(filepath.Join(t.TempDir(), "app.db"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(t.TempDir(), "app.db")))
	return NewService(store, backend, reminders)
}

// TestAgentLoopIntegration_AnswerOnFirstStep verifies that when PlanNext immediately
// returns LoopAnswer, handleAgentQuestion returns the answer without calling any tool.
func TestAgentLoopIntegration_AnswerOnFirstStep(t *testing.T) {
	t.Parallel()

	const wantAnswer = "Paris is the capital of France."

	backend := agentFakeAI{
		fakeAI: fakeAI{configured: true},
		planNextFn: func(_ context.Context, _ string, _ []ai.ConversationMessage, _ []ai.AgentToolDefinition, _ ai.AgentTaskState) (ai.LoopDecision, error) {
			return ai.LoopDecision{Action: ai.LoopAnswer, Answer: wantAnswer}, nil
		},
	}

	svc := newAgentTestService(t, backend)
	ctx := withTaskContext(context.Background())

	got, err := svc.handleAgentQuestion(ctx, MessageContext{UserID: "u1"}, "What is the capital of France?")
	if err != nil {
		t.Fatalf("handleAgentQuestion: %v", err)
	}
	if got != wantAnswer {
		t.Fatalf("want %q, got %q", wantAnswer, got)
	}
}

// TestAgentLoopIntegration_ToolThenAnswer verifies that when PlanNext first returns
// LoopContinue (calling local::knowledge_search), then LoopAnswer, the tool is
// executed and the final answer is returned.
func TestAgentLoopIntegration_ToolThenAnswer(t *testing.T) {
	t.Parallel()

	const wantAnswer = "The result from the knowledge search."
	calls := 0
	const wantWorkingSummary = "已经得到一次工具摘要。"

	backend := agentFakeAI{
		fakeAI: fakeAI{configured: true},
		planNextFn: func(_ context.Context, _ string, _ []ai.ConversationMessage, _ []ai.AgentToolDefinition, state ai.AgentTaskState) (ai.LoopDecision, error) {
			calls++
			if calls == 1 {
				// First call: request a tool
				return ai.LoopDecision{
					Action:    ai.LoopContinue,
					ToolName:  "local::knowledge_search",
					ToolInput: `{"query":"test"}`,
				}, nil
			}
			// Second call: return the answer; the tool result should be in state
			if len(state.ToolAttempts) == 0 {
				t.Error("expected at least one tool attempt in state on second call")
			}
			if got := state.ToolAttempts[0].OutputSummary; got == "" {
				t.Error("expected non-empty output summary on second call")
			}
			if got := state.WorkingSummary; got != wantWorkingSummary {
				t.Errorf("expected working summary %q, got %q", wantWorkingSummary, got)
			}
			return ai.LoopDecision{Action: ai.LoopAnswer, Answer: wantAnswer}, nil
		},
		summarizeWorkingStateFn: func(_ context.Context, state ai.AgentTaskState) (string, error) {
			if len(state.ToolAttempts) == 0 {
				return "", nil
			}
			if strings.TrimSpace(state.ToolAttempts[0].OutputSummary) == "" {
				t.Error("expected planner summary input to contain tool output summary")
			}
			return wantWorkingSummary, nil
		},
	}

	svc := newAgentTestService(t, backend)
	ctx := withTaskContext(context.Background())

	got, err := svc.handleAgentQuestion(ctx, MessageContext{UserID: "u1"}, "search for test")
	if err != nil {
		t.Fatalf("handleAgentQuestion: %v", err)
	}
	if got != wantAnswer {
		t.Fatalf("want %q, got %q", wantAnswer, got)
	}
	if calls != 2 {
		t.Fatalf("expected 2 PlanNext calls, got %d", calls)
	}
	if got := workingSummaryFromContext(ctx); got != wantWorkingSummary {
		t.Fatalf("expected working summary mirrored into task context, got %q", got)
	}
	if got := artifactsSummaryFromContext(ctx); !strings.Contains(got, "local::knowledge_search") {
		t.Fatalf("expected tool artifact summary recorded, got %q", got)
	}
}

// TestAgentLoopIntegration_MaxStepsReached verifies that the loop returns an
// TestAgentLoopIntegration_MaxStepsReached verifies that when the planner never
// returns LoopAnswer, the loop terminates gracefully with a stop reply.
func TestAgentLoopIntegration_MaxStepsReached(t *testing.T) {
	t.Parallel()

	backend := agentFakeAI{
		fakeAI: fakeAI{configured: true},
		planNextFn: func(_ context.Context, _ string, _ []ai.ConversationMessage, _ []ai.AgentToolDefinition, _ ai.AgentTaskState) (ai.LoopDecision, error) {
			// Always keep calling a tool — never answer.
			return ai.LoopDecision{
				Action:    ai.LoopContinue,
				ToolName:  "local::knowledge_search",
				ToolInput: `{"query":"x"}`,
			}, nil
		},
	}

	svc := newAgentTestService(t, backend)
	ctx := withTaskContext(context.Background())

	reply, err := svc.handleAgentQuestion(ctx, MessageContext{UserID: "u1"}, "loop forever")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reply == "" {
		t.Fatal("expected non-empty stop reply when max steps reached")
	}
}
