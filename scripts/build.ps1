# Build script for ViiB MediaHub
# This script builds the frontend and embeds it into the Go binary

$ErrorActionPreference = "Stop"

# Preflight checks for common tooling
if (-not (Get-Command node -ErrorAction SilentlyContinue)) {
    Write-Error "Node is not installed. Please install Node.js (LTS) and retry: https://nodejs.org/"
    exit 1
}
if (-not (Get-Command npm -ErrorAction SilentlyContinue)) {
    Write-Error "npm not found. Please ensure npm is installed with Node.js and in PATH."
    exit 1
}
if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    Write-Error "Go (golang) not found. Please install Go and retry: https://go.dev/dl/"
    exit 1
}
if (-not (Get-Command gcc -ErrorAction SilentlyContinue)) {
    Write-Host "Warning: 'gcc' not found in PATH. If build fails due to CGO, install a suitable gcc (mingw64) for windows build." -ForegroundColor Yellow
}

# Warn user about PowerShell version/encoding concerns
if ($PSVersionTable.PSVersion.Major -lt 6) {
    Write-Host "Note: Running Windows PowerShell < 6 detected. Ensure this file is saved as 'UTF-8 with BOM' or run with PowerShell 7+ (pwsh) to avoid encoding issues." -ForegroundColor Yellow
}

Write-Host "Stopping any existing running Viib MediaHub Process..." -ForegroundColor Cyan
Stop-Process -Name "ViiB-MediaHub" -Force -ErrorAction SilentlyContinue; Start-Sleep -Seconds 1
Write-Host "Building ViiB MediaHub for Web..." -ForegroundColor Cyan

# Get the script's directory (project root)
$projectRoot = Split-Path -Parent $PSScriptRoot
if (-not $projectRoot) {
    $projectRoot = Get-Location
}

Write-Host "Project root: $projectRoot"

# Step 1: Build Frontend
Write-Host "`n[Frontend] Building..." -ForegroundColor Yellow
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
Write-Host "`n[Copy] Copying frontend to backend..." -ForegroundColor Yellow
$distSource = Join-Path $projectRoot "dist"
$distDest = Join-Path $projectRoot "backend\cmd\viib\dist"

if (Test-Path $distDest) {
    Remove-Item -Recurse -Force $distDest
}

Copy-Item -Recurse $distSource $distDest

# Step 3: Build Go backend
Write-Host "`n[Go] Building Go backend..." -ForegroundColor Yellow
Push-Location (Join-Path $projectRoot "backend")
try {
    # Get dependencies
    go mod tidy
    
    # Check for icon
    $iconPath = Join-Path $projectRoot "backend\cmd\viib\icon.ico"
    if (-not (Test-Path $iconPath)) {
        Write-Error "icon.ico not found in backend/cmd/viib. Please place the icon file there."
        exit 1
    }

    # Install rsrc if needed
    if (-not (Get-Command rsrc -ErrorAction SilentlyContinue)) {
        Write-Host "Installing rsrc..."
        go install github.com/akavel/rsrc@latest
        # Add go bin to path for this session if needed
        $goBin = Join-Path $env:USERPROFILE "go\bin"
        $env:Path += ";$goBin"
    }

    # Generate syso
    Write-Host "Generating Windows resources..."
    Push-Location (Join-Path $projectRoot "backend\cmd\viib")
    try {
        rsrc -ico icon.ico -o rsrc.syso
    } finally {
        Pop-Location
    }

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
    
    Write-Host "`nBuild complete!" -ForegroundColor Green
    Write-Host "Output: $outputPath" -ForegroundColor Cyan
    
    # Get file size
    $size = (Get-Item $outputPath).Length / 1MB
    Write-Host "Size: $([math]::Round($size, 2)) MB" -ForegroundColor Gray
    
} finally {
    Pop-Location
}

Write-Host "`nTo run: .\build\ViiB-MediaHub.exe" -ForegroundColor Magenta
