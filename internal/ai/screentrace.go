package ai

import (
	"context"
	"fmt"
	"strings"

	"baize/internal/modelconfig"
)

type ScreenAnalysis struct {
	SceneSummary   string   `json:"scene_summary"`
	VisibleText    []string `json:"visible_text"`
	Apps           []string `json:"apps"`
	TaskGuess      string   `json:"task_guess"`
	Keywords       []string `json:"keywords"`
	SensitiveLevel string   `json:"sensitive_level"`
	Confidence     float64  `json:"confidence"`
}

type ScreenDigestRecord struct {
	CapturedAt   string   `json:"captured_at"`
	SceneSummary string   `json:"scene_summary"`
	VisibleText  []string `json:"visible_text"`
	Apps         []string `json:"apps"`
	TaskGuess    string   `json:"task_guess"`
	Keywords     []string `json:"keywords"`
}

type ScreenDigestSummary struct {
	Summary       string   `json:"summary"`
	Keywords      []string `json:"keywords"`
	DominantApps  []string `json:"dominant_apps"`
	DominantTasks []string `json:"dominant_tasks"`
}

func (s *Service) AnalyzeScreenImage(ctx context.Context, cfg modelconfig.Config, fileName, imageURL string) (ScreenAnalysis, error) {
	cfg, err := normalizeExplicitConfig(cfg)
	if err != nil {
		return ScreenAnalysis{}, err
	}

	instructions := strings.TrimSpace(`
You are analyzing a desktop screenshot for a lightweight activity record system.
Return strict JSON only.
Prefer concise Chinese output.
Summarize only what is clearly visible or strongly implied by the UI.
Extract useful search keywords, visible text, likely applications, and the user's likely task.
If the screen appears sensitive, set sensitive_level to low, medium, or high.
Confidence must be between 0 and 1.
`)

	schema := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"scene_summary": map[string]any{"type": "string"},
			"visible_text": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
			"apps": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
			"task_guess":      map[string]any{"type": "string"},
			"keywords":        map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"sensitive_level": map[string]any{"type": "string", "enum": []string{"low", "medium", "high"}},
			"confidence":      map[string]any{"type": "number"},
		},
		"required": []string{"scene_summary", "visible_text", "apps", "task_guess", "keywords", "sensitive_level", "confidence"},
	}

	content := []responseContentInput{
		{
			Type: "input_text",
			Text: fmt.Sprintf("请分析这张桌面截图，文件名是 %s。输出适合时间线记录和后续检索的结构化结果。", strings.TrimSpace(fileName)),
		},
		{
			Type:     "input_image",
			ImageURL: strings.TrimSpace(imageURL),
			Detail:   "low",
		},
	}

	req := generationRequest{
		Instructions: mergeInstructionsWithSkillContext(ctx, instructions),
		Input: []responseInputMessage{
			{
				Role:    "user",
				Content: content,
			},
		},
		SchemaName:      "screen_analysis",
		Schema:          schema,
		MaxOutputTokens: 600,
	}

	var out ScreenAnalysis
	text, err := s.generate(ctx, cfg, req)
	if err != nil {
		return ScreenAnalysis{}, err
	}
	if err := decodeStructuredResponse(text, &out); err != nil {
		return ScreenAnalysis{}, fmt.Errorf("decode screen analysis: %w", err)
	}
	out.SceneSummary = strings.TrimSpace(out.SceneSummary)
	out.TaskGuess = strings.TrimSpace(out.TaskGuess)
	out.SensitiveLevel = strings.TrimSpace(strings.ToLower(out.SensitiveLevel))
	out.VisibleText = normalizeSearchQueries(out.VisibleText)
	out.Apps = normalizeSearchQueries(out.Apps)
	out.Keywords = normalizeSearchQueries(out.Keywords)
	if out.SensitiveLevel == "" {
		out.SensitiveLevel = "low"
	}
	if out.Confidence < 0 {
		out.Confidence = 0
	}
	if out.Confidence > 1 {
		out.Confidence = 1
	}
	return out, nil
}

