param(
    [ValidateSet("amd64", "arm64")]
    [string]$Arch = "amd64",
    [string]$OutputDir = "dist",
    [string]$AppName = "myclaw",
    [string]$Version = "",
    [switch]$Clean
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$scriptDir = Split-Path -Parent $PSCommandPath
$repoRoot = (Resolve-Path (Join-Path $scriptDir "..")).Path
$desktopDir = Join-Path $repoRoot "cmd/myclaw-desktop"
$buildBinDir = Join-Path $desktopDir "build/bin"
$outputRoot = Join-Path $repoRoot $OutputDir

function Invoke-ExternalCommand {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Command,
        [Parameter(Mandatory = $true)]
        [string[]]$Arguments
    )

    & $Command @Arguments
    if ($LASTEXITCODE -ne 0) {
        throw "$Command $($Arguments -join ' ') failed with exit code $LASTEXITCODE"
    }
}

$wailsCommand = Get-Command wails -ErrorAction SilentlyContinue
if ($null -eq $wailsCommand) {
    throw "wails CLI not found in PATH. Install it first, then rerun this script."
}

$portableBaseName = if ([string]::IsNullOrWhiteSpace($Version)) {
    "{0}-desktop-windows-{1}" -f $AppName, $Arch
}
else {
    "{0}-desktop-{1}-windows-{2}" -f $AppName, $Version, $Arch
}
$portableExeName = "$portableBaseName.exe"
$portableExePath = Join-Path $outputRoot $portableExeName
$builtExePath = Join-Path $buildBinDir $portableExeName

if ($Clean) {
    if (Test-Path $builtExePath) {
        Remove-Item $builtExePath -Force
    }
    if (Test-Path $portableExePath) {
        Remove-Item $portableExePath -Force
    }
}

New-Item -ItemType Directory -Force -Path $outputRoot | Out-Null

$buildArgs = @(
    "build",
    "-platform", "windows/$Arch",
    "-o", $portableExeName,
    "-webview2", "download",
    "-m",
    "-s"
)

if (-not [string]::IsNullOrWhiteSpace($Version)) {
    $buildArgs += @("-ldflags", "-X main.appVersion=$Version")
}

Push-Location $desktopDir
try {
    Invoke-ExternalCommand -Command $wailsCommand.Source -Arguments $buildArgs
}
finally {
    Pop-Location
}

if (-not (Test-Path $builtExePath)) {
    $candidate = Get-ChildItem -Path $buildBinDir -Filter "$portableBaseName*.exe" -File |
        Where-Object { $_.Name -notlike "*-installer.exe" } |
        Sort-Object LastWriteTimeUtc -Descending |
        Select-Object -First 1
    if ($null -eq $candidate) {
        throw "Portable desktop executable not found in $buildBinDir"
    }
    $builtExePath = $candidate.FullName
}

Copy-Item $builtExePath -Destination $portableExePath -Force

Write-Host "Created portable desktop executable:"
Write-Host "  $portableExePath"
