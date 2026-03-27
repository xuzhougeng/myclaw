package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"time"

	"myclaw/internal/knowledge"
	"myclaw/internal/modelconfig"
)

type RouteDecision struct {
	Command      string `json:"command"`
	MemoryText   string `json:"memory_text"`
	AppendText   string `json:"append_text"`
	KnowledgeID  string `json:"knowledge_id"`
	ReminderSpec string `json:"reminder_spec"`
	ReminderID   string `json:"reminder_id"`
	Question     string `json:"question"`
}

type Service struct {
	configStore *modelconfig.Store
	httpClient  *http.Client
}

func NewService(configStore *modelconfig.Store) *Service {
	return &Service{
		configStore: configStore,
		httpClient: &http.Client{
			Timeout: 90 * time.Second,
		},
	}
}

func (s *Service) CurrentConfig(ctx context.Context) (modelconfig.Config, error) {
	return s.configStore.Load(ctx)
}

func (s *Service) IsConfigured(ctx context.Context) (bool, error) {
	cfg, err := s.CurrentConfig(ctx)
	if err != nil {
		return false, err
	}
	return len(cfg.MissingFields()) == 0, nil
}

func (s *Service) TestConnection(ctx context.Context) (string, error) {
	cfg, err := s.requireConfig(ctx)
	if err != nil {
		return "", err
	}
	return s.generateText(ctx, cfg, "You are a connectivity test endpoint.", "Reply with exactly OK.")
}

func (s *Service) RouteCommand(ctx context.Context, input string) (RouteDecision, error) {
	cfg, err := s.requireConfig(ctx)
	if err != nil {
		return RouteDecision{}, err
	}

	schema := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"command": map[string]any{
				"type": "string",
				"enum": []string{"remember", "append", "append_last", "forget", "notice_add", "notice_list", "notice_remove", "list", "stats", "help", "answer"},
			},
			"memory_text": map[string]any{
				"type": "string",
			},
			"append_text": map[string]any{
				"type": "string",
			},
			"knowledge_id": map[string]any{
				"type": "string",
			},
			"reminder_spec": map[string]any{
				"type": "string",
			},
			"reminder_id": map[string]any{
				"type": "string",
			},
			"question": map[string]any{
				"type": "string",
			},
		},
		"required": []string{"command", "memory_text", "append_text", "knowledge_id", "reminder_spec", "reminder_id", "question"},
	}

	instructions := strings.TrimSpace(`
You are the command router for myclaw.
Classify the user input into exactly one command:
- remember: save something into the knowledge base
- append: append a note to an existing knowledge item by ID or ID prefix
- append_last: append a note to the user's latest knowledge item in the current interface
- forget: delete one knowledge item by its ID or ID prefix
- notice_add: create a reminder
- notice_list: list reminders
- notice_remove: delete one reminder by ID or ID prefix
- list: list all knowledge
- stats: show knowledge stats
- help: show help
- answer: answer a question from the knowledge base

Rules:
- For remember, rewrite the memory as concise Markdown while preserving facts.
- For append, fill knowledge_id and append_text.
- For append_last, fill append_text.
- For forget, fill knowledge_id without the leading # when present.
- For notice_add, normalize reminder_spec into one of these executable forms:
  - <duration>后 <message>
  - 每天 HH:MM <message>
  - 明天 HH:MM <message>
  - YYYY-MM-DD HH:MM <message>
- For notice_remove, fill reminder_id without the leading # when present.
- For answer, put the cleaned question in question.
- Prefer commands over answer when the user is clearly asking to operate the system.
- Always fill unused text fields with an empty string.
- Respond only with JSON that matches the schema.
`)

	var decision RouteDecision
	if err := s.generateJSON(ctx, cfg, instructions, input, "route_decision", schema, &decision); err != nil {
		return RouteDecision{}, err
	}

	decision.Command = strings.TrimSpace(strings.ToLower(decision.Command))
	decision.MemoryText = strings.TrimSpace(decision.MemoryText)
	decision.AppendText = strings.TrimSpace(decision.AppendText)
	decision.KnowledgeID = strings.TrimSpace(strings.TrimPrefix(decision.KnowledgeID, "#"))
	decision.ReminderSpec = strings.TrimSpace(decision.ReminderSpec)
	decision.ReminderID = strings.TrimSpace(strings.TrimPrefix(decision.ReminderID, "#"))
	decision.Question = strings.TrimSpace(decision.Question)
	if decision.Command == "" {
		return RouteDecision{}, fmt.Errorf("model returned empty command")
	}
	return decision, nil
}

func (s *Service) Answer(ctx context.Context, question string, entries []knowledge.Entry) (string, error) {
	cfg, err := s.requireConfig(ctx)
	if err != nil {
		return "", err
	}

	var prompt strings.Builder
	prompt.WriteString("用户问题：\n")
	prompt.WriteString(strings.TrimSpace(question))
	prompt.WriteString("\n\n知识库内容：\n")
	if len(entries) == 0 {
		prompt.WriteString("(空)\n")
	} else {
		for index, entry := range entries {
			source := strings.TrimSpace(entry.Source)
			if source == "" {
				source = "unknown"
			}
			prompt.WriteString(fmt.Sprintf("%d. [%s] [%s] %s\n",
				index+1,
				entry.RecordedAt.Local().Format("2006-01-02 15:04:05"),
				source,
				entry.Text,
			))
		}
	}

	instructions := strings.TrimSpace(`
You are myclaw, a private knowledge-base assistant.
Answer in Chinese unless the user clearly asks otherwise.
Use only the provided knowledge base content.
If the knowledge base is insufficient, say so directly.
When helpful, cite the relevant memory item numbers.
Keep the answer concise but useful.
`)

	return s.generateText(ctx, cfg, instructions, prompt.String())
}

