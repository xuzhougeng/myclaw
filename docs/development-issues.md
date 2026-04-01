# 开发问题记录

这份文档专门记录开发过程中已经踩过、且后续很容易再次踩到的问题。

它不是 changelog，也不是零散 TODO。这里收口的是三类内容：

- 用户可见、且排查成本高的问题
- 看起来像 A，根因却在 B 的误导性问题
- 已经形成明确工程约束的问题

每次新增记录时，尽量补齐这四部分：

- 现象：用户看到什么
- 根因：真正出错的层和原因
- 修复：代码上做了什么
- 约束：以后应该如何避免

## 1. Windows 桌面端 `send is not defined`

### 现象

- `make dev` 正常
- Windows 安装包里打开工具页、聊天页时报 `send is not defined`
- 看起来像 Wails bridge 或 Windows wrapper 注入失败

### 根因

根因不在 bridge，而在前端模板字符串本身。

工具页模板里直接写了 Markdown 风格的反引号命令示例，例如：

- `` `/find` ``
- `` `/send ...` ``

在 JS 模板字符串里，这会被当成表达式求值，不是普通文本。

- `find` 恰好是浏览器全局函数，所以一开始没炸
- `send` 不是合法全局值，所以报 `ReferenceError: send is not defined`

### 修复

- 命令示例统一改成 `<code>/find</code>` 这类纯 HTML 文本
- 不再在 JS 模板字符串里直接写 Markdown 反引号代码片段

### 约束

- 前端模板字符串里展示命令、路径、关键字时，用 HTML 标签，不要用 Markdown 反引号
- 遇到 `xxx is not defined`，先定位实际报错行，不要先假设是 Wails 或 WebView2 注入失败

## 2. `ask` 模式误走 AI 路由

### 现象

- `ask mode` 下输入 `你好`
- 结果被路由成 `/help`
- 页面出现“可用命令”而不是普通聊天回复

### 根因

旧实现里 `ask` 和 `agent` 都先经过同一层 `RouteCommand()`。

这会导致：

- `ask` 实际不是“手动挡”
- 普通聊天先被 AI 重新分类
- 一旦分类误判，就会落到 `/help`、`/notice`、`/kb ...` 等逻辑

这和模式定义不一致。

### 修复

- `ask` 模式改成直接走普通聊天
- `agent` 模式才走 AI 路由、工具判断、自动决策
- 只保留显式手动触发：
  - slash command
  - `@ai`
  - `@kb`
  - `@agent`

### 约束

- `ask = 手动挡`
- `agent = 自动挡`
- 不要再让 `ask` 先经过自动路由或自动工具机会检测

## 3. 自动工具调用只能走 agent loop

### 现象

- UI 里已经把某个工具手动关闭了
- 但普通消息仍然能“识别需求 -> 匹配工具 -> 执行工具”
- 看起来像工具开关失效

### 根因

历史上存在一条快捷旁路，消息分发层会直接调用特定工具逻辑。

这意味着：

- `tool_provider_protocol.go` 只过滤了 agent loop 可见工具
- 但某些特殊链路仍然能绕过 agent loop 直接执行工具

典型例子就是自然语言文件检索旁路。

### 修复

- 删除自然语言文件检索旁路
- 显式 `/find` 继续保留为手动命令
- 自动工具调用统一只走 `ListAgentToolDefinitions -> ExecuteAgentTool`

### 约束

自动工具调用只有一条合法入口：

- agent loop 自己挑工具

允许做前置筛选，但只能筛候选集合，例如：

- UI 手动开关
- 平台能力差异
- 接口可用性判断

不允许在消息分发层直接绕过 agent loop 去调用工具。

## 4. 非持久命令回复在桌面端“闪一下就消失”

### 现象

- `/help`、某些命令回复会先显示
- 随后聊天列表刷新后又消失

### 根因

有一类命令按策略本来就不写入持久历史，例如 `/help`。

桌面前端先乐观渲染了回复，但随后又用后端真实历史覆盖界面。因为后端并没有持久化这条回复，所以 UI 看起来像“闪一下就没了”。

### 修复

- 后端在 `ChatResponse` 中显式返回 `historyPersisted`
- 前端把这类回复作为 transient message 合并展示
- 不再用后端真实历史直接粗暴覆盖

### 约束

- 前端不能假设所有回复都会写入持久历史
- 后端需要明确告诉界面：这次回复是否已持久化

## 5. Windows Debug 需要区分前端和后端

### 现象

- 右下角 `Desktop Diagnostics` 显示 `buildMode=debug`
- 但 `data/debug` 目录不存在
- 或者能看到前端 debug，后端却没有任何日志

