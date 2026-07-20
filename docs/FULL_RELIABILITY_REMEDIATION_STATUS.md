# Full Reliability Remediation Status

Branch: `agent/full-reliability-remediation`

## Phase 0 — Library integrity

- [x] Prevent destructive deletion reconciliation after incomplete root scans.
- [x] Replace path-prefix containment with `filepath.Rel` component checks.
- [x] Make quick-scan admission atomic.
- [x] Report inserts and updates separately and mark persistence failures as scan errors.
- [x] Add move-stable media fingerprints and path/hash identity reconciliation.
- [x] Keep identical files at two live paths as distinct library entries.
- [x] Preserve artwork MIME types and file extensions in the cache.
- [x] Add scanner path, fingerprint, and duplicate-visibility regression tests.

## Phase 1 — Spotify lifecycle

- [x] Remove the client secret from persisted renderer state and migrate legacy storage.
- [x] Persist Spotify playback preferences.
- [x] Implement backend-owned PKCE refresh using refresh token plus client ID.
- [x] Share refreshed credentials with API, download, and streaming sessions.
- [x] Retry Spotify API requests once after a `401` with a forced token refresh.
- [x] Synchronize renderer token refreshes back to the backend credential envelope.
- [x] Add PKCE refresh and unauthorized-retry regression tests.

## Phase 2 — Playback reliability

- [x] Preserve an explicit zero crossfade value.
- [x] Guard inactive audio-element buffering and error events.
- [x] Add playback-request sequencing for asynchronous source resolution.
- [x] Revoke application-created object URLs after the outgoing transition completes.
- [x] Carry playback context into listening analytics.
- [x] Measure listening from media-time movement and clamp server input.
- [x] Preload gapless handoffs into the reusable inactive audio player.
- [x] Add unit tests for crossfade, ReplayGain, and object-URL lifecycle.

## Phase 3 — Audio capability wiring

- [x] Apply and reset the selected main output device when `setSinkId` is supported.
- [x] Make gapless mode use a zero-duration handoff rather than a hidden fade.
- [x] Persist streaming enabled, quality, and local-playback preference.
- [x] Parse ReplayGain track gain/peak and Opus R128 track gain during library scans.
- [x] Persist per-track loudness metadata and apply clipping-safe normalization in the Web Audio graph.

## Phase 4 — Library identity and metadata

- [x] Add stable content fingerprints.
- [x] Reconcile file moves and renames without changing logical song IDs.
- [x] Preserve user metadata during normal rescans.
- [x] Correct cached cover extensions and MIME behavior.
- [x] Add an interactive Duplicate Manager that hides/restores redundant copies without deleting files.
- [x] Add M3U/M3U8 playlist import and export with matching/unmatched reporting.
- [x] Extend the application error boundary across the full router and global UI.

## Phase 5 — Validation

- [x] Add backend scanner, duplicate, playlist, and Spotify token tests.
- [x] Add frontend playback lifecycle and ReplayGain tests.
- [x] Run frontend checks, TypeScript, Vitest, production build, and high-severity npm audit.
- [x] Run Go tests, race detection, vet, staticcheck, application builds, and binary vulnerability scans.
- [x] Retain Windows Wails packaging and packaged-executable vulnerability gates.

## Remaining architecture-scale follow-ons

These items are intentionally not represented as completed because they require platform-specific infrastructure rather than the reliability and capability phases above:

- Move persisted credentials from encrypted SQLite to native OS credential stores.
- Remove access and refresh tokens from renderer memory entirely by migrating every Spotify operation behind typed backend endpoints.
- Perform a manual physical Windows release-candidate smoke test, including real library scans, output-device routing, Spotify reconnection, and long-running playback.
