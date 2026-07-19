# Full Remediation Status

Branch: `agent/full-remediation`

## Phase 0 — Security and release containment
- [x] Third-party playlist HTML rendered as text
- [x] OAuth state correlation added
- [x] Spotify PKCE no longer depends on a renderer client secret
- [x] Browser persistence of Spotify client secret removed
- [x] Generic settings API no longer returns provider secrets
- [x] Encryption coverage expanded to LLM and Last.fm API keys
- [x] Remote artwork downloads hardened against SSRF, redirects, oversized content and invalid images
- [x] Baseline browser security headers added

## Phase 1 — Correctness and data integrity
- [x] Download progress converted to per-client broadcast subscriptions
- [x] Force-restart double decrement removed
- [x] Spotify search response forwarding repaired
- [x] Spotify proxy paths constrained and HTTP clients time-bounded
- [x] Album identity and routes include album artist
- [x] Complete song enrichment/preferences DTO mapping restored
- [x] Listening-duration validation added
- [x] API/scanner/download worker lifecycle shutdown added
- [x] Version source centralized

## Phase 2 — CI and quality gates
- [x] Pull-request CI added
- [x] Frontend unit test runner added
- [x] Go tests, race detector, vet, staticcheck and govulncheck added
- [x] Windows Wails build gate added
- [x] Existing release workflow aligned to Go 1.25.2 and tests

## Phase 3 — Architecture and product hardening
- [x] Remote-media security moved into a dedicated package
- [x] Album identity moved into a shared frontend module
- [x] Lifecycle ownership moved into explicit API/scanner modules
- [x] Secret classification centralized in validation/crypto layers
- [ ] Follow-on: migrate tokens from encrypted SQLite to native OS credential stores
- [ ] Follow-on: move all Spotify Web API traffic behind the backend proxy so access tokens never enter renderer memory
- [ ] Follow-on: continue splitting the legacy API and database monoliths by bounded domain

The remaining follow-on items require platform-specific credential-store adapters and a broader Spotify client migration; they are documented explicitly rather than represented as completed.
