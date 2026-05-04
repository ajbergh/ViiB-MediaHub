# Albums

![Albums page](../assets/screenshots/albums.png)

The Albums page displays your library organized by album. Each album is shown as a card with cover art (or a color gradient when art is unavailable).

---

## Layout

Albums are shown in a responsive grid. The number of columns adjusts automatically based on window width.

Each card shows:
- Cover art image or generated color gradient
- Album title
- Artist name
- Track count

---

## Album Detail

Clicking an album card opens the **Album Detail** page, which shows:

- Large album artwork
- Album title, artist, and year (if available)
- Track listing with play counts and durations
- **Play Album** button (plays all tracks in album order)
- **Shuffle** button
- Like button for the whole album
- Context menu on each track

### Album Detail Context Menu

Right-click any track inside an album to:
- Play the track
- Play Next / Add to Queue
- Add to playlist
- Like / Unlike

---

## Metadata Enrichment

Albums can be enriched automatically in the background using the configured AI or Last.FM integration. Enriched albums may have:

- High-resolution cover art from Spotify or MusicBrainz
- Correct release year
- Genre tags

Run background enrichment from the [Settings](settings.md) page under **Library Intelligence**.

---

## Album Like

Clicking the heart icon on an album card toggles the like status. Liked albums appear in [Liked Albums](liked.md).
