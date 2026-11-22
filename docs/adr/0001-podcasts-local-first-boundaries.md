# ADR 0001 — Local-first podcast domain and initial security boundaries

- **Date:** 2026-11-22
- **Status:** Accepted for foundational implementation; provider policy, scheduler and UI details remain open
- **Scope:** `POD-00A`, partial `POD-00B/C/E/G`
- **Roadmap:** [Podcast roadmap](../PODCAST_CLIENT_IMPLEMENTATION_ROADMAP.md)

## Decision

Podcasts are a separate content domain in ViiB. The persisted source of truth
will be publisher RSS plus local SQLite records for shows, episode metadata,
subscription state and independent playback/download status. Podcasts will not
be inserted into the `songs` catalog, music playlists, Spotify download
records, DJ analysis or Last.fm history. Podcast media bytes will eventually
reside beneath a dedicated, managed `podcast_downloads` directory.

The existing normal music player and the new podcast player will coordinate
through a **single-owner audio session**: changing sources must stop the prior
source and preserve its queue and playback progress. The current
`hooks/useMediaSession.ts` music handlers cannot coexist with independent
podcast media-session registrations; an explicit coordinator will own them.
No audio/player refactor is part of this ADR commit.

Podcast REST routes belong in `backend/internal/api/podcast_routes.go`,
mounted at `/api/v2/podcasts` in `backend/internal/server/server.go` before
the generic `/api/v2` router. They must preserve the existing inbound
Host/Origin/CSRF boundary. No podcast routes are exposed until handlers and
server integration tests land.

Feed/network fetch and XML parsing are deliberately separated from SQLite
storage, with standard-library-only, independently testable Go primitives.
All URLs are untrusted even if supplied by a legitimate directory or feed.
The fetcher must reject non-public networks, address ambiguity and redirect
abuse. Public feed fetching is **HTTPS-only by default**, with a gated legacy
HTTP option (not yet user-exposed). Private and tokenized feeds are explicitly
out of scope until backend encryption, redaction and export rules are tested.

## Non-decisions and gates

1. **Directory search:** Podcast Index remains the preferred provider, but
   production credentials, service terms and attribution require review.
   Direct RSS will function without any provider key. Do not distribute a
   shared Podcast Index secret in desktop releases.
2. **Refresh scheduling:** ViiB's `operation_jobs` workers are capped at
   1–2; do not use them for feed refresh until contention and restart behavior
   are proven. Choose dedicated refresh dispatch versus adapted durable jobs
   in a follow-up ADR.
3. **Audio & progress:** no episode enters music queue/PlayerSlice. Define
   resume write throttling, source changes and completion percentage before
   implementing an episode player.
4. **Downloads:** no reuse of Spotify's `librespot-go` transport or
   conversion policy. Require dedicated leases, restart recovery and
   verified atomic file publication before enabling offline UI.
5. **Feature exposure:** the first foundation PR is intentionally backend-only;
   no navigation or claimed working Podcasts mode. Choose a backend-owned
   feature flag before user-facing pages are mounted.
6. **Backups:** SQLite `VACUUM INTO` includes all rows. Never store
   authenticated private feed URLs/tokens in plaintext; reject them until
   an explicit encrypted-column design is implemented.
7. **HTTP compatibility:** real podcasts may use plain-HTTP feeds/enclosures.
   Measure compatibility and define a secure, explicit opt-in rather than
   disabling public podcast imports by surprise.
8. **Testability:** deterministic local HTTP fixture tests must not bypass
   production SSRF validation; test fake DNS/dial seams only inside tests.

## Implementation ordering

1. Bounded XML RSS parsing, namespace fields, no external DTD, defensive data limits.
2. Public-IP URL validation and redirect-safe, DNS-pinned fetch.
3. Additive dedicated SQLite podcast schema and transaction-safe feed/episode upsert.
4. Typed REST route, subscription/refresh flow, importer fixtures, API and
   end-to-end tests.
5. React discovery/following UI, then audio session, offline storage and
   advanced controls.

## Known limits of this first milestone

The initial RSS parser is intentionally RSS 2.0 with iTunes namespace fields,
not Atom, Podcasting 2.0 chapters or private feeds. A successfully parsed feed
is not automatically a verified playable episode. There is no periodic
refresh, no network browsing UI, no streaming or downloads yet. These remain
open roadmap items and must not be marked complete.

## Source anchors

- [Architecture](../architecture.md)
- [Main roadmap](../ROADMAP.md)
- `backend/internal/server/server.go`, `backend/internal/api/v2_jobs.go`
- `backend/internal/db/db.go`, `backend/internal/db/jobs_schema.go`
- `hooks/useAudioPlayer.ts`, `hooks/useMediaSession.ts`
- `backend/internal/mediafetch/mediafetch.go`
