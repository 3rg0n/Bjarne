#Requires -Version 5.1
<#
.SYNOPSIS
    Installs bjarne - AI-assisted C/C++ code generation with mandatory validation

.DESCRIPTION
    This script downloads and installs bjarne to your system.
    It will:
    - Download the latest release from GitHub
    - Install to a directory and add to PATH
    - Check for podman/docker

.PARAMETER InstallDir
    Directory to install bjarne. Default: $HOME\.bjarne\bin

.PARAMETER Version
    Specific version to install. Default: latest

.PARAMETER NoPathUpdate
    Skip adding install directory to PATH

.EXAMPLE
    irm https://raw.githubusercontent.com/3rg0n/bjarne/master/install.ps1 | iex

.EXAMPLE
    .\install.ps1 -InstallDir "C:\Tools\bjarne" -Version "v0.1.0"
#>

[CmdletBinding()]
param(
    [string]$InstallDir = "$HOME\.bjarne\bin",
    [string]$Version = "latest",
    [switch]$NoPathUpdate
)

$ErrorActionPreference = "Stop"
$ProgressPreference = "SilentlyContinue"  # Faster downloads

$RepoOwner = "3rg0n"
$RepoName = "bjarne"

function Write-Step {
    param([string]$Message)
    Write-Host "`n>> " -ForegroundColor Yellow -NoNewline
    Write-Host $Message
}

function Write-Success {
    param([string]$Message)
    Write-Host "[OK] " -ForegroundColor Green -NoNewline
    Write-Host $Message
}

function Write-Warn {
    param([string]$Message)
    Write-Host "[!] " -ForegroundColor Yellow -NoNewline
    Write-Host $Message
}

function Get-LatestVersion {
    $releaseUrl = "https://api.github.com/repos/$RepoOwner/$RepoName/releases/latest"
    try {
        $release = Invoke-RestMethod -Uri $releaseUrl -Headers @{ "User-Agent" = "bjarne-installer" }
        return $release.tag_name
    }
    catch {
        throw "Failed to fetch latest release: $_"
    }
}

function Get-Architecture {
    $arch = [System.Environment]::GetEnvironmentVariable("PROCESSOR_ARCHITECTURE")
    switch ($arch) {
        "AMD64" { return "amd64" }
        "ARM64" { return "arm64" }
        default { throw "Unsupported architecture: $arch" }
    }
}

function Test-ContainerRuntime {
    $podman = Get-Command podman -ErrorAction SilentlyContinue
    $docker = Get-Command docker -ErrorAction SilentlyContinue

    if ($podman) {
        return @{ Name = "podman"; Path = $podman.Source }
    }
    elseif ($docker) {
        return @{ Name = "docker"; Path = $docker.Source }
    }
    return $null
}

# Banner
Write-Host @"

  _     _
 | |   (_) __ _ _ __ _ __   ___
 | |__ | |/ _` | '__| '_ \ / _ \
 | '_ \| | (_| | |  | | | |  __/
 |_.__/| |\__,_|_|  |_| |_|\___|
      _/ |
     |__/   AI C/C++ Code Generator

"@ -ForegroundColor Cyan

Write-Step "Detecting system..."
$arch = Get-Architecture
Write-Host "  Architecture: $arch"
Write-Host "  OS: Windows"

# Get version
Write-Step "Fetching release information..."
if ($Version -eq "latest") {
    $Version = Get-LatestVersion
}
Write-Host "  Version: $Version"

# Download URL - binary is released as standalone .exe
$exeName = "bjarne-windows-$arch.exe"
$downloadUrl = "https://github.com/$RepoOwner/$RepoName/releases/download/$Version/$exeName"

Write-Step "Downloading bjarne $Version..."

# Create install directory
if (-not (Test-Path $InstallDir)) {
    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
}

$destPath = Join-Path $InstallDir "bjarne.exe"

try {
    Invoke-WebRequest -Uri $downloadUrl -OutFile $destPath
    Write-Success "Downloaded bjarne.exe"
}
catch {
    throw "Failed to download bjarne: $_"
}

# Add to PATH
if (-not $NoPathUpdate) {
    Write-Step "Updating PATH..."
    $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
    if ($userPath -notlike "*$InstallDir*") {
        [Environment]::SetEnvironmentVariable("Path", "$InstallDir;$userPath", "User")
        $env:Path = "$InstallDir;$env:Path"
        Write-Success "Added $InstallDir to PATH"
    }
    else {
        Write-Host "  Already in PATH"
    }
}

# Check container runtime
Write-Step "Checking container runtime..."
$runtime = Test-ContainerRuntime
if ($runtime) {
    Write-Success "Found $($runtime.Name) at $($runtime.Path)"
}
else {
    Write-Warn "No container runtime found!"
    Write-Host @"

  bjarne requires podman or docker for code validation.

  Install options:
    - Podman (recommended): winget install RedHat.Podman
    - Docker Desktop: https://docker.com/products/docker-desktop

  After installing Podman, run:
    podman machine init
    podman machine start

"@ -ForegroundColor Yellow
}

# Verify installation
Write-Step "Verifying installation..."
if (Test-Path $destPath) {
    try {
        $ver = & $destPath --version 2>&1 | Select-Object -First 1
        Write-Success "bjarne installed successfully!"
        Write-Host "`n  $ver" -ForegroundColor Cyan
    }
    catch {
        Write-Success "bjarne installed to $destPath"
    }
}
else {
    throw "Installation failed - bjarne.exe not found"
}

Write-Host @"

Installation complete!

  Location: $destPath

  Quick start:
    bjarne              # Start interactive mode
    bjarne --help       # Show help

  First run will prompt to pull the validation container (~500MB).

  NOTE: Restart your terminal for PATH changes to take effect.

"@ -ForegroundColor Green
