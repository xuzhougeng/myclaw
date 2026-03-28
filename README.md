# myclaw

`myclaw` 现在是一个最小化的 Go 桌面常驻工具，当前支持三类入口：本地终端 REPL、微信桥接，以及基于 Wails v2 的桌面前端。它可以把需要记住的内容存进本地知识库，并在配置模型后用 AI 做意图识别、整理记忆和基于知识库回答。

当前版本刻意保持简单：

- 知识库存储在本地 JSON 文件里
- 模型配置只从本地环境变量读取，不在终端或聊天界面里暴露
- 配置模型后，会先做 AI 命令路由，再决定是“记住 / 遗忘 / 提醒 / 查看 / 回答”
- 普通问题默认走 direct 模式，直接调用 AI；切到 knowledge 模式后才会走“AI 检索计划 -> 本地候选检索 -> 模型复核 -> 回答”
- 支持图片直接总结入库；PDF 走 `go-fitz` 提取全文后再总结
- 支持单次提醒和每天重复提醒
- 微信桥接只保留扫码登录、长轮询、文本/语音文字收发
- 不做向量检索、权限隔离或多租户隔离

## 目录

```text
cmd/myclaw            程序入口
internal/ai           OpenAI responses 调用
internal/app          最小消息处理逻辑
internal/knowledge    本地知识库存储
internal/modelconfig  模型环境变量读取
internal/reminder     提醒调度与持久化
internal/terminal     终端 REPL
internal/weixin       iLink 微信桥接最小实现
cmd/myclaw-desktop    Wails 桌面前端入口
```

## 运行

### 0. 终端模式

直接运行即可进入终端：

```bash
go run ./cmd/myclaw
```

或者显式指定：

```bash
go run ./cmd/myclaw -terminal
```

### 0.5. Wails 桌面模式

直接启动桌面前端：

```bash
go run ./cmd/myclaw-desktop
```

桌面模式当前提供：

- 图片 / PDF 文件导入
- 知识库列表、补充、删除、清空
- 模型配置页面，可直接保存和测试连接
- 原生文件选择和确认对话框
- 微信页面，支持在桌面端直接显示二维码扫码登录
- 对话面板，可继续使用 `/remember`、`/notice`、`/forget`、`/debug-search`、`/mode` 等命令

桌面版默认数据目录会放到用户配置目录：

- Windows: `%LOCALAPPDATA%\myclaw\data`
- Linux/macOS: 对应系统的用户配置目录下 `myclaw/data`

如果你显式传了 `-data-dir` 或设置了 `MYCLAW_DATA_DIR`，则以传入值为准。

模型配置现在只从本地模型数据库读取，不再从 `MYCLAW_MODEL_*` 环境变量读取。

- 桌面端会把模型 profile 保存到数据目录下的 `model/profiles.db`
- API Key 会单独加密后保存，前端只显示掩码，不会回填明文
- 支持多 profile，并可切换当前活跃模型
- OpenAI 支持 `responses` 和 `chat_completions`
- Anthropic 支持 `messages`

Windows PowerShell 直接运行 terminal：

```powershell
.\scripts\run-terminal.ps1
```

### 0.6. 浏览器 HTTP Dev 模式

如果你要在浏览器里调前端，而不是直接起 Wails 窗口，可以运行：

```bash
make dev
```

默认会启动：

- HTTP 地址：`http://127.0.0.1:3415`
- 同一个 Go 进程内同时提供前端静态资源和 `/api/*` 后台接口

如果要改监听地址：

```bash
make dev HTTP_DEV_ADDR=127.0.0.1:8080
```

这个模式下前端会自动切到 HTTP backend 适配层，而不是调用 `window.go` / `window.runtime`。因此：

- 模型配置、记忆管理、聊天、微信扫码状态都可以直接联调
- 文件导入会走浏览器上传接口，而不是 Wails 原生文件对话框
- 原生提醒弹窗和 Wails 事件只在桌面窗口模式下可用

### 1. 微信扫码登录

```bash
go run ./cmd/myclaw -weixin-login
```

当前实现不依赖第三方 Go 包，但也没有内置终端二维码渲染。执行登录命令后，程序会输出 `qrcode_img_content`，你需要把这段内容生成二维码后再用微信扫码。

登录成功后，凭证会写到 `data/weixin-bridge/account.json`。

### 2. 启动微信桥接

```bash
go run ./cmd/myclaw -weixin
```

或者：

```bash
MYCLAW_WEIXIN_ENABLED=1 go run ./cmd/myclaw
```

### 3. 常用消息

