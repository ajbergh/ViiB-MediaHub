# Local preparation performance baseline

> Scope update — 2026-10-12: Existing preparation and offline one-/105-track measurements remain documented. Further local scan/backfill optimization and broad runtime/performance qualification are deferred to a future branch; historical measurements are not cross-platform qualification. See [current branch scope](SPOTIFY_BRANCH_REMAINING_WORK.md) and [future local-analysis work](LOCAL_ANALYSIS_FUTURE_BRANCH.md).

This opt-in benchmark exercises the production preparation runner, SQLite persistence, bounded artifact decoding and the real analysis-list HTTP handler. It creates and cleans up its own temporary database, reads existing audio in place, and loads no provider credentials, private caches or live database. It does not start the API constructor's scanner, download manager or scheduler.

Prepare a private JSON inventory outside tracked files, for example under `.tmp-review`. Each entry contains an opaque `id` and absolute `path` to an available local audio file. Keep the same ordered inventory for the one-track and 105-track runs. Do not put paths or account data in committed reports. Audio is referenced, never copied. Select representative healthy sources and record codec/duration coverage separately; this is an unlabeled performance inventory, not an accuracy corpus.

From `backend`, run the following after other library analysis has settled:

```powershell
go test ./internal/api -run '^$' -bench '^BenchmarkLocalPreparationQualification$' -benchtime=1x -count=1 -timeout 60m -args -preparation-inventory 'C:\private\inventory.json' -preparation-count 1 -preparation-report 'C:\private\one-track.json'
go test ./internal/api -run '^$' -bench '^BenchmarkLocalPreparationQualification$' -benchtime=1x -count=1 -timeout 60m -args -preparation-inventory 'C:\private\inventory.json' -preparation-count 105 -preparation-report 'C:\private\105-tracks.json'
```

Reports must be new files. `-benchtime=1x` prevents automatic repeated full-library preparation. The benchmark is skipped without an inventory. Regular tests exercise a synthetic one-track fixture to check instrumentation, warm settlement, payload/artifact size collection, Windows RSS availability and unchanged source bytes.

Each invocation measures two phases:

- `fresh_offline`: an empty scratch analysis database, missing-capability discovery and local preparation. Exactly one decoder open per track is required.
- `warm_offline`: missing discovery in that populated database, then a direct runner pass over every original ID. Zero missing candidates, zero decoder opens and all tracks skipped are required. Running every ID verifies the runner's authoritative skip behavior even when discovery returns nothing.

The report includes selection/preparation wall time, work counts, decoder opens, per-family stored artifact bytes and expanded decoded JSON bytes, actual analysis-list response size/read time/status, database/WAL sizes, environment and algorithm version. Inventory paths, titles, fingerprints, account IDs and credentials are omitted. Source errors and repeated warm work fail qualification rather than quietly improving the counts. `tempoAbstentions` and `keyAbstentions` count absent local dimensions separately; the older `timingAbstentions` metric counts tracks where both are absent.

On Windows, peak memory comes from the operating system's process lifetime peak working-set counter through [GetProcessMemoryInfo](https://learn.microsoft.com/en-us/windows/win32/api/psapi/nf-psapi-getprocessmemoryinfo). This includes earlier phases and measurement overhead; it is not reset for the warm phase. Other hosts currently omit this metric instead of reporting zero. Expanded artifact JSON size describes a portable decoded representation; it is not a Go heap allocation estimate. The HTTP handler runs in-process, so response bytes are real but network/browser latency is not measured. OS file caches are not flushed. Record competing host work separately before comparing results.

These measurements establish the local baseline required before setting performance thresholds. They do not qualify fresh/warm **connected provider caches**, provider HTTP request batching, independent accuracy, native/browser rendering, accessibility or macOS/Linux runtime. Provider I/O is disabled by construction; zero provider calls here cannot establish provider-connected efficiency. Keep those original acceptance requirements open until measured separately.


The first idle-library Windows reports are [one track](archive/spotify-metadata-foundation/evidence/spotify-preparation-one-track-offline-20261012T064059Z.json), [105 tracks](archive/spotify-metadata-foundation/evidence/spotify-preparation-105-track-offline-20261012T064059Z.json) and [host/job context](archive/spotify-metadata-foundation/evidence/spotify-preparation-context-20261012T064059Z.json). They predate the atomic claim-snapshot change in Milestone 256. Fresh preparation took 9.389s and 769.532s, respectively, with one decode per track; warm preparation took 0.015s and 1.575s with zero decodes. The 105-track process peaked at 73.49 MiB. Retain these as historical measurements.

The final Milestone 259 revision-fence reports are [one track](archive/spotify-metadata-foundation/evidence/spotify-preparation-one-track-offline-20261012T072030Z.json), [105 tracks](archive/spotify-metadata-foundation/evidence/spotify-preparation-105-track-offline-20261012T072030Z.json) and [host/source context](archive/spotify-metadata-foundation/evidence/spotify-preparation-context-20261012T072030Z.json). One track took 9.393s fresh/0.016s warm. The 105-track run took 759.456s fresh with 105 decodes and 1.517s warm with zero decodes, zero missing candidates and all 105 skipped. Peak process working set was 71.39 MiB; the list response was 508,896 bytes, read in 0.104s fresh/0.103s warm. Artifacts totaled 99,819,432 stored bytes and 169,027,409 expanded JSON bytes; DB plus WAL totaled 104,807,032 bytes. Three tempo and nine key abstentions were stable in both phases. All preparation and HTTP checks passed. Source hashes were observed after the run began and subsequently verified unchanged; the context explicitly records that limitation. These remain offline Windows measurements, with the broader acceptance requirements above still open.
