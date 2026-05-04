# Build script for ViiB MediaHub (Wails Windows ARM64 build)
#
# Builds the native Windows ARM64 desktop application using Wails.
# Output: build/ViiB-MediaHub-arm64.exe
#
# Prerequisites:
#   - Go 1.22+ with CGO_ENABLED=1
#   - Node.js 20+ and npm
#   - Wails CLI: go install github.com/wailsapp/wails/v2/cmd/wails@latest
#   - LLVM/Clang cross-compiler for arm64 Windows:
#       Option A: llvm-mingw (https://github.com/mstorsjo/llvm-mingw/releases)
#                 Add llvm-mingw/bin to PATH, use aarch64-w64-mingw32-gcc
#       Option B: MSYS2 clang64 (pacman -S mingw-w64-clang-aarch64-toolchain)
#                 Add C:\msys64\clangarm64\bin to PATH
#
# Usage:
#   .\scripts\build-wails-windows-arm64.ps1
#   .\scripts\build-wails-windows-arm64.ps1 -Debug
#   .\scripts\build-wails-windows-arm64.ps1 -Clean

param(
    [switch]$Debug,
    [switch]$Clean,
    [switch]$SkipFrontend
)

$ErrorActionPreference = "Stop"

# ============================================================================
# Preflight Checks
# ============================================================================

Write-Host "Stopping any existing running ViiB MediaHub Process..." -ForegroundColor Cyan
Stop-Process -Name "ViiB-MediaHub" -Force -ErrorAction SilentlyContinue

Write-Host "`n🔍 Checking prerequisites..." -ForegroundColor Cyan

# Check Node.js
if (-not (Get-Command node -ErrorAction SilentlyContinue)) {
    Write-Error "❌ Node.js not found. Install from https://nodejs.org/"
    exit 1
}
Write-Host "  ✓ Node.js $(node --version)" -ForegroundColor Green

# Check npm
if (-not (Get-Command npm -ErrorAction SilentlyContinue)) {
    Write-Error "❌ npm not found. Should be installed with Node.js."
    exit 1
}
Write-Host "  ✓ npm $(npm --version)" -ForegroundColor Green

# Check Go
if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    Write-Error "❌ Go not found. Install from https://go.dev/dl/"
    exit 1
}
Write-Host "  ✓ $(go version)" -ForegroundColor Green

# Check Wails
if (-not (Get-Command wails -ErrorAction SilentlyContinue)) {
    Write-Error @"
❌ Wails CLI not found.
Install with: go install github.com/wailsapp/wails/v2/cmd/wails@latest
"@
    exit 1
}
$wailsVersion = (wails version 2>&1 | Select-String -Pattern "v\d+\.\d+\.\d+").Matches.Value
Write-Host "  ✓ Wails $wailsVersion" -ForegroundColor Green

# Check for ARM64 cross-compiler
# Try llvm-mingw aarch64 first, then MSYS2 clangarm64
$arm64Compilers = @(
    "aarch64-w64-mingw32-gcc",   # llvm-mingw
    "aarch64-w64-mingw32-clang"  # MSYS2 clangarm64 variant
)
$foundCC = $null
foreach ($cc in $arm64Compilers) {
    if (Get-Command $cc -ErrorAction SilentlyContinue) {
        $foundCC = $cc
        break
    }
}

if (-not $foundCC) {
    Write-Host "  ⚠ ARM64 cross-compiler not found." -ForegroundColor Yellow
    Write-Host "    Options:" -ForegroundColor Yellow
    Write-Host "    A) llvm-mingw: https://github.com/mstorsjo/llvm-mingw/releases" -ForegroundColor Gray
    Write-Host "       Add <llvm-mingw>/bin to PATH" -ForegroundColor Gray
    Write-Host "    B) MSYS2 clangarm64: pacman -S mingw-w64-clang-aarch64-toolchain" -ForegroundColor Gray
    Write-Host "       Add C:\msys64\clangarm64\bin to PATH" -ForegroundColor Gray
    Write-Host "    Build may fail without a cross-compiler." -ForegroundColor Yellow
    Write-Host ""
} else {
    Write-Host "  ✓ ARM64 cross-compiler: $foundCC" -ForegroundColor Green
}

# ============================================================================
# Setup Paths
# ============================================================================

$projectRoot = Split-Path -Parent $PSScriptRoot
$wailsDir = Join-Path $projectRoot "backend\cmd\wails"
$distSource = Join-Path $projectRoot "dist"
$distDest = Join-Path $wailsDir "frontend\dist"

