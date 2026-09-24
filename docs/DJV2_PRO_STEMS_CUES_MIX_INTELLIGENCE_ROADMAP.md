# DJv2 Pro Stems, Cue Intelligence & Mix Planning Roadmap

**Status:** Proposed implementation roadmap  
**Scope:** DJv2 only; extends, but does not replace, DJV2_PROFESSIONAL_TRACK_ANALYSIS_ROADMAP.md  
**Repository snapshot reviewed:** main at d02ad01dd5d384a77383b70f2669f9dd1c6c1761 (v1.0.0-rc3)  
**Research snapshot:** 2026-09-24  
**Primary goals:** professional-grade stem playback, scan-time cue creation, Camelot-first library UX, 1-10 energy analysis, structure-aware transition planning, mashup auditioning, and high-quality DJ preparation workflows.  
**Stem-generation architecture decision:** stem generation is an ahead-of-time workflow owned by a separate future application/repository named **ViiB-StemLab**. ViiB MediaHub detects, validates, indexes and plays pre-generated stem packages; it does not embed Demucs/PyTorch or perform neural stem separation during DJ playback. The ViiB-StemLab repository will be created later, after the package contract in this roadmap is stable.

> This roadmap uses public behavior from Mixed In Key 11 Pro as product inspiration and StemDeck as an open-source implementation reference. It does not attempt to reproduce proprietary Mixed In Key algorithms or visual trade dress. Where StemDeck code is reused directly, Apache-2.0 attribution and third-party notices must be preserved.

---

## 1. Product vision

DJv2 should evolve from a capable two-deck player into a local-first DJ preparation and performance environment.

The target experience is:

1. Import or scan music once.
2. Analyze BPM, beatgrid, key, Camelot/Open Key, loudness, energy, song structure and up to eight useful cue points.
3. Prepare stems ahead of time with the separate **ViiB-StemLab** application (future repository), or import a compatible ViiB Stem Package generated elsewhere; MediaHub detects when a valid package is available.
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
- a current streaming loudness proxy stored as `IntegratedLUFS` and a peak-level proxy stored as `TruePeakDBFS` (these names must not be interpreted as full BS.1770 integrated loudness / oversampled true-peak compliance yet);
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

The backend already has a persisted long-running job system, auto-analysis triggers and a DJ playback pressure signal. That scheduler remains appropriate for MediaHub-owned analysis, discovery and validation work, but **stem generation itself is no longer a MediaHub job**.

The boundary is:

- **ViiB-StemLab** owns model/runtime management, GPU/CPU selection, generation queues, progress, cancellation and writing completed stem packages.
- **ViiB MediaHub** owns stem-package discovery, validation, indexing, source-hash matching and DJ playback.
- An optional future integration may let MediaHub launch or deep-link into ViiB-StemLab, but MediaHub must remain fully functional when ViiB-StemLab is not installed.

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


### 3.3 Open-source MIR reference stack and current ViiB evidence

The current ViiB analysis stack is already strong enough that these projects should be treated as **reference analyzers and targeted capability candidates**, not automatic replacements for the native Go implementation.

**Beat This! (CPJKU)**

- predicts timestamped beats and downbeats with a transformer-based model;
- code and published model weights are MIT licensed;
- supports CPU/GPU inference and exposes framewise beat/downbeat activations;
- is particularly relevant to ViiB because true musical downbeat inference is a larger current gap than scalar BPM estimation.

ViiB has already run the `final0` model as a development-only reference. Deriving scalar BPM from a fixed 16-beat median over its timestamp grid reached **111/122 strict BPM matches (90.98%)** on the r5 tuning set. That result is **diagnostic only**: the repository's overlap audit found tracks in the ViiB corpus that also occur in Beat This training material. Do not use that result as independent release evidence or promote the model without a clean, overlap-audited evaluation.

Reference:
https://github.com/CPJKU/beat_this

**All-In-One / All-In-One-Infer**

The All-In-One research family jointly predicts:

- tempo;
- beats;
- downbeats;
- functional segment boundaries;
- functional labels such as intro, verse, chorus, bridge and outro.

This is unusually well aligned with ViiB's cue-intelligence problem because one model can provide both rhythmic landmarks and semantic structure. The original project is MIT licensed. A maintained packaging fork, `all-in-one-infer`, was renamed at v3.0.0 in July 2026 and removes several older installation barriers while retaining the upstream analysis behavior.

References:

- https://github.com/mir-aidj/all-in-one
- https://github.com/openmirlab/all-in-one-infer

**BeatNet**

