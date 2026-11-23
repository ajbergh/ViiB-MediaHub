# Loads optional local .env / .env.local Podcast Index credentials without
# overwriting explicit environment variables and returns Go -ldflags flags for
# build-time embedding in local desktop and web builds.

function Import-LocalDotEnv {
    param([string]$ProjectRoot)

    foreach ($envFile in @('.env', '.env.local')) {
        $fullPath = Join-Path $ProjectRoot $envFile
        if (-not (Test-Path $fullPath)) { continue }
        foreach ($rawLine in Get-Content -Path $fullPath -Encoding UTF8) {
            $line = $rawLine.Trim()
            if ([string]::IsNullOrWhiteSpace($line) -or $line.StartsWith('#')) { continue }
            $idx = $line.IndexOf('=')
            if ($idx -le 0) { continue }
            $name = $line.Substring(0, $idx).Trim()
            $val = $line.Substring($idx + 1).Trim()
            if (($val.StartsWith('"') -and $val.EndsWith('"')) -or ($val.StartsWith("'") -and $val.EndsWith("'"))) {
                if ($val.Length -ge 2) { $val = $val.Substring(1, $val.Length - 2) }
            }
            if ([string]::IsNullOrWhiteSpace($name)) { continue }
            $existing = [Environment]::GetEnvironmentVariable($name, 'Process')
            if ([string]::IsNullOrWhiteSpace($existing) -and -not [string]::IsNullOrWhiteSpace($val)) {
                [Environment]::SetEnvironmentVariable($name, $val, 'Process')
            }
        }
    }
}

function Get-PodcastIndexLdflags {
    param([string]$ProjectRoot)

    Import-LocalDotEnv -ProjectRoot $ProjectRoot

    $apiKey = $env:VIIB_PODCAST_INDEX_API_KEY
    if ([string]::IsNullOrWhiteSpace($apiKey)) { $apiKey = $env:PODCAST_INDEX_API_KEY }

    $apiSecret = $env:VIIB_PODCAST_INDEX_API_SECRET
    if ([string]::IsNullOrWhiteSpace($apiSecret)) { $apiSecret = $env:PODCAST_INDEX_API_SECRET }

    if ([string]::IsNullOrWhiteSpace($apiKey) -or [string]::IsNullOrWhiteSpace($apiSecret)) {
        return ""
    }

    # Also keep VIIB_* process env populated for `go run` / `wails dev`.
    $env:VIIB_PODCAST_INDEX_API_KEY = $apiKey.Trim()
    $env:VIIB_PODCAST_INDEX_API_SECRET = $apiSecret.Trim()

    $pkg = "github.com/ajbergh/viib-mediahub/internal/podcast"
    $k = $apiKey.Trim().Replace("'", "")
    $s = $apiSecret.Trim().Replace("'", "")
    return "-X '${pkg}.embeddedDirectoryAPIKey=${k}' -X '${pkg}.embeddedDirectoryAPISecret=${s}'"
}