- `记住：Windows 版本先做微信接口`
- `请帮我记住这个东西：未来要支持 macOS`
- `/remember 未来要支持 macOS`
- `/remember-file ./docs/puppeteer.pdf`
- `./screenshots/puppeteer-home.png`
- `/append 6d2d7724 它是 Google 出品的一个工具`
- `给 #6d2d7724 补充：它是 Google 出品的一个工具`
- `再补充一点：它是 Google 出品的一个工具`
- `/skills`
- `/show-skill writer`
- `/load-skill writer`
- `/unload-skill writer`
- `/page-skills`
- `/mode`
- `/mode knowledge`
- `@kb macOS 什么时候做？`
- `@ai 帮我直接分析这个方案`
- `/translate Puppeteer is a browser automation tool.`
- `/forget 0015f908`
- `/notice 2小时后 喝水`
- `一分钟后提醒我喝水`
- `/notice 每天 09:00 写日报`
- `/notice 2026-03-30 14:00 交房租`
- `/notice list`
- `/notice remove <提醒ID前缀>`
- `/cron 每天 18:00 健身`
- `/list`
- `/stats`
- `/debug-search macOS 什么时候做？`
- `/clear`
- `现在我记了什么？`

文件摄入说明：

- 图片会走视觉输入，总结成适合后续检索的中文 Markdown
- PDF 会先用 `go-fitz` 提取全文，再做摘要
- 默认跨平台 release 包使用 `CGO_ENABLED=0`，因此图片可用，但 PDF 会返回“当前构建不包含 PDF 文本提取能力”
- 如果你要在 Windows 本机启用 PDF，总结请安装可用的 C 工具链后用 `.\scripts\build-windows.ps1 -UseCgo`

## 技能库

现在支持一个本地技能库，但只做人工控制，不做自动技能决策：

- `/skills` 查看技能库和当前会话已加载技能
- `/show-skill <技能名>` 查看某个技能内容
- `/load-skill <技能名>` 手动加载一个技能到当前会话
- `/unload-skill <技能名>` 从当前会话卸载一个技能
- `/page-skills` 查看当前会话已加载技能

说明：

- 模型不会自己决定加载哪个技能
- 由人先看 `/skills` 和 `/show-skill`，再手动决定是否 `/load-skill`
- 技能一旦加载，会影响当前页面 / 当前会话里的 AI 路由、翻译、检索计划和回答
- 现在技能隔离优先按 `SessionID`，没有会话 ID 时才回退到用户维度

## 对话模式

现在普通对话支持 3 种模式：

- `direct`：默认模式。普通问题直接走 AI，不依赖知识库。
- `knowledge`：普通问题走知识库检索、候选复核和基于知识库回答。
- `agent`：工具模式。默认包含一批本地工具，包括知识检索、记忆写入/删除、提醒查看/创建/删除；工具注册已经抽成 provider，可继续挂接 MCP / NCP / ACP。

切换方式：

- `/mode` 查看当前模式
- `/mode direct`
- `/mode knowledge`
- `/mode agent`

也可以只对单条消息临时覆盖：

- `@ai ...`
- `@kb ...`
- `@agent ...`

如果需要在代码里扩展 agent 工具，可以在创建 `app.Service` 后注册 provider：

```go
service.RegisterMCPToolProvider("docs", myMCPClient)
service.RegisterNCPToolProvider("desktop", myNCPClient)
service.RegisterACPToolProvider("wechat", myACPClient)
```

注册后，agent 会看到类似 `mcp.docs::lookup`、`ncp.desktop::open_app`、`acp.wechat::send_message` 这样的工具名，并按 provider 前缀分发执行。

默认技能目录：

- `<data-dir>/skills/<技能名>/SKILL.md`

也可以通过环境变量追加额外目录：

- `MYCLAW_SKILLS_DIRS`

示例：

```text
data/skills/writer/SKILL.md
```

`SKILL.md` 建议至少包含一个简短 frontmatter：

```md
---
name: writer
description: 帮助输出更清晰的中文写作
---

# Writer
给出简洁、结构清晰、少废话的中文输出。
```

desktop 端导入 `.zip` skill 包时，会校验这些规则：

- zip 内必须有且仅有一个 `SKILL.md`
- `SKILL.md` 必须位于 zip 根目录，或位于唯一顶层技能目录下
- `SKILL.md` 必须包含 frontmatter，且至少有非空的 `name` 和 `description`
- frontmatter 后还必须有实际的技能说明正文

## 编译

### Windows 本机

PowerShell:

```powershell
.\scripts\build-windows.ps1
.\scripts\build-windows.ps1 -All
.\scripts\build-windows.ps1 -Arch arm64 -RunTests
.\scripts\build-windows.ps1 -UseCgo
```

默认会输出到 `dist/`：

- `dist/myclaw-windows-amd64.exe`
- `dist/myclaw-windows-arm64.exe`（使用 `-All` 或 `-Arch arm64`）

说明：

- 默认脚本使用 `CGO_ENABLED=0`，更适合直接分发
- 如果要启用 `go-fitz` 的 PDF 提取，请在 Windows 本机准备好 C 工具链后加上 `-UseCgo`

