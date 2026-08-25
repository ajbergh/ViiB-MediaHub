# Smart Playlists / AI DJ

![Smart Playlists / AI DJ](../assets/screenshots/smart-playlists.png)

Smart Playlists and AI DJ operate on ViiB's canonical music catalog. After a Plex music library has synchronized successfully, Plex tracks can participate alongside local filesystem tracks in the same generated playlists and DJ sets.

Spotify's remote catalog is not implicitly searched by this feature; Spotify remains a separate integration.

---

## Playlist Mode

Playlist Mode builds a one-shot ViiB playlist from a natural-language request and the metadata/history already available in the ViiB catalog.

Examples:

```text
90s alternative rock
chill evening vibes
upbeat morning run
jazz standards for dinner
```

Generation can use artist/album/genre metadata, enriched mood/energy/tempo information, and listening history depending on the active implementation and configured provider.

Generated results use normal ViiB song IDs, so a result can contain both local and Plex-backed catalog tracks without a Plex-specific playback path in the UI.

---

## DJ Mode

DJ Mode adds set-building behavior such as persona/selection bias, duration targets, transition/flow constraints, and energy progression where available in the current UI.

The resulting set is still built from the ViiB catalog and handed to the normal player/queue.

---

## Smart Mixes

Home-page Smart Mixes are derived from catalog metadata and listening behavior. Because Plex synchronization writes normalized metadata into the same catalog, Plex tracks can appear in mixes such as favorites, rediscovery, recent/fresh, or genre-based mixes where they satisfy the mix rules.

See [Home](home.md).

---

## Source availability

A temporarily offline Plex server does not cause AI/Smart Mix generation to erase cached Plex catalog entries. Those tracks can remain discoverable as catalog candidates, but playback will require PMS to be reachable and authenticated at the time the track is played.

A later successful authoritative Plex synchronization reconciles additions, updates, and confirmed removals.

---

## Metadata enrichment

For best results, configure the desired AI/metadata provider in [Settings → Library Intelligence](settings.md#library-intelligence).

ViiB-side enrichment may add or improve genre, mood, energy, tempo, BPM, and year metadata used by Smart Mixes/AI DJ.

For Plex-backed tracks, ViiB enrichment is local ViiB state. ViiB does not silently write enriched metadata back to Plex; a later Plex synchronization can refresh fields that are sourced from PMS.
