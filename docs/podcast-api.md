# Podcast Preview API (work-in-progress)

This documents **only currently implemented endpoints** in
[`backend/internal/api/podcast_routes.go`](../backend/internal/api/podcast_routes.go).
The full prospective API is tracked in the
[Podcast implementation roadmap](PODCAST_CLIENT_IMPLEMENTATION_ROADMAP.md).
Podcast Index directory search/trending (via build-time `-ldflags` credentials
or optional encrypted user overrides), rich episode detail (chapters,
transcripts, bookmarks, sanitized show notes), cross-show triage/search, a
durable podcast queue, per-show download retention, live SSE events, and
206-range / bounded 200-only offline downloads are implemented on the draft
feature branch, pending multi-OS native qualification. Private/authenticated
RSS feeds remain deferred. Public HTTP(S) audio streams through the shared
music/Plex player via a local episode-ID route. OPML 2.0 subscription export and
safe import preview are available; neither exports nor imports listening
history.

The endpoints mount at `/api/v2/podcasts`, protected by ViiB's existing
local Host/Origin/CSRF middleware. JSON errors use the standard V2
`{ "error": { "code": "...", "message": "...", "retryable": false, ... } }`
shape with a diagnostic token. Calls are intentionally scoped to public HTTPS
RSS 2.0 feeds during initial implementation.

| Method | Path | Behavior |
|---|---|---|
| GET | `/events` | Server-Sent Events (`event: podcasts`) streaming `{ downloads, storage, refreshCount }` snapshots with 15 s heartbeat and 5 s fallback |
| GET | `/discover/status` | Read redacted Podcast Index configuration status (`configured`, `provider`, masked `keyHint`, `source`: `user` \| `embedded` \| `none`) |
| PUT | `/discover/credentials` | Strict JSON (`apiKey`, `apiSecret`, 1 KiB cap) stored encrypted at rest (`enc:v2:`); returns redacted status |
| DELETE | `/discover/credentials` | Remove stored Podcast Index credentials; returns redacted status |
| GET | `/search?q=&max=` | Search Podcast Index directory when configured (412 `provider_not_configured` otherwise) |
| GET | `/trending?category=&lang=&max=` | Fetch Podcast Index trending shows with optional category/language filter |
| GET | `/feeds` | List followed show records with episode counts |
| GET | `/feeds/{id}` | Read one followed show by stable ID; 404 when unknown or unfollowed |
| GET | `/episodes?view=&q=&offset=&limit=` | Cross-show episode triage views (`inbox`, `starred`, `in_progress`, `played`, `archived`, `all`) with optional query filter |
| GET | `/episodes/continue?limit=12` | Recently listened, uncompleted episodes on followed shows; bound 1–50, no publisher network requests |
| GET | `/episodes/search?q=&offset=&limit=` | Local cross-show search across subscribed episode titles, descriptions, and show metadata |
| GET | `/episodes/{id}` | Rich episode detail including sanitized HTML show notes, chapter/transcript presence, bookmark count, and download status |
| PATCH | `/episodes/{id}/state` | Update `starred`, `archived`, `isPlayed`, `positionMs`, or `listenedMs` with optional `expectedRevision` CAS |
| GET | `/episodes/{id}/chapters` | Fetch and sanitize Podcasting 2.0 JSON chapters for a subscribed episode |
| GET | `/episodes/{id}/transcript` | Fetch and parse Podcasting 2.0 JSON, WebVTT, SRT, or plain-text transcripts into cues |
| GET | `/episodes/{id}/bookmarks` | List saved timestamped bookmarks for a subscribed episode |
| POST | `/episodes/{id}/bookmarks` | Create a timestamped bookmark (`positionMs`, `note` ≤ 500 chars) |
| DELETE | `/episodes/{id}/bookmarks/{bookmarkId}` | Delete an episode bookmark |
| GET | `/queue` | List the durable SQLite `podcast_queue` in play order |
| POST | `/queue` | Add an episode to the durable queue (`position`: `next` or `end`) |
| PUT | `/queue` | Atomically replace/reorder the durable queue (`episodeIds`) |
| DELETE | `/queue/{episodeId}` | Remove an episode from the durable queue |
| GET | `/episodes/{id}/progress` | Revisioned durable progress; 404 for unknown episodes |
| GET | `/downloads` | Up to 100 recent statuses for followed episodes with safe `feedId`, `feedTitle`, `episodeTitle` metadata for the centralized Downloads view, but no raw publisher URLs or internal paths. Also returns `storage.usedBytes` and `storage.maxBytes` for the 2 GiB aggregate reservation budget |
| GET | `/episodes/{id}/download` | Fetch one subscribed episode's persisted download status; 404 when absent |
| POST | `/episodes/{id}/download` | Queue/retry a public supported audio enclosure; 202 for queued/current/completed record, idempotent; the worker starts independently |
| DELETE | `/episodes/{id}/download` | Cancel queued/in-progress work and return its status (200), or explicitly remove a completed offline file with 204. Filesystem conflicts return 409; episode and listening progress are preserved |
| GET | `/episodes/{id}/media` | Prefer subscription-scoped completed offline files served with local HTTP byte ranges after first-use SHA-256 verification. Bad cached audio returns 410 (`podcast_download_corrupt`/missing) and invalidates the download state. Otherwise proxy validated public 206 publisher ranges (maximum 2 MiB) or bounded streaming 200 fallback. Nonzero seeks on 200-only origins return 416 |

