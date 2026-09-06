# DJv2 Professional Track Analysis & Harmonic Mixing Roadmap

**Status:** Proposed implementation roadmap  
**Scope:** DJv2 / professional DJ workflow only  
**Research snapshot:** 2026-09-06  
**Target architecture:** Go/Wails, local-first, cross-platform, predominantly pure Go, no CGO unless explicitly justified  
**Repository:** `ajbergh/ViiB-MediaHub`

> This document is a research and implementation plan, not a claim that the described future functionality is already implemented. Sections labeled **Verified current state** describe behavior confirmed in the repository as of the research snapshot. Sections labeled **Proposed** describe future design.

---

## 1. Executive summary

ViiB should not add a standalone “BPM detector” and a Camelot column as isolated DJv2 features. It should establish a reusable **ViiB Track Analysis Engine** that owns deterministic, versioned, persistent audio-derived facts for every ViiB catalog track that ViiB can decode.

The immediate product outcome is:

1. audio-measured BPM with confidence and half/double-tempo handling;
2. audio-measured musical key with confidence;
3. derived Camelot and Open Key notation;
4. durable analysis metadata in the canonical ViiB catalog model;
5. background/manual analysis workflows;
6. DJv2 sorting, filtering, compatibility highlighting, and “what should I mix next?” assistance;
7. a stable analysis substrate for later beatgrids, downbeats, energy, phrase/section analysis, cue suggestions, transition scoring, and AI DJ sequencing.

### Primary architectural conclusions

- **Move authoritative analysis to Go.** The current BPM and key analyzers are browser/Web Audio implementations invoked when a track is loaded into a deck. They are useful baselines, but they are not a durable professional library-analysis system.
- **Do not treat the current generated beat grid as detected beat data.** Today ViiB creates uniformly spaced beat timestamps from one BPM value and an offset. A professional beatgrid must eventually contain detected beat/downbeat positions or tempo segments and must be independently editable/versioned.
- **Preserve the canonical `songs` catalog.** Local and Plex-synchronized tracks must continue to share ViiB song identity. Analysis should attach to `song_id`; do not create a DJ-only catalog and do not write ViiB analysis back to Plex by default.
- **Separate measured facts from inferred metadata.** `songs.bpm` currently participates in AI metadata enrichment and may be an estimated value based on genre/title/artist context rather than measured audio. New measured BPM must carry provenance and take precedence for DJ behavior without silently changing the meaning of old rows.
- **Reuse existing durable job infrastructure.** ViiB already has `operation_jobs` with queued/running/failed/interrupted/canceling/canceled states, progress, retry, and restart recovery. Track analysis should extend this rather than invent a parallel job framework.
- **Reuse one decoded PCM stream.** Waveform overview, onset/tempo, chroma/key, and later energy/loudness should share decoding, downmixing, resampling, windowing, and where practical spectral frames.
- **Prefer a separate scalar `track_analysis` row plus versioned artifact blobs.** Scalar query fields belong in a queryable table; high-volume beat/phrase/waveform artifacts should not become tens of millions of SQLite rows.
- **Use a conservative first harmonic model.** Initial “compatible key” behavior should be based on established same-key, adjacent Camelot number within the same mode, and relative major/minor moves. ViiB should not present arbitrary numerical compatibility scores as musical truth.
- **Treat codec support as an explicit capability matrix.** MP3 and Ogg/Vorbis already have pure-Go backend decoders in the dependency graph. WAV and AIFF PCM are straightforward to support in pure Go. FLAC has viable pure-Go options. AAC/M4A remains the highest-risk path because the strongest newly available pure-Go AAC option carries LGPL obligations and incomplete profile coverage.
- **Accuracy gates precede UX claims.** A detector that returns a number is not production-ready. BPM and key require repeatable benchmark corpora, half/double error accounting, confidence calibration, and cross-platform performance measurements.

### Recommended phase order

| Phase | Outcome | Complexity |
|---|---|---|
| 0 | Research harness, corpus, codec/algorithm spikes, current-detector baselines | Large |
| 1 | Pure-Go shared analysis foundation and persistence model | Large |
| 2 | Production static-tempo/BPM analyzer | Large |
| 3 | Production key + Camelot/Open Key analyzer | Large |
| 4 | Durable library analysis service and lifecycle | Large |
| 5 | DJv2 library/harmonic workflow integration | Large |
| 6 | Beatgrid + downbeat + dynamic tempo | Very Large |
| 7 | Energy/structure/cues/transition intelligence and AI DJ reuse | Very Large |

---

## 2. Current-state audit

### 2.1 Verified current behavior

The following findings were verified from the current repository rather than inferred from documentation.

#### Frontend key detection: active, browser-side, session-oriented

`lib/keyDetection.ts` currently implements:

- Web Audio `AudioContext` decoding;
- an 8192-point FFT with 2048-sample hop;
- Hann windowing;
- pitch-class/chromagram accumulation over approximately 65–2093 Hz;
- Krumhansl-Schmuckler major/minor profiles;
- correlation against 24 major/minor candidates;
- a confidence value derived from the winning correlation;
- traditional key, Camelot, and Open Key mappings;
- a `getKeyCompatibility()` helper.

`hooks/useDJAudioEngine.ts` invokes key analysis asynchronously when a track is loaded into a deck. The caller analyzes approximately the first 30 seconds and then writes the result into DJ deck state with `setDeckAnalysis()`.

**Implication:** this implementation is real and useful, but it is not a persistent catalog analyzer. It should be retained during migration as a baseline/test oracle and then retired from authoritative analysis once the Go implementation passes accuracy gates.

#### Frontend BPM detection: active, browser-side, session-oriented

`lib/bpmDetection.ts` currently:

- decodes with Web Audio;
- analyzes at 22.05 kHz;
- low-pass filters at approximately 150 Hz;
- extracts short-window energy peaks using an adaptive threshold;
- builds a one-BPM-resolution interval histogram;
- searches 60–200 BPM;
- applies limited half/double weighting;
- emits a confidence estimate;
- falls back to 120 BPM with confidence 0 when analysis cannot establish a tempo;
- can fold a detected tempo into a target range via `normalizeBPM()`;
- produces `generateBeatGrid()` by spacing beats uniformly from an offset.

`hooks/useDJAudioEngine.ts` runs this analyzer after deck load and stores the result in deck state.

**Professional gap:** the current detector is a reasonable prototype, not a production DJ analyzer. It is sensitive to peak-selection errors, uses only a short initial section, lacks a robust multi-candidate tempo model, does not estimate tempo drift, and can turn an uncertain analysis into a misleading `120` fallback if downstream code ignores confidence.

#### DJ deck analysis state is not canonical library analysis

`slices/djMixerSlice.ts` stores per-deck fields including:

- `originalBpm`;
- `effectiveBpm`;
- `key`;
- `beatGrid`;
- `beatGridOffset`;
- tempo/sync state.

These values are performance state in Zustand, not durable track-analysis records.

#### Sync is only as good as the current tempo/grid estimate

`pages/DJModeV2.tsx` BPM sync computes a tempo ratio from the current deck’s `originalBpm` and the other deck’s `effectiveBpm`, then applies the ratio within the supported tempo range. Beat-phase sync uses the deck beat information maintained by the current engine/store.

Because today’s “beat grid” is derived from a single BPM and offset rather than detected beats/downbeats, precise sync, loops, beat jumps, and beat-synchronous FX can inherit BPM/phase errors.

#### DJv2 library already exposes part of the desired UX

The active DJv2 path is:

```text
DJModeV2
  -> DJLibraryDrawer
     -> DJLibraryBrowserV2
```

`DJLibraryBrowserV2.tsx` already contains:

- virtualized rows with `react-virtuoso`;
- sorting;
- configurable columns;
- BPM and key columns;
- track colors;
- deck-load controls and drag/drop;
- key-compatibility coloring against a loaded deck.

However:

- BPM is read from `Song.bpm`;
- key is populated through a module-level session `Map` when a track that was loaded into a deck receives browser-side key analysis;
- Camelot is not yet the durable library representation;
- compatibility depends on transient deck/session analysis rather than canonical track metadata.

`components/dj/DJLibraryBrowser.tsx` is an older/legacy browser. It remains exported by the older DJ component barrel, but DJv2’s current drawer uses `DJLibraryBrowserV2`. Do not implement new professional analysis UX in the older browser unless the legacy mode still has an explicit supported requirement.

#### `songs.bpm` exists, but its provenance is not appropriate for professional Sync

The Go `db.Song` and frontend `Song` models already expose `bpm` and the SQLite `songs` table has a `bpm INTEGER` column.

The backend AI metadata/enrichment path explicitly asks the model to **estimate BPM based on genre conventions** and does not measure audio. AI DJ sequencing and scoring can use this BPM for approximate flow.

Therefore the existing scalar has ambiguous provenance: it may be useful metadata, but it must not be silently reinterpreted as an audio-measured, fractional, professional DJ tempo.

#### ReplayGain metadata already exists

`Song` already contains `replayGainDb` / `replayPeak`, and normal playback uses these when normalization is enabled. Track Analysis should not create an incompatible second loudness concept. A later loudness phase should either preserve tag-derived ReplayGain and add clearly named measured LUFS/true-peak fields, or explicitly define how newly measured normalization relates to the existing fields.

#### Backend waveform analysis exists, but is format-limited

`backend/internal/api/dj_waveform.go` provides lazy waveform generation and SQLite caching. Server-side generation currently decodes MP3 with `github.com/hajimehoshi/go-mp3`; OGG, FLAC, WAV, and AAC return a client-side fallback error.

This is useful precedent for cached derived audio data, but the new analysis engine should move decoding below waveform/BPM/key so one common PCM source can feed all analyzers.

#### Existing durable jobs should be reused

`backend/internal/db/jobs_schema.go` and `jobs_repository.go` already provide `operation_jobs` with:

- queued/running/succeeded/failed;
- canceling/canceled;
- interrupted-on-restart recovery;
- current/total progress;
- typed JSON parameters/results;
- stable error codes/messages;
- retry attempt count;
- cooperative cancellation.

Track analysis should introduce one or more `type` values and a dedicated scheduler/runner on top of this durable layer.

#### Scanner/import lifecycle offers integration hooks

The local scanner already has bounded worker behavior, incremental file-change detection, metadata extraction, and job/event infrastructure. New analysis should be queued after catalog ingestion, not mixed directly into the metadata scanner’s hot path.

#### Canonical local/Plex architecture must remain intact

`docs/architecture.md` and the Plex implementation establish:

- `songs` is canonical for local and synchronized Plex music;
- Plex identity belongs in `plex_sources` / `plex_tracks`;
- `/api/audio/{songId}` is source-transparent for the frontend;
- Plex credentials stay in the backend;
- Plex remote metadata is not to be destructively modified by normal ViiB catalog behavior.

**Analysis consequence:** persist ViiB analysis by canonical `song_id`. For Plex, decode from a backend-authenticated source stream when the source is online. Cache the resulting ViiB analysis locally. Do not write BPM/key/beatgrid changes back to PMS unless a separate opt-in writeback feature is designed later.

### 2.2 Current-state classification

| Area | Classification | Recommendation |
|---|---|---|
| `lib/keyDetection.ts` | Active prototype/baseline | Retain temporarily as benchmark oracle; replace as authoritative analyzer with Go |
| `lib/bpmDetection.ts` | Active prototype/baseline | Retain temporarily; replace as authoritative analyzer with Go |
| `generateBeatGrid()` uniform grid | Active but not true detection | Rename/contain as synthetic grid; do not persist as “analyzed beatgrid” |
| Zustand deck BPM/key/grid | Active performance state | Continue using effective deck state, but hydrate from persistent analysis first |
| `songs.bpm` | Active, provenance ambiguous/AI-inferred in enrichment path | Preserve compatibility, add measured source/provenance and effective-value resolver |
| DJLibraryBrowserV2 session key cache | Active workaround | Remove after persistent key fields are available |
| `getKeyCompatibility()` | Active UX helper | Replace arbitrary scalar scoring with explicit compatibility classes first |
| backend MP3 waveform cache | Active | Generalize/reuse beneath shared analysis artifacts |
| legacy `DJLibraryBrowser.tsx` | Legacy relative to DJv2 | Do not target for new DJv2 work |
| AI DJ BPM continuity scoring | Active | Feed measured effective BPM when available; retain metadata fallback |
| Semantic library | Active adjacent system | Reuse measured track features as structured ranking features, not embedding text by default |

