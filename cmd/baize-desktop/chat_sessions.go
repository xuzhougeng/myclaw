package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"baize/internal/ai"
	appsvc "baize/internal/app"
	"baize/internal/knowledge"
	"baize/internal/sessionstate"
)

const (
	maxChatConversationTitleRunes   = 28
	maxChatConversationPreviewRunes = 72
)

type ChatConversation struct {
	SessionID     string `json:"sessionId"`
	Title         string `json:"title"`
	CustomTitle   bool   `json:"customTitle"`
	Preview       string `json:"preview"`
	Source        string `json:"source"`
	SourceLabel   string `json:"sourceLabel"`
	Mode          string `json:"mode"`
	ReadOnly      bool   `json:"readOnly"`
	UpdatedAt     string `json:"updatedAt"`
	UpdatedAtUnix int64  `json:"updatedAtUnix"`
	MessageCount  int    `json:"messageCount"`
	HasMessages   bool   `json:"hasMessages"`
	Active        bool   `json:"active"`
}

type ChatMessage struct {
	Role    string             `json:"role"`
	Text    string             `json:"text"`
	Time    string             `json:"time"`
	Usage   *ai.TokenUsage     `json:"usage,omitempty"`
	Process []ai.CallTraceStep `json:"process,omitempty"`
}

type ChatState struct {
	SessionID     string             `json:"sessionId"`
	Conversations []ChatConversation `json:"conversations"`
	Messages      []ChatMessage      `json:"messages"`
}

type chatSessionSnapshot struct {
	SessionID   string
	SnapshotKey string
	Source      string
	SourceLabel string
	ReadOnly    bool
	Snapshot    sessionstate.Snapshot
}

func (a *DesktopApp) GetChatState() (ChatState, error) {
	project, err := a.currentProject(context.Background())
	if err != nil {
		return ChatState{}, err
	}
	return a.buildChatState(context.Background(), project)
}

