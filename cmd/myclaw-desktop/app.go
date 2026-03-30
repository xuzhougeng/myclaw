package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"myclaw/internal/ai"
	appsvc "myclaw/internal/app"
	"myclaw/internal/fileingest"
	"myclaw/internal/knowledge"
	"myclaw/internal/modelconfig"
	"myclaw/internal/projectstate"
	"myclaw/internal/promptlib"
	"myclaw/internal/reminder"
	"myclaw/internal/sessionstate"
	"myclaw/internal/skilllib"
	"myclaw/internal/weixin"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

const (
	desktopInterface         = "desktop"
	desktopUserID            = "primary"
	desktopChatSessionID     = "desktop-chat"
	maxKnowledgePreviewRunes = 180
)

type DesktopApp struct {
	ctx               context.Context
	dataDir           string
	store             *knowledge.Store
	promptStore       *promptlib.Store
	projectStore      *projectstate.Store
	modelStore        *modelconfig.Store
	aiService         *ai.Service
	service           *appsvc.Service
	sessionStore      *sessionstate.Store
	reminders         *reminder.Manager
	weixinBridge      *weixin.Bridge
	reminderCancel    context.CancelFunc
	dialogMu          sync.Mutex
	chatSessionMu     sync.RWMutex
	projectMu         sync.RWMutex
	trayMu            sync.Mutex
	weixinMu          sync.Mutex
	activeProject     string
	chatSessionMap    map[string]string
	trayController    desktopTrayController
	allowWindowClose  bool
	weixinStatus      WeixinStatus
	weixinRunCancel   context.CancelFunc
	weixinLoginCancel context.CancelFunc
}

type Overview struct {
	DataDir         string `json:"dataDir"`
	AIAvailable     bool   `json:"aiAvailable"`
	AIMessage       string `json:"aiMessage"`
	ActiveProject   string `json:"activeProject"`
	KnowledgeCount  int    `json:"knowledgeCount"`
	PromptCount     int    `json:"promptCount"`
	WeixinConnected bool   `json:"weixinConnected"`
	WeixinMessage   string `json:"weixinMessage"`
}

type ModelSettings struct {
	Profiles                  []modelconfig.Summary `json:"profiles"`
	ActiveProfileID           string                `json:"activeProfileId"`
	Configured                bool                  `json:"configured"`
	MissingFields             []string              `json:"missingFields"`
	EffectiveProfileName      string                `json:"effectiveProfileName"`
	EffectiveProvider         string                `json:"effectiveProvider"`
	EffectiveAPIType          string                `json:"effectiveApiType"`
	EffectiveBaseURL          string                `json:"effectiveBaseUrl"`
	EffectiveAPIKeyMasked     string                `json:"effectiveApiKeyMasked"`
	EffectiveModel            string                `json:"effectiveModel"`
	EffectiveMaxOutputTokens  *int                  `json:"effectiveMaxOutputTokens,omitempty"`
	EffectiveTemperature      *float64              `json:"effectiveTemperature,omitempty"`
	EffectiveTopP             *float64              `json:"effectiveTopP,omitempty"`
	EffectiveFrequencyPenalty *float64              `json:"effectiveFrequencyPenalty,omitempty"`
	EffectivePresencePenalty  *float64              `json:"effectivePresencePenalty,omitempty"`
	Message                   string                `json:"message"`
}

type ModelConfigInput struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	Provider         string   `json:"provider"`
	APIType          string   `json:"apiType"`
	BaseURL          string   `json:"baseUrl"`
	APIKey           string   `json:"apiKey"`
	Model            string   `json:"model"`
	MaxOutputTokens  *int     `json:"maxOutputTokens"`
	Temperature      *float64 `json:"temperature"`
	TopP             *float64 `json:"topP"`
	FrequencyPenalty *float64 `json:"frequencyPenalty"`
	PresencePenalty  *float64 `json:"presencePenalty"`
	SetActive        bool     `json:"setActive"`
	PreserveAPIKey   bool     `json:"preserveApiKey"`
}

type KnowledgeItem struct {
	ID             string   `json:"id"`
	ShortID        string   `json:"shortId"`
	Text           string   `json:"text"`
	Preview        string   `json:"preview"`
	Source         string   `json:"source"`
	RecordedAt     string   `json:"recordedAt"`
	RecordedAtUnix int64    `json:"recordedAtUnix"`
	Keywords       []string `json:"keywords"`
	IsFile         bool     `json:"isFile"`
}

type KnowledgeMutation struct {
	Message string        `json:"message"`
	Item    KnowledgeItem `json:"item"`
}

type PromptItem struct {
	ID             string `json:"id"`
	ShortID        string `json:"shortId"`
	Title          string `json:"title"`
	Content        string `json:"content"`
	Preview        string `json:"preview"`
	RecordedAt     string `json:"recordedAt"`
	RecordedAtUnix int64  `json:"recordedAtUnix"`
}

type PromptMutation struct {
	Message string     `json:"message"`
	Item    PromptItem `json:"item"`
}

type SkillItem struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Content     string `json:"content"`
	Dir         string `json:"dir"`
	Loaded      bool   `json:"loaded"`
}

type SkillMutation struct {
	Message string    `json:"message"`
	Item    SkillItem `json:"item"`
}

