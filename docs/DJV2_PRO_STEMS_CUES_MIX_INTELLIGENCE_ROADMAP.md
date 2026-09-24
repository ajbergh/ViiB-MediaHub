# DJv2 Pro Stems, Cue Intelligence & Mix Planning Roadmap

**Status:** Proposed implementation roadmap  
**Scope:** DJv2 only; extends, but does not replace, DJV2_PROFESSIONAL_TRACK_ANALYSIS_ROADMAP.md  
**Repository snapshot reviewed:** main at d02ad01dd5d384a77383b70f2669f9dd1c6c1761 (v1.0.0-rc3)  
**Research snapshot:** 2026-09-23  
**Primary goals:** professional-grade stem playback, scan-time cue creation, Camelot-first library UX, 1-10 energy analysis, structure-aware transition planning, mashup auditioning, and high-quality DJ preparation workflows.

> This roadmap uses public behavior from Mixed In Key 11 Pro as product inspiration and StemDeck as an open-source implementation reference. It does not attempt to reproduce proprietary Mixed In Key algorithms or visual trade dress. Where StemDeck code is reused directly, Apache-2.0 attribution and third-party notices must be preserved.

---

## 1. Product vision

DJv2 should evolve from a capable two-deck player into a local-first DJ preparation and performance environment.

The target experience is:

1. Import or scan music once.
2. Analyze BPM, beatgrid, key, Camelot/Open Key, loudness, energy, song structure and up to eight useful cue points.
3. Optionally pre-separate tracks into stems in the background.
4. Load a track and immediately see:
   - BPM and confidence;
   - musical key and color-coded Camelot notation;
   - 1-10 Energy Level;
   - phrase and section markers;
   - automatic cue points;
   - stem availability;
   - recommended next tracks and why they match.
5. During playback, use dedicated stem controls for vocals, drums, bass and music/melody without breaking beat sync, loops, cueing, EQ, FX, crossfader, headphone cue, scratch or deck state.
6. Switch to Mix Planning or Mashup Planning to audition candidate transitions, pitch shifts, loops and stem combinations before committing them to a playlist.

The objective is not merely feature parity with another application. The objective is to build a coherent DJ intelligence layer around the architecture ViiB already has.

---

## 2. Verified ViiB baseline

The repository already contains substantial foundations that should be reused instead of rebuilt.

### 2.1 Existing persistent analysis

ViiB already persists and exposes:

- audio-measured BPM and confidence;
- alternate tempo candidates and tempo stability;
- musical key and confidence;
- Camelot and Open Key notation;
- beat positions and downbeats;
- beatgrid manual edits;
- integrated LUFS and true peak;
- track energy curves;
- section-like energy regions;
- advisory mix-in, mix-out and section cue suggestions;
- transition recommendations based on energy, loudness and phrase-preparation evidence.

Relevant implementation areas:

- backend/internal/analysis/
- backend/internal/analysis/features/
- backend/internal/api/
- services/api.ts
- components/dj/v2/DJEnergyInsights.tsx
- docs/DJV2_PROFESSIONAL_TRACK_ANALYSIS_ROADMAP.md

### 2.2 Existing DJ hot cues

ViiB already supports eight saved hot-cue slots per track through:

- GET /api/dj/hotcues/{id}
- PUT /api/dj/hotcues/{id}

Current cue fields are:

- slot;
- position;
- label;
- color.

The new cue-intelligence work should extend this model rather than create a second unrelated cue system.

### 2.3 Existing DJ audio graph

lib/djAudio.ts currently creates one HTMLAudioElement per deck and routes each deck through the Web Audio graph:

    MediaElement source
        -> deck gain
        -> 3-band EQ
        -> deck FX
        -> crossfader
        -> deck analyser
        -> master
        -> limiter
        -> output

The engine already supports:

- independent deck transport;
- EQ;
- filter, delay, flanger and reverb;
- crossfader;
- separate headphone cue routing;
- master/headphone device routing;
- tempo control;
- browser pitch preservation through HTMLMediaElement.preservesPitch;
- beat sync;
- loops;
- slip/scratch work using AudioWorklet;
- VU metering.

Stem playback must integrate before the existing deck gain/EQ/FX section so the rest of the mixer remains unchanged.

### 2.4 Existing DJ library

DJLibraryBrowserV2 already displays durable analysis and supports:

- BPM;
- key/Camelot text;
- harmonic compatibility;
- sorting;
- playlists/categories;
- configurable columns;
- load-to-deck;
- drag/drop;
- manually assigned track color labels.

The current track color label is session-only state. It is not the same thing as deterministic Camelot coloring.

### 2.5 Existing job scheduler

The backend already has a persisted long-running job system, auto-analysis triggers and a DJ playback pressure signal. Stem separation should be another durable job type within that scheduler, not an unrelated background process manager.

---

## 3. Public reference behavior

### 3.1 StemDeck

StemDeck is Apache-2.0 and currently documents:

- local 6-stem separation using Demucs htdemucs_6s;
- vocals, drums, bass, guitar, piano and other;
- CUDA, Apple MPS and CPU execution;
- per-stem mute, solo and level controls;
- synchronized multitrack waveform display;
- cancellable jobs;
- local storage and cached model weights;
- stem export;
- FastAPI/SSE job progress;
- BPM, key and loudness analysis;
- optional song-section analysis.

Reference:
https://github.com/stemdeckapp/stemdeck

Its architecture is useful as a reference for model lifecycle, cancellation, device detection, model caching, file layout and job UX. ViiB should not embed StemDeck wholesale because ViiB is Go/Wails-first and already owns its job scheduler, catalog and Web Audio engine.

### 3.2 Mixed In Key 11 Pro

Public Mixed In Key material currently describes:

- key detection and Camelot notation;
- 1-10 Energy Level;
- automatic cue-point creation;
- up to eight cue points per track;
- cue points aligned to beats/downbeats;
- editable cue snapping;
- DJ Mix Mode recommendations;
- Mashup Mode recommendations;
- BPM, key and energy filtering;
- stem separation;
- vocals/instrumental auditioning;
- pitch shifting to discover additional compatible combinations;
- looped auditioning;
- playlist/favorite idea management;
- export of stems and DJ preparation metadata.