func (a *DesktopApp) NewChatSession(mode string) (ChatState, error) {
	project, err := a.currentProject(context.Background())
	if err != nil {
		return ChatState{}, err
	}

	sessionID := newDesktopChatSessionID(project)
	if err := a.ensureChatSession(context.Background(), project, sessionID); err != nil {
		return ChatState{}, err
	}
	if err := a.setChatSessionMode(context.Background(), project, sessionID, mode); err != nil {
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
	session, ok, err := a.findChatSession(context.Background(), project, sessionID)
	if err != nil {
		return ChatState{}, err
	}
	if !ok {
		return ChatState{}, errors.New("未找到要切换的对话")
	}
	if a.sessionStore != nil {
		if _, err := a.sessionStore.Save(context.Background(), session.Snapshot); err != nil {
			return ChatState{}, err
		}
	}
	a.rememberChatSession(project, sessionID)
	return a.buildChatState(context.Background(), project)
}

func (a *DesktopApp) RenameChatSession(sessionID, title string) (ChatState, error) {
	if a.sessionStore == nil {
		return ChatState{}, errors.New("聊天会话存储尚未启用")
	}

	project, err := a.currentProject(context.Background())
	if err != nil {
		return ChatState{}, err
	}

	sessionID = strings.TrimSpace(sessionID)
	title = strings.TrimSpace(title)
	if title == "" {
		return ChatState{}, errors.New("对话标题不能为空")
	}

	session, ok, err := a.findChatSession(context.Background(), project, sessionID)
	if err != nil {
		return ChatState{}, err
	}
	if !ok {
		return ChatState{}, errors.New("未找到要重命名的对话")
	}

	session.Snapshot.Title = title
	if _, err := a.sessionStore.Save(context.Background(), session.Snapshot); err != nil {
		return ChatState{}, err
	}
	a.rememberChatSession(project, sessionID)
	return a.buildChatState(context.Background(), project)
}

func (a *DesktopApp) DeleteChatSession(sessionID string) (ChatState, error) {
	if a.sessionStore == nil {
		return ChatState{}, errors.New("聊天会话存储尚未启用")
	}

	project, err := a.currentProject(context.Background())
	if err != nil {
		return ChatState{}, err
	}

	sessionID = strings.TrimSpace(sessionID)
	session, ok, err := a.findChatSession(context.Background(), project, sessionID)
	if err != nil {
		return ChatState{}, err
	}
	if !ok {
		return ChatState{}, errors.New("未找到要删除的对话")
	}

	currentSessionID, err := a.currentChatSession(context.Background(), project)
	if err != nil {
		return ChatState{}, err
	}

	if err := a.sessionStore.Delete(context.Background(), session.SnapshotKey); err != nil {
		return ChatState{}, err
	}

	remaining, err := a.listChatSessions(context.Background(), project)
	if err != nil {
		return ChatState{}, err
	}
	if nextSessionID, ok := firstChatSession(remaining); ok {
		if currentSessionID == sessionID || a.rememberedChatSession(project) == sessionID {
			a.rememberChatSession(project, nextSessionID)
		}
		return a.buildChatState(context.Background(), project)
	}

	nextSessionID := newDesktopChatSessionID(project)
	if err := a.ensureChatSession(context.Background(), project, nextSessionID); err != nil {
		return ChatState{}, err
	}
	a.rememberChatSession(project, nextSessionID)
	return a.buildChatState(context.Background(), project)
}

func (a *DesktopApp) RefreshChatResponse() (ChatResponse, error) {
	if a.sessionStore == nil {
		return ChatResponse{}, errors.New("聊天会话存储尚未启用")
	}

	project, err := a.currentProject(context.Background())
	if err != nil {
		return ChatResponse{}, err
	}

	session, err := a.currentChatConversation(context.Background(), project)
	if err != nil {
		return ChatResponse{}, err
	}
	if strings.TrimSpace(session.Snapshot.Key) == "" {
		return ChatResponse{}, errors.New("当前对话没有可刷新的回复")
	}

	trimmedSnapshot, input, err := trimmedChatSnapshotForRefresh(session.Snapshot)
	if err != nil {
		return ChatResponse{}, err
	}

	if _, err := a.sessionStore.Save(context.Background(), trimmedSnapshot); err != nil {
		return ChatResponse{}, err
	}
	a.rememberChatSession(project, session.SessionID)

	result, err := a.sendMessage(context.Background(), input, nil, nil)
	if err != nil {
		if _, restoreErr := a.sessionStore.Save(context.Background(), session.Snapshot); restoreErr != nil {
			return ChatResponse{}, fmt.Errorf("%w；恢复原回复失败: %v", err, restoreErr)
		}
		return ChatResponse{}, err
	}
	return result, nil
}

func (a *DesktopApp) buildChatState(ctx context.Context, project string) (ChatState, error) {
	project = knowledge.CanonicalProjectName(project)
	sessionID, err := a.currentChatSession(ctx, project)
	if err != nil {
		return ChatState{}, err
	}

	sessions, err := a.listChatSessions(ctx, project)
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
					Source:       desktopInterface,
					SourceLabel:  "桌面",
					Mode:         string(appsvc.ModeAgent),
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
		state.Conversations = append(state.Conversations, toChatConversation(item, active))
	}
	if activeFound {
		return state, nil
	}

	if item, ok, err := a.findChatSession(ctx, project, sessionID); err != nil {
		return ChatState{}, err
	} else if ok {
		state.Messages = toChatMessages(item.Snapshot)
		state.Conversations = append([]ChatConversation{toChatConversation(item, true)}, state.Conversations...)
	}
	return state, nil
}

func (a *DesktopApp) chatMessageContext(ctx context.Context, project string) (appsvc.MessageContext, error) {
	session, err := a.currentChatConversation(ctx, project)
	if err != nil {
		return appsvc.MessageContext{}, err
	}
	if !isDesktopConversationSource(session.Source) {
		return externalMessageContext(session.Source, session.SessionID), nil
	}
	return desktopMessageContext(project, session.SessionID), nil
}