BeatNet jointly estimates beats, downbeats, tempo and meter and remains useful as an independent rhythmic reference. Its repository is CC-BY-4.0 and its offline path carries a Python/PyTorch/madmom-style dependency footprint, so it is a weaker default packaging fit for MediaHub than a native implementation. It is still valuable for benchmark diversity because it uses a different CRNN/particle-filtering approach.

Reference:
https://github.com/mjhydri/BeatNet

**Essentia**

Essentia remains a valuable MIR laboratory and reference implementation, but it is AGPLv3 and should stay outside the shipping MediaHub dependency graph unless a separate licensing decision is made.

ViiB has already benchmarked Essentia `RhythmExtractor2013` as an isolated development process. Its production-aligned tuning configuration reached **99/122 strict BPM (81.15%)**, but only **39/54 strict (72.22%)** on the independent r5 held-out subset. That evidence does not justify replacing ViiB's native tempo engine with Essentia.

Reference:
https://github.com/MTG/essentia

### 3.4 Research conclusion: improve weak dimensions, do not replace the stack wholesale

The recommended direction is:

1. keep ViiB's native Go tempo, key, persistence, scheduler, provenance and DJ workflow as the production foundation;
2. improve **true beat/downbeat/meter inference** before relying heavily on phrase-aligned automatic cues;
3. evaluate **semantic structure models** separately from the existing energy-novelty sections;
4. keep Essentia, Beat This, BeatNet and All-In-One behind benchmark/reference adapters until an explicit promotion gate is passed;
5. if a neural model is ever promoted to shipping use, make packaging/runtime/licensing a separate architecture decision rather than silently adding PyTorch to MediaHub.


# PART I — TARGET ARCHITECTURE

## 4. Architectural principles

### 4.1 Preserve the current Go/Wails core

The ViiB backend remains the source of truth for:

- songs;
- playlists;
- analysis artifacts;
- jobs owned by MediaHub;
- discovered stem-package metadata;
- user cues;
- recommendation data;
- configuration.

MediaHub must **not** ship or embed the stem-generation ML runtime. Demucs/PyTorch, model downloads, GPU runtime handling and separation queues belong to the separate ViiB-StemLab application.

### 4.2 Separate the generation domain from the performance domain

Stem separation is an ahead-of-time preparation workflow.

**ViiB-StemLab** will be a separate application and future repository. Its responsibilities will be:

- select and manage stem models;
- select CUDA, Apple MPS or CPU execution;
- queue and cancel separation jobs;
- validate generated audio;
- write a versioned ViiB Stem Package;
- support batch preparation of tracks/playlists;
- optionally expose a CLI or launch/deep-link contract for MediaHub.

**ViiB MediaHub** is a stem consumer. Its responsibilities are:

- discover stem packages;
- verify schema and source-audio identity;
- index stem availability;
- detect stale or invalid packages;
- stream/decode stems for DJ playback;
- expose stem controls, waveforms, loops, sync, scratch and FX.

The core MediaHub experience must remain fully usable when ViiB-StemLab is not installed.

MediaHub package states should include:

- none;
- discovered;
- validating;
- ready;
- stale;
- invalid;
- unavailable.

ViiB-StemLab generation states are intentionally outside the MediaHub state machine and will be defined in the future ViiB-StemLab repository.

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

If stem files are missing, corrupt, stale or incompatible, the deck must fall back to the original track without changing the musical position.

### 4.6 Use a filesystem package contract as the integration boundary

The integration contract between ViiB-StemLab and ViiB MediaHub is a versioned **ViiB Stem Package**, not a Python API, local HTTP service or shared database.

A package contains:

- a machine-readable `manifest.json`;
- source-track identity including a cryptographic source hash;
- generator/model/version metadata;
- sample rate, channel and frame-count metadata;
- checksums for every stem;
- the generated audio files.

This keeps MediaHub independent from the implementation language or ML framework used by ViiB-StemLab and also allows future third-party generators to produce compatible packages.

---


### 4.7 Native-first analysis with swappable reference providers

The native Go analyzer remains authoritative unless a replacement wins a documented quality gate.

External/reference analyzers should use a narrow normalized result contract rather than writing directly to production tables. Suggested development result fields:

- analyzer name/version/model;
- source audio hash;
- canonical sample rate/input identity;
- beats[];
- downbeats[];
- meter when known;
- BPM and confidence when supplied;
- structure boundaries/labels when supplied;
- per-output confidence when available;
- runtime and device metadata.