type ProjectSummary struct {
	Name                 string `json:"name"`
	KnowledgeCount       int    `json:"knowledgeCount"`
	LatestRecordedAt     string `json:"latestRecordedAt"`
	LatestRecordedAtUnix int64  `json:"latestRecordedAtUnix"`
	Active               bool   `json:"active"`
}

type ProjectState struct {
	ActiveProject string           `json:"activeProject"`
	Projects      []ProjectSummary `json:"projects"`
}

type MessageResult struct {
	Message string `json:"message"`
}

type ChatModeState struct {
	Mode string `json:"mode"`
}

type ChatPromptState struct {
	PromptID string `json:"promptId"`
	ShortID  string `json:"shortId"`
	Title    string `json:"title"`
}

type ChatResponse struct {
	Reply          string `json:"reply"`
	Timestamp      string `json:"timestamp"`
	SessionID      string `json:"sessionId,omitempty"`
	SessionChanged bool   `json:"sessionChanged,omitempty"`
}

type ReminderItem struct {
	ID             string `json:"id"`
	ShortID        string `json:"shortId"`
	Message        string `json:"message"`
	Frequency      string `json:"frequency"`
	FrequencyLabel string `json:"frequencyLabel"`
	ScheduleLabel  string `json:"scheduleLabel"`
	NextRunAt      string `json:"nextRunAt"`
	NextRunAtUnix  int64  `json:"nextRunAtUnix"`
	CreatedAt      string `json:"createdAt"`
	CreatedAtUnix  int64  `json:"createdAtUnix"`
}

type reminderNotifier struct {
	app *DesktopApp
}

func NewDesktopApp(dataDir string, store *knowledge.Store, promptStore *promptlib.Store, projectStore *projectstate.Store, modelStore *modelconfig.Store, aiService *ai.Service, service *appsvc.Service, sessionStore *sessionstate.Store, reminders *reminder.Manager, weixinBridge *weixin.Bridge) *DesktopApp {
	return &DesktopApp{
		dataDir:        dataDir,
		store:          store,
		promptStore:    promptStore,
		projectStore:   projectStore,
		modelStore:     modelStore,
		aiService:      aiService,
		service:        service,
		sessionStore:   sessionStore,
		reminders:      reminders,
		weixinBridge:   weixinBridge,
		chatSessionMap: make(map[string]string),
		weixinStatus:   defaultWeixinStatus(),
	}
}

func (a *DesktopApp) startup(ctx context.Context) {
	a.ctx = ctx
	runtime.WindowCenter(ctx)
	a.initTrayController()
	a.startBackgroundServices()
}

func (a *DesktopApp) startBackgroundServices() {
	if a.reminders == nil {
		a.initWeixin()
		return
	}

	a.initWeixin()

	target := reminder.Target{
		Interface: desktopInterface,
		UserID:    desktopUserID,
	}
	a.reminders.RegisterNotifier(target, reminderNotifier{app: a})

	runCtx, cancel := context.WithCancel(context.Background())
	a.reminderCancel = cancel
	go func() {
		if err := a.reminders.Run(runCtx); err != nil && !errors.Is(err, context.Canceled) {
			log.Printf("reminder scheduler stopped: %v", err)
			if a.ctx != nil {
				runtime.LogErrorf(a.ctx, "reminder scheduler stopped: %v", err)
			}
		}
	}()
}

func (a *DesktopApp) shutdown(context.Context) {
	a.disposeTrayController()
	a.stopBackgroundServices()
}

func (a *DesktopApp) beforeClose(ctx context.Context) bool {
	a.trayMu.Lock()
	allowClose := a.allowWindowClose
	if allowClose {
		a.allowWindowClose = false
	}
	trayReady := a.trayController != nil
	a.trayMu.Unlock()

	if allowClose || !trayReady {
		return false
	}

	runtime.WindowHide(ctx)
	return true
}

func (a *DesktopApp) initTrayController() {
	controller, err := newDesktopTrayController(a)
	if err != nil {
		log.Printf("init tray controller: %v", err)
		return
	}
	if controller == nil {
		return
	}

	a.trayMu.Lock()
	a.trayController = controller
	a.trayMu.Unlock()
}

func (a *DesktopApp) disposeTrayController() {
	a.trayMu.Lock()
	controller := a.trayController
	a.trayController = nil
	a.trayMu.Unlock()

	if controller == nil {
		return
	}
	if err := controller.Dispose(); err != nil {
		log.Printf("dispose tray controller: %v", err)
	}
}

func (a *DesktopApp) restoreMainWindow() {
	if a.ctx == nil {
		return
	}
	runtime.WindowShow(a.ctx)
}

func (a *DesktopApp) quitFromTray() {
	if a.ctx == nil {
		return
	}
	a.trayMu.Lock()
	a.allowWindowClose = true
	a.trayMu.Unlock()
	runtime.Quit(a.ctx)
}

func (a *DesktopApp) stopBackgroundServices() {
	a.stopWeixin()
	if a.reminderCancel != nil {
		a.reminderCancel()
		a.reminderCancel = nil
	}
}

func (n reminderNotifier) Notify(_ context.Context, reminderItem reminder.Reminder) error {
	return n.app.showReminderDialog(reminderItem)
}