### Release 包

`make release` 现在除了编译各平台二进制，还会生成 zip 包：

- `dist/packages/myclaw-windows-amd64.zip`
- `dist/packages/myclaw-windows-arm64.zip`
- `dist/packages/myclaw-linux-amd64.zip`
- `dist/packages/myclaw-linux-arm64.zip`
- `dist/packages/myclaw-darwin-amd64.zip`
- `dist/packages/myclaw-darwin-arm64.zip`

Windows zip 包内会包含：

- `myclaw.exe`
- `run-weixin.ps1`
- `run-terminal.ps1`
- `run-all.ps1`
- `install-autostart.ps1`
- `uninstall-autostart.ps1`
- `README.txt`

默认数据目录和日志目录会使用：

- `%LOCALAPPDATA%\myclaw\data`
- `%LOCALAPPDATA%\myclaw\logs`

这样在 Windows 上解压后就可以直接复制整个目录并运行脚本，不需要 Go 源码环境，也不会因为换了解压目录而丢失微信登录状态。
如果你之前在旧版本里把登录态存在解压目录下的 `data/weixin-bridge/account.json`，第一次切到新目录启动时也会自动迁移过去。

### GitHub Actions NSIS 桌面安装包

仓库新增了 GitHub Actions workflow：

- `.github/workflows/windows-nsis-installer.yml`

这个 workflow 会在 `windows-latest` 上：

- 安装 Wails CLI
- 安装 NSIS
- 构建 `amd64` 的 Wails 桌面应用
- 用 `wails build -nsis` 生成桌面安装器 `.exe`
- 把安装器作为 Actions artifact 上传

桌面打包配置放在：

- `cmd/myclaw-desktop/wails.json`

产物会落在：

- `cmd/myclaw-desktop/build/bin/`

workflow 上传的 artifact 现在只包含 NSIS 安装器：

- `*-installer.exe`

如果你想在本地 Windows 手动构建桌面安装器，可以在 `cmd/myclaw-desktop/` 下执行：

```powershell
wails build -platform windows/amd64 -o myclaw-desktop-amd64.exe -nsis -webview2 download -m -s
```

### Windows 开机自启

先确保你已经编译出 Windows 可执行文件，然后安装用户级开机自启：

```powershell
.\scripts\install-autostart.ps1
```

默认行为：

- 登录 Windows 后自动启动 `myclaw`
- 以隐藏窗口方式运行
- 自动带上 `-weixin`
- 数据目录使用 `%LOCALAPPDATA%\myclaw\data`
- 日志写到 `%LOCALAPPDATA%\myclaw\logs\myclaw.log`

卸载开机自启：

```powershell
.\scripts\uninstall-autostart.ps1
```

如果你希望 AI 功能在开机自启后也可用，只要模型 profile 已经保存在同一个数据目录下即可，无需再配置 `MYCLAW_MODEL_*` 环境变量。

### Linux 交叉编译

```bash
make build-windows
make build-macos
make build-linux
make release
```

其中：

- `make build-windows` 会构建 `windows/amd64` 和 `windows/arm64`
- `make build-macos` 会构建 `darwin/amd64` 和 `darwin/arm64`
- `make build-linux` 会构建 `linux/amd64` 和 `linux/arm64`
- `make release` 会先跑测试，再把三类平台一起编出来

## Commit 规范

仓库内置了 `commit-msg` hook，提交信息必须使用下面三类前缀之一：

- `feat(scope): summary`
- `docs(scope): summary`
- `chore(scope): summary`

例如：

- `feat(weixin): add basic message loop`
- `docs(readme): explain build targets`
- `chore(hooks): enforce commit format`

### 安装 hook

Linux / macOS:

```bash
make install-hooks
```

Windows PowerShell:

```powershell
.\scripts\install-hooks.ps1
```

安装后，仓库会把 `core.hooksPath` 指向 `.githooks`，提交时会自动校验格式。

## 数据文件

- `data/knowledge/entries.json`: 知识库
- `data/reminders/items.json`: 提醒数据
- `data/weixin-bridge/account.json`: 微信登录凭证
- `data/weixin-bridge/sync_buf`: 微信长轮询游标

## 微信桥接协议说明

微信接入细节参考：[scAgent 文档](/home/xzg/project/scAgent/docs/weixin-bridge.md)。

当前实现只用了这份文档里的最小子集：

- `get_bot_qrcode`
- `get_qrcode_status`
- `getupdates`
- `sendmessage`

## Windows / macOS

目前代码用纯 Go 写成，没有绑死 Windows API，所以结构上已经为未来 macOS 支持留了空间。现阶段仍然按 Windows 桌面常驻进程来用，后续如果要加 GUI、托盘、模型调用或更复杂的能力，可以在这个骨架上继续扩。
