package eval

import "myclaw/internal/ai"

func DemoTools() map[string]ai.ToolCapability {
	return map[string]ai.ToolCapability{
		"everything_file_search": {
			Name:        "everything_file_search",
			Purpose:     "搜索本地文件系统",
			Description: "使用 Everything 搜索引擎快速查找文件",
			InputContract: `{
  "known_folders": ["Downloads", "Desktop", "Documents"],
  "extensions": ["pdf", "docx", "xlsx"],
  "keywords": ["report", "2024"]
}`,
			OutputContract: "返回匹配的文件路径列表",
		},
		"knowledge_search": {
			Name:        "knowledge_search",
			Purpose:     "搜索知识库",
			Description: "在用户的个人知识库中搜索相关笔记",
			InputContract: `{
  "query": "MCP tool calls"
}`,
			OutputContract: "返回匹配的知识条目",
		},
		"local::knowledge_search": {
			Name:        "local::knowledge_search",
			Purpose:     "搜索知识库",
			Description: "在用户的个人知识库中搜索相关笔记",
			InputContract: `{
  "query": "MCP tool calls"
}`,
			OutputContract: "返回匹配的知识条目",
		},
		"local::everything_file_search": {
			Name:        "local::everything_file_search",
			Purpose:     "搜索本地文件系统",
			Description: "使用 Everything 搜索引擎快速查找文件",
			InputContract: `{
  "known_folders": ["Downloads"],
  "extensions": ["pdf"]
}`,
			OutputContract: "返回匹配的文件路径列表",
		},
		"local::forget_knowledge": {
			Name:        "local::forget_knowledge",
			Purpose:     "从知识库中永久删除指定条目",
			Description: "删除知识库中的一条或多条笔记，操作不可逆",
			InputContract: `{
  "id": "note-id-to-delete"
}`,
			OutputContract: "返回删除结果",
		},
	}
}