Development adapters should emit benchmark artifacts first. Promotion to production requires a separate decision covering accuracy, deterministic behavior, cross-platform runtime, packaging, model-weight licensing and rollback/fallback behavior.

### 4.8 Canonical audio timing is mandatory for rhythm/structure comparisons

Beat and downbeat evaluation is sensitive to decoder offsets. External tools must not independently decode MP3/AAC and then be compared naively against ViiB timestamps.

For benchmark and optional companion analysis:

1. resolve the source through ViiB;
2. decode once through the canonical ViiB path where practical;
3. provide normalized PCM or a deterministic temporary WAV to the reference analyzer;
4. record any resampling delay or trim explicitly;
5. map returned timestamps back to the canonical ViiB track timeline before scoring or persisting them.

This avoids tens-of-milliseconds decoder-origin shifts being misdiagnosed as beat-tracker error.


## 5. Proposed backend components

Add the following MediaHub components:

    backend/internal/stems/
        manifest.go
        discovery.go
        registry.go
        resolver.go
        validation.go

    backend/internal/api/
        v2_stems.go

    backend/internal/db/
        stem registry/persistence additions

    backend/internal/analysis/cues/
        generator.go
        policy.go

    backend/internal/analysis/energy/
        level.go

    backend/internal/analysis/structure/
        structure.go

MediaHub does **not** contain a Python worker, model runtime or separation job implementation.

### 5.1 Future ViiB-StemLab repository

A separate repository will be created later for **ViiB-StemLab**. That repository is expected to contain the heavy stem-generation stack, potentially including:

    ViiB-StemLab/
        app/
        cli/
        stem_engine/
        models/
        packaging/
        tests/

The exact implementation language/runtime is intentionally not part of the MediaHub contract. The initial implementation may use Python/PyTorch/Demucs, while a future version could use ONNX or another engine without requiring changes to DJ playback.

The stable integration surface is the ViiB Stem Package schema.

### 5.2 Optional application-to-application integration

A future ViiB-StemLab release may expose:

- `viib-stemlab generate <track>`;
- `viib-stemlab prepare-playlist <manifest>`;
- a desktop deep link/protocol;
- a completion notification or package-directory rescan trigger.

These are convenience integrations only. MediaHub must always be able to discover a completed package by filesystem scan without requiring a running StemLab process.

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
- generator_name
- generator_version
- stem_layout
- status
- sample_rate
- channels
- duration_seconds
- generated_device
- package_path
- manifest_schema_version
- generated_at
- validation_error_code
- validation_error_message
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


### 6.6 Rhythm-grid provenance and musical-downbeat semantics

The current beatgrid representation is useful and should be preserved, but future analysis must distinguish:

- **beat phase**: where the repeating beat pulse falls;
- **musical downbeat**: which beat is bar position 1;
- **meter**: how beats group into bars;
- **phrase boundary**: higher-level 4/8/16/32-beat musical grouping.

A phase-aligned straight grid that marks every fourth beat as a downbeat is not equivalent to learned musical-downbeat detection.

Extend rhythm artifact metadata with:

- detector / model / algorithm version;
- beat confidence or aggregate beat confidence;
- downbeat confidence;
- meter and meter confidence;
- whether downbeats are `measured`, `inferred-from-meter`, or `manual`;
- source audio hash;
- edited/locked state;
- optional local-tempo anchors.

Cue and phrase logic must be able to tell the difference between a true measured downbeat and a meter-derived placeholder.


## 7. Proposed API surface

### 7.1 Stem locations and discovery

GET /api/v2/stems/locations

POST /api/v2/stems/rescan

Configured locations may include:

- adjacent `.stems` directories next to source tracks;
- one or more global ViiB Stem Libraries;
- explicitly linked package paths.

MediaHub scans package manifests rather than treating every stem audio file as an independent library item.

### 7.2 Track stem state

GET /api/v2/tracks/{id}/stems

Returns:

- status;
- active stem set;
- available stems;
- source-hash match state;
- generator/model/version;
- package path;
- disk usage;
- generatedAt;
- validation error information.

### 7.3 Link or refresh a completed package

POST /api/v2/tracks/{id}/stems/link

Body:

    {
      "packagePath": "D:/ViiB Stems/...",
      "replace": false
    }

POST /api/v2/tracks/{id}/stems/refresh

These endpoints validate/index completed packages. They do **not** perform separation.

### 7.4 Unlink or delete a package

DELETE /api/v2/tracks/{id}/stems/{stemSetId}

The default action should unlink the package from MediaHub. Physical deletion of generated stem files must be a separate explicit action and must be refused while either deck is actively using the set.

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

