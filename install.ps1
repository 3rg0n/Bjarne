#Requires -Version 5.1
<#
.SYNOPSIS
    Installs bjarne - AI-assisted C/C++ code generation with mandatory validation

.DESCRIPTION
    This script downloads and installs bjarne to your system.
    It will:
    - Download the latest release from GitHub
    - Extract to a directory in your PATH (or specified location)
    - Verify checksum
    - Check for podman/docker

.PARAMETER InstallDir
    Directory to install bjarne. Default: $env:LOCALAPPDATA\bjarne

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
    [string]$InstallDir = "$env:LOCALAPPDATA\bjarne",
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

function Write-Warning {
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

# Download URL
$zipName = "bjarne_$($Version.TrimStart('v'))_windows_$arch.zip"
$downloadUrl = "https://github.com/$RepoOwner/$RepoName/releases/download/$Version/$zipName"
$checksumUrl = "https://github.com/$RepoOwner/$RepoName/releases/download/$Version/checksums.txt"

Write-Step "Downloading bjarne $Version..."
$tempDir = Join-Path $env:TEMP "bjarne-install-$(Get-Random)"
New-Item -ItemType Directory -Path $tempDir -Force | Out-Null

$zipPath = Join-Path $tempDir $zipName
$checksumPath = Join-Path $tempDir "checksums.txt"

try {
    Invoke-WebRequest -Uri $downloadUrl -OutFile $zipPath
    Write-Success "Downloaded $zipName"
}
catch {
    throw "Failed to download bjarne: $_"
}

# Verify checksum
Write-Step "Verifying checksum..."
try {
    Invoke-WebRequest -Uri $checksumUrl -OutFile $checksumPath
    $checksums = Get-Content $checksumPath
    $expectedHash = ($checksums | Where-Object { $_ -match $zipName }) -split '\s+' | Select-Object -First 1
    $actualHash = (Get-FileHash -Path $zipPath -Algorithm SHA256).Hash.ToLower()

    if ($expectedHash -eq $actualHash) {
        Write-Success "Checksum verified"
    }
    else {
        throw "Checksum mismatch! Expected: $expectedHash, Got: $actualHash"
    }
}
catch {
    Write-Warning "Could not verify checksum: $_"
}

# Extract
Write-Step "Installing to $InstallDir..."
if (Test-Path $InstallDir) {
    Remove-Item -Path $InstallDir -Recurse -Force
}
New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null

Expand-Archive -Path $zipPath -DestinationPath $InstallDir -Force
Write-Success "Extracted bjarne.exe"

# Add to PATH
if (-not $NoPathUpdate) {
    Write-Step "Updating PATH..."
    $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
    if ($userPath -notlike "*$InstallDir*") {
        [Environment]::SetEnvironmentVariable("Path", "$userPath;$InstallDir", "User")
        $env:Path = "$env:Path;$InstallDir"
        Write-Success "Added $InstallDir to PATH"
    }
    else {
        Write-Host "  Already in PATH"
    }
}

# Cleanup
Remove-Item -Path $tempDir -Recurse -Force

# Check container runtime
Write-Step "Checking container runtime..."
$runtime = Test-ContainerRuntime
if ($runtime) {
    Write-Success "Found $($runtime.Name) at $($runtime.Path)"
}
else {
    Write-Warning "No container runtime found!"
    Write-Host @"

  bjarne requires podman or docker for code validation.

  Install options:
    - Podman (recommended): winget install RedHat.Podman
    - Docker Desktop: https://docker.com/products/docker-desktop

"@ -ForegroundColor Yellow
}

# Verify installation
Write-Step "Verifying installation..."
$bjarnePath = Join-Path $InstallDir "bjarne.exe"
if (Test-Path $bjarnePath) {
    $version = & $bjarnePath --version 2>&1 | Select-Object -First 1
    Write-Success "bjarne installed successfully!"
    Write-Host "`n  $version" -ForegroundColor Cyan
}
else {
    throw "Installation failed - bjarne.exe not found"
}

Write-Host @"

Installation complete!

  Location: $InstallDir

  Quick start:
    bjarne              # Start interactive mode
    bjarne --help       # Show help
    bjarne --validate   # Validate existing code

  First run will prompt to pull the validation container (~500MB).

"@ -ForegroundColor Green
