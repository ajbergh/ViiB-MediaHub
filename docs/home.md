# Home

![Home page](../assets/screenshots/home.png)

The Home page is the landing screen that appears when ViiB MediaHub starts. It gives you an at-a-glance view of your library and listening history so you can jump straight into music.

---

## Layout Options

The Home page has three selectable layouts. Change the active layout in [Settings](settings.md) → **Personalization**.

| Layout | Best for | What appears first |
|---|---|---|
| **Music Shelves** | Balanced browsing | Search, spotlight, quick tiles, albums, artists |
| **Cover Wall** | Artwork-forward browsing | Album-art mosaic hero with search and shelves below |
| **Compact Dashboard** | Dense library navigation | Search, jump-back-in tiles, top albums, top artists |

---

## Sections

### Search Bar
- Available in every Home layout.
- Pressing **Enter** while focused navigates to the [Search](search.md) page with your query pre-filled.
- The arrow button opens the Search page for mouse/touch users.
- The full [Search](search.md) page supports result tabs and filtering.

### Spotlight
- Appears in the **Music Shelves** layout.
- Features a recent album, top artist, or Smart Mix depending on available library data.
- The card opens detail view; the Play button starts playback without navigating.

### Cover Wall
- Appears in the **Cover Wall** layout.
- Uses real album artwork when available, with gradient fallbacks for missing covers.
- Falls back to the standard spotlight treatment if the library does not have enough album artwork.

### Jump Back In
- Appears most prominently in the **Compact Dashboard** layout.
- Shows quick tiles from recent playback or recent albums.

### Smart Mixes
- Horizontally scrollable row of all available Smart Mixes.
- Each card shows the mix name, track count, and a color gradient.
- Clicking a card opens [Smart Mix Detail](smart-playlists.md); the Play button starts playback.
- Hidden when `showSmartMixes` is disabled in Settings.

### Library Snapshot
- **Total Songs** — total tracks in the library.
- **Albums** — total distinct albums.
- **Artists** — total distinct artists.
- Each compact card is clickable and navigates to the corresponding library section.

### Recently Played
- Recent tracks played, sorted by most recent first.
- Each row shows album art, title, artist, and a relative timestamp (e.g., "2h ago", "Yesterday").
- Click a row to play the track.

### Recently Added Albums
- Newest albums added to the library.
- Useful for seeing what a recent scan picked up.

### Top Artists
- Artists ranked by library depth or listening activity, depending on the active layout.
- Click an artist to open their [Artist Detail](artists.md) page.

---

## Tips

- Smart Mixes are generated automatically when the library has enough tracks. Run a library scan if the carousel is empty.
- The search bar on the Home page is a shortcut — the full [Search](search.md) page supports category filtering.
- Recently Played only shows tracks that have been played through the player at least once. Plays are recorded in SQLite.
