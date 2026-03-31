package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"myclaw/internal/ai"
	"myclaw/internal/filesearch"
)

const maxFileSearchPlanningRounds = 3

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
		s.maybeAppendConversationHistory(ctx, mc, input, reply)
	}
	return reply, true, nil
}

func (s *Service) ResolveFileSearch(ctx context.Context, mc MessageContext, input string) (filesearch.ToolResult, string, bool, error) {
	text := strings.TrimSpace(input)
	command := normalizeSlash(text)
	if strings.HasPrefix(strings.ToLower(command), filesearch.ShortcutName) {
		rawQuery := strings.TrimSpace(strings.TrimPrefix(command, filesearch.ShortcutName))
		switch {
		case rawQuery == "":
			return filesearch.ToolResult{}, filesearch.ShortcutUsageText(), true, nil
		case strings.EqualFold(rawQuery, "help") || rawQuery == "帮助":
			return filesearch.ToolResult{}, filesearch.CommandHelpText(), true, nil
		case !filesearch.LooksLikeExplicitQuery(rawQuery):
			result, reply, handled, err := s.planAndExecuteFileSearch(ctx, mc, rawQuery)
			if err != nil || handled {
				return result, reply, true, err
			}
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

	return s.planAndExecuteFileSearch(ctx, mc, text)
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

func (s *Service) planAndExecuteFileSearch(ctx context.Context, mc MessageContext, task string) (filesearch.ToolResult, string, bool, error) {
	if s.aiService == nil {
		return filesearch.ToolResult{}, "", false, nil
	}

	configured, err := s.aiService.IsConfigured(ctx)
	if err != nil {
		return filesearch.ToolResult{}, "", false, err
	}
	if !configured {
		return filesearch.ToolResult{}, "", false, nil
	}

	tool := fileSearchToolCapability()
	matches, err := s.aiService.DetectToolOpportunities(s.withSkillContext(ctx, mc), task, []ai.ToolCapability{tool})
	if err != nil {
		return filesearch.ToolResult{}, "", false, err
	}
	if !containsToolOpportunity(matches, tool.Name) {
		return filesearch.ToolResult{}, "", false, nil
	}
	addProcessTrace(ctx, "识别需求", "将当前输入识别为文件检索请求。")
	addProcessTrace(ctx, "匹配工具", "选中工具 `"+tool.Name+"`。")

	prior := make([]ai.ToolExecution, 0, maxFileSearchPlanningRounds)
	seenQueries := make(map[string]struct{})
	var lastResult filesearch.ToolResult

	for round := 0; round < maxFileSearchPlanningRounds; round++ {
		decision, err := s.aiService.PlanToolUse(s.withSkillContext(ctx, mc), task, tool, prior)
		if err != nil {
			return filesearch.ToolResult{}, "", true, err
		}

		switch decision.Action {
		case "stop":
			addProcessTrace(ctx, "停止规划", strings.TrimSpace(decision.UserMessage))
			if strings.TrimSpace(decision.UserMessage) != "" {
				return lastResult, decision.UserMessage, true, nil
			}
			if strings.TrimSpace(lastResult.Query) != "" || len(lastResult.Items) > 0 {
				return lastResult, "", true, nil
			}
			return filesearch.ToolResult{}, "没有找到匹配文件，请补充更具体的文件名、目录、扩展名或时间范围。", true, nil
		case "tool":
			if name := strings.TrimSpace(decision.ToolName); name != "" && !strings.EqualFold(name, tool.Name) {
				return filesearch.ToolResult{}, "", true, fmt.Errorf("tool planner selected unexpected tool %q", decision.ToolName)
			}
		default:
			return filesearch.ToolResult{}, "", true, fmt.Errorf("unsupported file search tool action %q", decision.Action)
		}

		toolInput, err := decodeFileSearchToolInput(decision.ToolInput)
		if err != nil {
			return filesearch.ToolResult{}, "", true, err
		}
		compiledQuery := strings.TrimSpace(filesearch.CompileQuery(toolInput))
		addProcessTrace(ctx, fmt.Sprintf("规划调用 %d", round+1), "tool="+tool.Name+"\nquery="+compiledQuery+"\ninput="+mustMarshalJSON(toolInput))
		queryKey := strings.ToLower(compiledQuery)
		if queryKey == "" {
			addProcessTrace(ctx, fmt.Sprintf("规划调用 %d", round+1), "当前规划未形成有效检索式。")
			return filesearch.ToolResult{}, "我还不能形成有效的文件检索条件，请直接说文件名、目录、扩展名或磁盘范围。", true, nil
		}
		if _, ok := seenQueries[queryKey]; ok {
			addProcessTrace(ctx, fmt.Sprintf("规划调用 %d", round+1), "检测到重复检索式，停止重复执行。")
			if strings.TrimSpace(lastResult.Query) != "" || len(lastResult.Items) > 0 {
				return lastResult, "", true, nil
			}
			return filesearch.ToolResult{}, "我已经尝试过等价的检索条件了，请补充更具体的文件名、目录、扩展名或时间范围。", true, nil
		}
		seenQueries[queryKey] = struct{}{}

		result, reply, err := s.performFileSearch(ctx, toolInput)
		if err != nil {
			return filesearch.ToolResult{}, "", true, err
		}
		if reply != "" {
			addProcessTrace(ctx, fmt.Sprintf("执行搜索 %d", round+1), reply)
			return filesearch.ToolResult{}, reply, true, nil
		}
		lastResult = result
		addProcessTrace(ctx, fmt.Sprintf("执行搜索 %d", round+1), "query="+strings.TrimSpace(result.Query)+"\ncount="+fmt.Sprintf("%d", len(result.Items)))
		if len(result.Items) > 0 {
			return result, "", true, nil
		}

		prior = append(prior, ai.ToolExecution{
			ToolName:   tool.Name,
			ToolInput:  mustMarshalJSON(toolInput),
			ToolOutput: mustMarshalJSON(result),
		})
	}

	if strings.TrimSpace(lastResult.Query) != "" || len(lastResult.Items) > 0 {
		return lastResult, "", true, nil
	}
	return filesearch.ToolResult{}, "没有找到匹配文件，请补充更具体的文件名、目录、扩展名或时间范围。", true, nil
}

func fileSearchToolCapability() ai.ToolCapability {
	spec := filesearch.Definition()
	return ai.ToolCapability{
		Name:             spec.Name,
		Description:      spec.Description,
		Usage:            spec.Usage,
		InputJSONExample: spec.InputJSONExample,
	}
}

func containsToolOpportunity(matches []ai.ToolOpportunity, toolName string) bool {
	toolName = strings.TrimSpace(toolName)
	for _, match := range matches {
		if strings.EqualFold(strings.TrimSpace(match.ToolName), toolName) {
			return true
		}
	}
	return false
}

func decodeFileSearchToolInput(raw string) (filesearch.ToolInput, error) {
	var input filesearch.ToolInput
	if err := decodeAgentToolInput(raw, &input); err != nil {
		return filesearch.ToolInput{}, fmt.Errorf("decode file search tool input: %w", err)
	}
	input = filesearch.NormalizeInput(input)
	input.Limit = filesearch.DefaultLimit
	return input, nil
}

func mustMarshalJSON(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(data)
}
