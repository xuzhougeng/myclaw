package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"myclaw/internal/filesearch"
	"myclaw/internal/knowledge"
	"myclaw/internal/reminder"
	"myclaw/internal/toolcontract"
)

type localAgentToolProvider struct {
	service *Service
}

func newLocalAgentToolProvider(service *Service) AgentToolProvider {
	return &localAgentToolProvider{service: service}
}

func (p *localAgentToolProvider) ProviderKind() AgentToolProviderKind {
	return AgentToolProviderLocal
}

func (p *localAgentToolProvider) ProviderKey() string {
	return string(AgentToolProviderLocal)
}

func (p *localAgentToolProvider) ListAgentTools(context.Context, MessageContext) ([]AgentToolSpec, error) {
	return []AgentToolSpec{
		specWithLevel(agentToolSpecFromContract(localKnowledgeSearchContract()), ToolSideEffectReadOnly),
		specWithLevel(agentToolSpecFromContract(localRememberContract()), ToolSideEffectSoftWrite),
		specWithLevel(agentToolSpecFromContract(localAppendKnowledgeContract()), ToolSideEffectSoftWrite),
		specWithLevel(agentToolSpecFromContract(localForgetKnowledgeContract()), ToolSideEffectDestructive),
		specWithLevel(agentToolSpecFromContract(filesearch.Definition()), ToolSideEffectReadOnly),
		specWithLevel(agentToolSpecFromContract(localReminderListContract()), ToolSideEffectReadOnly),
		specWithLevel(agentToolSpecFromContract(localReminderAddContract()), ToolSideEffectSoftWrite),
		specWithLevel(agentToolSpecFromContract(localReminderRemoveContract()), ToolSideEffectDestructive),
	}, nil
}

func specWithLevel(s AgentToolSpec, level ToolSideEffectLevel) AgentToolSpec {
	s.SideEffectLevel = level
	return s
}

func (p *localAgentToolProvider) ExecuteAgentTool(ctx context.Context, mc MessageContext, toolName, rawInput string) (string, error) {
	handlers := map[string]func(context.Context, MessageContext, string) (string, error){
		"knowledge_search": p.executeKnowledgeSearch,
		"remember":         p.executeRemember,
		"append_knowledge": p.executeAppendKnowledge,
		"forget_knowledge": p.executeForgetKnowledge,
		filesearch.ToolName: p.executeFileSearch,
		"reminder_list":   p.executeReminderList,
		"reminder_add":    p.executeReminderAdd,
		"reminder_remove": p.executeReminderRemove,
	}
	name := strings.ToLower(strings.TrimSpace(toolName))
	handler, ok := handlers[name]
	if !ok {
		return "", fmt.Errorf("unknown local tool %q", toolName)
	}
	return handler(ctx, mc, rawInput)
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
		return "", fmt.Errorf("knowledge_search requires query")
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
		return "", fmt.Errorf("remember requires text")
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

func (p *localAgentToolProvider) executeFileSearch(ctx context.Context, _ MessageContext, rawInput string) (string, error) {
	var args filesearch.ToolInput
	if err := decodeAgentToolInput(rawInput, &args); err != nil {
		return "", err
	}
	result, reply, err := p.service.performFileSearch(ctx, args)
	if err != nil {
		return "", err
	}
	if reply != "" {
		return reply, nil
	}
	return filesearch.FormatSearchResult(result), nil
}

func (p *localAgentToolProvider) executeReminderList(ctx context.Context, mc MessageContext, rawInput string) (string, error) {
	if err := decodeAgentToolInput(rawInput, &struct{}{}); err != nil {
		return "", err
	}
	if p.service.reminders == nil {
		return "提醒功能未启用。", nil
	}
	items, err := p.service.reminders.List(ctx, reminder.Target{Interface: mc.Interface, UserID: mc.UserID})
	if err != nil {
		return "", err
	}
	return formatReminderList(items), nil
}

func (p *localAgentToolProvider) executeReminderAdd(ctx context.Context, mc MessageContext, rawInput string) (string, error) {
	var args struct {
		Spec string `json:"spec"`
	}
	if err := decodeAgentToolInput(rawInput, &args); err != nil {
		return "", err
	}
	if p.service.reminders == nil {
		return "提醒功能未启用。", nil
	}
	item, err := p.service.scheduleReminderSpec(ctx, reminder.Target{
		Interface: mc.Interface,
		UserID:    mc.UserID,
	}, args.Spec)
	if err != nil {
		return "", err
	}
	return formatReminderCreated(item), nil
}

func (p *localAgentToolProvider) executeReminderRemove(ctx context.Context, mc MessageContext, rawInput string) (string, error) {
	var args struct {
		ID string `json:"id"`
	}
	if err := decodeAgentToolInput(rawInput, &args); err != nil {
		return "", err
	}
	if p.service.reminders == nil {
		return "提醒功能未启用。", nil
	}
	item, ok, err := p.service.reminders.Remove(ctx, reminder.Target{Interface: mc.Interface, UserID: mc.UserID}, args.ID)
	if err != nil {
		return "", err
	}
	if !ok {
		return fmt.Sprintf("没有找到提醒 %q。", args.ID), nil
	}
	return fmt.Sprintf("已删除提醒 #%s\n内容: %s", shortID(item.ID), item.Message), nil
}

func decodeAgentToolInput(raw string, out any) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		raw = "{}"
	}
	if !strings.HasPrefix(raw, "{") {
		return fmt.Errorf("tool input must be a JSON object")
	}
	if err := json.Unmarshal([]byte(raw), out); err != nil {
		return fmt.Errorf("decode tool input: %w", err)
	}
	return nil
}

