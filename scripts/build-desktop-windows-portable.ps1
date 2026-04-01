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
        [string[]]$Arguments,
        [hashtable]$Environment = @{}
    )

    $previousEnvironment = @{}
    try {
        foreach ($name in $Environment.Keys) {
            $previousEnvironment[$name] = [Environment]::GetEnvironmentVariable($name, "Process")
            Set-Item -Path "Env:$name" -Value $Environment[$name]
        }

        & $Command @Arguments
        if ($LASTEXITCODE -ne 0) {
            throw "$Command $($Arguments -join ' ') failed with exit code $LASTEXITCODE"
        }
    }
    finally {
        foreach ($name in $Environment.Keys) {
            $previousValue = $previousEnvironment[$name]
            if ([string]::IsNullOrEmpty($previousValue)) {
                Remove-Item -Path "Env:$name" -ErrorAction SilentlyContinue
            }
            else {
                Set-Item -Path "Env:$name" -Value $previousValue
            }
        }
    }
}

function Get-GoFallbackEnvironment {
    param(
        [Parameter(Mandatory = $true)]
        [string]$GoCommand
    )

    $environment = @{}
    $currentGoProxy = (& $GoCommand env GOPROXY 2>$null).Trim()
    if ([string]::IsNullOrWhiteSpace($env:GOPROXY) -and (
        [string]::IsNullOrWhiteSpace($currentGoProxy) -or
        $currentGoProxy -eq "https://proxy.golang.org" -or
        $currentGoProxy -eq "https://proxy.golang.org,direct"
    )) {
        $environment["GOPROXY"] = "https://goproxy.cn,https://proxy.golang.org,direct"
    }

    $currentGoSumDB = (& $GoCommand env GOSUMDB 2>$null).Trim()
    if ([string]::IsNullOrWhiteSpace($env:GOSUMDB) -and (
        [string]::IsNullOrWhiteSpace($currentGoSumDB) -or
        $currentGoSumDB -eq "sum.golang.org"
    )) {
        $environment["GOSUMDB"] = "sum.golang.google.cn"
    }

    return $environment
}

function Invoke-FrontendBundleBuild {
    $goCommand = Get-Command go -ErrorAction SilentlyContinue
    if ($null -eq $goCommand) {
        throw "go was not found in PATH. It is required to build desktop frontend bundles."
    }

    $environment = Get-GoFallbackEnvironment -GoCommand $goCommand.Source

    Push-Location $repoRoot
    try {
        Invoke-ExternalCommand -Command $goCommand.Source -Arguments @(
            "run",
            "./scripts/build_frontend_bundle.go"
        ) -Environment $environment
    }
    finally {
        Pop-Location
    }
}

function Get-WailsVersionFromGoMod {
    $goModPath = Join-Path $repoRoot "go.mod"
    if (-not (Test-Path $goModPath)) {
        return "latest"
    }

    $versionLine = Select-String -Path $goModPath -Pattern 'github\.com/wailsapp/wails/v2\s+(v\S+)' | Select-Object -First 1
    if ($null -ne $versionLine -and $versionLine.Matches.Count -gt 0) {
        return $versionLine.Matches[0].Groups[1].Value
    }

    return "latest"
}

function Get-WailsInvocation {
    $wailsCommand = Get-Command wails -ErrorAction SilentlyContinue
    if ($null -ne $wailsCommand) {
        return @{
            Command = $wailsCommand.Source
            Arguments = @()
            Environment = @{}
            ResolvedVersion = ""
            UsesFallback = $false
        }
    }

    $goCommand = Get-Command go -ErrorAction SilentlyContinue
    if ($null -eq $goCommand) {
        throw "Neither wails nor go was found in PATH. Install Go first, or add the Wails CLI to PATH, then rerun this script."
    }

    $wailsVersion = Get-WailsVersionFromGoMod
    return @{
        Command = $goCommand.Source
        Arguments = @("run", "github.com/wailsapp/wails/v2/cmd/wails@$wailsVersion")
        Environment = Get-GoFallbackEnvironment -GoCommand $goCommand.Source
        ResolvedVersion = $wailsVersion
        UsesFallback = $true
    }
}
$wailsInvocation = Get-WailsInvocation
Invoke-FrontendBundleBuild

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
    # Keep standard Wails bindings; obfuscated bindings change the frontend call path.
    "-s"
)

if (-not [string]::IsNullOrWhiteSpace($Version)) {
    $buildArgs += @("-ldflags", "-X main.appVersion=$Version")
}

Push-Location $desktopDir
try {
    if ($wailsInvocation.UsesFallback) {
        Write-Host "wails CLI not found in PATH; using go run fallback pinned to $($wailsInvocation.ResolvedVersion)."
        if ($wailsInvocation.Environment.Count -gt 0) {
            Write-Host ("Applying Go fallback env: " + (($wailsInvocation.Environment.GetEnumerator() | Sort-Object Name | ForEach-Object { "{0}={1}" -f $_.Name, $_.Value }) -join ", "))
        }
    }
    Invoke-ExternalCommand -Command $wailsInvocation.Command -Arguments ($wailsInvocation.Arguments + $buildArgs) -Environment $wailsInvocation.Environment
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
