# Podcasts — Early Access

> **Availability:** Implemented on draft branch `feature/podcast-client-roadmap-2026-11-22` in [PR #167](https://github.com/ajbergh/ViiB-MediaHub/pull/167). This guide does **not** imply the podcast client has been merged into `main` or published in a release.
>
> **Acceptance checkpoint (2026-11-22):** The owner confirmed real podcast playback and seeking on commit [`83426a5`](https://github.com/ajbergh/ViiB-MediaHub/commit/83426a5d8378c2e063e37dba99c20238cab8d1e3), with all 17 baseline CI checks green. Later queue, speed/skip, bulk-edit and offline-download additions require their own CI and live-host acceptance.

## What you can do on this branch

- Open **Podcasts** from the Sidebar without connecting Spotify or enabling podcast refresh.
- **Discover podcasts (`/podcasts/discover`)**: Search and browse trending shows via Podcast Index using your own encrypted API key and secret (configured inline or in **Settings → Integrations & Spotify**), preview RSS episodes before following, or fall back to pasting any public HTTPS RSS URL directly without a directory key.
- Follow a supported **public HTTPS RSS 2.0** feed by pasting its URL; preview its title and first episodes before following.
- Triage across shows in **Podcasts → Episodes** (`Inbox`, `Starred`, `In Progress`, `Played`, `Archived`, `All`) with cross-show local search, Star/Archive/Played controls, and a durable **Podcasts → Queue** tab that can be reordered and played into the shared ViiB player.
- Open any **Episode Detail (`/podcasts/episode/:id`)** page or the **Now Playing → Chapters & Notes** drawer to read sanitized HTML show notes, jump to **Chapters** (Podcasting 2.0 JSON / Podlove), search and click-to-seek **Transcripts** (JSON, VTT, SRT, text), and save timestamped **Bookmarks** (also via the quick-bookmark button in the Player).
- Configure per-show **Download retention** (`Keep all` or `Latest 1 / 3 / 5 / 10` completed episodes) on each show page; starred and currently active episodes are protected from automatic retention pruning.
- Receive live **Downloads** updates over Server-Sent Events (`/api/v2/podcasts/events`) with automatic reconnect and fallback polling, and use the podcast-aware **Sleep Timer** (`End of This Episode` / `By Episodes` / timed fade).
- Browse followed shows, load additional episode pages, search followed show titles/authors and filter the **currently loaded** episode list by title/description, Played, Unplayed, In progress (unplayed with a saved position), or Not started (unplayed with no saved position). The visible count and empty state reflect loaded episodes, not the full archive.
- Stream public audio with **Listen** using the same persistent ViiB music/Plex player, queue, volume, EQ, Now Playing and OS Media Session hooks. Supported publishers provide byte-range seeking; not all CDN origins honor random byte ranges.
- Return to **Podcasts → Continue Listening** to resume an unfinished followed episode. State is stored in podcast-specific SQLite tables, not music listening statistics.
- On podcast episodes, use **Play Next** or **Add to Queue** to insert items into the existing audio queue. When the player is idle, these actions start playback. The existing Queue view can reorder or remove queued items.
- While playing a podcast, use the configurable skip buttons (default **-15s / +30s**, adjustable between 5s and 120s in **Settings → Playback & Audio**) and speed settings **0.75×, 1×, 1.25×, 1.5×, 1.75×, 2×**. You can set a global default podcast speed in Settings and per-show default speed, intro skip (0–600s when starting from 0), and outro skip (0–600s) on each show page. Switching to music/Plex restores music's 1× rate. These controls remain inside the shared player, not a separate podcast transport.
- Mark individual episodes **played/unplayed**. On a show, **Mark loaded played / Mark loaded unplayed** requires confirmation, operates only on the loaded pages, preserves saved position/listened time, offers Stop, and reports failures rather than overwriting a concurrent update.
- **Download episode**, **Cancel download** and **Retry download** on supported public-audio episodes, including a centralized Downloads section on the Podcasts home page. The early download worker persists queue state across restarts and saves verified 206-range or bounded 200-only audio privately; **Available offline** appears once complete. Subsequent Listen/Resume uses the same player and episode media URL without requesting publisher audio while the verified local file exists. This is new, not yet cross-platform runtime-qualified.
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
2. Watch the queued/downloading progress next to the episode, or return to **Podcasts → Downloads** to filter recent downloads by In progress (including scheduled retries), Available offline or Needs attention. You can **Cancel** while queued/in progress, **Retry** after a cancel/failure and **View show** from that section. If the publisher ignores HTTP byte ranges, ViiB tries a bounded full-file download instead. This needs room for the 512 MiB maximum file plus a 128 MiB free-space safety margin, even if the final audio is smaller.
3. Once the episode says **Available offline**, use the existing Listen/Resume action. The shared player reads a local byte-range response from the **same episode-ID URL**, independent of the publisher connection. Completed audio is subscription-scoped; unfollowing does not expose it through the API.
4. To reclaim space, choose **Remove download** beside an offline episode on either the show page or the central Downloads list, then confirm **Remove copy**. The offline file is removed without erasing the episode or its saved playback position; later Listen returns to publisher streaming. If the episode is currently selected for playback, switch to another episode or music first. If the file is still in use on Windows, the API reports an error instead of claiming it was deleted.

**Current limitations:** On publishers supplying a **strong ETag**, interrupted 206 downloads now retain a checked, private partial-file checkpoint. After a crashed worker's lease expires, ViiB can verify the staged bytes and resume with If-Range; missing/tampered staged bytes or changed publisher URLs safely restart at byte zero. HTTP 416 or changed content during the first resumed range permits one clean restart. Publishers without a strong ETag, and 200-only full-file transfers, still restart from zero. Recoverable network failures now schedule up to three automatic retries (15 seconds, 1 minute and 4 minutes) and reuse a strong-ETag checkpoint when possible. The Downloads section shows a retry time, and you can Cancel a scheduled retry. After the retry limit is exhausted, use manual Retry. Unsupported content, unsafe URL errors and storage failures do not auto-retry. The backend checks stored SHA-256 on the first local playback after a restart; damaged files are marked for explicit retry. A two-second download heartbeat renews the worker lease and stops blocked 200-only network requests after Cancel/Unfollow. Limited cleanup removes abandoned temporary files older than four hours and retry-time orphan files. On Windows/Linux/macOS, each download chunk also checks the destination drive has enough room for the remaining file plus **128 MiB free**; the overall podcast reservation budget remains **2 GiB**. These checks reduce, but cannot eliminate, risk from external disk pressure or simultaneous processes. On **Unfollow**, active downloads are canceled and completed offline copies are reclaimed, with retries if a file is busy; listening history remains intact. Configurable retention, cross-process stress testing and full platform qualification remain outstanding. Avoid relying on this build for guaranteed offline access until a true disconnect/restart test passes on your environment.

## Feed refresh

### Automatic downloads for new episodes

On an individual followed show, the **Automatic episode downloads** setting is **off by default**. Enable it and choose whether to save the **newest 1, 2 or 3 episodes** first discovered on each successful future feed refresh. It will **not download existing back-catalog episodes** or change listening history. ViiB caps pending work at 3 per show and 12 across all podcasts; full queues skip newly discovered episodes rather than backfilling them later. The existing private one-at-a-time worker still enforces file size, storage and network safety restrictions. To turn off future automatic downloads, uncheck the setting; already downloaded files remain until you explicitly remove them. This feature requires the separate **Enable feed refresh (early access)** option to be on, and it remains unqualified on real native platforms.

Following and playback do not require the optional refresh setting. The **Enable feed refresh (early access)** control on each show toggles the existing global `podcasts_preview_enabled` opt-in. When enabled, the dedicated worker can periodically check public followed feeds with conditional GET and retry/backoff; **Refresh feed** requests an asynchronous refresh. Disable the setting to stop automatic publisher polling. A refresh may take time and should preserve existing episode progress under known GUID identities.

## OPML portability

- **Export OPML** downloads `viib-podcasts.opml` of followed public feed URLs and titles only. No playback positions, downloaded audio, credentials or private-feed tokens are exported.
- **Import OPML** accepts a local OPML 2.0 file, at most 512 KiB and 100 feed outlines. Preview checks format and already-followed subscriptions without contacting publishers. Choose Import to subscribe sequentially; cancel and review the partial-failure report as needed.

## Security and limits

Public RSS and artwork retrieval remain strict public-Internet operations. The Go backend validates DNS/IP and redirects, blocks local/private addresses, and restricts media size and MIME. Public audio enclosures may be HTTP(S) and contain CDN-signed query parameters, but those remote URLs stay backend-only; the browser receives only local episode-ID media routes. Do not bypass these restrictions for a problematic publisher.

**Not implemented / not qualified yet:** Authenticated/private RSS feeds, AI smart podcast playlists, unplayed-N auto-download rules, live multi-OS Wails/browser release qualification across Windows/macOS/Linux, and real-world disconnected restart acceptance. Initial offline download, local-range playback, Podcast Index discovery, chapters/transcripts/bookmarks, durable queue, triage views, and per-show retention are implemented on the feature branch, **pending full CI and multi-platform runtime qualification**. A 200-only publisher can now be downloaded in a bounded full-file pass, after which local seeking works through the shared player. Live online seeking on an unbuffered 200-only source may still be unavailable.

## Development and troubleshooting

- If RSS works in Wails but the Vite web client returns **HTTP 404**, the browser may be talking to a stale Go API on port 8080. Keep the current Wails backend running and use `./scripts/dev.ps1 -UseWailsBackend` from the branch checkout to validate/use its loopback API on port 34115, or set `VIIB_API_PROXY_TARGET=http://127.0.0.1:34115` and restart Vite.
- A 502–504 from `/api/v2/podcasts/feeds` indicates a backend/proxy issue; a structured feed validation error may mean the publisher is inaccessible or outside the public-feed policy. Capture safe HTTP status and diagnostic token, not signed enclosure URLs.
- A download failure labeled `download_disk_space_low` means the destination drive lacks enough available space for the remaining episode plus its 128 MiB reserve, or the volume could not be checked. Free space before retrying; existing music/Spotify downloads are not removed by podcast cleanup.
- On a non-range publisher, Play may succeed while nonzero seeking is unsupported. If playback fails, verify the backend and frontend build are from the same feature commit and inspect redacted backend diagnostics.
- On changes to playback or queue, run `npm run check` and `npm run test:e2e` plus the Go backend/podcast suites. GitHub Actions CI and manual Wails/browser validation remain separate acceptance gates.

## Technical references

- [Podcast API and troubleshooting](podcast-api.md)
- [Living implementation roadmap](PODCAST_CLIENT_IMPLEMENTATION_ROADMAP.md)
- [ADR 0001: local-first boundaries](adr/0001-podcasts-local-first-boundaries.md)
- [ADR 0002: shared player / byte-range streaming](adr/0002-podcast-byte-range-streaming.md)
- [ADR 0003: podcast directory credentials](adr/0003-podcast-directory-credentials.md)
- [ADR 0004: podcast content & retention policy](adr/0004-podcast-content-retention-policy.md)
