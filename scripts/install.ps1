#!/usr/bin/env pwsh
# Gas Town cross-platform install script (PowerShell 7+ / Windows PowerShell 5.1)
#
# Usage:
#   pwsh scripts/install.ps1          # Build + install gt
#   pwsh scripts/install.ps1 -NoDaemon # Install without restarting daemon
#
# Requirements: Go 1.22+, git
# Installs to: ~/.local/bin/gt (Linux/macOS) or ~/bin/gt.exe (Windows)

param(
    [switch]$NoDaemon,
    [switch]$Help
)

$ErrorActionPreference = "Stop"

if ($Help) {
    Write-Host @"
Gas Town Installer (cross-platform)

Usage:
  pwsh scripts/install.ps1              Build and install gt
  pwsh scripts/install.ps1 -NoDaemon    Install without restarting daemon

Flags:
  -NoDaemon   Skip daemon restart after install (safe-install equivalent)
  -Help       Show this help message
"@
    exit 0
}

# Resolve repo root (parent of scripts/)
$RepoRoot = Split-Path -Parent $PSScriptRoot
Push-Location $RepoRoot

try {
    # Verify prerequisites
    if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
        Write-Error "Go is not installed. Install Go 1.22+ from https://go.dev/dl/"
    }
    if (-not (Get-Command git -ErrorAction SilentlyContinue)) {
        Write-Error "git is not installed."
    }

    # Build metadata
    $Version = (git describe --tags --always --dirty 2>$null) ?? "dev"
    $Commit = git rev-parse --short HEAD 2>$null
    $BuildTime = (Get-Date -Format "yyyy-MM-ddTHH:mm:ssZ")
    $LDFlags = "-s -w -X github.com/steveyegge/gastown/internal/version.Version=$Version -X github.com/steveyegge/gastown/internal/version.Commit=$Commit -X github.com/steveyegge/gastown/internal/version.BuildTime=$BuildTime -X github.com/steveyegge/gastown/internal/version.BuiltProperly=true"

    # Determine install directory
    if ($IsWindows -or ($env:OS -eq "Windows_NT")) {
        $InstallDir = Join-Path $HOME "bin"
        $BinaryName = "gt.exe"
    } else {
        $InstallDir = Join-Path $HOME ".local" "bin"
        $BinaryName = "gt"
    }

    # Build
    Write-Host "Building gt ($Version)..." -ForegroundColor Cyan
    $env:CGO_ENABLED = "0"
    go build -ldflags $LDFlags -o (Join-Path $InstallDir $BinaryName) ./cmd/gt/
    if ($LASTEXITCODE -ne 0) { throw "Build failed" }
    Write-Host "Installed to: $(Join-Path $InstallDir $BinaryName)" -ForegroundColor Green

    # Ensure install dir is in PATH
    $InstallDirResolved = (Resolve-Path $InstallDir).Path
    $PathDirs = $env:PATH -split [IO.Path]::PathSeparator
    if ($InstallDirResolved -notin $PathDirs) {
        Write-Host ""
        Write-Host "Add to PATH:" -ForegroundColor Yellow
        if ($IsWindows -or ($env:OS -eq "Windows_NT")) {
            Write-Host "  `$env:PATH += `";$InstallDirResolved`""
            Write-Host "  # Or permanently: [Environment]::SetEnvironmentVariable('PATH', `$env:PATH + ';$InstallDirResolved', 'User')"
        } else {
            Write-Host "  export PATH=`"$InstallDirResolved`:`$PATH`""
        }
    }

    # Daemon restart (unless -NoDaemon)
    if (-not $NoDaemon) {
        $gtPath = Join-Path $InstallDir $BinaryName
        if (Test-Path $gtPath) {
            Write-Host "Restarting daemon..." -ForegroundColor Cyan
            & $gtPath daemon stop 2>$null
            Start-Sleep -Seconds 1
            & $gtPath daemon start 2>$null
            if ($LASTEXITCODE -eq 0) {
                Write-Host "Daemon restarted." -ForegroundColor Green
            } else {
                Write-Host "Daemon start returned non-zero (may be normal on first install)." -ForegroundColor Yellow
            }
        }
    }

    # Plugin sync
    $PluginsDir = Join-Path $RepoRoot "plugins"
    if (Test-Path $PluginsDir) {
        $gtPath = Join-Path $InstallDir $BinaryName
        Write-Host "Syncing plugins..." -ForegroundColor Cyan
        & $gtPath plugin sync --source $PluginsDir 2>$null
    }

    Write-Host ""
    Write-Host "Done. Run 'gt version' to verify." -ForegroundColor Green

} finally {
    Pop-Location
}
