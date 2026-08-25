# Full Reliability Remediation Status

The reliability remediation originally tracked on `agent/full-reliability-remediation` is complete and merged. This file is retained as a historical phase record; current architecture/source behavior is documented in [architecture.md](architecture.md), [plex-music.md](plex-music.md), and [library-operations.md](library-operations.md).

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
- [x] Parse ReplayGain track gain/peak and Opus R128 track gain during local-library scans.
- [x] Persist per-track loudness metadata and apply clipping-safe normalization in the Web Audio graph.

## Phase 4 — Library identity and metadata

- [x] Add stable content fingerprints for local media.
- [x] Reconcile local file moves and renames without changing logical song IDs.
- [x] Preserve user metadata during normal local rescans.
- [x] Correct cached cover extensions and MIME behavior.
- [x] Add an interactive Duplicate Manager that hides/restores redundant local copies without deleting files.
- [x] Add M3U/M3U8 playlist import and export with matching/unmatched reporting.
- [x] Extend the application error boundary across the full router and global UI.

## Phase 5 — Validation

- [x] Add backend scanner, duplicate, playlist, and Spotify token tests.
- [x] Add frontend playback lifecycle and ReplayGain tests.
- [x] Run frontend checks, TypeScript, Vitest, production build, and production-dependency audit.
- [x] Run Go tests, race detection, vet, Staticcheck, application builds, and binary vulnerability scans.
- [x] Retain Windows Wails packaging and packaged-executable vulnerability gates.
- [x] Upgrade the Go security baseline to **1.25.13**.

## Subsequent source architecture work

After this remediation, Plex Media Server music support was added as a first-class remote music source. Plex extends the canonical ViiB catalog and player rather than replacing the reliability rules above:

- remote `plex://` catalog identity is excluded from local missing-file repair;
- failed PMS synchronization never becomes deletion reconciliation;
- Plex playback uses backend-authenticated source-aware media routes;
- Plex credentials use the existing encrypted sensitive-setting path;
- full frontend/backend/Windows CI passed before the Plex feature was merged.

## Remaining architecture-scale follow-ons

- Consider native OS credential stores for sensitive credentials where the platform complexity is justified.
- Continue reducing Spotify token exposure in renderer memory by moving remaining Spotify operations behind backend-owned typed APIs.
- Perform physical release-candidate smoke testing with a real large local library and real Plex Media Server, including output routing, Spotify reconnection, Plex discovery/authentication/sync/seeking, backup/restore, and long-running playback.
