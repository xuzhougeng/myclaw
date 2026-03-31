package ai

import (
	"context"
	"testing"
)

type fakeRuntime struct {
	persistCalled int
	toolOutput    string
	toolErr       error
}

func (r *fakeRuntime) LoadHistory(_ context.Context, _ any) []ConversationMessage {
	return nil
}

func (r *fakeRuntime) ListTools(_ context.Context, _ any) ([]AgentToolDefinition, error) {
	return []AgentToolDefinition{{Name: "fake_tool"}}, nil
}

func (r *fakeRuntime) ExecuteTool(_ context.Context, _ any, _, _ string) (string, error) {
	return r.toolOutput, r.toolErr
}

func (r *fakeRuntime) PersistTurn(_ context.Context, _ any, _, _, _ string) {
	r.persistCalled++
}

type fakePlanner struct {
	decisions []LoopDecision
	callIndex int
}

func (p *fakePlanner) PlanNext(_ context.Context, _ string, _ []ConversationMessage, _ []AgentToolDefinition, _ AgentTaskState) (LoopDecision, error) {
	if p.callIndex >= len(p.decisions) {
		return LoopDecision{Action: LoopStop, Reason: "no more decisions"}, nil
	}
	d := p.decisions[p.callIndex]
	p.callIndex++
	return d, nil
}

func (p *fakePlanner) SummarizeWorkingState(_ context.Context, _ AgentTaskState) (string, error) {
	return "working summary", nil
}

func (p *fakePlanner) SummarizeFinal(_ context.Context, _ AgentTaskState, _ string) (string, error) {
	return "final summary", nil
}

func TestRunAgentLoop_DirectAnswer(t *testing.T) {
	rt := &fakeRuntime{}
	pl := &fakePlanner{
		decisions: []LoopDecision{
			{Action: LoopAnswer, Answer: "hello world"},
		},
	}

	answer, _, err := RunAgentLoop(context.Background(), rt, pl, nil, "say hello", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if answer != "hello world" {
		t.Errorf("expected 'hello world', got %q", answer)
	}
	if rt.persistCalled != 1 {
		t.Errorf("expected PersistTurn called once, got %d", rt.persistCalled)
	}
}

func TestRunAgentLoop_ToolThenAnswer(t *testing.T) {
	rt := &fakeRuntime{toolOutput: "tool result"}
	pl := &fakePlanner{
		decisions: []LoopDecision{
			{Action: LoopContinue, ToolName: "fake_tool", ToolInput: `{}`},
			{Action: LoopAnswer, Answer: "done"},
		},
	}

	answer, _, err := RunAgentLoop(context.Background(), rt, pl, nil, "do something", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if answer != "done" {
		t.Errorf("expected 'done', got %q", answer)
	}
	if rt.persistCalled != 1 {
		t.Errorf("expected PersistTurn called once, got %d", rt.persistCalled)
	}
}

func TestRunAgentLoop_MaxStepsExceeded(t *testing.T) {
	rt := &fakeRuntime{toolOutput: "result"}
	// Always returns LoopContinue — loop will exhaust maxSteps.
	alwaysContinue := make([]LoopDecision, 5)
	for i := range alwaysContinue {
		alwaysContinue[i] = LoopDecision{Action: LoopContinue, ToolName: "fake_tool", ToolInput: `{}`}
	}
	pl := &fakePlanner{decisions: alwaysContinue}

	answer, _, err := RunAgentLoop(context.Background(), rt, pl, nil, "infinite task", 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if answer != "任务达到最大步骤限制，先在这里停止。" {
		t.Errorf("unexpected stop message: %q", answer)
	}
	if rt.persistCalled != 1 {
		t.Errorf("expected PersistTurn called once, got %d", rt.persistCalled)
	}
}

func TestRunAgentLoop_StopAction(t *testing.T) {
	rt := &fakeRuntime{}
	pl := &fakePlanner{
		decisions: []LoopDecision{
			{Action: LoopStop, Reason: "cannot proceed"},
		},
	}

	answer, _, err := RunAgentLoop(context.Background(), rt, pl, nil, "task", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if answer != "cannot proceed" {
		t.Errorf("expected 'cannot proceed', got %q", answer)
	}
	if rt.persistCalled != 1 {
		t.Errorf("expected PersistTurn called once, got %d", rt.persistCalled)
	}
}

func TestRunAgentLoop_AskAction(t *testing.T) {
	rt := &fakeRuntime{}
	pl := &fakePlanner{
		decisions: []LoopDecision{
			{Action: LoopAsk, Question: "which file?"},
		},
	}

	answer, _, err := RunAgentLoop(context.Background(), rt, pl, nil, "ambiguous task", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if answer != "which file?" {
		t.Errorf("expected 'which file?', got %q", answer)
	}
	if rt.persistCalled != 1 {
		t.Errorf("expected PersistTurn called once, got %d", rt.persistCalled)
	}
}
