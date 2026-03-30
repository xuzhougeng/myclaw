package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"myclaw/internal/ai"
	appsvc "myclaw/internal/app"
	"myclaw/internal/knowledge"
	"myclaw/internal/sessionstate"
)

const (
	maxChatConversationTitleRunes   = 28
	maxChatConversationPreviewRunes = 72
)

type ChatConversation struct {
	SessionID     string `json:"sessionId"`
	Title         string `json:"title"`
	Preview       string `json:"preview"`
	UpdatedAt     string `json:"updatedAt"`
	UpdatedAtUnix int64  `json:"updatedAtUnix"`
	MessageCount  int    `json:"messageCount"`
	HasMessages   bool   `json:"hasMessages"`
	Active        bool   `json:"active"`
}

type ChatMessage struct {
	Role  string         `json:"role"`
	Text  string         `json:"text"`
	Time  string         `json:"time"`
	Usage *ai.TokenUsage `json:"usage,omitempty"`
}

type ChatState struct {
	SessionID     string             `json:"sessionId"`
	Conversations []ChatConversation `json:"conversations"`
	Messages      []ChatMessage      `json:"messages"`
}

type chatSessionSnapshot struct {
	SessionID string
	Snapshot  sessionstate.Snapshot
}

func (a *DesktopApp) GetChatState() (ChatState, error) {
	project, err := a.currentProject(context.Background())
	if err != nil {
		return ChatState{}, err
	}
	return a.buildChatState(context.Background(), project)
}

func (a *DesktopApp) NewChatSession() (ChatState, error) {
	project, err := a.currentProject(context.Background())
	if err != nil {
		return ChatState{}, err
	}

	sessionID := newDesktopChatSessionID(project)
	if err := a.ensureChatSession(context.Background(), project, sessionID); err != nil {
		return ChatState{}, err
	}
	a.rememberChatSession(project, sessionID)
	return a.buildChatState(context.Background(), project)
}

func (a *DesktopApp) SwitchChatSession(sessionID string) (ChatState, error) {
	project, err := a.currentProject(context.Background())
	if err != nil {
		return ChatState{}, err
	}

	sessionID = strings.TrimSpace(sessionID)
	if !isDesktopChatSessionForProject(sessionID, project) {
		return ChatState{}, errors.New("无效的对话 ID")
	}

	snapshot, ok, err := a.loadChatSessionSnapshot(context.Background(), project, sessionID)
	if err != nil {
		return ChatState{}, err
	}
	if !ok {
		return ChatState{}, errors.New("未找到要切换的对话")
	}
	if a.sessionStore != nil {
		if _, err := a.sessionStore.Save(context.Background(), snapshot); err != nil {
			return ChatState{}, err
		}
	}
	a.rememberChatSession(project, sessionID)
	return a.buildChatState(context.Background(), project)
}

func (a *DesktopApp) buildChatState(ctx context.Context, project string) (ChatState, error) {
	project = knowledge.CanonicalProjectName(project)
	sessionID, err := a.currentChatSession(ctx, project)
	if err != nil {
		return ChatState{}, err
	}

	sessions, err := a.listProjectChatSessions(ctx, project)
	if err != nil {
		return ChatState{}, err
	}
	if len(sessions) == 0 {
		return ChatState{
			SessionID: sessionID,
			Conversations: []ChatConversation{
				{
					SessionID:    sessionID,
					Title:        "新对话",
					Preview:      "还没有消息",
					MessageCount: 0,
					HasMessages:  false,
					Active:       true,
				},
			},
			Messages: nil,
		}, nil
	}

	state := ChatState{
		SessionID:     sessionID,
		Conversations: make([]ChatConversation, 0, len(sessions)),
		Messages:      nil,
	}
	activeFound := false
	for _, item := range sessions {
		active := item.SessionID == sessionID
		if active {
			activeFound = true
			state.Messages = toChatMessages(item.Snapshot)
		}
		state.Conversations = append(state.Conversations, toChatConversation(item.SessionID, item.Snapshot, active))
	}
	if activeFound {
		return state, nil
	}

	if snapshot, ok, err := a.loadChatSessionSnapshot(ctx, project, sessionID); err != nil {
		return ChatState{}, err
	} else if ok {
		state.Messages = toChatMessages(snapshot)
		state.Conversations = append([]ChatConversation{toChatConversation(sessionID, snapshot, true)}, state.Conversations...)
	}
	return state, nil
}

func (a *DesktopApp) chatMessageContext(ctx context.Context, project string) (appsvc.MessageContext, error) {
	sessionID, err := a.currentChatSession(ctx, project)
	if err != nil {
		return appsvc.MessageContext{}, err
	}
	return desktopMessageContext(project, sessionID), nil
}

func (a *DesktopApp) currentChatSession(ctx context.Context, project string) (string, error) {
	project = knowledge.CanonicalProjectName(project)
	if sessionID := a.rememberedChatSession(project); sessionID != "" {
		return sessionID, nil
	}

	sessions, err := a.listProjectChatSessions(ctx, project)
	if err != nil {
		return "", err
	}
	if len(sessions) > 0 {
		a.rememberChatSession(project, sessions[0].SessionID)
		return sessions[0].SessionID, nil
	}

	sessionID := newDesktopChatSessionID(project)
	if err := a.ensureChatSession(ctx, project, sessionID); err != nil {
		return "", err
	}
	a.rememberChatSession(project, sessionID)
	return sessionID, nil
}

func (a *DesktopApp) rememberedChatSession(project string) string {
	a.chatSessionMu.RLock()
	defer a.chatSessionMu.RUnlock()
	return strings.TrimSpace(a.chatSessionMap[knowledge.CanonicalProjectName(project)])
}

