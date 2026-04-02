param(
    [string]$DataDir = "$env:LOCALAPPDATA\baize\data",
    [string]$LogFile = "$env:LOCALAPPDATA\baize\logs\baize-weixin.log"
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$exePath = Join-Path $PSScriptRoot "baize.exe"
if (-not (Test-Path $exePath)) {
    throw "Executable not found: $exePath"
}

New-Item -ItemType Directory -Force -Path $DataDir | Out-Null
New-Item -ItemType Directory -Force -Path (Split-Path -Parent $LogFile) | Out-Null

& $exePath -weixin -data-dir $DataDir -log-file $LogFile
if ($LASTEXITCODE -ne 0) {
    throw "baize exited with code $LASTEXITCODE"
}
