package app

import (
	"strings"

	"myclaw/internal/ai"
)

// seenQueryTracker deduplicates string keys across loop rounds.
type seenQueryTracker struct {
	seen map[string]struct{}
}

func newSeenQueryTracker() *seenQueryTracker {
	return &seenQueryTracker{seen: make(map[string]struct{})}
}

func (t *seenQueryTracker) IsDuplicate(key string) bool {
	_, ok := t.seen[key]
	return ok
}

func (t *seenQueryTracker) Mark(key string) {
	t.seen[key] = struct{}{}
}

// priorExecutions is a capped slice of ai.ToolExecution for prior-aware replanning.
type priorExecutions struct {
	items []ai.ToolExecution
	cap   int
}

func newPriorExecutions(cap int) *priorExecutions {
	return &priorExecutions{cap: cap}
}

func (p *priorExecutions) Add(exec ai.ToolExecution) {
	p.items = append(p.items, exec)
	if len(p.items) > p.cap {
		p.items = p.items[len(p.items)-p.cap:]
	}
}

func (p *priorExecutions) Slice() []ai.ToolExecution {
	return p.items
}

// compactToolOutputForPlanner truncates raw tool output for use in the next planning call.
func compactToolOutputForPlanner(rawOutput string, maxRunes int) string {
	s := strings.TrimSpace(rawOutput)
	if s == "" {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes]) + "[truncated]"
}
