param(
    [string]$DataDir = "$env:LOCALAPPDATA\myclaw\data",
    [string]$LogFile = "$env:LOCALAPPDATA\myclaw\logs\myclaw-terminal.log"
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$exePath = Join-Path $PSScriptRoot "myclaw.exe"
if (-not (Test-Path $exePath)) {
    throw "Executable not found: $exePath"
}

$requiredVars = @(
    "MYCLAW_MODEL_PROVIDER",
    "MYCLAW_MODEL_BASE_URL",
    "MYCLAW_MODEL_API_KEY",
    "MYCLAW_MODEL_NAME"
)

$missing = @()
foreach ($name in $requiredVars) {
    $item = Get-Item -Path "Env:$name" -ErrorAction SilentlyContinue
    if ($null -eq $item -or [string]::IsNullOrWhiteSpace($item.Value)) {
        $missing += $name
    }
}

if ($missing.Count -gt 0) {
    throw "Missing required env vars: $($missing -join ', ')"
}

New-Item -ItemType Directory -Force -Path $DataDir | Out-Null
New-Item -ItemType Directory -Force -Path (Split-Path -Parent $LogFile) | Out-Null

& $exePath -terminal -data-dir $DataDir -log-file $LogFile
if ($LASTEXITCODE -ne 0) {
    throw "myclaw exited with code $LASTEXITCODE"
}
