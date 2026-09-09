# Artists

![Artists page](../assets/screenshots/artists.png)

The Artists page lists distinct artists represented in the canonical ViiB song catalog. Local filesystem music and synchronized Plex music contribute to the same artist and discography views.

---

## Artist browser

Artist cards show the artist name, available artwork/fallback presentation, and catalog depth information. Selecting an artist opens the normal Artist Detail view.

There is no separate Plex artist browser after synchronization.

---

## Artist Detail

Artist Detail can show:

- artist identity and artwork;
- album/discography groupings;
- all matching catalog tracks;
- Play All / Shuffle actions;
- navigation to album details.

Local and Plex-backed songs use the same ViiB queue/player interactions.

---

## Plex behavior

Plex synchronization maps PMS artist/album/track metadata into ViiB's existing song fields. PMS machine/library/track identity is retained separately by the backend for synchronization and playback, so artist aggregation does not need Plex-specific UI branching. When PMS supplies an artist image, ViiB can serve it through its authenticated backend proxy; the browser never receives a Plex credential-bearing artwork URL.

If PMS is temporarily offline, already synchronized artist and album metadata remain in ViiB. Remote playback resumes when the Plex source becomes reachable and authenticated again.

---

## Metadata enrichment

Configured AI/metadata services can enrich ViiB's catalog presentation. Enrichment is ViiB-side state; ViiB does not silently write artist or track metadata back to Plex. The separate Plex writeback workflow requires review and explicit approval and is limited to eligible track genres and original release year; it never changes artist metadata or audio-file tags.

See [Settings → Library Intelligence](settings.md#library-intelligence).
