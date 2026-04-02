# AI 分阶段真实模型评测方案

本文档描述如何在 `baize` 里对不同 AI 阶段做真实模型评测。

目标不是替代现有单元测试，而是补上一层“拿你自己设计的数据集，直接调用真实模型，看每个阶段是否稳定工作”的评测链路。

## 目标

- 用真实模型验证结构化阶段输出，而不只验证 mock HTTP 或 fake AI。
- 让数据集可以人工维护、逐步扩充、按阶段回归。
- 把“阶段正确性”和“整条对话体验”拆开评估，避免问题混在一起。
- 让后续不同模型、不同 prompt、不同 profile 可以横向比较。

## 为什么不要直接把它做成普通 `go test`

真实模型评测和普通单元测试不是一类东西：

- 真实模型调用有成本。
- 真实模型存在非确定性。
- 模型 profile、温度、接口类型、上游供应商都会影响结果。
- 一次失败不一定代表代码坏了，也可能是模型波动。

所以更合适的做法是：

- 保留现在的 `*_test.go` 做确定性测试。
- 另做一套“评测数据集 + 独立 runner + 结果归档”。

第一版先把数据结构和评测口径定下来，runner 已在 `cmd/baize-eval` 实现。

## 当前仓库里最适合做阶段评测的入口

按代码当前结构，优先评测这些公开阶段：

1. 命令路由
   对应：`internal/ai/routing.go` 的 `RouteCommand`
2. 检索计划
   对应：`internal/ai/routing.go` 的 `BuildSearchPlan`
3. 检索候选复核
   对应：`internal/ai/routing.go` 的 `ReviewAnswerCandidates`
4. 工具机会识别
   对应：`internal/ai/tool_decider.go` 的 `DetectToolOpportunities`
5. 工具调用规划
   对应：`internal/ai/tool_decider.go` 的 `PlanToolUse`
6. agent 单步决策
   对应：`internal/ai/agent.go` 的 `DecideAgentStep`
7. 最终回答
   对应：`internal/ai/chat.go` 的 `Chat` / `Answer`
8. 总结类任务
   对应：`internal/ai/summarize.go` 的 `SummarizePDFText` / `SummarizeImageFile`

建议优先顺序：

1. 先做结构化输出阶段：`RouteCommand`、`BuildSearchPlan`、`DetectToolOpportunities`、`PlanToolUse`、`DecideAgentStep`
2. 再做半结构化阶段：`ReviewAnswerCandidates`
3. 最后再做开放式生成阶段：`Chat`、`Answer`、`Summarize*`

原因很简单：结构化阶段最容易稳定判分，也最容易定位退化点。

## 目录结构

```text
eval/
  testdata/
    route-command.jsonl
    search-plan.jsonl
    retrieval-review.jsonl
    tool-opportunity.jsonl
    tool-plan.jsonl
    agent-step.jsonl
    answer.jsonl
    summarize-pdf.jsonl
    runs/
      2026-03-31-openai-gpt-4.1-route-command.json
      2026-03-31-openai-gpt-4.1-tool-plan.json
```

约定：

- `eval/testdata/*.jsonl` 保存人工维护的数据集。
- `eval/testdata/runs/*.json` 保存某次真实跑分结果。
- 如果结果文件里可能包含敏感输入、知识库内容或 API 上下文，不要默认提交到仓库。

## 数据集设计原则

一条数据只测一个阶段的一个清晰能力，不要一条样本里同时掺杂多个意图。

每个阶段的数据集都建议包含：

- `id`：稳定样本 ID
- `stage`：阶段名
- `input` 或 `task`
- `context`：只放这个阶段真正需要的上下文
- `expect`：期望输出
- `judge`：如何判分
- `note`：人工备注，可选

建议区分三类样本：

- 正常样本：模型应该稳定答对
- 边界样本：容易混淆，但仍应有明确输出
- 负样本：模型应该拒绝猜测、返回空、或选 `stop`

## 判分方式建议

不要一上来就用“答案完全一致”评所有阶段。不同阶段要用不同规则。

推荐规则：

- `exact`
  适合枚举字段，如 `command`、`action`、`tool_name`
- `one_of`
  适合多种都可接受的固定答案
- `contains`
  适合文本里必须包含某个短语
- `contains_all`
  适合关键词集合
- `subset_of`
  适合“返回内容不能超出允许集合”
- `non_empty`
  适合只要求不为空的阶段
- `human_review`
  适合开放式回答的人工抽样复核

第一版建议：

- 结构化阶段只用规则判分，不引入第二个 LLM 当裁判。
- 开放式生成先做人工抽样，不要急着上 LLM-as-a-judge。

## 各阶段推荐数据格式

### 1. `RouteCommand`

适合用最严格的规则判分，因为输出结构固定。

