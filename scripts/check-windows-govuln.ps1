param(
    [Parameter(Mandatory = $true)]
    [string]$BinaryPath
)

$ErrorActionPreference = 'Stop'

$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$backendRoot = Join-Path $repoRoot 'backend'
$resolvedBinary = (Resolve-Path (Join-Path $repoRoot $BinaryPath)).Path

Write-Host "Scanning Windows binary: $resolvedBinary"
$scanOutput = (& govulncheck -mode binary $resolvedBinary 2>&1 | Out-String)
$scanExitCode = $LASTEXITCODE
Write-Host $scanOutput

if ($scanExitCode -eq 0) {
    Write-Host 'Windows binary vulnerability scan passed.'
    exit 0
}

$advisoryIds = [regex]::Matches($scanOutput, 'GO-\d{4}-\d+') |
    ForEach-Object { $_.Value } |
    Sort-Object -Unique

# Binary-mode govulncheck currently overmatches GO-2026-5932 in the stripped
# Windows Wails executable even though the production package graph does not
# import golang.org/x/crypto/openpgp. Keep this exception fail-closed: it is
# accepted only when it is the sole advisory and an independent tagged package
# graph check confirms that no OpenPGP package is linked by application source.
Push-Location $backendRoot
try {
    $productionDependencies = & go list -tags 'desktop,production' -deps ./cmd/wails 2>&1
    if ($LASTEXITCODE -ne 0) {
        throw "Unable to verify the Wails production dependency graph.`n$productionDependencies"
    }
} finally {
    Pop-Location
}

$openPgpDependencies = $productionDependencies |
    Where-Object { $_ -match '^golang\.org/x/crypto/openpgp($|/)' }

$onlyKnownFalsePositive =
    $advisoryIds.Count -eq 1 -and
    $advisoryIds[0] -eq 'GO-2026-5932' -and
    -not $openPgpDependencies

if ($onlyKnownFalsePositive) {
    Write-Warning 'Allowing GO-2026-5932: production dependency graph confirms golang.org/x/crypto/openpgp is not imported.'
    exit 0
}

Write-Error "Windows binary vulnerability scan failed. Advisories: $($advisoryIds -join ', ')"
exit 1
