param(
    [string]$DataDir = "data"
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$scriptDir = Split-Path -Parent $PSCommandPath
$repoRoot = (Resolve-Path (Join-Path $scriptDir "..")).Path

Push-Location $repoRoot
try {
    & go run ./cmd/baize -terminal -data-dir $DataDir
    if ($LASTEXITCODE -ne 0) {
        throw "go run failed with exit code $LASTEXITCODE"
    }
}
finally {
    Pop-Location
}