References:

- https://mixedinkey.com/pro/
- https://mixedinkey.com/workflows/how-to-use-the-camelot-wheel/
- https://mixedinkey.com/workflows/use-energy-level-detection/
- https://mixedinkey.com/harmonic-mixing-guide/beat-jumping-with-cue-points-dj-mixing-tutorial/
- https://mixedinkey.com/release-notes/mixed-in-key-pro/

These references define product behavior only. ViiB should implement its own analysis and scoring methods.

---

# PART I — TARGET ARCHITECTURE

## 4. Architectural principles

### 4.1 Preserve the current Go/Wails core

The ViiB backend remains the source of truth for:

- songs;
- playlists;
- analysis artifacts;
- jobs;
- stem metadata;
- user cues;
- recommendation data;
- configuration.

A stem model runtime may be implemented as a managed sidecar because PyTorch/Demucs is not a sensible dependency to force into the Go process.

### 4.2 Treat heavy ML as a capability

Stem separation must be capability-negotiated.

The application should be fully usable when stem separation is unavailable.

Capability states should include:

- unavailable;
- runtime_missing;
- model_missing;
- ready_cpu;
- ready_cuda;
- ready_mps;
- error.

Desktop builds may offer a managed local worker. Browser builds should expose stem processing only when the serving backend has a configured stem worker.

### 4.3 Never mix generated and user-authored DJ data without provenance

Every generated cue, stem set, energy score and structural marker must record:

- algorithm/model identifier;
- algorithm/model version;
- source audio hash;
- generation timestamp;
- confidence when applicable;
- whether the artifact has been manually edited.

A re-analysis operation may replace old generated artifacts, but must never silently replace a user-edited cue or beatgrid.

### 4.4 Preserve one transport clock per deck

A professional stem implementation cannot allow each stem to drift independently.

All stems on one deck must share:

- play state;
- position;
- tempo;
- loop boundaries;
- scratch state;
- sync state.

Per-stem gain/mute/solo is independent. Transport is not.

### 4.5 Degrade safely

If stem files are missing, corrupt, still processing or incompatible, the deck must fall back to the original track without changing the musical position.

---

## 5. Proposed backend components

Add the following high-level components:

    backend/internal/stems/
        capability.go
        worker.go
        manifest.go
        storage.go
        validation.go

    backend/internal/api/
        v2_stems.go
        v2_stem_jobs.go

    backend/internal/db/
        stem persistence additions

    backend/internal/analysis/cues/
        generator.go
        policy.go

    backend/internal/analysis/energy/
        level.go

    backend/internal/analysis/structure/
        structure.go

A Python worker may live under a separate runtime directory such as:

    stem-worker/
        pyproject.toml
        viib_stem_worker/
        models/
        tests/

The worker boundary should be intentionally small.

Recommended worker operations:

- health;
- capabilities;
- ensure-model;
- separate;
- cancel;
- shutdown.

Do not duplicate ViiB job state in the worker. ViiB owns the durable job record; the worker performs one task at a time and reports structured progress.

---

## 6. Proposed persistence model

### 6.1 Stem set

Create one logical stem set per track/model/audio revision.

Suggested fields:

**track_stem_sets**

- id
- song_id
- source_audio_hash
- model_name
- model_version
- worker_version
- stem_layout
- status
- sample_rate
- channels
- duration_seconds
- device
- generated_at
- error_code
- error_message
- manually_invalidated

Recommended unique identity:

    song_id + source_audio_hash + model_name + model_version + stem_layout

### 6.2 Individual stem artifacts

**track_stems**

- stem_set_id
- stem_name
- path
- byte_size
- checksum
- sample_rate
- channels
- frame_count
- peak_dbfs

Supported canonical names:

- vocals
- drums
- bass
- guitar
- piano
- other

Derived DJ groups do not need separate files initially.

The four-button DJ mapping is:

- VOCAL = vocals
- DRUMS = drums
- BASS = bass
- MUSIC = guitar + piano + other

Derived convenience mixes may later be cached:

- instrumental = all non-vocal stems
- acapella = vocals
- melody = guitar + piano + other

### 6.3 Generated cue provenance

Extend persisted hot cues with:

- origin: user | analysis
- generator_version
- confidence
- kind
- locked
- updated_at

Suggested kinds:

- intro
- phrase
- vocal-in
- build
- drop
- breakdown
- chorus
- mix-in
- mix-out
- outro
- custom

A user move/edit converts an analysis cue into user-owned state unless the user explicitly chooses to keep it linked to analysis.

### 6.4 Scalar Energy Level

Extend analysis metadata with:

- energy_level: integer 1-10
- energy_level_confidence
- energy_algorithm_version

Keep this separate from the existing time-varying energy curve.

A scalar score is useful for library sorting. The energy curve remains useful for transition and structure reasoning.

### 6.5 Structural timeline

Add a versioned structure artifact:

- start
- end
- label
- confidence
- downbeat_start
- downbeat_end
- mean_energy
- vocal_activity
- drum_activity
- harmonic_key when available

Initial labels:

- intro
- verse
- pre-chorus
- chorus
- build
- drop
- breakdown
- bridge
- outro
- unknown

Do not force a semantic label when confidence is weak. A phrase-boundary-only result is preferable to a wrong label.

---

## 7. Proposed API surface

### 7.1 Stem capabilities

GET /api/v2/stems/capabilities

Response should include:

- available;
- workerVersion;
- supportedModels;
- selectedModel;
- device;
- modelReady;
- estimatedModelBytes;
- supportedStemLayouts.

### 7.2 Track stem state

GET /api/v2/tracks/{id}/stems

Returns:

- status;
- active stem set;
- available stems;
- model/version;
- device used;
- disk usage;
- generatedAt;
- error information.

### 7.3 Request separation

POST /api/v2/tracks/{id}/stems

Body:

    {
      "model": "htdemucs_6s",
      "priority": "background",
      "replace": false
    }

The handler creates a durable operation_jobs row and returns the job.

### 7.4 Delete cached stems

DELETE /api/v2/tracks/{id}/stems/{stemSetId}

