# Performance, API, and Capability Remediation Status

Updated: 2026-08-05

## Pull request sequence

| Order | Pull request | Squash commit on `main` | Scope |
|---:|---|---|---|
| 1 | #9 | `0f69ccd` | Stable incremental identity, fingerprint persistence, event coalescing, ingest regression tests |
| 2 | #10 | `1b65b8a` | Revisioned snapshots/deltas, replay-aware SSE, incremental renderer synchronization, indexed server search |
| 3 | #11 | `cb8a1bf` | SQLite policy, bounded metadata processing, quarantine, scanner task coalescing, performance diagnostics |
| 4 | #12 | `f10b9b5` | Request IDs, structured v2 errors, OpenAPI, durable jobs, cancellable frontend transport |
| 5 | #13 | `c6bf184` | Diagnostics/repair, validated backup, staged offline restore, metadata editing, continuous monitoring, operations UI |

All five pull requests were squash-merged in the order above. The final remediation tree is `c6bf184` on `main`.

## Delivered outcomes

### Correctness

- Full and incremental ingest use the same stable fingerprint and identity-resolution contract.
- Incremental paths no longer persist a path-specific proposed ID as `file_hash`.
- Move/rename reconciliation, ReplayGain values, album-artist identity, and change accounting are preserved.
- Duplicate filesystem notifications are coalesced before persistence.

### Performance

- Routine scanner events no longer require repeated complete-library downloads.
- The renderer applies durable song upserts/deletes by ID.
- Library snapshots and changes are cursor/revision based.
- Backend-mode search is indexed and server-side.
- SQLite connection, busy-timeout, WAL, checkpoint, and synchronous policies are explicit.
- Metadata processing and background scanner work are bounded and coalesced.
- Repeated native-parser failures are quarantined with delayed retries.

### API maturity

- Additive `/api/v2` synchronization, search, performance, job, and operations surfaces are available.
- Request IDs and stable structured errors support correlation and retry decisions.
- Long-running scans have persisted job state, progress, cancellation requests, retry, and restart interruption handling.
- `docs/openapi-v2.yaml` is the machine-readable contract for the scalable API surface.

### Library operations

- Database integrity, missing media, broken playlist references, search-index state, revision-log state, and quarantined media are diagnosable.
- Repair operations can rebuild indexes, remove broken playlist references, and optionally remove confirmed missing-file records.
- Database metadata can be safely edited without mutating source media.
- Backups use a consistent SQLite snapshot, SHA-256 manifest, archive validation, and integrity validation.
- Restore is staged and applied offline with a timestamped rollback database.
- Continuous monitoring uses quick detection, deletion detection, event coalescing, and the bounded scanner.

## Guarded platform work

- Source-file tag write-back is deliberately rejected unless a build provides an audited writer. Database metadata editing is complete and safe; the application does not claim to have modified source files when it has not.
- Restore activation is deliberately offline. The running application stages and validates recovery data; the explicit `viib-restore` helper applies it after ViiB exits.
- Native OS credential-store migration and complete removal of Spotify tokens from renderer memory remain separate platform-security work. Existing encrypted SQLite secret handling remains in place; these PRs do not falsely claim completion of native credential integration.
- A physical Windows release-candidate test remains required after all phases merge, covering a real library, long playback, output switching, continuous monitoring, backup, staged restore, and Spotify reconnection.

## Validation record

The completed remediation was validated locally with:

- TypeScript, Vitest, design-token checks, and production frontend build.
- `npm audit --audit-level=high` against the refreshed dependency baseline.
- Go tests and race detection against the CGO SQLite driver.
- The final tree matches the previously built and locally vulnerability-scanned Phase 4 tree.
- Phase-specific database, scanner, synchronization, job, backup, repair, and restore tests.

Hosted CI remains useful as a release signal, but the stacked merges were performed after the local gates above at the operator's direction.
