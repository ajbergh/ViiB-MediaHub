# Home

![Home page](../assets/screenshots/home.png)

The Home page is ViiB MediaHub's landing screen. It summarizes the canonical ViiB music catalog and listening history, so local filesystem music and synchronized Plex music can appear together in the same shelves, Smart Mixes, recent activity, and statistics.

---

## Layout Options

The Home page supports multiple layouts configured under [Settings](settings.md) → **Personalization**.

| Layout | Best for | Typical emphasis |
|---|---|---|
| **Music Shelves** | Balanced browsing | Search, spotlight, albums, artists, Smart Mixes |
| **Cover Wall** | Artwork-forward browsing | Album-art mosaic with search and shelves |
| **Compact Dashboard** | Dense navigation | Recent activity, albums, artists, quick metrics |

---

## Sections

### Search

The Home search bar opens the full [Search](search.md) experience. Indexed search includes local and synchronized Plex catalog tracks; Spotify catalog search remains on the Spotify page.

### Spotlight and Cover Wall

These surfaces use ViiB catalog metadata and artwork. For Plex-backed albums, authenticated artwork is delivered through the ViiB backend rather than exposing Plex credentials to the renderer.

### Smart Mixes

Smart Mixes are derived from the ViiB catalog and listening/enrichment metadata. Plex tracks can participate after synchronization just like local catalog tracks.

See [Smart Playlists / AI DJ](smart-playlists.md).

### Library Snapshot

Catalog counters such as Songs, Albums, and Artists represent the unified ViiB catalog, not only local filesystem media.

### Recently Played

Recently Played is driven by ViiB play history. Plays of normal local/Plex catalog songs participate in the same history and statistics pipeline.

### Recently Added

Recently Added reflects catalog metadata. Local tracks enter through filesystem scanning; Plex tracks enter through successful PMS synchronization and use the available Plex added-date metadata where mapped.

A Plex outage does not remove already synchronized entries from the Home catalog surfaces.

---

## Keeping Home current

- For local folders, run or allow the normal ViiB scanner to reconcile filesystem changes.
- For Plex, use **Resynchronize** in [Library Operations](library-operations.md) to import new/changed/removed PMS music metadata.
- Temporary Plex connectivity/authentication failures retain the cached catalog until a later successful authoritative sync.
- Spotify streaming/browse content is not automatically folded into the ViiB catalog unless it becomes local downloaded media through the supported download workflow.
