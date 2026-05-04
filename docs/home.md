# Home

![Home page](../assets/screenshots/home.png)

The Home page is the landing screen that appears when ViiB MediaHub starts. It gives you an at-a-glance view of your library and listening history so you can jump straight into music.

---

## Layout

```
┌────────────────────────────────────────────────────────────┐
│  Search bar                                                │
├──────────────────────────┬─────────────────────────────────┤
│  Featured Mix (hero card)│  Smart Mixes carousel ──────► │
├──────────────────────────┴─────────────────────────────────┤
│  Stats: Total Songs · Albums · Artists                     │
├────────────────────────────────────────────────────────────┤
│  Recently Played (last 20 tracks with relative timestamps) │
├────────────────────────────────────────────────────────────┤
│  Recently Added (newest library additions)                 │
├────────────────────────────────────────────────────────────┤
│  Top Artists (most listened)                               │
└────────────────────────────────────────────────────────────┘
```

---

## Sections

### Search Bar
- Pressing **Enter** while the search bar is focused navigates to the [Search](search.md) page with your query pre-filled.
- The search bar is a quick entry point — full search is on the dedicated Search page.

### Featured Mix
- Displays the first available Smart Mix as a large hero card with a gradient background derived from the mix's metadata.
- Clicking the card plays the Smart Mix immediately or navigates to its detail view.
- Hidden when no Smart Mixes have been generated.

### Smart Mixes Carousel
- Horizontally scrollable row of all available Smart Mixes.
- Left/right arrows appear on hover to scroll the carousel.
- Each card shows the mix name, track count, and a color gradient.
- Clicking a card plays it or opens [Smart Mix Detail](smart-playlists.md).
- The carousel is hidden when `showSmartMixes` is disabled in settings.

### Stats Cards
- **Total Songs** — total tracks in the library.
- **Albums** — total distinct albums.
- **Artists** — total distinct artists.
- Each card is clickable and navigates to the corresponding library section.

### Recently Played
- Last 20 tracks played, sorted by most recent first.
- Each row shows album art, title, artist, and a relative timestamp (e.g., "2h ago", "Yesterday").
- Click a row to play the track.

### Recently Added
- Newest tracks added to the library (sorted by file addition date).
- Useful for seeing what a recent scan picked up.

### Top Artists
- Artists ranked by total play count.
- Click an artist to open their [Artist Detail](artists.md) page.

---

## Tips

- Smart Mixes are generated automatically when the library has enough tracks. Run a library scan if the carousel is empty.
- The search bar on the Home page is a shortcut — the full [Search](search.md) page supports category filtering.
- Recently Played only shows tracks that have been played through the player at least once. Plays are recorded in SQLite.
