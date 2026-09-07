# Architecture and Media Source Model

ViiB MediaHub is designed around a canonical local catalog with source-specific ingestion and playback adapters. The UI should normally reason about ViiB songs, albums, artists, playlists, queue entries, likes, and history rather than branching on Plex or filesystem implementation details.

## High-level model

```text
                    ┌──────────────────────┐
Local folders ─────>│ local scanner        │────┐
                    └──────────────────────┘    │
                                                ▼
                                          ┌───────────┐
Plex music ────────> PMS sync adapter ───>│ `songs`   │
                                          │ catalog   │
                                          └─────┬─────┘
                                                │
                   ┌────────────────────────────┼─────────────────────────┐
                   ▼                            ▼                         ▼
              Songs/Albums                 Queue/Player            Search/AI/Stats
              Artists/Genres               Playlists/Likes         History/Smart Mixes

Spotify browse/stream/download ──> separate Spotify integration
```

The SQLite `songs` table remains the canonical ViiB catalog for local filesystem tracks and synchronized Plex music.

## Semantic retrieval subsystem

Semantic retrieval is a local, SQLite-backed companion to the canonical catalog. It creates deterministic text documents for tracks, albums, and artists, persists L2-normalized embedding vectors in SQLite, and rebuilds its in-memory exact-search arenas from those durable rows on startup.

```text
User prompt → intent compiler → query embedding → exact track / album / artist retrieval
                                               → local filters + behaviour ranking + diversity
                                               → playlist or phase-aware DJ sequencing
```

The intent compiler never receives the full song catalog or local taxonomy. Query embeddings are cached in memory only; user listening behaviour, file paths, and internal song IDs are never included in embedding text. Semantic settings are separate from chat-provider settings. Local Ollama indexing stays local; OpenAI indexing requires an explicit one-time cost confirmation before deterministic document text is sent to its embeddings API.

The service indexes in the background, survives provider failures as retryable document errors, and can be reindexed or reloaded through local status endpoints. A model/provider identity or vector-dimension inconsistency cannot mix vector spaces: replacement vectors are checked before persistence, and the existing metadata-based AI DJ path remains a deterministic fallback whenever a searchable semantic index is not available.

## Durable job scheduler

Long-running library work runs as a persisted job in the `operation_jobs` table rather than as a detached goroutine. The table is the source of truth for job state, so work survives a restart.

```text
create / retry / resume ──> queued ──> atomic claim ──> running ──> succeeded
                              ▲                            │            failed
                         requeue on yield                  └──> canceling ──> canceled
                              │
                            paused ◄── pause                     restart ──> interrupted
```

The dispatcher is a bounded worker pool sized `min(2, max(1, NumCPU/4))`. Claiming is a single conditional `UPDATE`, so two workers can never take the same job, and the queue is ordered by `priority` then `created_at`. Three properties matter for correctness:

- **Restart keeps pending work.** Only `running` and `canceling` jobs become `interrupted`; `queued` and `paused` jobs are retained and drained when routes are wired.
- **A job can yield without failing.** `available_at` lets a running job be requeued with a backoff, which is how analysis steps aside for DJ playback without holding a worker or losing progress.
- **Pause is dispatch-level.** Pausing stops new work from starting and lets in-flight work finish. Nothing is suspended mid-item.

## Track analysis engine

Track analysis owns audio-derived facts — currently tempo and musical key — for every catalog track ViiB can decode. It is deliberately a shared engine rather than a DJ-only feature.

```text
song_id → local source resolve → decoder registry → mono downmix ──┬─> onset accumulator → tempo
          (+ fingerprint)        (wav/mp3/ogg)     (bounded chunks) └─> chroma accumulator → key
                                                                          │
                                                          one combined track_analysis row
```

Design constraints that shaped it:

- **One decode pass feeds every analyzer.** Tempo and key share decoding, downmixing, and chunking. Running them separately would double the I/O and, worse, let each one overwrite the other's status and algorithm version in the shared row.
- **Facts are versioned and fingerprinted.** Each row records the analysis version, the composite algorithm version, and a fingerprint of the source bytes. A change to any of them makes the row stale, which is what lets a run skip work it has already done and re-analyze only what actually changed.
- **The work list is derived, never stored.** Because per-track state lives in `track_analysis`, "what is left to do" is always recomputable from the catalog. A resumed job re-expands its recorded selection and skips valid rows, so a multi-day run needs no lease recovery.
- **Unknown is a result.** An analyzer that finds no reliable evidence records that explicitly with a stable error code. It never substitutes a default tempo or key.
- **Measured facts stay separate from inferred metadata.** Analysis writes to `track_analysis` and never to `songs.bpm`, which may hold an AI-estimated value. Manual user overrides live in their own table with independent locks and win at read time.

