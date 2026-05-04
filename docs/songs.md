# Songs

![Songs page](../assets/screenshots/songs.png)

The Songs page shows every track in your library with virtualized scrolling for high performance even with tens of thousands of tracks.

---

## Features

| Feature | Description |
|---|---|
| Virtualized list | Uses `react-virtuoso` — 50,000 track libraries render without lag |
| Sort options | Title (A-Z/Z-A), Artist, Album, Duration, Most Played, Recently Added |
| Filter bar | Type to filter the visible list in real time |
| Play All | Plays the entire (filtered/sorted) list from the first track |
| Shuffle All | Queues all visible tracks in random order |
| Add Folder | Opens the Settings folder browser to add a new scan directory |
| Context menu | Right-click any row for extended actions |
| Like button | Inline heart icon to like/unlike a track |
| Loading skeleton | Shows a placeholder grid while the library is loading |
| Empty state | Shown when the library is empty with a prompt to scan a folder |

---

## Sort Options

| Option | Description |
|---|---|
| Recently Added | Default; newest files first |
| Title (A-Z) / (Z-A) | Alphabetical by track title |
| Artist (A-Z) / (Z-A) | Alphabetical by artist name |
| Album (A-Z) / (Z-A) | Alphabetical by album name |
| Duration (Short) | Shortest tracks first |
| Duration (Long) | Longest tracks first |
| Most Played | Highest play count first |

---

## Context Menu Actions

Right-click any track to access:

- **Play** — Play this track now
- **Play Next** — Insert at the front of the queue
- **Add to Queue** — Append to the end of the queue
- **Add to Playlist** — Add to an existing playlist
- **Like / Unlike** — Toggle the liked status
- **Go to Album** — Navigate to the album detail page
- **Go to Artist** — Navigate to the artist detail page

---

## Row Layout

Each row displays:
- Album art thumbnail (or color gradient fallback)
- Track title and artist
- Album name
- Duration
- Like button
- More (`⋯`) button for the context menu

---

## Keyboard Shortcut

While on the Songs page, pressing **Space** plays/pauses the current track (handled globally by the player).