func (a *DesktopApp) GetOverview() (Overview, error) {
	projectCtx, project, err := a.projectContext(context.Background())
	if err != nil {
		return Overview{}, err
	}

	entries, err := a.store.List(projectCtx)
	if err != nil {
		return Overview{}, err
	}
	prompts, err := a.promptStore.List(context.Background())
	if err != nil {
		return Overview{}, err
	}
	available, message, err := a.aiStatus(context.Background())
	if err != nil {
		return Overview{}, err
	}
	weixinStatus := a.GetWeixinStatus()
	return Overview{
		DataDir:         a.dataDir,
		AIAvailable:     available,
		AIMessage:       message,
		ActiveProject:   project,
		KnowledgeCount:  len(entries),
		PromptCount:     len(prompts),
		WeixinConnected: weixinStatus.Connected,
		WeixinMessage:   weixinStatus.Message,
	}, nil
}

func (a *DesktopApp) GetProjectState() (ProjectState, error) {
	return a.buildProjectState(context.Background())
}

func (a *DesktopApp) SetActiveProject(name string) (ProjectState, error) {
	project := knowledge.CanonicalProjectName(name)
	if a.projectStore != nil {
		snapshot, err := a.projectStore.Save(context.Background(), project)
		if err != nil {
			return ProjectState{}, err
		}
		project = snapshot.ActiveProject
	}
	a.rememberActiveProject(project)
	return a.buildProjectState(context.Background())
}

func (a *DesktopApp) ListReminders() ([]ReminderItem, error) {
	if a.reminders == nil {
		return []ReminderItem{}, nil
	}

	items, err := a.reminders.List(context.Background(), reminder.Target{
		Interface: desktopInterface,
		UserID:    desktopUserID,
	})
	if err != nil {
		return nil, err
	}

	result := make([]ReminderItem, 0, len(items))
	for _, item := range items {
		result = append(result, toReminderItem(item))
	}
	return result, nil
}

func (a *DesktopApp) GetModelSettings() (ModelSettings, error) {
	if a.aiService == nil || a.modelStore == nil {
		return ModelSettings{}, errors.New("模型服务尚未启用")
	}

	snapshot, err := a.modelStore.List(context.Background())
	if err != nil {
		return ModelSettings{}, err
	}
	effective, err := a.aiService.CurrentConfig(context.Background())
	if err != nil {
		return ModelSettings{}, err
	}

	missing := effective.MissingFields()
	settings := ModelSettings{
		Profiles:                  snapshot.Profiles,
		ActiveProfileID:           snapshot.ActiveProfileID,
		Configured:                snapshot.ActiveProfileID != "" && len(missing) == 0,
		MissingFields:             missing,
		EffectiveProfileName:      effective.Name,
		EffectiveProvider:         effective.Provider,
		EffectiveAPIType:          effective.APIType,
		EffectiveBaseURL:          effective.BaseURL,
		EffectiveAPIKeyMasked:     modelconfig.MaskSecret(effective.APIKey),
		EffectiveModel:            effective.Model,
		EffectiveMaxOutputTokens:  effective.MaxOutputTokens,
		EffectiveTemperature:      effective.Temperature,
		EffectiveTopP:             effective.TopP,
		EffectiveFrequencyPenalty: effective.FrequencyPenalty,
		EffectivePresencePenalty:  effective.PresencePenalty,
		Message:                   desktopModelMessage(snapshot, missing),
	}
	return settings, nil
}

func (a *DesktopApp) SaveModelConfig(input ModelConfigInput) (ModelSettings, error) {
	if a.modelStore == nil {
		return ModelSettings{}, errors.New("模型配置存储尚未启用")
	}

	cfg := modelconfig.Config{
		ID:               input.ID,
		Name:             input.Name,
		Provider:         input.Provider,
		APIType:          input.APIType,
		BaseURL:          input.BaseURL,
		APIKey:           input.APIKey,
		Model:            input.Model,
		MaxOutputTokens:  input.MaxOutputTokens,
		Temperature:      input.Temperature,
		TopP:             input.TopP,
		FrequencyPenalty: input.FrequencyPenalty,
		PresencePenalty:  input.PresencePenalty,
	}
	if _, err := a.modelStore.Save(context.Background(), cfg, modelconfig.SaveOptions{
		SetActive:      input.SetActive,
		PreserveAPIKey: input.PreserveAPIKey,
	}); err != nil {
		return ModelSettings{}, err
	}
	return a.GetModelSettings()
}

func (a *DesktopApp) DeleteModelConfig(id string) (ModelSettings, error) {
	if a.modelStore == nil {
		return ModelSettings{}, errors.New("模型配置存储尚未启用")
	}
	deleted, err := a.modelStore.Delete(context.Background(), id)
	if err != nil {
		return ModelSettings{}, err
	}
	if !deleted {
		return ModelSettings{}, errors.New("未找到要删除的模型 profile")
	}
	return a.GetModelSettings()
}

func (a *DesktopApp) SetActiveModel(id string) (ModelSettings, error) {
	if a.modelStore == nil {
		return ModelSettings{}, errors.New("模型配置存储尚未启用")
	}
	if err := a.modelStore.SetActive(context.Background(), id); err != nil {
		return ModelSettings{}, err
	}
	return a.GetModelSettings()
}

