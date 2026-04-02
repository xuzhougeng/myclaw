package reminder

import "baize/internal/toolcontract"

const (
	ToolFamilyKey   = "reminder"
	ToolFamilyTitle = "提醒"

	ListToolName   = "reminder_list"
	AddToolName    = "reminder_add"
	RemoveToolName = "reminder_remove"
)

func AgentToolContracts() []toolcontract.Spec {
	return []toolcontract.Spec{
		ListToolContract(),
		AddToolContract(),
		RemoveToolContract(),
	}
}

func ListToolContract() toolcontract.Spec {
	return toolcontract.Spec{
		Name:              ListToolName,
		FamilyKey:         ToolFamilyKey,
		FamilyTitle:       ToolFamilyTitle,
		DisplayTitle:      "查看提醒",
		DisplayOrder:      80,
		Purpose:           "List reminders visible from the current runtime context.",
		Description:       "Read the reminder list for the current conversation target; desktop primary may aggregate reminders from other interfaces.",
		InputContract:     `Provide {}.`,
		OutputContract:    "Returns the current reminder list in plain text.",
		Usage:             "Use when the user asks what reminders are currently scheduled.",
		InputJSONExample:  `{}`,
		OutputJSONExample: "\"当前有 2 个提醒：\\n1. [单次] 明天 09:30 开会\\n2. [每天] 每天 09:00 写日报\"",
	}
}

func AddToolContract() toolcontract.Spec {
	return toolcontract.Spec{
		Name:              AddToolName,
		FamilyKey:         ToolFamilyKey,
		FamilyTitle:       ToolFamilyTitle,
		DisplayTitle:      "创建提醒",
		DisplayOrder:      90,
		Purpose:           "Create a reminder using the same syntax as /notice.",
		Description:       "Schedule a new reminder for the current interface and user.",
		InputContract:     `Provide {"spec":"..."}.`,
		OutputContract:    "Returns the created reminder summary and next run time.",
		Usage:             "Use when the user clearly provides reminder timing and message.",
		InputJSONExample:  `{"spec":"2小时后 喝水"}`,
		OutputJSONExample: "\"已创建提醒 #abcd1234\\n时间: 2026-04-01 14:00:00\\n内容: 喝水\"",
	}
}

func RemoveToolContract() toolcontract.Spec {
	return toolcontract.Spec{
		Name:              RemoveToolName,
		FamilyKey:         ToolFamilyKey,
		FamilyTitle:       ToolFamilyTitle,
		DisplayTitle:      "删除提醒",
		DisplayOrder:      100,
		Purpose:           "Delete a reminder by ID or prefix.",
		Description:       "Remove one reminder from the current visible reminder scope.",
		InputContract:     `Provide {"id":"..."}.`,
		OutputContract:    "Returns deletion confirmation for the removed reminder.",
		Usage:             "Use only when the user explicitly identifies a reminder to remove.",
		InputJSONExample:  `{"id":"abcd1234"}`,
		OutputJSONExample: "\"已删除提醒 #abcd1234\\n内容: 喝水\"",
	}
}
