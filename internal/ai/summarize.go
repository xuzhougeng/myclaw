package ai

import (
	"context"
	"fmt"
	"strings"

	"baize/internal/modelconfig"
)

const pdfSummaryChunkRunes = 12000

func (s *Service) SummarizePDFText(ctx context.Context, fileName, extractedText string) (string, error) {
	cfg, err := s.requireConfig(ctx)
	if err != nil {
		return "", err
	}

	extractedText = strings.TrimSpace(extractedText)
	if extractedText == "" {
		return "", fmt.Errorf("empty pdf text")
	}

	chunks := splitRunes(extractedText, pdfSummaryChunkRunes)
	if len(chunks) == 1 {
		return s.summarizePDFChunk(ctx, cfg, fileName, chunks[0], 1, 1)
	}

	partials := make([]string, 0, len(chunks))
	for index, chunk := range chunks {
		summary, err := s.summarizePDFChunk(ctx, cfg, fileName, chunk, index+1, len(chunks))
		if err != nil {
			return "", err
		}
		partials = append(partials, summary)
	}

	var prompt strings.Builder
	prompt.WriteString("文件名：")
	prompt.WriteString(fileName)
	prompt.WriteString("\n\n以下是分段摘要，请合并成一个适合后续快速检索的中文 Markdown 摘要：\n")
	for index, summary := range partials {
		prompt.WriteString(fmt.Sprintf("## 分段 %d\n%s\n\n", index+1, strings.TrimSpace(summary)))
	}

	instructions := strings.TrimSpace(`
You are consolidating PDF chunk summaries for a personal knowledge base.
Return concise, retrieval-friendly Simplified Chinese Markdown.
Focus on topic, key points, entities, dates, technical details, and searchable keywords.
Do not mention chunking or model behavior.
`)

	return s.generateText(ctx, cfg, instructions, prompt.String())
}

func (s *Service) SummarizeImageFile(ctx context.Context, fileName, imageURL string) (string, error) {
	cfg, err := s.requireConfig(ctx)
	if err != nil {
		return "", err
	}

	instructions := strings.TrimSpace(`
You are summarizing an image for a personal knowledge base.
Return concise, retrieval-friendly Simplified Chinese Markdown.
Describe the main subject, visible text, scene, objects, UI elements, and searchable keywords.
Do not mention token limits or model behavior.
`)

	content := []responseContentInput{
		{
			Type: "input_text",
			Text: fmt.Sprintf("请总结这张图片，文件名是 %s。输出适合后续快速检索的中文 Markdown 摘要。", fileName),
		},
		{
			Type:     "input_image",
			ImageURL: imageURL,
			Detail:   "high",
		},
	}
	return s.generateTextFromContent(ctx, cfg, instructions, content)
}

func (s *Service) summarizePDFChunk(ctx context.Context, cfg modelconfig.Config, fileName, chunk string, index, total int) (string, error) {
	instructions := strings.TrimSpace(`
You are summarizing extracted PDF text for a personal knowledge base.
Return concise, retrieval-friendly Simplified Chinese Markdown.
Focus on topic, key points, entities, dates, technical details, and searchable keywords.
Do not mention model behavior.
`)

	var prompt strings.Builder
	prompt.WriteString("文件名：")
	prompt.WriteString(fileName)
	if total > 1 {
		prompt.WriteString(fmt.Sprintf("\n分段：%d/%d", index, total))
	}
	prompt.WriteString("\n\n以下是从 PDF 提取的全文文本，请总结成适合快速检索的中文 Markdown：\n\n")
	prompt.WriteString(chunk)

	return s.generateText(ctx, cfg, instructions, prompt.String())
}
