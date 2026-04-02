package app

import (
	"context"
	"fmt"
	"strings"

	"baize/internal/ai"
	"baize/internal/promptlib"
)

func (s *Service) handlePromptCommand(ctx context.Context, mc MessageContext, input string) (string, error) {
	if s.promptStore == nil {
		return "Prompt profile 功能未启用。", nil
	}

	fields := strings.Fields(input)
	if len(fields) == 1 {
		return s.formatCurrentPromptProfile(ctx, mc)
	}

	switch strings.ToLower(fields[1]) {
	case "current":
		return s.formatCurrentPromptProfile(ctx, mc)
	case "list":
		return s.listPromptProfiles(ctx, mc)
	case "use", "select":
		if len(fields) < 3 {
			return promptProfileUsage(), nil
		}
		prompt, err := s.SetPromptProfile(ctx, mc, fields[2])
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("已为当前会话启用 Prompt profile #%s %s。", shortID(prompt.ID), strings.TrimSpace(prompt.Title)), nil
	case "clear", "unset":
		if err := s.ClearPromptProfile(ctx, mc); err != nil {
			return "", err
		}
		return "已清除当前会话的 Prompt profile。", nil
	default:
		return promptProfileUsage(), nil
	}
}

func (s *Service) CurrentPromptProfile(ctx context.Context, mc MessageContext) (promptlib.Prompt, bool, error) {
	if s.promptStore == nil {
		return promptlib.Prompt{}, false, nil
	}
	promptID := s.selectedPromptID(ctx, mc)
	if promptID == "" {
		return promptlib.Prompt{}, false, nil
	}
	prompt, ok, err := s.promptStore.Resolve(ctx, promptID)
	if err != nil {
		return promptlib.Prompt{}, false, err
	}
	return prompt, ok, nil
}

func (s *Service) SetPromptProfile(ctx context.Context, mc MessageContext, idOrPrefix string) (promptlib.Prompt, error) {
	if s.promptStore == nil {
		return promptlib.Prompt{}, fmt.Errorf("prompt store is not enabled")
	}
	prompt, ok, err := s.promptStore.Resolve(ctx, idOrPrefix)
	if err != nil {
		return promptlib.Prompt{}, err
	}
	if !ok {
		return promptlib.Prompt{}, fmt.Errorf("没有找到 Prompt #%s。", strings.TrimPrefix(strings.TrimSpace(idOrPrefix), "#"))
	}
	if err := s.setSelectedPromptID(ctx, mc, prompt.ID); err != nil {
		return promptlib.Prompt{}, err
	}
	return prompt, nil
}

func (s *Service) ClearPromptProfile(ctx context.Context, mc MessageContext) error {
	return s.setSelectedPromptID(ctx, mc, "")
}

func (s *Service) formatCurrentPromptProfile(ctx context.Context, mc MessageContext) (string, error) {
	prompt, ok, err := s.CurrentPromptProfile(ctx, mc)
	if err != nil {
		return "", err
	}
	if !ok {
		return "当前会话未启用 Prompt profile。\n\n" + promptProfileUsage(), nil
	}
	return fmt.Sprintf("当前 Prompt profile:\n#%s %s\n%s", shortID(prompt.ID), strings.TrimSpace(prompt.Title), preview(prompt.Content, maxReplyPreviewRunes)), nil
}

func (s *Service) listPromptProfiles(ctx context.Context, mc MessageContext) (string, error) {
	prompts, err := s.promptStore.List(ctx)
	if err != nil {
		return "", err
	}
	if len(prompts) == 0 {
		return "当前没有可用 Prompt。", nil
	}

	selectedID := s.selectedPromptID(ctx, mc)
	var builder strings.Builder
	builder.WriteString("可用 Prompt profiles：\n")
	for _, prompt := range prompts {
		builder.WriteString("- #")
		builder.WriteString(shortID(prompt.ID))
		builder.WriteString(" ")
		builder.WriteString(strings.TrimSpace(prompt.Title))
		if prompt.ID == selectedID {
			builder.WriteString(" [当前]")
		}
		if content := preview(prompt.Content, 80); content != "" {
			builder.WriteString("：")
			builder.WriteString(content)
		}
		builder.WriteString("\n")
	}
	return strings.TrimSpace(builder.String()), nil
}

func (s *Service) withSkillContext(ctx context.Context, mc MessageContext) context.Context {
	sections := make([]string, 0, 2)

	if prompt, ok, err := s.CurrentPromptProfile(ctx, mc); err == nil && ok {
		var builder strings.Builder
		builder.WriteString("Prompt profile selected for this conversation. Follow it when relevant.\n\n")
		builder.WriteString("## ")
		builder.WriteString(strings.TrimSpace(prompt.Title))
		builder.WriteString("\n")
		builder.WriteString(strings.TrimSpace(prompt.Content))
		sections = append(sections, strings.TrimSpace(builder.String()))
	}

	skills := s.loadedSkillsFor(mc)
	if len(skills) > 0 {
		var builder strings.Builder
		builder.WriteString("Loaded skills for this conversation. Apply them only when relevant.\n\n")
		for _, skill := range skills {
			builder.WriteString("## ")
			builder.WriteString(skill.Name)
			builder.WriteString("\n")
			builder.WriteString(strings.TrimSpace(skill.Content))
			builder.WriteString("\n\n")
		}
		sections = append(sections, strings.TrimSpace(builder.String()))
	}

	if len(sections) == 0 {
		return ctx
	}
	return ai.WithSkillContext(ctx, strings.Join(sections, "\n\n---\n\n"))
}

func promptProfileUsage() string {
	return "用法:\n" +
		"/prompt\n" +
		"/prompt current\n" +
		"/prompt list\n" +
		"/prompt use <PromptID前缀>\n" +
		"/prompt clear"
}
