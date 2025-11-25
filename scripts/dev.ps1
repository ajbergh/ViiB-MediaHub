# Development script - runs frontend and backend separately for hot-reload

$ErrorActionPreference = "Stop"

Write-Host "🎵 Starting ViiB MediaHub in development mode..." -ForegroundColor Cyan

$projectRoot = Split-Path -Parent $PSScriptRoot
if (-not $projectRoot) {
    $projectRoot = Get-Location
}

# Start Go backend in one terminal
Write-Host "`n🔧 Starting Go backend on port 8080..." -ForegroundColor Yellow
$backendJob = Start-Job -ScriptBlock {
    param($root)
    Set-Location (Join-Path $root "backend")
    $env:CGO_ENABLED = "1"
    go run ./cmd/viib -port 8080 -no-browser
} -ArgumentList $projectRoot

# Give backend time to start
Start-Sleep -Seconds 2

# Start frontend dev server
Write-Host "`n⚡ Starting Vite dev server on port 3000..." -ForegroundColor Yellow
Write-Host "Frontend will proxy API requests to backend at :8080" -ForegroundColor Gray
Write-Host "`nPress Ctrl+C to stop both servers" -ForegroundColor Magenta

Push-Location $projectRoot
try {
    npm run dev
} finally {
    Pop-Location
    Stop-Job $backendJob -ErrorAction SilentlyContinue
    Remove-Job $backendJob -ErrorAction SilentlyContinue
}
