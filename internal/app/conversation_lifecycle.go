package app

import (
	"fmt"
	"strings"
)

type ConversationLifecycleMode string

const (
	ConversationLifecycleLookup       ConversationLifecycleMode = "lookup"
	ConversationLifecycleBindOrCreate ConversationLifecycleMode = "bind_or_create"
	ConversationLifecycleForceNew     ConversationLifecycleMode = "force_new"
)

type ConversationLifecycleInput struct {
	Mode                     ConversationLifecycleMode
	BoundConversationID      string
	LegacyConversationID     string
	BoundConversationExists  bool
	LegacyConversationExists bool
	NextConversationID       string
}

type ConversationLifecycleResult struct {
	ConversationID        string
	BindingConversationID string
	PersistBinding        bool
	EnsureConversation    bool
	ActivateConversation  bool
	ClearInterfaceState   bool
	Notice                string
}

func ResolveConversationLifecycle(input ConversationLifecycleInput) (ConversationLifecycleResult, error) {
	boundID := strings.TrimSpace(input.BoundConversationID)
	legacyID := strings.TrimSpace(input.LegacyConversationID)
	nextID := strings.TrimSpace(input.NextConversationID)

	switch input.Mode {
	case ConversationLifecycleForceNew:
		if nextID == "" {
			return ConversationLifecycleResult{}, fmt.Errorf("next conversation id is required for force_new")
		}
		return ConversationLifecycleResult{
			ConversationID:        nextID,
			BindingConversationID: nextID,
			PersistBinding:        true,
			EnsureConversation:    true,
			ActivateConversation:  true,
			ClearInterfaceState:   true,
		}, nil

	case ConversationLifecycleLookup:
		switch {
		case boundID != "" && input.BoundConversationExists:
			return ConversationLifecycleResult{ConversationID: boundID}, nil
		case legacyID != "" && input.LegacyConversationExists:
			return ConversationLifecycleResult{ConversationID: legacyID}, nil
		default:
			return ConversationLifecycleResult{ConversationID: legacyID}, nil
		}

	case ConversationLifecycleBindOrCreate:
		switch {
		case boundID != "" && input.BoundConversationExists:
			return ConversationLifecycleResult{ConversationID: boundID}, nil

		case boundID != "" && !input.BoundConversationExists:
			if nextID == "" {
				return ConversationLifecycleResult{}, fmt.Errorf("next conversation id is required when bound conversation is missing")
			}
			return ConversationLifecycleResult{
				ConversationID:        nextID,
				BindingConversationID: nextID,
				PersistBinding:        true,
				EnsureConversation:    true,
				ActivateConversation:  true,
				ClearInterfaceState:   true,
				Notice:                "之前对话已丢失，已进入新对话。",
			}, nil

		case legacyID != "" && input.LegacyConversationExists:
			return ConversationLifecycleResult{
				ConversationID:        legacyID,
				BindingConversationID: legacyID,
				PersistBinding:        true,
			}, nil

		default:
			return ConversationLifecycleResult{
				ConversationID:        legacyID,
				BindingConversationID: legacyID,
				PersistBinding:        legacyID != "",
				EnsureConversation:    legacyID != "",
				ActivateConversation:  legacyID != "",
				ClearInterfaceState:   legacyID != "",
				Notice:                "当前对话已开始。",
			}, nil
		}

	default:
		return ConversationLifecycleResult{}, fmt.Errorf("unsupported conversation lifecycle mode %q", input.Mode)
	}
}