# PART II — STEM PACKAGE GENERATION & INTEGRATION

## 8. ViiB-StemLab architecture

### 8.1 Separate application and future repository

Stem generation will be implemented in a separate application named **ViiB-StemLab**.

The repository is intentionally **not being created as part of this roadmap update**. Once the ViiB Stem Package v1 contract is stable, create a dedicated repository, expected to be named:

    ajbergh/ViiB-StemLab

ViiB-StemLab is a preparation tool, not part of the live DJ audio path.

Its primary responsibilities:

- generate stems ahead of performance;
- batch-process folders and DJ playlists;
- manage model downloads and versions;
- select CPU/CUDA/MPS execution;
- display generation progress and errors;
- cancel/retry jobs;
- validate output;
- write completed ViiB Stem Packages.

MediaHub never requires ViiB-StemLab to remain running during DJ playback.

### 8.2 Initial model candidate

Use Demucs `htdemucs_6s` as the first production candidate for ViiB-StemLab because it provides:

- vocals;
- drums;
- bass;
- guitar;
- piano;
- other.

StemLab should auto-select:

1. CUDA when a supported NVIDIA runtime is available;
2. MPS on supported Apple Silicon;
3. CPU fallback.

This device/model information is written into the package manifest for provenance, but MediaHub does not need the generation runtime in order to play the package.

### 8.3 ViiB Stem Package v1

Recommended directory shape:

    <package-id>.viibstems/
        manifest.json
        vocals.flac
        drums.flac
        bass.flac
        guitar.flac
        piano.flac
        other.flac

The package may use WAV during early development, but FLAC should be evaluated as the default cache format because six uncompressed PCM files have substantial disk cost.

Minimum manifest information:

    {
      "schemaVersion": 1,
      "source": {
        "filename": "Human.flac",
        "sha256": "...",
        "duration": 355.21
      },
      "generator": {
        "name": "ViiB-StemLab",
        "version": "..."
      },
      "model": {
        "name": "htdemucs_6s",
        "version": "..."
      },
      "audio": {
        "sampleRate": 44100,
        "channels": 2,
        "frames": 15664861
      },
      "stems": {
        "vocals": "vocals.flac",
        "drums": "drums.flac",
        "bass": "bass.flac",
        "guitar": "guitar.flac",
        "piano": "piano.flac",
        "other": "other.flac"
      }
    }

The final schema must additionally include per-stem checksum/size/frame metadata and a schema-level compatibility policy.

### 8.4 Package finalization and validation

ViiB-StemLab writes into a temporary package directory and only atomically promotes it to the final `.viibstems` directory after generation succeeds.

Before finalization, StemLab validates:

- all required files exist;
- all files have identical sample rate;
- all files have identical frame count within tolerance;
- all files have compatible channel count;
- no file is empty;
- duration matches the source within tolerance;
- samples are finite;
- output does not contain catastrophic clipping;
- checksums are recorded.

MediaHub independently validates the manifest and critical audio geometry when it discovers the package. A package is never trusted merely because its directory exists.

### 8.5 Discovery locations

MediaHub should support both adjacent and centralized packages.

**Adjacent**

    Music/
        Human.flac
        Human.stems/
            manifest.json
            ...

**Central library**

    D:/ViiB Stems/
        <package-id>.viibstems/
            manifest.json
            ...

Resolution priority:

1. explicitly linked package;
2. adjacent stem package;
3. configured Stem Library locations;
4. optional MediaHub-managed imported package location.

### 8.6 Generation queue and cancellation belong to StemLab

Queue management, retries, progress, device utilization and cancellation are ViiB-StemLab responsibilities.

Cancellation must:

- terminate active model work;
- release GPU memory;
- remove temporary output;
- never expose an incomplete package as finalized.

MediaHub does not mirror this queue into its own durable job scheduler.

### 8.7 Optional integrated launch

MediaHub may later expose:

    Generate Stems

If ViiB-StemLab is installed, the action may launch/deep-link to StemLab with the selected track or playlist.

If StemLab is absent, MediaHub should show that stem generation requires the optional ViiB-StemLab application rather than presenting stem playback as broken.

### 8.8 Storage policy

Stem files are large, but storage ownership is clearer with a separate generator.

ViiB-StemLab should provide:

- output library location;
- estimated output size;
- batch storage requirements;
- cleanup tools;
- optional least-recently-used policies for its managed library.

MediaHub should provide:

- configured Stem Library locations;
- package index size/status;
- stale-package detection;
- unlink operations;
- explicit delete only when requested.

