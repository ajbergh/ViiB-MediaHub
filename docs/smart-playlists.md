# Smart Playlists / AI DJ

![Smart Playlists / AI DJ](../assets/screenshots/smart-playlists.png)

The Smart Playlists page provides two related capabilities:

1. **Playlist Mode** — Generate a one-shot playlist from a natural language prompt
2. **DJ Mode** — Generate a structured DJ set with energy progression, persona, and transitions

Both use the configured LLM provider (see [Settings → Library Intelligence](settings.md#library-intelligence)) to match your local library tracks against the prompt.

---

## Mode Toggle

A toggle at the top of the page switches between **Playlist** and **DJ** modes.

---

## Playlist Mode

### Prompt Input

Type a natural language description of the music you want:

```
90s alternative rock
chill evening vibes
upbeat morning run
jazz standards for dinner
```

### Options

| Option | Description |
|---|---|
| Discovery | **Balanced** — mix of familiar and new; **Discover New** — leans toward unplayed tracks; **Favorites** — leans toward your most-played |
| Avoid Recently Played | Exclude tracks played within the chosen window (1 h, 6 h, 24 h, 3 d, 1 wk) |
| One Per Artist | Ensures no artist repeats in the generated list |
| Time-Aware Mode | Adjusts energy level based on the current time of day |

### Generate

Click **Generate** to run the playlist generation. The backend performs a four-tier matching pass:

1. Exact genre/mood match from enriched metadata
2. Fuzzy match using the LLM provider
3. BPM/energy-range scoring
4. Fallback using play history popularity

### Result Actions

- **Play** — Load the generated list into the queue and start playing
- **Save as Playlist** — Saves the result as a named [Playlist](playlists.md)
- **Regenerate** — Run a new generation with the same prompt and options

---

## DJ Mode

DJ Mode adds professional set-building on top of Playlist Mode.

### Additional DJ Controls

| Control | Description |
|---|---|
| Persona | Chooses the scoring bias for track selection |
| Set Duration | Target length of the DJ set (15–120 minutes) |
| Flow Strictness | How tightly BPM must stay consistent across transitions |
| Talk Mode | Inserts DJ narration cue markers between tracks |

### DJ Personas

| Persona | Bias |
|---|---|
| Flow Master | Smooth BPM continuity, minimal key jumps |
| Crowd Pleaser | Favors your highest-rated and most-played tracks |
| Deep Cut DJ | Finds hidden gems and rarely played tracks |
| Explorer | Balanced between familiar and novel |
| Curator | Strict genre purity, one artist per set |
| Night Drive | Smooth tempos, medium energy, atmospheric feel |

### Energy Arc Visualization

When a DJ set is generated, a phase chart shows the energy arc — how the set builds from opening tracks through the peak and winds down to the closing tracks. Each phase is color-coded.

---

## Smart Mix Cards (Home Page)

Smart Mixes are pre-generated playlists that appear on the [Home](home.md) page as a scrollable carousel. They are automatically created from your library's genre and mood distribution. Smart Mixes refresh when the library changes.

### Smart Mix Detail

Clicking a Smart Mix card opens the Smart Mix Detail page, showing:
- Mix name and description
- Full track list
- Play / Shuffle controls

---

## Requirements

- At least one AI provider must be configured in [Settings → Library Intelligence](settings.md#library-intelligence).
- Genre enrichment should be run first for best results (the AI can still generate playlists using artist/album names alone).
- Mood, energy, and tempo metadata (from Unified Enrichment) improve DJ mode accuracy.
