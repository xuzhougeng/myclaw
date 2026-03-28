package ai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"myclaw/internal/knowledge"
	"myclaw/internal/modelconfig"
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

func TestRouteCommand(t *testing.T) {
	store := modelconfig.NewStore()
	t.Setenv("MYCLAW_MODEL_PROVIDER", "openai")
	t.Setenv("MYCLAW_MODEL_BASE_URL", "http://example.invalid/v1")
	t.Setenv("MYCLAW_MODEL_API_KEY", "secret")
	t.Setenv("MYCLAW_MODEL_NAME", "gpt-test")

	service := NewService(store)
	service.httpClient = newTestClient(t, func(r *http.Request) (*http.Response, error) {
		var req responsesRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.Text == nil || req.Text.Format.Type != "json_schema" {
			t.Fatalf("expected json schema request, got %#v", req.Text)
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

func TestAnswerUsesKnowledgeEntries(t *testing.T) {
	store := modelconfig.NewStore()
	t.Setenv("MYCLAW_MODEL_PROVIDER", "openai")
	t.Setenv("MYCLAW_MODEL_BASE_URL", "http://example.invalid/v1")
	t.Setenv("MYCLAW_MODEL_API_KEY", "secret")
	t.Setenv("MYCLAW_MODEL_NAME", "gpt-test")

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
	store := modelconfig.NewStore()
	t.Setenv("MYCLAW_MODEL_PROVIDER", "openai")
	t.Setenv("MYCLAW_MODEL_BASE_URL", "http://example.invalid/v1")
	t.Setenv("MYCLAW_MODEL_API_KEY", "secret")
	t.Setenv("MYCLAW_MODEL_NAME", "gpt-test")

	service := NewService(store)
	service.httpClient = newTestClient(t, func(r *http.Request) (*http.Response, error) {
		var req responsesRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.Text == nil || req.Text.Format.Type != "json_schema" {
			t.Fatalf("expected json schema request, got %#v", req.Text)
		}
		if req.Text.Format.Name != "search_plan" {
			t.Fatalf("unexpected schema name: %#v", req.Text.Format)
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

func TestReviewAnswerCandidates(t *testing.T) {
	store := modelconfig.NewStore()
	t.Setenv("MYCLAW_MODEL_PROVIDER", "openai")
	t.Setenv("MYCLAW_MODEL_BASE_URL", "http://example.invalid/v1")
	t.Setenv("MYCLAW_MODEL_API_KEY", "secret")
	t.Setenv("MYCLAW_MODEL_NAME", "gpt-test")

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
	store := modelconfig.NewStore()
	t.Setenv("MYCLAW_MODEL_PROVIDER", "openai")
	t.Setenv("MYCLAW_MODEL_BASE_URL", "http://example.invalid/v1")
	t.Setenv("MYCLAW_MODEL_API_KEY", "secret")
	t.Setenv("MYCLAW_MODEL_NAME", "gpt-test")

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
	store := modelconfig.NewStore()
	t.Setenv("MYCLAW_MODEL_PROVIDER", "openai")
	t.Setenv("MYCLAW_MODEL_BASE_URL", "http://example.invalid/v1")
	t.Setenv("MYCLAW_MODEL_API_KEY", "secret")
	t.Setenv("MYCLAW_MODEL_NAME", "gpt-test")

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
	store := modelconfig.NewStore()
	t.Setenv("MYCLAW_MODEL_PROVIDER", "openai")
	t.Setenv("MYCLAW_MODEL_BASE_URL", "http://example.invalid/v1")
	t.Setenv("MYCLAW_MODEL_API_KEY", "secret")
	t.Setenv("MYCLAW_MODEL_NAME", "gpt-test")

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

func TestSummarizePDFTextUsesExtractedText(t *testing.T) {
	store := modelconfig.NewStore()
	t.Setenv("MYCLAW_MODEL_PROVIDER", "openai")
	t.Setenv("MYCLAW_MODEL_BASE_URL", "http://example.invalid/v1")
	t.Setenv("MYCLAW_MODEL_API_KEY", "secret")
	t.Setenv("MYCLAW_MODEL_NAME", "gpt-test")

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
	store := modelconfig.NewStore()
	t.Setenv("MYCLAW_MODEL_PROVIDER", "openai")
	t.Setenv("MYCLAW_MODEL_BASE_URL", "http://example.invalid/v1")
	t.Setenv("MYCLAW_MODEL_API_KEY", "secret")
	t.Setenv("MYCLAW_MODEL_NAME", "gpt-test")

	service := NewService(store)
	service.httpClient = newTestClient(t, func(*http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusUnauthorized, `{"error":{"message":"bad key"}}`), nil
	})

	_, err := service.TestConnection(context.Background())
	if err == nil || !strings.Contains(err.Error(), "bad key") {
		t.Fatalf("expected api error, got %v", err)
	}
}
