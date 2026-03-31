package weixin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"myclaw/internal/app"
)

type ConversationUpdate struct {
	SessionID string
	Activate  bool
}

type conversationSessionState struct {
	Sessions map[string]string `json:"sessions,omitempty"`
}

func (b *Bridge) conversationSlotKey(msg WeixinMessage) string {
	userID := strings.TrimSpace(msg.FromUserID)
	contextToken := strings.TrimSpace(msg.ContextToken)
	switch {
	case userID != "":
		return "user:" + userID
	case contextToken != "":
		return "context:" + contextToken
	default:
		return "default"
	}
}

func (b *Bridge) conversationSessionStatePath() string {
	return filepath.Join(b.config.DataDir, "weixin-bridge", "conversation_sessions.json")
}

func (b *Bridge) ensureConversationSessionsLocked() error {
	if b.conversationLoaded {
		if b.conversationSessions == nil {
			b.conversationSessions = make(map[string]string)
		}
		return nil
	}

	b.conversationLoaded = true
	b.conversationSessions = make(map[string]string)

	path := b.conversationSessionStatePath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if len(data) == 0 {
		return nil
	}

	var state conversationSessionState
	if err := json.Unmarshal(data, &state); err != nil {
		return err
	}
	for slot, sessionID := range state.Sessions {
		slot = strings.TrimSpace(slot)
		sessionID = strings.TrimSpace(sessionID)
		if slot == "" || sessionID == "" {
			continue
		}
		b.conversationSessions[slot] = sessionID
	}
	return nil
}

func (b *Bridge) saveConversationSessionsLocked() error {
	if err := os.MkdirAll(filepath.Dir(b.conversationSessionStatePath()), 0o755); err != nil {
		return err
	}
	state := conversationSessionState{
		Sessions: make(map[string]string, len(b.conversationSessions)),
	}
	for slot, sessionID := range b.conversationSessions {
		slot = strings.TrimSpace(slot)
		sessionID = strings.TrimSpace(sessionID)
		if slot == "" || sessionID == "" {
			continue
		}
		state.Sessions[slot] = sessionID
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	tmpPath := b.conversationSessionStatePath() + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpPath, b.conversationSessionStatePath())
}

func (b *Bridge) conversationContext(msg WeixinMessage, sessionID string) app.MessageContext {
	return app.MessageContext{
		UserID:    msg.FromUserID,
		Interface: "weixin",
		SessionID: strings.TrimSpace(sessionID),
	}
}

func (b *Bridge) conversationExists(ctx context.Context, mc app.MessageContext) (bool, error) {
	if b.service == nil {
		return false, nil
	}
	return b.service.ConversationExists(ctx, mc)
}

func (b *Bridge) resolveConversationLifecycleLocked(ctx context.Context, msg WeixinMessage, mode app.ConversationLifecycleMode) (app.ConversationLifecycleResult, string, error) {
	if err := b.ensureConversationSessionsLocked(); err != nil {
		return app.ConversationLifecycleResult{}, "", err
	}

	slot := b.conversationSlotKey(msg)
	boundID := strings.TrimSpace(b.conversationSessions[slot])
	legacyID := weixinSessionID(msg)

	boundExists := false
	if boundID != "" {
		ok, err := b.conversationExists(ctx, b.conversationContext(msg, boundID))
		if err != nil {
			return app.ConversationLifecycleResult{}, "", err
		}
		boundExists = ok
	}

	legacyExists := false
	switch {
	case legacyID == "":
	case legacyID == boundID:
		legacyExists = boundExists
	default:
		ok, err := b.conversationExists(ctx, b.conversationContext(msg, legacyID))
		if err != nil {
			return app.ConversationLifecycleResult{}, "", err
		}
		legacyExists = ok
	}
	if !legacyExists {
		contextLegacyID := legacyContextSessionID(msg)
		switch {
		case contextLegacyID == "":
		case contextLegacyID == legacyID:
		case contextLegacyID == boundID:
			legacyID = contextLegacyID
			legacyExists = boundExists
		default:
			ok, err := b.conversationExists(ctx, b.conversationContext(msg, contextLegacyID))
			if err != nil {
				return app.ConversationLifecycleResult{}, "", err
			}
			if ok {
				legacyID = contextLegacyID
				legacyExists = true
			}
		}
	}

	input := app.ConversationLifecycleInput{
		Mode:                     mode,
		BoundConversationID:      boundID,
		LegacyConversationID:     legacyID,
		BoundConversationExists:  boundExists,
		LegacyConversationExists: legacyExists,
	}
	if mode == app.ConversationLifecycleForceNew || (mode == app.ConversationLifecycleBindOrCreate && boundID != "" && !boundExists) {
		input.NextConversationID = newWeixinConversationSessionID(msg)
	}

	result, err := app.ResolveConversationLifecycle(input)
	return result, slot, err
}

func (b *Bridge) currentConversationContext(ctx context.Context, msg WeixinMessage) (app.MessageContext, error) {
	b.sessionMu.Lock()
	defer b.sessionMu.Unlock()

	result, _, err := b.resolveConversationLifecycleLocked(ctx, msg, app.ConversationLifecycleLookup)
	if err != nil {
		return app.MessageContext{}, err
	}
	return b.conversationContext(msg, result.ConversationID), nil
}

func (b *Bridge) ensureConversation(ctx context.Context, mc app.MessageContext) error {
	if b.service == nil {
		return nil
	}
	return b.service.EnsureConversation(ctx, mc)
}

func (b *Bridge) bindConversationSession(ctx context.Context, msg WeixinMessage) (app.MessageContext, string, bool, error) {
	b.sessionMu.Lock()
	defer b.sessionMu.Unlock()

	result, slot, err := b.resolveConversationLifecycleLocked(ctx, msg, app.ConversationLifecycleBindOrCreate)
	if err != nil {
		return app.MessageContext{}, "", false, err
	}

	if result.PersistBinding {
		b.conversationSessions[slot] = result.BindingConversationID
		if err := b.saveConversationSessionsLocked(); err != nil {
			return app.MessageContext{}, "", false, err
		}
	}
	if result.ClearInterfaceState {
		if b.fileSearch != nil {
			b.fileSearch.ClearPendingSelection(slot)
		}
	}

	mc := b.conversationContext(msg, result.ConversationID)
	if result.EnsureConversation {
		if err := b.ensureConversation(ctx, mc); err != nil {
			return app.MessageContext{}, "", false, err
		}
	}
	return mc, result.Notice, result.ActivateConversation, nil
}

func (b *Bridge) startNewConversation(ctx context.Context, msg WeixinMessage) (app.MessageContext, error) {
	b.sessionMu.Lock()
	defer b.sessionMu.Unlock()

	result, slot, err := b.resolveConversationLifecycleLocked(ctx, msg, app.ConversationLifecycleForceNew)
	if err != nil {
		return app.MessageContext{}, err
	}

	b.conversationSessions[slot] = result.BindingConversationID
	if err := b.saveConversationSessionsLocked(); err != nil {
		return app.MessageContext{}, err
	}
	if result.ClearInterfaceState {
		if b.fileSearch != nil {
			b.fileSearch.ClearPendingSelection(slot)
		}
	}

	mc := b.conversationContext(msg, result.ConversationID)
	if result.EnsureConversation {
		if err := b.ensureConversation(ctx, mc); err != nil {
			return app.MessageContext{}, err
		}
	}
	return mc, nil
}

func newWeixinConversationSessionID(msg WeixinMessage) string {
	return fmt.Sprintf("%s:%d", weixinSessionID(msg), time.Now().UnixNano())
}
