package ai

import (
	"context"
	"fmt"
	"strings"

	"baize/internal/knowledge"
	"baize/internal/runtimepolicy"
)

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
				"enum": runtimepolicy.RouteDecisionCommands(),
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
You are the command router for baize.
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
- For answer, put the cleaned user question in question.
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

func (s *Service) BuildSearchPlan(ctx context.Context, question string) (SearchPlan, error) {
	cfg, err := s.requireConfig(ctx)
	if err != nil {
		return SearchPlan{}, err
	}

	schema := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"queries": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "string",
				},
			},
			"keywords": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "string",
				},
			},
		},
		"required": []string{"queries", "keywords"},
	}

	instructions := strings.TrimSpace(`
You are building a retrieval plan for a personal knowledge base.
Produce:
- queries: 1 to 3 short retrieval-oriented rewrites of the user's question
- keywords: concrete terms only when they materially help retrieval

Guidance:
- Queries may include likely answer terms, troubleshooting terms, aliases, command names, error phrases, or bilingual variants.
- Do not force keyword extraction when the question is abstract; use better retrieval queries instead.
- Prefer concise, search-friendly wording over natural conversation.
- Avoid generic filler words.
Return only JSON that matches the schema.
`)

	var plan SearchPlan
	if err := s.generateJSON(ctx, cfg, instructions, strings.TrimSpace(question), "search_plan", schema, &plan); err != nil {
		return SearchPlan{}, err
	}
	plan.Queries = normalizeSearchQueries(plan.Queries)
	plan.Keywords = knowledge.MergeKeywords(plan.Keywords)
	return plan, nil
}

func (s *Service) ReviewAnswerCandidates(ctx context.Context, question string, entries []knowledge.Entry) ([]string, error) {
	cfg, err := s.requireConfig(ctx)
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, nil
	}

	schema := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"selected_ids": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "string",
				},
			},
		},
		"required": []string{"selected_ids"},
	}

	var prompt strings.Builder
	prompt.WriteString("用户问题：\n")
	prompt.WriteString(strings.TrimSpace(question))
	prompt.WriteString("\n\n候选知识：\n")
	for index, entry := range entries {
		keywords := entry.Keywords
		if len(keywords) == 0 {
			keywords = knowledge.GenerateKeywords(entry.Text)
		}
		prompt.WriteString(fmt.Sprintf("%d. id=%s time=%s keywords=%s\n%s\n\n",
			index+1,
			entry.ID,
			entry.RecordedAt.Local().Format("2006-01-02 15:04:05"),
			strings.Join(keywords, ", "),
			trimForPrompt(entry.Text, 320),
		))
	}

	instructions := strings.TrimSpace(`
You are reviewing retrieved knowledge-base candidates for answer generation.
Select only the entries that are genuinely useful for answering the user's question.
Prefer precision over recall, but keep a small amount of supporting context when it materially helps.
Use only the provided candidates.
Return the selected full IDs in best-first order.
If none are useful, return an empty array.
Return only JSON that matches the schema.
`)

	var review struct {
		SelectedIDs []string `json:"selected_ids"`
	}
	if err := s.generateJSON(ctx, cfg, instructions, prompt.String(), "retrieval_review", schema, &review); err != nil {
		return nil, err
	}
	return review.SelectedIDs, nil
}
