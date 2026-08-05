# Performance, API, and Capability Remediation Plan

Program start: 2026-08-05

## Delivery model

The remediation was delivered as stacked pull requests and squash-merged in order on 2026-08-05. This document records the completed delivery plan; see [the status](PERFORMANCE_API_REMEDIATION_STATUS.md) for the resulting `main` commits and remaining release work.

1. `agent/phase-0-ingest-identity-correctness`
   - Unify identity resolution and stable fingerprint persistence across full, quick, background, and download-triggered ingest.
   - Coalesce duplicate filesystem events and add regression tests.

2. `agent/phase-1-library-revisions-search`
   - Add monotonic library revisions, cursor-based snapshots, delta synchronization, revision-aware SSE, and server-side search.
   - Replace repeated full-catalog frontend refreshes with incremental upserts and deletes.

3. `agent/phase-2-scanner-db-performance`
   - Define SQLite connection and lock policy.
   - Coalesce background work, bound metadata processing, quarantine repeated failures, and debounce aggregate refreshes.

4. `agent/phase-3-api-jobs-contract`
   - Add `/api/v2`, structured errors, request identifiers, transport timeouts, an OpenAPI contract, and durable cancellable jobs.

5. `agent/phase-4-library-operations`
   - Add backup and restore, diagnostics and repair, metadata editing, continuous watch controls, and performance diagnostics.

## Quality gates

Each phase must pass frontend checks and build, Go tests and race detection, vet, static analysis, application builds, dependency and binary vulnerability policy, database migration tests, and phase-specific regression tests.

## Historical merge strategy

Squash merge in numerical order. After each merge, retarget the next stacked pull request to `main`. New APIs remain additive while the legacy full-library snapshot endpoint remains available for compatibility.
