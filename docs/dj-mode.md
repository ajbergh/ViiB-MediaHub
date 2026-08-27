# DJ Mode

![DJ Mode](../assets/screenshots/dj-mode.png)

DJ Mode is ViiB MediaHub's two-deck mixing interface. Its library browser works from the ViiB catalog, so synchronized Plex music can be discoverable alongside local filesystem tracks where the active DJ/audio pipeline can load and decode the track through ViiB's normal media route.

> DJ Mode requires a sufficiently wide desktop layout. Mobile layouts are not supported for the full DJ interface.

---

## Source behavior

DJ Mode does not maintain a separate Plex library model. Track selection starts from the same ViiB song catalog used by Songs, Search, playlists, and the normal player.

For local tracks, audio comes from the local ViiB media route. For Plex-backed tracks, ViiB resolves the same `/api/audio/{songId}` contract through its backend-authenticated PMS proxy. Plex tokens remain server-side.

Because DJ workflows can depend on active audio decoding, waveform/analysis, cueing, and precise seeking, a Plex-backed track requires the PMS source to be reachable and authenticated while it is loaded/played. Cached Plex metadata can remain visible when PMS is offline, but remote audio-dependent DJ functions cannot operate without the source media.

Plex support remains audio-only; DJ Mode does not introduce Plex video or video-transcode support.

---

## AI DJ generated sets

The **AI DJ** page is the set-generation workflow; it is distinct from this two-deck performance interface. When the Semantic Retrieval Index is ready, each generated set phase retrieves a bounded pool of catalog songs matching the phase's positive and negative semantic intent. Existing persona, BPM/flow, recency, artist-diversity, and sequencing logic then chooses the final queue.

The LLM receives the prompt and compact planning context, not a dump of the ViiB catalog or local genre taxonomy. Generated queues always contain existing ViiB song IDs and retain the selected local/Plex source constraint. If the index is unavailable or its pools cannot satisfy the plan without reusing songs, ViiB falls back to the established metadata/full-catalog path before sequencing.

The AI DJ page shows index readiness and optional count-only retrieval diagnostics. Configure, reindex, test, or recover the semantic provider in **Settings → Library Intelligence**.

---

## Layout

The interface provides two decks, waveform/analysis surfaces, a central mixer, and a library browser. Depending on the current build and platform, controls include transport, tempo, cue/loop behavior, EQ, crossfader, VU meters, sampler, MIDI mapping, and output routing.

---

## Decks and transport

Each deck can load a ViiB catalog track and exposes the supported transport/mixing controls, such as:

- Play / Pause
- Cue
- Sync where BPM metadata/analysis is available
- Tempo/pitch adjustment
- Loop controls
- Hot cues and beat jump where supported
- Per-deck level/EQ/FX controls

Track-specific analysis features depend on media being readable by the active browser/WebView audio stack.

---

## Waveforms and analysis

DJ Mode can display waveform/position information and use BPM/key/beat metadata or analysis where available. Remote Plex media is not copied to a local music folder merely to enable DJ Mode; access remains through the ViiB/PMS playback path.

If a particular Plex audio codec is not directly supported by the current player/WebView pipeline, ViiB does not silently invoke Plex video/general transcoding. The Plex integration currently favors direct audio play.

---

## Mixer and output

The mixer combines Deck A and Deck B with the supported channel levels, EQ, crossfader, master level, and monitoring controls.

Audio output configuration can include a primary output and headphone/cue output on platforms where the Web Audio/device-routing APIs provide the required capabilities.

See [Settings](settings.md#audio-output-devices).

---

## Library Browser

The DJ library browser filters the ViiB catalog and exposes available metadata such as title, artist, duration, BPM, or key where populated.

Local and Plex tracks are not duplicated into separate source tabs merely because their media origin differs. A small source indicator may be useful in the UI, but catalog identity and normal selection behavior remain unified.

---

## Sampler, cue points, MIDI, and recording

DJ Mode also exposes the build's supported performance tools, which can include sampler pads, cue/beat-grid state, MIDI mappings, and recording of the ViiB master mix.

These are ViiB-side performance features. They do not modify Plex Media Server media or library configuration.

---

## Reliability notes for Plex tracks

- A temporary PMS outage does not delete synchronized Plex catalog metadata.
- Loading or playing a Plex track requires PMS connectivity and valid authentication.
- Seeking uses the normal Plex Range-aware backend proxy.
- No Plex credentials are exposed in the DJ renderer's media URL.
- Removing a Plex source from ViiB never deletes media from PMS.

For full Plex source behavior, see [Plex Media Server Music Support](plex-music.md).
