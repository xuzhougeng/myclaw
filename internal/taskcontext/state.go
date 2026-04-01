package taskcontext

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

type ToolArtifact struct {
	ToolName  string
	ToolInput string
	RawOutput string
	Summary   string
}

type State struct {
	mu             sync.Mutex
	turnSummary    string
	workingSummary string
	artifacts      []ToolArtifact
}

type stateKey struct{}

func WithState(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if ctx.Value(stateKey{}) != nil {
		return ctx
	}
	return context.WithValue(ctx, stateKey{}, &State{})
}

func SetTurnSummary(ctx context.Context, summary string) {
	state, _ := ctx.Value(stateKey{}).(*State)
	if state == nil {
		return
	}
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return
	}
	state.mu.Lock()
	state.turnSummary = summary
	state.mu.Unlock()
}

func TurnSummary(ctx context.Context) string {
	state, _ := ctx.Value(stateKey{}).(*State)
	if state == nil {
		return ""
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	return strings.TrimSpace(state.turnSummary)
}

func AddToolArtifact(ctx context.Context, artifact ToolArtifact) {
	state, _ := ctx.Value(stateKey{}).(*State)
	if state == nil {
		return
	}
	artifact.ToolName = strings.TrimSpace(artifact.ToolName)
	artifact.ToolInput = strings.TrimSpace(artifact.ToolInput)
	artifact.RawOutput = strings.TrimSpace(artifact.RawOutput)
	artifact.Summary = strings.TrimSpace(artifact.Summary)
	if artifact.ToolName == "" && artifact.ToolInput == "" && artifact.RawOutput == "" && artifact.Summary == "" {
		return
	}
	state.mu.Lock()
	state.artifacts = append(state.artifacts, artifact)
	state.mu.Unlock()
}

func ToolArtifacts(ctx context.Context) []ToolArtifact {
	state, _ := ctx.Value(stateKey{}).(*State)
	if state == nil {
		return nil
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	out := make([]ToolArtifact, len(state.artifacts))
	copy(out, state.artifacts)
	return out
}

// SetWorkingSummary updates the in-progress working summary for this task.
func SetWorkingSummary(ctx context.Context, summary string) {
	state, _ := ctx.Value(stateKey{}).(*State)
	if state == nil {
		return
	}
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return
	}
	state.mu.Lock()
	state.workingSummary = summary
	state.mu.Unlock()
}

// WorkingSummary returns the current working summary.
func WorkingSummary(ctx context.Context) string {
	state, _ := ctx.Value(stateKey{}).(*State)
	if state == nil {
		return ""
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	return strings.TrimSpace(state.workingSummary)
}

// ArtifactsSummary returns a compact numbered list of all tool artifacts.
// Returns empty string if no artifacts recorded.
func ArtifactsSummary(ctx context.Context) string {
	state, _ := ctx.Value(stateKey{}).(*State)
	if state == nil {
		return ""
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if len(state.artifacts) == 0 {
		return ""
	}
	var sb strings.Builder
	for i, a := range state.artifacts {
		var status string
		if a.Summary == "" {
			status = a.RawOutput
		} else {
			status = a.Summary
		}
		fmt.Fprintf(&sb, "%d. tool=%s: %s\n", i+1, a.ToolName, status)
	}
	return strings.TrimRight(sb.String(), "\n")
}
