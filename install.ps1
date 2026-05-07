# oxguard install script for Windows (PowerShell)
# Usage:
#   & ([scriptblock]::Create((iwr -useb https://github.com/oxDevelop/oxguard/releases/latest/download/install.ps1))) pyguard
#   & ([scriptblock]::Create((iwr -useb https://github.com/oxDevelop/oxguard/releases/latest/download/install.ps1))) tsguard
param(
    [Parameter(Position = 0)]
    [string]$Tool = ""
)

$ErrorActionPreference = 'Stop'

$Repo    = "oxDevelop/oxguard"
$InstallDir = if ($env:OXGUARD_INSTALL_DIR) { $env:OXGUARD_INSTALL_DIR } `
              else { Join-Path $env:LOCALAPPDATA "Programs\oxguard" }

# ── helpers ──────────────────────────────────────────────────────────────────

function Die([string]$msg) {
    Write-Host "  [ERROR] $msg" -ForegroundColor Red
    exit 1
}
function Info([string]$msg) { Write-Host "  $msg" }
function Ok([string]$msg)   { Write-Host "  [OK]   $msg" -ForegroundColor Green }

# ── argument parsing ─────────────────────────────────────────────────────────

switch ($Tool) {
    "pyguard" {}
    "tsguard" {}
    "" {
        Write-Host "Usage: install.ps1 [pyguard|tsguard]"
        Write-Host ""
        Write-Host "  pyguard  — Python quality gate (ruff, mypy, radon, bandit, ...)"
        Write-Host "  tsguard  — TypeScript quality gate (biome, vitest, fta-cli, ...)"
        exit 1
    }
    default { Die "unknown tool: $Tool (choose pyguard or tsguard)" }
}

# ── platform detection ───────────────────────────────────────────────────────

$Arch = switch ($env:PROCESSOR_ARCHITECTURE) {
    "AMD64"   { "amd64" }
    "ARM64"   { Die "windows/arm64 binaries are not published yet — build from source" }
    default   { Die "unsupported architecture: $env:PROCESSOR_ARCHITECTURE" }
}

$OS = "windows"

# ── version resolution ───────────────────────────────────────────────────────

$Version = if ($env:OXGUARD_VERSION) { $env:OXGUARD_VERSION } else {
    $apiUrl  = "https://api.github.com/repos/$Repo/releases/latest"
    $release = Invoke-RestMethod -Uri $apiUrl -UseBasicParsing
    $release.tag_name
}
if (-not $Version) { Die "could not resolve latest version from GitHub API" }

Info "installing $Tool $Version for $OS/$Arch"

# ── download ─────────────────────────────────────────────────────────────────

$ZipName   = "$Tool-$Version-$OS-$Arch.zip"
$BaseUrl   = "https://github.com/$Repo/releases/download/$Version"
$TmpDir    = [System.IO.Path]::GetTempPath() | Join-Path -ChildPath ([System.IO.Path]::GetRandomFileName())
New-Item -ItemType Directory -Path $TmpDir | Out-Null

try {
    $ZipPath  = Join-Path $TmpDir $ZipName
    $HashPath = "$ZipPath.sha256"

    Info "downloading $ZipName..."
    Invoke-WebRequest -Uri "$BaseUrl/$ZipName"        -OutFile $ZipPath  -UseBasicParsing
    Invoke-WebRequest -Uri "$BaseUrl/$ZipName.sha256" -OutFile $HashPath -UseBasicParsing

    # ── verify ────────────────────────────────────────────────────────────────

    Info "verifying checksum..."
    $expected = (Get-Content $HashPath -Raw).Trim().Split()[0]
    $actual   = (Get-FileHash $ZipPath -Algorithm SHA256).Hash.ToLower()
    if ($actual -ne $expected) {
        Die "checksum mismatch (got $actual, expected $expected)"
    }

    # ── extract ───────────────────────────────────────────────────────────────

    Expand-Archive -Path $ZipPath -DestinationPath $TmpDir -Force

    # ── install ───────────────────────────────────────────────────────────────

    if (-not (Test-Path $InstallDir)) {
        New-Item -ItemType Directory -Path $InstallDir | Out-Null
    }

    $BinName = "$Tool.exe"
    Copy-Item (Join-Path $TmpDir $BinName) (Join-Path $InstallDir $BinName) -Force

    Ok "$Tool installed to $InstallDir\$BinName"

    # pyguard: copy analysis helper scripts
    $analysisDir = Join-Path $TmpDir "analysis"
    if ($Tool -eq "pyguard" -and (Test-Path $analysisDir)) {
        $shareDir = Join-Path $env:LOCALAPPDATA "oxguard\pyguard"
        if (-not (Test-Path $shareDir)) { New-Item -ItemType Directory -Path $shareDir | Out-Null }
        Copy-Item $analysisDir $shareDir -Recurse -Force
    }

    # ── verify binary runs ────────────────────────────────────────────────────

    $binPath = Join-Path $InstallDir $BinName
    try {
        $ver = & $binPath --version 2>&1
        Ok "$Tool $ver is working"
    } catch {
        Die "$Tool installed but --version failed: $_"
    }

    # ── PATH guidance ─────────────────────────────────────────────────────────

    $currentPath = [System.Environment]::GetEnvironmentVariable("PATH", "User")
    if ($currentPath -notlike "*$InstallDir*") {
        Write-Host ""
        Write-Host "  [!] $InstallDir is not on your PATH." -ForegroundColor Yellow
        Write-Host "      Run this to add it permanently (requires terminal restart):"
        Write-Host "        setx PATH `"$InstallDir;%PATH%`""
        Write-Host ""
    }

    Write-Host ""
    Write-Host "  Next steps:"
    Write-Host "    cd your-project"
    Write-Host "    $Tool setup    # wire dev dependencies + AI tool hooks"
    Write-Host "    $Tool doctor   # verify environment"
    Write-Host ""

} finally {
    Remove-Item -Recurse -Force $TmpDir -ErrorAction SilentlyContinue
}
