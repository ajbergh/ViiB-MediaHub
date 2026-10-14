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

ViiB owns a fixed local embedding model, `minishlab/potion-base-8M`, pinned to revision `bf8b056651a2c21b8d2565580b8569da283cab23`. A pure-Go Model2Vec runtime implements the model's BERT normalization/WordPiece tokenizer and L2-normalized pooling of pretrained token vectors (256 dimensions). No Ollama installation, Python, GPU, native inference library, embedding API key, or user embedding settings are required. This is a compact English static embedding model; music matching uses catalog metadata rather than audio, and relevance should be evaluated on representative libraries.

First startup downloads approximately 31 MB of public model artifacts from Hugging Face into `<dataDir>/models/potion-base-8M-<revision>`. Exact lengths and SHA-256 checksums are checked on download and cache load. Downloads use temporary files and install only verified files; failed preparation retries automatically and can be retried from Settings. Cached inference and vector retrieval work offline. Setup runs in the background and is canceled/waited for on API shutdown. Only public artifact GETs leave the machine during setup; no catalog content or credentials are sent for embeddings.

The selected AI provider handles prompt interpretation and bounded candidate auditing independently. Cloud AI can receive the prompt and retrieved candidate metadata (including catalog IDs); it does not receive the whole catalog for semantic intent compilation. Embedding documents exclude file paths, listening behavior, and internal song IDs. Query embeddings are cached in memory only. Users select their AI provider/model and see local library preparation status, with rebuild/retry actions; embedding controls and cloud-embedding consent prompts are removed. The legacy `GET /semantic/settings` reports the managed identity; `PUT` returns 405.

Changing OpenAI to Anthropic, Gemini, OpenRouter, Ollama, or another supported chat provider does not restart the semantic service or re-embed the library. Provider changes clear the old chat credential unless a new credential is supplied. Existing embedding settings are ignored, including undecryptable legacy embedding keys. On upgrade from a different vector identity, only incompatible vector state is reset and rebuilt locally; catalog documents and saved playlists remain. Model revision plus inference/preprocessing version is the durable embedding identity, so future embedding upgrades deliberately rebuild rather than mix vector spaces. Saved generated playlists remain snapshots.

The service indexes in the background, reuses content hashes, tracks incremental catalog/metadata changes, and supports rebuild/retry through local endpoints. Standard metadata matching remains available while setup/indexing is incomplete or fails. Exact search retains all active vectors in memory; benchmark large libraries before replacing it with an approximate index.

