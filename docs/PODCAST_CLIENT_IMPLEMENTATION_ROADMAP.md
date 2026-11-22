# ViiB MediaHub — Podcast Client, Player & Offline Library Roadmap

> **Document status:** Proposed / planning only; no podcast feature is implemented by this document.
> **Created:** 2026-11-22
> **Last reviewed:** 2026-11-22 — codebase anchors re-verified against source (see [§13 Review log](#13-review-log))
> **Target:** ViiB MediaHub (React 19.2 / React Router 8 / TypeScript 5.8 / Zustand 5 / Go 1.26 / SQLite via `modernc.org/sqlite` / Wails v2.17, plus local-Go browser builds)
> **Ownership:** Podcast domain specification. Cross-project sequencing and release gates remain in [docs/ROADMAP.md](ROADMAP.md); the parent roadmap now links to this podcast feature-branch work with its own non-shipping status.
> **Source:** Research of commercial and FOSS podcast clients, official podcast standards, and inspection of ViiB `main` on 2026-11-22.
> **Implementation principle:** An independent podcast domain, integrated into ViiB navigation/player shell without treating episodes as songs or Spotify downloads.

## 0. Executive decision

Build a **local-first, subscription-based podcast client** inside ViiB with first-class RSS feeds, searchable podcast discovery, streaming, reliable optional offline downloads, per-episode listening progress, a podcast-specific queue, and thoughtful long-form audio controls. Provide a full-page Podcasts workspace and reuse existing ViiB shell, design tokens, playback presentation, media session, Go REST/SSE infrastructure, SQLite persistence, and desktop packaging where appropriate.

**Critical design decisions**

1. **RSS is the source of truth.** A directory finds feeds; feeds own episode identity, metadata, enclosure URLs, and refresh cadence. Users can always subscribe directly by RSS URL without a search-provider account.
2. **Podcast state is not music state.** Separate subscriptions, episodes, podcast queue, progress, bookmarks, download policies and files from `songs`, music playlists, Last.fm scrobbling, DJ analysis, AI Smart Mix, and Spotify catalog/download lineage.
3. **One active consumer audio session.** Podcast playback must arbitrate with the existing music player; entering DJ Mode must retain the existing DJ isolation contract. Reuse a shared shell and media-session integration, not force the existing music `Song` type to represent a 3-hour episode.
4. **Go owns feeds and offline media.** Browser and desktop UI communicate with ViiB's local backend. Persistent downloads and trusted media proxying live in Go, not service-worker/browser blob persistence.
5. **Offline is optional and explicit.** Stream without downloading; download one episode or configure per-show automatic policies. Never silently download an entire backlog.
6. **Provider-neutral discovery.** Start with Podcast Index, using an optionally configured server-side key/secret, and provide direct RSS/OPML without a key. Assess an additional directory adapter only after licensing, attribution, rate limits and reliability are reviewed.
7. **Do not promise Spotify/Apple exclusives.** The initial scope covers publisher-provided, accessible RSS enclosures. Private paid feeds require their own legitimate user credentials. No DRM defeat, platform scraping, or claiming access to proprietary catalog files.

### Release definition

**MVP (end of Phase 3):** Search or paste an RSS feed, subscribe, refresh, browse show/episodes, stream, pause/seek/speed/resume, queue, manually download/cancel/retry, switch to completed local media when offline, and export OPML. State survives app/browser reload and Go restart. Native and local-Go browser releases work.

**Full 1.0 podcast feature (end of Phase 6):** Include durable refresh and download policies, polished chapter/notes/transcript UX, smart queues/inbox, lifecycle cleanup, accessibility, robust import/export and support diagnostics.

**Post-1.0:** Silence trim, voice enhancement, cross-device sync, video/HLS, recommendation models, clipping and advanced Podcasting 2.0 functionality; each needs its own compatibility/privacy performance gate.

## 1. Competitive research (November 2026)

Feature descriptions below are based on current official product pages and open-source project documentation; they are **design references, not a request to copy UI/assets or vendor code**. Platform-specific or premium features may differ by device/account.

| Product / type | Relevant verified strengths | Take into ViiB | Deliberate restraint |
|---|---|---|---|
| [Pocket Casts](https://pocketcasts.com/podcast-player) — commercial; [App Store](https://apps.apple.com/us/app/pocket-casts-podcast-player/id414834813) | Cross-device position/subscription syncing, smart playlists, queue, speeds, silence trim, chapters, per-podcast settings | Polished subscriptions grid, Up Next, multi-rule playlists, accessible rich player, progressive enhancement | Hosted account/sync is not an MVP dependency |
| [Overcast](https://apps.apple.com/us/app/overcast-podcast-app/id888422857) — commercial | Smart Speed, Voice Boost, offline files, smart playlists, sleep timer and per-show priorities | Long-form-first controls; optional later DSP, never require DSP for baseline | Voice/silence processing needs measurable CPU, pitch and quality tests |
| [Apple Podcasts](https://www.apple.com/newsroom/2026/02/apple-introduces-a-new-video-podcast-experience-on-apple-podcasts/) — commercial | Large discovery surface, chapters, transcripts, dialogue enhancement and (from 2026) an HLS-based video experience that switches between watching and listening | Legible detail pages, chapter navigation, show/episode search, transcript-first exploration | Apple-generated transcripts and Apple's HLS video/dynamic-ad delivery are not portable RSS features; video/HLS stays post-1.0 (PLAY-16) |
| [Podcast Addict](https://podcastaddict.com/intro.php) — commercial/freemium | Per-feed automatic download, intro/outro skip, fine-grained speed, playlists, bookmarks and cleanup | Powerful expert settings with simple defaults; auto-download/retention | Keep rules understandable; avoid an overwhelming initial Settings page |
| [Castro](https://apps.apple.com/us/app/castro-podcast-app-player/id1080840241) — commercial | Inbox triage: accept, queue or dismiss newly arrived episodes | Optional triage inbox with explicit behavior by show | No auto-play/auto-download just because a feed is subscribed |
| [Spotify Podcasts](https://support.spotify.com/ws/article/podcasts-and-shows/) — commercial | Easy discovery, following, listening and downloads in a broad media app | Cohesive sidebar and global shell; bridge between music and spoken content | Spotify catalog playback/access rights are a distinct integration |
| [AntennaPod](https://antennapod.org/documentation/getting-started) — FOSS | Open RSS subscriptions, independent streaming/downloading, queue, autodownload, offline library and chapter/sleep controls | Local-first model, auto-download caps and storage reliability | Do not copy Android-specific UI patterns blindly |
| [gPodder](https://gpodder.github.io/docs/user-manual) — FOSS | Portable OPML, subscriptions/episode metadata, offline management, optional gpodder.net sync | OPML interoperability, future sync adapter, predictable local DB | Its external-player orientation is not ViiB's target UX |
| [Audiobookshelf](https://audiobookshelf.org/docs/documentation/introduction/) — FOSS | Search/add feeds and automated episodic downloads in a server-backed library | Persistent Go-owned download orchestration, a media library with explicit locations | No always-on server/cloud infrastructure requirement |
| [Podverse](https://podverse.fm/about) — FOSS | Podcasting 2.0 chapters/transcripts, open web/mobile UX, queues, history and optional sync | Consume publisher-supplied transcripts/chapters; open-standard extensibility | Social interactions/value payments are later optional integrations |

**Research synthesis:** Winning experience = Pocket Casts polish + Overcast audio ergonomics + Castro triage + AntennaPod automation + gPodder portability + Podverse open-format richness + Audiobookshelf-grade durable storage.

## 2. Baseline: actual ViiB integration points

Verified against source and repo documentation on 2026-11-22 (no podcast code exists yet anywhere in the frontend or backend):

| Area | Existing ViiB component | Podcast approach |
|---|---|---|
| Routes and shell | [App.tsx](../App.tsx) (`BrowserRouter` from `react-router`, pages loaded with `React.lazy`), [components/Layout.tsx](../components/Layout.tsx), [components/Sidebar.tsx](../components/Sidebar.tsx) | Add lazy `/podcasts`, `/podcasts/discover`, `/podcasts/show/:id`, `/podcasts/episode/:id` (matches existing `/spotify/artist/:id` style; the Go SPA fallback already serves `index.html` for extensionless paths); add Podcasts navigation/badges and responsive views |
| Frontend state | [store.ts](../store.ts) (Zustand `persist` to `localStorage` with an explicit `partialize` allowlist), [slices/playerSlice.ts](../slices/playerSlice.ts), [slices/types.ts](../slices/types.ts); the music `Song` type lives in root [types.ts](../types.ts) | Separate `podcastSlice` for *ephemeral* UI/player state, backend authoritative for subscriptions/progress; keep podcast state out of `partialize` except harmless UI preferences; avoid overloading `Song` or the music queue |
| Audio | [hooks/useAudioPlayer.ts](../hooks/useAudioPlayer.ts) (music crossfade/prebuffer), [hooks/useMediaSession.ts](../hooks/useMediaSession.ts) (Media Session metadata and play/pause/next/prev/seek handlers bound to the music store), [components/Player.tsx](../components/Player.tsx), [docs/player.md](player.md) | Implement podcast transport and audio-session arbitration; adapt shared chrome; make exactly one owner register Media Session handlers at a time (generalize `useMediaSession` or hand ownership to the coordinator) instead of two competing hooks; avoid music crossfade and DJ/song analytics |
| API | [services/api.ts](../services/api.ts), [backend/internal/api/api.go](../backend/internal/api/api.go), route mounting in [backend/internal/server/server.go](../backend/internal/server/server.go), [docs/openapi-v2.yaml](openapi-v2.yaml) | Add typed `services/podcasts.ts` client and a dedicated `PodcastRoutes()` chi router mounted at `/api/v2/podcasts` in `server.go` (same pattern as `PlexRoutes`/`V2JobRoutes`), **before** the catch-all `/api/v2` mount; reuse `respondV2JSON`/`respondV2Error` shapes |
| Database | [backend/internal/db/db.go](../backend/internal/db/db.go) plus per-domain `*_schema.go` files (e.g. [jobs_schema.go](../backend/internal/db/jobs_schema.go)) | There is no migration-version table: domains use idempotent `CREATE TABLE/INDEX IF NOT EXISTS` plus tolerant `ALTER TABLE … ADD COLUMN` inside an `Ensure…Schema()` call. Follow that pattern in `backend/internal/db/podcast_*.go`; maintain existing library integrity |
| Durable work | [backend/internal/api/v2_jobs.go](../backend/internal/api/v2_jobs.go), [backend/internal/db/jobs_schema.go](../backend/internal/db/jobs_schema.go) | Existing `operation_jobs` scheduler has a `supportedJobTypes` allowlist (scans, genre stats, track analysis, stem jobs), priority −100..100, cancel/retry, SSE snapshots at `/api/v2/jobs/events`, and only 1–2 workers (`NumCPU/4`, capped at 2). On restart, running jobs become `interrupted` (except `analyze_tracks`, which requeues). Feed refresh would share those workers with library scans/analysis, so prove it cannot starve or be starved before adopting it; media transfers need their own state and workers |
| Downloads | [backend/internal/api/download_manager.go](../backend/internal/api/download_manager.go), [docs/downloads.md](downloads.md) | Spotify-specific: `spotify_downloads` table, `librespot-go`, Ogg/Vorbis output with optional MP3 conversion, default `<dataDir>/spotify_downloads` overridable by the `spotify_download_path` setting. **Do not reuse its semantics**; create a podcast download worker that reuses proven patterns (SQLite queue, SSE progress, retries/backoff, cancellation, configurable directory setting) |
| Remote image safety | [backend/internal/mediafetch/mediafetch.go](../backend/internal/mediafetch/mediafetch.go) | Image-only `FetchImage`: HTTPS-only, rejects userinfo, resolves and checks every IP before the request and again at dial time, revalidates up to 4 redirects, 12 MiB cap, image decode/dimension checks. Reuse the principles, but implement format-specific feed/enclosure/transcript fetching with its own scheme policy and a broader blocklist (see §6.4) |
| Media serving | `/api/audio/*` registered in [server.go](../backend/internal/server/server.go) → `ServeLocalAudio` in [media_routes.go](../backend/internal/api/media_routes.go) (thin wrapper around `serveAudio` in `api.go`, which uses `http.ServeFile` for ranges) or Plex `ServeAudioSourceAware` for `plex_` IDs; [docs/architecture.md](architecture.md) | Add a distinct podcast media route; local files via `http.ServeContent` (precedent: `v2_stem_preview.go`), secure upstream streaming for remote; do not rewrite music `/api/audio/*`. CORS already allows `Range` and exposes `Content-Range`/`Accept-Ranges`/`Content-Length` |
| Media capabilities | [services/mediaCapabilities.ts](../services/mediaCapabilities.ts) backed by [backend/internal/audio/media-capabilities.json](../backend/internal/audio/media-capabilities.json); [AUDIO_FORMAT_SUPPORT_GAPS.md](AUDIO_FORMAT_SUPPORT_GAPS.md) | Use the canonical matrix and its error codes for enclosure codec support decisions; add a podcast source entry instead of inferring support from file extensions |
| Backup and diagnostics | [backend/internal/db/backup_primitives.go](../backend/internal/db/backup_primitives.go) (`VACUUM INTO` copy), [docs/support-bundles.md](support-bundles.md), credential encryption in [backend/internal/crypto](../backend/internal/crypto) (`enc:v2:`) | Podcast tables are automatically included in DB backups, so anything secret in them (private feed URLs/tokens) must be encrypted with the existing crypto package. Extend the support-bundle allowlist; do not create a second exporter |
| Automation/QA | [package.json](../package.json) (Vitest 4, Playwright 1.59), [e2e](../e2e), [backend/internal/api](../backend/internal/api), [.github/workflows/ci.yml](../.github/workflows/ci.yml), [docs/development.md](development.md) | Vitest + Go unit/integration + Playwright fixtures. CI already runs `go test -race` and an ubuntu/windows/macos matrix; add podcast suites to the existing jobs, including the local-Go browser shell |
| Security boundary | `loopbackTrustBoundary`, `IsTrustedHost` and `IsTrustedOrigin` in [server.go](../backend/internal/server/server.go); [docs/architecture.md](architecture.md) | Existing Host/Origin/`Sec-Fetch-Site` and SSE protection applies to everything mounted on the router and must remain effective. Feed URL import and HTTP redirects create a **new outbound SSRF surface** that this inbound boundary does not cover |

**Important asymmetry:** ViiB's browser *release* runs against a local Go executable, so backend-managed podcast downloads are feasible there. A purely static/frontend-only deployment or IndexedDB fallback is **not** an offline podcast download target in v1; show an explicit backend-required state rather than falsely reporting offline readiness.

**Architectural anti-goals:** no migration of existing `songs` into podcast rows, no `isPodcast` flag on music tracks, no Last.fm scrobbling of episodes, no automatic DJ BPM/beatgrid/stem analysis, no mutation of existing Spotify download rows, no remote feed fetch inside the React renderer.

## 3. Personas, navigation and experience

### User journeys (must work without a Spotify account)

- **Find & follow:** Enter search; see show art/title/author/categories, open show preview, read description, subscribe, and immediately browse recent episodes.
- **Paste feed:** Enter `https://publisher.example/feed.xml`, validate/preview, resolve redirects, subscribe with no catalog account, and report useful parse errors.
- **Try before subscribing:** Stream a discovered episode without following, then optionally subscribe or queue it. Define short-lived/retained episode identity for history.
- **Daily listening:** Podcasts Home shows Continue Listening, Up Next, New Episodes, Downloads, and recently followed shows; one click resumes accurately.
- **Go offline:** Choose Download on an episode or policy on a show; see bytes, speed, ETA when estimable, pause/cancel/retry, storage use and offline-ready badge. Disconnect network, launch ViiB, and finish episode.
- **Power listening:** Set 1.3x for one show, 1x another; skip 15 seconds back or 30 forward; select chapters, bookmark a moment, use sleep timer; maintain progress.
- **Catch-up/triage:** Browse Inbox of new/unplayed episodes, Play Next, Play Later, Archive/Mark Played; optionally auto-queue only opted-in shows.
- **Migration:** Import OPML from another podcast app, handle duplicate/failing feeds without destroying existing subscriptions; export a portable OPML at any time.
- **Mixed media:** Switch from podcast to music and back without overlapping sound, playback bleed, or losing each source's saved position.

### Information architecture

```text
Sidebar
  Home / Search / DJ Mode                  (existing; unchanged)
  Songs / Albums / Artists / Genres / Playlists
  Liked Songs / Liked Albums / AI Smart Mix
  Spotify / Downloads / Stats / Duplicates / Settings
  Podcasts                                 (new group)
    Home            Continue, Up Next, New, Followed, Downloads
    Discover        Search, featured/category results (provider-dependent)
    Following       Subscribed shows and collections
    Episodes        New | Unplayed | In Progress | Played | Saved
    Queue           Podcast-specific Up Next with reorder
    Downloads       Podcast tab + existing Spotify tab, or unified view by type
    History         Played/recent, bounded and user-clearable
    Settings        Feeds, auto-download, cleanup, privacy, storage, import/export
```

**Visual direction:** Follow existing ViiB dark, album-art-rich design, but show **square show cover cards** and **episode rows/cards** with publication date, duration, listened progress, status, download controls and contextual actions. Podcast Now Playing becomes a long-form detail overlay: large cover, elapsed/remaining, scrubber with buffered-range indication, speed, skip durations, sleep, chapter list, notes/transcript, episode queue and device output when supported. Never require a DJ-sized viewport. Skeleton, empty, offline, auth-required, feed-error and retry states are designed explicitly.

**Mobile/responsive:** Persistent compact player; accessible large transport hit targets; no hover-only actions; respect reduced-motion and keyboard navigation; support OS media keys via Media Session API where available.

## 4. Feature backlog and priorities

Legend: **P0** is mandatory for first useful podcast MVP; **P1** full v1 release; **P2** later improvement or research-gated. Each identifier should become a traceable issue/PR/checklist item.

### 4.1 Discovery and feed intake

| ID | Priority | Requirement / acceptance behavior |
|---|---|---|
| DISC-01 | P0 | Podcast directory abstraction; search show title/author/keywords, debounce, cancel, pagination, provider fallback/error/empty states; no frontend directory secret |
| DISC-02 | P0 | Add by arbitrary public RSS feed URL; validate before subscribe; preview title, artwork, publisher, language and episode count |
| DISC-03 | P0 | Follow/unfollow, show detail with newest/oldest, search/filter by played/unplayed, show website/share links |
| DISC-04 | P0 | Conditional refresh via ETag/Last-Modified, sane backoff, explicit Refresh Now and last-success/error state |
| DISC-05 | P1 | Search within an individual show's episode titles/descriptions and across locally indexed episodes |
| DISC-06 | P1 | Search episode catalog when licensed provider supports it; gracefully distinguish remote show search from local episode search |
| DISC-07 | P1 | Browse curated categories/trending from chosen directory with optional categories cache and age/explicit-content filters |
| DISC-08 | P0 (export) / P1 (import) | OPML 2.0 export is part of the MVP (POD-03F). Import adds preview counts, duplicate resolution, a partial-failure report, and keeps feeds independent (POD-04E) |
| DISC-09 | P1 | Recognize `podcast:guid`, feed move/redirects, enclosure changes and feed image changes without duplicating follows/history |
| DISC-10 | P2 | Search suggestions, discovery from current subscriptions, share/import deep links, extensible alternate providers |
| DISC-11 | P2 | Private authenticated feeds (Basic/Bearer or publisher-supported custom scheme) with secrets backend-only and per-feed access state |
| DISC-12 | P2 | RSS and Atom feature compatibility expansion beyond validated baseline, Podcasting 2.0 `podcast:medium` and alternate enclosures |

### 4.2 Following, organization and listening workflow

| ID | Priority | Requirement / acceptance behavior |
|---|---|---|
| LIB-01 | P0 | Subscriptions persist in SQLite and show update time, unread/new count, errors and configurable default sort |
| LIB-02 | P0 | Durable episode identity with GUID/feed scope, metadata, enclosure, duration, status and playback position |
| LIB-03 | P0 | Continue Listening and podcast-specific Up Next queue; add-next/add-last, reorder/remove/clear, resume after restart |
| LIB-04 | P0 | Mark played/unplayed, mark saved/starred, hide played without deleting the feed or media |
| LIB-05 | P1 | Inbox with New/Unplayed/In Progress/Played/Archived, batch actions and optional Castro-like triage |
| LIB-06 | P1 | Per-show policies: auto-queue new episodes, queue priority, sorting oldest/newest, download preferences and episode limits |
| LIB-07 | P1 | Custom playlists and smart filters (duration, date, show, unplayed, downloaded, star) without contaminating music playlists |
| LIB-08 | P1 | Listening history, completed-at, last played; optional playback statistics with clear/reset controls |
| LIB-09 | P1 | Per-show exclusion/search filters and sort, optional automatic archive after playback |
| LIB-10 | P2 | Folder/tags, pinned/favorite shows, collections and intelligent backlog recommendations |
| LIB-11 | P2 | Import listening state from compatible open standards/sync services with explicit preview/merge semantics |

### 4.3 Streaming and podcast-centric player

| ID | Priority | Requirement / acceptance behavior |
|---|---|---|
| PLAY-01 | P0 | Stream supported RSS enclosure without downloading; Go-mediated source or validated local origin, seeking via HTTP Range where supported |
| PLAY-02 | P0 | Play/pause, adjustable 0.5x–3x in 0.1 or sensible increments, native pitch-preserving rate (`HTMLMediaElement.preservesPitch`) where supported, 15s back/30s forward defaults |
| PLAY-03 | P0 | Resume per episode on reload/app restart and after switching music/podcasts; coalesced progress updates and final pause/close flush |
| PLAY-04 | P0 | Podcast session interrupts/stops music; music interrupts/stops podcast; never play concurrently; DJ behavior remains isolated |
| PLAY-05 | P0 | Explicit streaming vs downloaded source selection; completed, verified local file takes priority offline; clear offline/unavailable errors |
| PLAY-06 | P0 | Seekbar elapsed/remaining, duration fallback for unknown/live media, loading/buffering/error states, headphones/media key semantics |
| PLAY-07 | P1 | Podcast-specific per-show playback speed, seek-jump settings, auto-rewind after pause, configurable start/end skip offsets |
| PLAY-08 | P1 | Sleep timer (5/10/15/30/60 min, end of episode, end of queue) and safe cancel/restart behavior |
| PLAY-09 | P1 | Chapter navigation with title/time/artwork from published metadata, chapter progress and next/previous chapter |
| PLAY-10 | P1 | Queue auto-advance and completion policy; configurable played threshold (e.g. 95% or final small tail) without mistaken early completion |
| PLAY-11 | P1 | Bookmarks at timestamps, notes, jump-to-bookmark, episode notes hyperlinks, safe external-link handling |
| PLAY-12 | P1 | Global and per-show volume/normalization options where browser/Web Audio capabilities permit; no music EQ-setting regression |
| PLAY-13 | P2 | Silence trim/Smart Speed with pitch/time correctness, CPU/battery profiling, seek/chapters compatibility and bypass control |
| PLAY-14 | P2 | Speech boost/dialogue enhancement with transparent DSP + A/B quality test, no artificial loudness claims |
| PLAY-15 | P2 | Cast/AirPlay/Chromecast/network output only after platform capability/auth/privacy analysis |
| PLAY-16 | P2 | Video and HLS audio+video playback/download **separate** from basic progressive MP3/M4A; qualify DRM and legal support |

### 4.4 Offline downloads, retention and network resilience

| ID | Priority | Requirement / acceptance behavior |
|---|---|---|
| DOWN-01 | P0 | Download single episode from authorized publisher RSS enclosure, choose library path; reserve safe destination and write temp file |
| DOWN-02 | P0 | Persistent states queued/downloading/paused/completed/failed/canceled; recover abandoned jobs on restart; progress SSE + snapshot reconciliation |
| DOWN-03 | P0 | Cancel/retry with bounded concurrency, timeouts, exponential backoff + jitter and clear error codes |
| DOWN-04 | P0 | Validate final bytes, MIME/extension, actual media decodability or conservative format probe; only atomic rename into playable location on success |
| DOWN-05 | P0 | Offline playback after Go/app restart and network disconnect; corrupt/missing local file cannot be represented as downloaded |
| DOWN-06 | P0 | Show used bytes, available disk and human-readable transfer progress, clean manual delete without erasing listening state |
| DOWN-07 | P1 | HTTP 206 resumable partial downloads **only if** validator/If-Range and size semantics are safe; restart cleanly otherwise |
| DOWN-08 | P1 | Per-show auto-download: newest N or unplayed N, never full archive; global Wi-Fi/metered policy where platform telemetry is reliable |
| DOWN-09 | P1 | Global/per-show storage quotas, free-space threshold, retain pinned/unplayed/currently playing, configurable delete-after-played + grace period |
| DOWN-10 | P1 | Downloads page podcast filter, aggregate activity, pause-all/resume-all, bulk delete/repair, detailed failure states |
| DOWN-11 | P1 | Avoid duplicate download for same episode, serialized file mutation, stable hashed/sanitized paths, protection from symbolic-link/path escapes |
| DOWN-12 | P1 | Enclosure host throttling, Retry-After respect, redirection and auth expiry handling, HTTP 416 recovery, offline/back-online retry |
| DOWN-13 | P2 | Portable destination change with verified move/relink/reconciliation and rollback |
| DOWN-14 | P2 | Optional automatic download scheduling (quiet-hours, power source or schedule) after platform-specific capability checks |

**Hard rule:** Spotify music downloads and RSS podcast downloads are distinct workflows with separate authorization and provenance. Spotify `librespot-go` Ogg→MP3 conversion and credential assumptions do **not** apply to open RSS enclosures.

### 4.5 Metadata, accessibility and future content

| ID | Priority | Requirement / acceptance behavior |
|---|---|---|
| META-01 | P0 | RSS 2.0 + `itunes:` core show/episode fields, XML safety, encoding, HTML sanitization, stable GUID; graceful missing/malformed fields |
| META-02 | P0 | Art caching with bounded size, stale fallback and no secrets in browser-visible URLs |
| META-03 | P1 | Publisher-supplied chapters: `podcast:chapters` JSON, compatible ID3/MP4 chapter tags when decoder/metadata stack supports them |
| META-04 | P1 | Publisher-supplied `podcast:transcript`: VTT/SRT/JSON/HTML as permitted, timestamped follow-along, search within loaded transcript |
| META-05 | P1 | Safe show notes with allowed markup, external-links, funding link when supplied, explicit author/episode artwork/category labels |
| META-06 | P1 | Keyboard/a11y: screen-reader semantic order, controls labeled with action/time, visible focus, virtualized list accessibility |
| META-07 | P1 | Data export/backup, retention/privacy notice, explicit deletion of histories and downloads, no tracking telemetry by default |
| META-08 | P2 | Feed-level language, transcript language selection, alternate audio, soundbites, person/funding/value metadata when appropriate |
| META-09 | P2 | Optional on-device transcription and transcript embeddings, opt-in indexing, storage/CPU budgets; never silently upload private feeds |
| META-10 | P2 | Episode clip creation/export with publisher rights/permissions and clearly scoped offline source |

## 5. Directory, RSS and interoperability design

### 5.1 Directory adapter

**Initial choice:** [Podcast Index API](https://github.com/Podcastindex-org/docs-api), using its documented `/api/1.0/search/byterm`, `/api/1.0/podcasts/byfeedid` and `/api/1.0/podcasts/trending` endpoints. Requests are authenticated with `X-Auth-Key`, `X-Auth-Date` and an `Authorization` header holding the SHA-1 of key + secret + date, plus an identifying `User-Agent`. Register application credentials and verify the current [terms](https://api.podcastindex.org/tos_v1.1.pdf) and attribution/rate limits **before deployment**. The Go backend owns API key/secret and request signing. Never ship a static shared private secret inside public source, desktop binary or TypeScript bundle. Allow user-configured personal credentials or an approved deployable backend approach; define UX when no directory credentials are configured.

**Fallback:** Manual RSS URL and OPML must always work without Podcast Index credentials. A secondary provider such as [Apple iTunes Search API](https://developer.apple.com/library/archive/documentation/AudioVideo/Conceptual/iTuneSearchAPI/) requires separate review of permitted promotion, attribution, caching and artwork terms; do **not** assume arbitrary Apple metadata may be repurposed for independent players. Avoid hard dependencies on commercial paid APIs.

Proposed Go interface:

```go
type DirectoryProvider interface {
    SearchShows(ctx context.Context, query SearchQuery) ([]DirectoryShow, error)
    Trending(ctx context.Context, filter TrendingFilter) ([]DirectoryShow, error)
    ResolveFeed(ctx context.Context, providerID string) (FeedReference, error)
}
```

Keep canonical feed URL/provider IDs distinct; never use a directory record as authoritative episode history identity. Cache directory results briefly, with age and error/attribution metadata. Implement predictable 429 handling.

### 5.2 Feed ingestion contract

- Support bounded RSS XML with `channel`, `item`, `guid`, `enclosure`, `pubDate`, `description`, `itunes:duration`, `itunes:image`, explicit, season/episode/type, categories and `podcast:` namespaces. Accept namespace by URI, not presumed prefix.
- Atom entry/enclosure support is a researched compatibility enhancement; avoid claiming broad Atom equivalence until fixtures and product rules exist.
- Identity preference: publisher `podcast:guid` for feed where valid + normalized final feed identity; item `guid` scoped to feed; deterministic fallback hash of normalized enclosure URL + date/title if GUID absent, with collision handling. Preserve aliases across canonical redirects and GUID repairs. Never equate episodes globally by title.
- Subscription refresh: `If-None-Match`/`If-Modified-Since`, honor 304, bounded 301/302/307/308 redirect handling, respect `Retry-After`, use jittered schedules and avoid fetch storms on app startup.
- Do not delete historical episodes just because a publisher truncates RSS history; mark stale/unlisted while preserving playback, download and user-state records.
- Parse duration formats, timezone dates, HTML entities, Unicode and unusual enclosures defensively. Item order does not imply publish date. Multiple enclosures and changing CDN signed URLs need explicit selection/expiry policy.
- If an episode is missing a playable enclosure, render metadata-only with unavailable reason; do not silently invent audio.
- Podcasting 2.0: begin with [chapters](https://podcasting2.org/docs/podcast-namespace/tags/chapters), [transcripts](https://podcasting2.org/docs/podcast-namespace/tags/transcript) and [GUID](https://podcasting2.org/docs/podcast-namespace/tags/guid), preserving unsupported structured tags where safe and useful for future migrations.

### 5.3 OPML

Import XML outlines containing `xmlUrl` (feed URL); handle nested folders, XML entities, duplicates, invalid/non-HTTP schemes, 10k+ entries via bounded batching, partial success and progress/cancel. Export UTF-8 OPML 2.0 with subscribed feeds and human titles (not secrets); test round-trip with AntennaPod/gPodder fixtures. Private-feed URL tokens must **never** be exported without explicit, conspicuous consent.

## 6. Proposed technical architecture

```text
React: PodcastHome / Discover / Show / Episode / Downloads / PlayerOverlay
  -> services/podcasts.ts -> /api/v2/podcasts/*
  -> Zustand podcastSlice (UI + current session only; not durable truth)
                        |
                 Go podcast API module
          +-------------+------------------+
          |             |                  |
    Directory adapter  Feed ingester     Media resolver
          |             |                  |
  Podcast Index API   RSS/HTTP           local verified file
                      parser/cache         OR RSS enclosure proxy
          |             |                  |
          +------- SQLite repository -------+
                  subscriptions / episodes
                  progress / queue / settings
                  downloads / transfer leases
                        |
            bounded refresh/download workers
                        |
              podcast_downloads/ (user-owned)
              .part -> validate -> fsync/rename
```

**Suggested paths (create only as PRs land):**

```text
backend/internal/podcast/
  directory.go, feed_fetch.go, feed_parse.go, identity.go,
  model.go, download_worker.go, media.go, security.go
backend/internal/db/
  podcast_schema.go, podcast_repository.go, podcast_progress.go,
  podcast_downloads.go
backend/internal/api/
  podcast_routes.go, podcast_media.go, podcast_events.go
services/
  podcasts.ts, podcasts.test.ts
slices/
  podcastSlice.ts
hooks/
  usePodcastPlayer.ts   (Media Session: generalize existing useMediaSession.ts
                         rather than adding a second competing hook)
pages/
  Podcasts.tsx, PodcastDiscover.tsx, PodcastShow.tsx,
  PodcastEpisode.tsx, PodcastDownloads.tsx
components/podcasts/
  ShowCard.tsx, EpisodeRow.tsx, PodcastPlayer.tsx,
  EpisodeQueue.tsx, ChapterList.tsx, TranscriptView.tsx,
  DownloadBadge.tsx, FeedStatus.tsx, PodcastSettings.tsx
fixtures/podcasts/
  *.xml, *.opml, *.json, *.mp3, *.m4a
e2e/
  podcasts.spec.ts
docs/
  podcasts.md, podcast-api.md, PODCAST_CLIENT_IMPLEMENTATION_ROADMAP.md
```

### 6.1 SQLite model (illustrative; finalize constraints in Phase 0)

```sql
podcast_feeds(
  id TEXT PRIMARY KEY, canonical_url TEXT NOT NULL UNIQUE,
  podcast_guid TEXT, title TEXT NOT NULL, author TEXT, description TEXT,
  image_url TEXT, website_url TEXT, language TEXT, explicit INTEGER,
  etag TEXT, last_modified TEXT, last_checked_at INTEGER,
  last_success_at INTEGER, next_refresh_at INTEGER, fetch_status TEXT,
  last_error_code TEXT, created_at INTEGER, updated_at INTEGER
)
podcast_feed_aliases(
  original_url TEXT PRIMARY KEY, feed_id TEXT NOT NULL REFERENCES podcast_feeds(id)
)
podcast_subscriptions(
  feed_id TEXT PRIMARY KEY REFERENCES podcast_feeds(id), subscribed_at INTEGER,
  notifications INTEGER, auto_download_policy TEXT,
  keep_latest_count INTEGER, auto_queue_policy TEXT,
  default_speed REAL, skip_intro_seconds INTEGER, skip_outro_seconds INTEGER
)
podcast_episodes(
  id TEXT PRIMARY KEY, feed_id TEXT NOT NULL REFERENCES podcast_feeds(id),
  guid TEXT, identity_key TEXT NOT NULL, title TEXT NOT NULL, description TEXT,
  published_at INTEGER, duration_ms INTEGER, enclosure_url TEXT,
  enclosure_mime TEXT, enclosure_bytes INTEGER, artwork_url TEXT,
  chapters_url TEXT, transcript_url TEXT, listed_in_feed INTEGER,
  created_at INTEGER, updated_at INTEGER,
  UNIQUE(feed_id, identity_key)
)
podcast_episode_state(
  episode_id TEXT PRIMARY KEY REFERENCES podcast_episodes(id),
  position_ms INTEGER NOT NULL DEFAULT 0,
  listened_ms INTEGER NOT NULL DEFAULT 0, is_played INTEGER DEFAULT 0,
  completed_at INTEGER, last_played_at INTEGER,
  starred INTEGER DEFAULT 0, archived INTEGER DEFAULT 0, revision INTEGER DEFAULT 0
)
podcast_queue(
  episode_id TEXT PRIMARY KEY REFERENCES podcast_episodes(id),
  sort_key REAL NOT NULL, added_at INTEGER NOT NULL
)
podcast_downloads(
  id TEXT PRIMARY KEY, episode_id TEXT NOT NULL UNIQUE REFERENCES podcast_episodes(id),
  status TEXT NOT NULL, destination_relative_path TEXT,
  temp_relative_path TEXT, bytes_received INTEGER DEFAULT 0,
  expected_bytes INTEGER, etag TEXT, last_modified TEXT,
  verified_size INTEGER, sha256 TEXT, error_code TEXT,
  retry_after INTEGER, created_at INTEGER, updated_at INTEGER,
  completed_at INTEGER, lease_owner TEXT, lease_expires_at INTEGER
)
podcast_bookmarks(
  id TEXT PRIMARY KEY, episode_id TEXT NOT NULL REFERENCES podcast_episodes(id),
  position_ms INTEGER NOT NULL, note TEXT, created_at INTEGER
)
```

Schema notes:

- Follow the existing `Ensure…Schema()` convention (no migration-version table): `CREATE … IF NOT EXISTS` plus `ALTER TABLE … ADD COLUMN` that tolerates "duplicate column", with indexes created only after column migrations succeed. Add indexes on `(feed_id,published_at)`, state status, refresh deadlines, queue order and download state. Document foreign-key cascade vs retain-history semantics before implementation.
- Subscription removal should allow **keep downloads/history** vs **delete content and state**; do not accidentally cascade user progress or media files.
- Store durable state in SQLite, not localStorage. Frontend may cache transient display data only. Progress writes must be throttled yet flush at pause/seek/visibility/lifecycle boundaries and use a revision/timestamp to prevent stale overwrites.
- Store **relative** file paths beneath a managed root, plus verified metadata; validate resolution against that root on access. Do not persist arbitrary URL-auth tokens in plaintext or expose secrets via backup, SSE, error logs or OPML. The existing `VACUUM INTO` backup copies every table, so encrypt secret columns with `backend/internal/crypto` (`enc:v2:`) instead of relying on backup exclusion.
- Feed fetch metadata and directory cache may need separate tables from subscriptions; avoid conflating discovered result with followed show.
- Never treat filesystem deletion and database state transition as one unguarded operation: use a recovery/reconciliation state machine.

### 6.2 REST/API contracts (proposal)

Under `/api/v2/podcasts` (mounted in `backend/internal/server/server.go` ahead of the generic `/api/v2` mount) with existing ViiB `respondV2JSON`/`respondV2Error` shapes and the router-wide `loopbackTrustBoundary` Host/Origin controls:

| Method/route | Contract |
|---|---|
| `GET /search?q=&cursor=` | Directory search with provider, pagination and diagnostic source |
| `GET /trending?category=&lang=` | Provider-backed discover feed where enabled |
| `POST /feeds/preview` | Validate candidate RSS URL and return bounded preview without subscribing |
| `GET /feeds` / `POST /feeds` | List subscriptions / subscribe idempotently |
| `GET /feeds/{id}` / `DELETE /feeds/{id}` | Show details / unsubscribe (keep-user-data policy explicit) |
| `POST /feeds/{id}/refresh` | Schedule safe refresh; return job identifier, not block HTTP |
| `GET /feeds/{id}/episodes?cursor=&filter=` | Cursor-pagination; stable sort |
| `GET /episodes/{id}` | Metadata, state, availability and download status |
| `PATCH /episodes/{id}/state` | Position, played, saved and archived with validated state revision |
| `POST /queue` / `GET /queue` / `PUT /queue` | Add/retrieve/replace ordered podcast queue atomically |
| `POST /episodes/{id}/downloads` | Idempotent enqueue; cannot bypass rights/validation |
| `GET /downloads` / `POST /downloads/{id}/{pause,resume,cancel,retry}` | List durable transfers and control state |
| `DELETE /downloads/{id}` | Delete verified local episode bytes, retain episode/history |
| `GET /episodes/{id}/media` | Prefer verified local; otherwise safe remote streaming with supported HTTP ranges |
| `GET /feeds/{id}/artwork` / `GET /episodes/{id}/artwork` | Bounded cached proxy for show and episode art (episode falls back to show art) |
| `GET /episodes/{id}/chapters` / `GET /episodes/{id}/transcript` | Validated metadata/transcript format with cache |
| `GET /events` | SSE for refresh/download changes with reconnect and snapshot refetch (follow the `/api/v2/jobs/events` pattern: initial snapshot, change-driven snapshots, fallback poll, 15 s heartbeat) |
| `GET /opml` / `POST /opml/import` | Portable export / bounded import job + result report |
| `GET /settings` / `PATCH /settings` | Policies, destination and directory status; secrets redacted |

All mutating endpoints must enforce existing trusted-origin policy and request-size/content-type constraints. Define typed error codes, e.g. `feed_invalid`, `blocked_remote_target`, `provider_not_configured`, `auth_required`, `unsupported_enclosure`, `disk_full`, `resume_mismatch`, `download_corrupt`, `offline`. Add OpenAPI v2 schemas and TS typed clients alongside implementation. SSE must not be sole truth for progress.

### 6.3 Streaming & audio-session arbitration

- Introduce discriminated `PlaybackItem = {kind:'music', songId} | {kind:'podcast', episodeId}` at the **playback coordinator boundary** only. Do not refactor every music component upfront.
- Initially implement separate `usePodcastPlayer` audio element/transport behind the shared Player shell or dedicated Podcast Player with one centralized coordinator enforcing a **single active player**. Unmount/stop previous source on switch, save progress, preserve music queue. Explicitly handle crossfade/prebuffer cleanup.
- Podcast source resolver: `verified-local-file > safe-HTTP-RSS-enclosure > unavailable`. For local, use `http.ServeContent`/range-compatible serving; for remote, propagate accurate `206`, `416`, `Accept-Ranges`, `Content-Range`, `Content-Type`, and cancel when client disconnects. Strip cross-origin upstream credentials/unsafe hop-by-hop headers.
- Configure `preload='metadata'` initially; never pre-download the whole next podcast episode. Handle media seek after metadata available and reconnect to original position on transient source changes.
- Podcast playback does not count as music listens, change music shuffle/repeat, enter DJ queue, trigger music analysis, or activate music crossfade.
- Media Session API needs podcast-specific title/show/art, seek-back/seek-forward/seekto actions and platform fallback. The existing `hooks/useMediaSession.ts` already registers these actions but reads the music store directly (`currentSong`, `nextSong`, `prevSong`). Route handler registration through the coordinator so only the active source owns the session. Respect current [Player](player.md) and [DJ](dj-mode.md) behavior.

### 6.4 Secure remote media and parser requirements (P0 gate)

Feeds, episode enclosures, artwork, chapters and transcripts are all **untrusted remote URLs**. A malicious feed can point to `127.0.0.1`, private LAN, IPv6 link-local, cloud metadata endpoints or DNS-rebinding targets.

- Default **public web only**: explicit HTTP/HTTPS scheme policy (HTTPS by default; legacy HTTP feeds/enclosures only under the POD-00C policy, never an HTTPS→HTTP redirect downgrade when credentials are attached), reject userinfo/unsupported ports/schemes, resolve and pin remote public IP per dial, check *every* redirect hop and DNS resolution, bound redirect count and response bytes, protect IPv4/IPv6/private/loopback/link-local/multicast/reserved addresses. Never trust `Host` or a redirect target merely because initial URL was public.
- Do **not** assume `backend/internal/mediafetch` directly secures audio/XML; it is image-specific, HTTPS-only, buffers the whole body (12 MiB cap) and requires a decodable image. Its `isPublicIP` check covers loopback, RFC 1918/ULA private, unspecified, multicast and link-local (including `169.254.169.254` metadata) but **not** CGNAT `100.64.0.0/10`, benchmark `198.18.0.0/15`, documentation/reserved ranges, `0.0.0.0/8` or NAT64/IPv4-mapped forms. Extract vetted request-policy primitives with an extended blocklist, or build tested equivalents for each media class; enclosures must stream, not buffer.
- Enforce compressed/uncompressed XML caps, parse time budgets, prevent entity expansion/DTD/network entity fetch, strip unsafe markup/scripts, cap episode count, title/body lengths and feed redirects.
- Handle private-feed auth via backend-only encrypted secrets; redact query tokens, Authorization, private URLs and exception traces from logs, support bundles, frontend, analytics and shared links.
- Use TLS verification, timeouts, maximum concurrent per-host connections and bandwidth controls. Preserve podcast CDN range requests but never forward arbitrary client-supplied sensitive headers.
- Sanitize file names, reject symlink escapes, enforce owner-only file/dir permissions where possible, check disk quota, guard against oversized declared/actual enclosure, and validate media before publication.
- Add regression tests for IPv4/IPv6 DNS rebinding, redirect-to-LAN, HTTP 302 chains, XML bombs, poisoned transcripts/art, malformed range headers, token leaks and partial-file traversal.
- Data ownership: podcast content remains publisher-controlled. Respect RSS access restrictions, publisher licenses, private feed authorization and acceptable-use requirements; do not bypass paywalls/DRM or infer permission to republish downloaded files.

## 7. Phased delivery plan and traceable PR sequence

**Overall status:** Phase 0 partially completed; Phase 1 in progress. A discoverable Podcast Following/Show Preview UI and optional feed refresh are implemented on the feature branch. Podcast directory discovery, streaming playback and offline downloads are not implemented. Future phases remain planned. Sequence is dependency-aware. Keep PRs reviewable (one vertical behavior or small layer), add tests in the same PR, update this roadmap checkboxes and maintain a changelog as implementation begins.

### Phase 0 — Architecture, compliance and decision gates

- [x] **POD-00A (ADR):** Domain boundary, single-active-session arbitration direction, API routing, startup and backup constraints documented in [ADR 0001](adr/0001-podcasts-local-first-boundaries.md). Music-player refactor remains later work.
- [ ] **POD-00B (ADR):** Confirm Podcast Index credentials/deployment strategy, terms/attribution, and behavior without a directory key; review Apple fallback licensing.
- [ ] **POD-00C (ADR):** Define RSS namespace/format acceptance, allowed HTTP URLs, private feed policy, download folder ownership and retention semantics.
- [ ] **POD-00D (in progress):** Added basic RSS/iTunes XML fixture, parser/conditional-GET/redirect/oversize tests and synthetic browser UI journey in `e2e/podcasts.spec.ts`; GUID-change, OPML, transcript, chapters and 206/416 fixture coverage remains.
- [ ] **POD-00E:** Define player switching lifecycle, media-session semantics, progress completion threshold and API error shapes.
- [ ] **POD-00F:** Design review of podcast home/show/episode/player and responsive accessibility; identify key design tokens.
- [ ] **POD-00G (in progress):** Podcasts is now a permanent Sidebar destination and `/podcasts` routes are accessible without opting in. `podcasts_preview_enabled` is retained solely as an opt-in for automatic/manual RSS refresh; the Show page provides its own toggle and Settings remains an alternative. RSS following/browsing is available with the flag off. Frontend/E2E qualification remains.

**Exit gate:** Approved ADR(s), fixture corpus, threat model, API schema draft, observable end-to-end test plan; no feature shipped as functional before integration tests exist.

### Phase 1 — Persistent domain, safe RSS and following

- [ ] **POD-01A (in progress):** Added additive `EnsurePodcastSchema()`, isolated tables/indexes, transaction-safe feed/episode snapshot upserts, listing, unsubscribe and reopen/progress-preservation tests. Revision-checked durable episode progress methods and restart/conflict tests now added; schema upgrades, player integration, backup/restore qualification and CI remain.
- [ ] **POD-01B (in progress):** Implemented bounded RSS 2.0/iTunes parser, plain-text notes, DTD block, HTTPS-default fetcher, public-IP DNS pinning, redirect/size/304 controls and unit tests. Full reserved-address audit, feed identity churn, download/media policy and CI confirmation remain.
- [ ] **POD-01C (in progress):** Added crash-recoverable SQLite refresh leases, due scheduling, 304/ETag handling, bounded retry backoff and transaction-safe episode reconciliation preserving history. Dedicated serial polling worker started via API constructor and closed by API lifecycle; opt-in traffic gate, manual API, show-page Refresh control and worker/API/E2E tests implemented. Transactional public redirect aliases on follow and refresh, canonical/original-alias collision rollback, ID-reuse tests and a preview→subscribe original-URL fixture are implemented; Conservative GUID migration with strict metadata matching is implemented; real publisher churn, stress and full CI remain.
- [ ] **POD-01D (in progress):** Added guarded REST preview/subscribe/list/episodes/unsubscribe handlers under `/api/v2/podcasts`, typed `services/podcasts.ts` client and tests plus [provisional API documentation](podcast-api.md). Show detail endpoint, cursor pagination, OpenAPI schemas and full API integration qualification remain; bounded offset pagination is implemented; durable manual refresh/status endpoints are implemented.
- [ ] **POD-01E (in progress):** Added an always-accessible Podcasts (Early Access) section with RSS preview/follow form, Following grid, show detail, episode list, unsubscribe confirmation, loading/error/empty states. SSRF-guarded raster artwork proxy, safe URL filtering, followed/show cover art, local Following search and filter-loaded-episodes UI with tests implemented; visual/compatibility review remains.

**Exit:** Paste a real supported RSS URL, see show and episodes, restart process, retain subscriptions/episode IDs, refresh without duplicates; malicious remote URLs blocked.

### Phase 2 — Discovery and core streaming

- [ ] **POD-02A:** Podcast Index adapter, server-side credential management, status and pagination; no-key fallback/direct RSS works.
- [ ] **POD-02B:** Discover search UI, results with artwork, preview/follow, feed detail and episode filter.
- [ ] **POD-02C:** Safe local/remote episode media handler supporting HTTP Range, content-type, source availability and cancellation.
- [ ] **POD-02D:** Podcast audio transport, single-session music↔podcast coordination, media keys and progress persistence.
- [ ] **POD-02E:** Podcast Now Playing shell: playback, seek, speed, +30/-15, show art and metadata, buffering/error/retry.
- [ ] **POD-02F:** Podcast queue/Continue Listening and basic played/starred controls.

**Exit:** Discovery or RSS → subscribe → stream → pause → quit/reopen → resume at expected position; music, Spotify and DJ behavior unchanged.

### Phase 3 — Reliable optional offline MVP

- [ ] **POD-03A:** Dedicated podcast transfer table/worker; persistent states and restart recovery, cancellation/retry, bounded concurrency.
- [ ] **POD-03B:** Safe file lifecycle (unique temp file, disk threshold, validated bytes, atomic publish, relink on restart).
- [ ] **POD-03C:** Streaming-to-offline source preference and local media HTTP Range.
- [ ] **POD-03D:** Download buttons/progress badges, manual delete, dedicated Podcasts Downloads filter/view, SSE snapshot reconciliation.
- [ ] **POD-03E:** Integration tests for intermittent network, no Content-Length, disk full, corrupt file, interruption, path escape and true offline playback.
- [ ] **POD-03F:** OPML export and minimal docs/empty-state tutorial.

**MVP exit / milestone:** Manual search + feed + subscribe + stream + queue + speed + resume + offline download and playback across reboot on Windows/macOS/Linux and local-Go browser builds. No P0 security or data-loss regressions. **Do not merge this as one giant PR.**

### Phase 4 — Automation, housekeeping and power-user flow

- [ ] **POD-04A:** Per-show auto-download policies (newest N/unplayed N), global cap, host limits, dedup and opt-in defaults.
- [ ] **POD-04B:** Resume partial transfers with strong validator checks; 416/retry/range tests, disable resume when unsafe.
- [ ] **POD-04C:** Quota, retention, cleanup grace period, pinned/current protection and crash-safe file garbage collection.
- [ ] **POD-04D:** Inbox / Queue triage, auto-queue optional policies, bulk mark played/archive and rich episode filters.
- [ ] **POD-04E:** OPML import with preview, duplicate reconciliation, per-feed failure report and cancelable job.
- [ ] **POD-04F:** Per-show speed/seek/start/end skip defaults; global and show controls in Settings.
- [ ] **POD-04G:** Smart podcast playlists; keep isolated from music playlist DB and Smart Mix heuristics.

**Exit:** Fresh episodes are fetched and optionally downloaded under deterministic user rules, never exceed configured storage bounds, and do not remove pinned/unplayed/current playback unexpectedly.

### Phase 5 — Chapters, transcripts and polished player

- [ ] **POD-05A:** Podcasting 2.0 chapters JSON and source-file chapter adapters with stable timestamp model.
- [ ] **POD-05B:** Transcript ingest/render for validated publisher-supplied formats, click-to-seek, text search and accessibility.
- [ ] **POD-05C:** Bookmark list, detailed show notes, safe link and rich episode metadata.
- [ ] **POD-05D:** Sleep timer, chapter skip, configurable auto-rewind and playback completion options.
- [ ] **POD-05E:** Visual polish, keyboard focus/a11y, mobile/desktop responsiveness, reduced motion and screen-reader flows.
- [ ] **POD-05F:** Local search across subscribed shows and episodes with capped indexes, bounded results and debounced navigation.

**Exit:** Tested multi-hour episodes, changing artwork/chapters and transcript formats with fast seeking; no media session collisions or unbounded memory use.

### Phase 6 — Reliability, documentation and v1 release qualification

- [ ] **POD-06A:** Deep offline-first behavior: startup unavailable network, feed retry, download repair, deleted/corrupt-file reconciliation.
- [ ] **POD-06B:** Feed and download observability: structured redacted errors, job status, podcast entries in the existing support-bundle allowlist ([support-bundles.md](support-bundles.md)) and user-accessible diagnostics.
- [ ] **POD-06C:** Migrate/restore DB fixtures, OPML round-trip, export privacy/backup behavior and unsubscribe choices.
- [ ] **POD-06D:** CI hardening: add podcast suites to the existing `ci.yml` jobs (which already run `go test -race` and an ubuntu/windows/macos matrix), plus formatter/vet/static checks, TS strict check, Playwright desktop/browser smoke and package tests.
- [ ] **POD-06E:** Test at scale (1k subscriptions/100k cached episodes where feasible), bound memory, query latency, initial refresh storm and disk behavior.
- [ ] **POD-06F:** Documentation `docs/podcasts.md`, troubleshooting, security/privacy and full screenshots; update README/docs index and main roadmap.
- [ ] **POD-06G:** Cross-platform release checklist, versioning/rollback and issue triage; stage rollout behind feature flag until gates clear.

**Exit:** End-to-end acceptance suite green; release candidate proven on supported desktop and local-Go browser platforms with no known P0/P1 integrity, SSRF, offline or accessibility blockers.

### Phase 7 — Advanced listening DSP (research gate)

- [ ] **POD-07A:** Prototype speech-tailored loudness/voice boost, compare bypass vs DSP on actual podcast samples.
- [ ] **POD-07B:** Silence trim at constant perceived pitch, avoid jump artifacts, preserve timestamp/chapters/scrubbing semantics.
- [ ] **POD-07C:** Compare compute/battery/CPU in Chromium, WebKit/Wails and Linux; disable unsupported paths transparently.
- [ ] **POD-07D:** Per-show preferences and accessibility/quality controls; no degradation to music DSP pipeline.

### Phase 8 — Interoperability and expansion (separately scoped)

- [ ] **POD-08A:** Optional gPodder-compatible sync with conflict and account deletion policy; *not* the local source of truth.
- [ ] **POD-08B:** Podcasting 2.0 advanced metadata, related links, optional live streams, episode person/funding tags.
- [ ] **POD-08C:** Video/HLS compatibility and permissions; offline HLS only with explicitly legal/technically supportable source.
- [ ] **POD-08D:** Optional local transcript generation/search/smart episode recommendations with data/CPU consent.
- [ ] **POD-08E:** Cross-device sync/provider integrations only with explicit credentials, encryption and conflict rules.
- [ ] **POD-08F:** Sharing/clipping, optional safe remote device playback, rich statistics, no covert telemetry.

## 8. Recommended initial PR breakdown

| PR | Depends on | Scope | Verify before merge |
|---|---|---|---|
| PR-A | Phase 0 ADR | Schema + fixtures + migration tests | Existing users' music DB not changed destructively |
| PR-B | PR-A | Remote URL fetch security + parser + feed reconciliation | SSRF, redirects, XML bombs, GUID churn |
| PR-C | PR-B | Feed CRUD/refresh REST + typed client | 304/error/startup/pagination tests |
| PR-D | PR-C | Podcast UI shell and show pages | Browser, keyboard, responsive tests |
| PR-E | PR-C | Directory adapter & Discover | Key redaction, no-key fallback, provider 429 |
| PR-F | PR-C | Media resolver and streaming range handler | 200/206/416, cancel, offline, content-type |
| PR-G | PR-D/F | Podcast player + position + music arbitration | No double audio; restart/resume and DJ checks |
| PR-H | PR-A/F | Persistent download worker + atomic files | Crash/retry/quota/corrupt/host-security tests |
| PR-I | PR-G/H | Downloads UI and true offline playback | Disconnected restart E2E |
| PR-J | PR-C | OPML export (MVP, POD-03F) and import (POD-04E) | Round-trip, duplicates, partial failure, no secret export |
| PR-K | PR-G | Chapters/transcripts/bookmarks + advanced controls | Timed cues, sanitized content, accessibility |
| PR-L | All MVP | Docs/telemetry redaction/cross-platform gate | Full CI and release smoke |
| PR-M | PR-H/I | Post-MVP auto-download policies, quotas and retention (POD-04A/04C) | Limits respected, no unintended delete, pinned/current protection |

Branch strategy: short-lived `feature/podcasts-*` work branches from updated `main`; do not build all phases in a single stacked mega-PR. Rebase or merge current main at controlled intervals; gate each PR with automated tests. Keep this Markdown plan updated with PR links, completion states and decisions.

## 9. Acceptance criteria and test matrix

### Deterministic fixtures

- Valid public RSS; Apple/iTunes tags; chapters JSON; VTT/SRT transcript; explicit feeds; missing cover/duration; UTF-8 edge cases.
- `guid` absent, duplicated and changed; feed title and URL redirects; out-of-order dates; truncated back catalog; duplicate enclosures.
- 304/ETag/Last-Modified; 301/302/307; 401/403; 404/410; 429 with Retry-After; 500/timeout; malformed XML and response compression bombs.
- HTTP 200, 206 single range, 416, unknown length/chunked, ETag changed mid-transfer, disconnected midway, wrong declared size, .part file left after abrupt exit.
- Enclosure URL to private IP/localhost/IPv6, public→private redirect, DNS rebinding, image/transcript secondary fetch and path traversal/symlink exploit.
- OPML nested outlines/duplicates/large import; private token in URL; malformed XML; non-feed HTML page; multiple namespaces.
- Real safe small audio fixtures for MP3/M4A/Ogg/Opus as actually supported per desktop WebView; use the canonical capability matrix (`backend/internal/audio/media-capabilities.json` via `services/mediaCapabilities.ts`) and [format support gaps](AUDIO_FORMAT_SUPPORT_GAPS.md) to avoid unsupported claims.

### Test ownership

- **Go unit:** parser, normalization, secure network dial/redirect policy, filename path sanitation, bytes validation, HTTP response/range forwarding, downloaded-file reconciliation, SQL state transitions.
- **Go integration:** local HTTP fixture server with instrumented requests, SQLite test database, concurrent worker cancellation/restart, 304, redirect/SSRF, schema migration, crash recovery, race detector.
- **TypeScript/Vitest:** API type/error mapping, player coordinator, position serialization/stale-write handling, queue behavior, filtering, service fallback and redacted status.
- **Playwright:** search → follow → stream → seek → restart → download → disconnect → local play; music↔podcast switching, per-show controls, OPML and accessible controls.
- **CI smoke:** Windows/macOS/Linux native + local-Go browser distribution, WebKit audio capability, OS media control fallback; ensure binaries include required DB/schema resources.

### Release SLO targets (proposed; instrument and validate before claiming)

| Measure | Initial gate |
|---|---|
| Feed refresh integrity | Zero duplicate episodes for repeated identical feeds and redirect/304 fixtures |
| Playback resume | Within 2 seconds of saved position after normal pause/reopen in test fixtures |
| Offline durability | 100% of verified completed fixture downloads playable after Go restart and disconnected network |
| Download integrity | Zero partially written files marked completed; zero temp-file path escape |
| Player exclusivity | Zero concurrent podcast+music output on tested transition paths |
| API privacy | Zero directory keys, private feed tokens or upstream bearer headers in frontend/storage/diagnostics |
| Remote fetch trust | All known localhost/private/rebinding/redirect SSRF fixtures blocked by default |
| Resource bounds | Feed/XML/chapters/transcripts/download quotas respected under adversarial tests |
| Accessibility | Keyboard and screen reader basic listening/subscription actions pass documented audits |

## 10. Risks, tradeoffs and open decisions

| Risk / decision | Why it matters | Recommended path |
|---|---|---|
| Podcast Index API keys and terms | Public desktop binaries cannot conceal a shared secret | Permit end-user keys, plan permitted deployment and secure storage, direct RSS as unconditional fallback |
| RSS feed URL SSRF | User-supplied feeds can access privileged loopback service | Public-IP-only connector policy, redirect/dial validation, exhaustive tests; private LAN opt-in only if separately threat-modeled |
| Mixed playback state | Music `Song`/crossfade/queue is optimized for music | Separate podcast transport behind a small explicit audio-session coordinator |
| "Offline" in a browser | Static web cannot use Go filesystem safely | Support only local-Go browser release; state backend dependency clearly |
| Feed identity | CDN/GUID/feed URL churn produces duplicates | Feed aliases + scoped episode identity + reconciliation transaction |
| Codec variation | Native WebViews have platform-specific codec support | Capability probes, explicit error/format policy; do not claim unsupported Opus/HLS across all systems |
| Resume correctness | Multi-hour episodes amplify dropped progress writes | Backend-authoritative commits, periodic checkpoint + lifecycle flush, stale-revision protection |
| License/private content | Some feeds are paid and may be access-controlled | Legitimate credentials only, content access policy and user-visible errors, no extraction from Spotify/Apple |
| Large libraries | 1k feeds and 100k episode rows can saturate SQLite/network | Pagination/indexes, refresh jitter/worker caps, retention and explicit scale benchmarks |
| Storage/backup semantics | Backups and uninstall may leak private URLs or orphan large files | Export schema/policy, encrypted credentials, separate user-media backup behavior, cleanup/relink tests |
| Auto-cleanup vs trust | Silent deletion harms user confidence | Dry-run counts, pinned/current/unplayed protection, grace period and recovery documentation |
| DSP complexity | Silence trim and enhanced speech can distort timing or audio | Post-v1 isolated DSP gated by measurements and per-show bypass |

**Decisions to resolve in Phase 0 (with suggested defaults):**

1. **Discover key model:** user-supplied Podcast Index credentials or approved permitted backend arrangement; default direct RSS if none.
2. **Private RSS feeds:** defer to P2 until secrets and signed-URL privacy design is proven; public RSS is full v1.
3. **Unsupported formats:** show explicit unavailable/unsupported codec rather than transparently transcoding without a qualified backend.
4. **Download destination:** default `<dataDir>/podcast_downloads` (mirrors the existing `<dataDir>/spotify_downloads` default and its `spotify_download_path` setting override), optional managed custom folder via a `podcast_download_path` setting; require space validation and path safeguards.
5. **Listen completion:** consider end-of-media or threshold near final 2–5%; configurable and tested, never simply "started".
6. **History on unsubscribe:** prompt Keep History & Downloads vs Remove All; no implicit destructive cascade.
7. **Podcast queue isolation:** yes for v1; one audio coordinator, two independent queues.
8. **RSS HTTP scheme:** allow HTTPS by default; legacy HTTP only under explicitly documented restricted policy and no automatic credential downgrade. Plain-HTTP enclosures are still common in public feeds, so measure how often the fixture corpus and popular feeds need HTTP before fixing the MVP default, so that DISC-02 does not reject ordinary feeds.
9. **Scheduler ownership:** use the existing durable operation scheduler for refresh only after proving allowlist, restart (`interrupted`) and worker-contention behavior, because it runs only 1–2 workers shared with scans and analysis. A dedicated, low-cost refresh ticker may be simpler. Use independent transfer leases/state for media bytes.
10. **UI location:** primary Podcast hub in sidebar; optionally expose a Podcasts filter in consolidated Downloads, but keep download-source types clearly distinct.

## 11. Definition of done for each implementation PR

- [ ] Code matches module/contract boundaries and includes Go/TS types with no silent `any`/JSON passthrough for public routes.
- [ ] Unit/integration tests cover successful and unsafe/failing paths in the same PR.
- [ ] No regression in music, DJ, Plex or Spotify player/download behavior.
- [ ] New remote URL surface covered by SSRF/TLS/redirect/request-size threat model.
- [ ] DB changes are additive, safe under restart, and tested against old schema snapshots.
- [ ] Loading/error/offline/accessibility states and user-facing copy present.
- [ ] Credentials/private feed URLs never appear in renderer storage, logs, SSE or support exports.
- [ ] Typecheck, Vitest, Go tests/race where applicable, Playwright, builds and release checks green.
- [ ] Current roadmap checkbox/status/PR link updated; docs/OpenAPI updated for user-visible behavior.

## 12. Official references and additional research

**Commercial product references**
- [Pocket Casts player](https://pocketcasts.com/podcast-player) and [App Store feature description](https://apps.apple.com/us/app/pocket-casts-podcast-player/id414834813)
- [Overcast App Store](https://apps.apple.com/us/app/overcast-podcast-app/id888422857)
- [Apple Podcasts: 2026 video, chapters, transcripts and dialogue](https://www.apple.com/newsroom/2026/02/apple-introduces-a-new-video-podcast-experience-on-apple-podcasts/)
- [Podcast Addict overview](https://podcastaddict.com/intro.php), [custom per-show settings](https://podcastaddict.com/faq/340), [download behavior](https://podcastaddict.com/getting_started)
- [Castro App Store](https://apps.apple.com/us/app/castro-podcast-app-player/id1080840241)
- [Spotify podcasts support](https://support.spotify.com/ws/article/podcasts-and-shows/)

**Open-source references**
- [AntennaPod project](https://github.com/AntennaPod/AntennaPod), [getting started](https://antennapod.org/documentation/getting-started)
- [gPodder project](https://github.com/gpodder/gpodder), [user manual](https://gpodder.github.io/docs/user-manual)
- [Audiobookshelf project](https://github.com/advplyr/audiobookshelf), [official documentation](https://audiobookshelf.org/docs/documentation/introduction/)
- [Podverse project](https://github.com/podverse/podverse), [capabilities](https://podverse.fm/about)

**Directory and publisher standards**
- [Podcast Index API source/documentation](https://github.com/Podcastindex-org/docs-api), [API terms](https://api.podcastindex.org/tos_v1.1.pdf), [official example code](https://github.com/Podcastindex-org/example-code)
- [Apple iTunes Search API](https://developer.apple.com/library/archive/documentation/AudioVideo/Conceptual/iTuneSearchAPI/) — provider terms require review
- [Podcasting 2.0 namespace](https://podcasting2.org/docs/podcast-namespace/1.0), [chapters](https://podcasting2.org/docs/podcast-namespace/tags/chapters), [transcript](https://podcasting2.org/docs/podcast-namespace/tags/transcript), [feed GUID](https://podcasting2.org/docs/podcast-namespace/tags/guid)

**ViiB source anchors**
- [Architecture and media model](architecture.md), [Player](player.md), [Downloads](downloads.md), [Development](development.md), [Support bundles](support-bundles.md), [Product roadmap](ROADMAP.md), [format capability gaps](AUDIO_FORMAT_SUPPORT_GAPS.md)
- Code: [server.go](../backend/internal/server/server.go) (route mounting, trust boundary), [v2_jobs.go](../backend/internal/api/v2_jobs.go), [mediafetch.go](../backend/internal/mediafetch/mediafetch.go), [media_routes.go](../backend/internal/api/media_routes.go), [useMediaSession.ts](../hooks/useMediaSession.ts), [mediaCapabilities.ts](../services/mediaCapabilities.ts)

## 13. Review log

**2026-11-22 accuracy review.** All file links in §2 resolve. Corrections made in this revision:

- Pinned stack versions from `package.json`/`backend/go.mod`; noted that `docs/ROADMAP.md` does not yet reference this plan.
- §2: v2 routes are mounted in `backend/internal/server/server.go`, not `api.go`; `/api/audio/*` is dispatched there (local → `media_routes.go`/`serveAudio` with `http.ServeFile`, Plex → `ServeAudioSourceAware`); Host/Origin enforcement is `loopbackTrustBoundary`; `Song` lives in root `types.ts`; `hooks/useMediaSession.ts` already exists; migrations use `Ensure…Schema()` with no version table; recorded the scheduler's allowlist, worker cap and restart semantics; added backup (`VACUUM INTO`), support-bundle, crypto and media-capability anchors.
- Sidebar IA updated to the actual navigation items.
- DISC-08 split so OPML export is P0, consistent with the MVP definition and POD-03F.
- §6.4: documented `mediafetch` IP-blocklist gaps (CGNAT, benchmark, reserved, NAT64/IPv4-mapped).
- PR table: auto-download/retention moved out of PR-J (it depends on the download worker) into new PR-M.
- POD-00G/06B/06D reworded to reflect that no feature-flag framework exists, while support bundles and race-enabled multi-OS CI already do.
- Podcast Index endpoints given their `/api/1.0/` prefix and auth header scheme; Apple row updated to reflect the HLS-based 2026 video launch.

---

**Current next task:** Qualify and stabilize the latest podcast refresh implementation on draft PR #167; then complete feed aliases, stronger publisher compatibility, image/enclosure security tests, and remaining Phase 1 API behaviors. The streaming player and offline downloads have not yet landed.


## 14. Implementation progress log

> **Current checkpoint: 2026-11-22, navigation milestone.** Work is being committed directly to `feature/podcast-client-roadmap-2026-11-22`. Scope completed is explicitly distinguished from unverified tests and future work.

| Item | State | Implementation evidence | Outstanding |
|---|---|---|---|
| POD-00A | **ADR documented** | [ADR 0001](adr/0001-podcasts-local-first-boundaries.md) | Player coordinator is a future delivery; ADR does not claim it exists |
| POD-00B/C/E/F | **Open** | ADR lists defaults and unresolved decisions | Directory credentials, legal review, listening lifecycle, UI design |
| POD-00G | **In progress** | Permanent Podcasts nav/routes; refresh-only opt-in in Settings and Podcast Show; no gated navigation | E2E and full CI qualification |
| POD-00D | **In progress** | `backend/internal/podcast/testdata/basic.xml`, parser/fetch tests | More fixture formats, ID churn, OPML, media range, integration coverage |
| POD-01A | **In progress** | Podcast schema, snapshot/upsert, revision-checked `podcast_progress.go`, persistence/conflict tests | Public/player integration, migrations, backup/restore and CI |
| POD-01B | **In progress** | `backend/internal/podcast/feed.go`, `fetch.go`, unit tests | Expanded SSRF and RSS-format qualification, CI |
| POD-01C | **In progress** | Refresh worker, alias collision guards on subscribe/refresh, conservative GUID reconciliation and history-preservation tests | Real publisher/scale qualification and full CI |
| POD-01D | **In progress** | Podcast preview/follow/list/episodes/unfollow/refresh REST; bounded episode offset pagination, TS client + tests | Show details, cursor pagination, OpenAPI, full CI |
| POD-01E | **In progress** | Permanent Sidebar, followed/show + Load more, local library filters, SSRF-guarded artwork proxy/UI with tests | Full CI, wider publisher compatibility, UX refinement |
| Phase 2–8 | **Not started** | None | Discovery, player, offline downloads and remaining features |

**Commits:** `abde2f2` introduces the foundational ADR, RSS and URL-fetch primitives.
Commit `b44e89a` adds SQLite persistence and the first status log. API contract work is tracked by draft [PR #167](https://github.com/ajbergh/ViiB-MediaHub/pull/167).

**Validation status:** Podcast-specific Go/Vitest/Playwright tests have been added. Focused CI identified a SQL literal bug and a nondeterministic refresh-collision test fixture; both were fixed. The [podcast smoke job for commit `4427c45`](https://github.com/ajbergh/ViiB-MediaHub/actions/runs/45049685563) **passed**, including podcast Go parser/DB/API tests and URL-policy vet checks. That run's full backend and browser jobs were still pending at this checkpoint. A targeted `podcast-slice` CI job now runs `TestPodcast*` plus the RSS parser and URL-security tests in the parser, DB and API packages alongside the original full-suite jobs. The current HEAD is **not yet confirmed green**. GitHub Actions is authoritative for the required Go 1.26 toolchain. A prior backend run failed due to an unused import in the refresh worker; that import has been removed on the current branch. Full CI remains unverified. Do not mark tests green without a GitHub Actions run.

**Next vertical delivery:** Verify artwork proxy Go/Playwright and full CI, expand RSS compatibility, then address GUID changes; playback and downloads remain separately gated. The cross-project `docs/ROADMAP.md` links this plan as an unmerged domain roadmap, not a shipping feature. Keep playback and offline work explicitly later. Music playback remains untouched.
