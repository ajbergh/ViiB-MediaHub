# AI Smart Mix

![AI Smart Mix](../assets/screenshots/smart-playlists.png)

Smart Playlists and AI Smart Mix operate on ViiB's canonical music catalog. After a Plex music library has synchronized successfully, Plex tracks can participate alongside local filesystem tracks in the same generated playlists and DJ sets.

Spotify's remote catalog is not implicitly searched by this feature; Spotify remains a separate integration.

---

## Playlist Mode

Playlist Mode builds a one-shot ViiB playlist from a natural-language request. When the Semantic Retrieval Index is ready, ViiB retrieves matching tracks, albums, and artists by meaning before applying listening-history, source, year, artist, and diversity rules. When the index is unavailable, the established metadata/history matching path remains available.

Examples:

```text
90s alternative rock
chill evening vibes
upbeat morning run
jazz standards for dinner
```

Generation combines semantic relevance with source-current audio metadata and listening history. The AI model interprets the request; local code enforces numeric requirements, ranks eligible tracks, and orders the result. Listening behaviour stays a ranking signal; it is not added to embedding text.

Requests can include BPM ranges, local energy levels (1–10), Camelot keys, moods, an energy progression, and duration. For example: `45 minutes of instrumental jazz between 90 and 110 BPM, building gradually`. Explicit numeric requirements exclude tracks with unknown or unqualified measurements. Approximate preferences such as `around 100 BPM` rank known matches higher while keeping missing data neutral. Adaptive retrieval refills rejected candidates without relaxing requirements or semantic quality floors.

Manual overrides, admitted current Spotify metadata, and qualified local analysis use the existing source-aware resolver. Local energy levels, Spotify energy/danceability/valence scores, and native energy curves remain distinct metrics. Provider scores are optional preference signals. Tempo continuity, requested Camelot compatibility, mood, artist spacing, and energy progression guide ordering; current local structure/energy artifacts add transition evidence where available. Missing artifacts never become fabricated measurements. This ordering does not authorize beat synchronization.

Duration requests select the closest whole-track duration. Missing durations use a three-minute estimate, disclosed through the response's `durationEstimated` field. The response also includes interpreted `audioIntent` and `durationSeconds`. Selection and ordering are bounded, and genuine shortages are reported rather than filled with unrelated tracks. Source and scalar changes before publication require a fresh recommendation.

Generated results use normal ViiB song IDs, so a result can contain both local and Plex-backed catalog tracks without a Plex-specific playback path in the UI.

---

## Semantic retrieval

The **Semantic Retrieval Index** in **Settings → AI & Enrichment** uses a fixed, managed local embedding model (Model2Vec `minishlab/potion-base-8M`). ViiB automatically downloads and verifies pinned model files on first use, then computes embeddings and stores vectors in local SQLite. There are no embedding provider or model choices.

The index is independent of the selected chat provider. Switching between OpenAI, OpenRouter, Gemini, Anthropic, and Ollama does not change the vector space or require reindexing. The selected AI model interprets requests and audits candidates; the semantic index remains local. No embedding API or Ollama embedding model is required.

Indexing runs in the background. The Smart Playlists page displays whether semantic matching is ready, indexing, or using the standard fallback. File paths, internal IDs, listening history, and vectors are excluded from semantic document text sent to the AI model. Strict numeric audio requests require the semantic metadata path; if it is unavailable, generation reports that limitation instead of bypassing those requirements.

After a semantic result is generated, **Semantic retrieval details** can be expanded for count-only troubleshooting information such as candidate and track/album/artist-match counts. It is collapsed by default and never exposes document text or vectors.

Semantic matching is selected automatically when the chosen source has a searchable index. Explicit artist exclusions, source selection, hard year constraints, instrumental-only requests, recent-play avoidance, and one-per-artist selection are still enforced locally.

---

## DJ Mode

DJ Mode compiles a set plan with semantic intent for every phase, then retrieves bounded candidate pools for those phases before applying persona/selection bias, duration targets, transition/flow constraints, and energy progression. Actual track durations determine selection within each phase's share of the set duration; planner song counts provide the phase proportions. Whole tracks and limited matching pools can make the final duration approximate.

The resulting set is still built from the ViiB catalog and handed to the normal player/queue. ViiB does not send the full library or local genre taxonomy to the LLM. If the semantic index is unavailable, DJ Mode uses the legacy catalog path with the same resolved audio requirements. A shortage in a searchable semantic phase remains a reported shortage; it does not trigger unrelated catalog backfill. DJ ordering uses the shared audio features and transition scorer, including continuity across phase boundaries. Harmonic compatibility is an automatic advisory signal in DJ Mode.

Generated DJ rows display the fractional BPM and source used to sequence the set: Spotify, Manual, Measured, or Unknown. This analysis snapshot stays in page memory, is cleared when the Spotify session changes, and is omitted from saved song copies. A newer generation supersedes an older pending request. These scalar values do not authorize beat-phase sync or qualify analysis accuracy. Further local scanning/DSP improvements are [deferred to a future branch](LOCAL_ANALYSIS_FUTURE_BRANCH.md).

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

For best results, configure the desired AI/metadata provider in [Settings → AI & Enrichment](settings.md#ai--enrichment).

ViiB-side enrichment may add or improve genre, mood, energy, tempo, BPM, and year metadata used by Smart Mixes/AI Smart Mix.

For Plex-backed tracks, ViiB enrichment is local ViiB state. ViiB does not silently write enriched metadata back to Plex. The separate [AI Metadata Writeback](plex-music.md#ai-metadata-writeback-explicit-opt-in) workflow can send approved track genres and original release year after review; other enriched fields and embeddings remain ViiB-only. A later Plex synchronization can refresh fields sourced from PMS.
