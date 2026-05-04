# Genres

![Genres page](../assets/screenshots/genres.png)

The Genres page organizes your library by genre tag, sourced from the embedded file metadata or AI-enriched genre data.

---

## Layout

Genres are shown as a card grid. Each card shows:
- Genre name
- Track count

---

## Genre Detail

Clicking a genre card opens the **Genre Detail** page, which shows:

- All tracks tagged with that genre
- Play All and Shuffle buttons
- Context menu on individual tracks

---

## Genre Enrichment

Genres are populated from two sources:

1. **File metadata** — Tags embedded in MP3/FLAC/AAC files during ripping
2. **AI enrichment** — The configured LLM provider can infer genres from artist and album names

To trigger genre enrichment, go to [Settings → Library Intelligence → Genre Enrichment](settings.md#library-intelligence).

Genres that are blank in the original file tags will remain blank until enrichment runs.
