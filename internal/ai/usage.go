package ai

import (
	"context"
	"sync"
)

type TokenUsage struct {
	InputTokens  int `json:"inputTokens"`
	OutputTokens int `json:"outputTokens"`
	CachedTokens int `json:"cachedTokens"`
	TotalTokens  int `json:"totalTokens"`
}

func (u TokenUsage) IsZero() bool {
	return u.InputTokens == 0 && u.OutputTokens == 0 && u.CachedTokens == 0 && u.TotalTokens == 0
}

func (u TokenUsage) normalized() TokenUsage {
	if u.InputTokens < 0 {
		u.InputTokens = 0
	}
	if u.OutputTokens < 0 {
		u.OutputTokens = 0
	}
	if u.CachedTokens < 0 {
		u.CachedTokens = 0
	}
	if u.TotalTokens < 0 {
		u.TotalTokens = 0
	}
	if u.TotalTokens == 0 {
		u.TotalTokens = u.InputTokens + u.OutputTokens
	}
	return u
}

func (u TokenUsage) Add(other TokenUsage) TokenUsage {
	u = u.normalized()
	other = other.normalized()
	return TokenUsage{
		InputTokens:  u.InputTokens + other.InputTokens,
		OutputTokens: u.OutputTokens + other.OutputTokens,
		CachedTokens: u.CachedTokens + other.CachedTokens,
		TotalTokens:  u.TotalTokens + other.TotalTokens,
	}
}

type usageCollector struct {
	mu    sync.Mutex
	usage TokenUsage
}

type usageCollectorKey struct{}

func WithUsageCollector(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if ctx.Value(usageCollectorKey{}) != nil {
		return ctx
	}
	return context.WithValue(ctx, usageCollectorKey{}, &usageCollector{})
}

func UsageFromContext(ctx context.Context) TokenUsage {
	if ctx == nil {
		return TokenUsage{}
	}
	collector, _ := ctx.Value(usageCollectorKey{}).(*usageCollector)
	if collector == nil {
		return TokenUsage{}
	}
	collector.mu.Lock()
	defer collector.mu.Unlock()
	return collector.usage.normalized()
}

func AddUsage(ctx context.Context, usage TokenUsage) {
	if ctx == nil {
		return
	}
	collector, _ := ctx.Value(usageCollectorKey{}).(*usageCollector)
	if collector == nil {
		return
	}
	usage = usage.normalized()
	if usage.IsZero() {
		return
	}
	collector.mu.Lock()
	collector.usage = collector.usage.Add(usage)
	collector.mu.Unlock()
}
