param(
    [string]$StartupName = "baize-weixin",
    [string]$DataDir = "$env:LOCALAPPDATA\baize\data",
    [string]$LogFile = "$env:LOCALAPPDATA\baize\logs\baize.log",
    [string]$ExtraArgs = ""
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$exePath = Join-Path $PSScriptRoot "baize.exe"
if (-not (Test-Path $exePath)) {
    throw "Executable not found: $exePath"
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

$commandLine = '"' + $exePath.Replace('"', '""') + '" ' + ($quotedArgs -join ' ')
$vbsCommandLine = $commandLine.Replace('"', '""')
$startupFile = Join-Path $startupDir ($StartupName + ".vbs")

$content = @"
Set shell = CreateObject("WScript.Shell")
shell.Run "$vbsCommandLine", 0, False
"@

Set-Content -Path $startupFile -Value $content -Encoding ASCII

Write-Host "Autostart installed:"
Write-Host "  startup file: $startupFile"
Write-Host "  exe path:     $exePath"
Write-Host "  data dir:     $DataDir"
Write-Host "  log file:     $LogFile"
