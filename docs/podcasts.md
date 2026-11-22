# Podcasts — Early Access

> **Availability:** Implemented on draft branch `feature/podcast-client-roadmap-2026-11-22` in [PR #167](https://github.com/ajbergh/ViiB-MediaHub/pull/167). This guide does **not** imply the podcast client has been merged into `main` or published in a release.
>
> **Acceptance checkpoint (2026-11-22):** The owner confirmed real podcast playback and seeking on commit [`83426a5`](https://github.com/ajbergh/ViiB-MediaHub/commit/83426a5d8378c2e063e37dba99c20238cab8d1e3), with all 17 baseline CI checks green. Later queue, speed/skip, bulk-edit and offline-download additions require their own CI and live-host acceptance.

## What you can do on this branch

- Open **Podcasts** from the Sidebar without connecting Spotify or enabling podcast refresh.
- Follow a supported **public HTTPS RSS 2.0** feed by pasting its URL; preview its title and first episodes before following.
- Browse followed shows, load additional episode pages, search followed show titles/authors and filter the **currently loaded** episode list.
- Stream public audio with **Listen** using the same persistent ViiB music/Plex player, queue, volume, EQ, Now Playing and OS Media Session hooks. Supported publishers provide byte-range seeking; not all CDN origins honor random byte ranges.
- Return to **Podcasts → Continue Listening** to resume an unfinished followed episode. State is stored in podcast-specific SQLite tables, not music listening statistics.
- On podcast episodes, use **Play Next** or **Add to Queue** to insert items into the existing audio queue. When the player is idle, these actions start playback. The existing Queue view can reorder or remove queued items.
- While playing a podcast, use **-15s / +30s** and speed settings **0.75×, 1×, 1.25×, 1.5×, 1.75×, 2×**. Switching to music/Plex restores music's 1× rate. These controls remain inside the shared player, not a separate podcast transport.
- Mark individual episodes **played/unplayed**. On a show, **Mark loaded played / Mark loaded unplayed** requires confirmation, operates only on the loaded pages, preserves saved position/listened time, offers Stop, and reports failures rather than overwriting a concurrent update.
- **Download episode**, **Cancel download** and **Retry download** on supported public-audio episodes. The early download worker persists queue state across restarts and saves verified 206-range audio privately; **Available offline** appears once complete. Subsequent Listen/Resume uses the same player and episode media URL without requesting publisher audio while the verified local file exists. This is new, not yet cross-platform runtime-qualified.
- **Export OPML** for the followed public subscriptions (no listening history). Preview a bounded OPML import and explicitly confirm its feed-by-feed follow attempts; invalid/duplicate/private feeds are skipped and individual failures shown.

## Follow and listen

1. Open **Podcasts** and paste a public HTTPS podcast RSS URL, then choose **Preview feed** and **Follow podcast**.
2. In the show, use **Listen** on an episode with playable audio. Playback appears in ViiB's existing bottom player. Navigating away from the show does not create or destroy a second player.
3. Use normal play/pause/seek and podcast-specific speed or skip controls. A podcast queue is transient; do not assume it survives a full application restart.
4. To resume later, open **Podcasts → Continue Listening** and choose **Resume playback**. Saved progress must exist and the show must still be followed.
5. To plan listening, select **Play Next** or **Add to Queue** from the show/Continue cards. Queue controls are shared with local/Plex audio and will not interrupt a track already playing.

**Progress behavior:** Resume uses optimistic revision checks to prevent a stale client from overwriting another session. A conflict or failed network save should be handled explicitly; listening history can be partial after interrupted playback. Changing played state does not reset the episode's saved position.

## Optional offline downloads (early-access build)

1. On a followed show, choose **Download episode** on a supported audio episode. The background worker downloads one episode at a time in validated 2-MiB or smaller byte ranges (up to **512 MiB per episode**) into ViiB's private podcast storage. The podcast download budget is **2 GiB** across completed files and active reservations, shown near the episode list.
2. Watch the queued/downloading progress next to the episode. You can **Cancel download** while it is queued/in progress or **Retry download** after a cancel/failure. If the publisher doesn't honor byte ranges, the download fails with `download_range_required`; ordinary online streaming may still work.
3. Once the episode says **Available offline**, use the existing Listen/Resume action. The shared player reads a local byte-range response from the **same episode-ID URL**, independent of the publisher connection. Completed audio is subscription-scoped; unfollowing does not expose it through the API.
4. To reclaim space, choose **Remove download** beside an offline episode, then confirm **Remove copy**. The offline file is removed without erasing the episode or its saved playback position; later Listen returns to publisher streaming. If the episode is currently selected for playback, switch to another episode or music first. If the file is still in use on Windows, the API reports an error instead of claiming it was deleted.

**Current limitations:** Downloads deliberately restart from byte zero after a crash, not a validated partial resume. The 2 GiB library budget is not a physical disk-free threshold; low disk space can still fail a transfer. Automatic orphan cleanup/retention, 200-only offline download fallback, durable corruption-repair and full platform qualification are still absent. Avoid relying on this build for guaranteed offline access until a true disconnect/restart test passes on your environment.

## Feed refresh

Following and playback do not require the optional refresh setting. The **Enable feed refresh (early access)** control on each show toggles the existing global `podcasts_preview_enabled` opt-in. When enabled, the dedicated worker can periodically check public followed feeds with conditional GET and retry/backoff; **Refresh feed** requests an asynchronous refresh. Disable the setting to stop automatic publisher polling. A refresh may take time and should preserve existing episode progress under known GUID identities.

## OPML portability

- **Export OPML** downloads `viib-podcasts.opml` of followed public feed URLs and titles only. No playback positions, downloaded audio, credentials or private-feed tokens are exported.
- **Import OPML** accepts a local OPML 2.0 file, at most 512 KiB and 100 feed outlines. Preview checks format and already-followed subscriptions without contacting publishers. Choose Import to subscribe sequentially; cancel and review the partial-failure report as needed.

## Security and limits

Public RSS and artwork retrieval remain strict public-Internet operations. The Go backend validates DNS/IP and redirects, blocks local/private addresses, and restricts media size and MIME. Public audio enclosures may be HTTP(S) and contain CDN-signed query parameters, but those remote URLs stay backend-only; the browser receives only local episode-ID media routes. Do not bypass these restrictions for a problematic publisher.

**Not implemented / not qualified yet:** Podcast Index directory search, authenticated/private RSS, durable curated Up Next/Inbox or starred workflow, chapter/transcript features, advanced retention/disk-free preflight/partial resume, checksum repair after tampering, comprehensive native media-key testing, all-publisher HTTP behavior, and release qualification across Windows/macOS/Linux. Initial offline download and local-range playback exist on the feature branch, **but real network-disconnected restart acceptance remains open**. A 200-only publisher may stream from the beginning but reject nonzero seeking; offline downloading currently requires 206 support.

## Development and troubleshooting

- If RSS works in Wails but the Vite web client returns **HTTP 404**, the browser may be talking to a stale Go API on port 8080. Keep the current Wails backend running and use `./scripts/dev.ps1 -UseWailsBackend` from the branch checkout to validate/use its loopback API on port 34115, or set `VIIB_API_PROXY_TARGET=http://127.0.0.1:34115` and restart Vite.
- A 502–504 from `/api/v2/podcasts/feeds` indicates a backend/proxy issue; a structured feed validation error may mean the publisher is inaccessible or outside the public-feed policy. Capture safe HTTP status and diagnostic token, not signed enclosure URLs.
- On a non-range publisher, Play may succeed while nonzero seeking is unsupported. If playback fails, verify the backend and frontend build are from the same feature commit and inspect redacted backend diagnostics.
- On changes to playback or queue, run `npm run check` and `npm run test:e2e` plus the Go backend/podcast suites. GitHub Actions CI and manual Wails/browser validation remain separate acceptance gates.

## Technical references

- [Podcast API and troubleshooting](podcast-api.md)
- [Living implementation roadmap](PODCAST_CLIENT_IMPLEMENTATION_ROADMAP.md)
- [ADR 0001: local-first boundaries](adr/0001-podcasts-local-first-boundaries.md)
- [ADR 0002: shared player / byte-range streaming](adr/0002-podcast-byte-range-streaming.md)