MediaHub must never silently delete externally generated packages.

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

The preferred production design is a single AudioWorklet-based stem transport backed by frame-aligned PCM chunks read from a validated ViiB Stem Package and served by the Go backend.

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


### 11.1 Rhythm prerequisite: beat phase is not the same as bar-one detection

The existing ViiB beatgrid is already a good persistence and playback contract, but the current native detector primarily estimates global tempo plus beat phase. It then constructs a straight grid and assigns downbeat indices from the configured meter.

That means the system can have an accurately phased beat grid while still choosing the wrong musical bar-one offset.

This matters for:

- automatic cue placement;
- 8/16/32-beat phrase grids;
- intro/drop/breakdown labeling;
- transition alignment;
- stem-aware mashups;
- automatic loop suggestions.

Before calling a generated cue "downbeat aligned", the cue engine must know the downbeat provenance.

Recommended hierarchy:

1. manually confirmed/locked downbeat;
2. qualified learned downbeat detector;
3. qualified deterministic musical-downbeat detector;
4. meter-derived grid position with reduced confidence;
5. nearest beat when no downbeat evidence is reliable.

The first reference candidate to benchmark for this gap is **Beat This!** because it directly emits beats and downbeats and has already shown strong diagnostic tempo evidence in the ViiB harness. **All-In-One** is a second high-value candidate because it jointly emits beats, downbeats and functional structure.

The native Go path remains the fallback until an external/native candidate passes the release gate.


## 12. Cue generation pipeline

### 12.1 Inputs

Use:

- beatgrid;
- downbeats **with provenance/confidence**;
- meter/bar position when reliably measured;
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

1. manually confirmed or qualified measured downbeat when confidence is strong;
2. otherwise nearest beat, with lower cue confidence;
3. never promote a meter-derived placeholder to "measured downbeat" in rationale/provenance;
4. optional 1/4-beat editing resolution in the UI.

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

### 17.0 Fix loudness semantics before making it a major Energy input

The existing streaming feature is useful as a loudness proxy, but a future field presented as standards-compliant integrated LUFS should implement the required K-weighting and gating behavior from ITU-R BS.1770 / EBU-style measurement. A user-facing "true peak" claim should likewise use an oversampled inter-sample peak detector rather than ordinary sample peak.

Until that work lands:

- keep the existing proxy versioned;
- do not market it as broadcast-compliant integrated loudness or true peak;
- avoid giving the proxy disproportionate weight in Energy Level calibration.

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
- optional vocal activity;
- optional learned arousal/danceability descriptors if they independently improve held-out DJ ordering;
- stem-aware drum/bass activity when validated stem packages exist.

Normalize features over a lawful calibration corpus.

Do **not** make the absolute 1-10 value depend only on the user's current library distribution. A score that changes when unrelated tracks are added is poor durable metadata. Calibrate the absolute score against a fixed reference corpus; if useful, expose a separate library percentile as UI-only context.

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

### 18.3 Model evaluation matrix

Do not evaluate structure in isolation from rhythm. The most useful candidates produce timestamps that can directly strengthen the cue pipeline.

**Primary semantic-structure candidate: All-In-One / All-In-One-Infer**

Evaluate:

- beat F-measure;
- downbeat F-measure;
- bar-position/meter usefulness;
- segment-boundary precision/recall/F1;
- functional-label macro-F1;
- cue-quality lift when its output feeds the ViiB cue generator;
- runtime on CPU and available accelerators;
- cross-platform install/package burden.

All-In-One is particularly attractive because it jointly predicts beats, downbeats, boundaries and labels. The maintained `all-in-one-infer` packaging should be evaluated alongside upstream rather than assuming the older install constraints still apply.

**Primary beat/downbeat candidate: Beat This!**

Evaluate its beat/downbeat timestamps independently of scalar BPM. The key question is whether it materially improves ViiB's musical bar-one and phrase alignment on an overlap-audited corpus.

**Secondary rhythmic reference: BeatNet**

Use as an independent architecture reference for beat/downbeat/tempo/meter, not as a presumed production dependency.

**DSP reference: Essentia**

Keep as a benchmark/reference implementation. Existing ViiB evidence does not justify promoting its BPM path over the native Go implementation, and AGPL licensing makes direct shipping a separate decision.

Before any model adoption:

- verify code license and model-weight license separately;
- audit evaluation tracks against known/public model training corpora;
- benchmark runtime and memory;
- confirm deterministic/repeatable output within declared tolerances;
- standardize input timing through canonical PCM/WAV;
- ensure failure falls back to the native ViiB path;
- require measurable downstream improvement to cue/downbeat/structure quality, not merely an attractive demo.

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

