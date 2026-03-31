package filesearch

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const DefaultSelectionTTL = 15 * time.Minute

var explicitEverythingQueryPattern = regexp.MustCompile(`(?i)(^|[\s<|])(?:[a-z]:|dm:|dc:|rc:|recentchange:|shell:|parent:|infolder:|nosubfolders:|folder:|file:|size:|regex:|case:|type:|path:|ext:)`)

type SearchExecutor func(context.Context, string, ToolInput) (ToolResult, error)

type IntentResolver func(context.Context, string) (ToolInput, bool, error)

type FileSender func(context.Context, string) error

type PendingSelection struct {
	Query     string
	Paths     []string
	CreatedAt time.Time
}

type ShortcutRequest struct {
	SlotKey          string
	Text             string
	ResolveIntent    IntentResolver
	SendSelectedFile FileSender
}

type ShortcutResponse struct {
	Reply   string
	Handled bool
}

type ShortcutHandler struct {
	mu             sync.Mutex
	everythingPath string
	search         SearchExecutor
	pending        map[string]PendingSelection
	resultLimit    int
	selectionTTL   time.Duration
}

func NewShortcutHandler(everythingPath string, search SearchExecutor) *ShortcutHandler {
	if search == nil {
		search = ExecuteWithEverything
	}
	return &ShortcutHandler{
		everythingPath: strings.TrimSpace(everythingPath),
		search:         search,
		pending:        make(map[string]PendingSelection),
		resultLimit:    DefaultLimit,
		selectionTTL:   DefaultSelectionTTL,
	}
}

func (h *ShortcutHandler) SetEverythingPath(path string) {
	h.mu.Lock()
	h.everythingPath = strings.TrimSpace(path)
	h.mu.Unlock()
}

func (h *ShortcutHandler) EverythingPath() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.everythingPath
}

func (h *ShortcutHandler) SetSearchExecutor(search SearchExecutor) {
	if search == nil {
		search = ExecuteWithEverything
	}
	h.mu.Lock()
	h.search = search
	h.mu.Unlock()
}

func (h *ShortcutHandler) PendingSelection(slotKey string) (PendingSelection, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.pendingSelectionLocked(strings.TrimSpace(slotKey))
}

func (h *ShortcutHandler) ClearPendingSelection(slotKey string) {
	h.mu.Lock()
	delete(h.pending, strings.TrimSpace(slotKey))
	h.mu.Unlock()
}

func (h *ShortcutHandler) Handle(ctx context.Context, req ShortcutRequest) (ShortcutResponse, error) {
	if reply, handled, err := h.tryHandlePendingSelection(ctx, req); handled || err != nil {
		return ShortcutResponse{Reply: reply, Handled: handled || err != nil}, err
	}

	input, reply, handled, err := h.resolveInput(ctx, req.Text, req.ResolveIntent)
	if err != nil || !handled {
		return ShortcutResponse{Reply: reply, Handled: handled}, err
	}
	if reply != "" {
		return ShortcutResponse{Reply: reply, Handled: true}, nil
	}

	slotKey := strings.TrimSpace(req.SlotKey)
	if slotKey == "" {
		return ShortcutResponse{}, fmt.Errorf("filesearch: missing slot key")
	}

	everythingPath, search := h.snapshotConfig()
	result, err := search(ctx, everythingPath, input)
	if err != nil {
		switch {
		case errors.Is(err, ErrUnsupported):
			return ShortcutResponse{Reply: err.Error(), Handled: true}, nil
		case errors.Is(err, ErrUnconfigured):
			return ShortcutResponse{Reply: err.Error(), Handled: true}, nil
		default:
			return ShortcutResponse{Handled: true}, err
		}
	}

	if len(result.Items) == 0 {
		h.ClearPendingSelection(slotKey)
		return ShortcutResponse{
			Reply:   fmt.Sprintf("没有找到匹配文件：%s", strings.TrimSpace(result.Query)),
			Handled: true,
		}, nil
	}

	paths := resultItemPaths(result.Items)
	h.savePendingSelection(slotKey, PendingSelection{
		Query:     result.Query,
		Paths:     append([]string(nil), paths...),
		CreatedAt: time.Now(),
	})
	return ShortcutResponse{
		Reply:   FormatPendingSelection(result.Query, paths),
		Handled: true,
	}, nil
}

func (h *ShortcutHandler) resolveInput(ctx context.Context, text string, resolveIntent IntentResolver) (ToolInput, string, bool, error) {
	command := normalizeShortcutCommand(text)
	if strings.HasPrefix(strings.ToLower(command), ShortcutName) {
		rawQuery := strings.TrimSpace(strings.TrimPrefix(command, ShortcutName))
		if rawQuery == "" {
			return ToolInput{}, ShortcutUsageText(), true, nil
		}
		if strings.EqualFold(rawQuery, "help") || rawQuery == "帮助" {
			return ToolInput{}, CommandHelpText(), true, nil
		}
		if resolveIntent != nil && !LooksLikeExplicitQuery(rawQuery) {
			input, ok, err := resolveIntent(ctx, rawQuery)
			if err != nil {
				return ToolInput{}, "", true, err
			}
			if ok {
				return h.withResultLimit(input), "", true, nil
			}
		}
		return h.withResultLimit(ToolInput{Query: rawQuery}), "", true, nil
	}

	if resolveIntent == nil {
		return ToolInput{}, "", false, nil
	}
	input, ok, err := resolveIntent(ctx, text)
	if err != nil {
		return ToolInput{}, "", false, err
	}
	if !ok {
		return ToolInput{}, "", false, nil
	}
	return h.withResultLimit(input), "", true, nil
}

