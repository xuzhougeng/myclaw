package main

import (
	"context"
	"fmt"
	goruntime "runtime"
	"sort"
	"strings"

	"baize/internal/ai"
	"baize/internal/dirlist"
	"baize/internal/filesearch"
)

type ToolItem struct {
	Name            string `json:"name"`
	ShortName       string `json:"shortName"`
	FamilyKey       string `json:"familyKey,omitempty"`
	FamilyTitle     string `json:"familyTitle,omitempty"`
	Title           string `json:"title"`
	Description     string `json:"description"`
	Purpose         string `json:"purpose"`
	Provider        string `json:"provider"`
	ProviderKind    string `json:"providerKind"`
	SideEffectLevel string `json:"sideEffectLevel"`
	DisplayOrder    int    `json:"displayOrder,omitempty"`
	Status          string `json:"status"`
	StatusTone      string `json:"statusTone"`
	Enabled         bool   `json:"enabled"`
	Toggleable      bool   `json:"toggleable"`
	Configurable    bool   `json:"configurable"`
	ConfigValue     string `json:"configValue,omitempty"`
}

func (a *DesktopApp) ListTools() ([]ToolItem, error) {
	if a.service == nil {
		return nil, fmt.Errorf("工具服务尚未启用")
	}

	ctx := context.Background()
	project, err := a.currentProject(ctx)
	if err != nil {
		return nil, err
	}

	definitions, err := a.service.ListAllAgentToolDefinitions(ctx, desktopMessageContext(project, ""))
	if err != nil {
		return nil, err
	}

	settings, err := a.GetSettings()
	if err != nil {
		return nil, err
	}

	items := make([]ToolItem, 0, len(definitions))
	for _, definition := range definitions {
		items = append(items, toToolItem(definition, settings))
	}

	sort.SliceStable(items, func(i, j int) bool {
		left := toolSortOrder(items[i])
		right := toolSortOrder(items[j])
		if left != right {
			return left < right
		}
		if items[i].FamilyTitle != items[j].FamilyTitle {
			return strings.ToLower(items[i].FamilyTitle) < strings.ToLower(items[j].FamilyTitle)
		}
		return strings.ToLower(items[i].Title) < strings.ToLower(items[j].Title)
	})

	return items, nil
}

func toToolItem(definition ai.AgentToolDefinition, settings AppSettings) ToolItem {
	shortName := toolShortName(definition.Name)
	description := strings.TrimSpace(definition.Description)
	purpose := strings.TrimSpace(definition.Purpose)
	if description == "" {
		description = purpose
	}
	enabled := toolEnabled(definition.Name, settings)
	status, tone := toolStatus(definition.Name, settings)

	item := ToolItem{
		Name:            strings.TrimSpace(definition.Name),
		ShortName:       shortName,
		FamilyKey:       strings.TrimSpace(definition.FamilyKey),
		FamilyTitle:     strings.TrimSpace(definition.FamilyTitle),
		Title:           toolTitle(definition),
		Description:     description,
		Purpose:         purpose,
		Provider:        strings.TrimSpace(definition.Provider),
		ProviderKind:    strings.TrimSpace(definition.ProviderKind),
		SideEffectLevel: strings.TrimSpace(definition.SideEffectLevel),
		DisplayOrder:    definition.DisplayOrder,
		Status:          status,
		StatusTone:      tone,
		Enabled:         enabled,
		Toggleable:      true,
	}
	if shortName == filesearch.ToolName {
		item.Configurable = true
		item.ConfigValue = strings.TrimSpace(settings.WeixinEverythingPath)
	}
	return item
}

func toolShortName(name string) string {
	trimmed := strings.TrimSpace(name)
	if prefix, short, ok := strings.Cut(trimmed, "::"); ok && strings.TrimSpace(prefix) != "" {
		return strings.TrimSpace(short)
	}
	return trimmed
}

func toolTitle(definition ai.AgentToolDefinition) string {
	if title := strings.TrimSpace(definition.DisplayTitle); title != "" {
		return title
	}
	switch strings.TrimSpace(toolShortName(definition.Name)) {
	case filesearch.ToolName:
		return "文件检索"
	case dirlist.ToolName:
		return "目录浏览"
	default:
		return strings.ReplaceAll(strings.TrimSpace(toolShortName(definition.Name)), "_", " ")
	}
}

func toolSortOrder(item ToolItem) int {
	if item.DisplayOrder > 0 {
		return item.DisplayOrder
	}
	switch strings.TrimSpace(item.ShortName) {
	case filesearch.ToolName:
		return 10
	case dirlist.ToolName:
		return 20
	default:
		return 999
	}
}

func toolStatus(name string, settings AppSettings) (string, string) {
	if !toolEnabled(name, settings) {
		return "已关闭", "off"
	}
	if toolShortName(name) != filesearch.ToolName {
		return "已就绪", "on"
	}
	if goruntime.GOOS != "windows" {
		return "当前平台暂不支持", "off"
	}
	if strings.TrimSpace(settings.WeixinEverythingPath) == "" {
		return "需配置 es.exe 路径", "pending"
	}
	return "已就绪", "on"
}

func toolEnabled(name string, settings AppSettings) bool {
	disabled := make(map[string]struct{}, len(settings.DisabledToolNames))
	for _, item := range settings.DisabledToolNames {
		disabled[strings.ToLower(strings.TrimSpace(item))] = struct{}{}
	}
	_, ok := disabled[strings.ToLower(strings.TrimSpace(name))]
	return !ok
}
