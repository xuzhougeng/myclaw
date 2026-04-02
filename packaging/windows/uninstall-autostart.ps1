param(
    [string]$StartupName = "baize-weixin"
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$startupDir = [Environment]::GetFolderPath("Startup")
if ([string]::IsNullOrWhiteSpace($startupDir)) {
    throw "Unable to resolve Windows Startup folder"
}

$startupFile = Join-Path $startupDir ($StartupName + ".vbs")
if (Test-Path $startupFile) {
    Remove-Item $startupFile -Force
    Write-Host "Removed autostart file: $startupFile"
}
else {
    Write-Host "Autostart file not found: $startupFile"
}
