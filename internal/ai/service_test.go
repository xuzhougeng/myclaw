package ai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"baize/internal/filesearch"
	"baize/internal/knowledge"
	"baize/internal/modelconfig"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func newTestClient(t *testing.T, handler func(*http.Request) (*http.Response, error)) *http.Client {
	t.Helper()
	return &http.Client{
		Transport: roundTripFunc(handler),
	}
}

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header: http.Header{
			"Content-Type": []string{"application/json"},
		},
		Body: io.NopCloser(strings.NewReader(body)),
	}
}

func streamResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"text/event-stream"},
		},
		Body: io.NopCloser(strings.NewReader(body)),
	}
}

func newConfiguredStore(t *testing.T, cfg modelconfig.Config) *modelconfig.Store {
	t.Helper()

	store := modelconfig.NewStore(filepath.Join(t.TempDir(), "model", "app.db"))
	if _, err := store.Save(context.Background(), cfg, modelconfig.SaveOptions{SetActive: true}); err != nil {
		t.Fatalf("save model config: %v", err)
	}
	return store
}

func intPtr(v int) *int {
	return &v
}

func TestRouteCommandUsesOpenAIResponses(t *testing.T) {
	store := newConfiguredStore(t, modelconfig.Config{
		Provider:            modelconfig.ProviderOpenAI,
		APIType:             modelconfig.APITypeResponses,
		BaseURL:             "http://example.invalid/v1",
		APIKey:              "secret",
		Model:               "gpt-test",
		MaxOutputTokensText: intPtr(1501),
		MaxOutputTokensJSON: intPtr(901),
	})

	service := NewService(store)
	service.httpClient = newTestClient(t, func(r *http.Request) (*http.Response, error) {
		var req responsesRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.Text == nil || req.Text.Format.Type != "json_schema" {
			t.Fatalf("expected json schema request, got %#v", req.Text)
		}
		if req.MaxOutputTokens != 901 {
			t.Fatalf("expected json token override, got %d", req.MaxOutputTokens)
		}
		if r.URL.Path != "/v1/responses" {
			t.Fatalf("unexpected endpoint: %s", r.URL.Path)
		}
		return jsonResponse(http.StatusOK, `{"output":[{"type":"message","content":[{"type":"output_text","text":"{\"command\":\"remember\",\"memory_text\":\"- 已整理内容\",\"append_text\":\"\",\"knowledge_id\":\"\",\"reminder_spec\":\"\",\"reminder_id\":\"\",\"question\":\"\"}"}]}]}`), nil
	})

	decision, err := service.RouteCommand(context.Background(), "请帮我记住这个东西：abc")
	if err != nil {
		t.Fatalf("route: %v", err)
	}
	if decision.Command != "remember" {
		t.Fatalf("unexpected command: %#v", decision)
	}
	if !strings.Contains(decision.MemoryText, "整理内容") {
		t.Fatalf("unexpected memory text: %#v", decision)
	}
}

func TestChatUsesTextMaxOutputTokensOverride(t *testing.T) {
	store := newConfiguredStore(t, modelconfig.Config{
		Provider:            modelconfig.ProviderOpenAI,
		APIType:             modelconfig.APITypeResponses,
		BaseURL:             "http://example.invalid/v1",
		APIKey:              "secret",
		Model:               "gpt-test",
		MaxOutputTokensText: intPtr(1601),
		MaxOutputTokensJSON: intPtr(801),
	})

	service := NewService(store)
	service.httpClient = newTestClient(t, func(r *http.Request) (*http.Response, error) {
		var req responsesRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.MaxOutputTokens != 1601 {
			t.Fatalf("expected text token override, got %d", req.MaxOutputTokens)
		}
		if req.Text == nil || req.Text.Format.Type != "text" {
			t.Fatalf("expected text request, got %#v", req.Text)
		}
		return jsonResponse(http.StatusOK, `{"output":[{"type":"message","content":[{"type":"output_text","text":"你好"}]}]}`), nil
	})

	reply, err := service.Chat(context.Background(), "hi", nil)
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if reply != "你好" {
		t.Fatalf("unexpected reply: %q", reply)
	}
}

