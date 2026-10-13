# Spotify metadata branch — current scope and merge handoff

**Scope approved:** 2026-10-12. **Branch:** `feature/spotify-metadata-foundation`. **Target:** a new PR into `main`.

The owner explicitly deferred further local scanning and audio-analysis improvements to a future branch. This supersedes the original requirement to close all twelve broad gates before merging. The old **2/12 checked, 10/12 partial** tally is preserved as historical evidence; it is not a completion claim or the current PR checklist.

## Included in this PR

- Spotify metadata capture, sanitized private catalog persistence, durable source-bound imported observations, and account/source publication fences.
- Source-aware field precedence, original units, last-good evidence, manual locks, local fallback, and explicit unknown states. Library lists remain score-gated for imported scalar facts and do not hash entire audio files on list reads.
- Provider/local waveform selection, bounded stored-artifact reads, retention/traversal/staging lifecycle fixes, and existing scan/preparation correctness fixes.
- REST artist-album parent relations and filter/page isolation; DJ queue analysis uses the same admitted observations as sequencing and remains transient in the frontend.
- Reviewed manual-grid requirements for sync/snapping, and final bug fixes/regressions found during merge review.
- Existing reproducible benchmark tools and explicit experimental methods. No experimental estimator, threshold, range, model or runtime is promoted into production defaults.

## Deferred, not qualified

Further local scan/backfill optimization, BPM/key estimator changes, calibration, corpus expansion, independent held-out quality qualification, learned downbeats/meter, semantic structure/cues, Beat This integration, and broad performance/platform/accessibility qualification move to [the future-branch backlog](LOCAL_ANALYSIS_FUTURE_BRANCH.md). Existing local functionality remains available. Low-confidence and unknown results remain visible; generated meter-derived grids do not become reviewed musical downbeats.

Provider-contract and platform evidence gaps remain disclosed limitations: one connected recording or synthetic fixtures do not establish every account, market, error response, Recently Played semantics, native responsiveness, or macOS/Linux host behavior. See [resource inventory](SPOTIFY_CONNECTED_RESOURCE_INVENTORY.md) and [parity audit](SPOTIFY_COOKIE_AUTH_PARITY_AUDIT.md). This PR must describe implemented behavior without claiming complete provider parity or professional analysis accuracy.

## Merge requirements

1. Review the complete diff against current `origin/main` and fix concrete correctness regressions.
2. Reconcile active documentation and archive superseded plans/evidence with working links.
3. Run frontend checks, backend tests/race/vet/build and applicable benchmark-tool regressions; record actual results and local limitations.
4. Commit/push the reviewed work, open a new PR against `main`, and inspect required CI results. The PR is reviewable before all hosted jobs finish; merge readiness requires those checks to pass.
5. Leave merging to the branch owner. This request authorizes PR preparation, not automatic merge/deployment.

Current verification and PR state are in [the closeout checklist](SPOTIFY_BRANCH_CLOSEOUT_NEXT_STEPS.md). Superseded plans, milestone logs and dated corpus/live measurements are indexed in [the branch archive](archive/spotify-metadata-foundation/README.md).

## Known development credential limitation

The pre-existing encryption derivation includes the executable path. Restarting through a different `go run` temporary path can make saved Spotify and AI credentials unreadable. Use a stable executable path for development; existing encrypted data requires its original derivation inputs or reauthentication/reconfiguration. No secrets are included in the archive. A versioned credential migration is separate future work; this PR does not claim to fix it.