| PUT | `/episodes/{id}/progress` | Save position/listened/played with `expectedRevision` CAS; 409 on stale writes |
| POST | `/import/opml/preview` | Validate raw OPML 2.0 XML (512 KiB, 100 feed outlines, max nesting 24); list public candidates, duplicate/unsupported counts, and already-followed flags without network calls |

| GET | `/export/opml` | Download OPML 2.0 of currently followed public feeds (up to 5,000 subscriptions / 2 MiB); no listening history or private data |
| POST | `/feeds/preview` | Validate and retrieve one public HTTPS RSS feed; return first ten episodes and total |
| POST | `/feeds` | Fetch/feed parse + subscribe + transactional episode snapshot; returns ID/title/count |
| GET | `/feeds/{id}/artwork` | Proxy the followed show's HTTPS public raster artwork; max 2 MiB; PNG/JPEG/WebP/GIF only; no arbitrary URL parameter |
| GET | `/feeds/{id}/episodes?offset=0&limit=50` | Paginate episodes, default 50, maximum 100; response includes `nextOffset` (null on last page) |
| DELETE | `/feeds/{id}` | Atomically unfollow and revoke queued/in-progress podcast download leases; preserve episode metadata/progress. Attempt bounded cleanup of previously completed podcast offline copies, retrying busy files in background |
| GET | `/feeds/{id}/auto-download` | Read the subscribed show's opt-in `enabled` and bounded `newestCount` (1–3); default is disabled; no media URLs or paths returned |
| PUT | `/feeds/{id}/auto-download` | Strict JSON `{ "enabled": boolean, "newestCount": 1..3 }` with no unknown fields, 1 KiB body cap; re-arms future-only new episode selection and rejects unfollowed shows |
| GET | `/feeds/{id}/retention` | Read per-show download retention (`keepLatestCount`: `0` = keep all, `1..50`) |
| PUT | `/feeds/{id}/retention` | Strict JSON `{ "keepLatestCount": 0..50 }` updating retention and pruning older unstarred, inactive completed downloads |
| GET | `/feeds/{id}/playback-settings` | Read the subscribed show's `defaultSpeed` (`0` = global default, or `0.75`–`2`), `skipIntroSeconds` (`0..600`), and `skipOutroSeconds` (`0..600`) |
| PUT | `/feeds/{id}/playback-settings` | Strict JSON `{ "defaultSpeed", "skipIntroSeconds", "skipOutroSeconds" }` with no unknown fields, 1 KiB body cap; rejects unfollowed shows |
| GET | `/settings` | Read global podcast playback defaults `{ "playback": { "defaultSpeed", "skipBackSeconds", "skipForwardSeconds" } }` |
| PUT | `/settings` | Strict JSON updating global podcast `defaultSpeed` (`0.75`–`2`) and skip intervals (`5..120` seconds) |
| GET | `/feeds/{id}/refresh` | Read last check/success, next refresh, bounded failures and error code |
| POST | `/feeds/{id}/refresh` | Queue a durable manual refresh, return 202 immediately; requires Podcast Preview enabled |

Example preview request:

```http
POST /api/v2/podcasts/feeds/preview HTTP/1.1
Content-Type: application/json

{"url":"https://publisher.example/feed.xml"}
```

Example successful response:

```json
{
  "feed": {
    "url": "https://publisher.example/feed.xml",
    "title": "Example Podcast",
    "author": "Publisher",
    "episodes": [{"guid":"episode-01","title":"Episode 01"}]
  },
  "episodeCount": 1
}
```

**Limitations and security**

- Feed preview, follow and progress writes: bounded JSON, unknown properties rejected. OPML import preview accepts XML up to 512 KiB and never contacts publishers.
- Fetch: public HTTPS URLs, up to four redirects, no downgrade to plaintext
  HTTP, pinned public DNS addresses, 8 MiB response cap, 20-second HTTP
  timeout with a 15-second API context deadline.
- RSS: XML without DTDs, RSS 2.0 with core iTunes fields; show descriptions
  are reduced to plain text. Malformed/unsupported feeds return an error.
- Initial subscribe/preview requests remain synchronous. Subscribe stores the publisher's conditional validators and schedules a later routine check instead of immediately downloading the same feed again. Followed shows are
  refreshed by one dedicated worker at a time with 10-second polling, durable
  SQLite leases, ETag/Last-Modified conditional requests, 304 handling,
  bounded exponential retry and history-safe reconciliation. Background
  network access occurs only while Podcast Preview is enabled.
- Episode pages use `offset`/`limit` ordering by publication date and episode ID. Reload from offset 0 after feed refresh, since new releases can shift page boundaries.
- Refresh is at-least-once after failure/restart; concurrent/multi-instance
  workers require additional integration qualification before production.
  Redirected subscription URL aliases are persisted transactionally and reuse existing show IDs on subscribe and scheduled refresh. Canonical URL changes preserve old feed URLs as aliases; conflicting existing canonical ownership rolls back rather than merging histories. Publisher GUID changes are reconciled only with unique matching enclosure URL, exact title, publication timestamp and positive duration; ambiguous changes retain separate IDs to prevent playback-history loss. Unrelated show identity collisions remain unresolved.
- No custom ports or authenticated/tokenized feeds. Public query parameters are
  restricted to `feed=rss`, `feed=rss2`, `feed=podcast`, `format=rss`, or
  `format=xml`; private feeds require a separate encrypted-credential and
  export-redaction design.
- Artwork URLs are filtered at RSS parse, save and refresh; unsafe or tokenized artwork references are not persisted. The proxy resolves only subscribed show IDs, verifies public DNS at connection and each redirect, limits response size and sniffs actual raster content (never serves HTML or SVG). Browser caching is private for up to 15 minutes.
- Publisher enclosure URLs are backend-only and never serialized into public episode JSON. The subscription-scoped `GET /episodes/{id}/media` serves verified completed local files first via HTTP Range, otherwise guarded public 206 ranges or a bounded streaming-200 fallback. Initial download storage exists under a private fixed episode-ID path. **True disconnected restart and all-OS offline acceptance remain open**; a bounded 200-only offline fallback now exists but is not representative-provider qualified.
- **Podcasts** is a permanent Sidebar destination. RSS preview/follow and browsing work without opting in. Background and manual feed refreshing are controlled by an optional `podcasts_preview_enabled` setting on the Podcast Show page or in **Settings → Integrations & Spotify**. Disabled by default.
- Existing song, Spotify, DJ and music download APIs are untouched.

**Testing:** Go parser/fetch/repository/handler tests, TypeScript client tests
and synthetic browser Playwright tests are included in the feature branch.
CI qualification must pass before this preview is marked ready.


## Troubleshooting RSS preview/follow failures

**Expected on this draft feature branch:** supported public HTTPS RSS 2.0 feeds can be previewed and followed without enabling optional automatic refresh. Public podcast audio plays and seeks through the shared ViiB player on the user-tested build. Early-access bounded 206-range and 200-only offline downloads, plus completed-copy deletion, are implemented but not cross-platform qualified; podcast directory search, robust retention, and private authenticated feeds remain separate roadmap milestones.

The previous generic frontend message `The podcast request could not be
completed` meant the HTTP response had no recognized V2 JSON error envelope.
That could be an outdated Go backend route (404/405), unavailable backend
proxy (502/503), blocked local client origin (403), or other HTTP failure.
The current frontend reports the HTTP status and a more actionable explanation;
the presence of the generic message **does not prove an invalid feed**.