func (a *DesktopApp) rememberChatSession(project, sessionID string) {
	project = knowledge.CanonicalProjectName(project)
	sessionID = strings.TrimSpace(sessionID)
	if project == "" || sessionID == "" {
		return
	}

	a.chatSessionMu.Lock()
	defer a.chatSessionMu.Unlock()
	if a.chatSessionMap == nil {
		a.chatSessionMap = make(map[string]string)
	}
	a.chatSessionMap[project] = sessionID
}

func (a *DesktopApp) ensureChatSession(ctx context.Context, project, sessionID string) error {
	if a.sessionStore == nil {
		return nil
	}
	if _, ok, err := a.loadChatSessionSnapshot(ctx, project, sessionID); err != nil {
		return err
	} else if ok {
		return nil
	}
	_, err := a.sessionStore.Save(ctx, sessionstate.Snapshot{
		Key: desktopConversationSnapshotKey(project, sessionID),
	})
	return err
}

func (a *DesktopApp) loadChatSessionSnapshot(ctx context.Context, project, sessionID string) (sessionstate.Snapshot, bool, error) {
	if a.sessionStore == nil {
		return sessionstate.Snapshot{}, false, nil
	}
	return a.sessionStore.Load(ctx, desktopConversationSnapshotKey(project, sessionID))
}

func (a *DesktopApp) listProjectChatSessions(ctx context.Context, project string) ([]chatSessionSnapshot, error) {
	if a.sessionStore == nil {
		return nil, nil
	}

	items, err := a.sessionStore.List(ctx)
	if err != nil {
		return nil, err
	}

	project = knowledge.CanonicalProjectName(project)
	out := make([]chatSessionSnapshot, 0, len(items))
	for _, item := range items {
		itemProject, sessionID, ok := parseDesktopConversationKey(item.Key)
		if !ok {
			continue
		}
		if !strings.EqualFold(itemProject, project) {
			continue
		}
		if !isDesktopChatSessionForProject(sessionID, project) {
			continue
		}
		out = append(out, chatSessionSnapshot{
			SessionID: sessionID,
			Snapshot:  item,
		})
	}
	return out, nil
}

func desktopConversationSnapshotKey(project, sessionID string) string {
	project = knowledge.CanonicalProjectName(project)
	sessionID = strings.TrimSpace(sessionID)
	return strings.Join([]string{
		"source:" + desktopSourceLabel(),
		"session:" + sessionID,
		"project:" + strings.ToLower(project),
	}, "|")
}

func parseDesktopConversationKey(key string) (project string, sessionID string, ok bool) {
	parts := strings.Split(strings.TrimSpace(key), "|")
	var source string
	for _, part := range parts {
		switch {
		case strings.HasPrefix(part, "source:"):
			source = strings.TrimSpace(strings.TrimPrefix(part, "source:"))
		case strings.HasPrefix(part, "session:"):
			sessionID = strings.TrimSpace(strings.TrimPrefix(part, "session:"))
		case strings.HasPrefix(part, "project:"):
			project = strings.TrimSpace(strings.TrimPrefix(part, "project:"))
		}
	}
	if source != desktopSourceLabel() || sessionID == "" || project == "" {
		return "", "", false
	}
	return project, sessionID, true
}

func desktopChatSessionBase(project string) string {
	return desktopChatSessionID + ":" + knowledge.CanonicalProjectName(project)
}

func newDesktopChatSessionID(project string) string {
	return fmt.Sprintf("%s:%d", desktopChatSessionBase(project), time.Now().UnixNano())
}

func isDesktopChatSessionForProject(sessionID, project string) bool {
	base := desktopChatSessionBase(project)
	return sessionID == base || strings.HasPrefix(sessionID, base+":")
}

func toChatConversation(sessionID string, snapshot sessionstate.Snapshot, active bool) ChatConversation {
	updatedAt := strings.TrimSpace(snapshot.UpdatedAt.Local().Format("2006-01-02 15:04:05"))
	if snapshot.UpdatedAt.IsZero() {
		updatedAt = ""
	}
	return ChatConversation{
		SessionID:     sessionID,
		Title:         chatConversationTitle(snapshot.History),
		Preview:       chatConversationPreview(snapshot.History),
		UpdatedAt:     updatedAt,
		UpdatedAtUnix: snapshot.UpdatedAt.Unix(),
		MessageCount:  len(snapshot.History),
		HasMessages:   len(snapshot.History) > 0,
		Active:        active,
	}
}

func toChatMessages(snapshot sessionstate.Snapshot) []ChatMessage {
	messages := make([]ChatMessage, 0, len(snapshot.History))
	for _, item := range snapshot.History {
		role := strings.TrimSpace(item.Role)
		text := strings.TrimSpace(item.Content)
		if role == "" || text == "" {
			continue
		}
		messages = append(messages, ChatMessage{
			Role:  role,
			Text:  text,
			Time:  "",
			Usage: item.Usage,
		})
	}
	return messages
}

func chatConversationTitle(history []sessionstate.Message) string {
	for _, item := range history {
		text := strings.TrimSpace(item.Content)
		if strings.EqualFold(strings.TrimSpace(item.Role), "user") && text != "" {
			return preview(text, maxChatConversationTitleRunes)
		}
	}
	return "新对话"
}

func chatConversationPreview(history []sessionstate.Message) string {
	for index := len(history) - 1; index >= 0; index-- {
		text := strings.TrimSpace(history[index].Content)
		if text != "" {
			return preview(text, maxChatConversationPreviewRunes)
		}
	}
	return "还没有消息"
}
