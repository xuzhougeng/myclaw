package weixin

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"myclaw/internal/app"
)

const (
	findResultLimit = 10
	findStateTTL    = 15 * time.Minute
)

var (
	errEverythingUnsupported  = errors.New("当前仅 Windows 支持 /find，macOS/Linux 暂未实现")
	errEverythingUnconfigured = errors.New("请先在设置里配置 es.exe 路径")
)

type pendingFileSelection struct {
	Query     string
	Paths     []string
	CreatedAt time.Time
}

func (b *Bridge) SetEverythingPath(path string) {
	b.findMu.Lock()
	b.everythingPath = strings.TrimSpace(path)
	b.findMu.Unlock()
}

func (b *Bridge) EverythingPath() string {
	b.findMu.Lock()
	defer b.findMu.Unlock()
	return b.everythingPath
}

func (b *Bridge) maybeHandleFileFind(ctx context.Context, msg WeixinMessage, text string) (string, bool, error) {
	if reply, handled, err := b.tryHandlePendingFileSelection(ctx, msg, text); handled || err != nil {
		return reply, true, err
	}

	command := normalizeSlashCommand(text)
	query := ""
	var err error
	if strings.HasPrefix(strings.ToLower(command), "/find") {
		query = strings.TrimSpace(strings.TrimPrefix(command, "/find"))
		if query == "" {
			return "用法: /find <关键词>\n例如: /find 单细胞*.pdf", true, nil
		}
	} else {
		if b.service == nil {
			return "", false, nil
		}
		var ok bool
		query, ok, err = b.service.BuildWeixinFileSearchQuery(ctx, app.MessageContext{
			UserID:    msg.FromUserID,
			Interface: "weixin",
			SessionID: weixinSessionID(msg),
		}, text)
		if err != nil {
			return "", false, err
		}
		if !ok {
			return "", false, nil
		}
	}

	everythingPath := b.EverythingPath()
	paths, err := b.searchFiles(ctx, everythingPath, query, findResultLimit)
	if err != nil {
		switch {
		case errors.Is(err, errEverythingUnsupported):
			return err.Error(), true, nil
		case errors.Is(err, errEverythingUnconfigured):
			return err.Error(), true, nil
		default:
			return "", true, err
		}
	}
	if len(paths) == 0 {
		b.clearPendingFileSelection(weixinSessionID(msg))
		return fmt.Sprintf("没有找到匹配文件：%s", query), true, nil
	}

	b.savePendingFileSelection(weixinSessionID(msg), pendingFileSelection{
		Query:     query,
		Paths:     append([]string(nil), paths...),
		CreatedAt: time.Now(),
	})
	return formatPendingFileSelection(query, paths), true, nil
}

func (b *Bridge) tryHandlePendingFileSelection(ctx context.Context, msg WeixinMessage, text string) (string, bool, error) {
	selection, ok := b.pendingFileSelection(weixinSessionID(msg))
	if !ok {
		return "", false, nil
	}

	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return "", false, nil
	}

	if isCancelSelection(trimmed) {
		b.clearPendingFileSelection(weixinSessionID(msg))
		return "已取消本次文件选择。", true, nil
	}

	index, ok := parseSelectionIndex(trimmed, len(selection.Paths))
	if !ok {
		return "", false, nil
	}

	target := selection.Paths[index-1]
	if err := b.sendFile(ctx, msg.FromUserID, msg.ContextToken, target); err != nil {
		return "", true, err
	}

	b.clearPendingFileSelection(weixinSessionID(msg))
	return fmt.Sprintf("已通过 ClawBot 发送文件 %d: %s", index, fileBaseName(target)), true, nil
}

func (b *Bridge) pendingFileSelection(key string) (pendingFileSelection, bool) {
	b.findMu.Lock()
	defer b.findMu.Unlock()

	selection, ok := b.pendingFind[key]
	if !ok {
		return pendingFileSelection{}, false
	}
	if time.Since(selection.CreatedAt) > findStateTTL {
		delete(b.pendingFind, key)
		return pendingFileSelection{}, false
	}
	return selection, true
}

func (b *Bridge) savePendingFileSelection(key string, selection pendingFileSelection) {
	b.findMu.Lock()
	if b.pendingFind == nil {
		b.pendingFind = make(map[string]pendingFileSelection)
	}
	b.pendingFind[key] = selection
	b.findMu.Unlock()
}

func (b *Bridge) clearPendingFileSelection(key string) {
	b.findMu.Lock()
	delete(b.pendingFind, key)
	b.findMu.Unlock()
}

func formatPendingFileSelection(query string, paths []string) string {
	lines := []string{
		fmt.Sprintf("找到 %d 个文件，回复序号即可发送给你：", len(paths)),
	}
	for idx, item := range paths {
		lines = append(lines, fmt.Sprintf("%d. %s", idx+1, fileBaseName(item)))
		lines = append(lines, "   "+item)
	}
	lines = append(lines, fmt.Sprintf("检索式: %s", query))
	lines = append(lines, "回复 1-"+strconv.Itoa(len(paths))+" 选择，回复 0 / 取消 结束。")
	return strings.Join(lines, "\n")
}

func parseSelectionIndex(text string, max int) (int, bool) {
	text = strings.TrimSpace(strings.TrimPrefix(text, "#"))
	text = strings.TrimSpace(strings.TrimPrefix(text, "选择"))
	text = strings.TrimSpace(strings.TrimPrefix(text, "选"))
	text = strings.TrimSpace(strings.TrimPrefix(text, "发送"))
	if text == "" {
		return 0, false
	}

	value, err := strconv.Atoi(text)
	if err != nil || value < 1 || value > max {
		return 0, false
	}
	return value, true
}

func isCancelSelection(text string) bool {
	switch strings.ToLower(strings.TrimSpace(text)) {
	case "0", "取消", "/cancel", "cancel":
		return true
	default:
		return false
	}
}

func normalizeSlashCommand(text string) string {
	text = strings.TrimSpace(text)
	text = strings.ReplaceAll(text, "／", "/")
	return text
}

func searchFilesWithEverything(ctx context.Context, everythingPath, query string, limit int) ([]string, error) {
	if runtime.GOOS != "windows" {
		return nil, errEverythingUnsupported
	}

	commandPath := strings.Trim(strings.TrimSpace(everythingPath), "\"")
	if commandPath == "" {
		return nil, errEverythingUnconfigured
	}

	searchCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	args := []string{strings.TrimSpace(query)}
	if limit > 0 {
		args = []string{"-n", strconv.Itoa(limit), strings.TrimSpace(query)}
	}
	output, err := exec.CommandContext(searchCtx, commandPath, args...).Output()
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return nil, fmt.Errorf("%w: %s", errEverythingUnconfigured, commandPath)
		}
		return nil, fmt.Errorf("执行 es.exe 失败: %w", err)
	}

	lines := strings.Split(strings.ReplaceAll(string(output), "\r\n", "\n"), "\n")
	results := make([]string, 0, limit)
	seen := make(map[string]struct{})
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		key := strings.ToLower(line)
		if _, ok := seen[key]; ok {
			continue
		}
		info, statErr := os.Stat(line)
		if statErr != nil || info.IsDir() {
			continue
		}
		seen[key] = struct{}{}
		results = append(results, line)
		if limit > 0 && len(results) >= limit {
			break
		}
	}
	return results, nil
}
