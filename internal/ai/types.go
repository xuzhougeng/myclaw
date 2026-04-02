package ai

import "baize/internal/toolcontract"

type RouteDecision struct {
	Command      string `json:"command"`
	MemoryText   string `json:"memory_text"`
	AppendText   string `json:"append_text"`
	KnowledgeID  string `json:"knowledge_id"`
	ReminderSpec string `json:"reminder_spec"`
	ReminderID   string `json:"reminder_id"`
	Question     string `json:"question"`
}

type SearchPlan struct {
	Queries  []string `json:"queries"`
	Keywords []string `json:"keywords"`
}

type ConversationMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type CallTraceStep struct {
	Title  string `json:"title"`
	Detail string `json:"detail"`
}

type ToolCapability struct {
	Purpose           string `json:"purpose,omitempty"`
	Name              string `json:"name"`
	Description       string `json:"description"`
	InputContract     string `json:"input_contract,omitempty"`
	OutputContract    string `json:"output_contract,omitempty"`
	Usage             string `json:"usage,omitempty"`
	InputJSONExample  string `json:"input_json_example,omitempty"`
	OutputJSONExample string `json:"output_json_example,omitempty"`
}

type ToolOpportunity struct {
	ToolName string `json:"tool_name"`
	Goal     string `json:"goal"`
}

type ToolExecution struct {
	ToolName   string `json:"tool_name"`
	ToolInput  string `json:"tool_input"`
	ToolOutput string `json:"tool_output"`
}

type ToolPlanDecision struct {
	Action      string `json:"action"`
	ToolName    string `json:"tool_name"`
	ToolInput   string `json:"tool_input"`
	UserMessage string `json:"user_message"`
}

func ToolCapabilityFromContract(spec toolcontract.Spec) ToolCapability {
	spec = spec.Normalized()
	return ToolCapability{
		Name:              spec.Name,
		Purpose:           spec.Purpose,
		Description:       spec.Description,
		InputContract:     spec.InputContract,
		OutputContract:    spec.OutputContract,
		Usage:             spec.Usage,
		InputJSONExample:  spec.InputJSONExample,
		OutputJSONExample: spec.OutputJSONExample,
	}
}
