package weixin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"myclaw/internal/app"
	"myclaw/internal/filesearch"
	"myclaw/internal/reminder"
)

const maxReplyChunkRunes = 1400

var ErrSessionExpired = errors.New("weixin session expired")

type Account struct {
	Token     string `json:"token"`
	BaseURL   string `json:"base_url"`
	UserID    string `json:"user_id"`
	AccountID string `json:"account_id"`
}

type BridgeConfig struct {
	DataDir        string
	EverythingPath string
}

type Bridge struct {
	client               *Client
	service              *app.Service
	reminders            *reminder.Manager
	config               BridgeConfig
	conversationMu       sync.RWMutex
	onConversation       func(ConversationUpdate)
	sessionMu            sync.Mutex
	conversationSessions map[string]string
	conversationLoaded   bool
	fileSearch           *filesearch.ShortcutHandler
	fileSender           *FileSender
}

func NewBridge(client *Client, service *app.Service, reminders *reminder.Manager, config BridgeConfig) *Bridge {
	bridge := &Bridge{
		client:     client,
		service:    service,
		reminders:  reminders,
		config:     config,
		fileSearch: filesearch.NewShortcutHandler(strings.TrimSpace(config.EverythingPath), filesearch.ExecuteWithEverything),
		fileSender: NewFileSender(client),
	}
	return bridge
}

func (b *Bridge) SetConversationUpdatedHook(fn func(ConversationUpdate)) {
	b.conversationMu.Lock()
	b.onConversation = fn
	b.conversationMu.Unlock()
}

func (b *Bridge) notifyConversationUpdated(update ConversationUpdate) {
	b.conversationMu.RLock()
	fn := b.onConversation
	b.conversationMu.RUnlock()
	if fn != nil {
		fn(update)
	}
}

func (b *Bridge) StartLogin() (*QRCodeResponse, error) {
	qr, err := b.client.GetQRCode()
	if err != nil {
		return nil, fmt.Errorf("get QR code: %w", err)
	}
	if qr.QRCode == "" {
		return nil, fmt.Errorf("no QR code returned: %s", qr.Message)
	}
	return qr, nil
}

func (b *Bridge) WaitLogin(ctx context.Context, qrcode string, timeout time.Duration) (Account, error) {
	status, err := b.client.PollQRCodeStatus(ctx, qrcode, timeout)
	if err != nil {
		return Account{}, err
	}
	return b.finalizeLogin(status)
}

func (b *Bridge) Login() error {
	qr, err := b.StartLogin()
	if err != nil {
		return err
	}

	fmt.Println("\n请用微信扫描以下二维码：")
	if qr.QRCodeImgContent != "" {
		fmt.Println("当前环境未内置二维码渲染，请将下面这段内容生成二维码后用微信扫码：")
		fmt.Println(qr.QRCodeImgContent)
	} else {
		fmt.Printf("二维码会话 ID: %s\n", qr.QRCode)
	}

	fmt.Println("\n等待扫码确认...")

	account, err := b.WaitLogin(context.Background(), qr.QRCode, 8*time.Minute)
	if err != nil {
		return err
	}

	log.Printf("[weixin] login succeeded, account=%s", account.AccountID)
	return nil
}

func (b *Bridge) ReadSavedAccount() (Account, bool) {
	path := filepath.Join(b.config.DataDir, "weixin-bridge", "account.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return Account{}, false
	}
	var account Account
	if err := json.Unmarshal(data, &account); err != nil {
		return Account{}, false
	}
	if strings.TrimSpace(account.Token) == "" {
		return Account{}, false
	}
	return account, true
}

func (b *Bridge) LoadAccount() bool {
	account, ok := b.ReadSavedAccount()
	if !ok {
		return false
	}

	token := account.Token
	baseURL := account.BaseURL
	if baseURL == "" {
		baseURL = b.client.BaseURL()
	}
	b.client = NewClient(baseURL, token)
	log.Printf("[weixin] loaded account %s", account.AccountID)
	return true
}

