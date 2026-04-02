package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"baize/internal/knowledge"
)

func (p *localAgentToolProvider) knowledgeToolSet() localToolSet {
	return newLocalToolSet(
		knowledge.AgentToolContracts(),
		map[string]ToolSideEffectLevel{
			knowledge.SearchToolName:   ToolSideEffectReadOnly,
			knowledge.RememberToolName: ToolSideEffectSoftWrite,
			knowledge.AppendToolName:   ToolSideEffectSoftWrite,
			knowledge.ForgetToolName:   ToolSideEffectDestructive,
		},
		map[string]localToolHandler{
			knowledge.SearchToolName:   p.executeKnowledgeSearch,
			knowledge.RememberToolName: p.executeRemember,
			knowledge.AppendToolName:   p.executeAppendKnowledge,
			knowledge.ForgetToolName:   p.executeForgetKnowledge,
		},
		nil,
	)
}

func (p *localAgentToolProvider) executeKnowledgeSearch(ctx context.Context, _ MessageContext, rawInput string) (string, error) {
	var args struct {
		Query string `json:"query"`
	}
	if err := decodeAgentToolInput(rawInput, &args); err != nil {
		return "", err
	}
	args.Query = strings.TrimSpace(args.Query)
	if args.Query == "" {
		return "", fmt.Errorf("%s requires query", knowledge.SearchToolName)
	}
	results, err := p.service.searchCandidates(ctx, []string{args.Query}, nil, retrievalCandidateLimit)
	if err != nil {
		return "", err
	}
	if len(results) == 0 {
		return "没有命中的知识。", nil
	}
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("命中 %d 条知识：\n", len(results)))
	for index, result := range results {
		builder.WriteString(fmt.Sprintf("%d. #%s score=%d %s\n",
			index+1,
			shortID(result.Entry.ID),
			result.Score,
			preview(result.Entry.Text, maxReplyPreviewRunes),
		))
	}
	return strings.TrimSpace(builder.String()), nil
}

func (p *localAgentToolProvider) executeRemember(ctx context.Context, mc MessageContext, rawInput string) (string, error) {
	var args struct {
		Text string `json:"text"`
	}
	if err := decodeAgentToolInput(rawInput, &args); err != nil {
		return "", err
	}
	args.Text = strings.TrimSpace(args.Text)
	if args.Text == "" {
		return "", fmt.Errorf("%s requires text", knowledge.RememberToolName)
	}
	entry, err := p.service.store.Add(ctx, knowledge.Entry{
		Text:       args.Text,
		Source:     sourceLabel(mc),
		RecordedAt: time.Now(),
	})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("已记住 #%s\n%s", shortID(entry.ID), preview(entry.Text, maxReplyPreviewRunes)), nil
}

func (p *localAgentToolProvider) executeAppendKnowledge(ctx context.Context, _ MessageContext, rawInput string) (string, error) {
	var args struct {
		ID   string `json:"id"`
		Text string `json:"text"`
	}
	if err := decodeAgentToolInput(rawInput, &args); err != nil {
		return "", err
	}
	return p.service.appendKnowledge(ctx, args.ID, args.Text)
}

func (p *localAgentToolProvider) executeForgetKnowledge(ctx context.Context, _ MessageContext, rawInput string) (string, error) {
	var args struct {
		ID string `json:"id"`
	}
	if err := decodeAgentToolInput(rawInput, &args); err != nil {
		return "", err
	}
	return p.service.forgetKnowledge(ctx, args.ID)
}
