# Tool Unit 规范

本文档定义 `baize` 里“可复用 tool-style 模块”的新增要求。目标不是写概念文，而是给新增 / 改名 / 重构 tool 时一个统一落地标准。

如果你只是想知道当前仓库的总体设计思路，可以顺带参考 [独立工作单元设计规范](./work-unit-design.md)。本文更偏工程约束和提交流程。

## 适用范围

以下能力都按 tool unit 对待：

- 供 agent 直接调用的本地 tool
- 未来可能被多个接口复用的能力模块
- 有明确输入输出契约、可单独测试、可被 slash command 或 UI 薄包装的模块

以下内容通常不单独登记为 tool unit：

- 纯 transport 层逻辑，例如桌面弹窗、微信发消息、terminal 输出格式
- 单一接口专属的胶水代码
- 只作为某个更大 tool 内部实现细节的小函数

## 硬性要求

新增一个 tool unit 时，至少满足这些要求：

1. 放在独立包中。
   推荐路径是 `internal/<tool>`，不要把核心执行逻辑埋进 `internal/weixin`、`internal/terminal`、desktop UI 或某个 handler。

2. 具备稳定身份。
   至少要有稳定 `ToolName`，以及一个能输出完整契约的 `Definition()` 或等价入口。

3. 具备统一契约。
   至少要能明确说明：
   - purpose
   - description
   - input contract
   - output contract
   - usage/help
   - input JSON example
   - output JSON example

4. 输入输出结构显式化。
   使用明确的 `ToolInput` / `ToolResult` 结构，不要只靠自然语言提示词隐式约定字段。

5. 执行逻辑和运行时接入分层。
   基本分层应是：
   - `internal/ai`: 判断是否需要某类工具
   - `internal/app`: 暴露 / 编排 / 注册
   - `internal/<tool>`: 归一化和执行
   - transport/UI: 展示结果或触发副作用

6. 不决定会话生命周期。
   tool unit 不应自己决定是否新建对话、是否绑定当前会话、是否写历史。这些都属于 shared runtime policy。

7. shortcut 只是薄包装。
   如果有 `/find` 这种命令：
   - shortcut 不是核心逻辑本体
   - shortcut 不应重写 tool 执行逻辑
   - `help` 应回到 tool 自己的 usage/help
   - shortcut 应由真正拥有该能力的 runtime 注册，而不是默认全局暴露

8. 必须可测试。
   至少要覆盖契约完整性、标准执行路径，以及 runtime 注册或暴露行为。

9. 必须登记。
   新增、改名、删除 tool unit 后，要更新本文档末尾的 `Registered Tool Units`。

如果多个 tool 明显属于同一类能力，例如 reminder 的 list/add/remove 或 knowledge 的 search/remember/append/forget，建议额外补上家族元数据，至少包括：

- `FamilyKey`
- `FamilyTitle`
- `DisplayTitle`
- `DisplayOrder`

这样运行时可以按“工具家族”注册、排序和展示，而不是在 provider 或 UI 里继续靠工具名硬编码分组。

## 推荐代码骨架

一个标准 tool unit 推荐至少包含这些元素：

```go
package mytool

const ToolName = "my_tool"

type ToolInput struct {
  // ...
}

type ToolResult struct {
  // ...
}

func Definition() toolcontract.Spec {
  return toolcontract.Spec{
    Name:              ToolName,
    Purpose:           "...",
    Description:       "...",
    InputContract:     "...",
    OutputContract:    "...",
    InputJSONExample:  `{"...":"..."}`,
    OutputJSONExample: `{"tool":"my_tool"}`,
    Usage:             UsageText(),
  }
}

func UsageText() string {
  return "..."
}

func NormalizeInput(raw ToolInput) ToolInput {
  // optional but strongly recommended
}

func Execute(ctx context.Context, input ToolInput) (ToolResult, error) {
  // ...
}

// FormatResult is an optional helper; renders ToolResult as human-readable text.
// Used by transport layers (terminal, desktop) for display; not required by the agent loop.
func FormatResult(result ToolResult) (string, error) {
  // ...
}
```

如果工具需要平台或接口限制，也建议把限制函数显式化，例如：

- `AllowedForInterface(name string) bool`
- `SupportedForCurrentPlatform() bool`

但对 desktop / weixin 的 agent tool 暴露有一个额外约束：

- 默认不要因为接口名是 `weixin` 就隐藏工具
- desktop 和 weixin 的 `ListAgentToolDefinitions` 应默认保持一致
- 如果必须收窄候选集，理由应是平台不支持、用户显式关闭，或真正的安全/权限边界，而不是 transport 偏好

## 新建 Tool Checklist

提交一个新 tool 前，逐项确认：