func TestChatUsesAssistantOutputTextInResponsesHistory(t *testing.T) {
	store := newConfiguredStore(t, modelconfig.Config{
		Provider: modelconfig.ProviderOpenAI,
		APIType:  modelconfig.APITypeResponses,
		BaseURL:  "http://example.invalid/v1",
		APIKey:   "secret",
		Model:    "gpt-test",
	})

	service := NewService(store)
	service.httpClient = newTestClient(t, func(r *http.Request) (*http.Response, error) {
		var req responsesRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if len(req.Input) != 3 {
			t.Fatalf("expected 3 messages, got %#v", req.Input)
		}
		if req.Input[0].Role != "user" || req.Input[0].Content[0].Type != "input_text" {
			t.Fatalf("unexpected first history item: %#v", req.Input[0])
		}
		if req.Input[1].Role != "assistant" || req.Input[1].Content[0].Type != "output_text" {
			t.Fatalf("unexpected assistant history item: %#v", req.Input[1])
		}
		if req.Input[2].Role != "user" || req.Input[2].Content[0].Type != "input_text" {
			t.Fatalf("unexpected latest user item: %#v", req.Input[2])
		}
		return jsonResponse(http.StatusOK, `{"output":[{"type":"message","content":[{"type":"output_text","text":"你好"}]}]}`), nil
	})

	reply, err := service.Chat(context.Background(), "继续", []ConversationMessage{
		{Role: "user", Content: "第一句"},
		{Role: "assistant", Content: "第一句回复"},
	})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if reply != "你好" {
		t.Fatalf("unexpected reply: %q", reply)
	}
}

func TestServiceUsesConfiguredRequestTimeout(t *testing.T) {
	store := newConfiguredStore(t, modelconfig.Config{
		Provider:              modelconfig.ProviderOpenAI,
		APIType:               modelconfig.APITypeResponses,
		BaseURL:               "http://example.invalid/v1",
		APIKey:                "secret",
		Model:                 "gpt-test",
		RequestTimeoutSeconds: intPtr(210),
	})

	service := NewService(store)
	cfg, err := service.CurrentConfig(context.Background())
	if err != nil {
		t.Fatalf("current config: %v", err)
	}

	if got := service.httpClientForConfig(cfg).Timeout; got != 210*time.Second {
		t.Fatalf("expected configured timeout, got %v", got)
	}
	if got := service.httpClientForConfig(modelconfig.DefaultConfig()).Timeout; got != time.Duration(modelconfig.DefaultRequestTimeoutSeconds)*time.Second {
		t.Fatalf("expected default timeout, got %v", got)
	}
}

func TestOpenAIChatCompletionsStructuredRequest(t *testing.T) {
	store := newConfiguredStore(t, modelconfig.Config{
		Provider: modelconfig.ProviderOpenAI,
		APIType:  modelconfig.APITypeChatCompletions,
		BaseURL:  "http://example.invalid/v1",
		APIKey:   "secret",
		Model:    "gpt-4o-mini",
	})

	service := NewService(store)
	service.httpClient = newTestClient(t, func(r *http.Request) (*http.Response, error) {
		var req chatCompletionsRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected endpoint: %s", r.URL.Path)
		}
		if req.ResponseFormat == nil || req.ResponseFormat.Type != "json_schema" {
			t.Fatalf("expected chat completions json schema, got %#v", req.ResponseFormat)
		}
		if len(req.Messages) < 2 || req.Messages[0].Role != "system" {
			t.Fatalf("expected system+user messages, got %#v", req.Messages)
		}
		return jsonResponse(http.StatusOK, `{"choices":[{"message":{"content":"{\"command\":\"answer\",\"memory_text\":\"\",\"append_text\":\"\",\"knowledge_id\":\"\",\"reminder_spec\":\"\",\"reminder_id\":\"\",\"question\":\"整理一下\"}"}}]}`), nil
	})

	decision, err := service.RouteCommand(context.Background(), "整理一下")
	if err != nil {
		t.Fatalf("route via chat completions: %v", err)
	}
	if decision.Command != "answer" || decision.Question != "整理一下" {
		t.Fatalf("unexpected decision: %#v", decision)
	}
}

func TestDecodeStructuredResponseUsesLastJSONValue(t *testing.T) {
	var got struct {
		Command  string `json:"command"`
		Question string `json:"question"`
	}

	err := decodeStructuredResponse(
		`{"command":"answer","question":"first"}{"command":"help","question":"second"}`,
		&got,
	)
	if err != nil {
		t.Fatalf("decode structured response: %v", err)
	}
	if got.Command != "help" || got.Question != "second" {
		t.Fatalf("unexpected decoded value: %#v", got)
	}
}

func TestDecodeStructuredResponseSkipsWrapperText(t *testing.T) {
	var got struct {
		Command  string `json:"command"`
		Question string `json:"question"`
	}

	err := decodeStructuredResponse(
		"Here is the result:\n```json\n{\"command\":\"answer\",\"question\":\"整理一下\"}\n```\nDone.",
		&got,
	)
	if err != nil {
		t.Fatalf("decode structured response: %v", err)
	}
	if got.Command != "answer" || got.Question != "整理一下" {
		t.Fatalf("unexpected decoded value: %#v", got)
	}
}

