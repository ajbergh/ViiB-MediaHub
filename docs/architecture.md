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
