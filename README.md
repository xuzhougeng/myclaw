# myclaw

`myclaw` 现在是一个最小化的 Go 桌面常驻工具，当前支持两类入口：本地终端 REPL 和微信桥接。它可以把需要记住的内容存进本地知识库，并在配置模型后用 AI 做意图识别、整理记忆和基于知识库回答。

当前版本刻意保持简单：

- 知识库存储在本地 JSON 文件里
- 模型配置只从本地环境变量读取，不在终端或聊天界面里暴露
- 配置模型后，会先做 AI 命令路由，再决定是“记住 / 遗忘 / 提醒 / 查看 / 回答”
- 支持图片直接总结入库；PDF 走 `go-fitz` 提取全文后再总结
- 支持单次提醒和每天重复提醒
- 微信桥接只保留扫码登录、长轮询、文本/语音文字收发
- 没有向量检索、没有模型总结、没有多租户隔离

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

模型配置只允许通过本地环境变量传入，不提供终端内配置命令。先在本地 shell 里设置：

```bash
export MYCLAW_MODEL_PROVIDER=openai
export MYCLAW_MODEL_BASE_URL=https://api.openai.com/v1
export MYCLAW_MODEL_API_KEY=<你的 key>
export MYCLAW_MODEL_NAME=<你的模型名>
```

Windows PowerShell:

```powershell
$env:MYCLAW_MODEL_PROVIDER="openai"
$env:MYCLAW_MODEL_BASE_URL="https://api.openai.com/v1"
$env:MYCLAW_MODEL_API_KEY="<你的 key>"
$env:MYCLAW_MODEL_NAME="<你的模型名>"
.\scripts\run-terminal.ps1
```

使用的环境变量固定为：

- `MYCLAW_MODEL_PROVIDER`
- `MYCLAW_MODEL_BASE_URL`
- `MYCLAW_MODEL_API_KEY`
- `MYCLAW_MODEL_NAME`

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
- `/clear`
- `现在我记了什么？`

文件摄入说明：

- 图片会走视觉输入，总结成适合后续检索的中文 Markdown
- PDF 会先用 `go-fitz` 提取全文，再做摘要
- 默认跨平台 release 包使用 `CGO_ENABLED=0`，因此图片可用，但 PDF 会返回“当前构建不包含 PDF 文本提取能力”
- 如果你要在 Windows 本机启用 PDF，总结请安装可用的 C 工具链后用 `.\scripts\build-windows.ps1 -UseCgo`

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

如果你希望 AI 功能在开机自启后也可用，请把这些变量设成用户级环境变量，而不是只在当前终端窗口里临时设置：

```powershell
[Environment]::SetEnvironmentVariable("MYCLAW_MODEL_PROVIDER", "openai", "User")
[Environment]::SetEnvironmentVariable("MYCLAW_MODEL_BASE_URL", "https://api.openai.com/v1", "User")
[Environment]::SetEnvironmentVariable("MYCLAW_MODEL_API_KEY", "<你的 key>", "User")
[Environment]::SetEnvironmentVariable("MYCLAW_MODEL_NAME", "<你的模型名>", "User")
```

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
