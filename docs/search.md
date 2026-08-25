# Search

![Search page](../assets/screenshots/search.png)

The Search page finds tracks, albums, artists, playlists, genres, and media identities in the ViiB catalog. In desktop/backend mode it uses the indexed `/api/v2/search` endpoint; browser-only mode and failed server requests fall back to songs already loaded in the renderer.

The indexed catalog includes both local filesystem tracks and synchronized Plex music tracks. Spotify catalog search remains separate on the [Spotify](spotify.md) page.

---

## Accessing Search

- Click **Search** in the sidebar.
- Type a query in the [Home](home.md) search bar and press **Enter**.

---

## Features

| Feature | Description |
|---|---|
| Debounced results | Results update after typing pauses; a newer query cancels the previous request |
| Category tabs | Filter results by All, Tracks, Albums, Artists, Playlists |
| Indexed backend search | Desktop/backend mode searches the persisted catalog index instead of filtering a full library download |
| Source-transparent results | Local and synchronized Plex tracks use the same search/result interactions |
| Play directly | Click a result to play immediately through the normal ViiB player |
| Persistent state | Last query and selected tab are retained across navigation |
| Resilient fallback | If indexed search fails, the page can fall back to songs already loaded locally in the renderer |

---

## Result Categories

### All
Shows top results from all supported categories.

### Tracks
Backend mode matches indexed song metadata such as title, artist, album, album artist, genre, and source/media identity fields. Local fallback searches the song metadata already loaded in the renderer.

For Plex tracks, playback still uses the normal ViiB song result; the frontend does not receive a Plex token or raw authenticated PMS media URL.

### Albums
Matches album name, album artist, or track artist. Clicking an album opens [Album Detail](albums.md).

### Artists
Matches artist name. Clicking an artist opens [Artist Detail](artists.md).

### Playlists
Matches playlist name. Clicking a playlist opens [Playlist Detail](playlists.md).

---

## Search Tips

- Search is case-insensitive.
- Backend mode uses prefix-oriented indexing to keep large-library queries efficient; enter additional leading characters to narrow broad results.
- Plex tracks become searchable after a successful Plex library synchronization.
- A temporarily offline Plex server does not remove cached Plex tracks from search. Those tracks remain cataloged, while playback can report source unavailability until PMS reconnects.
- Spotify's remote catalog is not folded into ViiB's indexed catalog search; use [Spotify](spotify.md) for Spotify catalog discovery.
