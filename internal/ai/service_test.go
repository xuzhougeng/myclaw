package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"myclaw/internal/knowledge"
	"myclaw/internal/modelconfig"
)

func TestRouteCommand(t *testing.T) {
	store := modelconfig.NewStore()
	t.Setenv("MYCLAW_MODEL_PROVIDER", "openai")
	t.Setenv("MYCLAW_MODEL_BASE_URL", "http://example.invalid/v1")
	t.Setenv("MYCLAW_MODEL_API_KEY", "secret")
	t.Setenv("MYCLAW_MODEL_NAME", "gpt-test")

	service := NewService(store)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req responsesRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.Text == nil || req.Text.Format.Type != "json_schema" {
			t.Fatalf("expected json schema request, got %#v", req.Text)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"output":[{"type":"message","content":[{"type":"output_text","text":"{\"command\":\"remember\",\"memory_text\":\"- 已整理内容\",\"append_text\":\"\",\"knowledge_id\":\"\",\"reminder_spec\":\"\",\"reminder_id\":\"\",\"question\":\"\"}"}]}]}`))
	}))
	defer server.Close()
	service.httpClient = server.Client()

	t.Setenv("MYCLAW_MODEL_BASE_URL", server.URL)

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

func TestAnswerUsesKnowledgeEntries(t *testing.T) {
	store := modelconfig.NewStore()
	t.Setenv("MYCLAW_MODEL_PROVIDER", "openai")
	t.Setenv("MYCLAW_MODEL_BASE_URL", "http://example.invalid/v1")
	t.Setenv("MYCLAW_MODEL_API_KEY", "secret")
	t.Setenv("MYCLAW_MODEL_NAME", "gpt-test")

	service := NewService(store)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req responsesRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if len(req.Input) == 0 || !strings.Contains(req.Input[0].Content[0].Text, "未来要支持 macOS") {
			t.Fatalf("knowledge not included in prompt: %#v", req.Input)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"output":[{"type":"message","content":[{"type":"output_text","text":"可以，知识库里提到未来要支持 macOS。"}]}]}`))
	}))
	defer server.Close()
	service.httpClient = server.Client()

	t.Setenv("MYCLAW_MODEL_BASE_URL", server.URL)

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

func TestTranslateToChinese(t *testing.T) {
	store := modelconfig.NewStore()
	t.Setenv("MYCLAW_MODEL_PROVIDER", "openai")
	t.Setenv("MYCLAW_MODEL_BASE_URL", "http://example.invalid/v1")
	t.Setenv("MYCLAW_MODEL_API_KEY", "secret")
	t.Setenv("MYCLAW_MODEL_NAME", "gpt-test")

	service := NewService(store)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"output":[{"type":"message","content":[{"type":"output_text","text":"Puppeteer 是一个浏览器自动化工具。"}]}]}`))
	}))
	defer server.Close()
	service.httpClient = server.Client()

	t.Setenv("MYCLAW_MODEL_BASE_URL", server.URL)

	reply, err := service.TranslateToChinese(context.Background(), "Puppeteer is a browser automation tool.")
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	if !strings.Contains(reply, "浏览器自动化工具") {
		t.Fatalf("unexpected reply: %q", reply)
	}
}

func TestCreateResponseReturnsAPIErrors(t *testing.T) {
	store := modelconfig.NewStore()
	t.Setenv("MYCLAW_MODEL_PROVIDER", "openai")
	t.Setenv("MYCLAW_MODEL_BASE_URL", "http://example.invalid/v1")
	t.Setenv("MYCLAW_MODEL_API_KEY", "secret")
	t.Setenv("MYCLAW_MODEL_NAME", "gpt-test")

	service := NewService(store)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"bad key"}}`))
	}))
	defer server.Close()
	service.httpClient = server.Client()

	t.Setenv("MYCLAW_MODEL_BASE_URL", server.URL)

	_, err := service.TestConnection(context.Background())
	if err == nil || !strings.Contains(err.Error(), "bad key") {
		t.Fatalf("expected api error, got %v", err)
	}
}