Check the failing `POST /api/v2/podcasts/feeds/preview` request using browser
DevTools → Network. Note its HTTP status and response Content-Type. A working
same-origin `GET /api/v2/podcasts/feeds` returns a JSON object with a `feeds`
array, even when empty. A 404 or HTML response instead of JSON strongly
suggests the UI and Go API aren't running matching builds.

For Vite development, `vite.config.ts` defaults the `/api` proxy to
`http://127.0.0.1:8080`; the supported loopback-only `VIIB_API_PROXY_TARGET` override can reuse a running Wails backend on 34115. The Go backend must be built from the **same branch** and listening on the configured target. Running
only `npm run dev` does not start the Go backend. On Windows use
`./scripts/dev.ps1` from the repository root: it now checks that the
running Go process actually serves the podcast route before starting Vite. For a Wails desktop build,
recompile/restart the desktop Go executable; rebuilding the React UI alone
doesn't update the embedded Go routes. `./scripts/dev-wails.ps1` now refreshes
frontend assets on each launch to avoid a stale UI after branch switching.

If the server responds with a structured error code `invalid_feed_url`,
`blocked_remote_target`, `feed_invalid`, or `feed_unavailable`, investigate
the specific public-feed URL and current HTTPS/secret-query/security policy;
don't weaken SSRF protections to hide an HTTP problem. Include the HTTP status
and ViiB diagnostic token (if present) when reporting a failure, not private
feed URLs or credentials.

## Browser UI with a working Wails backend

When RSS import works in Wails but Vite web development returns
`Podcasts API endpoint unavailable (HTTP 404)`, verify the two Go origins.
Wails normally serves at `127.0.0.1:34115`; Vite historically defaulted
to `127.0.0.1:8080`. A stale process on 8080 can serve old routes.

On Windows, keep the working Wails app open and launch:

```powershell
.\scripts\dev.ps1 -UseWailsBackend
```

The script verifies that `/api/v2/podcasts/feeds` serves the expected JSON
and configures Vite to proxy all `/api` requests to the same known-working
Go backend. Alternatively run
`$env:VIIB_API_PROXY_TARGET = "http://127.0.0.1:34115"` followed by
`npm run dev`. Restart Vite whenever the proxy environment variable
changes. Default `dev.ps1` refuses to silently use an already-occupied
port 8080.


### Export followed podcasts

The **Export OPML** button on the Podcasts → Following page downloads
`viib-podcasts.opml` from `GET /api/v2/podcasts/export/opml`. This
contains currently followed public RSS feed URLs and show names only;
it is suitable for transferring subscriptions to another podcast client.
The server validates every URL before producing the document, escapes
publisher names as XML attributes, fails closed on unsupported/private
URLs, and sends `Cache-Control: no-store`.

OPML is **not** a listening-state or download backup; positions, playback
history, offline media and credentials are deliberately omitted. The
planned OPML **import** workflow will offer a preview, duplicate handling
and a partial-failure report in a later roadmap phase.


## Safe OPML import preview and follow

The Podcasts page accepts an OPML 2.0 file and sends it to
`POST /api/v2/podcasts/import/opml/preview`. The Go parser rejects DTDs,
entity declarations, invalid XML, unsupported root/version, oversized
documents and excessive nesting. It returns **only validated public HTTPS
RSS URLs**, never invalid/private input. Nested outlines work, canonical URL
duplicates are counted, and exact already-followed URLs are marked. No
subscriptions are changed during preview.

After confirmation, the web/desktop UI follows remaining eligible feeds
**one at a time through the existing `POST /feeds` endpoint**, with
cancellation and a per-show failure report using safe error codes. The
underlying subscribe operation retains transactional alias resolution.
Already-followed detection during preview matches canonical URLs; another
valid alias may be re-fetched but should not create a duplicate canonical
subscription. This is not a durable background import job and is not
resumable across app restarts.

The OPML import limit is **100 RSS feed outlines per file / 512 KiB**.
Legacy plain-HTTP feeds and token-bearing/private feeds are intentionally
skipped until policy/credential support is designed. The import workflow
does not import podcast listening state or downloaded media.

## Durable episode progress and Continue Listening

`GET /episodes/{id}/progress` returns the episode's revisioned progress;
`PUT` accepts exactly `positionMs`, `listenedMs`, `isPlayed`,
and `expectedRevision` (all mandatory) and rejects an outdated
`expectedRevision` with HTTP 409. Unknown episodes return 404. A manual
played/unplayed action is exposed in show episode cards and never rewinds
a saved position.

