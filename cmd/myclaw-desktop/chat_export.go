package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"myclaw/internal/sessionstate"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type ChatMarkdownExport struct {
	Filename string `json:"filename"`
	Markdown string `json:"markdown"`
}

type chatOptionPayload struct {
	Question string
	Options  []chatOption
}

type chatOption struct {
	Label string
	Value string
}

var (
	chatOptionQuestionPattern = regexp.MustCompile(`:question\s+"((?:\\.|[^"])*)"`)
	chatOptionOptionsPattern  = regexp.MustCompile(`(?s):options\s+\[(.*)\]`)
	chatOptionStringPattern   = regexp.MustCompile(`"((?:\\.|[^"])*)"`)
)

func (a *DesktopApp) ExportChatMarkdown() (MessageResult, error) {
	if a.ctx == nil {
		return MessageResult{}, errors.New("桌面上下文尚未初始化")
	}

	export, err := a.buildCurrentChatMarkdownExport(context.Background())
	if err != nil {
		return MessageResult{}, err
	}

	targetPath, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:            "导出当前对话为 Markdown",
		DefaultDirectory: a.defaultDialogDirectory(),
		DefaultFilename:  export.Filename,
		Filters: []runtime.FileFilter{
			{
				DisplayName: "Markdown Files",
				Pattern:     "*.md",
			},
		},
	})
	if err != nil {
		return MessageResult{}, err
	}

	targetPath = strings.TrimSpace(targetPath)
	if targetPath == "" {
		return MessageResult{Message: "已取消导出。"}, nil
	}
	if !strings.HasSuffix(strings.ToLower(targetPath), ".md") {
		targetPath += ".md"
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return MessageResult{}, err
	}
	if err := os.WriteFile(targetPath, []byte(export.Markdown), 0o644); err != nil {
		return MessageResult{}, err
	}
	return MessageResult{Message: fmt.Sprintf("已导出 Markdown：%s", targetPath)}, nil
}

func (a *DesktopApp) buildCurrentChatMarkdownExport(ctx context.Context) (ChatMarkdownExport, error) {
	project, err := a.currentProject(ctx)
	if err != nil {
		return ChatMarkdownExport{}, err
	}

	sessionID, err := a.currentChatSession(ctx, project)
	if err != nil {
		return ChatMarkdownExport{}, err
	}

	snapshot, ok, err := a.loadChatSessionSnapshot(ctx, project, sessionID)
	if err != nil {
		return ChatMarkdownExport{}, err
	}
	if !ok || countChatExportMessages(snapshot.History) == 0 {
		return ChatMarkdownExport{}, errors.New("当前对话还没有可导出的消息")
	}

	return ChatMarkdownExport{
		Filename: defaultChatMarkdownFilename(project, snapshot),
		Markdown: renderChatMarkdown(project, sessionID, snapshot),
	}, nil
}

func countChatExportMessages(history []sessionstate.Message) int {
	total := 0
	for _, item := range history {
		if strings.TrimSpace(item.Role) == "" || strings.TrimSpace(item.Content) == "" {
			continue
		}
		total++
	}
	return total
}

func defaultChatMarkdownFilename(project string, snapshot sessionstate.Snapshot) string {
	title := sanitizeChatMarkdownFilenameSegment(chatConversationTitle(snapshot.History))
	if title == "" {
		title = "conversation"
	}

	stamp := snapshot.UpdatedAt.Local().Format("20060102-150405")
	if snapshot.UpdatedAt.IsZero() {
		stamp = "export"
	}

	project = sanitizeChatMarkdownFilenameSegment(project)
	if project == "" {
		project = "default"
	}
	return fmt.Sprintf("myclaw-chat-%s-%s-%s.md", project, title, stamp)
}

func sanitizeChatMarkdownFilenameSegment(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}

	var builder strings.Builder
	lastDash := false
	for _, r := range value {
		switch {
		case r == '/' || r == '\\' || r == ':' || r == '*' || r == '?' || r == '"' || r == '<' || r == '>' || r == '|':
			if !lastDash {
				builder.WriteRune('-')
				lastDash = true
			}
		case r <= 31:
			continue
		case r == ' ' || r == '\t' || r == '\n' || r == '\r':
			if !lastDash {
				builder.WriteRune('-')
				lastDash = true
			}
		default:
			builder.WriteRune(r)
			lastDash = false
		}
	}

	return strings.Trim(builder.String(), "-. ")
}

func renderChatMarkdown(project, sessionID string, snapshot sessionstate.Snapshot) string {
	title := strings.TrimSpace(chatConversationTitle(snapshot.History))
	if title == "" {
		title = "新对话"
	}

	messageCount := countChatExportMessages(snapshot.History)
	exportedAt := snapshot.UpdatedAt.Local().Format("2006-01-02 15:04:05")
	if snapshot.UpdatedAt.IsZero() {
		exportedAt = ""
	}

	var builder strings.Builder
	builder.WriteString("# ")
	builder.WriteString(title)
	builder.WriteString("\n\n")
	builder.WriteString("- 项目：")
	builder.WriteString(strings.TrimSpace(project))
	builder.WriteString("\n")
	builder.WriteString("- 对话 ID：`")
	builder.WriteString(strings.TrimSpace(sessionID))
	builder.WriteString("`\n")
	if exportedAt != "" {
		builder.WriteString("- 最后更新：")
		builder.WriteString(exportedAt)
		builder.WriteString("\n")
	}
	builder.WriteString("- 消息数：")
	builder.WriteString(strconv.Itoa(messageCount))
	builder.WriteString("\n\n---\n\n")

	written := 0
	for _, item := range snapshot.History {
		role := strings.TrimSpace(item.Role)
		content := strings.TrimSpace(item.Content)
		if role == "" || content == "" {
			continue
		}

		if written > 0 {
			builder.WriteString("\n---\n\n")
		}
		builder.WriteString("## ")
		builder.WriteString(chatMarkdownRoleLabel(role))
		builder.WriteString("\n\n")
		builder.WriteString(renderChatMarkdownContent(content))
		builder.WriteString("\n")
		written++
	}

	return strings.TrimSpace(builder.String()) + "\n"
}