func (a *DesktopApp) TestModelConnection(id string) (MessageResult, error) {
	if a.aiService == nil {
		return MessageResult{}, errors.New("模型服务尚未启用")
	}

	ctx := context.Background()
	cfg, err := a.aiService.CurrentConfig(ctx)
	if err != nil {
		return MessageResult{}, err
	}
	if strings.TrimSpace(id) != "" {
		selected, ok, err := a.modelStore.Get(ctx, id)
		if err != nil {
			return MessageResult{}, err
		}
		if !ok {
			return MessageResult{}, errors.New("未找到要测试的模型 profile")
		}
		cfg = selected
	}

	result, err := a.aiService.TestConfig(ctx, cfg)
	if err != nil {
		return MessageResult{}, err
	}
	return MessageResult{Message: "模型连接测试成功：" + strings.TrimSpace(result)}, nil
}

func (a *DesktopApp) ListKnowledge() ([]KnowledgeItem, error) {
	projectCtx, _, err := a.projectContext(context.Background())
	if err != nil {
		return nil, err
	}

	entries, err := a.store.List(projectCtx)
	if err != nil {
		return nil, err
	}
	reverseKnowledge(entries)

	result := make([]KnowledgeItem, 0, len(entries))
	for _, entry := range entries {
		result = append(result, toKnowledgeItem(entry))
	}
	return result, nil
}

func (a *DesktopApp) CreateKnowledge(text string) (KnowledgeMutation, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return KnowledgeMutation{}, errors.New("请输入要保存的记忆内容。")
	}

	projectCtx, _, err := a.projectContext(context.Background())
	if err != nil {
		return KnowledgeMutation{}, err
	}

	entry, err := a.store.Add(projectCtx, knowledge.Entry{
		Text:       text,
		Source:     desktopSourceLabel(),
		RecordedAt: time.Now(),
	})
	if err != nil {
		return KnowledgeMutation{}, err
	}
	return KnowledgeMutation{
		Message: fmt.Sprintf("已记住 #%s", shortID(entry.ID)),
		Item:    toKnowledgeItem(entry),
	}, nil
}

func (a *DesktopApp) AppendKnowledge(idOrPrefix, addition string) (KnowledgeMutation, error) {
	idOrPrefix = strings.TrimSpace(idOrPrefix)
	addition = strings.TrimSpace(addition)
	if idOrPrefix == "" {
		return KnowledgeMutation{}, errors.New("请选择要补充的记忆。")
	}
	if addition == "" {
		return KnowledgeMutation{}, errors.New("请输入补充内容。")
	}

	projectCtx, _, err := a.projectContext(context.Background())
	if err != nil {
		return KnowledgeMutation{}, err
	}

	entry, ok, err := a.store.Append(projectCtx, idOrPrefix, addition)
	if err != nil {
		return KnowledgeMutation{}, err
	}
	if !ok {
		return KnowledgeMutation{}, fmt.Errorf("没有找到知识 #%s。", strings.TrimPrefix(idOrPrefix, "#"))
	}
	return KnowledgeMutation{
		Message: fmt.Sprintf("已补充 #%s", shortID(entry.ID)),
		Item:    toKnowledgeItem(entry),
	}, nil
}

func (a *DesktopApp) DeleteKnowledge(idOrPrefix string) (MessageResult, error) {
	projectCtx, _, err := a.projectContext(context.Background())
	if err != nil {
		return MessageResult{}, err
	}

	entry, ok, err := a.store.Remove(projectCtx, idOrPrefix)
	if err != nil {
		return MessageResult{}, err
	}
	if !ok {
		return MessageResult{}, fmt.Errorf("没有找到知识 #%s。", strings.TrimPrefix(strings.TrimSpace(idOrPrefix), "#"))
	}
	return MessageResult{
		Message: fmt.Sprintf("已删除 #%s", shortID(entry.ID)),
	}, nil
}

func (a *DesktopApp) ClearKnowledge() (MessageResult, error) {
	projectCtx, project, err := a.projectContext(context.Background())
	if err != nil {
		return MessageResult{}, err
	}

	if err := a.store.Clear(projectCtx); err != nil {
		return MessageResult{}, err
	}
	return MessageResult{Message: fmt.Sprintf("项目 %s 的知识库已清空。", project)}, nil
}

func (a *DesktopApp) ListPrompts() ([]PromptItem, error) {
	prompts, err := a.promptStore.List(context.Background())
	if err != nil {
		return nil, err
	}
	reversePrompts(prompts)

	result := make([]PromptItem, 0, len(prompts))
	for _, prompt := range prompts {
		result = append(result, toPromptItem(prompt))
	}
	return result, nil
}

func (a *DesktopApp) CreatePrompt(title, content string) (PromptMutation, error) {
	title = strings.TrimSpace(title)
	content = strings.TrimSpace(content)
	if title == "" {
		return PromptMutation{}, errors.New("请输入 Prompt 标题。")
	}
	if content == "" {
		return PromptMutation{}, errors.New("请输入 Prompt 内容。")
	}

	prompt, err := a.promptStore.Add(context.Background(), promptlib.Prompt{
		Title:      title,
		Content:    content,
		RecordedAt: time.Now(),
	})
	if err != nil {
		return PromptMutation{}, err
	}

	return PromptMutation{
		Message: fmt.Sprintf("已保存 Prompt #%s", shortID(prompt.ID)),
		Item:    toPromptItem(prompt),
	}, nil
}