示例：

```json
{"id":"route-001","stage":"route_command","input":"提醒我明天 09:00 开周会","expect":{"command":"notice_add","reminder_spec":"明天 09:00 开周会"},"judge":{"command":"exact","reminder_spec":"contains"}}
{"id":"route-002","stage":"route_command","input":"把 #abcd 这条再补充：需要带截图","expect":{"command":"append","knowledge_id":"abcd","append_text":"需要带截图"},"judge":{"command":"exact","knowledge_id":"exact","append_text":"contains"}}
{"id":"route-003","stage":"route_command","input":"帮我看看 MCP 文档怎么定义 tool calls","expect":{"command":"answer","question":"MCP 文档怎么定义 tool calls"},"judge":{"command":"exact","question":"contains"}}
```

### 2. `BuildSearchPlan`

这个阶段不适合要求查询句完全一致，更适合看“是否覆盖关键检索词”。

示例：

```json
{"id":"search-001","stage":"search_plan","input":"去年我记过 OpenAI responses API 的切换说明吗","expect":{"queries":["OpenAI responses API","切换说明"],"keywords":["OpenAI","responses API"]},"judge":{"queries":"contains_all","keywords":"contains_all"}}
{"id":"search-002","stage":"search_plan","input":"我之前记过 fitz 处理 PDF 的坑吗","expect":{"queries":["fitz PDF","处理 PDF 的坑"],"keywords":["fitz","PDF"]},"judge":{"queries":"contains_all","keywords":"contains_all"}}
```

### 3. `ReviewAnswerCandidates`

这里输入不是自然语言一句话，而是“问题 + 候选知识项”。

示例：

```json
{"id":"review-001","stage":"retrieval_review","question":"如何切换到 OpenAI responses API","entries":[{"id":"a1","text":"OpenAI responses API 已经替代一部分旧接口。"},{"id":"a2","text":"今天天气不错。"},{"id":"a3","text":"切换步骤：先改 api_type，再改 base_url。"}],"expect":{"selected_ids":["a1","a3"]},"judge":{"selected_ids":"contains_all"}}
```

### 4. `DetectToolOpportunities`

这是“用户需不需要工具”的判断，最怕模型乱猜。

示例：

```json
{"id":"tool-opp-001","stage":"tool_opportunity","task":"帮我找下载目录最近两天的 pdf","tools":["everything_file_search"],"expect":{"tool_names":["everything_file_search"]},"judge":{"tool_names":"exact"}}
{"id":"tool-opp-002","stage":"tool_opportunity","task":"解释一下 MCP 的 tool calls 是什么","tools":["everything_file_search"],"expect":{"tool_names":[]},"judge":{"tool_names":"exact"}}
```

### 5. `PlanToolUse`

这个阶段输入要带上工具契约和历史执行结果。

示例：

```json
{"id":"tool-plan-001","stage":"tool_plan","task":"帮我找下载目录最近两天的 pdf","tool_name":"everything_file_search","prior":[],"expect":{"action":"tool","tool_name":"everything_file_search","tool_input_contains":["Downloads","pdf"]},"judge":{"action":"exact","tool_name":"exact","tool_input_contains":"contains_all"}}
{"id":"tool-plan-002","stage":"tool_plan","task":"帮我找下载目录最近两天的 pdf","tool_name":"everything_file_search","prior":[{"tool_name":"everything_file_search","tool_input":"{\"known_folders\":[\"Downloads\"],\"extensions\":[\"pdf\"]}","tool_output":"没有结果"}],"expect":{"action":"stop"},"judge":{"action":"one_of"}}
```

说明：

- `tool_input` 本身是 JSON 字符串。
- 对它做判分时，更适合先解成对象后再按字段规则判，不适合直接整串字符串比对。

### 6. `DecideAgentStep`

这个阶段非常适合评估 agent 是否会乱用工具、是否能在已有结果足够时及时收敛。

示例：

```json
{"id":"agent-001","stage":"agent_step","task":"帮我查知识库里有没有 MCP tool calls 的说明","history":[],"tools":["local::knowledge_search"],"results":[],"expect":{"action":"tool","tool_name":"local::knowledge_search"},"judge":{"action":"exact","tool_name":"exact"}}
{"id":"agent-002","stage":"agent_step","task":"帮我查知识库里有没有 MCP tool calls 的说明","history":[],"tools":["local::knowledge_search"],"results":[{"tool_name":"local::knowledge_search","tool_input":"{\"query\":\"tool calls\"}","output":"找到了两条相关笔记。"}],"expect":{"action":"one_of","answer_non_empty":true},"judge":{"action":"one_of","answer":"non_empty"}}
```

### 7. `Chat` / `Answer`

开放式生成先不要追求自动精判。推荐：