---

## 3. Competitive product research

The useful lesson from professional DJ applications is not a particular skin or marketing label. It is their **preparation lifecycle**: analysis is durable, re-runnable, visible, correctable, and separated from performance-critical audio.

### 3.1 rekordbox 7

Current rekordbox 7 documentation exposes:

- automatic analysis when tracks enter the collection;
- manual **Analyze Track** and re-analysis;
- selectable analysis items including BPM/Grid, Key, Phrase, and Vocal;
- Normal, Dynamic, and Auto analysis modes;
- BPM ranges for Normal mode;
- a high-precision BeatGrid analysis option that trades speed for accuracy;
- progress state attached to collection rows;
- waveform/beatgrid/key/phrase analysis as durable preparation data;
- manual BPM/grid correction;
- Traffic Light-style compatible-key assistance in the broader rekordbox workflow.

**ViiB lesson:** expose an explicit accuracy/performance mode later, but initially ship one validated deterministic mode. Persist analyzer version so “new analysis data available” can be offered after algorithm upgrades.

### 3.2 Serato DJ Pro

Serato’s analysis workflow includes:

- Analyze on Import;
- Analyze Files for the library or selected files/crates;
- Analyze Entire Library / forced re-analysis;
- skipping already analyzed files by default;
- stop/resume behavior;
- optional Set Key and Analyze BPM/Beatgrid;
- user-selectable BPM ranges to mitigate half/double mistakes;
- analysis on deck load for unanalyzed tracks;
- persistent/manual beatgrid editing and locking;
- re-analysis rules that can overwrite manual changes unless tracks are protected/locked;
- waveform preparation and corruption detection outside live performance.

**ViiB lesson:** “reanalyze” must have explicit manual-override semantics. Never overwrite user grid/key/BPM corrections without warning or a policy choice.

### 3.3 Traktor Pro 4

Traktor provides:

- analysis at import and explicit analysis commands;
- configurable BPM ranges and automatic tempo interpretation;
- key analysis with switchable musical/Open Key display;
- auto gain;
- beatgrid/downbeat markers;
- analysis locks;
- reset/reanalyze controls;
- **Flexible Beatgrids** for changing-tempo/live material;
- multiple grid markers, each defining a tempo region;
- BPM tapping and manual grid adjustment;
- BeatSync/TempoSync behavior grounded in beatgrid data.

**ViiB lesson:** a future ViiB beatgrid representation should be capable of tempo segments/anchors rather than baking in a single constant BPM forever.

### 3.4 Mixed In Key 11

Mixed In Key focuses on library preparation:

- key and Camelot analysis;
- BPM;
- energy level;
- cue-point suggestions;
- segment-level energy display;
- sorting/filtering by key, tempo, energy, genre;
- exporting analysis/cues into other DJ ecosystems.

Its public accuracy claims should be treated as vendor claims unless independently reproduced.

**ViiB lesson:** BPM + key becomes significantly more useful when paired with energy and structurally useful cue regions. That belongs after measured BPM/key are trustworthy.

### 3.5 VirtualDJ

VirtualDJ provides:

- per-track, selected/batch, folder, and full-library analysis;
- BPM and key fields populated after scan;
- BPM Editor with half/double controls;
- rigid and fluid/dynamic BPM/grid modes;
- multiple anchors for fluid material;
- manual BPM and beat-phase adjustment;
- harmonic-key display and automatic/smart key-match tools;
- POI/Automix editors beyond basic analysis.

**ViiB lesson:** manual correction is not an edge case; it is a first-class professional workflow. Automatic confidence does not eliminate the need for user edits.

### 3.6 Algoriddim djay Pro

djay currently documents:

- automatic analysis when a track is loaded;
- BPM/beatgrid analysis;
- automatic straight vs Dynamic beatgrid selection;
- half/double tempo correction;
- fractional/manual BPM entry;
- tap BPM;
- explicit downbeat, anchor, and grid editing;
- dynamic grids for live drums, tempo changes, and older recordings.

**ViiB lesson:** the detector should retain ambiguity/candidate information internally and the UI should make correction cheap. Dynamic-tempo classification belongs with beatgrid work, not with a fake single-number BPM claim.

### 3.7 Mixxx — open-source architectural reference

Mixxx is especially instructive architecturally:

- background streaming analyzers share an analyzer interface;
- separate analyzer plugins can provide fixed BPM or actual beat positions;
- analysis can be fast (first minute) or full-track;
- constant-tempo vs variable-tempo behavior is explicit;
- stale analysis can be re-run when detector settings/version change;
- key analysis is separately versioned;
- its library supports fuzzy compatible-key and relative BPM searches;
- its current analyzer architecture distinguishes BPM, beats, key changes, waveform, ReplayGain, and silence analysis;
- Sync depends on beat maps/grids rather than merely one tempo number.

Mixxx is GPLv2-or-later and its commonly used `libkeyfinder` detector is GPLv3+. ViiB should study interfaces/data modeling/algorithms but not copy or translate implementation code into the MIT codebase.

### 3.8 Product-pattern synthesis

The recurring professional behaviors ViiB should adopt are:

1. analyze on import as an option;
2. Analyze Selected / Playlist / Missing / Entire Library;
3. explicit progress and cancel/pause semantics;
4. skip valid prior analysis;
5. version-based stale detection;
6. file/source-change invalidation;
7. analysis-on-load fallback for missing data;
8. BPM ranges/tempo priors to reduce octave errors;
9. fractional BPM;
10. half/double correction;
11. manual BPM and key override;
12. beatgrid manual editing and locking in a later phase;
13. straight vs dynamic tempo as a later explicit concept;
14. user-selectable key notation;
15. compatible-key and tempo filters;
16. analysis work throttled away from live performance.

---

## 4. Supplied open-source project evaluation

> License assessment is engineering guidance, not legal advice. Direct source reuse must follow the exact repository license and ViiB’s distribution model. When a project is GPL/AGPL, this roadmap recommends studying published algorithms and independently implementing from public descriptions/papers rather than translating its code.

### 4.1 Evaluation matrix

| Project | Language | DSP / behavior | Key | BPM | Beatgrid | Camelot | Accuracy evidence | Dependencies | License | Activity at snapshot | Pure-Go portability | Legally useful code/concepts | Recommendation |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| `jackbittiner/camelot-wheel` | JavaScript | Music-theory mapping/routing, no audio DSP | Mapping only | No | No | Yes | N/A | Node ecosystem | MIT | Last push observed 2023-01; still public | Concept trivially portable | MIT mapping/routing concepts are reusable, but ViiB already has equivalent maps | **Study/compare only; no dependency needed** |
| `ifeelvoid/keyfinder` | Swift | 16K FFT, harmonic weighting, Krumhansl-Schmuckler; spectral-flux/autocorrelation BPM | Yes | Yes | No | Yes | README claims ~90–95% on clear tracks; treat as self-reported until reproduced | Apple audio/framework ecosystem | MIT | Created/pushed in 2026; relatively new | Poor: macOS/Swift-centric | Algorithm layout and permissively licensed implementation can inform independent Go work | **Useful reference; benchmark ideas, do not adopt platform stack** |
| `lucaderumier/mixflow` | TypeScript/Svelte | Delegates analysis to Essentia.js/WASM; orders by BPM + Camelot | Via Essentia | Via Essentia | Not a core feature | Yes | No independent evidence in project | Essentia.js/WASM | Project MIT; Essentia/Essentia.js AGPLv3 | New/active project | App itself not Go; core analyzer carries WASM/AGPL issue | Compatibility/order UX concepts are useful | **Study UX only; reject Essentia.js as default embedded analyzer** |
| `benjojo/bpm` | Go | Port of Mark Hills `bpm-tools`; accepts mono float PCM | No | Yes | Progressive snapshots, not DJ beatgrid | No | No project benchmark demonstrating pro DJ accuracy | Expects caller PCM; examples use SoX/FFmpeg | GPLv2 | Old/small project | Technically Go, legally incompatible for direct MIT reuse absent relicensing strategy | Public algorithm lineage can be researched independently | **Do not copy/port code; study algorithm/history only** |
| `dlepaux/realtime-bpm-analyzer` | TypeScript | Web Audio / AudioWorklet real-time BPM; inspired by bpm-detective/Joe Sullivan | No | Yes | Beat/tempo stream, not professional edit grid | No | Test suite exists; no independent broad DJ benchmark identified | Browser Web Audio; zero runtime JS deps claimed | Apache-2.0 | Actively maintained; v5.x current in 2026 | Browser-only, not Go | Permissive algorithm/streaming patterns can be studied or translated with attribution compliance | **Good BPM reference; not production backend dependency** |
| `libraz/bpm-detector` | Python | librosa tempo, candidate harmonic clustering; chroma + K-S key; many advanced descriptors | Yes | Yes | Rhythm/structure modules, not ViiB-ready grid format | Not primary | Project benchmarks performance, not an independent DJ accuracy corpus | librosa, NumPy, SciPy, soundfile, sklearn, etc. | MIT | Active; development shifting toward libsonare | Poor due Python/native scientific stack | High-level algorithm decomposition is useful | **Study algorithms/tests; reject Python production stack** |
| `Rexaintreal/Resonate` | JavaScript + Flask/Python | Web Audio FFT/autocorrelation/tuner/chord/metronome toolkit | Pitch/chords rather than robust DJ key focus | Yes/basic | No | No | No pro-DJ validation | Web Audio, Flask, Firebase | MIT | Small/hack-project maturity | Poor | Basic visualization/autocorrelation examples | **Low-priority reference; not a foundation** |

### 4.2 Better/more relevant references

#### Mixxx

- **Type:** C++/Qt application, native dependencies
- **License:** GPLv2-or-later
- **Value:** best open reference here for analysis lifecycle, plugin boundaries, versioning, beat-map vs beat-grid modeling, manual corrections, and DJ-facing behavior.
- **Reuse:** architecture/concepts and public algorithm descriptions only unless ViiB deliberately adopts compatible licensing.

#### `mixxxdj/libkeyfinder`

- **Type:** C++ key detector
- **License:** GPLv3-or-later
- **Value:** mature dominant-key detection reference and research lineage.
- **Reuse:** study only; no direct translation into MIT ViiB.

#### Essentia / Essentia.js

- **Type:** C++ and WASM/JavaScript MIR library
- **License:** AGPLv3 for the principal library; external components have additional licenses.
- **Value:** excellent benchmark/reference implementation for broad MIR algorithms.
- **Reuse:** benchmark in a development-only harness if license/process permits; **reject as default bundled production engine** under current MIT/local-binary goals.

#### `libraz/libsonare`

- **Type:** C++ core with Python/Node/WASM bindings
- **License:** Apache-2.0 according to project documentation
- **Value:** current dependency-free C++/WASM implementation covering BPM, key, beat/downbeat, tempogram, structure, loudness, and more.
- **Reuse:** potentially valuable **benchmark/reference** and a future WASM fallback candidate. It is not pure Go, and maturity/accuracy must be independently measured before any product dependency.

#### Gonum DSP Fourier

- **Type:** Pure Go numerical/DSP package
- **License:** BSD-style Gonum license
- **Value:** avoid maintaining a bespoke FFT implementation if benchmarks show acceptable allocations/performance.
- **Recommendation:** preferred candidate for Phase 0 FFT correctness/performance spike; an internal optimized FFT can replace it only if profiling justifies the maintenance cost.

