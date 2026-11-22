# ADR 0003 — Podcast directory credentials and fallback policy

- **Date:** 2026-11-22
- **Status:** Accepted
- **Scope:** `POD-00B`, `POD-02A`
- **Depends on:** [ADR 0001](0001-podcasts-local-first-boundaries.md)

## Context

ViiB MediaHub is a local-first desktop (Wails) and local-server/browser
application distributed as source code and compiled client binaries. Users expect
to search for podcasts and browse trending shows in addition to subscribing
directly via RSS URLs or OPML files.

Podcast directory APIs such as Podcast Index require API key/secret
authentication (`X-Auth-Key` and SHA-1 `Authorization` headers) and enforce rate
limits and terms of service, including attribution requirements. Embedding a
shared developer API key/secret in distributed desktop binaries would expose the
secret to extraction, create single-key rate-limit exhaustion across all users,
and violate credential hygiene.

## Decision

### 1. Build-time embedded credentials (no secrets in git) + optional user override

ViiB will **never** commit a hardcoded Podcast Index API key or secret into
source control. Instead, official release builds and developer local builds
inject the maintainer's Podcast Index API key and secret **at compile time**
via Go `-ldflags` (`internal/podcast.embeddedDirectoryAPIKey` and
`internal/podcast.embeddedDirectoryAPISecret`) from:

- **GitHub Actions Releases / CI:** repository secrets
  `PODCAST_INDEX_API_KEY` and `PODCAST_INDEX_API_SECRET` in
  `.github/workflows/release.yml` and `.github/workflows/build-windows.yml`.
- **Local builds & dev runs:** environment variables or a gitignored `.env` /
  `.env.local` file loaded by `scripts/podcast-index-ldflags.ps1` /
  `scripts/podcast-index-ldflags.sh` (`PODCAST_INDEX_API_KEY` /
  `PODCAST_INDEX_API_SECRET` or `VIIB_PODCAST_INDEX_*`).

Credential resolution order at runtime:

1. **User-supplied DB override** (`podcast_index_credentials` in SQLite,
   encrypted via `enc:v2:`) — allows advanced users or third-party source
   builders to supply or rotate their own key.
2. **Build-time embedded / dev env credentials** — enables out-of-the-box
   directory search and trending discovery for end users without manual setup.
3. **Unconfigured fallback** — if a third-party source build has neither,
   directory endpoints fail cleanly (`412 provider_not_configured`) while
   direct RSS and OPML remain fully functional.

API responses (`GET /discover/status`) return `{ configured, provider, keyHint, source }`
where `source` is `"user"`, `"embedded"`, or `"none"`, and `keyHint` is
`"Built-in"` for embedded keys (never exposing any characters of the embedded
key or secret).

### 2. Machine-bound encrypted credential storage (`enc:v2:`)

Podcast Index credentials are stored in the SQLite `settings` table under the
key `podcast_index_credentials` as JSON `{"apiKey":"...","apiSecret":"..."}`.

- `podcast_index_credentials` is explicitly registered in
  `backend/internal/crypto` (`sensitiveKeys`), ensuring automatic AES-256-GCM
  encryption (`enc:v2:`) using the installation's master key (`credentials.key`,
  protected by OS DPAPI on Windows or wrapped AES-GCM on other platforms).
- Credentials are retrieved and decrypted only in the Go backend immediately
  before signing outbound Podcast Index API requests.
- API responses and backend logs **never** emit `apiSecret` or the full
  `apiKey`. Status endpoints return only `{ configured, provider, keyHint }`
  where `keyHint` exposes at most a masked suffix (e.g., `••••1234`).
- Deleting credentials removes the `podcast_index_credentials` row from SQLite.

### 3. Unconditional fallback to direct RSS and OPML

Podcast directory discovery is strictly optional. Without directory
credentials configured:

- Direct public HTTPS RSS feed preview/subscription
  (`POST /api/v2/podcasts/feeds/preview`, `POST /api/v2/podcasts/feeds`) and
  OPML 2.0 import/export continue to function without degradation.
- Directory search and trending endpoints fail fast with HTTP `412`
  (`provider_not_configured`) and a clear, actionable message guiding the user
  to either configure Podcast Index credentials or subscribe directly by RSS
  feed URL.

### 4. Podcast Index API v1.0 transport, attribution, and rate-limit handling

- **Transport & SSRF boundary:** All directory requests target
  `https://api.podcastindex.org/api/1.0` over HTTPS using the podcast
  public-IP DNS-pinning transport (`IsPublicIP` checks at dial time, proxy
  disabled, redirects blocked, bounded response size, and `User-Agent: ViiB-MediaHub/1.0`).
- **Authentication:** Each request signs `X-Auth-Date` (Unix seconds),
  `X-Auth-Key` (API key), and `Authorization` (`hex(sha1(apiKey + apiSecret + authDate))`).
- **Response sanitization:** Every `feedUrl` returned by the directory is
  filtered through `ValidatePublicURL(raw, false)` (dropping any unsafe,
  non-HTTPS, private-network, or tokenized URLs). Artwork URLs are filtered
  through `SafeArtworkURL(raw)`, website URLs are restricted to public
  HTTP(S) hosts, and titles/descriptions are reduced to bounded plain text.
- **Rate limits & errors:** HTTP `429` is mapped to `ErrDirectoryRateLimited`
  (HTTP `429`, retryable without automatic retry storms); `401`/`403` maps to
  `ErrDirectoryUnauthorized` (HTTP `401`, non-retryable); network/5xx errors
  map to `ErrDirectoryUnavailable` (HTTP `502`, retryable). Raw upstream
  headers and request details are never returned to clients.
- **Attribution:** Discovery UI surfaces powered by Podcast Index must include
  clear attribution ("Powered by Podcast Index") in accordance with Podcast
  Index API terms.

### 5. Apple Podcasts Search API deferral

Integrating the Apple Podcasts / iTunes Search API as a keyless or secondary
directory adapter is **deferred**. Apple's search endpoint carries distinct
terms of use, commercial/app-store attribution obligations, lacks Podcasting
2.0 metadata, and exhibits inconsistent rate limiting across client IPs. Any
future Apple or additional directory adapter requires a separate ADR reviewing
its licensing, attribution, privacy implications, and adapter boundary.

## Consequences

- **Positive:** Zero shared secrets in binaries; full user control over
  third-party API usage; existing RSS/OPML workflows require zero accounts or
  keys; all directory-returned URLs pass through the same SSRF boundary as
  manually entered feeds.
- **Trade-off:** Users must obtain and enter their own Podcast Index API key
  and secret to use in-app keyword search and trending discovery, or paste RSS
  feed URLs / OPML directly.

## Source anchors

- [ADR 0001](0001-podcasts-local-first-boundaries.md)
- [Podcast roadmap](../PODCAST_CLIENT_IMPLEMENTATION_ROADMAP.md)
- `backend/internal/crypto/podcast_sensitive.go`
- `backend/internal/podcast/directory.go`
- `backend/internal/db/podcast_directory.go`
- `backend/internal/api/podcast_directory.go`
