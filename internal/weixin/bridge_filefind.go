package weixin

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"myclaw/internal/app"
	"myclaw/internal/filesearch"
)

const (
	findResultLimit = 10
	findStateTTL    = 15 * time.Minute
)

var (
	errEverythingUnsupported       = filesearch.ErrUnsupported
	errEverythingUnconfigured      = filesearch.ErrUnconfigured
	explicitEverythingQueryPattern = regexp.MustCompile(`(?i)(^|[\s<|])(?:[a-z]:|dm:|dc:|rc:|recentchange:|shell:|parent:|infolder:|nosubfolders:|folder:|file:|size:|regex:|case:|type:|path:|ext:)`)
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

func (b *Bridge) maybeHandleFileFind(ctx context.Context, msg WeixinMessage, messageContext app.MessageContext, text string) (string, bool, error) {
	if reply, handled, err := b.tryHandlePendingFileSelection(ctx, msg, text); handled || err != nil {
		return reply, true, err
	}

	command := normalizeSlashCommand(text)
	if strings.HasPrefix(strings.ToLower(command), "/find") {
		arg := strings.TrimSpace(strings.TrimPrefix(command, "/find"))
		if arg == "" {
			return "用法: /find <关键词>\n例如: /find 单细胞*.pdf", true, nil
		}
		if strings.EqualFold(arg, "help") || arg == "帮助" {
			return filesearch.CommandHelpText(), true, nil
		}
	}

	input, handled, err := b.buildFileSearchToolInput(ctx, messageContext, text)
	if err != nil || !handled {
		return "", handled, err
	}

	everythingPath := b.EverythingPath()
	result, err := b.searchFiles(ctx, everythingPath, input)
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
	if len(result.Items) == 0 {
		b.clearPendingFileSelection(b.conversationSlotKey(msg))
		return fmt.Sprintf("没有找到匹配文件：%s", result.Query), true, nil
	}

	paths := resultItemPaths(result.Items)
	b.savePendingFileSelection(b.conversationSlotKey(msg), pendingFileSelection{
		Query:     result.Query,
		Paths:     append([]string(nil), paths...),
		CreatedAt: time.Now(),
	})
	return formatPendingFileSelection(result.Query, paths), true, nil
}

func (b *Bridge) tryHandlePendingFileSelection(ctx context.Context, msg WeixinMessage, text string) (string, bool, error) {
	selection, ok := b.pendingFileSelection(b.conversationSlotKey(msg))
	if !ok {
		return "", false, nil
	}

	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return "", false, nil
	}

	if isCancelSelection(trimmed) {
		b.clearPendingFileSelection(b.conversationSlotKey(msg))
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

	b.clearPendingFileSelection(b.conversationSlotKey(msg))
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

func (b *Bridge) buildFileSearchToolInput(ctx context.Context, messageContext app.MessageContext, text string) (filesearch.ToolInput, bool, error) {
	command := normalizeSlashCommand(text)

	if strings.HasPrefix(strings.ToLower(command), "/find") {
		rawQuery := strings.TrimSpace(strings.TrimPrefix(command, "/find"))
		if rawQuery == "" {
			return filesearch.ToolInput{}, true, nil
		}
		if b.service != nil && !looksLikeExplicitEverythingQuery(rawQuery) {
			intent, ok, err := b.service.BuildWeixinFileSearchIntent(ctx, messageContext, rawQuery)
			if err != nil {
				return filesearch.ToolInput{}, true, err
			}
			if ok {
				intent.ToolInput.Limit = findResultLimit
				return intent.ToolInput, true, nil
			}
		}
		return filesearch.ToolInput{
			Query: rawQuery,
			Limit: findResultLimit,
		}, true, nil
	}

	if b.service == nil {
		return filesearch.ToolInput{}, false, nil
	}
	intent, ok, err := b.service.BuildWeixinFileSearchIntent(ctx, messageContext, text)
	if err != nil {
		return filesearch.ToolInput{}, false, err
	}
	if !ok {
		return filesearch.ToolInput{}, false, nil
	}
	intent.ToolInput.Limit = findResultLimit
	return intent.ToolInput, true, nil
}

func looksLikeExplicitEverythingQuery(text string) bool {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return false
	}
	if explicitEverythingQueryPattern.MatchString(trimmed) {
		return true
	}
	return strings.ContainsAny(trimmed, `*?|!"`)
}

func resultItemPaths(items []filesearch.ResultItem) []string {
	paths := make([]string, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(item.Path) == "" {
			continue
		}
		paths = append(paths, item.Path)
	}
	return paths
}