func TestAnthropicMessagesStructuredRequest(t *testing.T) {
	store := newConfiguredStore(t, modelconfig.Config{
		Provider: modelconfig.ProviderAnthropic,
		APIType:  modelconfig.APITypeMessages,
		BaseURL:  "http://example.invalid/v1",
		APIKey:   "anthropic-secret",
		Model:    "claude-3-7-sonnet-latest",
	})

	service := NewService(store)
	service.httpClient = newTestClient(t, func(r *http.Request) (*http.Response, error) {
		var req anthropicMessagesRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if r.URL.Path != "/v1/messages" {
			t.Fatalf("unexpected endpoint: %s", r.URL.Path)
		}
		if r.Header.Get("X-Api-Key") != "anthropic-secret" {
			t.Fatalf("unexpected anthropic api key header: %q", r.Header.Get("X-Api-Key"))
		}
		if r.Header.Get("Anthropic-Version") != "2023-06-01" {
			t.Fatalf("unexpected anthropic version header: %q", r.Header.Get("Anthropic-Version"))
		}
		if !strings.Contains(req.System, "provided tool exactly once") {
			t.Fatalf("expected tool-use prompt, got %q", req.System)
		}
		if len(req.Tools) != 1 || req.Tools[0].Name != "route_decision" {
			t.Fatalf("expected structured tool, got %#v", req.Tools)
		}
		if req.ToolChoice == nil || req.ToolChoice.Type != "tool" || req.ToolChoice.Name != "route_decision" {
			t.Fatalf("expected forced tool choice, got %#v", req.ToolChoice)
		}
		if len(req.Messages) == 0 || len(req.Messages[0].Content) == 0 || req.Messages[0].Content[0].Type != "text" {
			t.Fatalf("unexpected anthropic content: %#v", req.Messages)
		}
		return jsonResponse(http.StatusOK, `{"content":[{"type":"tool_use","name":"route_decision","input":{"command":"help","memory_text":"","append_text":"","knowledge_id":"","reminder_spec":"","reminder_id":"","question":""}}]}`), nil
	})

	decision, err := service.RouteCommand(context.Background(), "help")
	if err != nil {
		t.Fatalf("route via anthropic: %v", err)
	}
	if decision.Command != "help" {
		t.Fatalf("unexpected decision: %#v", decision)
	}
}

func TestAnthropicMessagesStructuredRequestFallsBackWithoutTools(t *testing.T) {
	store := newConfiguredStore(t, modelconfig.Config{
		Provider: modelconfig.ProviderAnthropic,
		APIType:  modelconfig.APITypeMessages,
		BaseURL:  "http://example.invalid/v1",
		APIKey:   "anthropic-secret",
		Model:    "claude-3-7-sonnet-latest",
	})

	service := NewService(store)
	requests := 0
	service.httpClient = newTestClient(t, func(r *http.Request) (*http.Response, error) {
		requests++

		var req anthropicMessagesRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}

		switch requests {
		case 1:
			if len(req.Tools) != 1 || req.ToolChoice == nil {
				t.Fatalf("expected first anthropic request to use tools, got %#v / %#v", req.Tools, req.ToolChoice)
			}
			return jsonResponse(http.StatusBadRequest, `{"error":{"message":"tools are not supported on this upstream"}}`), nil
		case 2:
			if len(req.Tools) != 0 || req.ToolChoice != nil {
				t.Fatalf("expected legacy fallback without tools, got %#v / %#v", req.Tools, req.ToolChoice)
			}
			if !strings.Contains(req.System, "JSON schema") {
				t.Fatalf("expected legacy schema prompt, got %q", req.System)
			}
			return jsonResponse(http.StatusOK, `{"content":[{"type":"text","text":"{\"command\":\"help\",\"memory_text\":\"\",\"append_text\":\"\",\"knowledge_id\":\"\",\"reminder_spec\":\"\",\"reminder_id\":\"\",\"question\":\"\"}"}]}`), nil
		default:
			t.Fatalf("unexpected extra anthropic request: %d", requests)
			return nil, nil
		}
	})

	decision, err := service.RouteCommand(context.Background(), "help")
	if err != nil {
		t.Fatalf("route via anthropic fallback: %v", err)
	}
	if decision.Command != "help" {
		t.Fatalf("unexpected decision after fallback: %#v", decision)
	}
	if requests != 2 {
		t.Fatalf("expected 2 anthropic requests, got %d", requests)
	}
}