func localKnowledgeSearchContract() toolcontract.Spec {
	return toolcontract.Spec{
		Name:             "knowledge_search",
		Purpose:          "Search the current project's knowledge base for relevant entries.",
		Description:      "Retrieve the most relevant knowledge entries from the active project.",
		InputContract:    `Provide {"query":"..."}.`,
		OutputContract:   "Returns a ranked plain-text list of matched knowledge entries.",
		Usage:            "Use when the user asks what is already known before answering directly.",
		InputJSONExample: `{"query":"macOS 计划"}`,
	}
}

func localRememberContract() toolcontract.Spec {
	return toolcontract.Spec{
		Name:             "remember",
		Purpose:          "Save a new knowledge entry in the current project.",
		Description:      "Create one new knowledge record under the active project.",
		InputContract:    `Provide {"text":"..."}.`,
		OutputContract:   "Returns the created knowledge ID and preview text.",
		Usage:            "Use only when the user explicitly wants to save new information.",
		InputJSONExample: `{"text":"未来要支持 macOS"}`,
	}
}

func localAppendKnowledgeContract() toolcontract.Spec {
	return toolcontract.Spec{
		Name:             "append_knowledge",
		Purpose:          "Append content to an existing knowledge entry by ID or prefix.",
		Description:      "Extend one existing knowledge record with new text.",
		InputContract:    `Provide {"id":"...","text":"..."}.`,
		OutputContract:   "Returns the updated knowledge entry preview.",
		Usage:            "Use only when the target knowledge entry is already known.",
		InputJSONExample: `{"id":"6d2d7724","text":"补充说明"}`,
	}
}

func localForgetKnowledgeContract() toolcontract.Spec {
	return toolcontract.Spec{
		Name:             "forget_knowledge",
		Purpose:          "Delete a knowledge entry by ID or prefix.",
		Description:      "Remove one knowledge record from the active project.",
		InputContract:    `Provide {"id":"..."}.`,
		OutputContract:   "Returns deletion confirmation for the removed knowledge entry.",
		Usage:            "Use only when the user explicitly asks to delete stored knowledge.",
		InputJSONExample: `{"id":"6d2d7724"}`,
	}
}

func localReminderListContract() toolcontract.Spec {
	return toolcontract.Spec{
		Name:             "reminder_list",
		Purpose:          "List reminders for the current interface and user.",
		Description:      "Read the reminder list bound to the current conversation target.",
		InputContract:    `Provide {}.`,
		OutputContract:   "Returns the current reminder list in plain text.",
		Usage:            "Use when the user asks what reminders are currently scheduled.",
		InputJSONExample: `{}`,
	}
}

func localReminderAddContract() toolcontract.Spec {
	return toolcontract.Spec{
		Name:             "reminder_add",
		Purpose:          "Create a reminder using the same syntax as /notice.",
		Description:      "Schedule a new reminder for the current interface and user.",
		InputContract:    `Provide {"spec":"..."}.`,
		OutputContract:   "Returns the created reminder summary and next run time.",
		Usage:            "Use when the user clearly provides reminder timing and message.",
		InputJSONExample: `{"spec":"2小时后 喝水"}`,
	}
}

func localReminderRemoveContract() toolcontract.Spec {
	return toolcontract.Spec{
		Name:             "reminder_remove",
		Purpose:          "Delete a reminder by ID or prefix.",
		Description:      "Remove one reminder from the current interface and user scope.",
		InputContract:    `Provide {"id":"..."}.`,
		OutputContract:   "Returns deletion confirmation for the removed reminder.",
		Usage:            "Use only when the user explicitly identifies a reminder to remove.",
		InputJSONExample: `{"id":"abcd1234"}`,
	}
}
