# Remediation Status

This file records the current outcome of the major reliability/security remediation work. The original implementation branch was `agent/full-remediation`; completed work has since been merged into `main`.

## Security and release containment

- [x] Third-party playlist HTML rendered as text
- [x] OAuth state correlation added
- [x] Spotify PKCE no longer depends on a renderer client secret
- [x] Browser persistence of Spotify client secret removed
- [x] Generic settings API no longer returns provider secrets
- [x] Encryption coverage expanded to LLM, Last.fm, and Plex sensitive settings
- [x] Remote artwork downloads hardened against SSRF, redirects, oversized content, and invalid images
- [x] Baseline browser security headers added
- [x] Plex credentials kept out of browser media URLs and frontend persistent state

## Correctness and data integrity

- [x] Download progress converted to per-client broadcast subscriptions
- [x] Force-restart double decrement removed
- [x] Spotify search response forwarding repaired
- [x] Spotify proxy paths constrained and HTTP clients time-bounded
- [x] Album identity and routes include album artist
- [x] Complete song enrichment/preferences DTO mapping restored
- [x] Listening-duration validation added
- [x] API/scanner/download worker lifecycle shutdown added
- [x] Version source centralized
- [x] Plex catalog synchronization is authoritative only after a complete successful PMS read
- [x] Temporary Plex outages do not delete cached catalog rows
- [x] Plex library switching retains the previous cache until the new selection synchronizes successfully

## CI and quality gates

- [x] Pull-request CI added
- [x] Frontend unit test runner added
- [x] Go tests, race detector, vet, Staticcheck, and govulncheck enabled
- [x] Windows Wails build/package gate enabled
- [x] Go security baseline updated to **1.25.13**
- [x] Runtime production dependency audit used for the blocking npm audit gate
- [x] Linux browser/Wails binaries scanned with `govulncheck`
- [x] Packaged Windows Wails executable scanned with `govulncheck`

The Plex feature's final pre-merge validation passed the full frontend/backend/Windows CI chain, including race tests, Staticcheck, Wails packaging, and packaged-binary vulnerability scanning.

## Architecture and product hardening

- [x] Remote-media security moved into dedicated backend code
- [x] Album identity moved into shared frontend logic
- [x] Lifecycle ownership moved into explicit API/scanner modules
- [x] Secret classification centralized in validation/crypto layers
- [x] Plex implemented behind the existing ViiB catalog/playback abstractions instead of creating duplicate Plex-only UI models
- [x] Plex source identity separated into additive source/track tables while `songs` remains canonical
- [x] Plex GDM discovery, JWT/PIN authentication, music-only filtering, source-aware playback, artwork proxying, and offline-safe synchronization implemented

## Current follow-on items

- [ ] Consider migrating sensitive tokens from encrypted SQLite to native OS credential stores where the platform UX/maintenance tradeoff is justified
- [ ] Continue reducing renderer exposure to Spotify access tokens by moving more Spotify Web API traffic behind backend-owned flows
- [ ] Continue splitting large legacy API/database modules by bounded domain when doing so directly improves maintainability
- [ ] Validate real-PMS behavior during release-candidate smoke testing across LAN discovery, authentication, large-library synchronization, codec coverage, and long-running playback
- [ ] Revisit documented Plex audio transcoding only if it can preserve ViiB's seeking/session expectations without weakening the existing player contract

See [Architecture](architecture.md), [Plex Music](plex-music.md), and [Library Operations](library-operations.md) for the current production model.
