# Web UI development, with an explicit and verified local Go API target.
# Default: start Go on 8080 and Vite on 3000.
# When Wails is already working: .\scripts\dev.ps1 -UseWailsBackend
param(
    [switch]$UseWailsBackend
)

$ErrorActionPreference = "Stop"
$projectRoot = Split-Path -Parent $PSScriptRoot
if (-not $projectRoot) { $projectRoot = Get-Location }
$backendPort = if ($UseWailsBackend) { 34115 } else { 8080 }
$backendURL = "http://127.0.0.1:$backendPort"
$podcastProbe = "$backendURL/api/v2/podcasts/feeds"
$backendJob = $null
$previousProxyTarget = $env:VIIB_API_PROXY_TARGET

Write-Host "Starting ViiB browser development..." -ForegroundColor Cyan
if ($UseWailsBackend) {
    Write-Host "Reusing the running Wails Go API on $backendURL." -ForegroundColor Yellow
    Write-Host "Leave the Wails app running while using the browser UI." -ForegroundColor Gray
} else {
    # A pre-existing server can serve outdated routes and make all RSS import
    # attempts look like feed failures. Do not silently adopt that process.
    $listeners = @(Get-NetTCPConnection -LocalPort $backendPort -State Listen -ErrorAction SilentlyContinue)
    if ($listeners.Count -gt 0) {
        $pids = ($listeners | Select-Object -ExpandProperty OwningProcess -Unique) -join ", "
        throw "Port $backendPort is already occupied by PID(s) $pids. Close the stale backend first, or use -UseWailsBackend to reuse the working Wails API on port 34115."
    }

    . (Join-Path $PSScriptRoot "podcast-index-ldflags.ps1")
    $null = Get-PodcastIndexLdflags -ProjectRoot $projectRoot
    $devDataDir = Join-Path $env:APPDATA "ViiB-MediaHub-dev"
    Write-Host "Starting Go backend on $backendURL (data: $devDataDir)..." -ForegroundColor Yellow
    $backendScript = {
        param($root, $dataDir, $port, $piKey, $piSecret)
        Set-Location (Join-Path $root "backend")
        $env:CGO_ENABLED = "1"
        if ($piKey) { $env:VIIB_PODCAST_INDEX_API_KEY = $piKey }
        if ($piSecret) { $env:VIIB_PODCAST_INDEX_API_SECRET = $piSecret }
        go run ./cmd/viib -port $port -no-browser -data $dataDir
    }
    $backendJob = Start-Job -ScriptBlock $backendScript -ArgumentList @($projectRoot, $devDataDir, $backendPort, $env:VIIB_PODCAST_INDEX_API_KEY, $env:VIIB_PODCAST_INDEX_API_SECRET)
}

try {
    # The GET endpoint is handled by the same podcast router as RSS preview.
    # Validate JSON shape, not HTTP 200 alone (SPA fallbacks return HTML).
    Write-Host "Checking Podcasts API at $podcastProbe..." -ForegroundColor Yellow
    $backendReady = $false
    $attempts = if ($UseWailsBackend) { 8 } else { 120 }
    for ($attempt = 0; $attempt -lt $attempts; $attempt++) {
        if ($null -ne $backendJob -and $backendJob.State -in @("Failed", "Stopped", "Completed")) {
            break
        }
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
            # Go may still be compiling or the backend may be unavailable.
        }
        Start-Sleep -Milliseconds 500
    }

    if (-not $backendReady) {
        if ($null -ne $backendJob) {
            Receive-Job $backendJob -Keep | Out-Host
        }
        throw "No compatible Podcasts API at $podcastProbe. Verify the Go branch/build and port. With working Wails, use: .\scripts\dev.ps1 -UseWailsBackend"
    }

    # Keep Vite's proxy and the verified backend *identical* for this launch.
    $env:VIIB_API_PROXY_TARGET = $backendURL
    Write-Host "Podcasts API verified. Vite will proxy /api to $backendURL." -ForegroundColor Green
    Write-Host "Starting Vite at http://localhost:3000 (Ctrl+C to stop)." -ForegroundColor Cyan
    Push-Location $projectRoot
    try {
        npm run dev
    } finally {
        Pop-Location
    }
} finally {
    $env:VIIB_API_PROXY_TARGET = $previousProxyTarget
    if ($null -ne $backendJob) {
        Stop-Job $backendJob -ErrorAction SilentlyContinue
        Remove-Job $backendJob -ErrorAction SilentlyContinue
    }
}
