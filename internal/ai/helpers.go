package ai

import "strings"

func splitRunes(text string, size int) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	runes := []rune(text)
	if len(runes) <= size {
		return []string{text}
	}
	out := make([]string, 0, (len(runes)+size-1)/size)
	for start := 0; start < len(runes); start += size {
		end := min(start+size, len(runes))
		out = append(out, strings.TrimSpace(string(runes[start:end])))
	}
	return out
}

func trimForPrompt(text string, maxRunes int) string {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) <= maxRunes {
		return string(runes)
	}
	return string(runes[:maxRunes]) + "..."
}

func normalizeSearchQueries(values []string) []string {
	var out []string
	seen := make(map[string]struct{})
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func normalizeConversationMessages(values []ConversationMessage) []ConversationMessage {
	out := make([]ConversationMessage, 0, len(values))
	for _, value := range values {
		role := strings.ToLower(strings.TrimSpace(value.Role))
		if role != "user" && role != "assistant" {
			continue
		}
		content := strings.TrimSpace(value.Content)
		if content == "" {
			continue
		}
		out = append(out, ConversationMessage{
			Role:    role,
			Content: content,
		})
	}
	return out
}

func NormalizeConversationMessages(values []ConversationMessage) []ConversationMessage {
	return normalizeConversationMessages(values)
}