### 4.3 Licensing rules for implementation sessions

For every dependency added by future phases, the PR must record:

- package/repository;
- exact version/commit;
- dependency class: Pure Go / WASM / CGO-native / External executable / Cloud/API;
- SPDX license;
- transitive native/runtime requirements;
- whether static redistribution changes obligations;
- source-notice requirements;
- whether code was copied, adapted, or independently implemented from literature.

**Hard rule:** do not “port” GPL/AGPL source by line-by-line translation into Go. A language translation is still a derivative implementation concern; use papers, public algorithm descriptions, standards, and clean independent code instead.

---

## 5. DSP and algorithm research

## 5.1 Musical key detection

### Current ViiB baseline

The current browser detector demonstrates the basic viable chain:

```text
PCM -> Hann windows -> FFT -> pitch-class energy -> 12-bin chroma
    -> correlate 24 major/minor templates -> best key -> Camelot/Open Key
```

That is a valid baseline but not yet a professional implementation.

### Recommended production pipeline

```text
PCM source
  -> stereo-aware downmix
  -> analysis resample (candidate: 22.05 kHz or 44.1 kHz after benchmark)
  -> DC removal / level normalization safeguards
  -> STFT
  -> spectral magnitude/power
  -> tuning-frequency estimate (optional in first release, required before final accuracy gate)
  -> harmonic pitch-class profile / chroma
     - octave folding
     - harmonic weighting
     - spectral whitening or log compression
  -> segment-level chroma summaries
  -> template/profile scoring across 24 keys
  -> temporal voting / robust aggregation
  -> confidence calibration
  -> tonic + mode
  -> derived Traditional / Camelot / Open Key display
```

### Profiles to benchmark

At minimum compare:

- Krumhansl-Schmuckler/Krumhansl-Kessler profiles;
- Temperley-style profiles;
- one empirically tuned EDM/pop profile if a public, permissively documented source exists;
- current ViiB JS profile implementation as baseline.

Do not pick a profile from a handful of favorite tracks. Select by a labeled benchmark and retain per-genre confusion matrices.

### Why HPCP/chroma should improve over the current JS approach

Professional improvements to validate:

- tuning compensation rather than assuming A4 = exactly 440 Hz;
- better bin-to-pitch-class weighting than nearest MIDI note only;
- harmonic weighting to reduce octave/partial contamination;
- spectral whitening/log compression so a few dominant bins do not overwhelm tonal structure;
- stereo-aware mono mix rather than channel 0 only;
- segment voting so intros/outros/breakdowns do not dominate the entire result;
- optional exclusion/low weighting of very low-energy segments;
- top-2 score margin and temporal consistency for confidence;
- key-change evidence retained for future phase/phrase work.

### Confidence

Do not expose `(correlation + 1) / 2` as if it were a calibrated probability.

Start with an internal score assembled from:

- winner vs runner-up margin;
- winner absolute profile fit;
- agreement across track segments;
- chroma concentration/tonality;
- signal coverage.

Then calibrate display thresholds empirically on held-out labeled material. Recommended user-facing values initially: `High`, `Medium`, `Low` plus an internal 0–1 score for diagnostics/API. Do not imply statistical probability unless calibrated as such.

### Tracks with modulation/key changes

Phase 3 may persist one dominant key plus confidence. The pipeline should nevertheless compute segment observations so Phase 7 can add:

- key-change regions;
- intro/outro key;
- phrase-specific compatibility;
- “dominant key uncertain because track modulates” status.

---

## 5.2 BPM / tempo detection

### Current ViiB baseline

The current low-frequency energy peak + interval histogram detector is useful for regression comparison, but professional accuracy needs richer onset evidence and candidate selection.

### Recommended static-tempo pipeline

```text
PCM
  -> mono/downsample
  -> STFT and/or filtered sub-bands
  -> onset-strength envelope
     - spectral flux (primary candidate)
     - optional low/mid/high band weighting
     - adaptive local normalization
  -> periodicity representation
     - autocorrelation and/or tempogram
  -> multiple tempo candidates
  -> candidate scoring over track sections
  -> metrical ambiguity evaluation (0.5x / 2x, optionally 2/3x / 3/2x)
  -> genre/user BPM-range prior where configured
  -> global tempo + alternate candidate + confidence
```

### Required half/double-tempo handling

The engine must not collapse the ambiguity too early. For example, a rhythmic pulse may legitimately support 70/140 or 85/170 interpretations.

Persist or retain during analysis:

- primary tempo candidate;
- alternate metrical candidate;
- confidence/margin;
- configured BPM range/tempo prior used;
- evidence of section consistency.

User correction actions should include:

- ×2;
- ÷2;
- manual fractional BPM;
- tap BPM;
- re-analyze with another BPM range.

### BPM range / prior design

Proposed defaults:

- `Automatic` — broad detector with learned/heuristic metrical selection, no genre label required;
- common range presets, e.g. 60–120, 70–140, 80–160, 100–200;
- Custom min/max.

Do not infer the “correct” octave solely from AI genre metadata. Genre may be an optional weak prior, never a hidden hard rule.

### False-transient robustness

Benchmark and design for:

- kick-heavy dance tracks;
- syncopated hip-hop;
- breakbeats/DnB;
- sparse acoustic material;
- quiet intros;
- breakdowns without percussion;
- false peaks from vocals/guitars;
- live drums;
- tempo ramps/switches.

Use multiple representative sections or full-track analysis for final results. A “fast” mode can be introduced later only after benchmark evidence shows acceptable regression.

### Dynamic tempo

Do not force dynamic material into a static beatgrid.

Phase 2 should return the best global BPM plus a `tempo_stability`/`tempo_kind_candidate` diagnostic. Phase 6 should add local tempo tracking and actual beat positions/segments.

---

## 5.3 Beatgrid/downbeat research direction

A beatgrid is not “BPM repeated every 60/BPM seconds.” It requires phase.

Phase 6 should compare:

- onset envelope + dynamic-programming beat tracking;
- probabilistic beat tracking;
- autocorrelation/tempogram candidate tempo plus phase maximization;
- local-tempo/beat tracking for variable tempo;
- downbeat/bar-position inference after beat detection.

Representation should support both:

```text
Straight grid: first_beat + constant BPM
```

and

```text
Dynamic grid: ordered beat positions and/or tempo anchors/segments
```

The editor should allow anchors, BPM changes, offset shifts, half/double correction, and lock state.

---

## 6. Recommended architecture

### 6.1 Package layout

**Proposed:**

```text
backend/internal/analysis/
    service.go             # public analysis service / orchestration
    types.go               # stable scalar + artifact types
    version.go             # analysis schema/algorithm versions

    audio/
        source.go           # canonical song -> seekable/streaming audio source
        decoder.go          # decoder interface/registry
        pcm.go              # normalized frame type
        downmix.go
        resample.go
        wav.go              # simple PCM WAV if implemented in-house
        aiff.go             # PCM AIFF/AIFC subset if implemented in-house

    dsp/
        window.go
        fft.go              # adapter; Gonum candidate
        stft.go
        normalize.go
        stats.go

    rhythm/
        onset.go
        tempogram.go
        tempo.go
        candidates.go
        confidence.go
        beatgrid.go         # Phase 6
        downbeat.go         # Phase 6

    tonal/
        chroma.go
        hpcp.go
        tuning.go
        profiles.go
        key.go
        camelot.go
        openkey.go
        confidence.go

    features/
        energy.go           # later
        loudness.go         # later
        structure.go        # later
        phrase.go           # later

    jobs/
        scheduler.go
        runner.go
        priority.go
        progress.go

    cache/
        fingerprint.go
        artifacts.go
```

Exact file boundaries may change, but **decoder/DSP primitives must not live inside DJv2 UI/API handlers**.

### 6.2 Shared analysis pipeline

```text
Canonical Song ID
      |
      v
AudioSource resolver (local file or authenticated Plex stream)
      |
      v
Decoder -> PCM chunks
      |
      v
Downmix / resample / normalization safeguards
      |
      +------------------------+
      |                        |
      v                        v
waveform accumulator       STFT/shared frames
                               |
                 +-------------+--------------+
                 |                            |
                 v                            v
          onset / tempo                  chroma / key
                 |                            |
                 v                            v
        BPM + confidence            key + confidence
                 |                            |
                 +-------------+--------------+
                               |
                               v
                     persist scalar result
                     + versioned artifacts
                               |
                 +-------------+--------------+
                 |                            |
                 v                            v
             DJv2 UI                     AI DJ/ranking
```

### 6.3 Streaming memory model

Do not decode an entire 10-minute track to a giant `[]float64` by default.

Prefer:

- streaming PCM chunks;
- small overlap/ring buffers for STFT;
- online waveform peaks;
- onset envelope retained at a low temporal resolution;
- segment chroma summaries rather than every spectrum frame after accumulation;
- bounded artifact buffers.

This keeps memory approximately O(window + compact feature timeline), not O(raw audio length).

### 6.4 Analysis API boundary

Suggested backend service methods, independent of HTTP/Wails binding:

```go
type Service interface {
    AnalyzeTrack(ctx context.Context, songID string, opts Options) (Result, error)
    Enqueue(ctx context.Context, selection Selection, opts Options) (Job, error)
    GetResult(ctx context.Context, songID string) (Result, error)
    GetEffectiveFeatures(ctx context.Context, songID string) (EffectiveFeatures, error)
    SetOverride(ctx context.Context, songID string, patch ManualOverride) error
    ClearOverride(ctx context.Context, songID string, fields ...Field) error
}
```

HTTP/API routes should be thin adapters.

### 6.5 Suggested events

Reuse the existing event/SSE approach where practical:

```text
analysis.job.queued
analysis.job.started
analysis.job.progress
analysis.track.started
analysis.track.completed
analysis.track.skipped
analysis.track.failed
analysis.job.paused
analysis.job.resumed
analysis.job.canceled
analysis.result.updated
```

Do not emit frame-level DSP events.

---

## 7. Audio-decoding strategy

### 7.1 Decoder contract

The analysis engine needs PCM, not metadata tags and not a browser `AudioBuffer`.

Suggested contract:

```go
type Decoder interface {
    Format() PCMFormat
    ReadFrames(dst []float32) (int, error)
    Close() error
}

type PCMFormat struct {
    SampleRate int
    Channels   int
}
```

The analyzer can normalize every decoder to interleaved float32, then downmix/resample once.

### 7.2 Current and proposed codec matrix

| Format | Current ViiB backend evidence | Pure-Go path | License / class | Recommendation |
|---|---|---|---|---|
| MP3 | `go-mp3` already used for waveform generation | `github.com/hajimehoshi/go-mp3` | Apache-2.0; **Pure Go** | **Phase 1 supported** |
| OGG/Vorbis | `jfreymuth/oggvorbis` already used in OGG conversion | Existing dependency | MIT; **Pure Go** | **Phase 1 supported** |
| WAV PCM | Scanner recognizes; server waveform does not decode today | Small in-house RIFF/WAVE PCM/float reader, or vetted Go library | In-house MIT; **Pure Go** | **Phase 1 supported**; avoid adding archived dependency unless useful |
| FLAC | Scanner recognizes; no current server waveform decode | `mewkiz/flac` is viable pure Go | Unlicense; **Pure Go** | **Phase 1/2 candidate**, benchmark malformed-file handling and seek behavior |
| AIFF/AIFC PCM | Not in current scanner extension list shown in audit | Small in-house IFF/AIFF PCM reader for uncompressed/standard float subsets | In-house MIT; **Pure Go** | Add after test corpus; common pro-DJ format |
| AAC ADTS | Scanner recognizes AAC, but no backend PCM decoder | `tphakala/go-aac` is pure Go AAC-LC but LGPL-2.1-or-later and profile-limited | LGPL-2.1+; **Pure Go** | **Gated evaluation only**; legal/distribution review required |
| M4A/AAC | Metadata/playback may work, backend analysis decode not established | demux + AAC decoder required; pure-Go ecosystem is emerging | likely mixed; **Pure Go if selected stack qualifies** | Phase 0 spike; do not claim support until corpus passes |
| Opus in Ogg | Scanner recognizes `.opus`; `oggvorbis` is not an Opus decoder | evaluate a permissively licensed pure-Go Opus decoder or WASM fallback | TBD | Explicit unsupported-analysis status until validated |
| WMA | Scanner metadata list may recognize it; no pure-Go analysis path established | No recommended path identified | TBD | Do not block engine; mark codec unsupported for analysis |

