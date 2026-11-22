# Podcast Preview API (work-in-progress)

This documents **only currently implemented endpoints** in
[`backend/internal/api/podcast_routes.go`](../backend/internal/api/podcast_routes.go).
The full prospective API is tracked in the
[Podcast implementation roadmap](PODCAST_CLIENT_IMPLEMENTATION_ROADMAP.md).
No Podcast Index search, episode media streaming, downloads,
OPML, paid feeds or private RSS credential support is implemented yet.

The endpoints mount at `/api/v2/podcasts`, protected by ViiB's existing
local Host/Origin/CSRF middleware. JSON errors use the standard V2
`{ "error": { "code": "...", "message": "...", "retryable": false, ... } }`
shape with a diagnostic token. Calls are intentionally scoped to public HTTPS
RSS 2.0 feeds during initial implementation.

| Method | Path | Behavior |
|---|---|---|
| GET | `/feeds` | List followed show records with episode counts |
| GET | `/feeds/{id}` | Read one followed show by stable ID; 404 when unknown or unfollowed |
| GET | `/export/opml` | Download OPML 2.0 of currently followed public feeds (up to 5,000 subscriptions / 2 MiB); no listening history or private data |
| POST | `/feeds/preview` | Validate and retrieve one public HTTPS RSS feed; return first ten episodes and total |
| POST | `/feeds` | Fetch/feed parse + subscribe + transactional episode snapshot; returns ID/title/count |
| GET | `/feeds/{id}/artwork` | Proxy the followed show's HTTPS public raster artwork; max 2 MiB; PNG/JPEG/WebP/GIF only; no arbitrary URL parameter |
| GET | `/feeds/{id}/episodes?offset=0&limit=50` | Paginate episodes, default 50, maximum 100; response includes `nextOffset` (null on last page) |
| DELETE | `/feeds/{id}` | Unfollow but retain cached episode metadata and progress |
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

- Requests: JSON only, maximum 4 KiB, unknown properties rejected.
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
- Publisher enclosure URLs are catalog metadata only and **not** an
  implemented streaming or download endpoint.
- **Podcasts** is a permanent Sidebar destination. RSS preview/follow and browsing work without opting in. Background and manual feed refreshing are controlled by an optional `podcasts_preview_enabled` setting on the Podcast Show page or in **Settings → Integrations & Spotify**. Disabled by default.
- Existing song, Spotify, DJ and music download APIs are untouched.

**Testing:** Go parser/fetch/repository/handler tests, TypeScript client tests
and synthetic browser Playwright tests are included in the feature branch.
CI qualification must pass before this preview is marked ready.


## Troubleshooting RSS preview/follow failures

**Expected today:** supported public HTTPS RSS 2.0 feeds can be previewed and
followed without enabling the optional automatic-refresh setting. Playback,
download and podcast directory search are different roadmap milestones.

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

For Vite development, `vite.config.ts` proxies `/api` to
`http://127.0.0.1:8080`. The Go backend must be built from the **same branch**
and listening on that port (or the Vite proxy must be reconfigured). Running
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
