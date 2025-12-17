<#
.SYNOPSIS
    Smoke test for ViiB MediaHub Wails build

.DESCRIPTION
    Verifies the Wails-built executable starts correctly and responds to basic checks.

.PARAMETER ExePath
    Path to the ViiB-MediaHub.exe. Defaults to the Wails build output location.

.PARAMETER Timeout
    Seconds to wait for app startup. Default: 10

.EXAMPLE
    .\scripts\smoke-test.ps1
    
.EXAMPLE
    .\scripts\smoke-test.ps1 -ExePath "C:\path\to\ViiB-MediaHub.exe" -Timeout 15
#>

param(
    [string]$ExePath = "backend\cmd\wails\build\bin\ViiB-MediaHub.exe",
    [int]$Timeout = 10
)

$ErrorActionPreference = "Stop"

function Write-Status {
    param([string]$Message, [string]$Type = "Info")
    switch ($Type) {
        "Success" { Write-Host "✅ $Message" -ForegroundColor Green }
        "Error"   { Write-Host "❌ $Message" -ForegroundColor Red }
        "Warning" { Write-Host "⚠️  $Message" -ForegroundColor Yellow }
        "Info"    { Write-Host "ℹ️  $Message" -ForegroundColor Cyan }
    }
}

# --------------------------------------------------
# Pre-flight checks
# --------------------------------------------------

Write-Host ""
Write-Host "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━" -ForegroundColor DarkGray
Write-Host " ViiB MediaHub Smoke Test" -ForegroundColor Cyan
Write-Host "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━" -ForegroundColor DarkGray
Write-Host ""

# Resolve path
$ExePath = [System.IO.Path]::GetFullPath($ExePath)

if (-not (Test-Path $ExePath)) {
    Write-Status "Executable not found: $ExePath" -Type "Error"
    Write-Host ""
    Write-Host "Build the application first:" -ForegroundColor Yellow
    Write-Host "  .\scripts\build-wails.ps1" -ForegroundColor White
    exit 1
}

$fileInfo = Get-Item $ExePath
$sizeMB = [math]::Round($fileInfo.Length / 1MB, 2)
Write-Status "Found executable: $ExePath ($sizeMB MB)"

# --------------------------------------------------
# Start application
# --------------------------------------------------

Write-Host ""
Write-Status "Starting ViiB MediaHub..."

$process = $null
try {
    $process = Start-Process -FilePath $ExePath -PassThru -WindowStyle Normal
    Write-Status "Process started with PID: $($process.Id)"
}
catch {
    Write-Status "Failed to start process: $_" -Type "Error"
    exit 1
}

# --------------------------------------------------
# Wait for startup
# --------------------------------------------------

Write-Status "Waiting $Timeout seconds for startup..."
$startTime = Get-Date
$startupComplete = $false

for ($i = 1; $i -le $Timeout; $i++) {
    Start-Sleep -Seconds 1
    
    if ($process.HasExited) {
        Write-Status "Process exited unexpectedly after $i seconds" -Type "Error"
        Write-Host "Exit code: $($process.ExitCode)" -ForegroundColor Yellow
        exit 1
    }
    
    # Check if window is visible (basic responsiveness check)
    $hwnd = $process.MainWindowHandle
    if ($hwnd -ne [IntPtr]::Zero) {
        $startupComplete = $true
        Write-Status "Window detected after $i seconds" -Type "Success"
        break
    }
    
    Write-Host "  [$i/$Timeout] Waiting..." -ForegroundColor DarkGray
}

# --------------------------------------------------
# Verify running state
# --------------------------------------------------

Write-Host ""

if ($process.HasExited) {
    Write-Status "Process not running" -Type "Error"
    exit 1
}

Write-Status "Process is running" -Type "Success"

# Check memory usage
$mem = [math]::Round($process.WorkingSet64 / 1MB, 2)
Write-Status "Memory usage: $mem MB"

# Check CPU time
$cpu = $process.TotalProcessorTime.TotalSeconds
Write-Status "CPU time: $([math]::Round($cpu, 2)) seconds"

# --------------------------------------------------
# Cleanup
# --------------------------------------------------

Write-Host ""
Write-Status "Stopping application..."

try {
    Stop-Process -Id $process.Id -Force -ErrorAction SilentlyContinue
    Start-Sleep -Seconds 1
    
    if (-not $process.HasExited) {
        # Force kill if still running
        taskkill /F /PID $process.Id 2>$null
    }
    
    Write-Status "Application stopped" -Type "Success"
}
catch {
    Write-Status "Error stopping application: $_" -Type "Warning"
}

# --------------------------------------------------
# Summary
# --------------------------------------------------

Write-Host ""
Write-Host "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━" -ForegroundColor DarkGray
Write-Host " Smoke Test Results" -ForegroundColor Cyan
Write-Host "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━" -ForegroundColor DarkGray
Write-Host ""
Write-Status "All checks passed!" -Type "Success"
Write-Host ""

exit 0
