param(
    [string]$DataDir = "data"
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$scriptDir = Split-Path -Parent $PSCommandPath
$repoRoot = (Resolve-Path (Join-Path $scriptDir "..")).Path

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

Push-Location $repoRoot
try {
    & go run ./cmd/myclaw -terminal -data-dir $DataDir
    if ($LASTEXITCODE -ne 0) {
        throw "go run failed with exit code $LASTEXITCODE"
    }
}
finally {
    Pop-Location
}
