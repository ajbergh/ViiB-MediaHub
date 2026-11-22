# ADR 0002 — Strict bounded publisher audio streaming for Podcasts

- **Date:** 2026-11-22
- **Status:** Accepted for experimental media endpoint; full publisher and platform qualification pending
- **Scope:** POD-02C, partial POD-02D/E
- **Depends on:** [ADR 0001](0001-podcasts-local-first-boundaries.md)

## Decision

Podcast audio is not a music song. The app resolves a selected episode's
enclosure only from the local podcast SQLite collection and only while the
show is actively followed. Browsers and Wails receive an API URL keyed by
the episode ID; they never receive an upstream publisher URL to stream.

The Go API retrieves small byte-range segments over the existing public
HTTPS DNS-pinned podcast connector. Each remote request is limited to a
single range capped at 2 MiB and requires an actual 206 Partial Content
response. Validation checks the publisher's Content-Range (start, end,
total), audio content type, byte count, and disabled compression. Invalid
publishers cannot make the backend buffer or forward arbitrary whole files
and cannot supply HTML/SVG. Every redirect repeats URL and public-IP policy.

Enclosure URL rules are intentionally conservative: no credentials, unknown
query params or signed URLs; no private/local DNS, nonstandard ports or
plain HTTP. This sacrifices compatibility with certain podcast networks
until the credentials, retention and redaction policy is designed. It is
safer to show an unsupported source than weaken SSRF protection.

The early player is an isolated HTMLAudio instance inside show details.
Starting a podcast pauses the music transport; resuming the music transport
pauses the inline podcast. Listening state is persisted independently with
the existing optimistic-revision API and throttled writes. The music song,
DJ and Spotify playlists/states are not rewritten.

## Failure policy

- 404: unknown or unfollowed episode.
- 416: invalid/unsupported single-range request.
- 422: unsupported publisher URL, type or media envelope.
- 502: publisher failed, timed out or returned an invalid media response.
- Never reflect source addresses, upstream response bodies or credentials.

## Known limitations / acceptance gates

A true global media-session owner is still needed: OS/media keys and
Windows SMTC continue to belong to the existing music player in this
early slice. Navigating away from show details stops the inline podcast,
and no downloadable local media source is available. Streaming behavior
against representative real publishers, seek correctness across 2 MiB
ranges, CDN redirects, platform decoders, background playback and
failure/retry UX must be tested before general release.

Audio and subtitle/video compatibility is deliberately not inferred from
RSS metadata. Podcast enclosures must be independently validated at fetch.
The implementation is documented in [Podcast API](../podcast-api.md) and
[roadmap](../PODCAST_CLIENT_IMPLEMENTATION_ROADMAP.md).
