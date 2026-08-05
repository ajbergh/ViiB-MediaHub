# Search

![Search page](../assets/screenshots/search.png)

The Search page finds tracks, albums, artists, playlists, genres, and media paths in your local library. In desktop/backend mode it uses the indexed `/api/v2/search` endpoint; browser-only mode and a failed server request fall back to the songs already loaded in the renderer.

---

## Accessing Search

- Click **Search** in the sidebar.
- Type a query in the [Home](home.md) page search bar and press **Enter** — the Search page opens with that query pre-filled.

---

## Features

| Feature | Description |
|---|---|
| Debounced results | Results update 250 ms after typing stops; a new query aborts the previous server request |
| Category tabs | Filter results by All, Tracks, Albums, Artists, Playlists |
| Indexed backend search | Desktop/backend mode searches the persisted index instead of filtering a full library download |
| Play directly | Click a result to play immediately |
| Persistent state | Your last query and selected tab are remembered when you navigate away and return |
| Resilient fallback | If the indexed request fails, the page explains that it is showing locally loaded results |

---

## Result Categories

### All
Shows the top results from all categories combined.

### Tracks
In backend mode, matches the beginning of title, artist, album, album artist, first genre, or media path. The local fallback matches title, artist, album, and genre from already loaded songs.

### Albums
Matches the beginning of album name, album artist, or track artist. Clicking an album navigates to [Album Detail](albums.md).

### Artists
Matches the beginning of artist name. Clicking an artist navigates to [Artist Detail](artists.md).

### Playlists
Matches the beginning of playlist name. Clicking a playlist navigates to the [Playlist Detail](playlists.md) page.

---

## Search Tips

- Search is case-insensitive. Backend mode uses prefix matching to keep large-library queries index-friendly; begin with the title, artist, album, genre, path, or playlist name you expect.
- Results are capped per category. Add more leading characters to narrow a broad query.
- The search searches your **local library only** — for Spotify catalog search, use the [Spotify](spotify.md) page.
