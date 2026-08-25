# Performance, API, and Capability Remediation Status

Originally completed: 2026-08-05  
Current documentation pass: 2026-08-25

This file is retained as the historical record for PRs #9–#13. Those changes are merged into `main`. Subsequent CI/security and Plex work extended the same architecture; see the current-state addendum below and [Architecture](architecture.md).

## Original pull request sequence

| Order | Pull request | Squash commit on `main` | Scope |
|---:|---|---|---|
| 1 | #9 | `0f69ccd` | Stable incremental identity, fingerprint persistence, event coalescing, ingest regression tests |
| 2 | #10 | `1b65b8a` | Revisioned snapshots/deltas, replay-aware SSE, incremental renderer synchronization, indexed server search |
| 3 | #11 | `cb8a1bf` | SQLite policy, bounded metadata processing, quarantine, scanner task coalescing, performance diagnostics |
| 4 | #12 | `f10b9b5` | Request IDs, structured v2 errors, OpenAPI, durable jobs, cancellable frontend transport |
| 5 | #13 | `c6bf184` | Diagnostics/repair, validated backup, staged offline restore, metadata editing, continuous monitoring, operations UI |

All five pull requests were squash-merged in order.

## Delivered outcomes

### Correctness

- Full and incremental local ingest use the same stable fingerprint and identity-resolution contract.
- Move/rename reconciliation, ReplayGain values, album-artist identity, and change accounting are preserved.
- Duplicate filesystem notifications are coalesced before persistence.

### Performance

- Routine scanner events no longer require repeated complete-library downloads.
- The renderer applies durable song upserts/deletes by ID.
- Library snapshots and changes are cursor/revision based.
- Backend search is indexed and server-side.
- SQLite connection, busy-timeout, WAL, checkpoint, and synchronous policies are explicit.
- Metadata processing and background scanner work are bounded and coalesced.
- Repeated native-parser failures are quarantined with delayed retries.

### API maturity

- Additive `/api/v2` synchronization, search, performance, job, operations, and now Plex source-management surfaces are available.
- Request IDs and structured errors support correlation and retry decisions.
- Long-running scans have persisted job state, progress, cancellation requests, retry, and restart interruption handling.
- `docs/openapi-v2.yaml` is the machine-readable contract for the scalable API surface.

### Library operations

- Database integrity, missing local media, broken playlist references, search-index state, revision-log state, and quarantined media are diagnosable.
- Repair can rebuild indexes, remove broken playlist references, and optionally remove confirmed missing **local** file records.
- Plex remote catalog identities are excluded from local missing-file reconciliation.
- Database metadata can be edited without mutating source media.
- Backups use a consistent SQLite snapshot, SHA-256 manifest, archive validation, and integrity validation.
- Restore is staged and applied offline with a rollback database.
- Continuous monitoring applies to configured local folders; Plex uses explicit authoritative synchronization instead of filesystem watching.

## Current-state addendum

After the original remediation sequence:

- PR #18 restored/strengthened the CI baseline and upgraded Go to **1.25.13** after binary vulnerability analysis identified the fixed toolchain baseline.
- PR #17 added Plex Media Server music/audio libraries as a first-class ViiB source.
- Plex tracks synchronize into the canonical `songs` catalog and use existing Songs/Albums/Artists/Search/Queue/playlists/likes/history/AI/Stats flows.
- Plex GDM discovery, manual PMS configuration, authentication, music-only filtering, synchronization, source-aware audio/artwork proxying, Range/seeking, offline retention, and read-only safety are implemented.
- The final Plex pre-merge run passed frontend validation, Go tests/race/vet/Staticcheck, Linux builds and binary vulnerability scans, Windows Wails packaging, and vulnerability scanning of the packaged Windows executable.

## Guarded platform work

- Source-file tag write-back remains deliberately unavailable unless an audited source writer is introduced. Database metadata editing does not claim to modify local files or Plex metadata.
- Restore activation remains offline through the restore helper.
- Native OS credential-store migration and further Spotify renderer-token reduction remain potential security follow-ons.
- Automatic Plex audio transcoding is not claimed because the current direct-play/range contract has not been replaced with a validated transcode-session model.
- Physical release-candidate smoke testing remains appropriate for real large libraries, device/output routing, Spotify reconnection, Plex discovery/authentication/synchronization/seeking, backup/restore, and long-running playback.

For current operational behavior, prefer [Library Operations](library-operations.md), [Plex Music](plex-music.md), [Player](player.md), and [OpenAPI v2](openapi-v2.yaml).