func TestOpenAIResponsesChatStream(t *testing.T) {
	store := newConfiguredStore(t, modelconfig.Config{
		Provider: modelconfig.ProviderOpenAI,
		APIType:  modelconfig.APITypeResponses,
		BaseURL:  "http://example.invalid/v1",
		APIKey:   "secret",
		Model:    "gpt-test",
	})

	service := NewService(store)
	service.httpClient = newTestClient(t, func(r *http.Request) (*http.Response, error) {
		var req responsesRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if !req.Stream {
			t.Fatalf("expected streaming request, got %#v", req)
		}
		return streamResponse(strings.Join([]string{
			`data: {"type":"response.output_text.delta","delta":"你好"}`,
			``,
			`data: {"type":"response.output_text.delta","delta":"，世界"}`,
			``,
			`data: {"type":"response.completed","response":{"output":[{"type":"message","content":[{"type":"output_text","text":"你好，世界"}]}]}}`,
			``,
		}, "\n")), nil
	})

	var deltas []string
	reply, err := service.ChatStream(context.Background(), "hi", nil, func(delta string) {
		deltas = append(deltas, delta)
	})
	if err != nil {
		t.Fatalf("stream chat: %v", err)
	}
	if reply != "你好，世界" {
		t.Fatalf("unexpected reply: %q", reply)
	}
	if strings.Join(deltas, "") != "你好，世界" {
		t.Fatalf("unexpected deltas: %#v", deltas)
	}
}

func TestOpenAIResponsesUsageIsCollected(t *testing.T) {
	store := newConfiguredStore(t, modelconfig.Config{
		Provider: modelconfig.ProviderOpenAI,
		APIType:  modelconfig.APITypeResponses,
		BaseURL:  "http://example.invalid/v1",
		APIKey:   "secret",
		Model:    "gpt-test",
	})

	service := NewService(store)
	service.httpClient = newTestClient(t, func(r *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, `{
			"output":[{"type":"message","content":[{"type":"output_text","text":"你好"}]}],
			"usage":{
				"input_tokens":90,
				"input_tokens_details":{"cached_tokens":24},
				"output_tokens":15,
				"total_tokens":105
			}
		}`), nil
	})

	ctx := WithUsageCollector(context.Background())
	reply, err := service.Chat(ctx, "hi", nil)
	if err != nil {
		t.Fatalf("chat with usage: %v", err)
	}
	if reply != "你好" {
		t.Fatalf("unexpected reply: %q", reply)
	}

	usage := UsageFromContext(ctx)
	if usage.InputTokens != 90 || usage.OutputTokens != 15 || usage.CachedTokens != 24 || usage.TotalTokens != 105 {
		t.Fatalf("unexpected usage: %#v", usage)
	}
}

func TestOpenAIChatCompletionsStream(t *testing.T) {
	store := newConfiguredStore(t, modelconfig.Config{
		Provider: modelconfig.ProviderOpenAI,
		APIType:  modelconfig.APITypeChatCompletions,
		BaseURL:  "http://example.invalid/v1",
		APIKey:   "secret",
		Model:    "gpt-4o-mini",
	})

	service := NewService(store)
	service.httpClient = newTestClient(t, func(r *http.Request) (*http.Response, error) {
		var req chatCompletionsRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if !req.Stream {
			t.Fatalf("expected streaming request, got %#v", req)
		}
		if req.StreamOptions == nil || !req.StreamOptions.IncludeUsage {
			t.Fatalf("expected stream usage request, got %#v", req.StreamOptions)
		}
		return streamResponse(strings.Join([]string{
			`data: {"choices":[{"delta":{"content":"分"}}]}`,
			``,
			`data: {"choices":[{"delta":{"content":"段输出"}}]}`,
			``,
			`data: [DONE]`,
			``,
		}, "\n")), nil
	})

	var deltas []string
	reply, err := service.ChatStream(context.Background(), "hi", nil, func(delta string) {
		deltas = append(deltas, delta)
	})
	if err != nil {
		t.Fatalf("stream chat completions: %v", err)
	}
	if reply != "分段输出" {
		t.Fatalf("unexpected reply: %q", reply)
	}
	if strings.Join(deltas, "") != "分段输出" {
		t.Fatalf("unexpected deltas: %#v", deltas)
	}
}