func (s *Service) SummarizeScreenDigest(ctx context.Context, cfg modelconfig.Config, records []ScreenDigestRecord) (ScreenDigestSummary, error) {
	cfg, err := normalizeExplicitConfig(cfg)
	if err != nil {
		return ScreenDigestSummary{}, err
	}
	if len(records) == 0 {
		return ScreenDigestSummary{}, fmt.Errorf("empty screen digest records")
	}

	instructions := strings.TrimSpace(`
You are consolidating desktop activity records into a digest.
Return strict JSON only.
Prefer concise Chinese output that is easy to browse later.
Summarize the dominant work themes, applications, and visible tasks.
Do not mention model behavior.
`)

	schema := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"summary": map[string]any{"type": "string"},
			"keywords": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
			"dominant_apps": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
			"dominant_tasks": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
		},
		"required": []string{"summary", "keywords", "dominant_apps", "dominant_tasks"},
	}

	var builder strings.Builder
	builder.WriteString("以下是同一时间段内的桌面活动记录，请输出一个紧凑的摘要 JSON。\n\n")
	for index, item := range records {
		builder.WriteString(fmt.Sprintf("## 记录 %d\n", index+1))
		builder.WriteString("时间: ")
		builder.WriteString(strings.TrimSpace(item.CapturedAt))
		builder.WriteString("\n摘要: ")
		builder.WriteString(strings.TrimSpace(item.SceneSummary))
		if len(item.Apps) > 0 {
			builder.WriteString("\n应用: ")
			builder.WriteString(strings.Join(normalizeSearchQueries(item.Apps), " / "))
		}
		if strings.TrimSpace(item.TaskGuess) != "" {
			builder.WriteString("\n任务: ")
			builder.WriteString(strings.TrimSpace(item.TaskGuess))
		}
		if len(item.VisibleText) > 0 {
			builder.WriteString("\n可见文本: ")
			builder.WriteString(strings.Join(normalizeSearchQueries(item.VisibleText), " | "))
		}
		if len(item.Keywords) > 0 {
			builder.WriteString("\n关键词: ")
			builder.WriteString(strings.Join(normalizeSearchQueries(item.Keywords), " / "))
		}
		builder.WriteString("\n\n")
	}

	var out ScreenDigestSummary
	if err := s.generateJSON(ctx, cfg, instructions, builder.String(), "screen_digest_summary", schema, &out); err != nil {
		return ScreenDigestSummary{}, err
	}
	out.Summary = strings.TrimSpace(out.Summary)
	out.Keywords = normalizeSearchQueries(out.Keywords)
	out.DominantApps = normalizeSearchQueries(out.DominantApps)
	out.DominantTasks = normalizeSearchQueries(out.DominantTasks)
	return out, nil
}

func normalizeExplicitConfig(cfg modelconfig.Config) (modelconfig.Config, error) {
	cfg = cfg.Normalize()
	if missing := cfg.MissingFields(); len(missing) > 0 {
		return modelconfig.Config{}, fmt.Errorf("model is not configured, missing: %s", strings.Join(missing, ", "))
	}
	switch cfg.Provider {
	case modelconfig.ProviderOpenAI:
		if cfg.APIType != modelconfig.APITypeResponses && cfg.APIType != modelconfig.APITypeChatCompletions {
			return modelconfig.Config{}, fmt.Errorf("unsupported openai api type %q", cfg.APIType)
		}
	case modelconfig.ProviderAnthropic:
		if cfg.APIType != modelconfig.APITypeMessages {
			return modelconfig.Config{}, fmt.Errorf("unsupported anthropic api type %q", cfg.APIType)
		}
	default:
		return modelconfig.Config{}, fmt.Errorf("unsupported model provider %q", cfg.Provider)
	}
	return cfg, nil
}
