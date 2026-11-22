# ADR 0002 — Publisher audio through one shared ViiB Player

- **Date:** 2026-11-22
- **Status:** Implemented on draft feature branch; cross-platform acceptance pending
- **Scope:** POD-02C, POD-02D, POD-02E
- **Depends on:** [ADR 0001](0001-podcasts-local-first-boundaries.md)

## Decision

There is exactly **one regular audio player** for local music, Plex,
Spotify and podcasts. Podcast episode rows are mapped to transient
`Song`-compatible queue entries with a distinct `source: podcast` and
an episode ID. The existing `components/Player.tsx`,
`hooks/useAudioPlayer.ts`, volume/EQ, output routing, Now Playing,
queue and `useMediaSession` own the audio and OS controls.
No podcast-only HTMLAudio player exists. Podcast queue entries do not
enter the music SQLite catalog or music listening/AI-DJ statistics.
Crossfades and music preloading are disabled for podcast transitions.

Podcast listening positions and played state remain in podcast-owned
SQLite tables. The audio hook uses a revision-checked progress tracker;
it restores position, counts actual playback rather than seeks, writes
periodically, and flushes on pause/change/end. A stale revision blocks
subsequent writes and surfaces an actionable toast.

## Network and media boundary

The browser sees only a local episode-ID proxy route.
Raw publisher enclosure URLs (which can contain expiring signed tokens)
never enter episode JSON. The backend independently validates public
HTTP/HTTPS destinations, IP/dial-time DNS, redirects, and MIME and strips
publisher credentials/headers. Private networks, rebinds and HTTPS
downgrades are blocked.

When supported, publishers must return valid HTTP 206 and the server
proxies no more than 2 MiB at a time, validating Content-Range and audio
type. A verified 200 response to Range indicates a non-range publisher:
the server re-fetches without Range and forwards the full audio as a
bounded streaming 200 response (maximum 1 GiB) without buffering it in
memory or bypassing SSRF checks. Nonzero random seeks against such
publishers return 416 rather than silently playing from the wrong byte.

Plain HTTP audio is permitted for compatibility only when the host is
public, at the default port, and is not reached by a downgrade from
HTTPS. Source query parameters are retained backend-side to allow signed
CDN enclosures. The feed subscription/OPML export policy remains strictly
separate and does not permit private feed tokens.

## Acceptance gates

Synthetic Go/API/TypeScript and browser tests must pass. Retest actual
public HTTPS/HTTP RSS podcast audio in both Vite and Wails, including
initial play, page navigation, volume, pause/resume, seek where supported,
queue navigation and music/Plex handoff. Cross-platform native codecs
and signed URL expiry behavior have not yet been proven. Downloads and
offline sources remain unimplemented.

See the [Podcast API](../podcast-api.md) and the
[implementation roadmap](../PODCAST_CLIENT_IMPLEMENTATION_ROADMAP.md).