func TestOpenAIChatCompletionsStreamUsageIsCollected(t *testing.T) {
	store := newConfiguredStore(t, modelconfig.Config{
		Provider: modelconfig.ProviderOpenAI,
		APIType:  modelconfig.APITypeChatCompletions,
		BaseURL:  "http://example.invalid/v1",
		APIKey:   "secret",
		Model:    "gpt-4o-mini",
	})

	service := NewService(store)
	service.httpClient = newTestClient(t, func(r *http.Request) (*http.Response, error) {
		var req chatCompletionsRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.StreamOptions == nil || !req.StreamOptions.IncludeUsage {
			t.Fatalf("expected stream usage request, got %#v", req.StreamOptions)
		}
		return streamResponse(strings.Join([]string{
			`data: {"choices":[{"delta":{"content":"流"}}]}`,
			``,
			`data: {"choices":[{"delta":{"content":"式"}}]}`,
			``,
			`data: {"choices":[],"usage":{"prompt_tokens":120,"prompt_tokens_details":{"cached_tokens":30},"completion_tokens":25,"total_tokens":145}}`,
			``,
			`data: [DONE]`,
			``,
		}, "\n")), nil
	})

	ctx := WithUsageCollector(context.Background())
	reply, err := service.ChatStream(ctx, "hi", nil, func(string) {})
	if err != nil {
		t.Fatalf("stream chat completions with usage: %v", err)
	}
	if reply != "流式" {
		t.Fatalf("unexpected reply: %q", reply)
	}

	usage := UsageFromContext(ctx)
	if usage.InputTokens != 120 || usage.OutputTokens != 25 || usage.CachedTokens != 30 || usage.TotalTokens != 145 {
		t.Fatalf("unexpected usage: %#v", usage)
	}
}

func TestAnthropicMessagesStream(t *testing.T) {
	store := newConfiguredStore(t, modelconfig.Config{
		Provider: modelconfig.ProviderAnthropic,
		APIType:  modelconfig.APITypeMessages,
		BaseURL:  "http://example.invalid/v1",
		APIKey:   "anthropic-secret",
		Model:    "claude-3-7-sonnet-latest",
	})

	service := NewService(store)
	service.httpClient = newTestClient(t, func(r *http.Request) (*http.Response, error) {
		var req anthropicMessagesRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if !req.Stream {
			t.Fatalf("expected streaming request, got %#v", req)
		}
		return streamResponse(strings.Join([]string{
			`event: message_start`,
			`data: {"type":"message_start","message":{"content":[]}}`,
			``,
			`event: content_block_delta`,
			`data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"Claude "}}`,
			``,
			`event: content_block_delta`,
			`data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"stream"}}`,
			``,
			`event: message_stop`,
			`data: {"type":"message_stop"}`,
			``,
		}, "\n")), nil
	})

	var deltas []string
	reply, err := service.ChatStream(context.Background(), "hi", nil, func(delta string) {
		deltas = append(deltas, delta)
	})
	if err != nil {
		t.Fatalf("stream anthropic: %v", err)
	}
	if reply != "Claude stream" {
		t.Fatalf("unexpected reply: %q", reply)
	}
	if strings.Join(deltas, "") != "Claude stream" {
		t.Fatalf("unexpected deltas: %#v", deltas)
	}
}

func TestAnthropicMessagesStreamUsageIsCollected(t *testing.T) {
	store := newConfiguredStore(t, modelconfig.Config{
		Provider: modelconfig.ProviderAnthropic,
		APIType:  modelconfig.APITypeMessages,
		BaseURL:  "http://example.invalid/v1",
		APIKey:   "anthropic-secret",
		Model:    "claude-3-7-sonnet-latest",
	})

	service := NewService(store)
	service.httpClient = newTestClient(t, func(r *http.Request) (*http.Response, error) {
		return streamResponse(strings.Join([]string{
			`event: message_start`,
			`data: {"type":"message_start","message":{"content":[],"usage":{"input_tokens":40,"cache_creation_input_tokens":10,"cache_read_input_tokens":12,"output_tokens":1}}}`,
			``,
			`event: content_block_delta`,
			`data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"Claude"}}`,
			``,
			`event: message_delta`,
			`data: {"type":"message_delta","usage":{"output_tokens":18}}`,
			``,
			`event: message_stop`,
			`data: {"type":"message_stop"}`,
			``,
		}, "\n")), nil
	})

	ctx := WithUsageCollector(context.Background())
	reply, err := service.ChatStream(ctx, "hi", nil, func(string) {})
	if err != nil {
		t.Fatalf("stream anthropic with usage: %v", err)
	}
	if reply != "Claude" {
		t.Fatalf("unexpected reply: %q", reply)
	}

	usage := UsageFromContext(ctx)
	if usage.InputTokens != 62 || usage.OutputTokens != 18 || usage.CachedTokens != 12 || usage.TotalTokens != 80 {
		t.Fatalf("unexpected usage: %#v", usage)
	}
}

