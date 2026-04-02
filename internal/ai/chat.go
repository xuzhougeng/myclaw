package ai

import (
	"context"
	"fmt"
	"strings"

	"baize/internal/knowledge"
)

func (s *Service) Answer(ctx context.Context, question string, entries []knowledge.Entry) (string, error) {
	cfg, err := s.requireConfig(ctx)
	if err != nil {
		return "", err
	}

	var prompt strings.Builder
	prompt.WriteString("用户问题：\n")
	prompt.WriteString(strings.TrimSpace(question))
	prompt.WriteString("\n\n知识库内容：\n")
	if len(entries) == 0 {
		prompt.WriteString("(空)\n")
	} else {
		for index, entry := range entries {
			source := strings.TrimSpace(entry.Source)
			if source == "" {
				source = "unknown"
			}
			prompt.WriteString(fmt.Sprintf("%d. [%s] [%s] %s\n",
				index+1,
				entry.RecordedAt.Local().Format("2006-01-02 15:04:05"),
				source,
				entry.Text,
			))
		}
	}

	instructions := strings.TrimSpace(`
You are baize, a private knowledge-base assistant.
Answer in Chinese unless the user clearly asks otherwise.
Use only the provided knowledge base content.
If the knowledge base is insufficient, say so directly.
When helpful, cite the relevant memory item numbers.
Keep the answer concise but useful.
`)

	return s.generateText(ctx, cfg, instructions, prompt.String())
}

func (s *Service) AnswerStream(ctx context.Context, question string, entries []knowledge.Entry, onDelta func(string)) (string, error) {
	cfg, err := s.requireConfig(ctx)
	if err != nil {
		return "", err
	}

	var prompt strings.Builder
	prompt.WriteString("用户问题：\n")
	prompt.WriteString(strings.TrimSpace(question))
	prompt.WriteString("\n\n知识库内容：\n")
	if len(entries) == 0 {
		prompt.WriteString("(空)\n")
	} else {
		for index, entry := range entries {
			source := strings.TrimSpace(entry.Source)
			if source == "" {
				source = "unknown"
			}
			prompt.WriteString(fmt.Sprintf("%d. [%s] [%s] %s\n",
				index+1,
				entry.RecordedAt.Local().Format("2006-01-02 15:04:05"),
				source,
				entry.Text,
			))
		}
	}

	instructions := strings.TrimSpace(`
You are baize, a private knowledge-base assistant.
Answer in Chinese unless the user clearly asks otherwise.
Use only the provided knowledge base content.
If the knowledge base is insufficient, say so directly.
When helpful, cite the relevant memory item numbers.
Keep the answer concise but useful.
`)

	return s.generateTextStream(ctx, cfg, instructions, prompt.String(), onDelta)
}

func (s *Service) Chat(ctx context.Context, input string, history []ConversationMessage) (string, error) {
	cfg, err := s.requireConfig(ctx)
	if err != nil {
		return "", err
	}

	instructions := strings.TrimSpace(`
You are baize, a private AI workspace assistant.
Answer in Chinese unless the user clearly asks otherwise.
Be concise, practical, and direct.
Do not claim to have consulted a knowledge base unless one was explicitly provided.
`)

	messages := make([]responseInputMessage, 0, len(history)+1)
	for _, item := range normalizeConversationMessages(history) {
		messages = append(messages, newTextMessage(item.Role, item.Content))
	}
	messages = append(messages, newTextMessage("user", strings.TrimSpace(input)))

	req := generationRequest{
		Instructions:    mergeInstructionsWithSkillContext(ctx, instructions),
		Input:           messages,
		MaxOutputTokens: 1500,
	}
	return s.generate(ctx, cfg, req)
}

func (s *Service) ChatStream(ctx context.Context, input string, history []ConversationMessage, onDelta func(string)) (string, error) {
	cfg, err := s.requireConfig(ctx)
	if err != nil {
		return "", err
	}

	instructions := strings.TrimSpace(`
You are baize, a private AI workspace assistant.
Answer in Chinese unless the user clearly asks otherwise.
Be concise, practical, and direct.
Do not claim to have consulted a knowledge base unless one was explicitly provided.
`)

	messages := make([]responseInputMessage, 0, len(history)+1)
	for _, item := range normalizeConversationMessages(history) {
		messages = append(messages, newTextMessage(item.Role, item.Content))
	}
	messages = append(messages, newTextMessage("user", strings.TrimSpace(input)))

	req := generationRequest{
		Instructions:    mergeInstructionsWithSkillContext(ctx, instructions),
		Input:           messages,
		MaxOutputTokens: 1500,
	}
	return s.generateStream(ctx, cfg, req, onDelta)
}

func (s *Service) TranslateToChinese(ctx context.Context, input string) (string, error) {
	cfg, err := s.requireConfig(ctx)
	if err != nil {
		return "", err
	}

	instructions := strings.TrimSpace(`
You are baize's translation mode.
Translate the user's input into natural, fluent Simplified Chinese.
Preserve factual meaning, tone, names, technical terms, formatting, and line breaks whenever possible.
Do not explain, do not summarize, and do not add commentary.
Return only the Chinese translation.
`)

	return s.generateText(ctx, cfg, instructions, strings.TrimSpace(input))
}