func chatMarkdownRoleLabel(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "user":
		return "用户"
	case "assistant":
		return "助手"
	case "system":
		return "系统"
	default:
		return strings.TrimSpace(role)
	}
}

func renderChatMarkdownContent(content string) string {
	if payload, ok := parseChatOptionPayload(content); ok {
		return renderChatOptionMarkdown(payload)
	}
	return strings.TrimSpace(content)
}

func renderChatOptionMarkdown(payload chatOptionPayload) string {
	var builder strings.Builder
	builder.WriteString(strings.TrimSpace(payload.Question))
	builder.WriteString("\n\n")
	for _, option := range payload.Options {
		label := strings.TrimSpace(option.Label)
		value := strings.TrimSpace(option.Value)
		if label == "" {
			continue
		}
		builder.WriteString("- ")
		builder.WriteString(label)
		if value != "" && value != label {
			builder.WriteString(" (`")
			builder.WriteString(value)
			builder.WriteString("`)")
		}
		builder.WriteString("\n")
	}
	return strings.TrimSpace(builder.String())
}

func parseChatOptionPayload(content string) (chatOptionPayload, bool) {
	text := strings.TrimSpace(content)
	if !strings.HasPrefix(text, "{") || !strings.HasSuffix(text, "}") {
		return chatOptionPayload{}, false
	}

	if payload, ok := parseJSONChatOptionPayload(text); ok {
		return payload, true
	}
	return parseEDNChatOptionPayload(text)
}

func parseJSONChatOptionPayload(content string) (chatOptionPayload, bool) {
	var payload map[string]any
	if err := json.Unmarshal([]byte(content), &payload); err != nil {
		return chatOptionPayload{}, false
	}
	return normalizeChatOptionPayload(payload)
}

func parseEDNChatOptionPayload(content string) (chatOptionPayload, bool) {
	questionMatch := chatOptionQuestionPattern.FindStringSubmatch(content)
	optionsMatch := chatOptionOptionsPattern.FindStringSubmatch(content)
	if len(questionMatch) < 2 || len(optionsMatch) < 2 {
		return chatOptionPayload{}, false
	}

	options := make([]string, 0, 4)
	for _, item := range chatOptionStringPattern.FindAllStringSubmatch(optionsMatch[1], -1) {
		if len(item) < 2 {
			continue
		}
		options = append(options, unescapeChatOptionText(item[1]))
	}

	return normalizeChatOptionPayload(map[string]any{
		"question": unescapeChatOptionText(questionMatch[1]),
		"options":  options,
	})
}

func normalizeChatOptionPayload(payload map[string]any) (chatOptionPayload, bool) {
	questionType := normalizeChatOptionScalar(payload["questiontype"])
	if questionType == "" {
		questionType = normalizeChatOptionScalar(payload["questionType"])
	}
	if questionType != "" && !strings.EqualFold(questionType, "singleselect") {
		return chatOptionPayload{}, false
	}

	question := normalizeChatOptionScalar(payload["question"])
	options := normalizeChatOptionList(payload["options"])
	if question == "" || len(options) == 0 {
		return chatOptionPayload{}, false
	}
	return chatOptionPayload{Question: question, Options: options}, true
}

func normalizeChatOptionList(value any) []chatOption {
	items, ok := value.([]any)
	if !ok {
		if strings, ok := value.([]string); ok {
			items = make([]any, 0, len(strings))
			for _, item := range strings {
				items = append(items, item)
			}
		} else {
			return nil
		}
	}

	out := make([]chatOption, 0, len(items))
	for _, item := range items {
		option, ok := normalizeChatOption(item)
		if ok {
			out = append(out, option)
		}
	}
	return out
}

func normalizeChatOption(value any) (chatOption, bool) {
	switch item := value.(type) {
	case string:
		item = strings.TrimSpace(item)
		if item == "" {
			return chatOption{}, false
		}
		return chatOption{Label: item, Value: item}, true
	case float64:
		text := strings.TrimSpace(strconv.FormatFloat(item, 'f', -1, 64))
		if text == "" {
			return chatOption{}, false
		}
		return chatOption{Label: text, Value: text}, true
	case bool:
		text := strings.TrimSpace(strconv.FormatBool(item))
		if text == "" {
			return chatOption{}, false
		}
		return chatOption{Label: text, Value: text}, true
	case map[string]any:
		value := normalizeChatOptionScalar(item["value"])
		label := normalizeChatOptionScalar(item["label"])
		if label == "" {
			label = value
		}
		if value == "" {
			value = label
		}
		if label == "" || value == "" {
			return chatOption{}, false
		}
		return chatOption{Label: label, Value: value}, true
	default:
		return chatOption{}, false
	}
}

func normalizeChatOptionScalar(value any) string {
	switch item := value.(type) {
	case string:
		return strings.TrimSpace(item)
	case float64:
		return strings.TrimSpace(strconv.FormatFloat(item, 'f', -1, 64))
	case bool:
		return strings.TrimSpace(strconv.FormatBool(item))
	default:
		return ""
	}
}

func unescapeChatOptionText(value string) string {
	text, err := strconv.Unquote(`"` + value + `"`)
	if err != nil {
		return value
	}
	return text
}