### 7.3 Why browser decoding should not remain authoritative

Browser/WebView decoding has attractive codec coverage but creates problems for library preparation:

- analysis depends on WebView codec differences by OS;
- background bulk analysis is tied to renderer lifecycle;
- results are harder to reproduce in Go tests/CLI benchmarks;
- local/Plex handling is less controllable;
- analysis can compete with the same browser audio engine used for live DJ performance.

Browser analysis can remain a **temporary fallback/diagnostic comparator**, not the canonical persisted engine.

### 7.4 Fallback hierarchy

Recommended policy:

1. **Pure-Go decoder available and validated:** analyze normally.
2. **Pure-Go decoder available but license/profile uncertain:** feature-gate until approved.
3. **No backend decoder:** persist `unsupported_codec` analysis state; do not fake values.
4. **Optional future WASM codec pack:** only after benchmark, license, binary-size, and security review; run locally through a controlled WASM runtime.
5. **Bundled FFmpeg/native helper:** rejected for initial roadmap. Consider only if codec coverage becomes a release blocker and redistribution/license/attack-surface costs are explicitly accepted.

No dependency on a user-installed FFmpeg/SoX executable.

---

## 8. Database and metadata model

### 8.1 Do not make a DJ-only song database

`songs.id` remains the foreign key for analysis for both local and Plex tracks.

### 8.2 Recommended scalar table

**Proposed:**

```sql
CREATE TABLE track_analysis (
    song_id TEXT PRIMARY KEY REFERENCES songs(id) ON DELETE CASCADE,

    status TEXT NOT NULL,                 -- pending/running/complete/partial/failed/unsupported
    analysis_version INTEGER NOT NULL,
    algorithm_version TEXT NOT NULL,
    decoder_id TEXT,

    source_fingerprint TEXT NOT NULL,
    source_size INTEGER,
    source_mtime INTEGER,
    source_revision TEXT,                 -- remote/Plex revision material where available

    bpm REAL,
    bpm_confidence REAL,
    bpm_alt_candidate REAL,
    tempo_stability REAL,
    tempo_kind TEXT,                      -- unknown/static/dynamic-candidate/dynamic
    bpm_source TEXT,                      -- measured/imported/manual/legacy-ai

    key_tonic INTEGER,                    -- 0..11
    key_mode TEXT,                        -- major/minor
    key_confidence REAL,
    key_source TEXT,                      -- measured/imported/manual
    camelot_key TEXT,
    open_key TEXT,

    analyzed_at INTEGER,
    error_code TEXT,
    error_message TEXT
);
```

The exact migration may choose more normalized fields, but it must preserve provenance/version/fingerprint/confidence.

### 8.3 Manual overrides

Recommended separate table:

```sql
CREATE TABLE track_analysis_overrides (
    song_id TEXT PRIMARY KEY REFERENCES songs(id) ON DELETE CASCADE,
    bpm REAL,
    key_tonic INTEGER,
    key_mode TEXT,
    beatgrid_artifact_id TEXT,
    bpm_locked INTEGER NOT NULL DEFAULT 0,
    key_locked INTEGER NOT NULL DEFAULT 0,
    beatgrid_locked INTEGER NOT NULL DEFAULT 0,
    updated_at INTEGER NOT NULL
);
```

Effective values are resolved as:

```text
manual locked override
    > valid measured analysis
    > trustworthy imported tag where policy permits
    > legacy AI/inferred metadata fallback for non-performance ranking
    > unknown
```

Do not use AI-estimated BPM for beat-phase Sync when measured/manual tempo is absent.

### 8.4 Artifact table

Avoid one SQLite row per beat for a 50,000-track library.

```sql
CREATE TABLE track_analysis_artifacts (
    id TEXT PRIMARY KEY,
    song_id TEXT NOT NULL REFERENCES songs(id) ON DELETE CASCADE,
    kind TEXT NOT NULL,            -- waveform, beatgrid, chroma_segments, phrase_sections, etc.
    format_version INTEGER NOT NULL,
    algorithm_version TEXT NOT NULL,
    encoding TEXT NOT NULL,        -- binary-v1, gzip-json-v1, etc.
    data BLOB NOT NULL,
    created_at INTEGER NOT NULL,
    UNIQUE(song_id, kind, format_version, algorithm_version)
);
```

A compact binary format is preferable for high-volume beat positions. JSON may be used initially for low-volume diagnostics but should not become a permanent high-volume format by accident.

### 8.5 Existing `songs.bpm` migration

Do not immediately delete or overwrite it.

Migration sequence:

1. introduce `track_analysis`;
2. classify existing `songs.bpm` as legacy/inferred unless another verified source is available;
3. update AI DJ and DJv2 to call an effective-feature resolver;
4. measured BPM wins for DJ workflows;
5. legacy BPM remains a fallback for non-critical sequencing until sufficient library coverage exists;
6. later decide whether `songs.bpm` becomes a denormalized effective cache or is deprecated.

### 8.6 Plex fingerprinting

Local source fingerprint candidate:

```text
file size + mtime + existing metadata/file hash where trustworthy
```

Plex source fingerprint candidate:

```text
PMS machine identity + ratingKey/media part identity + size + duration + PMS updated timestamp/revision if available
```

A full cryptographic hash of every remote track is not required merely to detect common changes; it would force expensive full-media reads. The analysis itself already reads the stream when needed.

### 8.7 Analysis versioning

Use both:

- `analysis_version`: ViiB result/schema compatibility integer;
- `algorithm_version`: human/debuggable implementation identifier such as `tempo-v1.2.0+abc123`.

Reanalysis reasons should be explicit:

```text
missing
source_changed
algorithm_upgraded
settings_changed
manual_request
corrupt_previous_result
```

---

## 9. Analysis lifecycle

### 9.1 New local track

```text
Scanner discovers file
  -> metadata/catalog commit succeeds
  -> source fingerprint available
  -> if Analyze New Tracks = on:
       enqueue analysis job item
  -> analysis worker acquires low-priority slot
  -> decode once
  -> BPM/key (+ waveform where enabled)
  -> atomic persistence
  -> result-updated event
  -> DJv2/library row refreshes
```

Do not make the scanner wait for DSP completion.

### 9.2 New/synchronized Plex track

```text
Plex sync updates canonical song row + plex identity
  -> mark analysis missing/stale if source revision changed
  -> enqueue only when auto-analysis policy allows remote sources
  -> resolve authenticated PMS audio through backend source adapter
  -> stream/decode/analyze locally
  -> persist only in ViiB DB
```

Recommended setting:

```text
Analyze new local tracks automatically: On by default
Analyze new Plex tracks automatically: Off by default initially
```

Reason: remote library analysis can create significant network/PMS load. Offer a separate toggle and explain the cost.

### 9.3 Existing-library commands

DJv2/library context actions should eventually include:

- Analyze Selected Tracks
- Analyze Current Playlist/Crate
- Analyze Missing
- Analyze Stale
- Re-analyze Selected
- Re-analyze Entire Library
- Analyze BPM Only
- Analyze Key Only
- Analyze BPM + Key

The first implementation can expose a smaller set, but the service API should support selection + requested feature flags.

### 9.4 Queue behavior

Use the existing durable `operation_jobs` record for batch jobs and keep per-track work state in `track_analysis` or job parameters/result summaries.

Worker policy:

- default workers: conservative, e.g. `min(2, max(1, NumCPU/4))` until benchmarked;
- one worker on battery by default when battery state is available without platform-specific complexity;
- reduce to 0 or 1 background workers while DJv2 Performance mode has active playback;
- foreground “Analyze Selected Now” may use a higher priority but never run DSP on the real-time audio callback/path;
- bounded decode/read concurrency to prevent disk thrash;
- Plex per-source network concurrency cap, initially 1;
- cooperative cancellation between chunks/analysis stages;
- persist progress at track granularity, not every DSP window.

### 9.5 Pause/resume

`operation_jobs` already supports cancellation/interruption semantics. Add a service-level paused state only if required by UX, or implement pause as “stop dispatching new track items; let current item finish.” That is safer than serializing arbitrary DSP internals mid-track.

### 9.6 Failure semantics

Stable error codes should include at least:

```text
source_unavailable
source_changed_during_analysis
unsupported_codec
decode_failed
corrupt_audio
insufficient_audio
no_reliable_tempo
no_reliable_key
canceled
out_of_memory_guard
internal_analysis_error
```

A track may be `partial`: key succeeded while tempo failed, or vice versa.

### 9.7 On-load fallback

When a track is loaded and no valid analysis exists:

1. do not block playback;
2. use safe performance fallback behavior with Sync disabled or clearly limited;
3. schedule high-priority analysis if the source is decodable;
4. update deck values when the result arrives only if doing so will not cause an unsafe abrupt tempo jump;
5. require an explicit Sync action after a late BPM result unless product testing proves automatic application is safe.

---

## 10. DJv2 library UX design

### 10.1 Core columns

Phase 5 columns:

- BPM — fractional display, default one or two decimals according to precision policy;
- Key — notation according to preference;
- Camelot — optional distinct column if traditional key is selected;
- Analysis status icon.

Optional diagnostic columns:

- BPM confidence;
- Key confidence;
- Analysis age/version;
- Source.

Future:

- Energy;
- tempo type (Straight/Dynamic);
- loudness;
- phrase/intro/outro length.

### 10.2 Key-notation preference

One global DJ preference:

```text
Key notation:
  Camelot (recommended DJ default)
  Traditional
  Open Key
```

Persist the canonical tonic/mode. Display notation is derived; do not store three independently editable truths.

### 10.3 Analysis state UI

Suggested compact statuses:

- not analyzed;
- queued;
- analyzing;
- complete;
- low confidence;
- manually corrected/locked;
- stale;
- failed;
- unsupported codec/source unavailable.

Hover/details can expose algorithm/version/error. Do not clutter the main row with engineering text.

### 10.4 Context actions

Track row/context menu:

```text
Analyze Track
Re-analyze Track
Analyze BPM Only
Analyze Key Only
Set BPM…
Tap BPM…
Double BPM
Halve BPM
Set Key…
Clear Manual Override
Lock Analysis
```

Beatgrid actions appear only after Phase 6.

### 10.5 Filtering

Quick chips/dropdowns:

```text
Compatible Key
Similar BPM
Compatible Key + BPM
Analyzed / Missing / Low Confidence
```

BPM tolerance should be percentage-based because pitch sliders are percentage-based:

```text
±2%
±4%
±6%
±8%
Custom
```

Also permit absolute numeric BPM filters in advanced library search.

### 10.6 Sorting

Add/retain:

- BPM ascending/descending;
- Camelot wheel order;
- harmonic compatibility to Deck A;
- harmonic compatibility to Deck B;
- compatibility to current/master deck;
- later: transition score.

Compatibility sorting should be stable and transparent: harmonic class, then BPM distance, then existing sort tie-breaker.

### 10.7 Performance mode constraints

The drawer/browser is already virtualized and has a 12,000-track audit fixture. New compatibility computations must be O(rows) with tiny constants and memoized inputs; never run DSP in React.

