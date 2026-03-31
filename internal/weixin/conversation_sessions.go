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
	case userID != "" && contextToken != "":
		return "user:" + userID + "|context:" + contextToken
	case contextToken != "":
		return "context:" + contextToken
	case userID != "":
		return "user:" + userID
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

func (b *Bridge) ensureConversation(ctx context.Context, mc app.MessageContext) error {
	if b.service == nil {
		return nil
	}
	return b.service.EnsureConversation(ctx, mc)
}

func (b *Bridge) bindConversationSession(ctx context.Context, msg WeixinMessage) (app.MessageContext, string, bool, error) {
	b.sessionMu.Lock()
	defer b.sessionMu.Unlock()

	if err := b.ensureConversationSessionsLocked(); err != nil {
		return app.MessageContext{}, "", false, err
	}

	slot := b.conversationSlotKey(msg)
	legacySessionID := weixinSessionID(msg)
	if sessionID := strings.TrimSpace(b.conversationSessions[slot]); sessionID != "" {
		mc := b.conversationContext(msg, sessionID)
		ok, err := b.conversationExists(ctx, mc)
		if err != nil {
			return app.MessageContext{}, "", false, err
		}
		if ok {
			return mc, "", false, nil
		}

		nextSessionID := newWeixinConversationSessionID(msg)
		b.conversationSessions[slot] = nextSessionID
		if err := b.saveConversationSessionsLocked(); err != nil {
			return app.MessageContext{}, "", false, err
		}
		b.clearPendingFileSelection(slot)

		mc = b.conversationContext(msg, nextSessionID)
		if err := b.ensureConversation(ctx, mc); err != nil {
			return app.MessageContext{}, "", false, err
		}
		return mc, "之前对话已丢失，已进入新对话。", true, nil
	}

	legacyContext := b.conversationContext(msg, legacySessionID)
	ok, err := b.conversationExists(ctx, legacyContext)
	if err != nil {
		return app.MessageContext{}, "", false, err
	}

	b.conversationSessions[slot] = legacySessionID
	if err := b.saveConversationSessionsLocked(); err != nil {
		return app.MessageContext{}, "", false, err
	}

	if ok {
		return legacyContext, "", false, nil
	}

	if err := b.ensureConversation(ctx, legacyContext); err != nil {
		return app.MessageContext{}, "", false, err
	}
	return legacyContext, "当前对话已开始。", true, nil
}

func (b *Bridge) startNewConversation(ctx context.Context, msg WeixinMessage) (app.MessageContext, error) {
	b.sessionMu.Lock()
	defer b.sessionMu.Unlock()

	if err := b.ensureConversationSessionsLocked(); err != nil {
		return app.MessageContext{}, err
	}

	slot := b.conversationSlotKey(msg)
	sessionID := newWeixinConversationSessionID(msg)
	b.conversationSessions[slot] = sessionID
	if err := b.saveConversationSessionsLocked(); err != nil {
		return app.MessageContext{}, err
	}
	b.clearPendingFileSelection(slot)

	mc := b.conversationContext(msg, sessionID)
	if err := b.ensureConversation(ctx, mc); err != nil {
		return app.MessageContext{}, err
	}
	return mc, nil
}

func newWeixinConversationSessionID(msg WeixinMessage) string {
	return fmt.Sprintf("%s:%d", weixinSessionID(msg), time.Now().UnixNano())
}
