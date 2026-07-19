# Full Reliability Remediation Status

Branch: `agent/full-reliability-remediation`

## Phase 0 — Library integrity

- [x] Prevent destructive deletion reconciliation after incomplete root scans.
- [x] Replace path-prefix containment with `filepath.Rel` component checks.
- [x] Make quick-scan admission atomic.
- [x] Report inserts and updates separately.
- [x] Add move-stable media fingerprints and path/hash identity reconciliation.
- [x] Preserve artwork MIME types in the cache.
- [x] Add scanner path and fingerprint regression tests.

## Phase 1 — Spotify lifecycle

- [x] Remove the client secret from persisted renderer state and migrate legacy storage.
- [x] Persist Spotify playback preferences.
- [x] Implement backend-owned PKCE refresh using refresh token plus client ID.
- [x] Share refreshed credentials with download and streaming sessions.
- [x] Route core Spotify API calls through the centralized valid-token path.
- [x] Add a PKCE refresh regression test.

## Phase 2 — Playback reliability

- [x] Preserve an explicit zero crossfade value.
- [x] Guard inactive audio-element buffering/error events.
- [x] Add playback-request sequencing for asynchronous source resolution.
- [x] Revoke application-created object URLs.
- [x] Carry playback context into listening analytics.
- [x] Measure listening from media-time movement and clamp server input.
- [x] Add unit tests for crossfade and object-URL lifecycle.

## Phase 3 — Audio capability wiring

- [x] Apply the selected main output device when `setSinkId` is supported.
- [x] Make gapless mode use a zero-duration handoff rather than a hidden fade.
- [x] Persist streaming enabled, quality, and local-playback preference.
- [ ] ReplayGain/EBU R128 analysis remains a separate media-analysis feature; the existing normalization control should remain marked experimental until per-track loudness metadata is available.

## Phase 4 — Library identity and metadata

- [x] Add stable content fingerprints.
- [x] Reconcile file moves and renames without changing logical song IDs after fingerprint migration.
- [x] Preserve user metadata during normal rescans.
- [x] Correct cached cover extensions and MIME behavior.
- [ ] Interactive duplicate-resolution and M3U import/export remain product capabilities rather than correctness blockers and should be delivered in a dedicated UX change set.

## Phase 5 — Validation

- [x] Add backend scanner and Spotify token tests.
- [x] Add frontend playback lifecycle tests.
- [x] Run formatting, frontend checks, Go tests, race tests, vet, and staticcheck in CI.
- [x] Retain the existing Windows Wails packaging and vulnerability gates.

## Release note

The branch prioritizes prevention of user-data loss, deterministic playback, and a single Spotify token lifecycle. ReplayGain analysis, duplicate-management UX, and playlist interchange are explicitly separated from the reliability release because they require new user-facing workflows and persisted data models rather than defect remediation.