func TestAnswerUsesKnowledgeEntries(t *testing.T) {
	store := newConfiguredStore(t, modelconfig.Config{
		Provider: modelconfig.ProviderOpenAI,
		APIType:  modelconfig.APITypeResponses,
		BaseURL:  "http://example.invalid/v1",
		APIKey:   "secret",
		Model:    "gpt-test",
	})

	service := NewService(store)
	service.httpClient = newTestClient(t, func(r *http.Request) (*http.Response, error) {
		var req responsesRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if len(req.Input) == 0 || !strings.Contains(req.Input[0].Content[0].Text, "未来要支持 macOS") {
			t.Fatalf("knowledge not included in prompt: %#v", req.Input)
		}
		return jsonResponse(http.StatusOK, `{"output":[{"type":"message","content":[{"type":"output_text","text":"可以，知识库里提到未来要支持 macOS。"}]}]}`), nil
	})

	reply, err := service.Answer(context.Background(), "未来要支持什么？", []knowledge.Entry{
		{Text: "未来要支持 macOS"},
	})
	if err != nil {
		t.Fatalf("answer: %v", err)
	}
	if !strings.Contains(reply, "macOS") {
		t.Fatalf("unexpected reply: %q", reply)
	}
}

func TestBuildSearchPlan(t *testing.T) {
	store := newConfiguredStore(t, modelconfig.Config{
		Provider: modelconfig.ProviderOpenAI,
		APIType:  modelconfig.APITypeResponses,
		BaseURL:  "http://example.invalid/v1",
		APIKey:   "secret",
		Model:    "gpt-test",
	})

	service := NewService(store)
	service.httpClient = newTestClient(t, func(r *http.Request) (*http.Response, error) {
		var req responsesRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.Text == nil || req.Text.Format.Name != "search_plan" {
			t.Fatalf("unexpected schema request: %#v", req.Text)
		}
		return jsonResponse(http.StatusOK, `{"output":[{"type":"message","content":[{"type":"output_text","text":"{\"queries\":[\"macOS 支持计划\",\"macOS 什么时候做\"],\"keywords\":[\"macOS\",\"支持\"]}"}]}]}`), nil
	})

	plan, err := service.BuildSearchPlan(context.Background(), "macOS 什么时候做？")
	if err != nil {
		t.Fatalf("build search plan: %v", err)
	}
	if len(plan.Queries) != 2 || plan.Queries[0] != "macOS 支持计划" {
		t.Fatalf("unexpected queries: %#v", plan)
	}
	if len(plan.Keywords) == 0 || plan.Keywords[0] != "macos" {
		t.Fatalf("unexpected plan: %#v", plan)
	}
}

func TestDetectToolOpportunities(t *testing.T) {
	store := newConfiguredStore(t, modelconfig.Config{
		Provider: modelconfig.ProviderOpenAI,
		APIType:  modelconfig.APITypeResponses,
		BaseURL:  "http://example.invalid/v1",
		APIKey:   "secret",
		Model:    "gpt-test",
	})

	service := NewService(store)
	service.httpClient = newTestClient(t, func(r *http.Request) (*http.Response, error) {
		var req responsesRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.Text == nil || req.Text.Format.Name != "tool_opportunity_detection" {
			t.Fatalf("unexpected schema request: %#v", req.Text)
		}
		return jsonResponse(http.StatusOK, `{"output":[{"type":"message","content":[{"type":"output_text","text":"{\"matches\":[{\"tool_name\":\"everything_file_search\",\"goal\":\"查找 D 盘上的 PDF 文件\"}]}"}]}]}`), nil
	})

	matches, err := service.DetectToolOpportunities(context.Background(), "查找 D 盘单细胞相关的PDF文件", []ToolCapability{
		ToolCapabilityFromContract(filesearch.Definition()),
	})
	if err != nil {
		t.Fatalf("detect tool opportunities: %v", err)
	}
	if len(matches) != 1 || matches[0].ToolName != filesearch.ToolName {
		t.Fatalf("unexpected matches: %#v", matches)
	}
}

func TestPlanToolUse(t *testing.T) {
	store := newConfiguredStore(t, modelconfig.Config{
		Provider: modelconfig.ProviderOpenAI,
		APIType:  modelconfig.APITypeResponses,
		BaseURL:  "http://example.invalid/v1",
		APIKey:   "secret",
		Model:    "gpt-test",
	})

	service := NewService(store)
	service.httpClient = newTestClient(t, func(r *http.Request) (*http.Response, error) {
		var req responsesRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.Text == nil || req.Text.Format.Name != "tool_use_plan" {
			t.Fatalf("unexpected schema request: %#v", req.Text)
		}
		return jsonResponse(http.StatusOK, `{"output":[{"type":"message","content":[{"type":"output_text","text":"{\"action\":\"tool\",\"tool_name\":\"everything_file_search\",\"tool_input\":\"{\\\"paths\\\":[\\\"Downloads\\\"],\\\"extensions\\\":[\\\"pdf\\\"],\\\"date_field\\\":\\\"created\\\",\\\"date_value\\\":\\\"last2days\\\"}\",\"user_message\":\"\"}"}]}]}`), nil
	})

	decision, err := service.PlanToolUse(context.Background(), "查找下载目录下这两天下载的文件", ToolCapabilityFromContract(filesearch.Definition()), nil)
	if err != nil {
		t.Fatalf("plan tool use: %v", err)
	}
	if decision.Action != "tool" || decision.ToolName != filesearch.ToolName {
		t.Fatalf("unexpected decision: %#v", decision)
	}
	if !strings.Contains(decision.ToolInput, `"paths":["Downloads"]`) {
		t.Fatalf("unexpected tool input: %#v", decision)
	}
}

