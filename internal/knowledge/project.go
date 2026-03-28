package knowledge

import (
	"context"
	"strings"
)

const DefaultProjectName = "default"

type projectContextKey struct{}

func WithProject(ctx context.Context, project string) context.Context {
	project = strings.TrimSpace(project)
	if project == "" {
		return ctx
	}
	return context.WithValue(ctx, projectContextKey{}, project)
}

func ProjectFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	project, _ := ctx.Value(projectContextKey{}).(string)
	return strings.TrimSpace(project)
}

func CanonicalProjectName(project string) string {
	project = strings.TrimSpace(project)
	if project == "" {
		return DefaultProjectName
	}
	return project
}

func sameProject(left, right string) bool {
	return normalizedProjectKey(left) == normalizedProjectKey(right)
}

func normalizedProjectKey(project string) string {
	return strings.ToLower(CanonicalProjectName(project))
}
