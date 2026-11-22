# Podcast Preview API (work-in-progress)

This documents **only currently implemented endpoints** in
[`backend/internal/api/podcast_routes.go`](../backend/internal/api/podcast_routes.go).
The full prospective API is tracked in the
[Podcast implementation roadmap](PODCAST_CLIENT_IMPLEMENTATION_ROADMAP.md).
No Podcast Index search, episode media streaming, downloads, refresh scheduler,
OPML, paid feeds or private RSS credential support is implemented yet.

The endpoints mount at `/api/v2/podcasts`, protected by ViiB's existing
local Host/Origin/CSRF middleware. JSON errors use the standard V2
`{ "error": { "code": "...", "message": "...", "retryable": false, ... } }`
shape with a diagnostic token. Calls are intentionally scoped to public HTTPS
RSS 2.0 feeds during initial implementation.

| Method | Path | Behavior |
|---|---|---|
| GET | `/feeds` | List followed show records with episode counts |
| POST | `/feeds/preview` | Validate and retrieve one public HTTPS RSS feed; return first ten episodes and total |
| POST | `/feeds` | Fetch/feed parse + subscribe + transactional episode snapshot; returns ID/title/count |
| GET | `/feeds/{id}/episodes` | List up to 100 episodes in an existing followed show, newest-first |
| DELETE | `/feeds/{id}` | Unfollow but retain cached episode metadata and progress |

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
- Long-running subscribe/preview requests are currently synchronous; a
  durable background refresh job is planned, not present.
- No custom ports, credentials, signed/token query parameters, or private
  feed access in this preview. These require a separate security design.
- Publisher enclosure URLs are catalog metadata only and **not** an
  implemented streaming or download endpoint.
- The UI has a separate opt-in `podcasts_preview_enabled` backend setting
  under **Settings → Integrations & Spotify**. Disabled by default.
- Existing song, Spotify, DJ and music download APIs are untouched.

**Testing:** Go parser/fetch/repository/handler tests, TypeScript client tests
and synthetic browser Playwright tests are included in the feature branch.
CI qualification must pass before this preview is marked ready.