func (b *Bridge) Logout() error {
	dir := filepath.Join(b.config.DataDir, "weixin-bridge")
	for _, name := range []string{"account.json", "sync_buf"} {
		if err := os.Remove(filepath.Join(dir, name)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	b.client = NewClient(b.client.BaseURL(), "")
	return nil
}

func (b *Bridge) Run(ctx context.Context) error {
	log.Printf("[weixin] bridge started, polling for messages")

	bufPath := filepath.Join(b.config.DataDir, "weixin-bridge", "sync_buf")
	updatesBuf, _ := readTextFile(bufPath)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		resp, err := b.client.GetUpdates(ctx, updatesBuf)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			log.Printf("[weixin] getUpdates error: %v", err)
			time.Sleep(5 * time.Second)
			continue
		}

		if resp.ErrCode == -14 {
			return fmt.Errorf("%w: please run -weixin-login again", ErrSessionExpired)
		}

		if resp.GetUpdatesBuf != "" && resp.GetUpdatesBuf != updatesBuf {
			updatesBuf = resp.GetUpdatesBuf
			if err := writeTextFile(bufPath, updatesBuf); err != nil {
				log.Printf("[weixin] save sync buffer failed: %v", err)
			}
		}

		for _, msg := range resp.Msgs {
			if msg.MessageType != MessageTypeUser {
				continue
			}
			if strings.TrimSpace(extractText(msg)) == "" {
				continue
			}
			go b.handleMessage(ctx, msg)
		}
	}
}

func (b *Bridge) handleMessage(ctx context.Context, msg WeixinMessage) {
	text := extractText(msg)
	log.Printf("[weixin] inbound from=%s text=%s", msg.FromUserID, truncate(text, 80))
	inputPolicy := app.InspectInputPolicy(text)
	if inputPolicy.IsConversationControl && inputPolicy.Command == "/new" {
		messageContext, err := b.startNewConversation(ctx, msg)
		if err != nil {
			log.Printf("[weixin] start new conversation failed: %v", err)
			if sendErr := b.sendChunkedReply(ctx, msg.FromUserID, msg.ContextToken, "开启新对话失败，请稍后重试。"); sendErr != nil {
				log.Printf("[weixin] send /new failure reply failed: %v", sendErr)
			}
			return
		}
		reply := "已进入新对话。"
		if b.service != nil && strings.TrimSpace(text) != "" {
			b.service.RecordConversationTurn(ctx, messageContext, text, reply)
		}
		b.notifyConversationUpdated(ConversationUpdate{SessionID: messageContext.SessionID, Activate: true})
		if err := b.sendChunkedReply(ctx, msg.FromUserID, msg.ContextToken, reply); err != nil {
			log.Printf("[weixin] send /new reply failed: %v", err)
		}
		return
	}

	if inputPolicy.IsKnownCommand && inputPolicy.Execution == app.CommandExecutionService && !inputPolicy.PersistHistory && !inputPolicy.ActivateConversation {
		if err := b.handleStatelessCommand(ctx, msg, app.CanonicalizeCommandInput(text)); err != nil {
			log.Printf("[weixin] handle stateless command failed: %v", err)
		}
		return
	}

	statelessMessageContext, err := b.currentConversationContext(ctx, msg)
	if err != nil {
		log.Printf("[weixin] resolve stateless message context failed: %v", err)
		return
	}
	serviceCtx := app.WithConversationPersistenceDisabled(ctx)
	if reply, handled, err := b.handleFileSearch(serviceCtx, msg, statelessMessageContext, text); handled {
		if err != nil {
			log.Printf("[weixin] handle /find failed: %v", err)
			reply = "处理文件查找失败，请稍后重试。"
		}
		if strings.TrimSpace(reply) != "" {
			if sendErr := b.sendChunkedReply(ctx, msg.FromUserID, msg.ContextToken, reply); sendErr != nil {
				log.Printf("[weixin] send /find reply failed: %v", sendErr)
			}
		}
		return
	}

	messageContext, conversationNotice, activateConversation, err := b.bindConversationSession(ctx, msg)
	if err != nil {
		log.Printf("[weixin] bind conversation session failed: %v", err)
		return
	}

	if b.reminders != nil {
		userID := msg.FromUserID
		contextToken := msg.ContextToken
		b.reminders.RegisterNotifier(reminder.Target{Interface: "weixin", UserID: userID}, reminder.NotifierFunc(func(ctx context.Context, item reminder.Reminder) error {
			text := fmt.Sprintf("提醒时间到了：%s", item.Message)
			return b.sendChunkedReply(ctx, userID, contextToken, text)
		}))
	}

	if b.service == nil {
		reply := prefixConversationNotice(conversationNotice, "处理失败，服务尚未初始化。")
		b.notifyConversationUpdated(ConversationUpdate{SessionID: messageContext.SessionID, Activate: activateConversation})
		if err := b.sendChunkedReply(ctx, msg.FromUserID, msg.ContextToken, reply); err != nil {
			log.Printf("[weixin] send service unavailable reply failed: %v", err)
		}
		return
	}

	reply, err := b.service.HandleMessage(serviceCtx, messageContext, text)
	if err != nil {
		log.Printf("[weixin] handle message failed: %v", err)
		reply = "处理失败，请稍后重试。"
	}
	reply = prefixConversationNotice(conversationNotice, reply)
	if b.service != nil && strings.TrimSpace(text) != "" && strings.TrimSpace(reply) != "" {
		b.service.RecordConversationTurn(ctx, messageContext, text, reply)
	}
	b.notifyConversationUpdated(ConversationUpdate{SessionID: messageContext.SessionID, Activate: activateConversation})

	if err := b.sendChunkedReply(ctx, msg.FromUserID, msg.ContextToken, reply); err != nil {
		log.Printf("[weixin] send reply failed: %v", err)
	}
}