A0. Reference-adapter contract and overlap-audited rhythm/structure benchmark  
A1. True beat/downbeat/meter provenance and rhythm-grid v2 semantics  
A2. Camelot color system  
A3. Standards-correct loudness upgrade + Scalar Energy Level 1-10  
A4. Structure timeline V1  
A5. Eight-cue generator  
A6. Scan-time fill-empty cue policy  
A7. Cue editor and waveform markers

### Workstream B — Stem platform

B1. ViiB Stem Package v1 specification  
B2. Package discovery and resolver  
B3. Manifest/source-hash validation  
B4. Stem registry persistence and Stem Library locations  
B5. Completed-package preview/audio serving  
B6. DeckSource refactor  
B7. Stem transport worklet  
B8. Four-button DJ stem UI  
B9. Scratch/loop/sync/key-lock parity  
B10. Future ViiB-StemLab repository: generator UI/CLI, htdemucs_6s, batch queue and package writer

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

### Phase 0 — Stem package contract and benchmark foundation

Deliver:

- architecture decision record documenting ViiB-StemLab as a separate application/repository;
- ViiB Stem Package v1 schema;
- adjacent and centralized discovery rules;
- source-hash identity rules;
- deterministic synthetic package fixtures;
- manifest/audio validation rules;
- compatibility/versioning policy;
- storage budget measurements;
- third-party generator import policy.

Exit criteria:

- MediaHub can discover and validate a package without any stem-generation runtime installed;
- the same package fixture resolves on Windows, macOS and Linux;
- stale source-hash packages are rejected or marked stale;
- source/stem frame counts validate;
- the v1 schema is stable enough to implement in the future ViiB-StemLab repo.

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

### Phase 2 — Rhythm intelligence, structure and automatic cues

Deliver:

- canonical reference-adapter schema for beat/downbeat/structure experiments;
- Beat This development benchmark adapter;
- All-In-One / All-In-One-Infer development benchmark adapter;
- overlap audit against known model training corpora;
- rhythm-grid provenance that distinguishes measured/manual downbeats from meter-derived placeholders;
- structure artifact V1;
- eight-cue candidate generator;
- cue provenance migration;
- fill-empty policy;
- cue editor;
- scan-time cue generation.

Exit criteria:

- user cues survive reanalysis unchanged;
- every generated cue lands on valid track time;
- "downbeat-aligned" cues use measured/manual downbeats rather than an unlabeled meter-derived assumption;
- high-confidence cues quantize to expected beat/downbeat;
- reference-model benchmark results clearly separate tuning, held-out, and training-overlap-contaminated material;
- generated cues are reproducible for identical analysis inputs.

### Phase 3 — MediaHub stem package registry

Deliver:

- configured Stem Library locations;
- adjacent-package discovery;
- manifest parser and schema validation;
- source-hash matching and stale detection;
- stem-set persistence;
- explicit link/unlink workflow;
- package audio/preview endpoints;
- library stem-status badges.

Exit criteria:

- valid six-stem packages are indexed without any generator runtime;
- corrupt/incomplete packages never become ready;
- packages become stale when the source audio hash changes;
- duplicate candidate packages resolve deterministically.

### External project milestone — Create ViiB-StemLab repository

After the ViiB Stem Package v1 contract is stable, create the separate **ViiB-StemLab** repository.

Initial StemLab deliverables should include:

- desktop preparation UI;
- CLI for automation/integration;
- htdemucs_6s generation;
- CPU/CUDA/MPS device selection;
- batch queue;
- progress/cancellation/retry;
- output validation;
- atomic ViiB Stem Package v1 writer;
- configurable output Stem Library.

This milestone is a dependency for convenient first-party stem creation, but it is **not** a MediaHub backend subsystem and should be planned/tracked in its own repository once created.

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

### 35.1 MediaHub stem playback

MediaHub performance gates should focus on consumption of already-generated packages.

Measure:

- two-deck stem playback CPU;
- memory use;
- storage read throughput;
- decoder load;
- AudioWorklet underruns;
- seek latency;
- loop/beat-jump recovery;
- UI frame drops.

Test on Windows, macOS and Linux using identical pre-generated fixture packages.

### 35.2 ViiB-StemLab generation performance

Generation benchmarks belong to the future ViiB-StemLab repository.

StemLab should eventually measure:

- model startup time;
- separation wall time;
- real-time factor;
- peak RAM;
- peak VRAM;
- output disk size;
- CPU/CUDA/MPS performance.

