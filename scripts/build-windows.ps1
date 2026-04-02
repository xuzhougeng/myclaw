param(
    [ValidateSet("amd64", "arm64")]
    [string]$Arch = "amd64",
    [string]$OutputDir = "dist",
    [string]$AppName = "baize",
    [switch]$All,
    [switch]$RunTests,
    [switch]$UseCgo
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$scriptDir = Split-Path -Parent $PSCommandPath
$repoRoot = (Resolve-Path (Join-Path $scriptDir "..")).Path
$outputRoot = Join-Path $repoRoot $OutputDir

function Invoke-GoCommand {
    param(
        [Parameter(Mandatory = $true)]
        [string[]]$Arguments
    )

    & go @Arguments
    if ($LASTEXITCODE -ne 0) {
        throw "go $($Arguments -join ' ') failed with exit code $LASTEXITCODE"
    }
}

$previousGoos = $env:GOOS
$previousGoarch = $env:GOARCH
$previousCgoEnabled = $env:CGO_ENABLED

Push-Location $repoRoot
try {
    New-Item -ItemType Directory -Force -Path $outputRoot | Out-Null

    if ($RunTests) {
        Write-Host "Running tests before build..."
        Invoke-GoCommand -Arguments @("test", "./...")
    }

    $arches = if ($All) { @("amd64", "arm64") } else { @($Arch) }
    foreach ($targetArch in $arches) {
        $env:CGO_ENABLED = if ($UseCgo) { "1" } else { "0" }
        $env:GOOS = "windows"
        $env:GOARCH = $targetArch

        $output = Join-Path $outputRoot ("{0}-windows-{1}.exe" -f $AppName, $targetArch)
        Write-Host "Building $output (CGO_ENABLED=$($env:CGO_ENABLED)) ..."
        Invoke-GoCommand -Arguments @("build", "-trimpath", "-o", $output, "./cmd/baize")
    }
}
finally {
    if ($null -eq $previousGoos) {
        Remove-Item Env:GOOS -ErrorAction SilentlyContinue
    }
    else {
        $env:GOOS = $previousGoos
    }

    if ($null -eq $previousGoarch) {
        Remove-Item Env:GOARCH -ErrorAction SilentlyContinue
    }
    else {
        $env:GOARCH = $previousGoarch
    }

    if ($null -eq $previousCgoEnabled) {
        Remove-Item Env:CGO_ENABLED -ErrorAction SilentlyContinue
    }
    else {
        $env:CGO_ENABLED = $previousCgoEnabled
    }

    Pop-Location
}

Write-Host "Done."