func prefixConversationNotice(notice, reply string) string {
	notice = strings.TrimSpace(notice)
	reply = strings.TrimSpace(reply)
	switch {
	case notice == "":
		return reply
	case reply == "":
		return notice
	default:
		return notice + "\n\n" + reply
	}
}

func (b *Bridge) sendChunkedReply(ctx context.Context, toUserID, contextToken, text string) error {
	chunks := splitByRunes(strings.TrimSpace(text), maxReplyChunkRunes)
	if len(chunks) == 0 {
		chunks = []string{"已处理，但没有可返回的文本。"}
	}
	for _, chunk := range chunks {
		if err := b.client.SendTextMessage(ctx, toUserID, chunk, contextToken); err != nil {
			return err
		}
	}
	return nil
}

func weixinSessionID(msg WeixinMessage) string {
	if strings.TrimSpace(msg.FromUserID) != "" {
		return "weixin-user:" + strings.TrimSpace(msg.FromUserID)
	}
	if strings.TrimSpace(msg.ContextToken) != "" {
		return "weixin:" + strings.TrimSpace(msg.ContextToken)
	}
	return "weixin"
}

func legacyContextSessionID(msg WeixinMessage) string {
	if strings.TrimSpace(msg.ContextToken) == "" {
		return ""
	}
	return "weixin:" + strings.TrimSpace(msg.ContextToken)
}

func (b *Bridge) handleStatelessCommand(ctx context.Context, msg WeixinMessage, command string) error {
	reply := "处理失败，服务尚未初始化。"
	if b.service != nil {
		messageContext, err := b.currentConversationContext(ctx, msg)
		if err != nil {
			return err
		}
		serviceCtx := app.WithConversationPersistenceDisabled(ctx)

		nextReply, err := b.service.HandleMessage(serviceCtx, messageContext, command)
		if err != nil {
			reply = "处理失败，请稍后重试。"
		} else if strings.TrimSpace(nextReply) != "" {
			reply = nextReply
		}
	}
	return b.sendChunkedReply(ctx, msg.FromUserID, msg.ContextToken, reply)
}