Because stem generation is ahead-of-time, MediaHub's live DJ release must not depend on the generator being active or meeting a real-time inference target.

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


## 37.1 Rhythm and semantic-structure quality

Maintain an overlap-audited annotated rhythm/structure corpus separate from the scalar BPM/key gate.

Measure at minimum:

- beat precision/recall/F-measure using a declared timing tolerance;
- downbeat precision/recall/F-measure;
- bar-one offset error;
- meter accuracy where annotated;
- segment-boundary precision/recall/F1 at both tight and musically tolerant windows;
- functional-label macro-F1 where labels exist;
- downstream cue hit rate and median boundary distance in beats.

A model must not be promoted because it improves scalar BPM while producing poor downbeat placement.

### 37.2 Third-party model overlap gate

Before using a pretrained model result as release evidence:

1. record model/checkpoint identity;
2. document known training datasets;
3. search the evaluation manifest for exact and metadata-derived overlaps;
4. exclude or separately report contaminated tracks;
5. never tune thresholds on the held-out split;
6. preserve the raw prediction artifact and overlap audit.

The existing Beat This r5 result is the motivating example: it is useful diagnostic evidence, but known training-set overlap means it is not independent qualification evidence.


# PART XIII — TEST PLAN

## 38. Backend tests

Add tests for:

- stem manifest schema validation;
- source hash invalidation;
- adjacent package discovery;
- configured Stem Library discovery;
- explicit package linking;
- duplicate-package resolution;
- stale/invalid/ready state transitions;
- per-stem checksum/frame validation;
- safe unlink/delete behavior;
- generated cue overwrite policy;
- user cue preservation;
- Energy Level determinism;
- rhythm-grid provenance serialization;
- measured-vs-derived downbeat behavior;
- canonical timestamp mapping for external adapters;
- structure serialization;
- third-party model overlap-audit reports;
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

Those dependencies belong to **ViiB-StemLab**, not ViiB MediaHub. MediaHub should consume the resulting package without linking or shipping Demucs/PyTorch.

### 44.2 FFmpeg

StemDeck documents using an external FFmpeg executable.

Any FFmpeg dependency used for stem generation belongs to the future ViiB-StemLab distribution and requires an explicit licensing/release decision there.

MediaHub's package consumer should use its own existing supported decode path wherever practical and must not inherit StemLab's generation dependencies merely to play stems.

---


## 44.3 Open-source MIR references

Current license posture for the main research candidates:

- **Beat This!** — MIT code and published model weights; training-data provenance still requires evaluation-overlap review.
- **All-In-One** — MIT upstream; verify the exact fork/package and every bundled dependency/model before distribution.
- **All-In-One-Infer** — MIT integration layer; still review transitive model/runtime licenses before shipping.
- **BeatNet** — CC-BY-4.0 repository; attribution and dependency review required.
- **Essentia** — AGPLv3; keep benchmark-only unless an explicit product/licensing decision approves otherwise.

A permissive code license is necessary but not sufficient. Model weights, bundled DSP libraries, training-data implications, and transitive runtime licenses need separate review.


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
  - register stem discovery/registry/audio routes

- backend/internal/db/
  - stem registry migrations and persistence

- backend/internal/analysis/features/
  - Energy Level
  - structure
  - recommendation v2

- backend/internal/dj/
  - any server-side DJ recommendation aggregation

---

## 47. New files likely to be created

Suggested MediaHub files:

    backend/internal/stems/manifest.go
    backend/internal/stems/discovery.go
    backend/internal/stems/registry.go
    backend/internal/stems/resolver.go
    backend/internal/stems/validate.go

    docs/VIIB_STEM_PACKAGE_V1.md

    backend/internal/analysis/cues/generator.go
    backend/internal/analysis/cues/policy.go
    backend/internal/analysis/structure/analyzer.go
    backend/internal/analysis/energy/level.go

    scripts/beat_this_rhythm_benchmark.py
    scripts/allinone_structure_benchmark.py
    docs/REFERENCE_RHYTHM_STRUCTURE_BENCHMARK.md

    components/dj/v2/DJStemControls.tsx
    components/dj/v2/DJCueEditor.tsx
    components/dj/v2/DJMixNext.tsx
    components/dj/v2/DJMashupPlanner.tsx
    components/dj/v2/CamelotChip.tsx
    components/dj/v2/CamelotWheelPopover.tsx

    lib/stemTransport.worklet.js
    lib/camelotColors.ts
    lib/djPitch.ts