func (a *DesktopApp) DeletePrompt(idOrPrefix string) (MessageResult, error) {
	prompt, ok, err := a.promptStore.Remove(context.Background(), idOrPrefix)
	if err != nil {
		return MessageResult{}, err
	}
	if !ok {
		return MessageResult{}, fmt.Errorf("没有找到 Prompt #%s。", strings.TrimPrefix(strings.TrimSpace(idOrPrefix), "#"))
	}
	return MessageResult{
		Message: fmt.Sprintf("已删除 Prompt #%s", shortID(prompt.ID)),
	}, nil
}

func (a *DesktopApp) ClearPrompts() (MessageResult, error) {
	if err := a.promptStore.Clear(context.Background()); err != nil {
		return MessageResult{}, err
	}
	return MessageResult{Message: "Prompt 库已清空。"}, nil
}

func (a *DesktopApp) ListSkills() ([]SkillItem, error) {
	if a.service == nil {
		return nil, errors.New("技能服务尚未启用")
	}

	project, err := a.currentProject(context.Background())
	if err != nil {
		return nil, err
	}
	mc, err := a.chatMessageContext(context.Background(), project)
	if err != nil {
		return nil, err
	}

	available, err := a.service.ListAvailableSkills()
	if err != nil {
		return nil, err
	}
	loaded := a.service.ListLoadedSkills(mc)
	loadedSet := make(map[string]struct{}, len(loaded))
	for _, skill := range loaded {
		loadedSet[strings.ToLower(strings.TrimSpace(skill.Name))] = struct{}{}
	}

	result := make([]SkillItem, 0, len(available))
	for _, skill := range available {
		_, isLoaded := loadedSet[strings.ToLower(strings.TrimSpace(skill.Name))]
		result = append(result, toSkillItem(skill, isLoaded))
	}
	return result, nil
}

func (a *DesktopApp) LoadSkill(name string) (MessageResult, error) {
	if a.service == nil {
		return MessageResult{}, errors.New("技能服务尚未启用")
	}

	project, err := a.currentProject(context.Background())
	if err != nil {
		return MessageResult{}, err
	}
	mc, err := a.chatMessageContext(context.Background(), project)
	if err != nil {
		return MessageResult{}, err
	}

	message, err := a.service.LoadSkillForSession(mc, name)
	if err != nil {
		return MessageResult{}, err
	}
	return MessageResult{Message: message}, nil
}

func (a *DesktopApp) UnloadSkill(name string) (MessageResult, error) {
	if a.service == nil {
		return MessageResult{}, errors.New("技能服务尚未启用")
	}

	project, err := a.currentProject(context.Background())
	if err != nil {
		return MessageResult{}, err
	}
	mc, err := a.chatMessageContext(context.Background(), project)
	if err != nil {
		return MessageResult{}, err
	}

	message, err := a.service.UnloadSkillForSession(mc, name)
	if err != nil {
		return MessageResult{}, err
	}
	return MessageResult{Message: message}, nil
}

func (a *DesktopApp) GetChatMode() (ChatModeState, error) {
	if a.service == nil {
		return ChatModeState{}, errors.New("聊天服务尚未启用")
	}

	project, err := a.currentProject(context.Background())
	if err != nil {
		return ChatModeState{}, err
	}
	mc, err := a.chatMessageContext(context.Background(), project)
	if err != nil {
		return ChatModeState{}, err
	}

	mode, err := a.service.GetMode(context.Background(), mc)
	if err != nil {
		return ChatModeState{}, err
	}
	return ChatModeState{Mode: string(mode)}, nil
}

func (a *DesktopApp) SetChatMode(mode string) (ChatModeState, error) {
	if a.service == nil {
		return ChatModeState{}, errors.New("聊天服务尚未启用")
	}

	project, err := a.currentProject(context.Background())
	if err != nil {
		return ChatModeState{}, err
	}
	mc, err := a.chatMessageContext(context.Background(), project)
	if err != nil {
		return ChatModeState{}, err
	}

	selected, err := a.service.SetMode(context.Background(), mc, appsvc.Mode(mode))
	if err != nil {
		return ChatModeState{}, err
	}
	return ChatModeState{Mode: string(selected)}, nil
}

func (a *DesktopApp) GetChatPrompt() (ChatPromptState, error) {
	if a.service == nil {
		return ChatPromptState{}, errors.New("聊天服务尚未启用")
	}

	project, err := a.currentProject(context.Background())
	if err != nil {
		return ChatPromptState{}, err
	}
	mc, err := a.chatMessageContext(context.Background(), project)
	if err != nil {
		return ChatPromptState{}, err
	}

	prompt, ok, err := a.service.CurrentPromptProfile(context.Background(), mc)
	if err != nil {
		return ChatPromptState{}, err
	}
	if !ok {
		return ChatPromptState{}, nil
	}
	return ChatPromptState{
		PromptID: prompt.ID,
		ShortID:  shortID(prompt.ID),
		Title:    strings.TrimSpace(prompt.Title),
	}, nil
}