func (h *ShortcutHandler) tryHandlePendingSelection(ctx context.Context, req ShortcutRequest) (string, bool, error) {
	slotKey := strings.TrimSpace(req.SlotKey)
	if slotKey == "" {
		return "", false, nil
	}

	selection, ok := h.PendingSelection(slotKey)
	command := normalizeShortcutCommand(req.Text)
	if !ok {
		if strings.HasPrefix(strings.ToLower(command), SendShortcutName) {
			return "当前没有待发送文件，请先使用 /find 查找。", true, nil
		}
		return "", false, nil
	}

	if !strings.HasPrefix(strings.ToLower(command), SendShortcutName) {
		return "", false, nil
	}
	arg := strings.TrimSpace(strings.TrimPrefix(command, SendShortcutName))
	if arg == "" {
		return "用法: /send <序号>\n例如: /send 1", true, nil
	}
	if IsCancelSelection(arg) {
		h.ClearPendingSelection(slotKey)
		return "已取消本次文件发送。", true, nil
	}
	index, ok := ParseSelectionIndex(arg, len(selection.Paths))
	if !ok {
		return "请使用 `/send <序号>` 发送文件，例如 `/send 1`。", true, nil
	}
	if req.SendSelectedFile == nil {
		return "", true, fmt.Errorf("filesearch: missing file sender")
	}

	target := selection.Paths[index-1]
	if err := req.SendSelectedFile(ctx, target); err != nil {
		return "", true, err
	}

	h.ClearPendingSelection(slotKey)
	return fmt.Sprintf("已发送文件 %d: %s", index, fileBaseName(target)), true, nil
}

func (h *ShortcutHandler) pendingSelectionLocked(slotKey string) (PendingSelection, bool) {
	selection, ok := h.pending[slotKey]
	if !ok {
		return PendingSelection{}, false
	}
	if time.Since(selection.CreatedAt) > h.selectionTTL {
		delete(h.pending, slotKey)
		return PendingSelection{}, false
	}
	return selection, true
}

func (h *ShortcutHandler) savePendingSelection(slotKey string, selection PendingSelection) {
	h.mu.Lock()
	if h.pending == nil {
		h.pending = make(map[string]PendingSelection)
	}
	h.pending[slotKey] = selection
	h.mu.Unlock()
}

func (h *ShortcutHandler) snapshotConfig() (string, SearchExecutor) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.everythingPath, h.search
}

func (h *ShortcutHandler) withResultLimit(input ToolInput) ToolInput {
	input = NormalizeInput(input)
	input.Limit = h.resultLimit
	return input
}

func FormatPendingSelection(query string, paths []string) string {
	lines := []string{
		fmt.Sprintf("找到 %d 个文件，回复序号即可发送给你：", len(paths)),
	}
	for idx, item := range paths {
		lines = append(lines, fmt.Sprintf("%d. %s", idx+1, fileBaseName(item)))
		lines = append(lines, "   "+item)
	}
	lines = append(lines, fmt.Sprintf("检索式: %s", query))
	lines = append(lines, "发送请用 `/send 1-"+strconv.Itoa(len(paths))+"`，取消请用 `/send cancel`。")
	return strings.Join(lines, "\n")
}

func ParseSelectionIndex(text string, max int) (int, bool) {
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

func IsCancelSelection(text string) bool {
	switch strings.ToLower(strings.TrimSpace(text)) {
	case "0", "取消", "/cancel", "cancel":
		return true
	default:
		return false
	}
}

func LooksLikeExplicitQuery(text string) bool {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return false
	}
	if explicitEverythingQueryPattern.MatchString(trimmed) {
		return true
	}
	return strings.ContainsAny(trimmed, `*?|!"`)
}

func normalizeShortcutCommand(text string) string {
	text = strings.TrimSpace(text)
	text = strings.ReplaceAll(text, "／", "/")
	return text
}

func shortcutUsageText() string {
	return "用法: /find <关键词>\n例如: /find 单细胞*.pdf"
}

func ShortcutUsageText() string {
	return shortcutUsageText()
}

func FormatSearchResult(result ToolResult) string {
	if len(result.Items) == 0 {
		query := strings.TrimSpace(result.Query)
		if query == "" {
			return "没有找到匹配文件。"
		}
		return fmt.Sprintf("没有找到匹配文件：%s", query)
	}

	lines := []string{
		fmt.Sprintf("找到 %d 个文件：", len(result.Items)),
	}
	for idx, item := range result.Items {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			name = fileBaseName(item.Path)
		}
		lines = append(lines, fmt.Sprintf("%d. %s", idx+1, name))
		if strings.TrimSpace(item.Path) != "" {
			lines = append(lines, "   "+item.Path)
		}
	}
	if query := strings.TrimSpace(result.Query); query != "" {
		lines = append(lines, fmt.Sprintf("检索式: %s", query))
	}
	return strings.Join(lines, "\n")
}

func resultItemPaths(items []ResultItem) []string {
	paths := make([]string, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(item.Path) == "" {
			continue
		}
		paths = append(paths, item.Path)
	}
	return paths
}
