# ADR 0004 — Podcast RSS formats, private feed policy, download ownership, and retention

- **Date:** 2026-11-22
- **Status:** Accepted for ViiB podcast ingestion, storage, and retention boundaries
- **Scope:** `POD-00C`
- **Depends on:** [ADR 0001](0001-podcasts-local-first-boundaries.md), [ADR 0002](0002-podcast-byte-range-streaming.md)
- **Roadmap:** [Podcast roadmap](../PODCAST_CLIENT_IMPLEMENTATION_ROADMAP.md)

## Decision

### 1. Accepted RSS / chapter / transcript formats and text sanitization

1. **Core feed format:** ViiB accepts **RSS 2.0** (`<rss><channel><item>`) documents with **Apple iTunes namespace** metadata (`http://www.itunes.com/dtds/podcast-1.0.dtd`) as the canonical subscription format.
2. **Extended episode metadata:** **Podcasting 2.0** chapter and transcript extensions (`podcast:chapters`, `podcast:transcript`) and **Podlove Simple Chapters** (`psc:chapters`) are the accepted standards for structured chapters and transcripts when enabled in Phase 5 (`POD-05A`/`POD-05B`). All external chapter/transcript URLs must pass the same public-network SSRF, redirect, and byte-limit policy as feeds.
3. **XML hardening:** Feed parsing uses strict XML decoding with bounded payload size (`8 MiB`), bounded item count (`1,000` episodes), and outright rejection of `<!DOCTYPE` and `<!ENTITY` declarations to prevent entity expansion (XXE/billion-laughs) and external DTD fetches.
4. **Plain-text / sanitized HTML policy:** Publisher-supplied show and episode descriptions are untrusted input. The baseline parser (`backend/internal/podcast/feed.go`) strips `<script>`/`<style>` blocks and markup tags, unescapes entities, collapses whitespace, and truncates to bounded plain text. Any future rich show-notes rendering must use an allowlisted, non-executable HTML sanitizer that strips scripts, inline event handlers, styles, iframes, and remote tracking pixels.

### 2. Network transport, signed-CDN enclosures, and private feed deferral

1. **Public HTTPS default for feeds, artwork, and OPML:** Feed subscriptions, feed previews, artwork proxies, and OPML export/import permit public **HTTPS** URLs on the default port (`443`) only. Requests reject embedded userinfo, fragments, non-standard ports, non-public IP literals (loopback, RFC 1918, CGNAT `100.64.0.0/10`, link-local/metadata `169.254.0.0/16`, benchmarking, reserved, multicast, NAT64/IPv4-mapped IPv6), and arbitrary query strings (allowing only explicit non-secret feed variants `feed=rss|rss2|podcast` and `format=rss|xml`). Every connection resolves DNS and pins the validated public IP at dial time across at most 4 redirects, blocking HTTPS-to-HTTP downgrades and stripping `Authorization`/`Cookie` headers across redirects.
2. **Media enclosure HTTP/HTTPS signed-CDN policy:** Audio enclosures (`podcast.ValidateMediaURL`) permit public **HTTP** (port `80`) and **HTTPS** (port `443`) without HTTPS-to-HTTP downgrades, and preserve backend-only query parameters to support signed CDN URLs common in public podcast hosting. Raw enclosure URLs and signed query strings remain backend-private and are never serialized in episode JSON, download listings, logs, SSE events, OPML exports, or support bundles.
3. **Deferral of private / authenticated RSS feeds to Phase 8:** Tokenized feed URLs, HTTP Basic/Bearer feeds, cookie-authenticated feeds, and paywalled/private feeds are **explicitly deferred to Phase 8** (`POD-08E`). Private feed support will not be enabled until:
   - feed credentials and private URLs use encrypted-at-rest storage (`enc:v2:` via `backend/internal/crypto`),
   - SQLite `VACUUM INTO` backups and OPML exports exclude or safely handle encrypted credentials, and
   - logs, error payloads, SSE streams, and support bundles are verified by regression tests to redact all private URLs, tokens, and headers.

### 3. Managed download directory ownership and storage bounds

1. **Directory ownership:** Offline podcast audio is stored exclusively under a backend-owned directory at `<dataDir>/podcasts/episodes` created with owner-only permissions (`0700`) and verified at every open/cleanup step to be a real directory free of symlink escapes. Staged transfers use `0600` `.part-*` temporary files and publish atomically via `fsync` + `rename` to deterministic `<episodeID>.audio` filenames (where `episodeID` is a 40-character hex SHA-256 digest). Paths are never derived from publisher metadata or client input.
2. **Future managed path policy:** Any future user-configurable custom folder (e.g., `podcast_download_path`, analogous to `spotify_download_path`) must remain a dedicated ViiB-managed directory subject to the same symlink checks, owner-only permissions, relative-path DB references, and disk-space guards; arbitrary per-episode filesystem paths will never be accepted.
3. **Storage limits and free-space guard:**
   - **Aggregate reservation:** `2 GiB` (`2 << 30` bytes, `db.MaxPodcastOfflineLibraryBytes`) across all `completed`, `downloading`, and `paused` rows (including unfollowed shows until reclaimed).
   - **Per-episode cap:** `512 MiB` (`512 << 20` bytes, `db.MaxPodcastDownloadBytes`) maximum per episode; unknown-length HTTP 200 fallback downloads reserve the full `512 MiB` ceiling up front and reconcile to verified bytes on completion.
   - **Free-space guard:** `128 MiB` (`128 << 20` bytes, `podcastDiskFreeReserve`) minimum free volume capacity must remain above the remaining bytes required for a transfer, checked before starting and between ranged chunks.

### 4. Retention policy (`keep_latest_count`) and protected episodes

1. **Unsubscribe reclamation:** Unsubscribing from a show atomically revokes active download leases and queues bounded background deletion of completed offline files for that show while preserving episode metadata and listening progress.
2. **Per-show `keep_latest_count` retention:** `podcast_subscriptions.keep_latest_count` configures automated count-based pruning of completed offline downloads per subscribed show (`0` disables automatic count pruning). When automated retention runs, it retains at least the `keep_latest_count` newest completed episodes by publication timestamp and **always protects**:
   - **Starred episodes** (`podcast_episode_state.starred = 1`),
   - **Currently playing episodes** (active playback/session items, plus in-progress listening state), and
   - **In-flight or paused downloads** until completed or canceled.
3. **Non-destructive cleanup:** Retention and explicit download deletion remove only the managed `.audio` file and its `podcast_downloads` record; `podcast_episodes`, `podcast_episode_state` (position, played state, starred), and `podcast_bookmarks` are never deleted by download cleanup.

## Source anchors

- `backend/internal/podcast/feed.go`, `backend/internal/podcast/fetch.go`, `backend/internal/podcast/media.go`
- `backend/internal/db/podcast_schema.go`, `backend/internal/db/podcast_downloads.go`
- `backend/internal/api/podcast_download_worker.go`, `backend/internal/api/podcast_download_routes.go`
- [ADR 0001](0001-podcasts-local-first-boundaries.md), [ADR 0002](0002-podcast-byte-range-streaming.md)