Deletion must be refused while either deck is actively using the set.

### 7.5 Stem audio access

For simple preview/export:

GET /api/v2/tracks/{id}/stems/{stemSetId}/{stemName}

For professional synchronized deck playback, add a frame-oriented endpoint instead of relying on six independent media elements:

GET /api/v2/tracks/{id}/stems/{stemSetId}/frames

Parameters:

- startFrame;
- frameCount;
- layout=dj4 or six;
- format=f32le or s16le.

The response packs all requested stem channels from the same frame range so the renderer receives phase-aligned data.

### 7.6 Analysis cues

GET /api/v2/tracks/{id}/analysis-cues

POST /api/v2/tracks/{id}/analysis-cues/apply

Apply modes:

- fill-empty;
- replace-generated;
- selected-only.

Never provide a destructive replace-all mode that can overwrite user cues without explicit confirmation.

---

# PART II — FULL STEM SEPARATION

## 8. Stem worker design

### 8.1 Initial model

Use Demucs htdemucs_6s as the first production candidate because it provides the six outputs required by the requested DJ controls.

The worker should auto-select:

1. CUDA when a supported NVIDIA runtime is available;
2. MPS on supported Apple Silicon;
3. CPU fallback.

The selected device must be visible in Settings and job details.

### 8.2 Runtime packaging

Recommended product approach:

**Development**
- local Python 3.12 environment;
- uv-managed dependency lock;
- worker launched by ViiB.

**Desktop production**
- managed runtime package downloaded or installed on first stem use;
- model weights downloaded separately and cached;
- versioned worker directory;
- checksum verification;
- update independent of the main application when practical.

Do not increase every ViiB installer by the full PyTorch/model footprint unless release testing demonstrates that this is acceptable.

**Web deployment**
- no browser-side PyTorch requirement;
- backend capability endpoint determines availability;
- server administrator configures worker/device.

### 8.3 Worker contract

Input:

- absolute source path or prepared PCM path;
- output directory;
- model;
- device;
- cancellation token/job id.

Progress events:

- preparing;
- loading_model;
- separating;
- validating;
- complete;
- failed;
- cancelled.

The worker should write outputs into a temporary directory and ViiB should atomically promote the directory only after validation succeeds.

### 8.4 Output validation

Before a stem set becomes usable:

- all required files exist;
- all files have identical sample rate;
- all files have identical frame count within a defined tolerance;
- all files have compatible channel count;
- no file is empty;
- duration matches the source within tolerance;
- samples are finite;
- output does not contain catastrophic clipping;
- checksums are recorded.

### 8.5 Cancellation

Reuse the durable job cancellation model.

Cancelling must:

- signal the active worker;
- terminate child processes if needed;
- release GPU memory;
- remove the temporary output directory;
- leave no stem-set row in ready state.

### 8.6 Disk policy

Stem files are large. Add a configurable stem-cache budget.

Settings:

- Stem Cache Location
- Maximum Stem Cache Size
- Delete least-recently-used generated stems automatically
- Keep stems for favorited tracks
- Keep stems used in saved DJ playlists
- Never auto-delete manually exported stems

The cache manager should use stem-set last-used timestamps, not file modification time.

---

## 9. Stem playback architecture

### 9.1 Do not use six unrelated deck clocks

Creating six HTMLAudioElements and pressing play on all of them is acceptable for a prototype, but it is not the target architecture because sample alignment and long-running drift are not guaranteed.

### 9.2 Introduce a DeckSource abstraction

Refactor DJAudioEngine so the rest of the mixer is independent from the transport source.

Suggested interface:

    interface DeckSource {
      load(track): Promise<void>
      play(): Promise<void>
      pause(): void
      seek(seconds): void
      setTempo(rate): void
      setKeyLock(enabled): void
      setLoop(start, end, enabled): void
      getPosition(): number
      getDuration(): number
      isPlaying(): boolean
      dispose(): void
    }

Implement:

- SingleTrackDeckSource — wraps the existing HTMLAudioElement path.
- StemDeckSource — owns the stem transport.

DJAudioEngine continues to own EQ, FX, cue, crossfader, VU and output routing.

### 9.3 Stem bus

The stem source should expose four stereo buses for normal DJ mode:

- vocals;
- drums;
- bass;
- music.

Each bus feeds a GainNode.

Routing:

    Stem transport
      -> Vocal Gain ----      -> Drum Gain ------      -> Bass Gain --------> Stem Sum -> existing deck gain -> EQ -> FX -> crossfader
      -> Music Gain ------/

Stem gain changes must use short Web Audio ramps rather than hard zero/one steps to avoid clicks.

Recommended ramp:

- 5-15 ms for mute/unmute;
- optionally 20-40 ms for performance-safe stem switching.

### 9.4 Production transport

The preferred production design is a single AudioWorklet-based stem transport backed by frame-aligned PCM chunks from the Go server.

Benefits:

- one deck clock;
- deterministic alignment;
- bounded memory;
- no multi-element drift;
- precise seek and loop boundaries;
- a clean path to scratch/slip support.

The worklet can expose four stereo outputs and receive frame blocks for all four groups at once.

The main thread should prefetch enough data to protect against WebView scheduling jitter.

Initial buffering target:

- 2-4 seconds ahead during normal playback;
- larger adaptive buffer on slower storage;
- a separate short seek pre-roll.

### 9.5 Tempo and key lock

Current normal-deck key lock depends on HTMLMediaElement.preservesPitch.

A stem worklet cannot rely on that API.

Therefore introduce a TempoProcessor abstraction before Stem Mode is considered complete.

Required evaluation:

- high-quality WSOLA/phase-vocoder/WASM option;
- compatible license;
- Windows WebView2;
- macOS WKWebView;
- CPU usage on two simultaneous decks;
- latency;
- transient quality on drums;
- vocal quality at +/- 8 percent tempo;
- pitch-shift quality at +/- 4 semitones.

Until a production processor passes the gate, Stem Mode should clearly report any reduced capability. Do not silently change pitch when Key Lock is shown as active.

### 9.6 Scratch/slip

The existing project already has an AudioWorklet scratch path.

The stem transport should eventually feed scratch from the currently audible stem mix, not from the original full track.

