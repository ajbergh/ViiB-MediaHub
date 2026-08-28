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

$advisoryIds = @(
    [regex]::Matches($scanOutput, 'GO-\d{4}-\d+') |
        ForEach-Object { $_.Value } |
        Sort-Object -Unique
)

# Binary-mode govulncheck can overmatch packages in the stripped Windows Wails
# executable. Keep exceptions fail-closed: every reported advisory must be a
# specifically known Windows binary overmatch, and an independent production
# package-graph check must confirm that the affected package is not imported.
$knownWindowsBinaryOvermatches = @{
    'GO-2026-5932' = '^golang\.org/x/crypto/openpgp($|/)'
    'GO-2026-6303' = '^golang\.org/x/crypto/ssh($|/)'
}

Push-Location $backendRoot
try {
    $productionDependencies = & go list -tags 'desktop,production' -deps ./cmd/wails 2>&1
    if ($LASTEXITCODE -ne 0) {
        throw "Unable to verify the Wails production dependency graph.`n$productionDependencies"
    }
} finally {
    Pop-Location
}

$unexpectedAdvisories = @(
    $advisoryIds |
        Where-Object { -not $knownWindowsBinaryOvermatches.ContainsKey($_) }
)

if ($unexpectedAdvisories.Count -gt 0) {
    Write-Error "Windows binary vulnerability scan failed with unexpected advisories: $($unexpectedAdvisories -join ', ')"
    exit 1
}

$linkedKnownAdvisories = @()
foreach ($advisoryId in $advisoryIds) {
    $packagePattern = $knownWindowsBinaryOvermatches[$advisoryId]
    $matchingDependencies = @(
        $productionDependencies |
            Where-Object { $_ -match $packagePattern }
    )

    if ($matchingDependencies.Count -gt 0) {
        $linkedKnownAdvisories += $advisoryId
    }
}

if ($advisoryIds.Count -gt 0 -and $linkedKnownAdvisories.Count -eq 0) {
    Write-Warning "Allowing known Windows binary-mode overmatch(es): $($advisoryIds -join ', '). Production dependency graph confirms the affected packages are not imported."
    exit 0
}

if ($linkedKnownAdvisories.Count -gt 0) {
    Write-Error "Windows binary vulnerability scan failed. Affected package(s) are present in the production dependency graph for: $($linkedKnownAdvisories -join ', ')"
    exit 1
}

Write-Error "Windows binary vulnerability scan failed without a recognized advisory."
exit 1