`GET /episodes/continue?limit=12` lists saved positions for uncompleted
episodes on currently followed podcasts. The endpoint is local-only and
read-only; the UI links back to the show. The new inline player can
resume an episode after selecting it from that show; Continue Listening
cards do not yet deep-link to or auto-start an individual episode.


## Podcast episode playback through the shared ViiB Player

The show page's **Listen** action sends episodes into the existing app-level
Player/queue. The same audio elements, EQ, output routing, volume, Now Playing,
queue controls, media keys and Windows media-session integration used by
local/Plex music now own podcast audio. The podcast does **not** mount a
separate audio player or register competing Media Session handlers. Podcast
queue entries are transient adapters: episodes are **not** saved as music
songs and are excluded from music play-count/DJ analytics.

`GET /api/v2/podcasts/episodes/{id}/media` resolves the enclosure only
for a currently followed episode. Raw enclosure URLs never enter renderer
JSON or music storage; episode metadata exposes `hasAudio` and the UI
constructs a same-origin media URL from the stable episode ID. Signed
publisher CDN query parameters remain backend-only. Other than the
publisher metadata already stored in local SQLite, no remote credentials
are persisted by the renderer. Subscription/OPML feed URL restrictions are
intentionally **separate** from media enclosure URL validation.

Publisher enclosures can use public HTTP or HTTPS; URL credentials,
private/local/special-use IPs, DNS rebinding, mixed public/private DNS
sets and HTTPS-to-HTTP redirect downgrades remain blocked. Nondefault
ports, invalid schemes, and nonpublic DNS suffixes are refused.
Every remote connection rechecks and pins the public IP, including
after redirects. Upstream auth headers/cookies are not forwarded.

For a publisher supporting byte ranges, the endpoint requests at most
2 MiB and requires genuine HTTP `206 Partial Content`, correct
`Content-Range`/total/length, and a supported audio MIME. Publisher
`application/octet-stream` is accepted only when RSS declares a
supported audio type and obvious HTML/XML is rejected. Successful
responses set `Content-Range`, `Content-Length`, `Accept-Ranges:
bytes`, `Cache-Control: private, no-store` and `nosniff`.

**Compatibility fallback:** Some public hosts ignore Range and
return HTTP 200. For initial byte-zero requests only, the backend
re-fetches without Range and **streams** a verified audio body to the
same player as HTTP 200, using bounded transfer (maximum 1 GiB,
without whole-file memory buffering). The player can listen, pause and
resume while the connection is active. Publishers that cannot seek
return HTTP 416 for requests starting beyond byte zero; random
seeking may be unavailable even though playback works.

The shared player restores the saved podcast position when metadata
loads and records progress in podcast SQLite with optimistic revisions,
roughly every 10 seconds and when paused, switched or completed.
Music listening and DJ metrics remain untouched. Background playback
across Podcasts page navigation is supported through the persistent
Player component, although desktop/Wails acceptance remains pending.

**Still pending:** cross-device sync, reliable seeking on hosts that do
not provide ranges, private/paywalled authentication, expiry handling
for signed CDN URLs, configurable retention, cross-process disk-pressure resilience and advanced integrity repair,
advanced podcast speed/silence processing and representative real-provider
and cross-platform offline acceptance tests.


### Early-access offline disk safety and cleanup (2026-11-22)

The download worker checks the destination volume's available bytes **before each publisher-chunk write** and requires the remaining episode total plus 128 MiB of spare capacity. The probe is fail-closed; status becomes `failed` with `errorCode: "download_disk_space_low"` if disk space is insufficient or the volume is not readable. This limit is separate from the 2 GiB aggregate reservation cap, which continues to use `download_quota_exceeded`. Neither system can prevent another process from consuming disk space after the check.

Unfollowing a show revokes its queued and active transfer leases in the same SQLite transaction as subscription deletion, while preserving episode progress. Completed local files are reclaimed only by the backend using fixed episode-ID names and regular-file checks; an open file on Windows can defer reclamation until a later worker pass. There is no raw file-path delete endpoint or browser storage access.


### In-progress publisher representation validation

