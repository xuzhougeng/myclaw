package main

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"runtime/debug"
	"strings"
	"time"

	"baize/internal/weixin"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	qr "rsc.io/qr"
)

const weixinLoginTimeout = 8 * time.Minute

type WeixinStatus struct {
	Connected     bool   `json:"connected"`
	LoggingIn     bool   `json:"loggingIn"`
	HasAccount    bool   `json:"hasAccount"`
	AccountID     string `json:"accountId"`
	UserID        string `json:"userId"`
	QRCode        string `json:"qrCode"`
	QRCodeDataURL string `json:"qrCodeDataUrl"`
	Message       string `json:"message"`
}

func defaultWeixinStatus() WeixinStatus {
	return WeixinStatus{
		Message: "未连接微信，可在桌面端直接生成二维码扫码登录。",
	}
}

func (a *DesktopApp) initWeixin() {
	if a.weixinBridge == nil {
		return
	}

	if !a.weixinBridge.LoadAccount() {
		a.setWeixinStatus(defaultWeixinStatus())
		return
	}

	account, _ := a.weixinBridge.ReadSavedAccount()
	a.setWeixinStatus(WeixinStatus{
		Connected:  true,
		HasAccount: true,
		AccountID:  account.AccountID,
		UserID:     account.UserID,
		Message:    "微信已连接，桌面端正在接收消息。",
	})
	a.startWeixinBridge()
}

func (a *DesktopApp) stopWeixin() {
	a.weixinMu.Lock()
	loginCancel := a.weixinLoginCancel
	runCancel := a.weixinRunCancel
	a.weixinLoginCancel = nil
	a.weixinRunCancel = nil
	a.weixinMu.Unlock()

	if loginCancel != nil {
		loginCancel()
	}
	if runCancel != nil {
		runCancel()
	}
}

func (a *DesktopApp) GetWeixinStatus() WeixinStatus {
	a.weixinMu.Lock()
	defer a.weixinMu.Unlock()
	return a.weixinStatus
}

func (a *DesktopApp) StartWeixinLogin() (WeixinStatus, error) {
	if a.weixinBridge == nil {
		return WeixinStatus{}, errors.New("当前构建未启用微信模块。")
	}

	current := a.GetWeixinStatus()
	if current.Connected || current.LoggingIn {
		return current, nil
	}

	a.publishWeixinStatus(WeixinStatus{
		LoggingIn: true,
		Message:   "正在获取微信登录二维码...",
	})

	qrCode, err := a.weixinBridge.StartLogin()
	if err != nil {
		status := defaultWeixinStatus()
		status.Message = "生成二维码失败：" + err.Error()
		a.publishWeixinStatus(status)
		return status, err
	}

	dataURL, err := encodeWeixinQRCodeDataURL(qrCode.QRCodeImgContent)
	if err != nil {
		status := defaultWeixinStatus()
		status.Message = "二维码渲染失败：" + err.Error()
		a.publishWeixinStatus(status)
		return status, err
	}

	loginCtx, cancel := context.WithCancel(context.Background())

	status := WeixinStatus{
		LoggingIn:     true,
		QRCode:        qrCode.QRCode,
		QRCodeDataURL: dataURL,
		Message:       "请用微信扫码，并在手机上确认登录。二维码 8 分钟内有效。",
	}

	a.weixinMu.Lock()
	if a.weixinLoginCancel != nil {
		a.weixinLoginCancel()
	}
	a.weixinLoginCancel = cancel
	a.weixinStatus = status
	a.weixinMu.Unlock()
	a.emitWeixinStatus(status)

	go a.awaitWeixinLogin(loginCtx, qrCode.QRCode)
	return status, nil
}

func (a *DesktopApp) CancelWeixinLogin() (MessageResult, error) {
	a.weixinMu.Lock()
	cancel := a.weixinLoginCancel
	a.weixinLoginCancel = nil
	current := a.weixinStatus
	a.weixinMu.Unlock()

	if cancel != nil {
		cancel()
	}
	if !current.LoggingIn {
		return MessageResult{Message: "当前没有进行中的微信扫码登录。"}, nil
	}

	status := defaultWeixinStatus()
	status.Message = "已取消微信扫码登录。"
	a.publishWeixinStatus(status)
	return MessageResult{Message: status.Message}, nil
}