func (a *DesktopApp) SetChatPrompt(idOrPrefix string) (ChatPromptState, error) {
	if a.service == nil {
		return ChatPromptState{}, errors.New("聊天服务尚未启用")
	}

	project, err := a.currentProject(context.Background())
	if err != nil {
		return ChatPromptState{}, err
	}
	mc, err := a.chatMessageContext(context.Background(), project)
	if err != nil {
		return ChatPromptState{}, err
	}

	prompt, err := a.service.SetPromptProfile(context.Background(), mc, idOrPrefix)
	if err != nil {
		return ChatPromptState{}, err
	}
	return ChatPromptState{
		PromptID: prompt.ID,
		ShortID:  shortID(prompt.ID),
		Title:    strings.TrimSpace(prompt.Title),
	}, nil
}

func (a *DesktopApp) ClearChatPrompt() (ChatPromptState, error) {
	if a.service == nil {
		return ChatPromptState{}, errors.New("聊天服务尚未启用")
	}

	project, err := a.currentProject(context.Background())
	if err != nil {
		return ChatPromptState{}, err
	}
	mc, err := a.chatMessageContext(context.Background(), project)
	if err != nil {
		return ChatPromptState{}, err
	}
	if err := a.service.ClearPromptProfile(context.Background(), mc); err != nil {
		return ChatPromptState{}, err
	}
	return ChatPromptState{}, nil
}

func (a *DesktopApp) ConfirmAction(title, message string) (bool, error) {
	if a.ctx == nil {
		return false, errors.New("桌面上下文尚未初始化")
	}

	result, err := runtime.MessageDialog(a.ctx, runtime.MessageDialogOptions{
		Type:          runtime.QuestionDialog,
		Title:         strings.TrimSpace(title),
		Message:       strings.TrimSpace(message),
		DefaultButton: "No",
	})
	if err != nil {
		return false, err
	}

	switch strings.ToLower(strings.TrimSpace(result)) {
	case "yes", "ok":
		return true, nil
	default:
		return false, nil
	}
}

func (a *DesktopApp) OpenImportDialog() (string, error) {
	if a.ctx == nil {
		return "", errors.New("桌面上下文尚未初始化")
	}

	return runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title:            "选择要导入的图片或 PDF",
		DefaultDirectory: a.defaultDialogDirectory(),
		Filters: []runtime.FileFilter{
			{
				DisplayName: "Image / PDF Files",
				Pattern:     "*.png;*.jpg;*.jpeg;*.webp;*.gif;*.pdf",
			},
		},
	})
}

func (a *DesktopApp) OpenSkillImportDialog() (string, error) {
	if a.ctx == nil {
		return "", errors.New("桌面上下文尚未初始化")
	}

	return runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title:            "选择要导入的 skill zip",
		DefaultDirectory: a.defaultDialogDirectory(),
		Filters: []runtime.FileFilter{
			{
				DisplayName: "ZIP Files",
				Pattern:     "*.zip",
			},
		},
	})
}

func (a *DesktopApp) ImportFile(path string) (KnowledgeMutation, error) {
	projectCtx, _, err := a.projectContext(context.Background())
	if err != nil {
		return KnowledgeMutation{}, err
	}

	entry, err := a.ingestFile(projectCtx, path)
	if err != nil {
		return KnowledgeMutation{}, err
	}
	return KnowledgeMutation{
		Message: fmt.Sprintf("已导入文件并写入 #%s", shortID(entry.ID)),
		Item:    toKnowledgeItem(entry),
	}, nil
}

func (a *DesktopApp) ImportSkillArchive(path string) (SkillMutation, error) {
	if a.service == nil {
		return SkillMutation{}, errors.New("技能服务尚未启用")
	}

	pkg, err := skilllib.InspectArchive(path)
	if err != nil {
		return SkillMutation{}, err
	}

	available, err := a.service.ListAvailableSkills()
	if err != nil {
		return SkillMutation{}, err
	}
	for _, skill := range available {
		if !strings.EqualFold(strings.TrimSpace(skill.Name), strings.TrimSpace(pkg.Skill.Name)) {
			continue
		}
		return SkillMutation{}, fmt.Errorf("skill %q 已存在，请先删除或更换 frontmatter name。", pkg.Skill.Name)
	}

	imported, err := skilllib.ImportArchive(path, filepath.Join(a.dataDir, "skills"))
	if err != nil {
		return SkillMutation{}, err
	}
	return SkillMutation{
		Message: fmt.Sprintf("已导入 skill %s。", imported.Name),
		Item:    toSkillItem(imported, false),
	}, nil
}

func (a *DesktopApp) SendMessage(input string) (ChatResponse, error) {
	project, err := a.currentProject(context.Background())
	if err != nil {
		return ChatResponse{}, err
	}
	if strings.EqualFold(strings.TrimSpace(input), "/new") {
		state, err := a.NewChatSession()
		if err != nil {
			return ChatResponse{}, err
		}
		return ChatResponse{
			Reply:          "已开启新对话。",
			Timestamp:      time.Now().Local().Format("2006-01-02 15:04:05"),
			SessionID:      state.SessionID,
			SessionChanged: true,
		}, nil
	}
	mc, err := a.chatMessageContext(context.Background(), project)
	if err != nil {
		return ChatResponse{}, err
	}

	reply, err := a.service.HandleMessage(context.Background(), mc, input)
	if err != nil {
		return ChatResponse{}, err
	}
	return ChatResponse{
		Reply:     reply,
		Timestamp: time.Now().Local().Format("2006-01-02 15:04:05"),
		SessionID: mc.SessionID,
	}, nil
}

