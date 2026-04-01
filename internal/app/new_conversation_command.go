package app

import (
	"errors"
	"strings"
)

const newConversationCommandUsage = "用法: /new [ask|agent]"

func ParseNewConversationMode(input string) (Mode, error) {
	text := strings.TrimSpace(normalizeSlash(input))
	if !IsNewConversationCommand(text) {
		return "", errors.New(newConversationCommandUsage)
	}

	fields := strings.Fields(text)
	switch len(fields) {
	case 1:
		return defaultMode(), nil
	case 2:
		switch strings.ToLower(strings.TrimSpace(fields[1])) {
		case string(ModeAsk):
			return ModeAsk, nil
		case string(ModeAgent):
			return ModeAgent, nil
		}
	}

	return "", errors.New(newConversationCommandUsage)
}
