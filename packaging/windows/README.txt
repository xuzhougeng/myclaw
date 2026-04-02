baize Windows package

Files:
- baize.exe
- run-weixin.ps1
- run-terminal.ps1
- run-all.ps1
- install-autostart.ps1
- uninstall-autostart.ps1

Typical usage:
1. Run desktop once and save your model profile in the "模型配置" page.
2. Run:
   .\run-weixin.ps1
   or
   .\run-terminal.ps1
   or
   .\run-all.ps1

File ingest:
- Image summary works in this packaged build.
- PDF summary uses go-fitz and requires a native CGO-enabled build.
- The default release zip is built with CGO disabled, so PDF ingest will return a clear "PDF extraction unavailable" message instead of crashing.

Default data and logs:
- Data: %LOCALAPPDATA%\baize\data
- Logs: %LOCALAPPDATA%\baize\logs
- WeChat login state is stored under %LOCALAPPDATA%\baize\data\weixin-bridge.

Autostart:
- install-autostart.ps1 installs a hidden Startup-folder launcher
- uninstall-autostart.ps1 removes it
- AI model profiles are reused from %LOCALAPPDATA%\baize\data\model