func (a *DesktopApp) aiStatus(ctx context.Context) (bool, string, error) {
	if a.aiService == nil {
		return false, "模型尚未启用。", nil
	}

	configured, err := a.aiService.IsConfigured(ctx)
	if err != nil {
		return false, "", err
	}
	if !configured {
		return false, "模型未配置。请在桌面端的模型页面填写 Provider、Base URL、API Key 和 Model，或设置对应环境变量。", nil
	}
	return true, "模型已配置，可直接做文件总结和对话检索。", nil
}

func (a *DesktopApp) ingestFile(ctx context.Context, rawPath string) (knowledge.Entry, error) {
	input, ok, err := fileingest.Resolve(rawPath)
	if err != nil {
		return knowledge.Entry{}, err
	}
	if !ok {
		return knowledge.Entry{}, errors.New("只支持导入现有的图片或 PDF 文件。")
	}

	available, message, err := a.aiStatus(ctx)
	if err != nil {
		return knowledge.Entry{}, err
	}
	if !available {
		return knowledge.Entry{}, errors.New(message)
	}

	var summary string
	switch input.Kind {
	case fileingest.KindPDF:
		extractedText, extractErr := fileingest.ExtractPDFText(input.Path)
		if extractErr != nil {
			if errors.Is(extractErr, fileingest.ErrPDFExtractorUnavailable) {
				return knowledge.Entry{}, errors.New("当前这个构建不包含 PDF 文本提取能力。请使用启用 CGO 的构建来导入 PDF。")
			}
			return knowledge.Entry{}, extractErr
		}
		summary, err = a.aiService.SummarizePDFText(ctx, input.Name, extractedText)
	case fileingest.KindImage:
		summary, err = a.aiService.SummarizeImageFile(ctx, input.Name, input.DataURL)
	default:
		return knowledge.Entry{}, errors.New("暂不支持这个文件类型。")
	}
	if err != nil {
		return knowledge.Entry{}, err
	}

	return a.store.Add(ctx, knowledge.Entry{
		Text:       fileingest.FormatKnowledgeText(input, summary),
		Source:     desktopSourceLabel(),
		RecordedAt: time.Now(),
	})
}

func (a *DesktopApp) showReminderDialog(reminderItem reminder.Reminder) error {
	if a.ctx == nil {
		return nil
	}

	a.dialogMu.Lock()
	defer a.dialogMu.Unlock()

	runtime.EventsEmit(a.ctx, "reminder:due", map[string]string{
		"id":      reminderItem.ID,
		"shortId": shortID(reminderItem.ID),
		"message": reminderItem.Message,
	})
	runtime.WindowShow(a.ctx)
	_, err := runtime.MessageDialog(a.ctx, runtime.MessageDialogOptions{
		Type:    runtime.InfoDialog,
		Title:   "myclaw 提醒",
		Message: reminderItem.Message,
	})
	return err
}

func (a *DesktopApp) defaultDialogDirectory() string {
	candidates := []string{}
	if homeDir, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates, homeDir)
	}
	candidates = append(candidates, a.dataDir)

	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
	}
	return ""
}

func (a *DesktopApp) projectContext(ctx context.Context) (context.Context, string, error) {
	project, err := a.currentProject(ctx)
	if err != nil {
		return nil, "", err
	}
	return knowledge.WithProject(ctx, project), project, nil
}

func (a *DesktopApp) currentProject(ctx context.Context) (string, error) {
	a.projectMu.RLock()
	current := strings.TrimSpace(a.activeProject)
	a.projectMu.RUnlock()
	if current != "" {
		return current, nil
	}

	if a.projectStore == nil {
		current = knowledge.DefaultProjectName
		a.rememberActiveProject(current)
		return current, nil
	}

	snapshot, err := a.projectStore.Load(ctx)
	if err != nil {
		return "", err
	}
	current = knowledge.CanonicalProjectName(snapshot.ActiveProject)
	a.rememberActiveProject(current)
	return current, nil
}

func (a *DesktopApp) rememberActiveProject(project string) {
	a.projectMu.Lock()
	a.activeProject = knowledge.CanonicalProjectName(project)
	a.projectMu.Unlock()
}

func (a *DesktopApp) buildProjectState(ctx context.Context) (ProjectState, error) {
	activeProject, err := a.currentProject(ctx)
	if err != nil {
		return ProjectState{}, err
	}

	infos, err := a.store.ListProjects(context.Background())
	if err != nil {
		return ProjectState{}, err
	}

	projects := make([]ProjectSummary, 0, len(infos)+1)
	var activeSummary ProjectSummary
	activeFound := false
	for _, info := range infos {
		summary := toProjectSummary(info, activeProject)
		if summary.Active {
			activeSummary = summary
			activeFound = true
			continue
		}
		projects = append(projects, summary)
	}
	if activeFound {
		projects = append([]ProjectSummary{activeSummary}, projects...)
	} else {
		projects = append([]ProjectSummary{{
			Name:           activeProject,
			KnowledgeCount: 0,
			Active:         true,
		}}, projects...)
	}

	return ProjectState{
		ActiveProject: activeProject,
		Projects:      projects,
	}, nil
}

