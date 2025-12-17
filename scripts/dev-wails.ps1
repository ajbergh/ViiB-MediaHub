# Development mode for ViiB MediaHub (Wails)
#
# This script runs the Wails application in development mode with hot reload.
# The frontend is served via Vite dev server and the Go backend is rebuilt on changes.
#
# Prerequisites:
#   - Go 1.22+ with CGO_ENABLED=1
#   - Node.js 20+ and npm
#   - Wails CLI (go install github.com/wailsapp/wails/v2/cmd/wails@latest)
#   - MSYS2 mingw-w64-x86_64-gcc (for CGO/SQLite)
#
# Usage:
#   .\scripts\dev-wails.ps1
#
# For web development (browser-based), use: .\scripts\dev.ps1

$ErrorActionPreference = "Stop"

# ============================================================================
# Preflight Checks
# ============================================================================

Write-Host "`n🔍 Checking prerequisites..." -ForegroundColor Cyan

# Check Wails
if (-not (Get-Command wails -ErrorAction SilentlyContinue)) {
    Write-Error @"
❌ Wails CLI not found.

Install Wails with:
  go install github.com/wailsapp/wails/v2/cmd/wails@latest

Then verify with:
  wails doctor
"@
    exit 1
}

# Check GCC
if (-not (Get-Command gcc -ErrorAction SilentlyContinue)) {
    Write-Host "⚠ gcc not found in PATH. CGO build may fail." -ForegroundColor Yellow
    Write-Host "  Install MSYS2 and mingw-w64:" -ForegroundColor Yellow
    Write-Host "    1. Download from https://www.msys2.org/" -ForegroundColor Gray
    Write-Host "    2. Run: pacman -S mingw-w64-x86_64-gcc" -ForegroundColor Gray
    Write-Host "    3. Add C:\msys64\mingw64\bin to PATH" -ForegroundColor Gray
    Write-Host ""
}

# ============================================================================
# Setup
# ============================================================================

$projectRoot = Split-Path -Parent $PSScriptRoot
$wailsDir = Join-Path $projectRoot "backend\cmd\wails"

Write-Host "📁 Project root: $projectRoot" -ForegroundColor Gray
Write-Host "📁 Wails dir: $wailsDir" -ForegroundColor Gray

# Validate Wails directory
if (-not (Test-Path $wailsDir)) {
    Write-Error "❌ Wails directory not found: $wailsDir"
    exit 1
}

if (-not (Test-Path (Join-Path $wailsDir "wails.json"))) {
    Write-Error "❌ wails.json not found in $wailsDir"
    exit 1
}

# ============================================================================
# Ensure frontend dist exists (for embed)
# ============================================================================

$frontendDist = Join-Path $wailsDir "frontend\dist"
if (-not (Test-Path $frontendDist)) {
    Write-Host "`n📦 Frontend dist not found. Building..." -ForegroundColor Yellow
    
    Push-Location $projectRoot
    try {
        npm run build
        if ($LASTEXITCODE -ne 0) { throw "Frontend build failed" }
    } finally {
        Pop-Location
    }
    
    # Copy to Wails location
    $distSource = Join-Path $projectRoot "dist"
    $frontendDir = Split-Path $frontendDist
    if (-not (Test-Path $frontendDir)) {
        New-Item -ItemType Directory -Path $frontendDir -Force | Out-Null
    }
    Copy-Item -Recurse $distSource $frontendDist
    
    Write-Host "  ✓ Frontend built and copied" -ForegroundColor Green
}

# ============================================================================
# Start Wails Dev Mode
# ============================================================================

Write-Host "`n" -NoNewline
Write-Host "═══════════════════════════════════════════════════════════" -ForegroundColor Cyan
Write-Host "  🚀 Starting ViiB MediaHub (Wails Dev Mode)" -ForegroundColor Cyan
Write-Host "═══════════════════════════════════════════════════════════" -ForegroundColor Cyan
Write-Host ""
Write-Host "  • Go backend will rebuild on changes" -ForegroundColor Gray
Write-Host "  • Press Ctrl+C to stop" -ForegroundColor Gray
Write-Host ""

Push-Location $wailsDir
try {
    # Set CGO environment
    $env:CGO_ENABLED = "1"
    
    # Run Wails dev mode
    wails dev
} finally {
    Pop-Location
}
