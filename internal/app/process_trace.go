package app

import (
	"context"
	"strings"

	"baize/internal/ai"
)

func addProcessTrace(ctx context.Context, title, detail string) {
	title = strings.TrimSpace(title)
	detail = strings.TrimSpace(detail)
	if title == "" && detail == "" {
		return
	}
	ai.AddCallTraceStep(ctx, ai.CallTraceStep{
		Title:  title,
		Detail: detail,
	})
}
