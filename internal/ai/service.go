package ai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"reflect"
	"strings"
	"time"

	"baize/internal/modelconfig"
)

type Service struct {
	configStore *modelconfig.Store
	httpClient  *http.Client
}

func NewService(configStore *modelconfig.Store) *Service {
	return &Service{
		configStore: configStore,
		httpClient: &http.Client{
			Timeout: time.Duration(modelconfig.DefaultRequestTimeoutSeconds) * time.Second,
		},
	}
}

func (s *Service) httpClientForConfig(cfg modelconfig.Config) *http.Client {
	if s.httpClient == nil {
		return &http.Client{
			Timeout: configuredRequestTimeout(cfg),
		}
	}
	client := *s.httpClient
	client.Timeout = configuredRequestTimeout(cfg)
	return &client
}

func configuredRequestTimeout(cfg modelconfig.Config) time.Duration {
	seconds := modelconfig.DefaultRequestTimeoutSeconds
	if cfg.RequestTimeoutSeconds != nil && *cfg.RequestTimeoutSeconds > 0 {
		seconds = *cfg.RequestTimeoutSeconds
	}
	return time.Duration(seconds) * time.Second
}

func (s *Service) doRequest(cfg modelconfig.Config, req *http.Request) (*http.Response, error) {
	return s.httpClientForConfig(cfg).Do(req)
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
	return s.TestConfig(ctx, cfg)
}

func (s *Service) TestConfig(ctx context.Context, cfg modelconfig.Config) (string, error) {
	cfg = cfg.Normalize()
	if missing := cfg.MissingFields(); len(missing) > 0 {
		return "", fmt.Errorf("model is not configured, missing: %s", strings.Join(missing, ", "))
	}
	switch cfg.Provider {
	case modelconfig.ProviderOpenAI:
		if cfg.APIType != modelconfig.APITypeResponses && cfg.APIType != modelconfig.APITypeChatCompletions {
			return "", fmt.Errorf("unsupported openai api type %q", cfg.APIType)
		}
	case modelconfig.ProviderAnthropic:
		if cfg.APIType != modelconfig.APITypeMessages {
			return "", fmt.Errorf("unsupported anthropic api type %q", cfg.APIType)
		}
	default:
		return "", fmt.Errorf("unsupported model provider %q", cfg.Provider)
	}
	return s.generateText(ctx, cfg, "You are a connectivity test endpoint.", "Reply with exactly OK.")
}

