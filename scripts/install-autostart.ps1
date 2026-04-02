param(
    [string]$ExePath = "dist\baize-windows-amd64.exe",
    [string]$StartupName = "baize-weixin",
    [string]$DataDir = "$env:LOCALAPPDATA\baize\data",
    [string]$LogFile = "$env:LOCALAPPDATA\baize\logs\baize.log",
    [string]$ExtraArgs = ""
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$scriptDir = Split-Path -Parent $PSCommandPath
$repoRoot = (Resolve-Path (Join-Path $scriptDir "..")).Path
$resolvedExePath = (Resolve-Path (Join-Path $repoRoot $ExePath) -ErrorAction SilentlyContinue)
if ($null -eq $resolvedExePath) {
    throw "Executable not found: $ExePath. Build it first, e.g. .\scripts\build-windows.ps1"
}

$startupDir = [Environment]::GetFolderPath("Startup")
if ([string]::IsNullOrWhiteSpace($startupDir)) {
    throw "Unable to resolve Windows Startup folder"
}

New-Item -ItemType Directory -Force -Path $DataDir | Out-Null
New-Item -ItemType Directory -Force -Path (Split-Path -Parent $LogFile) | Out-Null

$argsList = @(
    "-weixin",
    "-data-dir", $DataDir,
    "-log-file", $LogFile
)

if (-not [string]::IsNullOrWhiteSpace($ExtraArgs)) {
    $argsList += $ExtraArgs
}

$quotedArgs = foreach ($arg in $argsList) {
    if ($arg -match '\s') {
        '"' + ($arg -replace '"', '""') + '"'
    }
    else {
        $arg
    }
}

$commandLine = '"' + $resolvedExePath.Path.Replace('"', '""') + '" ' + ($quotedArgs -join ' ')
$vbsCommandLine = $commandLine.Replace('"', '""')
$startupFile = Join-Path $startupDir ($StartupName + ".vbs")

$content = @"
Set shell = CreateObject("WScript.Shell")
shell.Run "$vbsCommandLine", 0, False
"@

Set-Content -Path $startupFile -Value $content -Encoding ASCII

Write-Host "Autostart installed:"
Write-Host "  startup file: $startupFile"
Write-Host "  exe path:     $($resolvedExePath.Path)"
Write-Host "  data dir:     $DataDir"
Write-Host "  log file:     $LogFile"
Write-Host ""
Write-Host "AI model profiles are stored in the local data directory."
Write-Host "If you use desktop once to save model profiles, autostart will reuse the same data dir automatically."
