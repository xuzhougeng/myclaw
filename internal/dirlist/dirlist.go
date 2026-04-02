package dirlist

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"baize/internal/toolcontract"
)

const (
	ToolName     = "list_directory"
	DefaultLimit = 50
	MaxLimit     = 200
)

type ToolInput struct {
	Path            string `json:"path,omitempty"`
	Limit           int    `json:"limit,omitempty"`
	IncludeHidden   bool   `json:"include_hidden,omitempty"`
	DirectoriesOnly bool   `json:"directories_only,omitempty"`
}

type ResultItem struct {
	Index      int    `json:"index"`
	Name       string `json:"name"`
	Path       string `json:"path"`
	IsDir      bool   `json:"is_dir"`
	SizeBytes  int64  `json:"size_bytes,omitempty"`
	ModifiedAt string `json:"modified_at,omitempty"`
}

type ToolResult struct {
	Tool      string       `json:"tool"`
	Path      string       `json:"path"`
	Limit     int          `json:"limit"`
	Count     int          `json:"count"`
	Truncated bool         `json:"truncated,omitempty"`
	Items     []ResultItem `json:"items"`
}

func Definition() toolcontract.Spec {
	return toolcontract.Spec{
		Name:              ToolName,
		Purpose:           "List files and folders in a local directory through native filesystem reads.",
		Description:       "Browse a local directory without invoking shell commands. Useful for answering questions like what is inside C:\\, ~/Downloads, or the current working directory.",
		InputContract:     `Provide {"path":"..."} and optionally {"limit":50,"include_hidden":false,"directories_only":false}. Empty path defaults to the current working directory.`,
		OutputContract:    "Returns JSON with the resolved directory path, effective limit, returned item count, truncation flag, and ordered items including name, path, type, size, and modified_at when available.",
		InputJSONExample:  `{"path":"C:\\","limit":20}`,
		OutputJSONExample: `{"tool":"list_directory","path":"C:\\","limit":20,"count":2,"items":[{"index":1,"name":"Program Files","path":"C:\\Program Files","is_dir":true},{"index":2,"name":"Users","path":"C:\\Users","is_dir":true}]}`,
		Usage:             UsageText(),
	}
}

func UsageText() string {
	return strings.TrimSpace(`
Tool: list_directory
Purpose: inspect the contents of a local directory without running shell commands.

Input:
- path: target directory path. Empty means the current working directory.
- limit: maximum number of items to return. Default is 50 and the hard cap is 200.
- include_hidden: when true, include dotfiles and platform-hidden entries.
- directories_only: when true, return folders only.

Output:
- tool: always list_directory.
- path: resolved absolute directory path.
- limit: the effective result limit.
- count: number of returned items.
- truncated: true when more matching entries existed than were returned.
- items: [{index, name, path, is_dir, size_bytes, modified_at}]

Notes:
- Entries are sorted with directories first, then by case-insensitive name.
- This tool is read-only and works across Windows, macOS, and Linux.
`)
}

func AllowedForInterface(name string) bool {
	return true
}

func NormalizeInput(raw ToolInput) ToolInput {
	path := strings.TrimSpace(raw.Path)
	if path == "" {
		path = "."
	}
	path = expandHome(path)
	path = filepath.Clean(path)

	limit := raw.Limit
	switch {
	case limit <= 0:
		limit = DefaultLimit
	case limit > MaxLimit:
		limit = MaxLimit
	}

	return ToolInput{
		Path:            path,
		Limit:           limit,
		IncludeHidden:   raw.IncludeHidden,
		DirectoriesOnly: raw.DirectoriesOnly,
	}
}

func Execute(raw ToolInput) (ToolResult, error) {
	input := NormalizeInput(raw)

	absPath, err := filepath.Abs(input.Path)
	if err != nil {
		return ToolResult{}, fmt.Errorf("resolve path %q: %w", input.Path, err)
	}
	info, err := os.Stat(absPath)
	if err != nil {
		return ToolResult{}, fmt.Errorf("stat path %q: %w", absPath, err)
	}
	if !info.IsDir() {
		return ToolResult{}, fmt.Errorf("path %q is not a directory", absPath)
	}

	entries, err := os.ReadDir(absPath)
	if err != nil {
		return ToolResult{}, fmt.Errorf("read directory %q: %w", absPath, err)
	}

	items := make([]ResultItem, 0, len(entries))
	for _, entry := range entries {
		entryPath := filepath.Join(absPath, entry.Name())
		if !input.IncludeHidden && isHiddenPath(entryPath, entry) {
			continue
		}
		if input.DirectoriesOnly && !entry.IsDir() {
			continue
		}

		item := ResultItem{
			Name:  entry.Name(),
			Path:  entryPath,
			IsDir: entry.IsDir(),
		}
		if info, err := entry.Info(); err == nil {
			if !item.IsDir {
				item.SizeBytes = info.Size()
			}
			if modTime := info.ModTime(); !modTime.IsZero() {
				item.ModifiedAt = modTime.Format(time.RFC3339)
			}
		}
		items = append(items, item)
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].IsDir != items[j].IsDir {
			return items[i].IsDir && !items[j].IsDir
		}
		left := strings.ToLower(items[i].Name)
		right := strings.ToLower(items[j].Name)
		if left == right {
			return items[i].Name < items[j].Name
		}
		return left < right
	})

	truncated := false
	if len(items) > input.Limit {
		items = items[:input.Limit]
		truncated = true
	}
	for i := range items {
		items[i].Index = i + 1
	}

	return ToolResult{
		Tool:      ToolName,
		Path:      absPath,
		Limit:     input.Limit,
		Count:     len(items),
		Truncated: truncated,
		Items:     items,
	}, nil
}

func FormatResult(result ToolResult) (string, error) {
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func expandHome(value string) string {
	if value == "" || value[0] != '~' {
		return value
	}
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return value
	}
	if value == "~" {
		return home
	}
	if value[1] == '/' || value[1] == '\\' {
		return filepath.Join(home, value[2:])
	}
	return value
}
