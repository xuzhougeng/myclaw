package app

import (
	"context"
	"strings"

	"baize/internal/taskcontext"
)

const (
	maxToolContextRunes = 600
	maxToolContextLines = 8
)

func withTaskContext(ctx context.Context) context.Context {
	return taskcontext.WithState(ctx)
}

func setTurnSummary(ctx context.Context, summary string) {
	taskcontext.SetTurnSummary(ctx, strings.TrimSpace(summary))
}

func turnSummaryFromContext(ctx context.Context) string {
	return strings.TrimSpace(taskcontext.TurnSummary(ctx))
}

func recordToolArtifact(ctx context.Context, toolName, toolInput, rawOutput, summary string) {
	taskcontext.AddToolArtifact(ctx, taskcontext.ToolArtifact{
		ToolName:  toolName,
		ToolInput: toolInput,
		RawOutput: rawOutput,
		Summary:   summary,
	})
}

func setWorkingSummary(ctx context.Context, summary string) {
	taskcontext.SetWorkingSummary(ctx, strings.TrimSpace(summary))
}

func workingSummaryFromContext(ctx context.Context) string {
	return strings.TrimSpace(taskcontext.WorkingSummary(ctx))
}

func artifactsSummaryFromContext(ctx context.Context) string {
	return taskcontext.ArtifactsSummary(ctx)
}

// summarizeToolOutputForModel compacts raw tool output into the planner-facing summary
// that is carried through the synchronous agent loop. It is intentionally lossy so
// scratchpad artifacts remain available for debugging while planner prompts stay bounded.
func summarizeToolOutputForModel(output string) string {
	output = strings.TrimSpace(strings.ReplaceAll(output, "\r\n", "\n"))
	if output == "" {
		return ""
	}
	lines := strings.Split(output, "\n")
	truncated := false
	if len(lines) > maxToolContextLines {
		lines = lines[:maxToolContextLines]
		truncated = true
	}
	output = strings.TrimSpace(strings.Join(lines, "\n"))
	if output == "" {
		return ""
	}
	if runeCount(output) > maxToolContextRunes {
		output = preview(output, maxToolContextRunes)
		truncated = true
	}
	if truncated {
		output = strings.TrimSpace(output) + "\n[truncated]"
	}
	return output
}

func runeCount(text string) int {
	return len([]rune(text))
}