Decoding is bounded and streaming; whole files are never held in memory. Plex-hosted tracks need an authenticated source adapter that does not exist yet and are excluded from analysis rather than queued and failed.

## Local filesystem source

Local folders are configured in Settings and ingested by the Go scanner.

The scanner is responsible for:

- discovering supported audio files;
- extracting metadata;
- maintaining filesystem identity;
- detecting additions/changes/deletions;
- updating the canonical `songs` catalog;
- participating in local filesystem diagnostics and continuous monitoring.

Local audio playback uses ViiB's normal media route and reads the configured filesystem media on the backend.

## Plex Media Server source

Plex is a remote, read-only music source.

The `backend/internal/plex` package separates:

- GDM discovery;
- PMS URL normalization and identity validation;
- Plex account/device authentication;
- music-library discovery;
- track metadata mapping;
- source identity generation;
- PMS media requests.

Additive database tables store Plex-specific identity outside the generic song shape:

- `plex_sources` — PMS machine identity, connection, selected library, availability/sync state;
- `plex_tracks` — ViiB song ID to Plex library/rating/media/artwork identity.

A Plex song ID is derived from stable PMS identity rather than reusing raw Plex numeric IDs. This avoids collisions with local ViiB IDs and does not assume a single PMS forever.

## Plex synchronization semantics

A PMS synchronization becomes authoritative only after the complete remote library read succeeds.

A successful synchronization can:

- add new Plex tracks;
- update changed metadata;
- remove cached ViiB rows that PMS successfully confirms are no longer in the selected library.

A failed synchronization must not be interpreted as an empty Plex library. DNS failures, timeouts, TLS errors, authentication failures, and server outages retain the cached catalog.

Changing the selected Plex music library is also outage-safe: the previous cache remains until a successful synchronization of the new selection can reconcile it.

## Playback abstraction

The frontend uses the existing source-transparent routes:

```text
GET /api/audio/{songId}
GET /api/cover/{songId}
```

For local songs, the backend serves local media/artwork.

For Plex songs, the backend resolves Plex source metadata, attaches `X-Plex-Token` server-side, and proxies the PMS media/artwork response. The browser never needs a credential-bearing PMS URL.

Plex audio proxy behavior includes:

- direct play by default;
- HTTP Range forwarding;
- seeking through `206 Partial Content`;
- preservation of `Content-Range`, `Content-Length`, `Accept-Ranges`, and content type;
- preservation of valid `416 Range Not Satisfiable` semantics;
- streaming/cancellation without buffering entire tracks into memory;
- source availability/authentication state on upstream failures.

Automatic Plex audio transcoding is not enabled because PMS transcode sessions are not a verified drop-in replacement for the current byte-range player contract.

## Spotify integration

Spotify is intentionally different from Plex.

Spotify browsing and direct streaming remain a separate integration rather than being synchronized as remote Spotify rows into the canonical ViiB catalog. Spotify downloads can become normal local media after they are saved and scanned.

This distinction keeps the canonical catalog focused on media ViiB can identify persistently as either local filesystem content or a configured remote PMS music source.

## Frontend state

The frontend should prefer source-neutral `Song` behavior.

Plex configuration/authentication state is handled separately from song browsing state, and Plex access tokens/private keys are never stored in Zustand or browser `localStorage`.

A source indicator can be displayed when useful, but source-specific copies of Songs, Albums, Artists, Search, Queue, or Now Playing should be avoided.

## Security boundary

Sensitive Plex credentials are persisted through the existing encrypted sensitive-settings mechanism.

Security invariants:

- Plex tokens/private device-key material stay out of public settings responses;
- browser-visible media URLs contain ViiB song IDs, not Plex tokens;
- PMS authentication is attached by the Go backend;
- credentials are redacted from errors/logs;
- cross-origin asset requests do not inherit the PMS token;
- removing Plex from ViiB never sends destructive media operations to PMS.

## Library Operations boundary

Filesystem diagnostics and repair must understand that `plex://` catalog identities are remote records, not missing local paths.

Consequently:

- local missing-file repair applies only to local filesystem songs;
- PMS outages are represented by Plex source availability/authentication state;
- Plex cache removal happens only after successful authoritative sync or explicit source removal;
- backup/restore remains a ViiB database operation and never modifies PMS media.

## Extensibility

The current Settings UX manages one active Plex source, but source IDs, machine identifiers, library IDs, and per-track source metadata are deliberately separated so future multiple-PMS/multiple-library support does not require replacing the canonical song model.