Files for model execution, PyTorch/Demucs integration, generation queues and package writing belong in the future **ViiB-StemLab** repository and should not be added to ViiB-MediaHub.

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

### PR 3 — Rhythm/downbeat benchmark and provenance

- normalized benchmark result schema
- Beat This reference adapter
- All-In-One reference adapter
- canonical PCM/WAV timing path
- training-set overlap audit
- measured/manual/derived downbeat provenance
- no shipping ML runtime

### PR 4 — Auto-cue generator V1

- consume existing beatgrid/energy/structure features
- generate up to eight cues
- fill-empty policy
- tests

### PR 5 — Energy Level V1

- scalar 1-10
- versioned persistence
- column/filter

### PR 6 — ViiB Stem Package v1

- versioned manifest specification
- source-hash identity
- six-stem canonical names
- checksum/frame metadata
- deterministic fixture packages
- no generator runtime

### PR 7 — MediaHub stem discovery and registry

- adjacent/configured-location discovery
- manifest validation
- source-hash stale detection
- stem metadata persistence
- link/unlink API
- stem status badge

### Future repo kickoff — ViiB-StemLab

Once the ViiB Stem Package PR stabilizes the package contract, create the separate ViiB-StemLab repository and implement the first-party generator there.

This is not a MediaHub PR.

### PR 8 — DeckSource refactor

- preserve existing behavior only

### PR 9 — Stem package preview/audio serving

- prove package resolution and audio APIs before real-time deck integration

### PR 10 — Stem deck transport

- worklet/frame streaming
- no advanced UI yet

### PR 11 — Four-button stem UI

- Vocal/Drums/Bass/Music
- Full/Acapella/Instrumental
- stem badge

This sequence keeps ML packaging out of MediaHub entirely and reduces the risk of coupling package discovery, real-time transport, UI and generator implementation into one unreviewable change set.

---

# PART XVIII — DEFINITION OF DONE

## 49. Professional stem release

MediaHub stem functionality is release-ready only when:

- valid ViiB Stem Package v1 packages can be discovered from adjacent and configured locations;
- source-hash mismatches are detected as stale;
- corrupt or partial packages never become ready;
- MediaHub does not require ViiB-StemLab, Python, PyTorch or Demucs at playback time;
- four DJ stem groups are sample aligned;
- seek/loop/sync remain aligned;
- Key Lock behavior is truthful;
- stem mode works on both decks;
- fallback to original audio is reliable;
- user-facing package errors are actionable;
- normal DJ mode remains unchanged when no stems are available.

First-party generation readiness is defined separately in the future ViiB-StemLab repository and should include complete six-stem generation, device transparency, reliable cancellation, atomic package finalization and storage controls.

---

## 50. Professional preparation release

Cue/Energy/Camelot preparation is release-ready only when:

- generated cues never destroy manual cues;
- cues are beat/downbeat aligned where evidence supports it;
- musical downbeat provenance is explicit and meter-derived placeholders are not mislabeled as measured downbeats;
- cue provenance is visible;
- all 24 Camelot keys have deterministic colors;
- Energy Level is versioned and reproducible;
- any user-facing standards-compliant LUFS/true-peak claim uses a qualified implementation rather than the current proxy;
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
- MediaHub -> ViiB-StemLab playlist handoff for batch generation;
- ViiB-StemLab “generate stems for next N tracks” preflight before a gig.

---

# 52. Recommended product priority

For the next DJv2 development cycle, prioritize in this order:

1. **True beat/downbeat provenance + overlap-audited Beat This / All-In-One benchmark**
2. **Automatic cue points + provenance**
3. **Camelot visual system**
4. **Standards-correct loudness foundation + Energy Level 1-10**
5. **ViiB Stem Package v1 + MediaHub discovery/registry**
6. **Create the separate ViiB-StemLab repository and first-party generator**
7. **Four-group stem deck playback**
8. **Mix Next v2**
9. **Pitch-shift/mashup planning**
10. **Third-party DJ export**

This order fixes the most important prerequisite for trustworthy cue/phrase intelligence first: distinguishing a true musical downbeat from a merely phase-aligned meter grid. It then produces visible DJ-preparation value early, stabilizes the package boundary before creating the generator repository, and lets the more difficult real-time stem transport work proceed independently from ML/runtime packaging.

The most important architectural constraint is now explicit: **separate ahead of time; perform in real time**. ViiB-StemLab owns separation. ViiB MediaHub owns package discovery and performance. Demucs or any future model can evolve independently as long as StemLab continues to emit a compatible package.
