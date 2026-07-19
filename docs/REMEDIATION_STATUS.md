# Full Remediation Status

Branch: `agent/full-remediation`

## Phase 0 — Security and release containment
- [x] Third-party playlist HTML rendered as text by the migration
- [x] OAuth state correlation added by the migration
- [x] Spotify PKCE no longer depends on a persisted renderer client secret
- [x] Generic settings API no longer returns provider secrets
- [x] Encryption coverage expanded to LLM and Last.fm API keys
- [x] Hardened remote artwork fetcher added for SSRF, redirect, size, MIME and image validation
- [x] Baseline browser security headers added by the migration

## Phase 1 — Correctness and data integrity
- [x] Download progress converted to per-client broadcast subscriptions by the migration
- [x] Force-restart double decrement removed by the migration
- [x] Spotify search response forwarding repaired
- [x] Spotify proxy paths constrained and HTTP clients time-bounded
- [x] Album identity and routes include album artist
- [x] Complete song enrichment/preferences DTO mapping restored
- [x] Listening-duration validation added
- [x] API/scanner/download worker lifecycle shutdown added
- [x] Version source centralized

## Phase 2 — CI and quality gates
- [x] Pull-request CI added
- [x] Frontend unit test runner added by the migration
- [x] Go tests, race detector, vet, staticcheck and govulncheck added
- [x] Windows Wails build gate added
- [x] Existing release workflow aligned to Go 1.25.2 and tests by the migration

## Phase 3 — Architecture and product hardening
- [x] Remote-media security moved into a dedicated package
- [x] Album identity moved into a shared frontend module
- [x] Lifecycle ownership moved into explicit API/scanner modules
- [x] Secret classification centralized in validation/crypto layers
- [ ] Follow-on: migrate tokens from encrypted SQLite to native OS credential stores
- [ ] Follow-on: move all Spotify Web API traffic behind the backend proxy so access tokens never enter renderer memory
- [ ] Follow-on: continue splitting the legacy API and database monoliths by bounded domain

The branch retains `scripts/apply_full_remediation.py` as the auditable source migration for the large existing modules. The remaining follow-on items require platform-specific credential-store adapters and a broader Spotify client migration; they are explicitly documented rather than falsely marked complete.