Release gate:

- grabbing the jog wheel must not switch audibly from stem mix back to the original track;
- slip mode must resume at the correct underlying stem position.

### 9.7 Loops and beat sync

Because all stems use one transport:

- loops operate on one frame position;
- Sync changes one shared tempo;
- beat-phase nudge changes one shared deck position;
- quantized cue jumps are applied once.

This is a strong reason to avoid independent HTMLMediaElement clocks.

---

## 10. DJ stem controls

### 10.1 Primary four-button mode

Add four large performance controls to each deck:

- VOCAL
- DRUMS
- BASS
- MUSIC

Recommended interaction:

- click: toggle stem on/off;
- Shift+click: solo that stem;
- double-click or dedicated FULL button: restore all;
- secondary menu: per-stem level.

The button should be brightly active when audible and visually dim when removed.

### 10.2 Quick modes

Add optional quick actions:

- FULL
- ACAPELLA
- INSTRUMENTAL

Mappings:

- FULL = all stems
- ACAPELLA = vocals only
- INSTRUMENTAL = everything except vocals

### 10.3 Advanced six-stem panel

An expandable panel exposes:

- Vocals
- Drums
- Bass
- Guitar
- Piano
- Other

The standard deck stays four-group for performance simplicity.

### 10.4 Stem-aware waveform

Add a waveform mode selector:

- Full
- Vocal
- Drums
- Bass
- Music
- Split

Split mode may show four aligned mini-waveforms.

The main playhead and cues are shared.

### 10.5 Stem status in library

Add a compact stem badge to library rows:

- no badge: not generated;
- queued;
- processing;
- ready;
- failed.

Right-click actions:

- Generate Stems
- Regenerate Stems
- Delete Cached Stems
- Open Stem Folder
- Export Stems

---

# PART III — AUTOMATIC CUE INTELLIGENCE

## 11. Goal

Every analyzed track should be able to leave scanning with useful DJ cue points already prepared while preserving user edits.

Mixed In Key publicly describes up to eight automatically generated cue points aligned to musically useful parts of a track. ViiB should implement its own structure-aware generator using its existing beatgrid, energy and analysis stack.

---

## 12. Cue generation pipeline

### 12.1 Inputs

Use:

- beatgrid;
- downbeats;
- BPM;
- structural boundaries;
- energy curve;
- loudness;
- onset density;
- optional vocal activity from stems;
- optional drum activity from stems;
- optional local key timeline.

### 12.2 Candidate generation

Generate candidates at downbeat-aligned phrase boundaries.

Candidate types:

- first stable downbeat;
- intro start;
- first mix-safe phrase;
- first vocal entrance;
- first significant build;
- first drop/chorus;
- strongest later drop/chorus;
- breakdown;
- mix-out start;
- outro start;
- last safe phrase.

### 12.3 Candidate scoring

Each candidate gets a transparent score based on:

- section salience;
- downbeat confidence;
- phrase boundary confidence;
- energy delta;
- distance from adjacent chosen cues;
- usefulness for mixing;
- vocal/drum state when stems are available.

Avoid placing several cues inside the same short section unless the track genuinely demands it.

### 12.4 Eight-slot policy

Suggested initial slot policy:

1. Intro / first useful downbeat
2. Early mix-in phrase
3. First major musical section
4. First drop or chorus
5. Breakdown / reset
6. Second major drop or chorus
7. Mix-out phrase
8. Outro / final useful downbeat

This is a heuristic, not a hard semantic guarantee.

When a track lacks eight distinct sections, fill remaining slots using high-confidence phrase boundaries with minimum spacing.

### 12.5 Quantization

Default generated cues should snap to:

1. detected downbeat when confidence is strong;
2. otherwise nearest beat;
3. optional 1/4-beat editing resolution in the UI.

Manual movement should support:

- quantize off;
- 1 beat;
- 1/2;
- 1/4.

### 12.6 Re-analysis policy

Add Setting:

**Automatic DJ Cue Points**
- Off
- Suggest only
- Fill empty slots
- Refresh generated cues

Recommended default: Fill empty slots.

Rules:

- user cues are never replaced;
- generated cues may be replaced by newer generated cues;
- moving or renaming a generated cue makes it user-owned;
- deleting a generated cue can optionally create a do-not-regenerate tombstone for that slot/track.

---

## 13. Cue UI

### 13.1 Waveform markers

Generated and user cues need distinct provenance styling.

Example:

- user cue: solid marker;
- generated cue: solid marker plus small sparkle/AI-analysis icon;
- low-confidence generated cue: outlined marker.

Do not use an “AI” label if no AI model produced the cue. “Auto” or “Analyzed” is more accurate.

### 13.2 Cue editor

Add a compact cue drawer:

- slot;
- time;
- label;
- color;
- type;
- confidence;
- rationale;
- lock state.

Actions:

- Jump
- Rename
- Recolor
- Move to Playhead
- Snap
- Lock
- Regenerate
- Convert to Manual

### 13.3 Batch scan behavior

During library analysis, the normal analysis job should optionally create cues after structure/downbeat analysis.

Analysis stage order:

    decode
      -> tempo / beatgrid
      -> key
      -> energy / loudness
      -> structure
      -> generated cue candidates
      -> cue persistence policy

If stem activity is available later, a stem-enhanced cue pass may improve the generated cue set without touching manual cues.

---

# PART IV — CAMELOT-FIRST UX

## 14. Canonical key handling

Keep a canonical musical key in analysis, then derive display systems:

- Traditional: F minor
- Camelot: 4A
- Open Key
- optional custom notation

Pitch changes must transform the canonical key first, then derive the displayed Camelot code.

---

## 15. ViiB Camelot color system

The current library colors keys based on compatibility state, not on the Camelot wheel position itself.

Add a deterministic Camelot palette.

Recommended ViiB-owned palette:

| Camelot number | Base hue |
|---|---|
| 1 | #49D3C5 |
| 2 | #67CE7B |
| 3 | #A8CF62 |
| 4 | #E8CF5B |
| 5 | #F2B65A |
| 6 | #F38B83 |
| 7 | #EE6F9D |
| 8 | #DA63C1 |
| 9 | #B66FE1 |
| 10 | #8B82E3 |
| 11 | #6EA6DF |
| 12 | #50C5DF |