func (a *DesktopApp) currentChatSession(ctx context.Context, project string) (string, error) {
	project = knowledge.CanonicalProjectName(project)
	sessions, err := a.listChatSessions(ctx, project)
	if err != nil {
		return "", err
	}

	if sessionID := a.rememberedChatSession(project); sessionID != "" {
		for _, item := range sessions {
			if item.SessionID == sessionID {
				return sessionID, nil
			}
		}
	}

	if sessionID, ok := firstChatSession(sessions); ok {
		a.rememberChatSession(project, sessionID)
		return sessionID, nil
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
	if a.chatSessionMap == nil {
		a.chatSessionMap = make(map[string]string)
	}
	a.chatSessionMap[project] = sessionID
	a.chatSessionMu.Unlock()

	a.persistChatSessionSelection(project, sessionID)
}

func (a *DesktopApp) persistChatSessionSelection(project, sessionID string) {
	if a.settingsStore == nil {
		return
	}
	cfg, _, err := a.settingsStore.Load()
	if err != nil {
		return
	}
	if cfg.DesktopChatSessions == nil {
		cfg.DesktopChatSessions = make(map[string]string)
	}
	cfg.DesktopChatSessions[strings.ToLower(knowledge.CanonicalProjectName(project))] = strings.TrimSpace(sessionID)
	_ = a.settingsStore.Save(cfg)
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
		Key:  desktopConversationSnapshotKey(project, sessionID),
		Mode: string(appsvc.ModeAgent),
	})
	return err
}

func (a *DesktopApp) setChatSessionMode(ctx context.Context, project, sessionID, mode string) error {
	if a.sessionStore == nil {
		return nil
	}

	mode = normalizeNewChatMode(mode)
	snapshot, ok, err := a.loadChatSessionSnapshot(ctx, project, sessionID)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("未找到要设置模式的对话")
	}
	snapshot.Mode = mode
	_, err = a.sessionStore.Save(ctx, snapshot)
	return err
}

func (a *DesktopApp) loadChatSessionSnapshot(ctx context.Context, project, sessionID string) (sessionstate.Snapshot, bool, error) {
	if a.sessionStore == nil {
		return sessionstate.Snapshot{}, false, nil
	}
	return a.sessionStore.Load(ctx, desktopConversationSnapshotKey(project, sessionID))
}

func (a *DesktopApp) currentChatConversation(ctx context.Context, project string) (chatSessionSnapshot, error) {
	sessionID, err := a.currentChatSession(ctx, project)
	if err != nil {
		return chatSessionSnapshot{}, err
	}
	session, ok, err := a.findChatSession(ctx, project, sessionID)
	if err != nil {
		return chatSessionSnapshot{}, err
	}
	if !ok {
		return chatSessionSnapshot{}, errors.New("未找到当前对话")
	}
	return session, nil
}

func (a *DesktopApp) findChatSession(ctx context.Context, project, sessionID string) (chatSessionSnapshot, bool, error) {
	sessions, err := a.listChatSessions(ctx, project)
	if err != nil {
		return chatSessionSnapshot{}, false, err
	}
	for _, item := range sessions {
		if item.SessionID == strings.TrimSpace(sessionID) {
			return item, true, nil
		}
	}
	return chatSessionSnapshot{}, false, nil
}