- 已创建独立包 `internal/<tool>`
- 已定义稳定 `ToolName`
- 已提供 `Definition()`
- 已提供 `ToolInput` / `ToolResult`
- 已提供 `UsageText()` 或等价帮助文本
- 已提供 input/output JSON example
- 已把归一化和执行逻辑放在 tool 包内
- 已把 shortcut、provider 注册、UI 接入放在运行时层
- 已补测试
- 已更新本文档的 registry
- 如果此变更影响架构边界，已同步更新 `AGENTS.md` / `README.md`

## Shortcut 规则

有 slash command 的 tool，按下面约束处理：

- shortcut 应该是 runtime 的注册层，不是 tool 本体
- shortcut 解析失败时，应能回到 tool 的 usage/help
- shortcut 不应偷偷附带会话生命周期副作用，除非 runtime policy 明确允许
- 多接口支持时，每个接口可以决定是否暴露该 shortcut
- 如果某个接口需要补充专属动作，例如微信 `/send <序号>`，这类动作仍应作为 transport adapter 的补充层，而不是塞回核心 tool

## 测试要求

一个成熟 tool 至少建议覆盖这些测试：

- 契约完整性测试
  例如 `Definition()` 不为空，字段齐全。
- 输入归一化测试
  默认值、别名、大小写、路径、时间表达式等。
- 执行测试
  正常执行、错误路径、边界情况。
- 帮助可达性测试
  shortcut help 或 usage 文本能稳定返回。
- runtime 注册测试
  例如 `tool provider`、command registry、接口 gating。
- desktop / weixin 暴露一致性测试
  如果某个工具对 desktop 暴露，也应验证它默认对 weixin 暴露，除非存在明确的平台不可用条件。

## 运行时接入要求

通常按下面方式接入：

- `internal/ai`
  做“是否需要这个 tool”的识别、候选工具提示、工具计划输入。
- `internal/app`
  负责 tool provider 暴露、执行编排、shared runtime policy。
- `internal/<tool>`
  只负责定义契约、规范输入、执行能力。
- interface adapter
  只负责展示、交互、发文件、弹窗、扫码等界面或协议动作。

如果一个实现把这几层揉成一个 handler，通常说明它还不够像一个规范化 tool unit。

## Eval Runner

`cmd/baize-eval` 已实现，可用于对 tool unit 进行批量评测。

```
baize-eval -data-dir <dir> -dataset <path> [-output <path>]
```

Flags：

- `-data-dir`：数据目录，默认 `"data"`，用于定位 `app.db` 和密钥文件。
- `-dataset`：必填，`.jsonl` 数据集文件路径（例如 `docs/evals/route-command.jsonl`）。
- `-output`：可选，结果输出路径。默认写到 `eval/testdata/runs/<timestamp>-<provider>-<model>-<dataset>.json`。

运行后会在控制台逐条打印 pass / fail，并将完整报告以 JSON 写入输出路径。

## Registered Tool Units

### `everything_file_search`

- Package: `internal/filesearch`
- Purpose: Search local Windows files via Everything (`es.exe`) using either native queries or structured semantic filters.
- Input contract: `query`, `keywords`, `drives`, `known_folders`, `paths`, `extensions`, `date_field`, `date_value`, `limit`
- Output contract: executed query, effective limit, result count, ordered file items with `index`, `name`, and `path`
- Shortcut registration: `/find` and `/find help`, handled by the shared app runtime; WeChat additionally supports `/send <序号>` through its interface adapter
- Current pipeline split:
  - generic tool opportunity detection and tool planning in `internal/ai`
  - runtime orchestration in `internal/app`
  - search execution and selection state in `internal/filesearch`
  - WeChat file delivery in `internal/weixin/filesender.go`

### `list_directory`

- Package: `internal/dirlist`
- Purpose: List files and folders in a local directory through native filesystem reads instead of shell commands.
- Input contract: `path`, `limit`, `include_hidden`, `directories_only`
- Output contract: resolved directory path, effective limit, returned item count, truncation flag, and ordered items with `index`, `name`, `path`, `is_dir`, `size_bytes`, and `modified_at`
- Shortcut registration: none; exposed through the shared local agent tool provider and hidden from WeChat contexts
- Shortcut registration: none; exposed through the shared local agent tool provider for both desktop and WeChat
- Current pipeline split:
  - tool opportunity detection and planning in `internal/ai`
  - runtime exposure in `internal/app`
  - directory enumeration and filtering in `internal/dirlist`

### Shell Family

- Packages:
  - `internal/bashtool`
  - `internal/powershelltool`
