# Build script for ViiB MediaHub (Wails Windows build)
# 
# This script builds the native Windows desktop application using Wails.
# Output: build/ViiB-MediaHub.exe
#
# Prerequisites:
#   - Go 1.22+ with CGO_ENABLED=1
#   - Node.js 20+ and npm
#   - Wails CLI (go install github.com/wailsapp/wails/v2/cmd/wails@latest)
#   - MSYS2 mingw-w64-x86_64-gcc (for CGO/SQLite)
#
# Usage:
#   .\scripts\build-wails.ps1
#   .\scripts\build-wails.ps1 -Debug    # Enable debug mode
#   .\scripts\build-wails.ps1 -Clean    # Clean build artifacts first
#
# For web-embedded build (browser-based), use: .\scripts\build.ps1

param(
    [switch]$Debug,
    [switch]$Clean,
    [switch]$SkipFrontend
)

$ErrorActionPreference = "Stop"

# ============================================================================
# Preflight Checks
# ============================================================================

Write-Host "Stopping any existing running Viib MediaHub Process..." -ForegroundColor Cyan
Stop-Process -Name "ViiB-MediaHub" -Force -ErrorAction SilentlyContinue; Start-Sleep -Seconds 1
Write-Host "Building ViiB MediaHub for Wails..." -ForegroundColor Cyan

Write-Host "`n🔍 Checking prerequisites..." -ForegroundColor Cyan

# Check Node.js
if (-not (Get-Command node -ErrorAction SilentlyContinue)) {
    Write-Error "❌ Node.js not found. Install from https://nodejs.org/"
    exit 1
}
$nodeVersion = (node --version)
Write-Host "  ✓ Node.js $nodeVersion" -ForegroundColor Green

# Check npm
if (-not (Get-Command npm -ErrorAction SilentlyContinue)) {
    Write-Error "❌ npm not found. Should be installed with Node.js."
    exit 1
}
$npmVersion = (npm --version)
Write-Host "  ✓ npm $npmVersion" -ForegroundColor Green

# Check Go
if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    Write-Error "❌ Go not found. Install from https://go.dev/dl/"
    exit 1
}
$goVersion = (go version)
Write-Host "  ✓ $goVersion" -ForegroundColor Green

# Make binaries installed with `go install` available even when the Go bin
# directory has not been added to the user's persistent PATH.
$goBin = (go env GOBIN).Trim()
if ([string]::IsNullOrWhiteSpace($goBin)) {
    $goPath = ((go env GOPATH).Trim() -split [IO.Path]::PathSeparator)[0]
    $goBin = Join-Path $goPath "bin"
}
if ((Test-Path $goBin) -and (($env:Path -split ';') -notcontains $goBin)) {
    $env:Path = "$goBin;$env:Path"
}

# Check Wails
if (-not (Get-Command wails -ErrorAction SilentlyContinue)) {
    Write-Error "❌ Wails not found. Install with: go install github.com/wailsapp/wails/v2/cmd/wails@latest"
    exit 1
}
$wailsVersion = (wails version 2>&1 | Select-String -Pattern "v\d+\.\d+\.\d+").Matches.Value
Write-Host "  ✓ Wails $wailsVersion" -ForegroundColor Green

# Check GCC (for CGO/SQLite)
# MSYS2's MinGW toolchain is the supported Windows CGO compiler. Make a
# standard installation discoverable without requiring a machine-wide PATH edit.
if (-not (Get-Command gcc -ErrorAction SilentlyContinue)) {
    foreach ($mingwBin in @('C:\msys64\mingw64\bin', 'C:\msys64\ucrt64\bin')) {
        if ((Test-Path (Join-Path $mingwBin 'gcc.exe')) -and (($env:Path -split ';') -notcontains $mingwBin)) {
            $env:Path = "$mingwBin;$env:Path"
            break
        }
    }
}
if (-not (Get-Command gcc -ErrorAction SilentlyContinue)) {
    Write-Host "  ⚠ gcc not found. CGO build may fail." -ForegroundColor Yellow
    Write-Host "    Install MSYS2 and run: pacman -S mingw-w64-x86_64-gcc" -ForegroundColor Yellow
    Write-Host "    Then add C:\msys64\mingw64\bin to PATH" -ForegroundColor Yellow
} else {
    $gccVersion = (gcc --version | Select-Object -First 1)
    Write-Host "  ✓ $gccVersion" -ForegroundColor Green
}

# ============================================================================
# Setup Paths
# ============================================================================

$projectRoot = Split-Path -Parent $PSScriptRoot
$wailsDir = Join-Path $projectRoot "backend\cmd\wails"
$distSource = Join-Path $projectRoot "dist"
$distDest = Join-Path $wailsDir "frontend\dist"

Write-Host "`n📁 Project root: $projectRoot" -ForegroundColor Gray

# Validate Wails directory exists
if (-not (Test-Path $wailsDir)) {
    Write-Error "❌ Wails directory not found: $wailsDir"
    exit 1
}

# ============================================================================
# Clean (if requested)
# ============================================================================

