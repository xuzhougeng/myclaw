package main

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os/exec"
	goruntime "runtime"
	"strings"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

func (a *DesktopApp) OpenExternalURL(rawURL string) (MessageResult, error) {
	target, err := normalizeExternalURL(rawURL)
	if err != nil {
		return MessageResult{}, err
	}
	if err := openExternalURL(a.ctx, target); err != nil {
		return MessageResult{}, err
	}
	return MessageResult{Message: fmt.Sprintf("已打开链接：%s", target)}, nil
}

func normalizeExternalURL(rawURL string) (string, error) {
	value := strings.TrimSpace(rawURL)
	if value == "" {
		return "", errors.New("链接地址不能为空")
	}

	parsed, err := url.Parse(value)
	if err != nil {
		return "", fmt.Errorf("链接地址无效: %w", err)
	}
	scheme := strings.ToLower(strings.TrimSpace(parsed.Scheme))
	if scheme != "http" && scheme != "https" {
		return "", errors.New("目前仅支持打开 http/https 链接")
	}
	if strings.TrimSpace(parsed.Host) == "" {
		return "", errors.New("链接地址缺少主机名")
	}
	return parsed.String(), nil
}

func openExternalURL(ctx context.Context, target string) error {
	if ctx != nil {
		wailsruntime.BrowserOpenURL(ctx, target)
		return nil
	}

	var cmd *exec.Cmd
	switch goruntime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", target)
	case "darwin":
		cmd = exec.Command("open", target)
	default:
		cmd = exec.Command("xdg-open", target)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("打开链接失败: %w", err)
	}
	return nil
}