Write-Host "`n📁 Project root: $projectRoot" -ForegroundColor Gray

if (-not (Test-Path $wailsDir)) {
    Write-Error "❌ Wails directory not found: $wailsDir"
    exit 1
}

# ============================================================================
# Clean (if requested)
# ============================================================================

if ($Clean) {
    Write-Host "`n🧹 Cleaning build artifacts..." -ForegroundColor Yellow
    $buildBinDir = Join-Path $wailsDir "build\bin"
    if (Test-Path $buildBinDir) {
        Remove-Item -Recurse -Force $buildBinDir
        Write-Host "  Removed: $buildBinDir" -ForegroundColor Gray
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
        if (-not (Test-Path "node_modules")) {
            Write-Host "  Installing npm dependencies..." -ForegroundColor Gray
            npm ci
            if ($LASTEXITCODE -ne 0) { throw "npm ci failed" }
        }

        npm run build
        if ($LASTEXITCODE -ne 0) { throw "Frontend build failed" }

        Write-Host "  ✓ Frontend built successfully" -ForegroundColor Green
    } finally {
        Pop-Location
    }
} else {
    Write-Host "`n⏭️  [1/3] Skipping frontend build (-SkipFrontend)" -ForegroundColor Gray
}

# ============================================================================
# Step 2: Copy Frontend to Wails Location
# ============================================================================

Write-Host "`n📋 [2/3] Copying frontend to Wails..." -ForegroundColor Yellow

if (-not (Test-Path $distSource)) {
    Write-Error "❌ Frontend dist not found: $distSource"
    exit 1
}

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
# Step 3: Build with Wails (Windows/arm64)
# ============================================================================

Write-Host "`n🔨 [3/3] Building Wails application (windows/arm64)..." -ForegroundColor Yellow

Push-Location $wailsDir
try {
    $env:CGO_ENABLED = "1"
    $env:GOARCH = "arm64"
    $env:GOOS = "windows"

    # Set CC to ARM64 cross-compiler if found
    if ($foundCC) {
        $env:CC = $foundCC
    }

    $buildArgs = @("-platform", "windows/arm64")

    if (-not $Debug) {
        $buildArgs += @("-ldflags", "-s -w")
    }

    # Clean previous bin
    $buildBinDir = Join-Path $wailsDir "build\bin"
    if (Test-Path $buildBinDir) {
        Remove-Item -Recurse -Force $buildBinDir
    }

    Write-Host "  Running: wails build $($buildArgs -join ' ')" -ForegroundColor Gray
    & wails build @buildArgs

    if ($LASTEXITCODE -ne 0) { throw "Wails build failed" }

    Write-Host "  ✓ Wails build complete" -ForegroundColor Green
} finally {
    # Reset env vars
    Remove-Item Env:GOARCH -ErrorAction SilentlyContinue
    Remove-Item Env:GOOS -ErrorAction SilentlyContinue
    Remove-Item Env:CC -ErrorAction SilentlyContinue
    Pop-Location
}

# Copy output to /build
$wailsOutputPath = Join-Path $wailsDir "build\bin\ViiB-MediaHub.exe"
$finalOutputDir = Join-Path $projectRoot "build"
$finalOutputPath = Join-Path $finalOutputDir "ViiB-MediaHub-arm64.exe"

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

$outputPath = Join-Path $projectRoot "build\ViiB-MediaHub-arm64.exe"

Write-Host "`n"
Write-Host "═══════════════════════════════════════════════════════════" -ForegroundColor Green
Write-Host "  ✅ BUILD COMPLETE (Windows ARM64)" -ForegroundColor Green
Write-Host "═══════════════════════════════════════════════════════════" -ForegroundColor Green

if (Test-Path $outputPath) {
    $fileInfo = Get-Item $outputPath
    $sizeMB = [math]::Round($fileInfo.Length / 1MB, 2)

    Write-Host "`n  Output:  $outputPath" -ForegroundColor Cyan
    Write-Host "  Size:    $sizeMB MB" -ForegroundColor Gray
    Write-Host "  Arch:    arm64" -ForegroundColor Gray
    Write-Host "  Mode:    $(if ($Debug) { 'Debug' } else { 'Release' })" -ForegroundColor Gray
} else {
    Write-Host "`n  ⚠ Output not found at expected location" -ForegroundColor Yellow
    Write-Host "    Expected: $outputPath" -ForegroundColor Gray
}

Write-Host ""