The podcast downloader keeps a bounded **strong ETag** (when present) and normalized Last-Modified from the first valid 206 range response. Subsequent ranges send `If-Range` using the same strict public-network fetcher; a changed ETag or Last-Modified returns safe download failure code `download_publisher_changed` rather than publishing mixed content. No validator or weak-only ETag permits basic range downloading, **not** restart resume. For strong-ETag sources the worker now stores private prefix SHA-256, byte count, original enclosure URL fingerprint and staged basename after file sync. On stale lease reclaim it verifies the prefix and resumes with If-Range; invalid staging falls back to zero, and HTTP 416/first resumed changed representation gets exactly one byte-zero restart. The validator and file metadata remain backend-only, never in the public Downloads list or shared music catalog. POD-04B still requires native crash/reopen and real-publisher qualification.


### Bounded 200-only publisher downloads (2026-11-22)

When the first strict 206 request returns `ErrPublisherNoRange`, the private download worker may fetch the enclosure **without Range** through the same guarded public-host transport. Only HTTP 200 with a supported audio MIME (or RSS audio declaration for generic octet stream) and no HTML/XML/SVG sniff signature is accepted. The response is streamed to a private temporary file, not buffered in Go memory, with a strict 512 MiB cap even for unknown `Content-Length`. The worker reserves 512 MiB in SQLite and checks destination free space for 512 MiB plus 128 MiB safety margin before starting. Final actual bytes are reconciled, SHA-256 recorded and publication remains atomic. A two-second SQLite heartbeat renews the lease while streaming; Cancel/Unfollow revokes the lease and cancels the upstream request even when the publisher is stalled. Cancellation cannot publish a revoked lease. After completion the existing shared player's local episode media route serves offline HTTP byte ranges. This does not enable nonzero online seeking against a publisher that ignores range requests. 200-only full-file downloads remain restart-from-zero; persisted validator-gated resume applies only to incomplete 206 transfers and is not yet platform-qualified.


### Durable 206 partial checkpoint protocol (2026-11-22)

For supported strong-ETag 206 publishers, the server fsyncs each non-final chunk and atomically persists only **backend-internal** state (`temp_relative_path`, `bytes_received`, `expected_bytes`, `etag`, `last_modified`, `mime`, `staged_sha256` and `source_sha256`) with the current worker owner/lease and aggregate quota check. Stale-lease claims retain this checkpoint. Before reuse, the worker validates the basename and local file, source URL SHA-256 and staged prefix SHA-256, then copies the verified bytes into a fresh private temporary file to isolate old worker file descriptors. The first continuation request begins at the recovered offset with `If-Range`. A changed first resumed response or HTTP 416 invalidates the checkpoint and retries exactly once from byte zero. Publisher URLs, ETags and local paths never appear in download status JSON. This is early-access recovery and does **not** guarantee native Wails crash/disconnect behavior yet.


### Automatic retry for transient download failures (2026-11-22)

A recoverable network interruption (`download_unavailable`) or interrupted worker context (`download_interrupted`) moves the download to `status: "paused"` with a backend-generated `retryAfter` timestamp in **Unix milliseconds**, visible in subscription-scoped `GET /downloads` and `GET /episodes/{id}/download`. After 15s, 60s and 240s backoff windows, at most three automatic claims can occur. Each newly claimed worker must verify its existing strong-ETag partial checkpoint and fresh ownership before continuing; otherwise it starts at zero. While paused, `expectedBytes` continues to reserve capacity against the 2 GiB podcast quota. A paused job may be canceled with the existing DELETE endpoint, and an explicit POST Retry resets its automatic retry budget and staged checkpoint. Terminal validation, SSRF, publisher-representation change, disk and quota failures are not auto-retried; an exhausted job remains `failed` until manually retried. No user-provided `retryAfter` or filesystem paths are accepted.


### Per-show new-episode auto-download (2026-11-22)

Automatic download is **disabled by default**. A successful subscription-scoped PUT stores an explicit per-show `enabled` and `newestCount` (1–3), with an internal arming time. No historical episodes are scheduled by enabling or editing this setting. Upon a later successful HTTP 200 RSS refresh, the same SQLite transaction that reconciles episodes may enqueue at most the configured count of **newly inserted public-audio episodes**, excluding played items and existing download records. HTTP 304, feed subscribe/preview and unchanged episodes never trigger automatic work. The scheduler refuses to exceed **12 pending jobs globally** or **3 pending jobs per show**; a full queue intentionally skips rather than defers historical backfill. Publisher fetching and offline playback still use the existing bounded download worker and shared music/Plex/podcast player. Unplayed-N, keep-latest retention and Wi-Fi/metered policies are not implemented; native and real-provider qualification remain open.