func (a *DesktopApp) listChatSessions(ctx context.Context, project string) ([]chatSessionSnapshot, error) {
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
		source, sessionID, itemProject, ok := parseConversationKey(item.Key)
		if !ok {
			continue
		}
		switch {
		case isDesktopConversationSource(source):
			if !strings.EqualFold(itemProject, project) {
				continue
			}
			if !isDesktopChatSessionForProject(sessionID, project) {
				continue
			}
		case isWeixinConversationSource(source):
			if len(item.History) == 0 && strings.TrimSpace(item.Title) == "" {
				continue
			}
		default:
			continue
		}
		out = append(out, chatSessionSnapshot{
			SessionID:   sessionID,
			SnapshotKey: item.Key,
			Source:      source,
			SourceLabel: conversationSourceLabel(source),
			ReadOnly:    conversationReadOnly(source),
			Snapshot:    item,
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

func parseConversationKey(key string) (source string, sessionID string, project string, ok bool) {
	parts := strings.Split(strings.TrimSpace(key), "|")
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
	if source == "" || sessionID == "" {
		return "", "", "", false
	}
	return source, sessionID, project, true
}

func parseDesktopConversationKey(key string) (project string, sessionID string, ok bool) {
	source, sessionID, project, ok := parseConversationKey(key)
	if !ok || !isDesktopConversationSource(source) || project == "" {
		return "", "", false
	}
	return project, sessionID, true
}

func desktopChatSessionBase(project string) string {
	return desktopChatSessionID + ":" + knowledge.CanonicalProjectName(project)
}

func normalizeNewChatMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "ask":
		return "ask"
	default:
		return "agent"
	}
}

func newDesktopChatSessionID(project string) string {
	return fmt.Sprintf("%s:%d", desktopChatSessionBase(project), time.Now().UnixNano())
}

func isDesktopChatSessionForProject(sessionID, project string) bool {
	base := desktopChatSessionBase(project)
	return sessionID == base || strings.HasPrefix(sessionID, base+":")
}

func isDesktopConversationSource(source string) bool {
	return strings.EqualFold(strings.TrimSpace(source), desktopSourceLabel())
}

func isWeixinConversationSource(source string) bool {
	name, _ := splitConversationSource(source)
	return strings.EqualFold(name, "weixin")
}

func conversationReadOnly(source string) bool {
	return false
}

func conversationSourceLabel(source string) string {
	name, userID := splitConversationSource(source)
	switch {
	case strings.EqualFold(name, "weixin"):
		return "微信"
	case strings.EqualFold(name, desktopInterface):
		return "桌面"
	case userID != "":
		return name + " · " + userID
	default:
		return source
	}
}

func splitConversationSource(source string) (name string, userID string) {
	source = strings.TrimSpace(source)
	name, userID, found := strings.Cut(source, ":")
	if !found {
		return source, ""
	}
	return strings.TrimSpace(name), strings.TrimSpace(userID)
}

func firstChatSession(sessions []chatSessionSnapshot) (string, bool) {
	for _, item := range sessions {
		return item.SessionID, true
	}
	return "", false
}

func externalMessageContext(source, sessionID string) appsvc.MessageContext {
	name, userID := splitConversationSource(source)
	return appsvc.MessageContext{
		UserID:    userID,
		Interface: name,
		SessionID: strings.TrimSpace(sessionID),
	}
}

func toChatConversation(item chatSessionSnapshot, active bool) ChatConversation {
	updatedAt := strings.TrimSpace(item.Snapshot.UpdatedAt.Local().Format("2006-01-02 15:04:05"))
	if item.Snapshot.UpdatedAt.IsZero() {
		updatedAt = ""
	}
	customTitle := strings.TrimSpace(item.Snapshot.Title) != ""
	return ChatConversation{
		SessionID:     item.SessionID,
		Title:         chatConversationTitle(item.Snapshot),
		CustomTitle:   customTitle,
		Preview:       chatConversationPreview(item.Snapshot.History),
		Source:        strings.TrimSpace(item.Source),
		SourceLabel:   strings.TrimSpace(item.SourceLabel),
		Mode:          conversationModeLabel(item),
		ReadOnly:      item.ReadOnly,
		UpdatedAt:     updatedAt,
		UpdatedAtUnix: item.Snapshot.UpdatedAt.Unix(),
		MessageCount:  len(item.Snapshot.History),
		HasMessages:   len(item.Snapshot.History) > 0,
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
			Role:    role,
			Text:    text,
			Time:    "",
			Usage:   item.Usage,
			Process: item.Process,
		})
	}
	return messages
}

func conversationModeLabel(item chatSessionSnapshot) string {
	return normalizeNewChatMode(item.Snapshot.Mode)
}

func trimmedChatSnapshotForRefresh(snapshot sessionstate.Snapshot) (sessionstate.Snapshot, string, error) {
	if len(snapshot.History) < 2 {
		return sessionstate.Snapshot{}, "", errors.New("当前对话没有可刷新的回复")
	}

	assistantIndex := len(snapshot.History) - 1
	assistant := snapshot.History[assistantIndex]
	if !strings.EqualFold(strings.TrimSpace(assistant.Role), "assistant") || strings.TrimSpace(assistant.Content) == "" {
		return sessionstate.Snapshot{}, "", errors.New("当前结果尚未生成完成，暂时不能刷新")
	}

	userIndex := assistantIndex - 1
	user := snapshot.History[userIndex]
	if !strings.EqualFold(strings.TrimSpace(user.Role), "user") {
		return sessionstate.Snapshot{}, "", errors.New("没有找到当前回复对应的提问")
	}

	input := strings.TrimSpace(user.Content)
	if input == "" {
		return sessionstate.Snapshot{}, "", errors.New("没有找到当前回复对应的提问")
	}

	next := snapshot
	next.History = append([]sessionstate.Message(nil), snapshot.History[:userIndex]...)
	return next, input, nil
}

func chatConversationTitle(snapshot sessionstate.Snapshot) string {
	if title := strings.TrimSpace(snapshot.Title); title != "" {
		return title
	}
	history := snapshot.History
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
