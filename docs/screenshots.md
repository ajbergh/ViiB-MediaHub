# UI screenshots

Current guide and README screenshots live in `assets/screenshots/`. The October 15, 2026 refresh was captured from the live local app at **1440 × 900 CSS pixels**, including all primary routes, seven Settings tabs, Queue, and EQ.

## Refresh the captures

Install dependencies with `npm ci`, start the Go backend and frontend using the [development guide](development.md), then run from the repository root:

```bash
node scripts/take-screenshots.mjs
```

The script uses the repository's Playwright dependency and a fresh Chromium browser context. Install its Chromium binary with `npx playwright install chromium` if needed. It opens `http://localhost:3000/` and overwrites 25 PNGs in `assets/screenshots/`.

Use a library and account state suitable for publication. Review every capture for personal paths, account details, credentials, error messages, loading placeholders, and clipped content before committing. The script visits Settings and opens player panels; it does not populate sample tracks, sign into services, generate playlists, or start playback. Existing backend scans and indexing can appear in status indicators.

Keep the viewport at least 1440 CSS pixels wide to capture the full DJ interface. Capture actual UI state and describe it accurately: a disconnected Spotify screen demonstrates onboarding, while an empty deck demonstrates the workspace layout. Runtime song counts and progress are examples, not product limits.

After capture, run `python scripts/check_documentation_links.py` and `git diff --check`. The link checker does not inspect screenshot contents or Markdown-generated anchors, so review images and navigation separately.
