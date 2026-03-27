param(
    [string]$DataDir = "$env:LOCALAPPDATA\myclaw\data",
    [string]$LogFile = "$env:LOCALAPPDATA\myclaw\logs\myclaw-weixin.log"
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$exePath = Join-Path $PSScriptRoot "myclaw.exe"
if (-not (Test-Path $exePath)) {
    throw "Executable not found: $exePath"
}

New-Item -ItemType Directory -Force -Path $DataDir | Out-Null
New-Item -ItemType Directory -Force -Path (Split-Path -Parent $LogFile) | Out-Null

& $exePath -weixin -data-dir $DataDir -log-file $LogFile
if ($LASTEXITCODE -ne 0) {
    throw "myclaw exited with code $LASTEXITCODE"
}