func TestReviewAnswerCandidates(t *testing.T) {
	store := newConfiguredStore(t, modelconfig.Config{
		Provider: modelconfig.ProviderOpenAI,
		APIType:  modelconfig.APITypeResponses,
		BaseURL:  "http://example.invalid/v1",
		APIKey:   "secret",
		Model:    "gpt-test",
	})

	service := NewService(store)
	service.httpClient = newTestClient(t, func(r *http.Request) (*http.Response, error) {
		var req responsesRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.Text == nil || req.Text.Format.Name != "retrieval_review" {
			t.Fatalf("unexpected review request: %#v", req.Text)
		}
		if len(req.Input) == 0 || !strings.Contains(req.Input[0].Content[0].Text, "11111111aaaa1111") {
			t.Fatalf("candidate ids missing from prompt: %#v", req.Input)
		}
		return jsonResponse(http.StatusOK, `{"output":[{"type":"message","content":[{"type":"output_text","text":"{\"selected_ids\":[\"11111111aaaa1111\"]}"}]}]}`), nil
	})

	selected, err := service.ReviewAnswerCandidates(context.Background(), "macOS 什么时候做？", []knowledge.Entry{
		{
			ID:         "11111111aaaa1111",
			Text:       "未来需要支持 macOS。",
			RecordedAt: time.Date(2026, 3, 27, 10, 0, 0, 0, time.UTC),
		},
	})
	if err != nil {
		t.Fatalf("review candidates: %v", err)
	}
	if len(selected) != 1 || selected[0] != "11111111aaaa1111" {
		t.Fatalf("unexpected selected ids: %#v", selected)
	}
}

func TestTranslateToChinese(t *testing.T) {
	store := newConfiguredStore(t, modelconfig.Config{
		Provider: modelconfig.ProviderOpenAI,
		APIType:  modelconfig.APITypeResponses,
		BaseURL:  "http://example.invalid/v1",
		APIKey:   "secret",
		Model:    "gpt-test",
	})

	service := NewService(store)
	service.httpClient = newTestClient(t, func(r *http.Request) (*http.Response, error) {
		var req responsesRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if len(req.Input) == 0 || !strings.Contains(req.Input[0].Content[0].Text, "Puppeteer is a browser automation tool.") {
			t.Fatalf("translation source not included: %#v", req.Input)
		}
		if !strings.Contains(req.Instructions, "translation mode") {
			t.Fatalf("unexpected instructions: %q", req.Instructions)
		}
		return jsonResponse(http.StatusOK, `{"output":[{"type":"message","content":[{"type":"output_text","text":"Puppeteer 是一个浏览器自动化工具。"}]}]}`), nil
	})

	reply, err := service.TranslateToChinese(context.Background(), "Puppeteer is a browser automation tool.")
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	if !strings.Contains(reply, "浏览器自动化工具") {
		t.Fatalf("unexpected reply: %q", reply)
	}
}

func TestTranslateToChineseIncludesSkillContext(t *testing.T) {
	store := newConfiguredStore(t, modelconfig.Config{
		Provider: modelconfig.ProviderOpenAI,
		APIType:  modelconfig.APITypeResponses,
		BaseURL:  "http://example.invalid/v1",
		APIKey:   "secret",
		Model:    "gpt-test",
	})

	service := NewService(store)
	service.httpClient = newTestClient(t, func(r *http.Request) (*http.Response, error) {
		var req responsesRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if !strings.Contains(req.Instructions, "Loaded skills") {
			t.Fatalf("expected skill context in instructions, got %q", req.Instructions)
		}
		return jsonResponse(http.StatusOK, `{"output":[{"type":"message","content":[{"type":"output_text","text":"你好"}]}]}`), nil
	})

	ctx := WithSkillContext(context.Background(), "Loaded skills\n\n## writer\nUse concise writing.")
	reply, err := service.TranslateToChinese(ctx, "hello")
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	if reply != "你好" {
		t.Fatalf("unexpected reply: %q", reply)
	}
}