For 50,000-track libraries, consider server-side/query-backed filtering only if client-side catalog scale becomes a measured bottleneck. Do not prematurely replace current virtualization.

---

## 11. Harmonic-mixing model

### 11.1 Initial compatibility classes

For a current Camelot key `8A`, ViiB may safely label:

- `8A`: **Same key**
- `7A`, `9A`: **Adjacent compatible key**
- `8B`: **Relative major/minor**

These are the initial **recommended** compatibility relations.

Do not initially label `±2`, cross-mode adjacent, “energy boost” jumps, diagonal moves, or semitone key-shift tricks as universally compatible. Some are valid creative techniques, but they need explicit product terminology and musical rationale rather than a hidden score.

### 11.2 Compatibility as typed data, not magic number

Prefer:

```ts
type HarmonicRelation =
  | 'same'
  | 'adjacent'
  | 'relative'
  | 'other'
  | 'unknown';
```

Optionally expose a ranking weight internally, but UI explains the relation.

### 11.3 Key color system

A wheel-derived consistent color can help scanning, but color is supplementary:

- text notation must remain present;
- color must not be the only compatibility cue;
- accessible contrast and color-vision testing required.

### 11.4 Pitch/key lock

Harmonic recommendations must use the **effective audible key** when a deck has pitch/key shifting enabled, not always the stored original key. Separate:

```text
track original key
current/effective deck key
```

The library’s “compatible with Deck A” reference should use the effective deck key where that value is reliable.

---

## 12. Track compatibility and transition scoring

### 12.1 Do not ship a black-box score in Phase 5

Start with visible independent dimensions:

```text
Harmonic: Same / Adjacent / Relative / Other
Tempo: +1.8% pitch required
Energy: unavailable (until measured)
Phrase: unavailable (until measured)
```

This makes recommendations explainable and allows benchmark/UX tuning.

### 12.2 Later composite scoring

After Phase 7 features exist, a transition candidate can include:

```go
type TransitionFeatures struct {
    HarmonicRelation HarmonicRelation
    RequiredTempoPct float64
    TempoConfidence  float64
    EnergyDelta      float64
    PhraseFit        float64
    VocalOverlapRisk float64 // future, only if classifier exists
}
```

An overall score may then be learned/tuned, but it must not erase component explanations.

### 12.3 Shared human/AI feature layer

AI DJ should consume the same effective measured features:

```text
track analysis -> effective feature resolver
                -> DJv2 recommendations
                -> AI DJ deterministic scoring
                -> semantic/ranking feature inputs
```

Do not create `ai_bpm`, `dj_bpm`, and `library_bpm` as independent truths.

Measured BPM/key should be structured numeric/categorical features in AI DJ ranking. They do not need to be embedded into semantic text to be useful.

---

## 13. Performance strategy

### 13.1 Measurement first

Record for every benchmark run:

- OS and architecture;
- CPU model/core count;
- Go version;
- decoder/algorithm version;
- codec/bitrate/sample rate/channels;
- track duration;
- wall time;
- CPU time where practical;
- peak RSS;
- allocations/op for isolated DSP tests;
- throughput in audio-minutes per wall-minute.

### 13.2 Initial target envelopes

These are engineering targets to validate, not claims about current performance.

**Background static BPM + key, one worker:**

- preferred target: at least **5× real-time** on a representative current laptop CPU for common MP3/FLAC dance tracks;
- stretch target: 10×+ real-time after correctness is stable;
- memory target: < 150 MB incremental RSS per active worker for long tracks, preferably far lower;
- no full-file PCM buffer requirement.

**DJv2 active playback:**

- background analyzer CPU should be reduced aggressively;
- audio underruns/dropouts are a zero-tolerance acceptance failure;
- UI 95th-percentile interaction latency must not measurably regress in the existing audit suite.

### 13.3 Library-scale modeling

At 4 minutes/track:

| Library | Audio duration | At 5× real-time, one worker | Notes |
|---|---:|---:|---|
| 1,000 tracks | ~67 h | ~13.3 h | First-run overnight job; progress/resume important |
| 10,000 tracks | ~667 h | ~133 h | Requires concurrency and skip/stale behavior |
| 50,000 tracks | ~3,333 h | ~667 h | Multi-day initial analysis; incremental operation is essential |

Parallel workers reduce elapsed time only until CPU, disk, decoder, or remote-source contention becomes limiting. Benchmark rather than assuming linear scaling.

### 13.4 Optimization priorities

1. downmix to mono once;
2. resample once;
3. stream decode;
4. reuse STFT for tempo/key/future features where window/hop needs can be reconciled;
5. cache compact intermediates only when reanalysis economics justify disk cost;
6. vectorize/simple loop optimizations after profiling;
7. avoid architecture-specific SIMD until pure-Go correctness is proven and portable fallback remains;
8. use representative-section “fast” analysis only after accuracy comparison against full track.

### 13.5 Cache policy

Do **not** persist raw spectra for every track by default; the cache would dwarf the catalog.

Good persistent artifacts:

- waveform overview;
- beatgrid/tempo anchors;
- low-rate energy/section summaries;
- optional low-rate chroma timeline if needed for key-change/phrase work.

Recompute high-resolution STFT when algorithms change unless a benchmark proves a compact reusable representation is worthwhile.

---

## 14. Accuracy and benchmark methodology

### 14.1 Benchmark harness

Create a backend test/benchmark tool, for example:

```text
backend/cmd/analysisbench/
backend/internal/analysis/testdata/manifest.json
```

Manifest fields:

```json
{
  "id": "fixture-id",
  "path": "...",
  "license": "CC0/CC-BY/private-local/etc",
  "genre": "house",
  "expectedBpm": 128.0,
  "acceptedMetricBpm": [64.0, 128.0],
  "expectedKey": "F minor",
  "notes": "steady electronic track"
}
```

Never commit copyrighted commercial audio merely because it was used locally for comparison.

### 14.2 BPM corpus

Must include:

- house;
- techno;
- drum & bass;
- hip-hop;
- breakbeat;
- rock/live drums;
- disco;
- ambient;
- acoustic;
- sparse/no-percussion tracks;
- 3/4 or 6/8 where reasonable;
- tempo ramps/switches;
- deliberate 70/140 and 85/170 ambiguity cases.

Metrics:

- exact within ±0.1 BPM for stable electronic fixtures;
- ±0.5 BPM;
- ±1.0 BPM;
- Accuracy1 / strict chosen metrical level;
- Accuracy2-style metrical allowance (half/double where defined by benchmark policy);
- half/double rate reported separately;
- no-result rate;
- confidence vs actual correctness.

### 14.3 Key corpus

Compare:

1. current JS detector;
2. Go candidates/profiles;
3. permitted open reference outputs;
4. manual outputs from commercial DJ software where the tester has lawful access.

Metrics:

- exact tonic+mode accuracy;
- relative major/minor confusion;
- perfect-fifth/adjacent harmonic error distribution;
- Camelot-compatible accuracy as a separate DJ metric;
- unknown/low-confidence rate;
- calibration curve: confidence bucket vs correctness.

### 14.4 GiantSteps

The `GiantSteps/giantsteps-key-dataset` contains annotations for 604 approximately two-minute Beatport previews and is a valuable established EDM key reference. Its repository instructs users to download preview audio from Beatport; it does not establish that ViiB may redistribute those audio files.

Policy:

- use annotations/reference methodology where lawful;
- do not vendor Beatport previews into ViiB;
- support a local benchmark manifest that developers can populate separately;
- maintain committed synthetic/CC/public-domain fixtures for CI.

Also evaluate GiantSteps tempo annotations under the same licensing rule.

### 14.5 Synthetic fixtures

Generate deterministic CI audio in Go:

- clicks at exact BPMs;
- offbeat/syncopated clicks;
- missing beats;
- tempo ramps;
- additive tonal triads/chord progressions in all 24 major/minor keys;
- detuned A4 references (e.g. 438/442 Hz);
- noise/transient contamination;
- silence/quiet intros.

Synthetic audio cannot prove musical accuracy but is excellent for invariants, regressions, half/double logic, and cross-platform determinism.

### 14.6 Acceptance thresholds before labeling “professional”

Phase 0 must establish thresholds from the baseline corpus rather than inventing a marketing number. At minimum, the new Go detector must:

- materially outperform the current JS BPM detector on strict tempo accuracy;
- materially reduce half/double errors under Automatic mode;
- match or exceed the JS key detector on exact key and improve confidence calibration;
- return unknown/low-confidence rather than plausible-looking garbage on deliberately unclassifiable fixtures;
- remain deterministic across Windows/macOS/Linux for the same decoded PCM within documented floating-point tolerance.

---

## 15. Detailed implementation phases

## Phase 0 — Research, baselines, codec spikes, and acceptance gates

### Objective

Turn this roadmap into measurable algorithm and codec decisions before production schema/UI behavior hardens around an inaccurate detector.

### User value

Prevents ViiB from shipping confident-but-wrong BPM/key data that damages Sync and track-selection trust.

### Current-state gap

Current browser detectors have no durable benchmark suite, no cross-platform backend comparator, and no authoritative codec capability matrix.

### Architecture

Build `analysisbench` and isolated prototype packages behind internal/test-only interfaces. No production UI dependency yet.

### Backend changes

- create benchmark CLI/harness;
- add deterministic synthetic PCM fixtures;
- spike Gonum FFT vs small internal FFT;
- spike full-track streaming vs representative-section analysis;
- validate MP3/Vorbis/WAV/FLAC decoding;
- spike AAC-LC/M4A pure-Go paths with legal review flag;
- implement comparator for current JS outputs via exported fixture results rather than importing browser code into Go.

### Frontend changes

None required beyond optional developer-only export of baseline detector results.

### Database changes

None required.

### APIs/events

None production. Benchmark CLI outputs JSON/CSV.

### Candidate Go packages

- existing `hajimehoshi/go-mp3` — Pure Go, Apache-2.0;
- existing `jfreymuth/oggvorbis` — Pure Go, MIT;
- `gonum.org/v1/gonum/dsp/fourier` — Pure Go, BSD-style;
- `mewkiz/flac` — Pure Go, Unlicense;
- evaluate `tphakala/go-aac` — Pure Go, LGPL-2.1-or-later, legal gate.

### Licensing considerations

Create a dependency decision record. Do not import GPL/AGPL benchmark code into ViiB production packages. Development-only external comparators must be optional and documented.

### Performance considerations

Benchmark allocation patterns and streaming memory early. Measure decode separately from DSP.

### Failure modes

- benchmark corpus too EDM-heavy;
- commercial/reference outputs treated as ground truth;
- decoder errors confused with detector errors;
- confidence tuned on training corpus only;
- AAC technical success accepted without license review.

### Unit tests

Synthetic FFT/STFT, resample, tone/chroma, onset, tempo-candidate invariants.

### Integration tests

Benchmark CLI over MP3/WAV/Vorbis/FLAC fixtures.

### Accuracy tests

Current JS vs Go candidates on labeled corpus.

### Cross-platform tests

Run same committed fixtures on Windows/macOS/Linux; compare JSON within tolerance.

### Acceptance criteria

- documented decoder matrix with pass/fail corpus;
- chosen FFT/resample approach;
- reproducible baseline metrics for JS BPM/key;
- selected BPM and key algorithms with quantified improvement target;
- license classification for every candidate;
- no production feature exposed yet.

### Dependencies

None.

### Complexity

**Large**

---

## Phase 1 — Pure-Go analysis foundation and persistence

### Objective

Create the shared streaming audio-analysis substrate and durable result model.

### User value

Makes analysis persistent, reproducible, source-transparent, and extensible.

### Current-state gap

Authoritative analysis is renderer/deck-load scoped; backend waveform decoding is MP3-only.

### Architecture

Implement `backend/internal/analysis/{audio,dsp,cache,jobs}` foundations and result/version types.

### Backend changes

