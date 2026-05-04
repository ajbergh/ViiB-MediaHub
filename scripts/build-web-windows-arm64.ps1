# Build script for ViiB MediaHub (Web browser build - Windows ARM64)
#
# Builds the web-embedded binary that serves a local HTTP server and opens
# the default browser. Does NOT require Wails.
# Output: build\ViiB-MediaHub.exe
#
# Prerequisites:
#   - Go 1.22+ with CGO_ENABLED=1
#   - Node.js 20+ and npm
#   - ARM64 cross-compiler (when building on amd64 host):
#       Option A: llvm-mingw (https://github.com/mstorsjo/llvm-mingw/releases)
#                 Use aarch64-w64-mingw32-gcc
#       Option B: MSYS2 clangarm64: pacman -S mingw-w64-clang-aarch64-toolchain
#                 Add C:\msys64\clangarm64\bin to PATH
#
# Usage:
#   .\scripts\build-web-windows-arm64.ps1
#   .\scripts\build-web-windows-arm64.ps1 -Debug
#   .\scripts\build-web-windows-arm64.ps1 -Clean

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

Write-Host "Building ViiB MediaHub (web browser mode) for Windows ARM64..." -ForegroundColor Cyan
Write-Host "`n🔍 Checking prerequisites..." -ForegroundColor Cyan

if (-not (Get-Command node -ErrorAction SilentlyContinue)) {
    Write-Error "❌ Node.js not found. Install from https://nodejs.org/"
    exit 1
}
Write-Host "  ✓ Node.js $(node --version)" -ForegroundColor Green

if (-not (Get-Command npm -ErrorAction SilentlyContinue)) {
    Write-Error "❌ npm not found."
    exit 1
}
Write-Host "  ✓ npm $(npm --version)" -ForegroundColor Green

if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    Write-Error "❌ Go not found. Install from https://go.dev/dl/"
    exit 1
}
Write-Host "  ✓ $(go version)" -ForegroundColor Green

# Check for ARM64 cross-compiler
$arm64Compilers = @("aarch64-w64-mingw32-gcc", "aarch64-w64-mingw32-clang")
$foundCC = $null
foreach ($cc in $arm64Compilers) {
    if (Get-Command $cc -ErrorAction SilentlyContinue) {
        $foundCC = $cc
        break
    }
}

if (-not $foundCC) {
    Write-Host "  ⚠ ARM64 cross-compiler not found. Build may fail." -ForegroundColor Yellow
    Write-Host "    Options:" -ForegroundColor Yellow
    Write-Host "    A) llvm-mingw: https://github.com/mstorsjo/llvm-mingw/releases" -ForegroundColor Gray
    Write-Host "    B) MSYS2: pacman -S mingw-w64-clang-aarch64-toolchain" -ForegroundColor Gray
    Write-Host "       Add C:\msys64\clangarm64\bin to PATH" -ForegroundColor Gray
} else {
    Write-Host "  ✓ ARM64 cross-compiler: $foundCC" -ForegroundColor Green
}

# ============================================================================
# Setup Paths
# ============================================================================

$projectRoot = Split-Path -Parent $PSScriptRoot
$viibDir = Join-Path $projectRoot "backend\cmd\viib"
$distSource = Join-Path $projectRoot "dist"
$distDest = Join-Path $viibDir "dist"
$buildDir = Join-Path $projectRoot "build"
$outputPath = Join-Path $buildDir "ViiB-MediaHub-arm64.exe"

Write-Host "`n📁 Project root: $projectRoot" -ForegroundColor Gray

if (-not (Test-Path $viibDir)) {
    Write-Error "❌ viib cmd directory not found: $viibDir"
    exit 1
}

# ============================================================================
# Clean (if requested)
# ============================================================================

if ($Clean) {
    Write-Host "`n🧹 Cleaning build artifacts..." -ForegroundColor Yellow
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
# Step 2: Copy Frontend to backend embed location
# ============================================================================

Write-Host "`n📋 [2/3] Copying frontend to backend..." -ForegroundColor Yellow

if (-not (Test-Path $distSource)) {
    Write-Error "❌ Frontend dist not found: $distSource"
    exit 1
}

if (Test-Path $distDest) {
    Remove-Item -Recurse -Force $distDest
}

Copy-Item -Recurse $distSource $distDest
Write-Host "  ✓ Frontend copied to $distDest" -ForegroundColor Green

# ============================================================================
# Step 3: Build Go backend (Windows/arm64)
# ============================================================================

Write-Host "`n🔨 [3/3] Building Go backend (windows/arm64)..." -ForegroundColor Yellow

if (-not (Test-Path $buildDir)) {
    New-Item -ItemType Directory -Path $buildDir -Force | Out-Null
}

Push-Location (Join-Path $projectRoot "backend")
try {
    go mod tidy

    $env:CGO_ENABLED = "1"
    $env:GOOS = "windows"
    $env:GOARCH = "arm64"

    if ($foundCC) {
        $env:CC = $foundCC
    }

    $ldflags = "-s -w -H windowsgui"
    if ($Debug) { $ldflags = "-H windowsgui" }

    go build -ldflags="$ldflags" -o $outputPath ./cmd/viib

    if ($LASTEXITCODE -ne 0) { throw "Go build failed" }

    Write-Host "  ✓ Go build complete" -ForegroundColor Green
} finally {
    Remove-Item Env:GOOS -ErrorAction SilentlyContinue
    Remove-Item Env:GOARCH -ErrorAction SilentlyContinue
    Remove-Item Env:CC -ErrorAction SilentlyContinue
    Pop-Location
}

# ============================================================================
# Report Results
# ============================================================================

Write-Host "`n"
Write-Host "═══════════════════════════════════════════════════════════" -ForegroundColor Green
Write-Host "  ✅ BUILD COMPLETE (Windows ARM64 web browser build)" -ForegroundColor Green
Write-Host "═══════════════════════════════════════════════════════════" -ForegroundColor Green

if (Test-Path $outputPath) {
    $fileInfo = Get-Item $outputPath
    $sizeMB = [math]::Round($fileInfo.Length / 1MB, 2)

    Write-Host "`n  Output:  $outputPath" -ForegroundColor Cyan
    Write-Host "  Size:    $sizeMB MB" -ForegroundColor Gray
    Write-Host "  Arch:    arm64" -ForegroundColor Gray
    Write-Host "  Mode:    $(if ($Debug) { 'Debug' } else { 'Release' })" -ForegroundColor Gray
    Write-Host "`n  To run:" -ForegroundColor White
    Write-Host "    $outputPath" -ForegroundColor Yellow
} else {
    Write-Host "`n  ⚠ Output not found at expected location" -ForegroundColor Yellow
    Write-Host "    Expected: $outputPath" -ForegroundColor Gray
}

Write-Host ""