func TestSummarizeImageFileUsesVisionInput(t *testing.T) {
	store := newConfiguredStore(t, modelconfig.Config{
		Provider: modelconfig.ProviderOpenAI,
		APIType:  modelconfig.APITypeResponses,
		BaseURL:  "http://example.invalid/v1",
		APIKey:   "secret",
		Model:    "gpt-test",
	})

	service := NewService(store)
	service.httpClient = newTestClient(t, func(r *http.Request) (*http.Response, error) {
		var req responsesRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if len(req.Input) == 0 || len(req.Input[0].Content) < 2 {
			t.Fatalf("unexpected input: %#v", req.Input)
		}
		if req.Input[0].Content[1].Type != "input_image" {
			t.Fatalf("expected input_image, got %#v", req.Input[0].Content)
		}
		return jsonResponse(http.StatusOK, `{"output":[{"type":"message","content":[{"type":"output_text","text":"- 图像摘要"}]}]}`), nil
	})

	reply, err := service.SummarizeImageFile(context.Background(), "sample.png", "data:image/png;base64,AAAA")
	if err != nil {
		t.Fatalf("summarize image: %v", err)
	}
	if !strings.Contains(reply, "图像摘要") {
		t.Fatalf("unexpected reply: %q", reply)
	}
}

func TestAnthropicSummarizeImageFileUsesBase64ImageSource(t *testing.T) {
	store := newConfiguredStore(t, modelconfig.Config{
		Provider: modelconfig.ProviderAnthropic,
		APIType:  modelconfig.APITypeMessages,
		BaseURL:  "http://example.invalid/v1",
		APIKey:   "anthropic-secret",
		Model:    "claude-3-5-sonnet-latest",
	})

	service := NewService(store)
	service.httpClient = newTestClient(t, func(r *http.Request) (*http.Response, error) {
		var req anthropicMessagesRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if len(req.Messages) == 0 || len(req.Messages[0].Content) < 2 {
			t.Fatalf("unexpected anthropic request: %#v", req.Messages)
		}
		imagePart := req.Messages[0].Content[1]
		if imagePart.Type != "image" || imagePart.Source == nil {
			t.Fatalf("expected anthropic image block, got %#v", imagePart)
		}
		if imagePart.Source.MediaType != "image/png" || imagePart.Source.Data != "AAAA" {
			t.Fatalf("unexpected image source: %#v", imagePart.Source)
		}
		return jsonResponse(http.StatusOK, `{"content":[{"type":"text","text":"- Anthropic 图像摘要"}]}`), nil
	})

	reply, err := service.SummarizeImageFile(context.Background(), "sample.png", "data:image/png;base64,AAAA")
	if err != nil {
		t.Fatalf("summarize image via anthropic: %v", err)
	}
	if !strings.Contains(reply, "Anthropic 图像摘要") {
		t.Fatalf("unexpected reply: %q", reply)
	}
}

func TestSummarizePDFTextUsesExtractedText(t *testing.T) {
	store := newConfiguredStore(t, modelconfig.Config{
		Provider: modelconfig.ProviderOpenAI,
		APIType:  modelconfig.APITypeResponses,
		BaseURL:  "http://example.invalid/v1",
		APIKey:   "secret",
		Model:    "gpt-test",
	})

	service := NewService(store)
	service.httpClient = newTestClient(t, func(r *http.Request) (*http.Response, error) {
		var req responsesRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if len(req.Input) == 0 || len(req.Input[0].Content) < 1 {
			t.Fatalf("unexpected input: %#v", req.Input)
		}
		if req.Input[0].Content[0].Type != "input_text" {
			t.Fatalf("expected input_text, got %#v", req.Input[0].Content)
		}
		if !strings.Contains(req.Input[0].Content[0].Text, "Puppeteer PDF full text") {
			t.Fatalf("expected extracted pdf text in prompt, got %#v", req.Input[0].Content[0])
		}
		return jsonResponse(http.StatusOK, `{"output":[{"type":"message","content":[{"type":"output_text","text":"- PDF 摘要"}]}]}`), nil
	})

	reply, err := service.SummarizePDFText(context.Background(), "sample.pdf", "Puppeteer PDF full text")
	if err != nil {
		t.Fatalf("summarize pdf: %v", err)
	}
	if !strings.Contains(reply, "PDF 摘要") {
		t.Fatalf("unexpected reply: %q", reply)
	}
}

func TestCreateResponseReturnsAPIErrors(t *testing.T) {
	store := newConfiguredStore(t, modelconfig.Config{
		Provider: modelconfig.ProviderOpenAI,
		APIType:  modelconfig.APITypeResponses,
		BaseURL:  "http://example.invalid/v1",
		APIKey:   "secret",
		Model:    "gpt-test",
	})

	service := NewService(store)
	service.httpClient = newTestClient(t, func(*http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusUnauthorized, `{"error":{"message":"bad key"}}`), nil
	})

	_, err := service.TestConnection(context.Background())
	if err == nil || !strings.Contains(err.Error(), "bad key") {
		t.Fatalf("expected api error, got %v", err)
	}
}