func desktopMessageContext(project string, sessionID string) appsvc.MessageContext {
	return appsvc.MessageContext{
		Interface: desktopInterface,
		UserID:    desktopUserID,
		SessionID: strings.TrimSpace(sessionID),
		Project:   project,
	}
}

func desktopSourceLabel() string {
	return desktopInterface + ":" + desktopUserID
}

func toKnowledgeItem(entry knowledge.Entry) KnowledgeItem {
	text := strings.TrimSpace(entry.Text)
	return KnowledgeItem{
		ID:             entry.ID,
		ShortID:        shortID(entry.ID),
		Text:           text,
		Preview:        preview(text, maxKnowledgePreviewRunes),
		Source:         strings.TrimSpace(entry.Source),
		RecordedAt:     entry.RecordedAt.Local().Format("2006-01-02 15:04:05"),
		RecordedAtUnix: entry.RecordedAt.Unix(),
		Keywords:       append([]string(nil), entry.Keywords...),
		IsFile:         strings.HasPrefix(text, "## 文件摘要"),
	}
}

func toPromptItem(prompt promptlib.Prompt) PromptItem {
	content := strings.TrimSpace(prompt.Content)
	return PromptItem{
		ID:             prompt.ID,
		ShortID:        shortID(prompt.ID),
		Title:          strings.TrimSpace(prompt.Title),
		Content:        content,
		Preview:        preview(content, maxKnowledgePreviewRunes),
		RecordedAt:     prompt.RecordedAt.Local().Format("2006-01-02 15:04:05"),
		RecordedAtUnix: prompt.RecordedAt.Unix(),
	}
}

func toReminderItem(item reminder.Reminder) ReminderItem {
	frequency := string(item.Frequency)
	frequencyLabel := "单次"
	scheduleLabel := "单次"
	if item.Frequency == reminder.FrequencyDaily {
		frequencyLabel = "每天"
		scheduleLabel = fmt.Sprintf("每天 %02d:%02d", item.DailyHour, item.DailyMinute)
	}

	return ReminderItem{
		ID:             item.ID,
		ShortID:        shortID(item.ID),
		Message:        strings.TrimSpace(item.Message),
		Frequency:      frequency,
		FrequencyLabel: frequencyLabel,
		ScheduleLabel:  scheduleLabel,
		NextRunAt:      item.NextRunAt.Local().Format("2006-01-02 15:04:05"),
		NextRunAtUnix:  item.NextRunAt.Unix(),
		CreatedAt:      item.CreatedAt.Local().Format("2006-01-02 15:04:05"),
		CreatedAtUnix:  item.CreatedAt.Unix(),
	}
}

func toSkillItem(skill skilllib.Skill, loaded bool) SkillItem {
	return SkillItem{
		Name:        strings.TrimSpace(skill.Name),
		Description: strings.TrimSpace(skill.Description),
		Content:     strings.TrimSpace(skill.Content),
		Dir:         strings.TrimSpace(skill.Dir),
		Loaded:      loaded,
	}
}

func toProjectSummary(info knowledge.ProjectInfo, activeProject string) ProjectSummary {
	return ProjectSummary{
		Name:                 knowledge.CanonicalProjectName(info.Name),
		KnowledgeCount:       info.KnowledgeCount,
		LatestRecordedAt:     info.LatestRecordedAt.Local().Format("2006-01-02 15:04:05"),
		LatestRecordedAtUnix: info.LatestRecordedAt.Unix(),
		Active:               strings.EqualFold(knowledge.CanonicalProjectName(info.Name), knowledge.CanonicalProjectName(activeProject)),
	}
}

func reverseKnowledge(entries []knowledge.Entry) {
	for left, right := 0, len(entries)-1; left < right; left, right = left+1, right-1 {
		entries[left], entries[right] = entries[right], entries[left]
	}
}

func reversePrompts(prompts []promptlib.Prompt) {
	for left, right := 0, len(prompts)-1; left < right; left, right = left+1, right-1 {
		prompts[left], prompts[right] = prompts[right], prompts[left]
	}
}

func shortID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8]
}

func preview(text string, maxRunes int) string {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) <= maxRunes {
		return string(runes)
	}
	return string(runes[:maxRunes]) + "..."
}

func desktopModelMessage(snapshot modelconfig.Snapshot, missing []string) string {
	switch {
	case len(snapshot.Profiles) == 0:
		return "尚未保存任何模型 profile。"
	case snapshot.ActiveProfileID == "":
		return "已保存模型 profile，但尚未选择活跃模型。"
	case len(missing) == 0:
		return fmt.Sprintf("本地已保存 %d 个模型 profile，当前活跃模型已生效。", len(snapshot.Profiles))
	default:
		return "当前活跃模型 profile 已保存，但配置仍不完整。"
	}
}
