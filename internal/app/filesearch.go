package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"baize/internal/filesearch"
)

const fileSearchDisabledReply = "文件检索工具当前已关闭，请先在工具页启用。"

func (s *Service) SetFileSearchEverythingPath(path string) {
	s.settingsMu.Lock()
	s.fileSearchPath = strings.TrimSpace(path)
	s.settingsMu.Unlock()
}

func (s *Service) FileSearchEverythingPath() string {
	s.settingsMu.RLock()
	defer s.settingsMu.RUnlock()
	return s.fileSearchPath
}

func (s *Service) SetFileSearchExecutor(exec filesearch.SearchExecutor) {
	if exec == nil {
		exec = filesearch.ExecuteWithEverything
	}
	s.settingsMu.Lock()
	s.fileSearchExec = exec
	s.settingsMu.Unlock()
}

func (s *Service) fileSearchRuntime() (string, filesearch.SearchExecutor) {
	s.settingsMu.RLock()
	defer s.settingsMu.RUnlock()

	exec := s.fileSearchExec
	if exec == nil {
		exec = filesearch.ExecuteWithEverything
	}
	return s.fileSearchPath, exec
}

func (s *Service) tryHandleFileSearch(ctx context.Context, mc MessageContext, input string) (string, bool, error) {
	result, immediateReply, handled, err := s.ResolveFileSearch(ctx, mc, input)
	if err != nil || !handled {
		return "", handled, err
	}
	reply := strings.TrimSpace(immediateReply)
	if reply == "" {
		reply = filesearch.FormatSearchResult(result)
	}
	if reply != "" {
		setTurnSummary(ctx, summarizeFileSearchTurn(result, reply))
		s.maybeAppendConversationHistory(ctx, mc, input, reply)
	}
	return reply, true, nil
}

func (s *Service) ResolveFileSearch(ctx context.Context, _ MessageContext, input string) (filesearch.ToolResult, string, bool, error) {
	text := strings.TrimSpace(input)
	command := normalizeSlash(text)
	if strings.HasPrefix(strings.ToLower(command), filesearch.ShortcutName) {
		rawQuery := strings.TrimSpace(strings.TrimPrefix(command, filesearch.ShortcutName))
		switch {
		case rawQuery == "":
			return filesearch.ToolResult{}, filesearch.ShortcutUsageText(), true, nil
		case strings.EqualFold(rawQuery, "help") || rawQuery == "帮助":
			return filesearch.ToolResult{}, filesearch.CommandHelpText(), true, nil
		case !s.isFileSearchToolEnabled():
			addProcessTrace(ctx, "文件检索已禁用", "tool="+fileSearchFullToolName())
			return filesearch.ToolResult{}, fileSearchDisabledReply, true, nil
		}
		addProcessTrace(ctx, "显式文件搜索", "收到 `/find` 原生检索式，直接执行。\nquery="+rawQuery)
		result, reply, err := s.performFileSearch(ctx, filesearch.ToolInput{
			Query: rawQuery,
			Limit: filesearch.DefaultLimit,
		})
		if err == nil && reply == "" {
			addProcessTrace(ctx, "执行搜索", "query="+strings.TrimSpace(result.Query)+"\ncount="+fmt.Sprintf("%d", len(result.Items)))
		}
		return result, reply, true, err
	}

	return filesearch.ToolResult{}, "", false, nil
}

func (s *Service) executeFileSearch(ctx context.Context, input filesearch.ToolInput) (string, error) {
	result, reply, err := s.performFileSearch(ctx, input)
	if reply != "" || err != nil {
		return reply, err
	}
	return filesearch.FormatSearchResult(result), nil
}

func (s *Service) performFileSearch(ctx context.Context, input filesearch.ToolInput) (filesearch.ToolResult, string, error) {
	everythingPath, exec := s.fileSearchRuntime()
	input = filesearch.NormalizeInput(input)
	input.Limit = filesearch.DefaultLimit
	result, err := exec(ctx, everythingPath, input)
	if err != nil {
		switch {
		case errors.Is(err, filesearch.ErrUnsupported):
			return filesearch.ToolResult{}, err.Error(), nil
		case errors.Is(err, filesearch.ErrUnconfigured):
			return filesearch.ToolResult{}, err.Error(), nil
		default:
			return filesearch.ToolResult{}, "", err
		}
	}
	return result, "", nil
}

func fileSearchFullToolName() string {
	return joinProviderToolName(string(AgentToolProviderLocal), filesearch.ToolName)
}

func (s *Service) isFileSearchToolEnabled() bool {
	return s.isAgentToolEnabled(fileSearchFullToolName())
}

func summarizeFileSearchTurn(result filesearch.ToolResult, reply string) string {
	query := strings.TrimSpace(result.Query)
	if query == "" {
		return summarizeToolOutputForModel(reply)
	}
	if len(result.Items) == 0 {
		return fmt.Sprintf("文件检索已执行。query=%s count=0", query)
	}
	names := make([]string, 0, minInt(3, len(result.Items)))
	for _, item := range result.Items[:minInt(3, len(result.Items))] {
		names = append(names, strings.TrimSpace(item.Name))
	}
	return fmt.Sprintf("文件检索已执行。query=%s count=%d top=%s", query, len(result.Items), strings.Join(names, ", "))
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}
