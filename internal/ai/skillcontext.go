package ai

import (
	"context"
	"strings"
)

type skillContextKey struct{}

func WithSkillContext(ctx context.Context, content string) context.Context {
	content = strings.TrimSpace(content)
	if content == "" {
		return ctx
	}
	return context.WithValue(ctx, skillContextKey{}, content)
}

func SkillContextFromContext(ctx context.Context) string {
	value, _ := ctx.Value(skillContextKey{}).(string)
	return strings.TrimSpace(value)
}

func mergeInstructionsWithSkillContext(ctx context.Context, instructions string) string {
	instructions = strings.TrimSpace(instructions)
	skillContext := SkillContextFromContext(ctx)
	if skillContext == "" {
		return instructions
	}
	return strings.TrimSpace(skillContext + "\n\n---\n\n" + instructions)
}
