package knowledge

import "baize/internal/toolcontract"

const (
	ToolFamilyKey   = "knowledge"
	ToolFamilyTitle = "知识库"

	SearchToolName   = "knowledge_search"
	RememberToolName = "remember"
	AppendToolName   = "append_knowledge"
	ForgetToolName   = "forget_knowledge"
)

func AgentToolContracts() []toolcontract.Spec {
	return []toolcontract.Spec{
		SearchToolContract(),
		RememberToolContract(),
		AppendToolContract(),
		ForgetToolContract(),
	}
}

func SearchToolContract() toolcontract.Spec {
	return toolcontract.Spec{
		Name:              SearchToolName,
		FamilyKey:         ToolFamilyKey,
		FamilyTitle:       ToolFamilyTitle,
		DisplayTitle:      "检索知识",
		DisplayOrder:      40,
		Purpose:           "Search the current project's knowledge base for relevant entries.",
		Description:       "Retrieve the most relevant knowledge entries from the active project.",
		InputContract:     `Provide {"query":"..."}.`,
		OutputContract:    "Returns a ranked plain-text list of matched knowledge entries.",
		Usage:             "Use when the user asks what is already known before answering directly.",
		InputJSONExample:  `{"query":"macOS 计划"}`,
		OutputJSONExample: "\"命中 2 条知识：\\n1. #6d2d7724 score=12 未来要支持 macOS\\n2. #a1b2c3d4 score=9 Windows 版本先做微信接口\"",
	}
}

func RememberToolContract() toolcontract.Spec {
	return toolcontract.Spec{
		Name:              RememberToolName,
		FamilyKey:         ToolFamilyKey,
		FamilyTitle:       ToolFamilyTitle,
		DisplayTitle:      "保存知识",
		DisplayOrder:      50,
		Purpose:           "Save a new knowledge entry in the current project.",
		Description:       "Create one new knowledge record under the active project.",
		InputContract:     `Provide {"text":"..."}.`,
		OutputContract:    "Returns the created knowledge ID and preview text.",
		Usage:             "Use only when the user explicitly wants to save new information.",
		InputJSONExample:  `{"text":"未来要支持 macOS"}`,
		OutputJSONExample: "\"已记住 #6d2d7724\\n未来要支持 macOS\"",
	}
}

func AppendToolContract() toolcontract.Spec {
	return toolcontract.Spec{
		Name:              AppendToolName,
		FamilyKey:         ToolFamilyKey,
		FamilyTitle:       ToolFamilyTitle,
		DisplayTitle:      "补充知识",
		DisplayOrder:      60,
		Purpose:           "Append content to an existing knowledge entry by ID or prefix.",
		Description:       "Extend one existing knowledge record with new text.",
		InputContract:     `Provide {"id":"...","text":"..."}.`,
		OutputContract:    "Returns the updated knowledge entry preview.",
		Usage:             "Use only when the target knowledge entry is already known.",
		InputJSONExample:  `{"id":"6d2d7724","text":"补充说明"}`,
		OutputJSONExample: "\"已补充知识 #6d2d7724\\n未来要支持 macOS\\n\\n补充说明\"",
	}
}

func ForgetToolContract() toolcontract.Spec {
	return toolcontract.Spec{
		Name:              ForgetToolName,
		FamilyKey:         ToolFamilyKey,
		FamilyTitle:       ToolFamilyTitle,
		DisplayTitle:      "删除知识",
		DisplayOrder:      70,
		Purpose:           "Delete a knowledge entry by ID or prefix.",
		Description:       "Remove one knowledge record from the active project.",
		InputContract:     `Provide {"id":"..."}.`,
		OutputContract:    "Returns deletion confirmation for the removed knowledge entry.",
		Usage:             "Use only when the user explicitly asks to delete stored knowledge.",
		InputJSONExample:  `{"id":"6d2d7724"}`,
		OutputJSONExample: "\"已删除知识 #6d2d7724\\n未来要支持 macOS\"",
	}
}