- source resolver for local/Plex canonical songs;
- decoder registry;
- float32 PCM normalization;
- downmix/resample;
- shared window/STFT adapter;
- analysis service interface;
- fingerprint/staleness logic;
- effective-feature resolver;
- extend/generalize waveform accumulation if benchmark supports sharing decode.

### Frontend changes

Add TypeScript analysis result/status types but no major DJ UI yet.

### Database changes

- create `track_analysis`;
- create `track_analysis_artifacts`;
- create `track_analysis_overrides` or equivalent;
- migration preserving legacy `songs.bpm`.

### APIs/events

Read-only result/status endpoint and internal result-updated event are sufficient initially.

### Candidate Go packages

Chosen Phase 0 decoder/FFT packages only. Favor existing dependencies.

### Licensing considerations

All production packages must have approved redistribution classification. Generate/update third-party notice inventory if needed.

### Performance considerations

Streaming bounded memory; zero duplicate decode per analysis pass; context cancellation checks at chunk/stage boundaries.

### Failure modes

- source disappears mid-read;
- Plex token/source outage;
- unsupported codec;
- stale fingerprint race;
- corrupt frames;
- long track memory growth.

### Unit tests

Decoder contract, downmix, resample, version invalidation, fingerprint comparison, scalar/artifact encode/decode.

### Integration tests

Local MP3/Vorbis/WAV/FLAC + mocked/range-aware Plex source stream.

### Accuracy tests

DSP primitives against known synthetic signals.

### Cross-platform tests

Decoder and SQLite artifact determinism on Windows/macOS/Linux.

### Acceptance criteria

- one song can be resolved, decoded, normalized, and streamed through analysis primitives in Go;
- result can be persisted/reloaded with source/version metadata;
- stale source invalidates result;
- local/Plex use same `song_id` model;
- no CGO introduced without explicit ADR.

### Dependencies

Phase 0 decisions.

### Complexity

**Large**

---

## Phase 2 — Production BPM/tempo analyzer

### Objective

Deliver measured fractional static BPM with confidence, alternate metrical candidate, and robust half/double handling.

### User value

Reliable tempo sorting, matching, and a trustworthy prerequisite for Sync.

### Current-state gap

Current browser detector uses short low-frequency peak intervals and stores only deck-session state; catalog BPM may be AI-estimated.

### Architecture

`analysis/rhythm`: onset strength -> periodicity/tempogram -> candidate scoring -> section consensus -> confidence.

### Backend changes

- spectral-flux onset envelope;
- adaptive normalization;
- autocorrelation/tempogram candidate generation;
- candidate scoring including half/double relation;
- configurable BPM range/prior;
- tempo stability diagnostic;
- persistence and effective BPM resolver.

### Frontend changes

Minimal settings surface for BPM analysis range; status/details developer view.

### Database changes

Populate BPM scalar/confidence/source/alternate/stability fields in `track_analysis`.

### APIs/events

Analyze BPM feature flag; manual range option in analysis request.

### Candidate Go packages

No new runtime package expected beyond Phase 1 DSP stack.

### Licensing considerations

Independent implementation from public DSP literature; do not translate `bpm-tools`, Mixxx, Essentia, or other copyleft code.

### Performance considerations

Downsample mono aggressively after validation; retain onset envelope only at low rate; reuse STFT if key analysis is included in same job.

### Failure modes

- half/double selection;
- weak/no percussion;
- breakdown-heavy first minute;
- live drums;
- dynamic tempo;
- 3/4 interpretation;
- silence/short track.

### Unit tests

Exact click BPMs, half/double ambiguous patterns, jitter, missing beats, tempo ramps, silence.

### Integration tests

Genre-stratified licensed/private benchmark manifest.

### Accuracy tests

Strict ±0.5/±1, half/double rate, no-result rate, confidence calibration.

### Cross-platform tests

Same fixture outputs/tolerance plus CPU/RSS benchmark on Windows/macOS.

### Acceptance criteria

- beats current JS baseline by Phase 0 target;
- no hardcoded 120 BPM “success” fallback;
- fractional BPM persisted;
- half/double correction metadata/UI path exists;
- low-confidence result is distinguishable from reliable result;
- DJ performance not yet forced to trust measured BPM until Phase 5 integration gate.

### Dependencies

Phase 1.

### Complexity

**Large**

---

## Phase 3 — Production key + Camelot/Open Key analyzer

### Objective

Deliver persistent dominant musical key, mode, confidence, and derived DJ notations.

### User value

Library-wide harmonic preparation instead of key becoming available only after a track is loaded.

### Current-state gap

Current JS key detector is session-only, first-section oriented, and library key cache is ephemeral.

### Architecture

`analysis/tonal`: STFT -> tuned/weighted chroma/HPCP -> segment summaries -> 24-key profiles -> robust voting -> confidence.

### Backend changes

- chroma/HPCP extractor;
- profile variants selected by benchmark;
- tuning compensation if Phase 0/3 benchmark justifies;
- segment-level voting;
- confidence features;
- canonical tonic/mode and notation conversion utilities.

### Frontend changes

Key notation preference model; analysis status data wiring.

### Database changes

Populate tonic/mode/confidence/source + derived Camelot/Open Key fields.

### APIs/events

Analyze Key feature flag; manual key override endpoint/schema support.

### Candidate Go packages

Phase 1 FFT/DSP stack; no keyfinder native library.

### Licensing considerations

Krumhansl/Temperley-style profile values and published algorithms must be sourced/documented appropriately. No GPL `libkeyfinder` code translation.

### Performance considerations

Reuse shared spectral frames with tempo where practical; aggregate segments online.

### Failure modes

- modal ambiguity;
- relative major/minor confusion;
- sparse/no-tonality material;
- tuning deviation;
- key modulation;
- percussive/noisy tracks;
- intros in a different key from main body.

### Unit tests

Synthetic chords/scales in all 24 keys, detuning, mixtures/noise, modulation fixtures.

### Integration tests

Labeled licensed/private music corpus and GiantSteps-local optional manifest.

### Accuracy tests

Exact key, relative confusion, Camelot-compatible result, confidence calibration, per-genre confusion matrix.

### Cross-platform tests

Deterministic notation and close-enough floating scores/results across target OS/architectures.

### Acceptance criteria

- meets/exceeds JS exact-key baseline by agreed margin;
- confidence identifies difficult tracks better than current JS confidence;
- one canonical key maps correctly to all display notations;
- library no longer requires deck-load session cache for key;
- manual override survives reanalysis according to lock policy.

### Dependencies

Phase 1; may run in parallel with Phase 2 after foundation stabilizes.

### Complexity

**Large**

---

## Phase 4 — Library analysis service and professional lifecycle

### Objective

Turn analyzers into a dependable library-preparation workflow.

### User value

DJs can prepare thousands of tracks before a set, see progress, cancel, resume, and trust that only stale/missing work is repeated.

### Current-state gap

Analysis occurs opportunistically on deck load and has no durable batch lifecycle.

### Architecture

Dedicated analysis scheduler/runner using existing `operation_jobs`, prioritized work queue, per-track validity checks.

### Backend changes

- job type(s) for track analysis;
- selection expansion (IDs/playlist/missing/stale/all);
- bounded worker pool;
- pause dispatch/cancel;
- retry policy;
- auto-analyze integration after scan/Plex sync;
- performance-mode throttle signal;
- progress aggregation;
- stale/skip logic.

### Frontend changes

- Library Analysis panel/dialog;
- Analyze New Tracks setting;
- separate Plex auto-analysis setting;
- progress, pause/cancel, failed-item summary;
- selected/playlist/missing/all actions.

### Database changes

Reuse `operation_jobs`; add only analysis-specific indexes/state if benchmark requires.

### APIs/events

Create/list/get/cancel/pause/resume analysis jobs; SSE job/result events.

### Candidate Go packages

Standard library concurrency/context; no worker-pool dependency required unless existing repo utility fits.

### Licensing considerations

None beyond prior phases.

### Performance considerations

Adaptive/conservative workers; disk/network caps; reduce concurrency during playback; no real-time audio-thread work.

### Failure modes

- app restart;
- source disappears;
- Plex offline/auth expires;
- library changes during job;
- repeated poison/corrupt file;
- cancellation race;
- duplicate overlapping jobs.

### Unit tests

Priority ordering, skip/stale logic, cancellation, dedupe, retry limits, restart recovery.

### Integration tests

1k synthetic catalog job with controlled failures; mocked Plex outage/recovery.

### Accuracy tests

Ensure persisted output equals direct analyzer output for same source/version.

### Cross-platform tests

Long-running job on Windows/macOS; sleep/wake and app restart where automation supports it.

### Acceptance criteria

- Analyze Missing/Selected/All works durably;
- canceled jobs stop dispatching promptly;
- restart does not leave “running” ghosts;
- already-valid tracks are skipped;
- algorithm/source changes make only relevant tracks stale;
- active DJ playback reduces analysis pressure without audio dropout.

### Dependencies

Phases 1–3.

### Complexity

**Large**

---

## Phase 5 — DJv2 persistent analysis and harmonic-mixing UX

### Objective

Make measured BPM/key usable at performance speed in `DJLibraryBrowserV2` and deck workflows.

### User value

DJs can sort/filter/select compatible tracks immediately and answer “what should I mix next?” without loading every candidate.

### Current-state gap

BPM source is ambiguous and key is session cached after deck load; no durable Camelot-first workflow.

### Architecture

Frontend consumes effective track features already present in catalog/API responses or a keyed analysis map; compatibility is a pure deterministic presentation/ranking layer.

### Backend changes

- expose effective BPM/key/notation/status in catalog/API efficiently;
- update AI DJ/DJ services to prefer measured BPM;
- provide filtered query endpoint only if client-scale profiling requires it.

### Frontend changes

- replace session key cache with persisted key;
- Camelot default preference;
- BPM/key/Camelot columns;
- status/confidence affordances;
- compatible key and BPM-tolerance filters;
- Deck A/B/master compatibility sorting;
- explicit harmonic relation highlighting;
- manual BPM/key edit actions;
- hydrate deck `originalBpm`/key from persistent analysis before falling back to on-load analysis.

### Database changes

No new schema expected beyond indexes for BPM/key/status if server-side filtering is introduced.

### APIs/events

Catalog result updates when analysis completes; manual override update endpoint.

### Candidate Go packages

None.

### Licensing considerations

Camelot mapping can be implemented internally as music-theory data; no need for a Node dependency.

### Performance considerations

Compatibility computation is scalar and must not trigger deck/audio rerenders. Preserve Virtuoso row stability and the existing drawer memo boundaries.

### Failure modes

- measured BPM arrives after load and changes deck semantics;
- manual override overwritten;
- unknown/low-confidence key falsely highlighted;
- pitch/key-shift makes original-key compatibility stale;
- 50k row sort/filter jank.

### Unit tests

Camelot mapping, relation classes, BPM percentage distance, effective-value precedence, notation switching.

### Integration tests

Extend `scripts/dj-overlay-audit.mjs` with fixture BPM/key/status and compatible filtering while playback continues.

### Accuracy tests

Golden compatibility cases; no “compatible” label when key is unknown/low-confidence per product threshold.

### Cross-platform tests

1080p/1440p Windows/macOS Wails UI, keyboard/focus, large-library virtualization, active playback.

### Acceptance criteria

- persistent measured BPM/key visible before deck load;
- Camelot/Traditional/Open Key switch without reanalysis;
- same/adjacent/relative key filters work;
- BPM tolerance uses relative percentage;
- no session `analyzedKeyCache` required for normal rows;
- Sync uses manual/measured BPM, not hidden AI estimate;
- existing drawer geometry/performance audit remains green.

### Dependencies

Phases 2–4.

### Complexity

**Large**

---

## Phase 6 — Beatgrid, downbeat, and dynamic-tempo analysis

### Objective

Create real timing analysis that can support precise Sync, quantized loops/jumps, and live-drum/tempo-drift material.