func (b *Bridge) SetEverythingPath(path string) {
	if b.fileSearch == nil {
		return
	}
	b.fileSearch.SetEverythingPath(path)
}

func (b *Bridge) EverythingPath() string {
	if b.fileSearch == nil {
		return ""
	}
	return b.fileSearch.EverythingPath()
}

func (b *Bridge) handleFileSearch(ctx context.Context, msg WeixinMessage, messageContext app.MessageContext, text string) (string, bool, error) {
	if b.fileSearch == nil {
		return "", false, nil
	}

	response, err := b.fileSearch.Handle(ctx, filesearch.ShortcutRequest{
		SlotKey: b.conversationSlotKey(msg),
		Text:    text,
		ResolveIntent: func(ctx context.Context, input string) (filesearch.ToolInput, bool, error) {
			return b.resolveFileSearchToolInput(ctx, messageContext, input)
		},
		SendSelectedFile: func(ctx context.Context, path string) error {
			if b.fileSender == nil {
				return fmt.Errorf("weixin file sender is not initialized")
			}
			return b.fileSender.Send(ctx, msg.FromUserID, msg.ContextToken, path)
		},
	})
	if strings.HasPrefix(response.Reply, "已发送文件 ") {
		response.Reply = strings.Replace(response.Reply, "已发送文件", "已通过 ClawBot 发送文件", 1)
	}
	return response.Reply, response.Handled, err
}

func (b *Bridge) resolveFileSearchToolInput(ctx context.Context, messageContext app.MessageContext, input string) (filesearch.ToolInput, bool, error) {
	if b.service == nil {
		return filesearch.ToolInput{}, false, nil
	}

	intent, ok, err := b.service.BuildWeixinFileSearchIntent(ctx, messageContext, input)
	if err != nil || !ok {
		return filesearch.ToolInput{}, ok, err
	}
	return intent.ToolInput, true, nil
}

func (b *Bridge) finalizeLogin(status *QRCodeStatusResponse) (Account, error) {
	if strings.TrimSpace(status.BotToken) == "" {
		return Account{}, fmt.Errorf("wechat login succeeded without bot token: %s", status.Message)
	}

	b.client.SetToken(status.BotToken)
	if status.BaseURL != "" {
		b.client = NewClient(status.BaseURL, status.BotToken)
	}

	account := Account{
		Token:     status.BotToken,
		BaseURL:   status.BaseURL,
		UserID:    status.ILinkUserID,
		AccountID: status.ILinkBotID,
	}
	if err := b.saveAccount(account); err != nil {
		return Account{}, err
	}
	return account, nil
}

func (b *Bridge) saveAccount(account Account) error {
	dir := filepath.Join(b.config.DataDir, "weixin-bridge")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(account, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "account.json"), data, 0o644)
}

func extractText(msg WeixinMessage) string {
	for _, item := range msg.ItemList {
		if item.Type == ItemTypeText && item.TextItem != nil {
			return strings.TrimSpace(item.TextItem.Text)
		}
		if item.Type == ItemTypeVoice && item.VoiceItem != nil && item.VoiceItem.Text != "" {
			return strings.TrimSpace(item.VoiceItem.Text)
		}
	}
	return ""
}

func splitByRunes(text string, size int) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}

	runes := []rune(text)
	if len(runes) <= size {
		return []string{text}
	}

	chunks := make([]string, 0, (len(runes)+size-1)/size)
	for start := 0; start < len(runes); start += size {
		end := min(start+size, len(runes))
		chunks = append(chunks, string(runes[start:end]))
	}
	return chunks
}

func truncate(text string, size int) string {
	runes := []rune(text)
	if len(runes) <= size {
		return text
	}
	return string(runes[:size]) + "..."
}

func readTextFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func writeTextFile(path, value string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(value), 0o644)
}
