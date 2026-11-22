# Development script - runs frontend and backend separately for hot-reload

$ErrorActionPreference = "Stop"

Write-Host "🎵 Starting ViiB MediaHub in development mode..." -ForegroundColor Cyan

$projectRoot = Split-Path -Parent $PSScriptRoot
if (-not $projectRoot) {
    $projectRoot = Get-Location
}

# Start Go backend in one terminal
Write-Host "`n🔧 Starting Go backend on port 8080..." -ForegroundColor Yellow
$devDataDir = Join-Path $env:APPDATA "ViiB-MediaHub-dev"
Write-Host "Using development data directory: $devDataDir" -ForegroundColor Gray

$backendScript = {
    param($root, $dataDir)
    Set-Location (Join-Path $root "backend")
    $env:CGO_ENABLED = "1"
    go run ./cmd/viib -port 8080 -no-browser -data $dataDir
}
$backendJob = Start-Job -ScriptBlock $backendScript -ArgumentList @($projectRoot, $devDataDir)

# Fail fast if the Go backend is not running on the Vite proxy port
# or this branch's podcast router is absent from an older executable.
# Go's first compilation can take longer than an arbitrary two-second sleep.
Write-Host "Checking Go API and Podcasts route on port 8080..." -ForegroundColor Yellow
$backendReady = $false
$podcastProbe = "http://127.0.0.1:8080/api/v2/podcasts/feeds"
for ($attempt = 0; $attempt -lt 120; $attempt++) {
    if ($backendJob.State -in @("Failed", "Stopped", "Completed")) { break }
    try {
        $response = Invoke-WebRequest -Uri $podcastProbe -UseBasicParsing -TimeoutSec 2 -ErrorAction Stop
        if ($response.StatusCode -eq 200) {
            $payload = $response.Content | ConvertFrom-Json
            if ($payload.PSObject.Properties.Name -contains "feeds") {
                $backendReady = $true
                break
            }
        }
    } catch {
        # Startup or package compilation can still be in progress.
    }
    Start-Sleep -Milliseconds 500
}
if (-not $backendReady) {
    Write-Host "The Go backend does not expose the Podcasts API at $podcastProbe" -ForegroundColor Red
    Write-Host "Check for an old process on port 8080 or a Go build/startup failure." -ForegroundColor Yellow
    Receive-Job $backendJob -Keep | Out-Host
    Stop-Job $backendJob -ErrorAction SilentlyContinue
    Remove-Job $backendJob -ErrorAction SilentlyContinue
    throw "Cannot start Vite with a mismatched or unavailable Go Podcasts backend."
}

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
