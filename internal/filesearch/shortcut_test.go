package filesearch

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestShortcutHandlerSearchAndSelection(t *testing.T) {
	t.Parallel()

	paths := []string{
		`E:\xwechat_files\a.pdf`,
		`E:\xwechat_files\b.pdf`,
	}
	handler := NewShortcutHandler("es.exe", func(_ context.Context, everythingPath string, input ToolInput) (ToolResult, error) {
		if everythingPath != "es.exe" {
			t.Fatalf("unexpected everything path: %q", everythingPath)
		}
		if input.Query != "单细胞" {
			t.Fatalf("unexpected query: %q", input.Query)
		}
		if input.Limit != DefaultLimit {
			t.Fatalf("unexpected limit: %d", input.Limit)
		}
		return ToolResult{
			Tool:  ToolName,
			Query: input.Query,
			Limit: input.Limit,
			Count: len(paths),
			Items: []ResultItem{
				{Index: 1, Name: "a.pdf", Path: paths[0]},
				{Index: 2, Name: "b.pdf", Path: paths[1]},
			},
		}, nil
	})

	resp, err := handler.Handle(context.Background(), ShortcutRequest{
		SlotKey: "weixin:ctx-1",
		Text:    "/find 单细胞",
	})
	if err != nil {
		t.Fatalf("search file: %v", err)
	}
	if !resp.Handled {
		t.Fatal("expected /find to be handled")
	}
	if !strings.Contains(resp.Reply, "找到 2 个文件") || !strings.Contains(resp.Reply, `E:\xwechat_files\b.pdf`) {
		t.Fatalf("unexpected search reply: %q", resp.Reply)
	}

	var sentPath string
	resp, err = handler.Handle(context.Background(), ShortcutRequest{
		SlotKey: "weixin:ctx-1",
		Text:    "2",
		SendSelectedFile: func(_ context.Context, path string) error {
			sentPath = path
			return nil
		},
	})
	if err != nil {
		t.Fatalf("select file: %v", err)
	}
	if !resp.Handled {
		t.Fatal("expected selection to be handled")
	}
	if sentPath != paths[1] {
		t.Fatalf("unexpected sent path: %q", sentPath)
	}
	if !strings.Contains(resp.Reply, "已发送文件 2") {
		t.Fatalf("unexpected selection reply: %q", resp.Reply)
	}
	if _, ok := handler.PendingSelection("weixin:ctx-1"); ok {
		t.Fatal("expected pending selection to be cleared")
	}
}

func TestShortcutHandlerNaturalLanguageUsesResolver(t *testing.T) {
	t.Parallel()

	handler := NewShortcutHandler("es.exe", func(_ context.Context, _ string, input ToolInput) (ToolResult, error) {
		if input.Query != "d: ext:pdf 单细胞" {
			t.Fatalf("unexpected query: %q", input.Query)
		}
		if input.Limit != DefaultLimit {
			t.Fatalf("unexpected limit: %d", input.Limit)
		}
		return ToolResult{
			Tool:  ToolName,
			Query: input.Query,
			Limit: input.Limit,
			Count: 1,
			Items: []ResultItem{
				{Index: 1, Name: "单细胞报告.pdf", Path: `D:\docs\单细胞报告.pdf`},
			},
		}, nil
	})

	resp, err := handler.Handle(context.Background(), ShortcutRequest{
		SlotKey: "weixin:ctx-1",
		Text:    "查找 D 盘单细胞相关的PDF文件",
		ResolveIntent: func(_ context.Context, text string) (ToolInput, bool, error) {
			if text != "查找 D 盘单细胞相关的PDF文件" {
				t.Fatalf("unexpected text: %q", text)
			}
			return ToolInput{Query: "d: ext:pdf 单细胞"}, true, nil
		},
	})
	if err != nil {
		t.Fatalf("natural search: %v", err)
	}
	if !resp.Handled {
		t.Fatal("expected natural language file find to be handled")
	}
	if !strings.Contains(resp.Reply, "检索式: d: ext:pdf 单细胞") {
		t.Fatalf("unexpected reply: %q", resp.Reply)
	}
}

func TestShortcutHandlerFindHelpReturnsModuleHelp(t *testing.T) {
	t.Parallel()

	handler := NewShortcutHandler("es.exe", nil)
	resp, err := handler.Handle(context.Background(), ShortcutRequest{
		SlotKey: "weixin:ctx-1",
		Text:    "/find help",
	})
	if err != nil {
		t.Fatalf("find help: %v", err)
	}
	if !resp.Handled {
		t.Fatal("expected /find help to be handled")
	}
	if !strings.Contains(resp.Reply, ToolName) || !strings.Contains(resp.Reply, "/find help") {
		t.Fatalf("unexpected help reply: %q", resp.Reply)
	}
}

func TestShortcutHandlerSlashFindUsesResolverForNaturalLanguageQuery(t *testing.T) {
	t.Parallel()

	handler := NewShortcutHandler("es.exe", func(_ context.Context, _ string, input ToolInput) (ToolResult, error) {
		if input.Query != "file: shell:Downloads *.pdf" {
			t.Fatalf("unexpected query: %q", input.Query)
		}
		return ToolResult{
			Tool:  ToolName,
			Query: input.Query,
			Limit: input.Limit,
			Count: 1,
			Items: []ResultItem{
				{Index: 1, Name: "单细胞.pdf", Path: `C:\Users\demo\Downloads\单细胞.pdf`},
			},
		}, nil
	})

	resp, err := handler.Handle(context.Background(), ShortcutRequest{
		SlotKey: "weixin:ctx-1",
		Text:    "/find 查找下载目录下的pdf文件",
		ResolveIntent: func(_ context.Context, text string) (ToolInput, bool, error) {
			if text != "查找下载目录下的pdf文件" {
				t.Fatalf("unexpected resolver text: %q", text)
			}
			return ToolInput{Query: "file: shell:Downloads *.pdf"}, true, nil
		},
	})
	if err != nil {
		t.Fatalf("slash natural search: %v", err)
	}
	if !resp.Handled {
		t.Fatal("expected /find natural language file find to be handled")
	}
	if !strings.Contains(resp.Reply, "检索式: file: shell:Downloads *.pdf") {
		t.Fatalf("unexpected reply: %q", resp.Reply)
	}
}

func TestShortcutHandlerMapsSetupErrorsToReplies(t *testing.T) {
	t.Parallel()

	handler := NewShortcutHandler("", func(context.Context, string, ToolInput) (ToolResult, error) {
		return ToolResult{}, ErrUnconfigured
	})

	resp, err := handler.Handle(context.Background(), ShortcutRequest{
		SlotKey: "weixin:ctx-1",
		Text:    "/find output.csv",
	})
	if err != nil {
		t.Fatalf("handle unconfigured search: %v", err)
	}
	if !resp.Handled || !strings.Contains(resp.Reply, ErrUnconfigured.Error()) {
		t.Fatalf("unexpected response: %#v", resp)
	}
}

func TestShortcutHandlerReturnsSearchErrors(t *testing.T) {
	t.Parallel()

	searchErr := errors.New("boom")
	handler := NewShortcutHandler("es.exe", func(context.Context, string, ToolInput) (ToolResult, error) {
		return ToolResult{}, searchErr
	})

	resp, err := handler.Handle(context.Background(), ShortcutRequest{
		SlotKey: "weixin:ctx-1",
		Text:    "/find output.csv",
	})
	if !resp.Handled {
		t.Fatalf("expected handled response, got %#v", resp)
	}
	if !errors.Is(err, searchErr) {
		t.Fatalf("expected %v, got %v", searchErr, err)
	}
}
