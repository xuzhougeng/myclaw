param(
    [Parameter(Mandatory = $true)]
    [string]$Version,
    [switch]$DebugMode
)

$ErrorActionPreference = "Stop"

$binaryName = "baize"
$versionedBaseName = "$binaryName-$Version"
$versionedExeName = "$versionedBaseName.exe"
$versionedInstallerName = "$versionedBaseName-amd64-installer.exe"
$scriptDir = Split-Path -Parent $PSCommandPath
$repoRoot = (Resolve-Path (Join-Path $scriptDir "..")).Path
$outputRoot = Join-Path $repoRoot "dist"
$installerPath = Join-Path $outputRoot $versionedInstallerName
$ldflagsParts = @("-X main.appVersion=$Version")
if ($DebugMode) {
    $ldflagsParts += "-X main.desktopBuildMode=debug"
}
$ldflags = $ldflagsParts -join " "

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

function Get-DesktopBuildEnvironment {
    param(
        [Parameter(Mandatory = $true)]
        [string]$GoCommand
    )

    $environment = Get-GoFallbackEnvironment -GoCommand $GoCommand
    # Desktop builds depend on sqlite via go-sqlite3, so they must not inherit CGO_ENABLED=0.
    $environment["CGO_ENABLED"] = "1"
    return $environment
}

function Invoke-FrontendBundleBuild {
    param(
        [switch]$DebugMode
    )

    $goCommand = Get-Command go -ErrorAction SilentlyContinue
    if ($null -eq $goCommand) {
        throw "go was not found in PATH. It is required to build desktop frontend bundles."
    }

    $environment = Get-GoFallbackEnvironment -GoCommand $goCommand.Source
    $environment["BAIZE_DESKTOP_DEBUG_DIAGNOSTICS"] = if ($DebugMode) { "1" } else { "0" }

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
        Environment = Get-DesktopBuildEnvironment -GoCommand $goCommand.Source
        ResolvedVersion = $wailsVersion
        UsesFallback = $true
    }
}

$wailsCommand = Get-Command wails -ErrorAction SilentlyContinue
$wailsBuildEnvironment = if ($null -ne $wailsCommand) {
    Get-DesktopBuildEnvironment -GoCommand (Get-Command go).Source
}
else {
    @{}
}
$wailsInvocation = Get-WailsInvocation
Invoke-FrontendBundleBuild -DebugMode:$DebugMode

if ($DebugMode) {
    Write-Host "Desktop build debug mode enabled: frontend self-diagnostics will be bundled into this artifact."
}

if (Test-Path $installerPath) {
    Remove-Item $installerPath -Force
}

# Build with Wails
Push-Location (Join-Path $repoRoot "cmd/baize-desktop")
try {
    if ($wailsInvocation.UsesFallback) {
        Write-Host "wails CLI not found in PATH; using go run fallback pinned to $($wailsInvocation.ResolvedVersion)."
        if ($wailsInvocation.Environment.Count -gt 0) {
            Write-Host ("Applying Go fallback env: " + (($wailsInvocation.Environment.GetEnumerator() | Sort-Object Name | ForEach-Object { "{0}={1}" -f $_.Name, $_.Value }) -join ", "))
        }
    } elseif ($wailsBuildEnvironment.Count -gt 0) {
        Write-Host ("Applying desktop build env: " + (($wailsBuildEnvironment.GetEnumerator() | Sort-Object Name | ForEach-Object { "{0}={1}" -f $_.Name, $_.Value }) -join ", "))
    }
    $buildEnvironment = if ($wailsInvocation.UsesFallback) { $wailsInvocation.Environment } else { $wailsBuildEnvironment }
    Invoke-ExternalCommand -Command $wailsInvocation.Command -Arguments ($wailsInvocation.Arguments + @(
        "build",
        "-platform", "windows/amd64",
        "-o", $versionedExeName,
        "-nsis",
        "-webview2", "download",
        "-ldflags", $ldflags,
        # Skip go mod tidy to avoid rewriting go.mod/go.sum during desktop packaging.
        "-m",
        "-s"
    )) -Environment $buildEnvironment
}
finally {
    Pop-Location
}

# Normalize installer filename so build/bin and dist both carry the version.
$buildBinDir = Join-Path $repoRoot "cmd/baize-desktop/build/bin"
$versionedBuiltInstallerPath = Join-Path $buildBinDir $versionedInstallerName
$builtInstaller = Get-ChildItem -Path $buildBinDir -Filter "$versionedBaseName-*-installer.exe" | Select-Object -First 1
if ($null -eq $builtInstaller) {
    $builtInstaller = Get-ChildItem -Path $buildBinDir -Filter "*-installer.exe" | Sort-Object LastWriteTimeUtc -Descending | Select-Object -First 1
}
if ($null -eq $builtInstaller) {
    throw "Installer not found in cmd/baize-desktop/build/bin"
}

if ($builtInstaller.Name -ne $versionedInstallerName) {
    if (Test-Path $versionedBuiltInstallerPath) {
        Remove-Item $versionedBuiltInstallerPath -Force
    }
    Move-Item $builtInstaller.FullName $versionedBuiltInstallerPath
}

New-Item -ItemType Directory -Path $outputRoot -Force | Out-Null
Copy-Item $versionedBuiltInstallerPath -Destination $installerPath

Write-Host "Created $installerPath"
