package app

import (
	"strings"
	"testing"

	"myclaw/internal/ai"
)

func TestSeenQueryTrackerDuplicateDetection(t *testing.T) {
	t.Parallel()

	tracker := newSeenQueryTracker()
	tracker.Mark("foo")

	if !tracker.IsDuplicate("foo") {
		t.Fatal("expected IsDuplicate(\"foo\") == true after Mark")
	}
	if tracker.IsDuplicate("bar") {
		t.Fatal("expected IsDuplicate(\"bar\") == false for unmarked key")
	}
}

func TestSeenQueryTrackerEmptyKey(t *testing.T) {
	t.Parallel()

	tracker := newSeenQueryTracker()
	if tracker.IsDuplicate("any") {
		t.Fatal("expected IsDuplicate on fresh tracker to return false")
	}
}

func TestPriorExecutionsCapEnforced(t *testing.T) {
	t.Parallel()

	prior := newPriorExecutions(3)
	for i := 0; i < 5; i++ {
		prior.Add(ai.ToolExecution{ToolName: "tool", ToolInput: "{}", ToolOutput: "{}"})
	}
	if len(prior.Slice()) != 3 {
		t.Fatalf("expected 3 items after adding 5 with cap=3, got %d", len(prior.Slice()))
	}
}

func TestPriorExecutionsUnderCap(t *testing.T) {
	t.Parallel()

	prior := newPriorExecutions(5)
	for i := 0; i < 2; i++ {
		prior.Add(ai.ToolExecution{ToolName: "tool", ToolInput: "{}", ToolOutput: "{}"})
	}
	if len(prior.Slice()) != 2 {
		t.Fatalf("expected 2 items after adding 2 with cap=5, got %d", len(prior.Slice()))
	}
}

func TestCompactToolOutputForPlannerTruncates(t *testing.T) {
	t.Parallel()

	input := strings.Repeat("a", 200)
	maxRunes := 50
	output := compactToolOutputForPlanner(input, maxRunes)
	if !strings.Contains(output, "[truncated]") {
		t.Fatal("expected output to contain \"[truncated]\"")
	}
	if len([]rune(output)) > maxRunes+len([]rune("[truncated]")) {
		t.Fatalf("output rune length %d exceeds maxRunes+len(\"[truncated]\")", len([]rune(output)))
	}
}

func TestCompactToolOutputForPlannerNoTruncation(t *testing.T) {
	t.Parallel()

	input := "short string"
	output := compactToolOutputForPlanner(input, 100)
	if strings.Contains(output, "[truncated]") {
		t.Fatal("expected no truncation marker for short string")
	}
	if output != input {
		t.Fatalf("expected output to equal input, got %q", output)
	}
}

func TestCompactToolOutputForPlannerEmpty(t *testing.T) {
	t.Parallel()

	output := compactToolOutputForPlanner("", 100)
	if output != "" {
		t.Fatalf("expected empty output for empty input, got %q", output)
	}
}
