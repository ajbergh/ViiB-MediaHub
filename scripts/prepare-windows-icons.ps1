# Wails reuses build/windows/icon.ico if it already exists. Always overwrite it
# with the tracked, validated ICO so clean and incremental builds agree.
param(
    [string]$Destination = (Join-Path $PSScriptRoot '..\backend\cmd\wails\build')
)

$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path $PSScriptRoot -Parent
$windowsDir = Join-Path $Destination 'windows'
New-Item -ItemType Directory -Path $windowsDir -Force | Out-Null
Copy-Item -LiteralPath (Join-Path $repoRoot 'assets\icons\appicon.png') -Destination (Join-Path $Destination 'appicon.png') -Force
Copy-Item -LiteralPath (Join-Path $repoRoot 'backend\cmd\wails\appicon.ico') -Destination (Join-Path $windowsDir 'icon.ico') -Force
