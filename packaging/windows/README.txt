myclaw Windows package

Files:
- myclaw.exe
- run-weixin.ps1
- run-terminal.ps1
- run-all.ps1
- install-autostart.ps1
- uninstall-autostart.ps1

Typical usage:
1. Set user env vars if you want AI features:
   MYCLAW_MODEL_PROVIDER
   MYCLAW_MODEL_BASE_URL
   MYCLAW_MODEL_API_KEY
   MYCLAW_MODEL_NAME
2. Run:
   .\run-weixin.ps1
   or
   .\run-terminal.ps1
   or
   .\run-all.ps1

Default data and logs:
- Data: %LOCALAPPDATA%\myclaw\data
- Logs: %LOCALAPPDATA%\myclaw\logs
- If an older package stored login data under .\data\weixin-bridge, the first run will migrate it automatically.

Autostart:
- install-autostart.ps1 installs a hidden Startup-folder launcher
- uninstall-autostart.ps1 removes it
