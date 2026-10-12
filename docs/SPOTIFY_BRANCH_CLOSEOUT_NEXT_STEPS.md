# Spotify metadata branch — PR closeout checklist

**Reviewed:** 2026-10-12. **Branch:** `feature/spotify-metadata-foundation`. **Base:** `main`.

The owner approved deferring further local scanning/DSP improvements to a future branch. [Current scope](SPOTIFY_BRANCH_REMAINING_WORK.md) replaces the historical twelve-gate merge requirement; [deferred work](LOCAL_ANALYSIS_FUTURE_BRANCH.md) remains unqualified. The [archived milestone log](archive/spotify-metadata-foundation/SPOTIFY_BRANCH_CLOSEOUT_NEXT_STEPS.md) preserves implementation and measurement history through 275.

## Final review

- Existing provider capture, source/account fencing, DJ scalar publication, retained evidence and reviewed-grid safeguards remain included.
- Final review found an overlapping Smart Mix request race. Per-request generation now guards success/error publication and loading settlement, and unmount invalidates pending publication. Regression coverage exercises both completion orders within one Spotify session.
- Beat-grid reset now explicitly clears the source fingerprint alongside the grid/lock. Existing late-source-change fencing remains tested.
- Repeated validation reproduced missing catalog snapshots under concurrent download-resource writes. Catalog publication now retries only SQLite busy/locked errors with bounded backoff, releases the runtime read lock while waiting, and rechecks account/fence admission on each attempt. Cancellation, superseded runtime errors and other failures are not retried. The original HTTP response stays intact; no provider request is repeated by this fix.
- The executable-path credential dependency is unchanged from `main`; the current handoff documents the development limitation. No credential migration or live settings change is made.
- Benchmark experiments remain opt-in and unpromoted. No reserved corpus runs or further local-scanning improvements are undertaken.
- Staticcheck findings were cleaned up: four unused private helpers, two overwritten test assignments, a redundant boolean comparison and error-message capitalization. No estimator behavior changes.
- Release checks identified Go standard-library/x/net and frontend metadata-parser advisories. The baseline is now Go 1.26.9 across module/CI/build/release instructions, x/net v0.60.0 (with required x/crypto/sys/text updates), and music-metadata 11.16.1. These are targeted dependency fixes, not a scanner feature rollout. [Go releases](https://go.dev/dl/), [x/net advisory](https://pkg.go.dev/vuln/GO-2026-6617), [parser release](https://github.com/Borewit/music-metadata/releases/tag/v11.16.1).

## Verification and publication

Documentation validation passes: **67 Markdown files, 471 local targets, 37 documentation JSON files**, zero failures. The new check runs in CI and rejects ignored targets absent from a clean checkout. An old screenshot reference is now explicitly labelled local-only rather than presented as a repository link. Public-evaluation tools pass all 12 acquisition and five rhythm-adapter regressions. Formatting and whitespace checks pass.

After the dependency patch, full frontend checks pass **114 files / 491 tests**, types/boundaries/colors and production build; the shipped dependency audit reports zero vulnerabilities. Full backend tests pass on Go 1.26.9 after the catalog retry fix. Vet, both application builds and Staticcheck pass. Both built binaries report **zero affected vulnerabilities**; one non-called module advisory remains in the scanner's inventory. The catalog contention/cancellation/retirement checks and existing capture regression pass **200 repetitions**.

The earlier Windows full race run reached its ten-minute per-package limit in API/DB; its log reports no data race. It is a timeout, not a pass. CI and release validation now allow 25 minutes for the full race suite, retaining all tests. The required hosted race/native/platform checks must pass on the latest PR head before merge.

Focused catalog retry/download-bundle/artist-page race regressions pass three repetitions (17.260s); affected vet and Staticcheck pass. This narrow result does not replace the full hosted race suite.

[PR #117](https://github.com/ajbergh/ViiB-MediaHub/pull/117) targets `main` and was opened as a draft while validation ran. Inspect its current checks/review state for the merge decision. Native/browser and independent corpus qualification gaps remain disclosed rather than treated as completed by tests. No automatic merge or deployment is authorized.