See [managed model attribution](third-party/model2vec.md) and [upstream inference](https://github.com/MinishLab/model2vec/blob/main/model2vec/model.py). Real-model validation can be run with `VIIB_LOCAL_MODEL_TEST_DIR` set to a writable cache directory and `go test ./internal/semantic -run TestLocalModelReferenceAndMusicRelevance -count=1`; ordinary tests never download weights.

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

Track analysis owns audio-derived facts — tempo, musical key, beatgrid, and energy/structure features — for every catalog track ViiB can decode. It is deliberately a shared engine rather than a DJ-only feature.

```text
song_id → source resolve → decoder registry → mono downmix ──┬─> onset accumulator → tempo
          (+ fingerprint)  (local/Plex, bounded chunks)      ├─> chroma accumulator → key
                                                             ├─> phase accumulator → beatgrid
                                                             └─> energy accumulator → structure/features
                                                                    │
                                                       one combined analysis result/row
```

Design constraints that shaped it:

- **One decode pass feeds every analyzer.** Tempo, key, beatgrid, and energy share decoding, downmixing, and chunking. Running them separately would double the I/O and, worse, let each one overwrite the other's status and algorithm version in the shared row.
- **Facts are versioned and fingerprinted.** Each row records the analysis version, the composite algorithm version, and a fingerprint of the source bytes. A change to any of them makes the row stale, which is what lets a run skip work it has already done and re-analyze only what actually changed.
- **The work list is derived, never stored.** Because per-track state lives in `track_analysis`, "what is left to do" is always recomputable from the catalog. A resumed job re-expands its recorded selection and skips valid rows, so a multi-day run needs no lease recovery.
- **Unknown is a result.** An analyzer that finds no reliable evidence records that explicitly with a stable error code. It never substitutes a default tempo or key.
- **Measured facts stay separate from inferred metadata.** Local analysis writes to `track_analysis` and never to `songs.bpm`, which may hold an AI-estimated value. Manual user overrides live in their own table with independent locks and win at read time. Optional recording-linked Spotify scalar observations keep their own provenance and can supply effective BPM/key; local analysis still supplies audio artifacts.

Decoding is bounded and streaming; whole files are never held in memory. Reachable, authenticated Plex tracks are analyzed through the PMS direct-play path; tracks from an unavailable source are skipped until the source is available again.

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

Plex is a remote music source with read-only media behavior by default. The explicit AI metadata-writeback workflow is the sole exception: an approved per-track preview can update and lock genres and original release year in PMS when the token has management permission. It never edits source audio-file tags, deletes media, or reconfigures PMS. See [Plex metadata writeback](plex-music.md#ai-metadata-writeback-explicit-opt-in).

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

Normal sign-in captures a session from Spotify’s own page in an isolated Chrome/Chromium/Edge window. The backend owns encrypted session persistence, renewal, catalog adapters, and account retirement; public status responses omit cookies and bearer tokens. Legacy OAuth is a compatibility path. [Parity and platform gates](SPOTIFY_COOKIE_AUTH_PARITY_AUDIT.md) remain open.

[Spotify BPM/key import](SPOTIFY_BPM_KEY_IMPORT.md) binds downloaded observations to final source identity and preserves manual-lock precedence and local artifact fallback. Source-bound provider metadata and three-band waveform selection are implemented; live contract/platform qualification remains limited. [Current branch scope](SPOTIFY_BRANCH_REMAINING_WORK.md) records delivered behavior and limitations. Further local scanning/DSP improvements are [deferred to a future branch](LOCAL_ANALYSIS_FUTURE_BRANCH.md).

## Stem packages

ViiB consumes pre-generated `.viibstems` packages discovered beside music files or under separate configured Stem Libraries. Package scans associate validated stems with existing songs instead of cataloging stem audio as songs. Package generation belongs to the future ViiB-StemLab workflow. The [v1 contract](VIIB_STEM_PACKAGE_V1.md) defines identity, manifest, audio, and validation requirements.

## Frontend state

The frontend should prefer source-neutral `Song` behavior.

Plex configuration/authentication state is handled separately from song browsing state, and Plex access tokens/private keys are never stored in Zustand or browser `localStorage`.

A source indicator can be displayed when useful, but source-specific copies of Songs, Albums, Artists, Search, Queue, or Now Playing should be avoided.

## Security boundary

### Loopback HTTP trust boundary

The Go backend listens on loopback (`127.0.0.1`), which means other browser tabs on the same machine can attempt cross-origin or DNS-rebinding requests if not explicitly restricted. `internal/server/server.go` enforces a centralized trust boundary before routing or emitting CORS/SSE headers:

- **Trusted `Host` authorities (`IsTrustedHost`):** `127.0.0.1`, `localhost`, `[::1]`, `wails.localhost` (each with an optional valid port `1..65535`), and `wails` (no port). Any other `Host` value—including DNS-rebinding domains resolving to `127.0.0.1`—is rejected with `403 Forbidden`.
- **Trusted `Origin` headers (`IsTrustedOrigin`):** `wails://wails`, `http(s)://wails.localhost(:port)`, local dev origins `http://(localhost|127.0.0.1|[::1]):(3000|5173)`, and same-port loopback origins `http(s)://(localhost|127.0.0.1|[::1])` matching the active server port. Any request carrying an untrusted `Origin` (including `Origin: null`) is rejected with `403 Forbidden` and receives no `Access-Control-Allow-Origin` header.
- **Cross-site fetch defense:** Requests with `Sec-Fetch-Site: cross-site` that do not carry a trusted `Origin` are rejected with `403 Forbidden` on all mutating HTTP methods (`POST`, `PUT`, `PATCH`, `DELETE`) and `/api` routes, while preserving Wails desktop WebView requests where custom-scheme or virtual-host origins send `Origin: wails://wails` or `Origin: http(s)://wails.localhost`.
- **Centralized CORS and SSE headers:** `cors.Handler` delegates to `IsTrustedOrigin`, and SSE handlers (`/api/library/events`, `/api/library/enrich-*/stream`, `/api/spotify/downloads/events`, `/api/v2/library/events`, `/api/v2/jobs/events`) rely on the router middleware rather than setting `Access-Control-Allow-Origin: *`.

### Plex and provider credential boundary

Sensitive credentials (`spotify_webplayer_session`, `spotify_credentials`, `plex_credentials`, `semantic_embedding_api_key`, `gemini_api_key`, `llm_api_key`, `lastfm_api_key`, `lastfm_shared_secret`, `lastfm_session_key`) are persisted through the versioned `enc:v2:` encrypted sensitive-settings mechanism in `internal/crypto`:

- **OS-protected / relocation-safe master key (`enc:v2:`):** On first use in a data directory, ViiB generates a random 256-bit master key stored in `credentials.key` (mode `0600`). On Windows, the master key is protected at rest using Windows DPAPI (`CryptProtectData` bound to the current user profile). On other platforms (or if OS protection is unavailable), the master key is wrapped with AES-256-GCM using a relocation-safe machine/user key that excludes `os.Executable()`, so moving or upgrading the application binary never strands stored credentials.
- **Automatic `enc:v1:` migration and reauthentication reporting:** Readable plaintext or legacy `enc:v1:` settings are automatically upgraded to `enc:v2:` on database startup and on `GetSetting` reads. If a legacy `enc:v1:` entry cannot be decrypted (for example, the executable was moved prior to upgrading to `enc:v2:`), `GetSetting` returns `crypto.ErrReauthRequired` and API status endpoints surface a clear reauthentication message without failing startup or disclosing ciphertext.

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
