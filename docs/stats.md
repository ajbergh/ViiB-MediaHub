# Stats

![Stats page](../assets/screenshots/stats.png)

The Stats page shows personalized listening statistics and insights derived from your play history.

---

## Accessing Stats

Click **Stats** in the sidebar.

---

## Overview Cards

The top of the page shows high-level numbers:

| Stat | Description |
|---|---|
| Total Listening Time | Sum of all play durations recorded |
| This Month | Listening time for the current calendar month |
| This Week | Listening time for the current week |
| Total Plays | Total number of play events recorded |

---

## Top Charts

### Top Artists
- Ranked by total play count.
- Click an artist to navigate to their [Artist Detail](artists.md) page.

### Top Albums
- Ranked by total play count.
- Click an album to navigate to its [Album Detail](albums.md) page.

### Top Genres
- Ranked by total plays across all tracks in each genre.
- Requires genre enrichment to be populated (see [Settings → Library Intelligence](settings.md#library-intelligence)).

---

## Activity Calendar / Heatmap

A calendar grid showing your listening activity over time. Each cell represents one day and is shaded by the number of plays — darker = more active.

---

## Fun Statistics

A set of additional highlight stats, including:

- **Most Played Song** — the track with the highest all-time play count
- **Longest Listening Session** — the session with the most continuous listening time
- **Favorite Day** — the day of the week you listen most
- **Average Session Length** — mean duration of a listening session

---

## Recently Played

A chronological list of the last 50 play events with track name, artist, and timestamp.

---

## Empty State

If you have never played a track, the Stats page shows an empty state prompting you to start listening. Stats begin accumulating from the first play.

---

## Data Source

All statistics are computed in real time from the `plays` table in the SQLite database. No data is sent externally.