if ($Clean) {
    Write-Host "`n🧹 Cleaning build artifacts..." -ForegroundColor Yellow
    
    $buildDir = Join-Path $wailsDir "build"
    if (Test-Path $buildDir) {
        Remove-Item -Recurse -Force $buildDir
        Write-Host "  Removed: $buildDir" -ForegroundColor Gray
    }
    
    if (Test-Path $distDest) {
        Remove-Item -Recurse -Force $distDest
        Write-Host "  Removed: $distDest" -ForegroundColor Gray
    }
}

# ============================================================================
# Step 1: Build Frontend
# ============================================================================

if (-not $SkipFrontend) {
    Write-Host "`n📦 [1/3] Building frontend..." -ForegroundColor Yellow
    
    Push-Location $projectRoot
    try {
        # Install dependencies if needed
        if (-not (Test-Path "node_modules")) {
            Write-Host "  Installing npm dependencies..." -ForegroundColor Gray
            npm ci
            if ($LASTEXITCODE -ne 0) { throw "npm ci failed" }
        }
        
        # Build frontend
        npm run build
        if ($LASTEXITCODE -ne 0) { throw "Frontend build failed" }
        
        Write-Host "  ✓ Frontend built successfully" -ForegroundColor Green
    } finally {
        Pop-Location
    }
} else {
    Write-Host "`n⏭️ [1/3] Skipping frontend build (--SkipFrontend)" -ForegroundColor Gray
}

# ============================================================================
# Step 2: Copy Frontend to Wails Location
# ============================================================================

Write-Host "`n📋 [2/3] Copying frontend to Wails..." -ForegroundColor Yellow

if (-not (Test-Path $distSource)) {
    Write-Error "❌ Frontend dist not found: $distSource"
    Write-Error "   Run 'npm run build' first or remove --SkipFrontend flag"
    exit 1
}

# Remove existing and copy fresh
if (Test-Path $distDest) {
    Remove-Item -Recurse -Force $distDest
}

$frontendDir = Split-Path $distDest
if (-not (Test-Path $frontendDir)) {
    New-Item -ItemType Directory -Path $frontendDir -Force | Out-Null
}

Copy-Item -Recurse $distSource $distDest
Write-Host "  ✓ Frontend copied to $distDest" -ForegroundColor Green

# ============================================================================
# Step 3: Build with Wails
# ============================================================================

Write-Host "`n🔨 [3/3] Building Wails application..." -ForegroundColor Yellow

Push-Location $wailsDir
try {
    # Set CGO environment
    $env:CGO_ENABLED = "1"
    
    # Build arguments
    $buildArgs = @("-platform", "windows/amd64")
    
    if (-not $Debug) {
        $buildArgs += @("-ldflags", "-s -w")
    }
    
    # Clean previous build
    $buildBinDir = Join-Path $wailsDir "build\bin"
    if (Test-Path $buildBinDir) {
        Remove-Item -Recurse -Force $buildBinDir
    }
    
    # Run Wails build
    Write-Host "  Running: wails build $($buildArgs -join ' ')" -ForegroundColor Gray
    & wails build @buildArgs
    
    if ($LASTEXITCODE -ne 0) { throw "Wails build failed" }
    
    Write-Host "  ✓ Wails build complete" -ForegroundColor Green
} finally {
    Pop-Location
}

# Copy output to /build
$wailsOutputPath = Join-Path $wailsDir "build\bin\ViiB-MediaHub.exe"
$finalOutputDir = Join-Path $projectRoot "build"
$finalOutputPath = Join-Path $finalOutputDir "ViiB-MediaHub.exe"

if (Test-Path $wailsOutputPath) {
    if (-not (Test-Path $finalOutputDir)) {
        New-Item -ItemType Directory -Path $finalOutputDir -Force | Out-Null
    }
    Copy-Item -Force $wailsOutputPath $finalOutputPath
    Write-Host "  ✓ Copied to $finalOutputPath" -ForegroundColor Green
}

# ============================================================================
# Report Results
# ============================================================================

$outputPath = Join-Path $projectRoot "build\ViiB-MediaHub.exe"

Write-Host "`n" -NoNewline
Write-Host "═══════════════════════════════════════════════════════════" -ForegroundColor Green
Write-Host "  ✅ BUILD COMPLETE" -ForegroundColor Green
Write-Host "═══════════════════════════════════════════════════════════" -ForegroundColor Green

if (Test-Path $outputPath) {
    $fileInfo = Get-Item $outputPath
    $sizeMB = [math]::Round($fileInfo.Length / 1MB, 2)
    
    Write-Host "`n  Output:  $outputPath" -ForegroundColor Cyan
    Write-Host "  Size:    $sizeMB MB" -ForegroundColor Gray
    Write-Host "  Mode:    $(if ($Debug) { 'Debug' } else { 'Release' })" -ForegroundColor Gray
    
    Write-Host "`n  To run:" -ForegroundColor White
    Write-Host "    $outputPath" -ForegroundColor Yellow
    Write-Host "    $outputPath -debug    # With dev tools" -ForegroundColor Gray
} else {
    Write-Host "`n  ⚠ Output not found at expected location" -ForegroundColor Yellow
    Write-Host "    Expected: $outputPath" -ForegroundColor Gray
}

Write-Host ""
