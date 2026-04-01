package taskcontext

import (
	"context"
	"strings"
	"testing"
)

func TestWorkingSummaryRoundtrip(t *testing.T) {
	ctx := WithState(context.Background())

	if got := WorkingSummary(ctx); got != "" {
		t.Fatalf("expected empty initial working summary, got %q", got)
	}

	SetWorkingSummary(ctx, "  progress so far  ")
	if got := WorkingSummary(ctx); got != "progress so far" {
		t.Fatalf("expected trimmed summary, got %q", got)
	}

	// Empty string must not overwrite existing value.
	SetWorkingSummary(ctx, "")
	if got := WorkingSummary(ctx); got != "progress so far" {
		t.Fatalf("empty update should not overwrite, got %q", got)
	}

	// Later non-empty update replaces.
	SetWorkingSummary(ctx, "updated")
	if got := WorkingSummary(ctx); got != "updated" {
		t.Fatalf("expected updated summary, got %q", got)
	}
}

func TestWorkingSummaryNilContext(t *testing.T) {
	// Context without state should return empty and not panic.
	ctx := context.Background()
	SetWorkingSummary(ctx, "ignored")
	if got := WorkingSummary(ctx); got != "" {
		t.Fatalf("expected empty for stateless context, got %q", got)
	}
}

func TestArtifactsSummaryFormat(t *testing.T) {
	ctx := WithState(context.Background())

	AddToolArtifact(ctx, ToolArtifact{ToolName: "bash", RawOutput: "exit 0", Summary: "ran ok"})
	AddToolArtifact(ctx, ToolArtifact{ToolName: "read", RawOutput: "raw data", Summary: ""})

	got := ArtifactsSummary(ctx)
	lines := strings.Split(got, "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d: %q", len(lines), got)
	}
	if !strings.Contains(lines[0], "tool=bash") || !strings.Contains(lines[0], "ran ok") {
		t.Errorf("line 0 unexpected: %q", lines[0])
	}
	if !strings.Contains(lines[1], "tool=read") || !strings.Contains(lines[1], "raw data") {
		t.Errorf("line 1 unexpected: %q", lines[1])
	}
	if strings.HasSuffix(got, "\n") {
		t.Errorf("trailing newline not trimmed: %q", got)
	}
}

func TestArtifactsSummaryEmpty(t *testing.T) {
	ctx := WithState(context.Background())
	if got := ArtifactsSummary(ctx); got != "" {
		t.Fatalf("expected empty summary with no artifacts, got %q", got)
	}

	// Nil-state context also returns empty.
	ctx2 := context.Background()
	if got := ArtifactsSummary(ctx2); got != "" {
		t.Fatalf("expected empty summary for stateless context, got %q", got)
	}
}