Use the same hue family for A and B and distinguish major/minor with tone, border or luminance rather than inventing a second unrelated hue.

This should be ViiB visual design, not an exact copy of Mixed In Key artwork.

### 15.1 Library chip

Replace plain key text with a compact colored chip:

    4A

Tooltip:

    F minor
    Camelot 4A
    Measured · 91% confidence

### 15.2 Camelot wheel popover

Add an optional wheel popover near the library filters.

Selecting a key filters the library to:

- exact key;
- compatible keys;
- optional extended moves.

### 15.3 Compatibility classes

Define explicit relations:

- exact;
- adjacent -1;
- adjacent +1;
- relative major/minor;
- energy boost +2;
- dramatic/custom relation;
- incompatible/unknown.

Recommendations should explain the relation rather than collapse everything into an unexplained percentage.

---

# PART V — ENERGY LEVEL 1-10

## 16. Why add a scalar Energy Level

ViiB already has a time-varying normalized energy curve. DJs also benefit from one sortable track-level number.

Add Energy Level 1-10 as a separate derived feature.

It must not simply be loudness divided into ten buckets.

---

## 17. Candidate features

Evaluate a weighted model using:

- integrated loudness;
- short-term loudness distribution;
- crest factor;
- onset density;
- spectral centroid/brightness;
- high-frequency energy;
- low-frequency rhythmic energy;
- drum activity;
- tempo;
- dynamic range;
- proportion of track at high normalized energy;
- drop/chorus peak intensity;
- optional vocal activity.

Normalize features over a lawful calibration corpus.

The output should include confidence and algorithm version.

### 17.1 Calibration

Acceptance requires:

- stable scores across Windows/macOS/Linux;
- no systematic assumption that faster BPM always means higher energy;
- genre-diverse listening review;
- reproducibility after re-scan;
- sensible ordering inside curated warmup/groove/peak-time test playlists.

Do not claim Mixed In Key Energy Level equivalence. ViiB Energy Level is its own model.

### 17.2 UI

Add:

- Energy column;
- sortable 1-10 badge;
- range filter;
- tiny energy curve preview on hover;
- direction filters: Lower / Hold / Lift.

---

# PART VI — SONG STRUCTURE AND LOCAL KEY

## 18. Structure analysis

ViiB currently has energy sections, but cue intelligence and mashup planning benefit from semantic structure.

### 18.1 V1

Implement deterministic phrase segmentation:

- novelty curve;
- onset/rhythm change;
- spectral change;
- energy change;
- downbeat-aligned boundaries.

Then use conservative heuristics to label obvious:

- intro;
- build;
- drop;
- breakdown;
- outro.

Unknown is acceptable.

### 18.2 V2 stem-assisted structure

When stems exist, add:

- vocal activity;
- drum density;
- bass activity;
- melodic activity.

This improves labels such as:

- vocal entrance;
- instrumental section;
- breakdown;
- drop;
- mix-safe intro/outro.

### 18.3 Optional model evaluation

StemDeck currently references an All-In-One-derived inference package for section labeling. It may be evaluated as an optional worker-side experiment.

Before adoption:

- verify code license;
- verify model-weight license;
- benchmark runtime;
- benchmark native packaging;
- confirm deterministic output;
- ensure it does not become a mandatory dependency for basic DJ analysis.

---

## 19. Local key timeline

Add a future analysis artifact for tracks whose harmony changes materially.

Store segment-level:

- start/end;
- key;
- confidence;
- Camelot code.

Uses:

- warn when a long blend crosses a key change;
- recommend a safer mix-out section;
- improve mashup vocal/instrumental matching;
- color structure segments by local harmonic center.

This is a later phase and must not block the first cue/stem release.

---

# PART VII — DJ MIX MODE

## 20. Upgrade transition recommendations

The current ViiB transition scorer primarily combines energy continuity, loudness match and phrase-preparation evidence.

Expand the recommendation vector with:

- Camelot relation;
- BPM delta;
- required tempo change percentage;
- Energy Level delta;
- tail/head energy;
- LUFS delta;
- phrase alignment;
- intro/outro compatibility;
- vocal overlap risk;
- stem availability;
- genre/playlist filters;
- recency/play-history filters when desired.

Every score component must remain inspectable.

---

## 21. Direction-aware recommendations

The DJ should choose intent:

- Hold
- Lift
- Reset
- Harmonic
- Surprise
- Vocal-safe

Examples:

**Hold**
- similar energy;
- compatible key;
- modest tempo change.

**Lift**
- Energy Level +1 or selected amount;
- compatible key or intentional energy-boost relation;
- incoming section with strong onset.

**Reset**
- lower energy;
- longer clean intro;
- reduced vocal collision.

The recommendation engine should not imply one universally correct next track.

---

## 22. Mix audition

Add **Test Mix**.

Behavior:

1. keep the currently selected track as Deck A reference;
2. load candidate on preview/Deck B;
3. jump outgoing deck to recommended mix-out region;
4. jump incoming deck to recommended mix-in region;
5. apply temporary beatmatch;
6. optionally apply temporary key shift;
7. play a short synchronized preview;
8. return both decks to their original states when preview ends.

For live performance safety, planning preview should preferably route to the headphone/cue bus rather than master by default.

---

## 23. Idea filters

Candidate filters:

- current playlist;
- selected playlists;
- genre;
- BPM range;
- Energy Level range;
- compatible Camelot only;
- stems available;
- instrumental-friendly;
- vocal-friendly;
- not recently played.

---

# PART VIII — MASHUP MODE

## 24. Stem-aware mashup suggestions

Mashup Mode should answer:

“What vocal from my library could work over this instrumental?”

And the reverse.

Scoring should consider:

- key compatibility after optional semitone shift;
- BPM compatibility;
- phrase length;
- vocal activity density;
- instrumental vocal-free windows;
- energy relationship;
- section duration;
- stem quality/availability.

---

## 25. Pitch shift

Add an explicit pitch-shift control separate from tempo.

Initial range:

- -6 to +6 semitones;
- optional expert range to +/-12.

When pitch changes:

- displayed musical key updates immediately;
- Camelot chip updates;
- recommendations recalculate;
- saved idea records the transposition.

Pitch shift requires the same production-grade time/pitch DSP evaluation described in the stem transport section.

---

## 26. Loop audition

Mashup planning should allow:

- select a cue or section;
- loop 1, 2, 4, 8, 16 or 32 beats;
- audition vocal stem over instrumental stem;
- move candidate section by phrase;
- save idea.

A saved mashup idea stores references and edits, not a rendered audio copy by default.

Suggested fields:

- source track A;
- source track B;
- selected stems;
- section/cue positions;
- tempo;
- semitone shifts;
- loop lengths;
- rating;
- notes.

---

# PART IX — LIBRARY PREPARATION FEATURES

## 27. Pro analysis columns

Add optional DJ library columns:

- BPM
- Camelot
- Traditional Key
- Energy
- Cue Count
- Stem Status
- Structure Status
- Analysis Confidence
- LUFS
- Date Analyzed

Keep columns virtualized and user-configurable.

---

## 28. Analysis inspector

Add a right-side inspector inspired by professional preparation tools, but using ViiB visual design.

Sections:

- Song Info
- Analysis
- Cue Points
- Stems
- Structure
- Transition Ideas

Editable fields:

- title;
- artist;
- album;
- genre;
- manual BPM override;
- manual key override;
- Energy Level override;
- comments.

Overrides require provenance and a Reset to Measured action.

---

## 29. Key verification keyboard

Low-priority professional feature:

- small 12-note keyboard;
- play reference notes/chords;
- highlight detected tonic/scale;
- allow manual key correction.

This is useful for low-confidence key results and does not require a proprietary algorithm.

---

# PART X — EXPORT AND INTEROPERABILITY

## 30. Stem export

Support:

- individual six stems;
- DJ four-group stems;
- acapella;
- instrumental;
- selected stem mix.

Export should preserve metadata where possible.

Later offline render can optionally apply:

- pitch shift;
- tempo;
- selected stem levels.

---

## 31. DJ metadata export

Future interoperability can include:

- key;
- Energy Level;
- comments;
- cue points;
- playlists.

Potential targets:

- Serato
- Rekordbox
- Traktor

Each target should be implemented only after format/version research and round-trip tests.

Do not overwrite third-party library databases directly without backups and explicit opt-in.

---

# PART XI — ROADMAP AND DEPENDENCIES

## 32. Parallel workstreams

The fastest route is three parallel tracks.

### Workstream A — Preparation intelligence

A1. Camelot color system  
A2. Scalar Energy Level 1-10  
A3. Structure timeline V1  
A4. Eight-cue generator  
A5. Scan-time fill-empty cue policy  
A6. Cue editor and waveform markers

### Workstream B — Stem platform

B1. Stem worker spike  
B2. Capability API  
B3. Durable stem jobs  
B4. Stem persistence/cache  
B5. Stem export/preview  
B6. DeckSource refactor  
B7. Stem transport worklet  
B8. Four-button DJ stem UI  
B9. Scratch/loop/sync/key-lock parity

### Workstream C — Mix intelligence

C1. Recommendation vector v2  
C2. Direction-aware Mix Next  
C3. Mix audition  
C4. Stem-aware vocal overlap  
C5. Mashup Mode  
C6. Pitch-shift-aware recommendations  
C7. Saved mix/mashup ideas

---

## 33. Proposed implementation phases

### Phase 0 — Contract and benchmark foundation

Deliver:

- architecture decision record for stem runtime;
- stem capability types;
- benchmark fixtures;
- worker protocol;
- output manifest format;
- queue/cancellation tests;
- memory and disk budget measurements;
- legal/license inventory.

Exit criteria:

- Windows, macOS and Linux worker strategy documented;
- at least one real track separates end to end in a development harness;
- cancellation leaves no GPU-holding process;
- source/stem frame counts validate.

### Phase 1 — Camelot UX and Energy Level

Deliver:

- deterministic Camelot palette;
- key chips;
- wheel/filter popover;
- Energy Level algorithm v1;
- Energy library column/filter;
- analysis versioning.

Exit criteria:

- no regression to existing key compatibility;
- color mapping unit tests for all 24 keys;
- Energy Level reproducible on supported platforms.

### Phase 2 — Structure and automatic cues

Deliver:

- structure artifact V1;
- eight-cue candidate generator;
- cue provenance migration;
- fill-empty policy;
- cue editor;
- scan-time cue generation.

Exit criteria:

- user cues survive reanalysis unchanged;
- every generated cue lands on valid track time;
- high-confidence cues quantize to expected beat/downbeat;
- generated cues are reproducible for identical analysis inputs.

### Phase 3 — Stem worker and cache

Deliver:

- managed worker prototype;
- htdemucs_6s;
- CPU/CUDA/MPS capability reporting;
- durable stem job;
- cancellation;
- stem-set persistence;
- disk cache settings;
- export/preview endpoints.

Exit criteria:

- complete six-stem output;
- restart-safe job state;
- corrupt/incomplete output never marked ready;
- cache invalidates when source audio hash changes.

### Phase 4 — DeckSource refactor

Deliver:

- source abstraction;
- existing single-track path moved behind SingleTrackDeckSource;
- regression tests for playback, seek, loop, cue, sync and scratch;
- no user-visible stem dependency yet.

Exit criteria:

- normal DJ behavior is functionally unchanged.

### Phase 5 — Stem playback MVP

Deliver:

- StemDeckSource;
- phase-aligned streaming prototype;
- VOCAL/DRUMS/BASS/MUSIC controls;
- FULL/ACAPELLA/INSTRUMENTAL;
- stem status badges;
- per-stem gain ramps.

Exit criteria:

- no audible transport drift;
- stem toggles do not click;
- switching full-track -> stems preserves position;
- both decks can use stems simultaneously on target hardware.

### Phase 6 — Stem performance parity

Deliver:

- production tempo/key-lock path;
- loops;
- beat sync;
- hot cues;
- scratch/slip;
- headphone cue;
- FX;
- recording/master path;
- stem VU and waveform modes.