### User value

More reliable Sync and performance tools, especially outside quantized electronic dance music.

### Current-state gap

ViiB currently generates uniform beat timestamps from one BPM; it does not detect beat/downbeat positions.

### Architecture

Beat tracker consumes onset/tempo features, outputs straight or dynamic beat representation; downbeat stage adds bar phase; artifact is versioned and manually editable.

### Backend changes

- beat-phase tracker;
- downbeat estimator;
- straight-grid constructor from detected phase;
- local tempo/beat tracking for dynamic material;
- compressed artifact codec;
- validation/edit operations;
- deck engine hydration from persisted beatgrid.

### Frontend changes

- beatgrid editor;
- first-downbeat marker;
- shift grid;
- BPM fine adjustment;
- add/move tempo anchor for dynamic grid;
- halve/double;
- tap;
- lock;
- Straight/Dynamic badge.

### Database changes

Persist beatgrid artifact + manual override/lock reference and analyzer version.

### APIs/events

Get/update/reset beatgrid; reanalyze preserving/clearing manual edits according to explicit option.

### Candidate Go packages

No copyleft beat-tracker library. Implement from public literature atop Phase 1 DSP unless a permissive pure-Go detector is independently validated.

### Licensing considerations

Mixxx/QM/Vamp/Essentia implementations are study references only where their licenses conflict. Cite papers used for independent implementation.

### Performance considerations

Beat timeline storage must be compact; dynamic tracking likely requires full-track onset envelope but not full PCM.

### Failure modes

- wrong first downbeat;
- beat phase offset despite correct BPM;
- swing/shuffle;
- missing/extra beat detections;
- tempo drift;
- meter changes;
- manual edit reanalysis loss.

### Unit tests

Synthetic click tracks with offsets, meter, missing beats, jitter, ramps, step tempo changes.

### Integration tests

Beatgrid-to-Sync/loop/beat-jump correctness with generated audio fixtures.

### Accuracy tests

Beat F-measure/tolerance, downbeat accuracy, drift across track length, dynamic segment error.

### Cross-platform tests

Playback-phase behavior under Windows/macOS Wails audio stacks; Linux backend artifact correctness.

### Acceptance criteria

- constant grid aligns detected beat phase, not zero offset;
- long stable tracks do not visibly drift due to BPM rounding;
- dynamic fixtures retain local tempo changes;
- manual anchors/locks survive normal reanalysis;
- Sync/loops consume persisted beatgrid artifact;
- no audio dropouts under background analysis.

### Dependencies

Phases 1, 2, 4, 5.

### Complexity

**Very Large**

---

## Phase 7 — Advanced DJ intelligence: energy, structure, cues, recommendations

### Objective

Build higher-order preparation and transition intelligence on the same PCM/features/results.

### User value

Faster set building and better next-track decisions without replacing DJ judgment.

### Current-state gap

AI metadata has coarse mood/energy/tempo descriptors, but no unified audio-derived DJ energy/phrase/cue/transition feature model.

### Architecture

Incremental feature analyzers consume shared spectral/onset/beatgrid summaries and write typed artifacts/scalars.

### Backend changes

Prioritized subphases:

1. measured energy/intensity curve;
2. LUFS/true-peak analysis aligned with existing ReplayGain strategy;
3. section/novelty segmentation;
4. phrase/downbeat-aligned structure;
5. mix-in/mix-out candidate regions;
6. cue-point suggestions;
7. transition feature vector/scoring;
8. AI DJ scorer consumes the same measured features.

### Frontend changes

- Energy column/curve;
- section markers on waveform;
- suggested cues requiring user acceptance/edit;
- Recommended Next panel/filter;
- explainable transition dimensions.

### Database changes

Extend scalar `track_analysis` only for query-worthy values; store time-series/sections in artifacts.

### APIs/events

Feature-specific analysis flags and recommendation query; no cloud dependency required.

### Candidate Go packages

Prefer existing DSP foundation. Avoid ML runtime until a later evidence-based ADR; classical MIR first.

### Licensing considerations

Any model weights require separate model license, redistribution, provenance, and size review. No model should be silently downloaded from cloud for core offline behavior.

### Performance considerations

Reuse beat/chroma/onset summaries; schedule expensive structure analysis at lower priority; allow selective features.

### Failure modes

- energy conflated with loudness;
- automatic cue points treated as authoritative;
- phrase classifier genre bias;
- opaque transition score;
- AI DJ overfits numeric compatibility and loses musical diversity.

### Unit tests

Feature invariants, section artifact codecs, score monotonicity/weight sanity.

### Integration tests

Recommendation explanations over fixture catalogs; AI DJ measured-feature fallback behavior.

### Accuracy tests

Human-labeled section/cue subset; ranking evaluation with expert-reviewed transition pairs; do not use vendor claims as ground truth.

### Cross-platform tests

Analysis determinism and recommendation parity; UI performance with new columns/markers.

### Acceptance criteria

- every recommendation exposes component rationale;
- human DJ can ignore/override cues/scores;
- AI DJ and DJv2 use one effective-feature source;
- no required cloud service;
- advanced analysis can be disabled/selectively run.

### Dependencies

Phases 1–6 as appropriate; energy can begin after Phase 5, phrase/cues should wait for beat/downbeat.

### Complexity

**Very Large**

---

## 16. Global acceptance criteria

The professional track-analysis program is successful when:

### Correctness

- BPM/key are measured from audio and carry provenance/confidence;
- manual values are never silently overwritten;
- unknown is preferred over fabricated certainty;
- measured and AI-inferred metadata are distinguishable;
- beatgrid is not claimed until actual beat phase/positions are analyzed.

### Architecture

- one canonical `song_id` works for local and Plex;
- no DJ-only catalog;
- analysis is backend-owned and locally executable;
- decoder/DSP service is reusable outside DJv2;
- job lifecycle reuses ViiB durable operations infrastructure;
- algorithm/source versioning supports targeted reanalysis.

### UX

- DJs can analyze selected/missing/all tracks;
- see progress and failures;
- choose key notation;
- sort/filter by measured BPM/key;
- identify same/adjacent/relative harmonic candidates;
- correct BPM/key cheaply;
- analysis does not move or degrade the DJv2 deck workspace.

### Performance

- no audible dropout or timing regression during DJ performance;
- background concurrency is bounded/throttled;
- analysis streams instead of retaining full-track PCM;
- large-library UI remains virtualized/responsive.

### Portability

- Windows/macOS/Linux backend tests pass;
- no Python/Node runtime production requirement;
- no dependency on installed FFmpeg/SoX;
- CGO/native/WASM dependencies, if any, are explicit approved exceptions.

### Licensing

- every added dependency has SPDX/classification notes;
- no copied/translated GPL/AGPL implementation enters MIT ViiB unintentionally;
- artifact/model/codec redistribution obligations are documented.

---

## 17. Risks and mitigations

| Risk | Impact | Mitigation |
|---|---|---|
| Half/double BPM errors | Wrong Sync/filtering | retain alternate candidate, BPM ranges, correction controls, strict benchmark |
| Browser/backend results disagree | User distrust during migration | choose one authoritative Go result; JS used only as temporary baseline/fallback |
| AAC/M4A decoder gap | Common DJ files cannot be analyzed | Phase 0 codec gate; clear unsupported status; evaluate pure-Go LGPL path and optional WASM only deliberately |
| Plex bulk analysis overloads PMS/network | Poor server/network behavior | separate opt-in, per-source concurrency 1, pause/throttle, cache result |
| Analysis blocks DJ audio | Critical performance failure | separate workers, reduce concurrency in Performance mode, no DSP on audio callback |
| Copyleft source gets translated into MIT code | Licensing exposure | clean implementation policy, dependency ADRs, source provenance notes |
| Confidence appears probabilistic but is not | Misleading UX | calibrated buckets; label diagnostic score appropriately |
| Reanalysis destroys edits | Professional workflow failure | overrides/locks separate from detected result; explicit clear/reset semantics |
| SQLite grows excessively with beat rows | Large-library degradation | compressed typed artifacts rather than row-per-beat |
| Algorithm overfit to EDM | Weak general DJ usefulness | genre-diverse corpus incl. live/acoustic/hip-hop/disco |
| Full-track analysis too slow | Poor onboarding | optimize after accuracy; incremental/background lifecycle; validated fast mode later |
| AI DJ changes unexpectedly when measured BPM arrives | Regression in set generation | feature precedence tests, fallback logging, rollout flag if needed |
| Source changes during analysis | Stale result | fingerprint before/after where feasible; discard result on revision mismatch |

---

## 18. Rejected alternatives

### Keep browser Web Audio as authoritative analyzer

**Rejected as primary architecture.** Good codec coverage and existing code are attractive, but results depend on WebView/platform codecs, renderer lifecycle, and competition with performance audio. Keep temporarily for baseline/fallback comparison only.

### Directly port `benjojo/bpm` / `bpm-tools`

**Rejected.** `benjojo/bpm` is explicitly GPLv2 because it is a direct port. ViiB is MIT. Study algorithm lineage; do not translate implementation code.

### Embed Mixxx/libkeyfinder source

**Rejected.** Excellent reference, but Mixxx is GPLv2-or-later and libkeyfinder GPLv3-or-later. Also C++/native integration conflicts with the pure-Go target.

### Embed Essentia/Essentia.js

**Rejected for default production.** Technically capable but principal license is AGPLv3 and the stack is C++/WASM with broader dependency/licensing complexity. It remains valuable as an optional development benchmark.

### Require installed FFmpeg or SoX

**Rejected.** Violates self-contained/offline redistribution goals, creates environment/version drift, and complicates support.

### Bundle a native FFmpeg binary immediately

**Rejected for initial phases.** It solves codecs but adds substantial binary size, native redistribution/license work, update/security responsibilities, and a second execution boundary. Reconsider only after pure-Go/WASM codec evaluation proves inadequate for real libraries.

### Use `songs.bpm` as-is for professional Sync

**Rejected.** Current enrichment can estimate BPM from metadata/genre context rather than audio. Provenance is insufficient.

### Put all new fields directly in `songs`

**Rejected as sole model.** A few scalar effective values could eventually be denormalized, but versioning, confidence, source fingerprints, manual overrides, and future time-series artifacts justify a dedicated analysis model.

### Create a separate DJ database/catalog

**Rejected.** Violates ViiB’s canonical source-transparent catalog architecture and would split AI DJ/human DJ metadata.

### Persist every beat as a SQLite row

**Rejected for large libraries.** A 50k-track library can easily create tens of millions of beat rows. Use compact versioned artifacts/tempo segments.

### Hard-code a single composite transition score now

**Rejected.** BPM + key alone is not sufficient to imply transition quality, and arbitrary weights become opaque product behavior. Expose dimensions first.

### Make Camelot mapping a runtime Node dependency

**Rejected.** The mapping is tiny deterministic music-theory data and ViiB already has equivalent TypeScript logic. Implement shared Go mapping and mirror/test frontend display if needed.

### Treat current uniform generated grid as “beatgrid analysis”

**Rejected.** It contains no detected phase/downbeat evidence and can drift when BPM is slightly wrong.

---

## 19. Future opportunities

Once the Track Analysis Engine is trusted, the same foundation enables:

### DJ performance

- true beat-phase Sync;
- dynamic tempo Sync;
- phrase-aware loops/beat jumps;
- automatic downbeat-aware cue suggestions;
- mix-in/mix-out regions;
- waveform section overlays;
- key-change display;
- effective-key matching after pitch shift.

### Library intelligence

- Smart Playlists by measured BPM/key/energy;
- “not analyzed” and “low confidence” maintenance views;
- duplicate/remaster comparison using analysis fingerprints where appropriate;
- warm-up/peak/cool-down energy planning;
- DJ set crates generated by BPM/key/energy trajectories.

### AI DJ