- Tools: `bash_tool`, `powershell_tool`
- Family metadata: `FamilyKey=shell`, `FamilyTitle=Shell`
- Purpose: Inspect local machine state through platform-native shell command surfaces, while keeping execution inside strict read-only allowlists.
- Input contract: `command`, optional `args`, optional `timeout_seconds`
- Output contract: tool name, shell name, executed command, args, exit code, stdout, stderr, truncation flag
- Shortcut registration: none; exposed through the shared local agent tool provider and filtered only by platform support
- Current pipeline split:
  - tool opportunity detection and tool planning in `internal/ai`
  - runtime exposure and platform gating in `internal/app`
  - Bash-oriented execution contract in `internal/bashtool`
  - PowerShell-oriented execution contract in `internal/powershelltool`

### Screen Family

- Package: `internal/screencapture`
- Tools: `screen_capture`
- Family metadata: `FamilyKey=screen`, `FamilyTitle=屏幕`
- Purpose: Capture the current host screen as a local JPEG file for one-shot inspection, with optional visual summarization.
- Input contract: `analyze`, `max_dimension`, `jpeg_quality`
- Output contract: tool name, saved image path, mime type, dimensions, display index, capture timestamp, analysis status, and optional summary
- Shortcut registration: none; exposed through the shared local agent tool provider for both desktop and WeChat, and filtered only by platform support
- Current pipeline split:
  - tool selection and invocation in `internal/ai`
  - runtime exposure and AI summary wiring in `internal/app`
  - capture contract, image normalization, and file output in `internal/screencapture`

### macOS Family

- Package: `internal/osascripttool`
- Tools: `osascript_tool`
- Family metadata: `FamilyKey=macos`, `FamilyTitle=macOS`
- Purpose: Inspect the current macOS desktop state and activate a target application through a small allowlisted AppleScript surface.
- Input contract: `action`, plus `app_name` for `activate_app` and `open_or_activate_app`, and optional `timeout_seconds`
- Output contract: tool name, shell, action, exit code, stdout, stderr, truncation flag
- Shortcut registration: none; exposed through the shared local agent tool provider for both desktop and WeChat, and filtered only by platform support
- Current pipeline split:
  - tool selection and invocation in `internal/ai`
  - runtime exposure in `internal/app`
  - macOS action normalization, allowlisting, and osascript execution in `internal/osascripttool`

### Windows Family

- Package: `internal/windowsautomationtool`
- Tools: `windows_automation_tool`
- Family metadata: `FamilyKey=windows`, `FamilyTitle=Windows`
- Purpose: Inspect Windows top-level desktop windows and bring a target window or app to the foreground through a small allowlisted automation surface.
- Input contract: `action`, plus `title_contains` for `focus_window`, `process_name` for `focus_app`, `app_name` for `launch_or_focus_app`, optional `limit` for `list_windows`, and optional `timeout_seconds`
- Output contract: tool name, shell, action, exit code, stdout, stderr, truncation flag
- Shortcut registration: none; exposed through the shared local agent tool provider for both desktop and WeChat, and filtered only by platform support
- Current pipeline split:
  - tool selection and invocation in `internal/ai`
  - runtime exposure in `internal/app`
  - Windows action normalization, allowlisting, and PowerShell execution in `internal/windowsautomationtool`

### Knowledge Family

- Package: `internal/knowledge`
- Tools: `knowledge_search`, `remember`, `append_knowledge`, `forget_knowledge`
- Family metadata: `FamilyKey=knowledge`, `FamilyTitle=知识库`
- Purpose: Search, create, extend, and delete knowledge entries in the active project knowledge base.
- Input contract:
  - `knowledge_search`: `query`
  - `remember`: `text`
  - `append_knowledge`: `id`, `text`
  - `forget_knowledge`: `id`
- Output contract: plain-text summaries for search hits, created entries, updated entries, and deletion confirmations.
- Shortcut registration: `/kb ...` stays in the shared runtime command layer; agent tools are exposed separately through the shared local agent tool provider.
- Current pipeline split:
  - tool opportunity detection and tool planning in `internal/ai`
  - runtime command routing and tool orchestration in `internal/app`
  - storage, search primitives, and family contracts in `internal/knowledge`

### Reminder Family

- Package: `internal/reminder`
- Tools: `reminder_list`, `reminder_add`, `reminder_remove`
- Family metadata: `FamilyKey=reminder`, `FamilyTitle=提醒`
- Purpose: List, create, and delete reminders visible to the current runtime context.
- Input contract:
  - `reminder_list`: no arguments
  - `reminder_add`: `spec`
  - `reminder_remove`: `id`
- Output contract: plain-text summaries for reminder listings, creation confirmation, and deletion confirmation.
- Shortcut registration: `/notice` and `/cron` remain runtime shortcuts; agent tools are exposed separately through the shared local agent tool provider.
- Current pipeline split:
  - tool opportunity detection and tool planning in `internal/ai`
  - runtime command routing, reminder scoping, and tool orchestration in `internal/app`
  - reminder persistence, scheduling, and family contracts in `internal/reminder`
