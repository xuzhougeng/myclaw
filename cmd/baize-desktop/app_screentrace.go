package main

import (
	"context"
	"fmt"
	"strings"

	"baize/internal/knowledge"
	"baize/internal/screentrace"
)

func (a *DesktopApp) recordScreenTraceDigest(ctx context.Context, digest screentrace.Digest) (string, error) {
	if a == nil || a.store == nil {
		return "", nil
	}
	projectCtx, _, err := a.projectContext(ctx)
	if err != nil {
		return "", err
	}
	entry, err := a.store.Add(projectCtx, knowledge.Entry{
		Text:       formatScreenTraceDigestKnowledge(digest),
		Source:     "desktop:screentrace",
		RecordedAt: digest.BucketEnd,
	})
	if err != nil {
		return "", err
	}
	return entry.ID, nil
}

func formatScreenTraceDigestKnowledge(digest screentrace.Digest) string {
	var builder strings.Builder
	builder.WriteString("## 活动记录摘要\n")
	builder.WriteString(fmt.Sprintf("- 时间段: %s - %s\n", digest.BucketStart.Local().Format("2006-01-02 15:04:05"), digest.BucketEnd.Local().Format("15:04:05")))
	builder.WriteString(fmt.Sprintf("- 记录数: %d\n", digest.RecordCount))
	if len(digest.DominantApps) > 0 {
		builder.WriteString("- 主要应用: ")
		builder.WriteString(strings.Join(digest.DominantApps, " / "))
		builder.WriteString("\n")
	}
	if len(digest.DominantTasks) > 0 {
		builder.WriteString("- 主要任务: ")
		builder.WriteString(strings.Join(digest.DominantTasks, " / "))
		builder.WriteString("\n")
	}
	if len(digest.Keywords) > 0 {
		builder.WriteString("- 关键词: ")
		builder.WriteString(strings.Join(digest.Keywords, " / "))
		builder.WriteString("\n")
	}
	builder.WriteString("\n")
	builder.WriteString(strings.TrimSpace(digest.Summary))
	return strings.TrimSpace(builder.String())
}