### 根因

前端 debug 和后端 debug 是两条独立链路：

- 前端 debug 由 bundle 构建注入
- 后端 debug 由 Go `ldflags` 注入 `desktopBuildMode=debug`

前端面板显示 debug，并不能证明 Go 后端也在 debug 模式。

### 修复

- Debug 构建启动时，后端强制写一条 startup marker 到：
  - `%LOCALAPPDATA%\myclaw\data\debug\desktop-backend-debug.log`
- marker 包含：
  - `buildMode`
  - `dataDir`
  - `logPath`
  - `pid`

### 约束

排查 Windows 桌面问题时先做两步确认：

1. 看右下角 `Desktop Diagnostics`
2. 看 `desktop-backend-debug.log` 是否存在 startup marker

只有两边都确认后，才开始判断是前端问题还是后端问题。

## 6. 微信调试不能只看前端，需要看后端边界日志

### 现象

- 桌面窗口直接消失或行为异常
- 前端诊断信息不足
- WER dump 也不一定出现

### 根因

微信桥是后台 goroutine 长轮询链路，很多问题发生在：

- `handleMessage`
- `conversationUpdated`
- `sendChunkedReply`
- reminder notifier
- bridge goroutine exit

这些路径不一定经过前端，也不一定生成崩溃转储。

### 修复

Debug 构建里补了后端边界日志，记录：

- `weixin.handleMessage.start`
- `weixin.handleMessage.beforeService`
- `weixin.processTrace`
- `weixin.handleMessage.afterService`
- `weixin.handleMessage.replyReady`
- `weixin.sendChunkedReply.before/after`
- `weixin.reminder.notify.before/after`
- `weixin.conversationUpdated.before/after`
- `desktop.emitChatChanged.before/after`
- `desktop.weixinBridge.start/exit`
- `desktop.shutdown`

### 约束

Windows 桌面 + 微信问题的排查顺序应固定为：

1. 先确认是否是 release 包还是 Debug 包
2. 看前端 `Desktop Diagnostics`
3. 看后端 `desktop-backend-debug.log`
4. 先确定最后一条边界日志，再讨论根因

不要在没有边界日志的情况下直接猜：

- Wails bridge
- WebView2
- 前端页面
- 提醒模块

## 7. 微信里的“提醒时间到了 …”不等于 AI 回答了提醒内容

### 现象

- 微信里发送 `hi` / `你好`
- 回复看起来像“提醒时间到了：喝水”
- 容易误判为 AI 回复错了

### 根因

这类内容通常不是 AI 生成，而是到期提醒通过 notifier 发出的提醒消息。

这次排查中已经确认：

- `hi` 主流程先因为模型网络超时失败，回复了“处理失败，请稍后重试。”
- 随后积压提醒继续通过 notifier 补发

所以“提醒时间到了 …”和 `hi` 的主回复是两条不同链路。

### 修复

- 微信 notifier 改成当前消息回复完成后再注册
- Debug 日志里显式区分：
  - `handleMessage.replyReady`
  - `reminder.notify.before/after`

### 约束

微信里如果看到提醒内容混在普通对话回复附近，先判断消息来源，不要先假设是模型回答内容被污染。

## 8. 一次性提醒调度不能边遍历边删除

### 现象

- 多条一次性提醒同时到期时，行为异常
- 有机会引发错乱、遗漏，甚至把进程拖进不稳定状态

### 根因

旧实现里 `reminder.Manager.runDue()` 使用 `for index := range items` 遍历，同时又在循环里对同一个 slice 做删除。

这是一种脆弱写法，尤其在多条提醒同轮到期时容易出问题。

### 修复

- 改成构造 `nextItems` 的安全迭代方式
- 每轮只做：
  - 保留未到期项
  - 成功触发后移除一次性提醒
  - 成功触发后重排 daily 提醒

### 约束

- 不要在 `range` 原 slice 的同时修改它
- 涉及调度、删除、重排的逻辑，优先采用“读旧列表，构造新列表”的方式

## 9. AI 路由失败不等于程序退出

### 现象

- 日志里出现：
  - `AI 路由失败`
  - 网络超时
  - `reply=处理失败，请稍后重试。`

### 根因

这说明模型网络不可达，例如：

- `api.openai-proxy.org:443` 超时

它解释的是“为什么这条消息没有正常 AI 回复”，不是“为什么桌面程序退出”。

### 约束

排障时要把这两类问题拆开：

- 业务回复失败：看 `processTrace`
- 进程退出：看生命周期边界日志和 panic/debug 日志

不要把“主流程返回错误”误当成“进程崩溃原因”。
