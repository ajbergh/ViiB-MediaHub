# Build script for ViiB MediaHub
# This script builds the frontend and embeds it into the Go binary

$ErrorActionPreference = "Stop"

Write-Host "🎵 Building ViiB MediaHub..." -ForegroundColor Cyan

# Get the script's directory (project root)
$projectRoot = Split-Path -Parent $PSScriptRoot
if (-not $projectRoot) {
    $projectRoot = Get-Location
}

Write-Host "Project root: $projectRoot"

# Step 1: Build Frontend
Write-Host "`n📦 Building frontend..." -ForegroundColor Yellow
Push-Location $projectRoot
try {
    npm run build
    if ($LASTEXITCODE -ne 0) {
        throw "Frontend build failed"
    }
} finally {
    Pop-Location
}

# Step 2: Copy frontend dist to backend embed location
Write-Host "`n📋 Copying frontend to backend..." -ForegroundColor Yellow
$distSource = Join-Path $projectRoot "dist"
$distDest = Join-Path $projectRoot "backend\cmd\viib\dist"

if (Test-Path $distDest) {
    Remove-Item -Recurse -Force $distDest
}

Copy-Item -Recurse $distSource $distDest

# Step 3: Build Go backend
Write-Host "`n🔨 Building Go backend..." -ForegroundColor Yellow
Push-Location (Join-Path $projectRoot "backend")
try {
    # Get dependencies
    go mod tidy
    
    # Build for Windows
    $env:CGO_ENABLED = "1"
    $env:GOOS = "windows"
    $env:GOARCH = "amd64"
    
    $outputPath = Join-Path $projectRoot "build\ViiB-MediaHub.exe"
    $outputDir = Split-Path $outputPath
    
    if (-not (Test-Path $outputDir)) {
        New-Item -ItemType Directory -Path $outputDir | Out-Null
    }
    
    go build -ldflags="-s -w -H windowsgui" -o $outputPath ./cmd/viib
    
    if ($LASTEXITCODE -ne 0) {
        throw "Go build failed"
    }
    
    Write-Host "`n✅ Build complete!" -ForegroundColor Green
    Write-Host "Output: $outputPath" -ForegroundColor Cyan
    
    # Get file size
    $size = (Get-Item $outputPath).Length / 1MB
    Write-Host "Size: $([math]::Round($size, 2)) MB" -ForegroundColor Gray
    
} finally {
    Pop-Location
}

Write-Host "`n🚀 To run: .\build\ViiB-MediaHub.exe" -ForegroundColor Magenta