func (s *Service) requireConfig(ctx context.Context) (modelconfig.Config, error) {
	cfg, err := s.CurrentConfig(ctx)
	if err != nil {
		return modelconfig.Config{}, err
	}
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

type generationRequest struct {
	Instructions     string
	Input            []responseInputMessage
	SchemaName       string
	Schema           map[string]any
	MaxOutputTokens  int
	Temperature      *float64
	TopP             *float64
	FrequencyPenalty *float64
	PresencePenalty  *float64
}

func (r generationRequest) WantsJSON() bool {
	return strings.TrimSpace(r.SchemaName) != "" && len(r.Schema) > 0
}

func (s *Service) generateJSON(ctx context.Context, cfg modelconfig.Config, instructions, input, schemaName string, schema map[string]any, out any) error {
	req := generationRequest{
		Instructions: mergeInstructionsWithSkillContext(ctx, instructions),
		Input: []responseInputMessage{
			newTextMessage("user", input),
		},
		SchemaName:      schemaName,
		Schema:          schema,
		MaxOutputTokens: 800,
	}

	text, err := s.generate(ctx, cfg, req)
	if err != nil {
		return err
	}
	if err := decodeStructuredResponse(text, out); err != nil {
		return fmt.Errorf("decode structured response: %w", err)
	}
	return nil
}

func (s *Service) generateText(ctx context.Context, cfg modelconfig.Config, instructions, input string) (string, error) {
	return s.generateTextFromContent(ctx, cfg, instructions, []responseContentInput{
		{
			Type: "input_text",
			Text: input,
		},
	})
}

func (s *Service) generateTextFromContent(ctx context.Context, cfg modelconfig.Config, instructions string, content []responseContentInput) (string, error) {
	req := generationRequest{
		Instructions: mergeInstructionsWithSkillContext(ctx, instructions),
		Input: []responseInputMessage{
			{
				Role:    "user",
				Content: content,
			},
		},
		MaxOutputTokens: 1500,
	}
	return s.generate(ctx, cfg, req)
}

func (s *Service) generateTextStream(ctx context.Context, cfg modelconfig.Config, instructions, input string, onDelta func(string)) (string, error) {
	return s.generateTextFromContentStream(ctx, cfg, instructions, []responseContentInput{
		{
			Type: "input_text",
			Text: input,
		},
	}, onDelta)
}

func (s *Service) generateTextFromContentStream(ctx context.Context, cfg modelconfig.Config, instructions string, content []responseContentInput, onDelta func(string)) (string, error) {
	req := generationRequest{
		Instructions: mergeInstructionsWithSkillContext(ctx, instructions),
		Input: []responseInputMessage{
			{
				Role:    "user",
				Content: content,
			},
		},
		MaxOutputTokens: 1500,
	}
	return s.generateStream(ctx, cfg, req, onDelta)
}

func (s *Service) generate(ctx context.Context, cfg modelconfig.Config, req generationRequest) (string, error) {
	req = applyConfigToGenerationRequest(cfg, req)
	switch cfg.Provider {
	case modelconfig.ProviderOpenAI:
		switch cfg.APIType {
		case modelconfig.APITypeResponses:
			return s.createOpenAIResponse(ctx, cfg, req)
		case modelconfig.APITypeChatCompletions:
			return s.createOpenAIChatCompletion(ctx, cfg, req)
		}
	case modelconfig.ProviderAnthropic:
		if cfg.APIType == modelconfig.APITypeMessages {
			return s.createAnthropicMessage(ctx, cfg, req)
		}
	}
	return "", fmt.Errorf("unsupported provider/api combination %q/%q", cfg.Provider, cfg.APIType)
}

func (s *Service) generateStream(ctx context.Context, cfg modelconfig.Config, req generationRequest, onDelta func(string)) (string, error) {
	if req.WantsJSON() {
		return s.generate(ctx, cfg, req)
	}
	req = applyConfigToGenerationRequest(cfg, req)
	switch cfg.Provider {
	case modelconfig.ProviderOpenAI:
		switch cfg.APIType {
		case modelconfig.APITypeResponses:
			return s.createOpenAIResponseStream(ctx, cfg, req, onDelta)
		case modelconfig.APITypeChatCompletions:
			return s.createOpenAIChatCompletionStream(ctx, cfg, req, onDelta)
		}
	case modelconfig.ProviderAnthropic:
		if cfg.APIType == modelconfig.APITypeMessages {
			return s.createAnthropicMessageStream(ctx, cfg, req, onDelta)
		}
	}
	return s.generate(ctx, cfg, req)
}

func applyConfigToGenerationRequest(cfg modelconfig.Config, req generationRequest) generationRequest {
	if override := configuredMaxOutputTokens(cfg, req.WantsJSON()); override != nil && *override > 0 {
		req.MaxOutputTokens = *override
	}
	if cfg.Temperature != nil && req.Temperature == nil {
		req.Temperature = cfg.Temperature
	}
	if cfg.TopP != nil && req.TopP == nil {
		req.TopP = cfg.TopP
	}
	if cfg.FrequencyPenalty != nil && req.FrequencyPenalty == nil {
		req.FrequencyPenalty = cfg.FrequencyPenalty
	}
	if cfg.PresencePenalty != nil && req.PresencePenalty == nil {
		req.PresencePenalty = cfg.PresencePenalty
	}
	return req
}

func configuredMaxOutputTokens(cfg modelconfig.Config, wantsJSON bool) *int {
	if wantsJSON {
		if cfg.MaxOutputTokensJSON != nil {
			return cfg.MaxOutputTokensJSON
		}
	} else {
		if cfg.MaxOutputTokensText != nil {
			return cfg.MaxOutputTokensText
		}
	}
	return cfg.MaxOutputTokens
}

func (s *Service) createOpenAIResponse(ctx context.Context, cfg modelconfig.Config, req generationRequest) (string, error) {
	reqBody := responsesRequest{
		Model:           cfg.Model,
		Instructions:    req.Instructions,
		Input:           req.Input,
		MaxOutputTokens: req.MaxOutputTokens,
		Temperature:     req.Temperature,
		TopP:            req.TopP,
	}
	if req.WantsJSON() {
		reqBody.Text = &responseTextOptions{
			Format: responseFormat{
				Type:        "json_schema",
				Name:        req.SchemaName,
				Schema:      req.Schema,
				Strict:      true,
				Description: "Structured response for baize",
			},
		}
	} else {
		reqBody.Text = &responseTextOptions{
			Format: responseFormat{
				Type: "text",
			},
			Verbosity: "low",
		}
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	endpoint := strings.TrimRight(cfg.BaseURL, "/") + path.Join("/", "responses")
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := s.doRequest(cfg, httpReq)
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
	AddUsage(ctx, result.Usage.TokenUsage())
	text := strings.TrimSpace(result.OutputText())
	if text == "" {
		return "", fmt.Errorf("model returned empty output")
	}
	return text, nil
}

func (s *Service) createOpenAIChatCompletion(ctx context.Context, cfg modelconfig.Config, req generationRequest) (string, error) {
	reqBody := chatCompletionsRequest{
		Model:            cfg.Model,
		Messages:         buildChatCompletionMessages(req),
		MaxTokens:        req.MaxOutputTokens,
		Temperature:      req.Temperature,
		TopP:             req.TopP,
		FrequencyPenalty: req.FrequencyPenalty,
		PresencePenalty:  req.PresencePenalty,
	}
	if req.WantsJSON() {
		reqBody.ResponseFormat = &chatCompletionsResponseFormat{
			Type: "json_schema",
			JSONSchema: &chatCompletionsJSONSchema{
				Name:   req.SchemaName,
				Schema: req.Schema,
				Strict: true,
			},
		}
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	endpoint := strings.TrimRight(cfg.BaseURL, "/") + path.Join("/", "chat", "completions")
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := s.doRequest(cfg, httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		var apiErr openAIErrorResponse
		if err := json.Unmarshal(body, &apiErr); err == nil && strings.TrimSpace(apiErr.Error.Message) != "" {
			return "", fmt.Errorf("openai chat completions api returned %d: %s", resp.StatusCode, apiErr.Error.Message)
		}
		return "", fmt.Errorf("openai chat completions api returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var result chatCompletionsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	AddUsage(ctx, result.Usage.TokenUsage())
	text := strings.TrimSpace(result.OutputText())
	if text == "" {
		return "", fmt.Errorf("model returned empty output")
	}
	return text, nil
}

func (s *Service) createOpenAIResponseStream(ctx context.Context, cfg modelconfig.Config, req generationRequest, onDelta func(string)) (string, error) {
	reqBody := responsesRequest{
		Model:           cfg.Model,
		Instructions:    req.Instructions,
		Input:           req.Input,
		MaxOutputTokens: req.MaxOutputTokens,
		Temperature:     req.Temperature,
		TopP:            req.TopP,
		Stream:          true,
	}
	reqBody.Text = &responseTextOptions{
		Format: responseFormat{
			Type: "text",
		},
		Verbosity: "low",
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	endpoint := strings.TrimRight(cfg.BaseURL, "/") + path.Join("/", "responses")
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := s.doRequest(cfg, httpReq)
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

	var builder strings.Builder
	var completedText string
	var usage TokenUsage
	err = consumeServerSentEvents(resp.Body, func(_ string, data []byte) error {
		payload := bytes.TrimSpace(data)
		if len(payload) == 0 || bytes.Equal(payload, []byte("[DONE]")) {
			return nil
		}

		var event struct {
			Type     string            `json:"type"`
			Delta    string            `json:"delta"`
			Response responsesResponse `json:"response"`
		}
		if err := json.Unmarshal(payload, &event); err != nil {
			return err
		}
		switch event.Type {
		case "response.output_text.delta", "response.refusal.delta":
			if event.Delta != "" {
				builder.WriteString(event.Delta)
				if onDelta != nil {
					onDelta(event.Delta)
				}
			}
		case "response.completed":
			completedText = strings.TrimSpace(event.Response.OutputText())
			usage = event.Response.Usage.TokenUsage()
		case "error":
			return parseStreamError(payload)
		}
		return nil
	})
	if err != nil {
		return "", err
	}

	text := strings.TrimSpace(builder.String())
	if text == "" {
		text = completedText
	}
	if text == "" {
		return "", fmt.Errorf("model returned empty output")
	}
	AddUsage(ctx, usage)
	return text, nil
}

func (s *Service) createOpenAIChatCompletionStream(ctx context.Context, cfg modelconfig.Config, req generationRequest, onDelta func(string)) (string, error) {
	reqBody := chatCompletionsRequest{
		Model:            cfg.Model,
		Messages:         buildChatCompletionMessages(req),
		MaxTokens:        req.MaxOutputTokens,
		Temperature:      req.Temperature,
		TopP:             req.TopP,
		FrequencyPenalty: req.FrequencyPenalty,
		PresencePenalty:  req.PresencePenalty,
		Stream:           true,
		StreamOptions: &chatCompletionsStreamOptions{
			IncludeUsage: true,
		},
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	endpoint := strings.TrimRight(cfg.BaseURL, "/") + path.Join("/", "chat", "completions")
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := s.doRequest(cfg, httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		var apiErr openAIErrorResponse
		if err := json.Unmarshal(body, &apiErr); err == nil && strings.TrimSpace(apiErr.Error.Message) != "" {
			return "", fmt.Errorf("openai chat completions api returned %d: %s", resp.StatusCode, apiErr.Error.Message)
		}
		return "", fmt.Errorf("openai chat completions api returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var builder strings.Builder
	var usage TokenUsage
	err = consumeServerSentEvents(resp.Body, func(_ string, data []byte) error {
		payload := bytes.TrimSpace(data)
		if len(payload) == 0 || bytes.Equal(payload, []byte("[DONE]")) {
			return nil
		}

		var chunk struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
					Refusal string `json:"refusal"`
				} `json:"delta"`
			} `json:"choices"`
			Usage *chatCompletionsUsage `json:"usage,omitempty"`
			Error *struct {
				Message string `json:"message"`
			} `json:"error,omitempty"`
		}
		if err := json.Unmarshal(payload, &chunk); err != nil {
			return err
		}
		if chunk.Error != nil && strings.TrimSpace(chunk.Error.Message) != "" {
			return fmt.Errorf("openai chat completions api stream error: %s", strings.TrimSpace(chunk.Error.Message))
		}
		if chunk.Usage != nil {
			usage = chunk.Usage.TokenUsage()
		}
		for _, choice := range chunk.Choices {
			for _, delta := range []string{choice.Delta.Content, choice.Delta.Refusal} {
				if delta == "" {
					continue
				}
				builder.WriteString(delta)
				if onDelta != nil {
					onDelta(delta)
				}
			}
		}
		return nil
	})
	if err != nil {
		return "", err
	}

	text := strings.TrimSpace(builder.String())
	if text == "" {
		return "", fmt.Errorf("model returned empty output")
	}
	AddUsage(ctx, usage)
	return text, nil
}

func (s *Service) createAnthropicMessage(ctx context.Context, cfg modelconfig.Config, req generationRequest) (string, error) {
	reqBody, err := buildAnthropicRequest(cfg.Model, req)
	if err != nil {
		return "", err
	}

	result, err := s.doAnthropicMessage(ctx, cfg, reqBody)
	if err != nil && req.WantsJSON() && len(reqBody.Tools) > 0 && shouldRetryAnthropicLegacyJSON(err) {
		legacyReqBody, legacyErr := buildAnthropicLegacyRequest(cfg.Model, req)
		if legacyErr != nil {
			return "", legacyErr
		}
		legacyResult, fallbackErr := s.doAnthropicMessage(ctx, cfg, legacyReqBody)
		if fallbackErr != nil {
			return "", fmt.Errorf("%v; legacy structured retry failed: %w", err, fallbackErr)
		}
		result = legacyResult
		err = nil
	}
	if err != nil {
		return "", err
	}
	AddUsage(ctx, result.Usage.TokenUsage())
	if req.WantsJSON() {
		text, ok, err := result.StructuredOutput()
		if err != nil {
			return "", err
		}
		if ok {
			return text, nil
		}
	}
	text := strings.TrimSpace(result.OutputText())
	if text == "" {
		return "", fmt.Errorf("model returned empty output")
	}
	return text, nil
}

func (s *Service) createAnthropicMessageStream(ctx context.Context, cfg modelconfig.Config, req generationRequest, onDelta func(string)) (string, error) {
	reqBody, err := buildAnthropicRequest(cfg.Model, req)
	if err != nil {
		return "", err
	}
	reqBody.Stream = true

	data, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	endpoint := strings.TrimRight(cfg.BaseURL, "/") + path.Join("/", "messages")
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-Api-Key", cfg.APIKey)
	httpReq.Header.Set("Anthropic-Version", "2023-06-01")

	resp, err := s.doRequest(cfg, httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		var apiErr anthropicErrorResponse
		if err := json.Unmarshal(body, &apiErr); err == nil && strings.TrimSpace(apiErr.Error.Message) != "" {
			return "", &anthropicAPIError{
				StatusCode: resp.StatusCode,
				Message:    strings.TrimSpace(apiErr.Error.Message),
			}
		}
		return "", &anthropicAPIError{
			StatusCode: resp.StatusCode,
			Message:    strings.TrimSpace(string(body)),
		}
	}

	var builder strings.Builder
	var usage anthropicUsage
	err = consumeServerSentEvents(resp.Body, func(_ string, data []byte) error {
		payload := bytes.TrimSpace(data)
		if len(payload) == 0 {
			return nil
		}

		var event struct {
			Type    string `json:"type"`
			Message struct {
				Usage anthropicUsageEvent `json:"usage"`
			} `json:"message"`
			Usage anthropicUsageEvent `json:"usage"`
			Delta struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"delta"`
		}
		if err := json.Unmarshal(payload, &event); err != nil {
			return err
		}
		switch event.Type {
		case "message_start":
			event.Message.Usage.ApplyTo(&usage)
		case "message_delta":
			event.Usage.ApplyTo(&usage)
		case "content_block_delta":
			if event.Delta.Type == "text_delta" && event.Delta.Text != "" {
				builder.WriteString(event.Delta.Text)
				if onDelta != nil {
					onDelta(event.Delta.Text)
				}
			}
		case "error":
			return parseStreamError(payload)
		}
		return nil
	})
	if err != nil {
		return "", err
	}

	text := strings.TrimSpace(builder.String())
	if text == "" {
		return "", fmt.Errorf("model returned empty output")
	}
	AddUsage(ctx, usage.TokenUsage())
	return text, nil
}

func (s *Service) doAnthropicMessage(ctx context.Context, cfg modelconfig.Config, reqBody anthropicMessagesRequest) (anthropicMessagesResponse, error) {
	data, err := json.Marshal(reqBody)
	if err != nil {
		return anthropicMessagesResponse{}, err
	}

	endpoint := strings.TrimRight(cfg.BaseURL, "/") + path.Join("/", "messages")
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return anthropicMessagesResponse{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-Api-Key", cfg.APIKey)
	httpReq.Header.Set("Anthropic-Version", "2023-06-01")

	resp, err := s.doRequest(cfg, httpReq)
	if err != nil {
		return anthropicMessagesResponse{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		var apiErr anthropicErrorResponse
		if err := json.Unmarshal(body, &apiErr); err == nil && strings.TrimSpace(apiErr.Error.Message) != "" {
			return anthropicMessagesResponse{}, &anthropicAPIError{
				StatusCode: resp.StatusCode,
				Message:    strings.TrimSpace(apiErr.Error.Message),
			}
		}
		return anthropicMessagesResponse{}, &anthropicAPIError{
			StatusCode: resp.StatusCode,
			Message:    strings.TrimSpace(string(body)),
		}
	}

	var result anthropicMessagesResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return anthropicMessagesResponse{}, err
	}
	return result, nil
}

func shouldRetryAnthropicLegacyJSON(err error) bool {
	var apiErr *anthropicAPIError
	if !errors.As(err, &apiErr) {
		return false
	}
	return apiErr.StatusCode == http.StatusBadRequest || apiErr.StatusCode == http.StatusUnprocessableEntity
}

func newTextMessage(role, text string) responseInputMessage {
	return responseInputMessage{
		Role: role,
		Content: []responseContentInput{
			{
				Type: responseTextContentType(role),
				Text: text,
			},
		},
	}
}

func responseTextContentType(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "assistant":
		return "output_text"
	default:
		return "input_text"
	}
}

type responsesRequest struct {
	Model           string                 `json:"model"`
	Instructions    string                 `json:"instructions,omitempty"`
	Input           []responseInputMessage `json:"input"`
	Text            *responseTextOptions   `json:"text,omitempty"`
	MaxOutputTokens int                    `json:"max_output_tokens,omitempty"`
	Temperature     *float64               `json:"temperature,omitempty"`
	TopP            *float64               `json:"top_p,omitempty"`
	Stream          bool                   `json:"stream,omitempty"`
}

type responseInputMessage struct {
	Role    string                 `json:"role"`
	Content []responseContentInput `json:"content"`
}

type responseContentInput struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	ImageURL string `json:"image_url,omitempty"`
	FileData string `json:"file_data,omitempty"`
	Filename string `json:"filename,omitempty"`
	Detail   string `json:"detail,omitempty"`
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
	Usage  responsesUsage       `json:"usage"`
}

type responsesUsage struct {
	InputTokens        int `json:"input_tokens"`
	InputTokensDetails struct {
		CachedTokens int `json:"cached_tokens"`
	} `json:"input_tokens_details"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

func (u responsesUsage) TokenUsage() TokenUsage {
	return TokenUsage{
		InputTokens:  u.InputTokens,
		OutputTokens: u.OutputTokens,
		CachedTokens: u.InputTokensDetails.CachedTokens,
		TotalTokens:  u.TotalTokens,
	}
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

type chatCompletionsRequest struct {
	Model            string                         `json:"model"`
	Messages         []chatCompletionsMessage       `json:"messages"`
	ResponseFormat   *chatCompletionsResponseFormat `json:"response_format,omitempty"`
	MaxTokens        int                            `json:"max_tokens,omitempty"`
	Temperature      *float64                       `json:"temperature,omitempty"`
	TopP             *float64                       `json:"top_p,omitempty"`
	FrequencyPenalty *float64                       `json:"frequency_penalty,omitempty"`
	PresencePenalty  *float64                       `json:"presence_penalty,omitempty"`
	Stream           bool                           `json:"stream,omitempty"`
	StreamOptions    *chatCompletionsStreamOptions  `json:"stream_options,omitempty"`
}

type chatCompletionsMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type chatCompletionsContentPart struct {
	Type     string                       `json:"type"`
	Text     string                       `json:"text,omitempty"`
	ImageURL *chatCompletionsImageURLPart `json:"image_url,omitempty"`
}

type chatCompletionsImageURLPart struct {
	URL    string `json:"url"`
	Detail string `json:"detail,omitempty"`
}

type chatCompletionsResponseFormat struct {
	Type       string                     `json:"type"`
	JSONSchema *chatCompletionsJSONSchema `json:"json_schema,omitempty"`
}

type chatCompletionsStreamOptions struct {
	IncludeUsage bool `json:"include_usage,omitempty"`
}

type chatCompletionsJSONSchema struct {
	Name   string         `json:"name"`
	Schema map[string]any `json:"schema"`
	Strict bool           `json:"strict,omitempty"`
}

type chatCompletionsResponse struct {
	Choices []chatCompletionsChoice `json:"choices"`
	Usage   chatCompletionsUsage    `json:"usage"`
}

type chatCompletionsUsage struct {
	PromptTokens        int `json:"prompt_tokens"`
	PromptTokensDetails struct {
		CachedTokens int `json:"cached_tokens"`
	} `json:"prompt_tokens_details"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

func (u chatCompletionsUsage) TokenUsage() TokenUsage {
	return TokenUsage{
		InputTokens:  u.PromptTokens,
		OutputTokens: u.CompletionTokens,
		CachedTokens: u.PromptTokensDetails.CachedTokens,
		TotalTokens:  u.TotalTokens,
	}
}

type chatCompletionsChoice struct {
	Message chatCompletionsOutputMessage `json:"message"`
}

type chatCompletionsOutputMessage struct {
	Content string `json:"content"`
}

func (r chatCompletionsResponse) OutputText() string {
	if len(r.Choices) == 0 {
		return ""
	}
	return strings.TrimSpace(r.Choices[0].Message.Content)
}

type anthropicMessagesRequest struct {
	Model       string                    `json:"model"`
	System      string                    `json:"system,omitempty"`
	Messages    []anthropicMessageRequest `json:"messages"`
	MaxTokens   int                       `json:"max_tokens"`
	Tools       []anthropicToolDefinition `json:"tools,omitempty"`
	ToolChoice  *anthropicToolChoice      `json:"tool_choice,omitempty"`
	Temperature *float64                  `json:"temperature,omitempty"`
	TopP        *float64                  `json:"top_p,omitempty"`
	Stream      bool                      `json:"stream,omitempty"`
}

type anthropicMessageRequest struct {
	Role    string                 `json:"role"`
	Content []anthropicContentPart `json:"content"`
}

type anthropicContentPart struct {
	Type   string                `json:"type"`
	Text   string                `json:"text,omitempty"`
	Source *anthropicImageSource `json:"source,omitempty"`
}

type anthropicImageSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
}

type anthropicToolDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	InputSchema map[string]any `json:"input_schema"`
}

type anthropicToolChoice struct {
	Type string `json:"type"`
	Name string `json:"name,omitempty"`
}

type anthropicMessagesResponse struct {
	Content []anthropicContentBlock `json:"content"`
	Usage   anthropicUsage          `json:"usage"`
}

type anthropicUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
}

func (u anthropicUsage) TokenUsage() TokenUsage {
	inputTokens := u.InputTokens + u.CacheCreationInputTokens + u.CacheReadInputTokens
	return TokenUsage{
		InputTokens:  inputTokens,
		OutputTokens: u.OutputTokens,
		CachedTokens: u.CacheReadInputTokens,
		TotalTokens:  inputTokens + u.OutputTokens,
	}
}

type anthropicUsageEvent struct {
	InputTokens              *int `json:"input_tokens,omitempty"`
	OutputTokens             *int `json:"output_tokens,omitempty"`
	CacheCreationInputTokens *int `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens     *int `json:"cache_read_input_tokens,omitempty"`
}

func (u anthropicUsageEvent) ApplyTo(target *anthropicUsage) {
	if target == nil {
		return
	}
	if u.InputTokens != nil {
		target.InputTokens = *u.InputTokens
	}
	if u.OutputTokens != nil {
		target.OutputTokens = *u.OutputTokens
	}
	if u.CacheCreationInputTokens != nil {
		target.CacheCreationInputTokens = *u.CacheCreationInputTokens
	}
	if u.CacheReadInputTokens != nil {
		target.CacheReadInputTokens = *u.CacheReadInputTokens
	}
}

type anthropicContentBlock struct {
	Type  string          `json:"type"`
	Text  string          `json:"text,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
}

func (r anthropicMessagesResponse) OutputText() string {
	var parts []string
	for _, item := range r.Content {
		if item.Type == "text" && strings.TrimSpace(item.Text) != "" {
			parts = append(parts, strings.TrimSpace(item.Text))
		}
	}
	return strings.Join(parts, "\n")
}

func (r anthropicMessagesResponse) StructuredOutput() (string, bool, error) {
	for _, item := range r.Content {
		if item.Type != "tool_use" {
			continue
		}
		if len(bytes.TrimSpace(item.Input)) == 0 {
			return "", true, fmt.Errorf("anthropic tool_use returned empty input")
		}
		var compact bytes.Buffer
		if err := json.Compact(&compact, item.Input); err != nil {
			return "", true, fmt.Errorf("compact anthropic tool_use input: %w", err)
		}
		return compact.String(), true, nil
	}
	return "", false, nil
}

type anthropicErrorResponse struct {
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
}

type anthropicAPIError struct {
	StatusCode int
	Message    string
}

func (e *anthropicAPIError) Error() string {
	return fmt.Sprintf("anthropic messages api returned %d: %s", e.StatusCode, e.Message)
}

func buildChatCompletionMessages(req generationRequest) []chatCompletionsMessage {
	messages := make([]chatCompletionsMessage, 0, len(req.Input)+1)
	if strings.TrimSpace(req.Instructions) != "" {
		messages = append(messages, chatCompletionsMessage{
			Role:    "system",
			Content: req.Instructions,
		})
	}
	for _, message := range req.Input {
		messages = append(messages, chatCompletionsMessage{
			Role:    message.Role,
			Content: chatCompletionContent(message.Content),
		})
	}
	return messages
}

func chatCompletionContent(content []responseContentInput) any {
	if len(content) == 1 && isTextContentType(content[0].Type) {
		return content[0].Text
	}
	parts := make([]chatCompletionsContentPart, 0, len(content))
	for _, item := range content {
		switch item.Type {
		case "input_text", "output_text":
			parts = append(parts, chatCompletionsContentPart{
				Type: "text",
				Text: item.Text,
			})
		case "input_image":
			parts = append(parts, chatCompletionsContentPart{
				Type: "image_url",
				ImageURL: &chatCompletionsImageURLPart{
					URL:    item.ImageURL,
					Detail: item.Detail,
				},
			})
		}
	}
	return parts
}

func buildAnthropicRequest(model string, req generationRequest) (anthropicMessagesRequest, error) {
	return buildAnthropicRequestWithMode(model, req, req.WantsJSON())
}

func buildAnthropicLegacyRequest(model string, req generationRequest) (anthropicMessagesRequest, error) {
	return buildAnthropicRequestWithMode(model, req, false)
}

func buildAnthropicRequestWithMode(model string, req generationRequest, useStructuredTool bool) (anthropicMessagesRequest, error) {
	request := anthropicMessagesRequest{
		Model:       model,
		System:      anthropicSystemPrompt(req, useStructuredTool),
		Messages:    make([]anthropicMessageRequest, 0, len(req.Input)),
		MaxTokens:   req.MaxOutputTokens,
		Temperature: req.Temperature,
		TopP:        req.TopP,
	}
	for _, message := range req.Input {
		content, err := anthropicContent(message.Content)
		if err != nil {
			return anthropicMessagesRequest{}, err
		}
		request.Messages = append(request.Messages, anthropicMessageRequest{
			Role:    message.Role,
			Content: content,
		})
	}
	if req.WantsJSON() && useStructuredTool {
		request.Tools = []anthropicToolDefinition{
			{
				Name:        req.SchemaName,
				Description: "Return the final structured response for baize.",
				InputSchema: req.Schema,
			},
		}
		request.ToolChoice = &anthropicToolChoice{
			Type: "tool",
			Name: req.SchemaName,
		}
	}
	return request, nil
}

func anthropicSystemPrompt(req generationRequest, useStructuredTool bool) string {
	prompt := strings.TrimSpace(req.Instructions)
	if !req.WantsJSON() {
		return prompt
	}
	if useStructuredTool {
		extra := "Return the final structured result by calling the provided tool exactly once. Do not emit explanatory text outside the tool call."
		if prompt == "" {
			return extra
		}
		return strings.TrimSpace(prompt + "\n\n" + extra)
	}
	schemaText, err := json.MarshalIndent(req.Schema, "", "  ")
	if err != nil {
		return strings.TrimSpace(prompt + "\n\nReturn only valid JSON.")
	}
	extra := "Return only valid JSON that exactly matches this JSON schema:\n" + string(schemaText)
	if prompt == "" {
		return extra
	}
	return strings.TrimSpace(prompt + "\n\n" + extra)
}

func anthropicContent(content []responseContentInput) ([]anthropicContentPart, error) {
	parts := make([]anthropicContentPart, 0, len(content))
	for _, item := range content {
		switch item.Type {
		case "input_text", "output_text":
			parts = append(parts, anthropicContentPart{
				Type: "text",
				Text: item.Text,
			})
		case "input_image":
			mediaType, data, err := parseDataURL(item.ImageURL)
			if err != nil {
				return nil, err
			}
			parts = append(parts, anthropicContentPart{
				Type: "image",
				Source: &anthropicImageSource{
					Type:      "base64",
					MediaType: mediaType,
					Data:      data,
				},
			})
		}
	}
	return parts, nil
}

func isTextContentType(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "input_text", "output_text":
		return true
	default:
		return false
	}
}

func parseDataURL(value string) (string, string, error) {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, "data:") {
		return "", "", fmt.Errorf("anthropic image input requires data url content")
	}
	header, data, ok := strings.Cut(value, ",")
	if !ok {
		return "", "", fmt.Errorf("invalid data url image content")
	}
	mediaType := strings.TrimPrefix(header, "data:")
	if !strings.HasSuffix(strings.ToLower(mediaType), ";base64") {
		return "", "", fmt.Errorf("anthropic image input requires base64 data url content")
	}
	mediaType = strings.TrimSuffix(mediaType, ";base64")
	if mediaType == "" {
		mediaType = "application/octet-stream"
	}
	if _, err := base64.StdEncoding.DecodeString(data); err != nil {
		return "", "", fmt.Errorf("invalid base64 image content: %w", err)
	}
	return mediaType, data, nil
}

func consumeServerSentEvents(r io.Reader, handle func(eventName string, data []byte) error) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var eventName string
	var dataLines []string
	dispatch := func() error {
		if len(dataLines) == 0 {
			eventName = ""
			return nil
		}
		data := strings.Join(dataLines, "\n")
		currentEvent := eventName
		eventName, dataLines = "", nil
		return handle(currentEvent, []byte(data))
	}

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if err := dispatch(); err != nil {
				return err
			}
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		if strings.HasPrefix(line, "event:") {
			eventName = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			continue
		}
		if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return dispatch()
}

func parseStreamError(payload []byte) error {
	var event struct {
		Type    string `json:"type"`
		Message string `json:"message"`
		Error   *struct {
			Message string `json:"message"`
		} `json:"error,omitempty"`
	}
	if err := json.Unmarshal(payload, &event); err != nil {
		return err
	}
	if event.Error != nil && strings.TrimSpace(event.Error.Message) != "" {
		return errors.New(strings.TrimSpace(event.Error.Message))
	}
	if strings.TrimSpace(event.Message) != "" {
		return errors.New(strings.TrimSpace(event.Message))
	}
	return fmt.Errorf("stream error: %s", strings.TrimSpace(string(payload)))
}

func decodeStructuredResponse(text string, out any) error {
	var firstErr error
	for _, candidate := range structuredResponseCandidates(text) {
		if err := decodeStructuredCandidate(candidate, out); err == nil {
			return nil
		} else if firstErr == nil {
			firstErr = err
		}
	}
	if firstErr == nil {
		firstErr = io.EOF
	}
	return firstErr
}

func stripCodeFence(text string) string {
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, "```") {
		return text
	}
	lines := strings.Split(text, "\n")
	if len(lines) < 3 {
		return text
	}
	if strings.HasPrefix(strings.TrimSpace(lines[len(lines)-1]), "```") {
		lines = lines[1 : len(lines)-1]
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func structuredResponseCandidates(text string) []string {
	trimmed := strings.TrimSpace(text)
	stripped := stripCodeFence(text)
	candidates := []string{trimmed, stripped}

	if jsonStart := extractJSONStart(trimmed); jsonStart != "" {
		candidates = append(candidates, jsonStart)
	}
	if jsonStart := extractJSONStart(stripped); jsonStart != "" {
		candidates = append(candidates, jsonStart)
	}

	seen := make(map[string]struct{}, len(candidates))
	unique := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		if _, ok := seen[candidate]; ok {
			continue
		}
		seen[candidate] = struct{}{}
		unique = append(unique, candidate)
	}
	return unique
}

func decodeStructuredCandidate(text string, out any) error {
	if raw, err := decodeStructuredJSONValue(text); err == nil {
		return json.Unmarshal(raw, out)
	}

	decoded := newDecodeTarget(out)
	if err := json.Unmarshal([]byte(text), decoded.Interface()); err != nil {
		return err
	}
	reflect.ValueOf(out).Elem().Set(decoded.Elem())
	return nil
}

func decodeStructuredJSONValue(text string) ([]byte, error) {
	decoder := json.NewDecoder(strings.NewReader(strings.TrimSpace(text)))
	var last json.RawMessage
	for {
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			if len(last) > 0 {
				return last, nil
			}
			return nil, err
		}
		if len(bytes.TrimSpace(raw)) == 0 {
			continue
		}
		last = append(last[:0], raw...)
	}
	if len(last) == 0 {
		return nil, io.EOF
	}
	return last, nil
}

func extractJSONStart(text string) string {
	text = strings.TrimSpace(text)
	start := strings.IndexAny(text, "{[")
	if start == -1 {
		return ""
	}
	return strings.TrimSpace(text[start:])
}

func newDecodeTarget(out any) reflect.Value {
	target := reflect.ValueOf(out)
	if target.Kind() != reflect.Pointer || target.IsNil() {
		panic("decode target must be a non-nil pointer")
	}
	return reflect.New(target.Elem().Type())
}