- 只做基础质量门槛：非空、中文、无明显胡编
- 每个版本抽样人工 review
- 真要自动评，也只在第二阶段再做

示例：

```json
{"id":"answer-001","stage":"answer","question":"如何切换到 responses API","entries":[{"id":"a1","text":"切换步骤：修改 api_type 为 responses。"}],"expect":{"must_contain":["responses","api_type"]},"judge":{"answer":"contains_all"}}
```

## 运行方式

使用 `cmd/baize-eval` 命令运行评测：

```bash
go run ./cmd/baize-eval -dataset docs/evals/route-command.jsonl
go run ./cmd/baize-eval -dataset docs/evals/route-command.jsonl -output eval/testdata/runs/result.json
```

参数：
- `-dataset`（必填）：数据集 `.jsonl` 文件路径
- `-data-dir`：数据目录（默认 `data`）
- `-output`：输出文件路径（默认自动生成到 `eval/testdata/runs/<timestamp>-<provider>-<model>-<dataset>.json`）

## runner 输出建议

每次跑分至少记录：

- 数据集文件
- 样本 ID
- 模型 provider / model / api_type
- 开始时间和耗时
- 原始输出
- 归一化输出
- 判分结果
- 错误信息
- token usage

当前代码里已经有 `usageCollector`，后续 runner 可以通过上下文把 token usage 一并记下来。

推荐输出结构：

```json
{
  "dataset": "eval/testdata/route-command.jsonl",
  "provider": "openai",
  "model": "gpt-4.1",
  "api_type": "responses",
  "started_at": "2026-03-31T09:30:00+08:00",
  "cases": [
    {
      "id": "route-001",
      "pass": true,
      "duration_ms": 842,
      "usage": {
        "inputTokens": 221,
        "outputTokens": 34,
        "totalTokens": 255
      },
      "raw_output": {
        "command": "notice_add",
        "memory_text": "",
        "append_text": "",
        "knowledge_id": "",
        "reminder_spec": "明天 09:00 开周会",
        "reminder_id": "",
        "question": ""
      },
      "judge_result": {
        "command": "pass",
        "reminder_spec": "pass"
      }
    }
  ]
}
```

## 模型与运行环境建议

为了让结果更可比，建议：

- 用固定 profile 跑一整批数据，不要中途换模型。
- 尽量把 `temperature` 设低，优先 `0` 或接近 `0`。
- 尽量固定 `provider`、`api_type`、`model`。
- 一次变更只改一个变量，例如只换 prompt、只换模型、只换工具契约。

如果要比较稳定性，建议同一批数据集连续跑 3 次，然后看：

- 首次通过率
- 三次全通过率
- 波动样本列表

真实模型评测里，“偶尔答对”没有意义，“稳定答对”才有意义。

## 推荐先做哪些数据集

第一批建议只做下面 5 个文件：

1. `eval/testdata/route-command.jsonl`
2. `eval/testdata/search-plan.jsonl`
3. `eval/testdata/tool-opportunity.jsonl`
4. `eval/testdata/tool-plan.jsonl`
5. `eval/testdata/agent-step.jsonl`

原因：

- 都是当前架构的关键阶段。
- 都能直接映射到 `internal/ai` 公共方法。
- 都以结构化输出为主，判分容易先做对。
- 真出问题时，定位 prompt、契约还是 orchestration 也更直接。

## 不建议的做法

- 用 end-to-end 聊天 transcript 直接混测所有阶段。
- 把真实模型评测强行并进 `go test ./...` 的默认流程。
- 用“字符串完全一致”来判 `BuildSearchPlan` 或开放式回答。
- 一开始就让另一个 LLM 当裁判，导致误差来源翻倍。
- 不记录原始输出，只看一个总分。

## 建议的落地顺序

1. 先按本文档把 `eval/testdata/*.jsonl` 数据集写出来
2. 先只覆盖结构化阶段
3. 已完成：`cmd/baize-eval` 直接调用 `internal/ai` 的公开方法
4. 把每次跑分结果落盘到 `eval/testdata/runs/`
5. 下一步：持续扩充测试用例，覆盖更多边界和负样本
6. 再决定是否需要把开放式回答引入半自动评审

## 一个务实的起点

如果你现在就要开始，不要先追求“大而全”。

最小可行版本只需要：

- 20 条 `RouteCommand`
- 20 条 `DetectToolOpportunities`
- 20 条 `PlanToolUse`

先把这 60 条跑通，你基本就能知道：

- 你的 prompt 是否稳定
- 你的结构化 schema 是否足够约束
- 你的工具契约是否足够自解释
- 模型切换后最先退化的是哪一层

`cmd/baize-eval` 已实现，现在的优先级是扩充测试用例，而不是继续往 `docs/` 里堆说明文字。