Exit criteria:

- Stem Mode does not silently disable core DJ functions;
- any unsupported function is explicitly gated before release.

### Phase 7 — Mix Next v2

Deliver:

- harmonic/BPM/energy/structure recommendation components;
- Hold/Lift/Reset intent;
- playlist/genre filters;
- transition preview;
- explainable recommendation UI.

Exit criteria:

- recommendation explanations match the numeric score components;
- preview never permanently mutates deck state unless the user accepts the load.

### Phase 8 — Mashup Mode

Deliver:

- stem-aware pairing;
- vocal/instrumental candidate search;
- loop audition;
- pitch shift;
- key recalculation;
- saved ideas.

### Phase 9 — Interoperability and polish

Deliver:

- DJ metadata export;
- stem export presets;
- optional keyboard/key verification;
- advanced six-stem view;
- accessibility and keyboard mappings;
- controller/MIDI mapping hooks for stem pads.

---

# PART XII — ACCEPTANCE GATES

## 34. Stem audio quality

Required:

- no missing frames;
- no NaN/Inf samples;
- no unexpected duration mismatch;
- no audible stem desynchronization;
- no audible click from toggle ramps;
- deterministic start/seek alignment.

Recommended measured target:

- deck stem alignment error below 2 ms during transport tests;
- zero cumulative drift over a 10-minute test;
- seek alignment returns within one audio block after preroll.

The exact implementation may exceed these targets; they are intended as minimum regression gates.

---

## 35. Performance

Measure on:

- Windows CPU-only;
- Windows NVIDIA GPU;
- Apple Silicon MPS;
- Linux CPU;
- Linux NVIDIA GPU where available.

Record:

- model startup time;
- separation wall time;
- real-time factor;
- peak RAM;
- peak VRAM;
- output disk size;
- two-deck stem playback CPU;
- AudioWorklet underruns;
- UI frame drops.

Background stem jobs should yield to active DJ playback where resource contention is measurable.

Reuse the existing analysis-pressure concept for heavy stem processing.

---

## 36. Cue quality

Maintain a hand-reviewed lawful cue corpus.

For each test track record reference regions such as:

- intro;
- first drop;
- breakdown;
- outro;
- mix-in window;
- mix-out window.

Metrics:

- boundary distance in beats;
- downbeat hit rate;
- duplicate/too-close cue rate;
- invalid cue rate;
- user acceptance rate during manual review.

Do not score the system solely on whether it matches another vendor's cue locations.

---

## 37. Energy quality

Create curated ordered groups:

- chill;
- warmup;
- groove;
- peak;
- reset.

Evaluate pairwise ordering and intra-genre consistency.

The scalar Energy Level is a navigation aid, not an objective truth.

---

# PART XIII — TEST PLAN

## 38. Backend tests

Add tests for:

- stem manifest validation;
- source hash invalidation;
- durable job restart;
- cancellation;
- disk-cache LRU;
- API capability states;
- generated cue overwrite policy;
- user cue preservation;
- Energy Level determinism;
- structure serialization;
- recommendation score components.

---

## 39. Frontend tests

Add tests for:

- all 24 Camelot colors;
- key transposition;
- stem button state;
- solo/full/instrumental modes;
- generated/manual cue styling;
- cue editor operations;
- Mix Next filters;
- preview state restoration;
- fallback to full-track source after stem failure.

---

## 40. Audio-engine integration tests

Build deterministic synthetic stem fixtures where the sum of stems reconstructs the source.

Test:

- start;
- pause/resume;
- seek;
- 1/2/4/8-beat loops;
- stem mute/unmute;
- solo;
- tempo;
- key lock;
- crossfader;
- headphone cue;
- track reload;
- deck unload;
- scratch entry/exit;
- slip;
- underrun recovery.

A reconstruction fixture is particularly important: with all stem gains at unity, the stem mix should remain numerically close to the expected combined source after accounting for separation/output encoding.

---

# PART XIV — UX DETAILS

## 41. Recommended deck layout

Do not add a large permanent stem panel that makes the deck taller.

Recommended compact row below hot cues or performance pads:

    FULL | VOCAL | DRUMS | BASS | MUSIC | STEMS…

The advanced six-stem mixer opens as a popover/drawer.

This is consistent with the existing DJv2 effort to avoid layout shifts.

---

## 42. Library row example

Suggested compact cells:

    126   4A   E6   C8   STEM ✓

Where:

- 126 = BPM
- 4A = Camelot chip
- E6 = Energy Level 6
- C8 = eight cues available
- STEM ✓ = stems ready

Tooltips provide analysis confidence.

---

## 43. Waveform overlays

Recommended layers:

1. waveform;
2. beatgrid/downbeats;
3. structure bands;
4. cue markers;
5. loop region;
6. playhead.

Avoid rendering every analysis detail at full opacity.

Structure can use subtle background bands and labels only when zoomed out enough to fit them.

---

# PART XV — LICENSING AND PRODUCT BOUNDARIES

## 44. StemDeck

StemDeck is Apache-2.0 according to its repository.

Preferred approach:

- use the project as an architectural reference;
- reuse small components only where it meaningfully accelerates delivery;
- retain required Apache notices for copied/adapted code;
- maintain a third-party notices inventory.

### 44.1 Demucs

Demucs is open source, but model/runtime packaging and all transitive licenses still require release review.

### 44.2 FFmpeg

StemDeck documents using an external FFmpeg executable.

ViiB should not inherit StemDeck's exact FFmpeg packaging without an explicit licensing/release decision.

The first ViiB stem path can accept source formats the managed worker can decode directly or receive a prepared PCM file from ViiB.

---

## 45. Mixed In Key

Do not copy:

- proprietary algorithms;
- proprietary assets;
- exact UI artwork;
- exact visual trade dress;
- undocumented internal file formats.

Safe inspiration includes publicly described workflow concepts such as:

- automatic cue preparation;
- Energy Level-style sorting;
- harmonic matching;
- DJ mix planning;
- mashup auditioning;
- stem controls.

ViiB should name and implement its own scoring systems where appropriate.

---

# PART XVI — FILE-BY-FILE IMPLEMENTATION MAP

