package app

import (
	"context"
	"errors"
	"strings"

	"myclaw/internal/ai"
	"myclaw/internal/filesearch"
)

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
	toolInput, immediateReply, handled, err := s.resolveFileSearchInput(ctx, mc, input)
	if err != nil || !handled {
		return "", handled, err
	}
	if immediateReply != "" {
		return immediateReply, true, nil
	}
	reply, err := s.executeFileSearch(ctx, toolInput)
	return reply, true, err
}

func (s *Service) resolveFileSearchInput(ctx context.Context, mc MessageContext, input string) (filesearch.ToolInput, string, bool, error) {
	text := strings.TrimSpace(input)
	command := normalizeSlash(text)
	if strings.HasPrefix(strings.ToLower(command), filesearch.ShortcutName) {
		rawQuery := strings.TrimSpace(strings.TrimPrefix(command, filesearch.ShortcutName))
		switch {
		case rawQuery == "":
			return filesearch.ToolInput{}, filesearch.ShortcutUsageText(), true, nil
		case strings.EqualFold(rawQuery, "help") || rawQuery == "帮助":
			return filesearch.ToolInput{}, filesearch.CommandHelpText(), true, nil
		case !filesearch.LooksLikeExplicitQuery(rawQuery):
			intent, ok, err := s.BuildFileSearchIntent(ctx, mc, rawQuery)
			if err != nil {
				return filesearch.ToolInput{}, "", true, err
			}
			if ok {
				return normalizeFileSearchToolInput(intent), "", true, nil
			}
		}
		return filesearch.ToolInput{
			Query: rawQuery,
			Limit: filesearch.DefaultLimit,
		}, "", true, nil
	}

	intent, ok, err := s.BuildFileSearchIntent(ctx, mc, text)
	if err != nil || !ok {
		return filesearch.ToolInput{}, "", ok, err
	}
	return normalizeFileSearchToolInput(intent), "", true, nil
}

func (s *Service) executeFileSearch(ctx context.Context, input filesearch.ToolInput) (string, error) {
	everythingPath, exec := s.fileSearchRuntime()
	result, err := exec(ctx, everythingPath, input)
	if err != nil {
		switch {
		case errors.Is(err, filesearch.ErrUnsupported):
			return err.Error(), nil
		case errors.Is(err, filesearch.ErrUnconfigured):
			return err.Error(), nil
		default:
			return "", err
		}
	}
	return filesearch.FormatSearchResult(result), nil
}

func normalizeFileSearchToolInput(intent ai.FileSearchIntent) filesearch.ToolInput {
	input := filesearch.NormalizeInput(intent.ToolInput)
	if strings.TrimSpace(input.Query) == "" && strings.TrimSpace(intent.Query) != "" {
		input.Query = strings.TrimSpace(intent.Query)
	}
	input.Limit = filesearch.DefaultLimit
	return input
}