func (a *DesktopApp) LogoutWeixin() (MessageResult, error) {
	if a.weixinBridge == nil {
		return MessageResult{}, errors.New("当前构建未启用微信模块。")
	}

	a.stopWeixin()
	if err := a.weixinBridge.Logout(); err != nil {
		return MessageResult{}, err
	}

	status := defaultWeixinStatus()
	status.Message = "微信已退出登录。"
	a.publishWeixinStatus(status)
	return MessageResult{Message: status.Message}, nil
}

func (a *DesktopApp) awaitWeixinLogin(ctx context.Context, qrCode string) {
	account, err := a.weixinBridge.WaitLogin(ctx, qrCode, weixinLoginTimeout)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return
		}

		status := defaultWeixinStatus()
		status.Message = describeWeixinLoginError(err)

		a.weixinMu.Lock()
		if a.weixinLoginCancel != nil {
			a.weixinLoginCancel = nil
		}
		a.weixinStatus = status
		a.weixinMu.Unlock()
		a.emitWeixinStatus(status)
		return
	}

	status := WeixinStatus{
		Connected:  true,
		HasAccount: true,
		AccountID:  account.AccountID,
		UserID:     account.UserID,
		Message:    "微信已连接，桌面端正在接收消息。",
	}

	a.weixinMu.Lock()
	a.weixinLoginCancel = nil
	a.weixinStatus = status
	a.weixinMu.Unlock()
	a.emitWeixinStatus(status)
	a.startWeixinBridge()
}

func (a *DesktopApp) startWeixinBridge() {
	if a.weixinBridge == nil {
		return
	}

	a.weixinMu.Lock()
	if a.weixinRunCancel != nil {
		a.weixinRunCancel()
	}
	runCtx, cancel := context.WithCancel(context.Background())
	a.weixinRunCancel = cancel
	a.weixinMu.Unlock()

	go func() {
		reportDesktopBackendEvent(a.dataDir, "desktop.weixinBridge.start", nil)
		defer func() {
			if recovered := recover(); recovered != nil {
				reportDesktopBackendPanic(a.dataDir, "desktop.weixinBridge.run", recovered, debug.Stack())
			}
		}()

		err := a.weixinBridge.Run(runCtx)
		reportDesktopBackendEvent(a.dataDir, "desktop.weixinBridge.exit", map[string]string{
			"error": strings.TrimSpace(fmt.Sprint(err)),
		})
		if errors.Is(err, context.Canceled) || runCtx.Err() != nil {
			return
		}

		status := defaultWeixinStatus()
		switch {
		case errors.Is(err, weixin.ErrSessionExpired):
			status.Message = "微信登录已失效，请重新扫码。"
			if logoutErr := a.weixinBridge.Logout(); logoutErr != nil {
				log.Printf("clear expired weixin account: %v", logoutErr)
			}
		case err != nil:
			status.Message = "微信连接已中断：" + err.Error()
		}

		log.Printf("weixin bridge stopped: %v", err)

		a.weixinMu.Lock()
		a.weixinRunCancel = nil
		a.weixinStatus = status
		a.weixinMu.Unlock()
		a.emitWeixinStatus(status)
	}()
}

func (a *DesktopApp) setWeixinStatus(status WeixinStatus) {
	a.weixinMu.Lock()
	a.weixinStatus = status
	a.weixinMu.Unlock()
}

func (a *DesktopApp) publishWeixinStatus(status WeixinStatus) {
	a.setWeixinStatus(status)
	a.emitWeixinStatus(status)
}

func (a *DesktopApp) emitWeixinStatus(status WeixinStatus) {
	if a.ctx == nil {
		return
	}
	runtime.EventsEmit(a.ctx, "weixin:status", status)
}

func encodeWeixinQRCodeDataURL(content string) (string, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return "", errors.New("微信接口没有返回二维码内容")
	}

	code, err := qr.Encode(content, qr.M)
	if err != nil {
		return "", err
	}
	code.Scale = 10

	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(code.PNG()), nil
}

func describeWeixinLoginError(err error) string {
	switch {
	case err == nil:
		return "微信登录失败。"
	case errors.Is(err, context.DeadlineExceeded):
		return "微信扫码超时，请重新生成二维码。"
	case strings.Contains(strings.ToLower(err.Error()), "timed out"):
		return "微信扫码超时，请重新生成二维码。"
	default:
		return fmt.Sprintf("微信登录失败：%s", err.Error())
	}
}