## 46. Existing files likely to change

### Frontend

- lib/djAudio.ts
  - DeckSource abstraction
  - StemDeckSource integration
  - stem bus routing

- hooks/useDJAudioEngine.ts
  - stem load lifecycle
  - capability state
  - deck stem actions

- slices/djMixerSlice.ts
  - per-deck stem state
  - solo/mute/levels
  - generated cue provenance if deck-local state needs it

- components/dj/v2/
  - DJDeck*
  - DJWaveform*
  - DJLibraryBrowserV2.tsx
  - DJEnergyInsights.tsx
  - new DJStemControls.tsx
  - new DJCueEditor.tsx
  - new DJMixNext.tsx
  - new DJMashupPlanner.tsx
  - new CamelotChip.tsx
  - new CamelotWheelPopover.tsx

- services/api.ts
  - typed stem/cue/structure/energy endpoints

### Backend

- backend/internal/api/api.go
  - register stem routes

- backend/internal/api/v2_jobs*.go
  - stem job integration

- backend/internal/db/
  - migrations and persistence

- backend/internal/analysis/features/
  - Energy Level
  - structure
  - recommendation v2

- backend/internal/dj/
  - any server-side DJ recommendation aggregation

---

## 47. New files likely to be created

Suggested names:

    backend/internal/stems/worker.go
    backend/internal/stems/capabilities.go
    backend/internal/stems/manifest.go
    backend/internal/stems/storage.go
    backend/internal/stems/validate.go

    backend/internal/analysis/cues/generator.go
    backend/internal/analysis/cues/policy.go
    backend/internal/analysis/structure/analyzer.go
    backend/internal/analysis/energy/level.go

    components/dj/v2/DJStemControls.tsx
    components/dj/v2/DJCueEditor.tsx
    components/dj/v2/DJMixNext.tsx
    components/dj/v2/DJMashupPlanner.tsx
    components/dj/v2/CamelotChip.tsx
    components/dj/v2/CamelotWheelPopover.tsx

    lib/stemTransport.worklet.js
    lib/camelotColors.ts
    lib/djPitch.ts

---

# PART XVII — FIRST IMPLEMENTATION SLICES

## 48. Recommended first PRs

Keep early PRs small and independently reviewable.

### PR 1 — Camelot visual system

- CamelotChip
- ViiB palette
- unit tests
- DJ library integration
- no analysis changes

### PR 2 — Cue provenance model

- database migration
- API type extension
- backward compatibility
- no generator yet

### PR 3 — Auto-cue generator V1

- consume existing beatgrid/energy features
- generate up to eight cues
- fill-empty policy
- tests

### PR 4 — Energy Level V1

- scalar 1-10
- versioned persistence
- column/filter

### PR 5 — Stem worker contract

- capabilities
- health
- local development worker
- no DJ playback yet

### PR 6 — Durable stem job

- operation_jobs integration
- progress/cancel
- output validation
- stem metadata persistence

### PR 7 — DeckSource refactor

- preserve existing behavior only

### PR 8 — Stem preview/export

- prove files and APIs before real-time deck integration

### PR 9 — Stem deck transport

- worklet/frame streaming
- no advanced UI yet

### PR 10 — Four-button stem UI

- Vocal/Drums/Bass/Music
- Full/Acapella/Instrumental
- stem badge

This sequence reduces the risk of attempting UI, ML packaging, transport and database changes in one unreviewable branch.

---

# PART XVIII — DEFINITION OF DONE

## 49. Professional stem release

Stem functionality is release-ready only when:

- six stems can be generated locally;
- worker/model/device status is transparent;
- jobs survive/recover correctly;
- cancellation is reliable;
- storage is bounded;
- four DJ stem groups are sample aligned;
- seek/loop/sync remain aligned;
- Key Lock behavior is truthful;
- stem mode works on both decks;
- fallback to original audio is reliable;
- user-facing errors are actionable;
- normal DJ mode remains unchanged when stems are disabled.

---

## 50. Professional preparation release

Cue/Energy/Camelot preparation is release-ready only when:

- generated cues never destroy manual cues;
- cues are beat/downbeat aligned where evidence supports it;
- cue provenance is visible;
- all 24 Camelot keys have deterministic colors;
- Energy Level is versioned and reproducible;
- library sort/filter is fast on large catalogs;
- analysis can be re-run safely.

---

## 51. Professional Mix Planning release

Mix Planning is release-ready only when:

- recommendations combine BPM, key, energy and structure;
- score explanations are visible;
- user can choose musical intent;
- transition preview is reversible;
- candidate filtering works on playlists/genres;
- no recommendation is presented as objectively correct.

---

# PART XIX — HIGH-VALUE FOLLOW-ONS

After the core roadmap lands, high-value extensions include:

- MIDI/controller mapping for stem pads;
- stem-specific FX sends;
- vocal echo-out button;
- drum-only loop roll;
- stem-aware automatic transition routines;
- local key-change timeline;
- 32-beat phrase grid;
- phrase-aware beat jump;
- transition history and saved set planning;
- set-level Energy Level curve;
- set harmonic-path visualization;
- playlist optimizer using constraints rather than opaque ranking;
- offline render of a saved mashup idea;
- local recording of the master output;
- external controller LED feedback;
- per-stem waveform coloring;
- batch stem generation for a playlist;
- “generate stems for next N tracks” preflight before a gig.

---

# 52. Recommended product priority

For the next DJv2 development cycle, prioritize in this order:

1. **Automatic cue points + provenance**
2. **Camelot visual system**
3. **Energy Level 1-10**
4. **Stem worker + durable cache**
5. **Four-group stem deck playback**
6. **Mix Next v2**
7. **Pitch-shift/mashup planning**
8. **Third-party DJ export**

This order produces visible DJ-preparation value early while the more difficult real-time stem transport work proceeds behind a stable architecture.

The most important engineering constraint is that stem separation itself is not the hardest part. Demucs already solves the offline separation problem. The difficult product work is making stems behave like a first-class DJ source while preserving one transport clock, bounded memory, tempo/key-lock quality, scratch/loop behavior and the rest of ViiB's existing mixer.