- replace inferred BPM with measured BPM where available;
- harmonic compatibility as a deterministic scoring feature;
- energy curve planning;
- phrase-aware transition timing;
- transition-score explanations;
- style-aware, but not genre-hardcoded, metrical interpretation.

### Interoperability

After internal semantics stabilize, investigate import/export of:

- Rekordbox XML/device-library metadata where legally/documentedly supported;
- Serato-readable tag fields where supported;
- Traktor NML;
- standard file tags for BPM/key;
- M3U/CSV set-preparation exports.

Interoperability should map into ViiB detected/manual provenance and never overwrite protected user edits blindly.

### Optional future ML

Classical DSP is sufficient for the first roadmap. Later ML may improve:

- downbeats;
- phrase/section classification;
- vocal/instrumental regions;
- energy/style embeddings;
- cue suggestions.

Any model adoption requires:

- offline inference path;
- model license/redistribution review;
- deterministic versioned model identity;
- CPU/memory/binary-size benchmarks;
- no silent cloud dependency;
- baseline showing enough accuracy gain to justify complexity.

---

## 20. Unresolved technical questions

These should be answered in Phase 0, not guessed during later implementation:

1. Which pure-Go FFT implementation gives the best correctness/allocation tradeoff: Gonum or a small internal implementation?
2. What analysis sample rate and STFT geometry best balance BPM and key reuse without materially degrading either task?
3. Is shared STFT practical for both onset and chroma, or should they share only decoded/resampled PCM?
4. How much accuracy is lost by 60–90 seconds of representative-section analysis versus full-track analysis by genre?
5. Which chroma/HPCP profile combination wins on ViiB’s benchmark corpus?
6. What confidence features best identify key uncertainty/modulation?
7. What candidate/prior policy minimizes half/double errors without genre hardcoding?
8. Can M4A/AAC-LC be supported through a pure-Go stack with acceptable licensing and profile coverage in ViiB’s static binaries?
9. What is the correct pure-Go Opus strategy for `.opus` catalog entries?
10. How should legacy AI-estimated `songs.bpm` be surfaced during migration—hidden provenance, explicit `Estimated` badge, or non-DJ fallback only?
11. Should backend waveform cache be migrated into `track_analysis_artifacts` or remain a specialized table sharing only the decoder pipeline?
12. How should Plex source revision/fingerprint be computed without downloading media twice?
13. What worker count keeps Windows/macOS Wails deck playback glitch-free on representative 4/8/16-core machines?
14. At what library size does DJLibraryBrowserV2 need server-side BPM/key filtering rather than client filtering?
15. What user-facing threshold turns confidence into High/Medium/Low without implying probability?

---

## 21. Dependency classification register

This register covers dependencies discussed by this roadmap. Implementation PRs must update it with exact versions.

| Dependency / project | Classification | License | Proposed use |
|---|---|---|---|
| `github.com/hajimehoshi/go-mp3` | Pure Go | Apache-2.0 | Existing MP3 PCM decode; retain |
| `github.com/jfreymuth/oggvorbis` | Pure Go | MIT | Existing Ogg/Vorbis PCM decode; retain |
| `gonum.org/v1/gonum/dsp/fourier` | Pure Go | BSD-style | Candidate FFT/STFT primitive; benchmark first |
| `github.com/mewkiz/flac` | Pure Go | Unlicense | Candidate FLAC decode |
| in-house WAV/AIFF decoder | Pure Go | ViiB MIT | Preferred simple uncompressed PCM container support |
| `github.com/tphakala/go-aac` | Pure Go | LGPL-2.1-or-later | Technical codec spike only pending legal/static-distribution review |
| Essentia | CGO/native if embedded native; can be compiled to WASM | AGPLv3 principal license | Development/reference benchmark only; reject default production |
| Essentia.js | WASM + JavaScript | AGPLv3 principal license | Development/reference benchmark only |
| `libraz/libsonare` | CGO/native or WASM depending binding | Apache-2.0 per project | Benchmark/reference; possible future optional WASM path after validation |
| `mixxxdj/mixxx` | CGO/native application architecture | GPLv2-or-later | Architectural study only |
| `mixxxdj/libkeyfinder` | CGO/native C++ | GPLv3-or-later | Key algorithm study/benchmark only |
| `benjojo/bpm` | Pure Go code, but copyleft implementation | GPLv2 | Study only; no source reuse |
| `dlepaux/realtime-bpm-analyzer` | Browser JavaScript/TypeScript | Apache-2.0 | BPM algorithm reference/baseline only |
| `ifeelvoid/keyfinder` | Native Swift/macOS | MIT | Algorithm/reference comparison only |
| `libraz/bpm-detector` | Python + native/scientific stack | MIT project; dependency licenses vary | Algorithm/reference comparison only |
| `Rexaintreal/Resonate` | Browser JS + Python/Flask + cloud pieces | MIT | Low-priority conceptual reference |
| installed FFmpeg/SoX | External executable | mixed licenses | Rejected |
| cloud audio-analysis API | Cloud/API dependency | provider-specific | Rejected for core/offline engine |

---

## 22. Recommended implementation PR decomposition

Keep implementation PRs reviewable. Suggested sequence after this roadmap:

1. `analysis/phase0-benchmarks-and-codec-matrix`
2. `analysis/foundation-pcm-dsp-persistence`
3. `analysis/tempo-v1`
4. `analysis/key-v1`
5. `analysis/library-jobs`
6. `djv2/persistent-analysis-library-ux`
7. `analysis/beatgrid-downbeat-v1`
8. `djv2/beatgrid-editor-sync-integration`
9. `analysis/energy-structure-v1`
10. `djv2/transition-recommendations`
11. `ai-dj/measured-track-features`

Each phase PR should update this roadmap’s status table and include benchmark deltas.

---

## 23. Source and reference links

### ViiB current implementation

- `pages/DJModeV2.tsx`
- `components/dj/v2/DJLibraryBrowserV2.tsx`
- `components/dj/v2/DJLibraryDrawer.tsx`
- `components/dj/DJLibraryBrowser.tsx`
- `hooks/useDJAudioEngine.ts`
- `slices/djMixerSlice.ts`
- `lib/bpmDetection.ts`
- `lib/keyDetection.ts`
- `backend/internal/api/dj_waveform.go`
- `backend/internal/audio/metadata.go`
- `backend/internal/audio/ogg_to_mp3.go`
- `backend/internal/scanner/scanner.go`
- `backend/internal/db/db.go`
- `backend/internal/db/jobs_schema.go`
- `backend/internal/db/jobs_repository.go`
- `backend/internal/dj/scoring.go`
- `backend/internal/dj/sequencer.go`
- `backend/go.mod`
- `docs/dj-mode.md`
- `docs/architecture.md`
- `docs/plex-music.md`

### Professional DJ product documentation

- rekordbox 7 manual (current manual indexed during research): https://cdn.rekordbox.com/files/20260409151936/rekordbox7.214_manual_EN.pdf
- rekordbox overview / analysis: https://rekordbox.com/en/feature/overview/
- Serato — Preparing and Analyzing Your Files: https://support.serato.com/hc/en-us/articles/202538540-Preparing-and-Analyzing-Your-Files
- Serato — Analysis setting: https://support.serato.com/hc/en-us/articles/14361435885327-Analysis-setting
- Serato — Beatgrids: https://support.serato.com/hc/en-us/articles/202856014-Beatgrids-in-Serato-DJ-Pro
- Traktor Pro manual — Managing Your Track Collection: https://docs.native-instruments.com/ni-tech-manuals/traktor-pro-manual/en/managing-your-track-collection
- Traktor Pro manual — Preferences / Analyze Options: https://docs.native-instruments.com/ni-tech-manuals/traktor-pro-manual/en/preferences
- Traktor Pro manual — Advanced Usage / Beatgrids: https://docs.native-instruments.com/ni-tech-manuals/traktor-pro-manual/en/advanced-usage-tutorials
- Traktor Pro 4 product / Flexible Beatgrids: https://www.native-instruments.com/products/traktor-pro
- Mixed In Key — Analyze your music library: https://mixedinkey.com/workflows/analyze-your-music-library/
- Mixed In Key 11 overview: https://mixedinkey.com/learn-more/
- VirtualDJ — Analyze Tracks: https://virtualdj.com/manuals/virtualdj/interface/database/analyze.html
- VirtualDJ — BPM Editor: https://virtualdj.com/manuals/virtualdj/editors/bpmeditor.html
- Algoriddim djay — Beatgrids: https://help.algoriddim.com/user-manual/djay-pro-windows/dj-tools/beatgrids-bpm-sync/beatgrids
- Algoriddim djay — Adjusting BPM: https://help.algoriddim.com/user-manual/djay-pro-windows/dj-tools/beatgrids-bpm-sync/adjusting-bpm
- Mixxx 2.5 manual: https://manual.mixxx.org/2.5/en/
- Mixxx — Beat Detection: https://manual.mixxx.org/2.5/en/chapters/preferences/beat_detection
- Mixxx — Developer Guide Analyzers: https://github.com/mixxxdj/mixxx/wiki/Developer-Guide-Analysers

### Supplied/open-source references

- https://github.com/jackbittiner/camelot-wheel
- https://github.com/ifeelvoid/keyfinder
- https://github.com/lucaderumier/mixflow
- https://github.com/benjojo/bpm
- https://github.com/dlepaux/realtime-bpm-analyzer
- https://github.com/libraz/bpm-detector
- https://github.com/Rexaintreal/Resonate
- https://github.com/mixxxdj/mixxx
- https://github.com/mixxxdj/libkeyfinder
- https://github.com/MTG/essentia
- https://github.com/MTG/essentia.js
- https://github.com/libraz/libsonare

### Candidate Go codec/DSP references

- https://github.com/hajimehoshi/go-mp3
- https://github.com/jfreymuth/oggvorbis
- https://github.com/mewkiz/flac
- https://github.com/tphakala/go-aac
- https://pkg.go.dev/gonum.org/v1/gonum/dsp/fourier

### Benchmark/data references

- GiantSteps Key Dataset: https://github.com/GiantSteps/giantsteps-key-dataset
- GiantSteps Tempo Dataset: https://github.com/GiantSteps/giantsteps-tempo-dataset
- Knees et al., ISMIR 2015, “Two data sets for tempo estimation and key detection in electronic dance music annotated from user corrections” (referenced by the GiantSteps repositories)

### DSP literature / concepts to use for independent implementation

Implementation sessions should trace algorithm choices to published DSP/MIR literature rather than copyleft source translations. At minimum review:

- Krumhansl/Kessler tonal hierarchy and key profiles;
- Temperley key-profile approaches;
- chroma/HPCP literature for tonal descriptors and tuning robustness;
- spectral-flux onset detection literature;
- autocorrelation/tempogram tempo estimation literature;
- dynamic-programming/probabilistic beat-tracking literature;
- standard loudness work such as ITU-R BS.1770 if/when LUFS is implemented.

---

## 24. Final recommendation

Proceed with the Track Analysis Engine, but make **Phase 0 an accuracy/licensing gate, not ceremonial research**.

The strongest near-term architecture is:

```text
Pure-Go/source-transparent PCM layer
  -> shared streaming DSP foundation
  -> measured fractional BPM + confidence
  -> measured key + confidence + Camelot/Open Key
  -> versioned SQLite track_analysis
  -> durable ViiB operation_jobs analysis queue
  -> DJLibraryBrowserV2 persistent columns/filters/compatibility
  -> later real beatgrid/downbeat/dynamic tempo
  -> later energy/phrase/cues/transition intelligence
  -> same effective features consumed by AI DJ
```

The existing JavaScript BPM/key detectors are not wasted work. They provide a concrete baseline, working Camelot mappings, and a migration comparator. But the professional target should be a backend-owned, versioned, benchmarked system whose outputs survive sessions, work across the canonical local/Plex catalog, and can be trusted by both Sync and track-selection workflows.