func (s *Service) TranslateToChinese(ctx context.Context, input string) (string, error) {
	cfg, err := s.requireConfig(ctx)
	if err != nil {
		return "", err
	}

	instructions := strings.TrimSpace(`
You are myclaw's translation mode.
Translate the user's input into natural, fluent Simplified Chinese.
Preserve factual meaning, tone, names, technical terms, formatting, and line breaks whenever possible.
Do not explain, do not summarize, and do not add commentary.
Return only the Chinese translation.
`)

	return s.generateText(ctx, cfg, instructions, strings.TrimSpace(input))
}

func (s *Service) requireConfig(ctx context.Context) (modelconfig.Config, error) {
	cfg, err := s.CurrentConfig(ctx)
	if err != nil {
		return modelconfig.Config{}, err
	}
	if cfg.Provider != "openai" {
		return modelconfig.Config{}, fmt.Errorf("unsupported model provider %q", cfg.Provider)
	}
	if missing := cfg.MissingFields(); len(missing) > 0 {
		return modelconfig.Config{}, fmt.Errorf("model is not configured, missing: %s", strings.Join(missing, ", "))
	}
	return cfg, nil
}

func (s *Service) generateJSON(ctx context.Context, cfg modelconfig.Config, instructions, input, schemaName string, schema map[string]any, out any) error {
	req := responsesRequest{
		Model:        cfg.Model,
		Instructions: instructions,
		Input: []responseInputMessage{
			newTextMessage("user", input),
		},
		Text: &responseTextOptions{
			Format: responseFormat{
				Type:        "json_schema",
				Name:        schemaName,
				Schema:      schema,
				Strict:      true,
				Description: "Structured response for myclaw",
			},
		},
		MaxOutputTokens: 800,
	}

	text, err := s.createResponse(ctx, cfg, req)
	if err != nil {
		return err
	}
	if err := json.Unmarshal([]byte(text), out); err != nil {
		return fmt.Errorf("decode structured response: %w", err)
	}
	return nil
}

func (s *Service) generateText(ctx context.Context, cfg modelconfig.Config, instructions, input string) (string, error) {
	req := responsesRequest{
		Model:        cfg.Model,
		Instructions: instructions,
		Input: []responseInputMessage{
			newTextMessage("user", input),
		},
		Text: &responseTextOptions{
			Format: responseFormat{
				Type: "text",
			},
			Verbosity: "low",
		},
		MaxOutputTokens: 1500,
	}
	return s.createResponse(ctx, cfg, req)
}

func (s *Service) createResponse(ctx context.Context, cfg modelconfig.Config, reqBody responsesRequest) (string, error) {
	data, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	endpoint := strings.TrimRight(cfg.BaseURL, "/") + path.Join("/", "responses")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		var apiErr openAIErrorResponse
		if err := json.Unmarshal(body, &apiErr); err == nil && strings.TrimSpace(apiErr.Error.Message) != "" {
			return "", fmt.Errorf("openai responses api returned %d: %s", resp.StatusCode, apiErr.Error.Message)
		}
		return "", fmt.Errorf("openai responses api returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var result responsesResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	text := strings.TrimSpace(result.OutputText())
	if text == "" {
		return "", fmt.Errorf("model returned empty output")
	}
	return text, nil
}

func newTextMessage(role, text string) responseInputMessage {
	return responseInputMessage{
		Role: role,
		Content: []responseContentInput{
			{
				Type: "input_text",
				Text: text,
			},
		},
	}
}

type responsesRequest struct {
	Model           string                 `json:"model"`
	Instructions    string                 `json:"instructions,omitempty"`
	Input           []responseInputMessage `json:"input"`
	Text            *responseTextOptions   `json:"text,omitempty"`
	MaxOutputTokens int                    `json:"max_output_tokens,omitempty"`
}

type responseInputMessage struct {
	Role    string                 `json:"role"`
	Content []responseContentInput `json:"content"`
}

type responseContentInput struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type responseTextOptions struct {
	Format    responseFormat `json:"format"`
	Verbosity string         `json:"verbosity,omitempty"`
}

type responseFormat struct {
	Type        string         `json:"type"`
	Name        string         `json:"name,omitempty"`
	Schema      map[string]any `json:"schema,omitempty"`
	Strict      bool           `json:"strict,omitempty"`
	Description string         `json:"description,omitempty"`
}

type responsesResponse struct {
	Output []responseOutputItem `json:"output"`
}

func (r responsesResponse) OutputText() string {
	var parts []string
	for _, item := range r.Output {
		for _, content := range item.Content {
			switch content.Type {
			case "output_text":
				if strings.TrimSpace(content.Text) != "" {
					parts = append(parts, strings.TrimSpace(content.Text))
				}
			case "refusal":
				if strings.TrimSpace(content.Refusal) != "" {
					parts = append(parts, strings.TrimSpace(content.Refusal))
				}
			}
		}
	}
	return strings.Join(parts, "\n")
}

type responseOutputItem struct {
	Type    string                  `json:"type"`
	Content []responseOutputContent `json:"content"`
}

type responseOutputContent struct {
	Type    string `json:"type"`
	Text    string `json:"text,omitempty"`
	Refusal string `json:"refusal,omitempty"`
}

type openAIErrorResponse struct {
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
}
