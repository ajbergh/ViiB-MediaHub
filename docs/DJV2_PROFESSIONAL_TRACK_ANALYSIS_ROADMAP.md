# DJv2 Professional Track Analysis & Harmonic Mixing Roadmap

**Status:** In progress — Phase 0 (benchmark harness, comparator, and synthetic corpus)
**Scope:** DJv2 / professional DJ workflow only  
**Research snapshot:** 2026-09-06  
**Repository verification snapshot:** commit `3359371`, branch `main`  
**Toolchain baseline:** Go 1.26.8, Wails v2.11.0, `modernc.org/sqlite` (pure-Go SQLite, no CGO)  
**Target architecture:** Go/Wails, local-first, cross-platform, predominantly pure Go, no CGO unless explicitly justified  
**Repository:** `ajbergh/ViiB-MediaHub`

> This document is a research and implementation plan, not a claim that the described future functionality is already implemented. Sections labeled **Verified current state** describe behavior confirmed in the repository as of the verification snapshot, with `file:line` evidence recorded in §2.0. Sections labeled **Proposed** describe future design.

> **Reader's note on trust.** Every current-state claim in §2 was re-checked against the tree at commit `3359371`. §2.0 is the evidence ledger; §2.3 records the assumptions that survived challenge and the ones that did not. If you are picking this up to implement, read §2.0, §2.3, and Phase 0 first — they contain the corrections that change scope.

### Implementation progress

| Date | Branch | Scope | State | Validation | Notes |
|---|---|---|---|---|---|
| 2026-09-06 | `analysis/phase0-benchmarks-and-codec-matrix` | Phase 0, Slice 1 | Complete | `go test ./internal/analysisbench`; `go run ./cmd/analysisbench -format=json` | Added deterministic synthetic fixtures, bounded WAV header validation, and a machine-readable codec matrix. This is harness infrastructure only; it does not claim production analysis or codec support. |
| 2026-09-07 | `analysis/phase0-benchmarks-and-codec-matrix` | Phase 0, Slice 2 | Complete | `go test -count=1 ./internal/analysisbench`; `go run ./cmd/analysisbench -format=json` | Added a label-only corpus manifest/result schema and held-out comparison report. It measures strict and accepted-metrical BPM accuracy, half/double and unknown rates, plus exact and conservative Camelot-compatible key accuracy. Browser output remains an external JSON export; no Web Audio code is imported into Go. |
| 2026-09-07 | `analysis/phase0-benchmarks-and-codec-matrix` | Phase 0, Slice 3 | Complete | `go test -count=1 ./internal/analysisbench`; `go build -buildvcs=false ./cmd/analysisbench`; `go run ./cmd/analysisbench -format=json` | Expanded committed synthetic CI coverage to 35 fixtures: syncopated and missing-beat tempos, dynamic tempo, deterministic noise, quiet intros, 24 additive major/minor triads, and detuned references. Dynamic fixtures deliberately carry no static BPM claim. |
| 2026-09-07 | `analysis/phase0-benchmarks-and-codec-matrix` | Phase 0, Slice 4 | Complete | `go test -count=1 ./internal/analysisbench`; `go test -run '^$' -bench 'Benchmark(Radix2\|Gonum)FFT8192$' -benchmem ./internal/analysisbench` | Added an isolated radix-2/Gonum correctness and allocation benchmark at the current 8192-point key geometry. On Windows/AMD Ryzen 5 3600, radix-2 measured 208,150 ns/op, 131,075 B/op, 1 alloc; Gonum v0.17.0 measured 745,976 ns/op, 524,290 B/op, 3 allocs. Retain the internal implementation as the provisional Phase 0 baseline; this is not yet a production shared-STFT decision. |
| 2026-09-07 | `analysis/phase0-benchmarks-and-codec-matrix` | Phase 0, Slice 5 | Complete | `go test -count=1 ./internal/api -run 'TestGenerateWaveform'` | Corrected waveform capability reporting so `.opus` is explicitly deferred as Opus rather than grouped with Ogg/Vorbis. No decoder was added; this prevents a false codec-support implication while the Opus spike remains pending. |
| 2026-09-07 | `analysis/phase0-benchmarks-and-codec-matrix` | Phase 0, Slice 6 | Complete | `go test -count=1 ./internal/analysisbench` | Added a bounded PCM16 RIFF/WAVE writer for generated fixtures and verified its output with the Phase 0 WAV inspector, including clipping behavior. This makes synthetic inputs transferable to external baseline tooling without committing commercial audio; it is not the Phase 1 production WAV decoder. |
| 2026-09-07 | `analysis/phase0-benchmarks-and-codec-matrix` | Phase 0, Slice 7 | Complete | `go test -count=1 ./internal/analysisbench` | Added a bounded streaming WAV PCM16 decode spike and round-trip test. The codec matrix now reports WAV PCM16 as decode-spike-ready; float and additional PCM widths remain Phase 1 scope. |
| 2026-09-07 | `analysis/phase0-benchmarks-and-codec-matrix` | Phase 0, Slice 8 | Complete | `go test -count=1 ./internal/analysisbench`; `go build -buildvcs=false ./cmd/analysisbench`; `analysisbench -write-wav-dir <empty-dir>` | Added non-overwriting `-write-wav-dir` output for all 35 synthetic PCM16 fixtures and verified their artifact count and WAV geometry. These generated files are the lawful, reproducible inputs for the pending browser-baseline and codec measurements. |
| 2026-09-07 | `analysis/phase0-benchmarks-and-codec-matrix` | Phase 0, Slice 9 | Complete | `go test -count=1 ./internal/analysisbench`; `analysisbench -manifest internal/analysisbench/testdata/manifest.example.json -results internal/analysisbench/testdata/results.example.json -split held_out` | Added explicit expected-unknown labels and metrics, plus runnable synthetic manifest/result examples. Corpus reports now display the 200-track/held-out deficit alongside the results, so a smoke fixture cannot be mistaken for Phase 0 exit evidence. |
| 2026-09-07 | `analysis/phase0-benchmarks-and-codec-matrix` | Phase 0, Slice 10 | Complete | `go test -count=1 ./internal/analysisbench`; `go test -run '^$' -bench '^BenchmarkPrototypeTempoClick128$' -benchmem ./internal/analysisbench` | Added an isolated onset-interval tempo prototype to establish measurable behavior before Phase 2. It passes stable/fractional/syncopated/missing-beat/noisy/quiet-intro synthetic cases and returns unknown for silence. The 8-second 128 BPM synthetic fixture measured 5,890,878 ns/op, 584,513 B/op, 34 allocs on Windows/AMD Ryzen 5 3600; this is not evidence of real-music accuracy or a production selection. |
| 2026-09-07 | `analysis/phase0-benchmarks-and-codec-matrix` | Phase 0, Slice 11 | Complete | `go test -count=1 ./internal/analysisbench`; `go test -run '^$' -bench '^BenchmarkPrototypeKeyTriad$' -benchmem ./internal/analysisbench` | Added an isolated direct-chroma/Krumhansl-profile key prototype. It identifies all 24 generated major/minor triads and returns unknown for silence. The one-second 22.05 kHz triad measured 18,399,526 ns/op, 8 B/op, 1 alloc on Windows/AMD Ryzen 5 3600; it remains a synthetic profile-plumbing spike, not a Phase 3 production key selection. |
| 2026-09-07 | `analysis/foundation-pcm-dsp-persistence` | Phase 1, Slice 1 | Complete | `go test ./internal/db -run '^TestTrackAnalysisSchema'` | Added additive song-keyed `track_analysis`, `track_analysis_artifacts`, and `track_analysis_overrides` tables with version, provenance, fingerprint, scalar confidence, artifact, and lock fields. Tests prove fractional measured BPM remains separate from `songs.bpm` and related records cascade on song removal. Decoder/DSP/service work remains. |

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
- **Reuse the durable job *state* model, but expect to build the *scheduler*.** ViiB has `operation_jobs` with queued/running/succeeded/failed/canceling/canceled/interrupted states, progress counters, retry, and restart recovery. What it does **not** have is a queue: `createJobV2` calls `go a.runJob(job.ID)` immediately, so `queued` is a status that is written but never *drained* ([v2_jobs.go:75](../backend/internal/api/v2_jobs.go#L75)). There is no worker pool, no concurrency bound, no priority column, no claim/lease, and no paused state. Track analysis should extend the persistence layer and **must** add the scheduler that does not exist yet. This is real Phase 4 scope, not glue — see §2.3-B and §9.4.
- **Analysis queues must survive restart.** Current restart recovery marks `queued`, `running`, *and* `canceling` rows as `interrupted` ([jobs_schema.go:70-78](../backend/internal/db/jobs_schema.go#L70-L78)). For a scan that finishes in minutes that is correct. For a 50,000-track analysis run measured in days, it silently discards all pending work. Durable per-track state in `track_analysis` — not job rows — is what makes an analysis run resumable.
- **Reuse one decoded PCM stream.** Waveform overview, onset/tempo, chroma/key, and later energy/loudness should share decoding, downmixing, resampling, windowing, and where practical spectral frames.
- **Prefer a separate scalar `track_analysis` row plus versioned artifact blobs.** Scalar query fields belong in a queryable table; high-volume beat/phrase/waveform artifacts should not become tens of millions of SQLite rows.
- **Use a conservative first harmonic model.** Initial “compatible key” behavior should be based on established same-key, adjacent Camelot number within the same mode, and relative major/minor moves. ViiB should not present arbitrary numerical compatibility scores as musical truth.
- **Treat codec support as an explicit capability matrix.** MP3 and Ogg/Vorbis already have pure-Go backend decoders in the dependency graph. WAV PCM is straightforward to support in pure Go. FLAC has viable pure-Go options. AAC/M4A remains the highest-risk path because the strongest newly available pure-Go AAC option carries LGPL obligations and incomplete profile coverage. AIFF has an additional, easily missed prerequisite: the scanner cannot currently *ingest* `.aiff`/`.aif` at all, so decoder work without a scanner change is dead code (§2.3-C).
- **A sandboxed WASM codec path is already an available, low-ceremony option — not an exotic future one.** ViiB already compiles, ships, and runs a WASM runtime in the production binary today: `go.senan.xyz/taglib` executes TagLib as WebAssembly on `github.com/tetratelabs/wazero`, and it is on the critical path for all metadata extraction (`go mod why` resolves `internal/audio → go.senan.xyz/taglib → wazero`). The runtime is pure Go, needs no CGO, and its binary-size and security review are already sunk costs. This materially de-risks AAC/M4A and Opus, which are otherwise the two worst codec gaps, and it reorders §7.4. It does **not** make WASM free — per-codec licensing, profile coverage, and throughput still require Phase 0 measurement, and wazero interpretation is materially slower than native Go for tight DSP loops, so WASM belongs at the *decode* boundary only, never in the DSP inner loop.
- **Accuracy gates precede UX claims.** A detector that returns a number is not production-ready. BPM and key require repeatable benchmark corpora, half/double error accounting, confidence calibration, and cross-platform performance measurements.

### Recommended phase order

| Phase | Outcome | Depends on | Parallelizable with | Complexity |
|---|---|---|---|---|
| 0 | Research harness, corpus, codec/algorithm spikes, current-detector baselines | — | — | Large |
| 1 | Pure-Go shared analysis foundation and persistence model | 0 | — | Large |
| 2 | Production static-tempo/BPM analyzer | 1 | 3 | Large |
| 3 | Production key + Camelot/Open Key analyzer | 1 | 2 | Large |
| 4 | Durable library analysis service, scheduler, and lifecycle | 1 | 2, 3 | Large |
| 5 | DJv2 library/harmonic workflow integration | 2, 3, 4 | — | Large |
| 6 | Beatgrid + downbeat + dynamic tempo | 1, 2, 4, 5 | — | Very Large |
| 7 | Energy/structure/cues/transition intelligence and AI DJ reuse | 5 (energy); 6 (phrase/cues) | — | Very Large |

**Critical path:** 0 → 1 → {2, 3, 4 concurrent} → 5 → 6 → 7. Phase 4 depends only on Phase 1's persistence and service interface, not on either analyzer being finished, so the scheduler can be built against a stub analyzer while 2 and 3 proceed. Phase 5 is the first phase that delivers user-visible value, and it is gated on all three of 2, 3, and 4. Nothing before Phase 5 should ship user-facing BPM/key claims.

**Minimum shippable slice.** If the full program cannot be funded, the smallest coherent release is **0 → 1 → 2 → 4 → 5 (BPM only)**: measured fractional BPM with confidence, durable and resumable, sortable and filterable in DJv2, with manual override. Key can follow. The reverse (key first) is also coherent but less valuable, because tempo gates Sync. Do not ship Phase 5 without Phase 4 — persistent analysis with no way to run it in bulk is not a professional workflow.

---

## 2. Current-state audit

### 2.0 Evidence ledger

Every current-state claim below was re-verified against commit `3359371`. This table exists so a future reader can tell which statements are load-bearing facts and which are design opinion, and can re-check them cheaply after the tree moves.

| Claim | Evidence | Verdict |
|---|---|---|
| JS key detector: 8192-point FFT, 65–2093 Hz chroma range, Camelot + Open Key maps, `getKeyCompatibility()` | [keyDetection.ts:123](../lib/keyDetection.ts#L123), [:230-232](../lib/keyDetection.ts#L230-L232), [:293-294](../lib/keyDetection.ts#L293-L294), [:345](../lib/keyDetection.ts#L345) | Confirmed |
| JS BPM detector: 22.05 kHz, 150 Hz low-pass, 60–200 BPM search, 1-BPM histogram, 120 BPM fallback at confidence 0 | [bpmDetection.ts:16-19](../lib/bpmDetection.ts#L16-L19), [:201](../lib/bpmDetection.ts#L201), [:211](../lib/bpmDetection.ts#L211), [:217](../lib/bpmDetection.ts#L217), [:225](../lib/bpmDetection.ts#L225) | Confirmed |
| **Both** JS detectors analyze only the first 30 s | `ANALYSIS_DURATION = 30` [bpmDetection.ts:20](../lib/bpmDetection.ts#L20) (fixed `OfflineAudioContext` length, [:57](../lib/bpmDetection.ts#L57)); `detectKey(url, { duration: 30 })` [useDJAudioEngine.ts:327](../hooks/useDJAudioEngine.ts#L327) | Confirmed |
| Integer-only BPM output in current detector | `Math.round((60 * sampleRate) / interval)` [bpmDetection.ts:225](../lib/bpmDetection.ts#L225) | Confirmed — fractional BPM is not merely unexposed, it is unrepresentable |
| `generateBeatGrid()` spaces beats uniformly from an offset | [bpmDetection.ts:274-279](../lib/bpmDetection.ts#L274-L279) | Confirmed |
| Deck analysis runs on load and writes Zustand performance state | [useDJAudioEngine.ts:296-336](../hooks/useDJAudioEngine.ts#L296-L336), [djMixerSlice.ts:119-126](../slices/djMixerSlice.ts#L119-L126) | Confirmed |
| DJv2 browser is virtualized, reads `song.bpm`, caches key in a module-level `Map` | `TableVirtuoso` [DJLibraryBrowserV2.tsx:15](../components/dj/v2/DJLibraryBrowserV2.tsx#L15), `analyzedKeyCache` [:64](../components/dj/v2/DJLibraryBrowserV2.tsx#L64), [:224](../components/dj/v2/DJLibraryBrowserV2.tsx#L224) | Confirmed |
| 12,000-track virtualization audit fixture exists | [dj-overlay-audit.mjs:26](../scripts/dj-overlay-audit.mjs#L26), [:49](../scripts/dj-overlay-audit.mjs#L49) | Confirmed |
| `songs.bpm` is **INTEGER**, mapped to Go `int` | [db.go:231](../backend/internal/db/db.go#L231), [db.go:115](../backend/internal/db/db.go#L115) | Confirmed — see §2.3-D |
| `songs.bpm` is written by the AI enrichment path from genre priors, not audio | `"BPM: Estimate based on genre conventions (e.g., punk ~170, ballads ~70, dance ~128)"` [gemini.go:389](../backend/internal/gemini/gemini.go#L389); written via `UpdateSongMood(... bpm int ...)` [db.go:962](../backend/internal/db/db.go#L962) | Confirmed |
| ReplayGain fields already exist | `replay_gain_db REAL`, `replay_peak REAL` [db.go:531-532](../backend/internal/db/db.go#L531-L532) | Confirmed |
| Backend waveform decodes MP3 only; OGG/FLAC/WAV/AAC return a client-side fallback error | [dj_waveform.go:188-206](../backend/internal/api/dj_waveform.go#L188-L206) | Confirmed |
| `operation_jobs` state machine and restart recovery | [jobs_schema.go:11-17](../backend/internal/db/jobs_schema.go#L11-L17), [:70-78](../backend/internal/db/jobs_schema.go#L70-L78) | Confirmed |
| Existing v2 job API with SSE events, cancel, retry | `V2JobRoutes()` [v2_jobs.go:25-36](../backend/internal/api/v2_jobs.go#L25-L36), `jobEventsV2` [:200-206](../backend/internal/api/v2_jobs.go#L200-L206) | Confirmed |
| `go-mp3` (Apache-2.0) and `oggvorbis` (MIT) are existing pure-Go deps | [go.mod](../backend/go.mod); `oggvorbis.NewReader` [ogg_to_mp3.go:61](../backend/internal/audio/ogg_to_mp3.go#L61) | Confirmed |
| No CGO in the tree; SQLite is pure Go | `modernc.org/sqlite v1.56.0`, no `import "C"` in `backend/` | Confirmed |
| No `backend/internal/analysis` package exists | `ls backend/internal/` | Confirmed — Phase 1 is greenfield |
| **There is no job queue or scheduler** | `go a.runJob(job.ID)` [v2_jobs.go:75](../backend/internal/api/v2_jobs.go#L75); no `ClaimJob`/lease/priority in [jobs_repository.go](../backend/internal/db/jobs_repository.go) | **Corrected** — see §2.3-B |
| **A WASM runtime already ships in production** | `go mod why wazero` → `internal/audio → go.senan.xyz/taglib → wazero` | **New finding** — see §2.3-A |
| **`.aiff`/`.aif` cannot be ingested at all** | `supportedExtensions` [scanner.go:42-51](../backend/internal/scanner/scanner.go#L42-L51) | **Corrected** — see §2.3-C |
| **AI DJ BPM has a third, text-derived provenance tier** | `getSongBPM` falls back to `TempoToBPM(song.Tempo)` [scoring.go:326-331](../backend/internal/dj/scoring.go#L326-L331) | **New finding** — see §2.3-E |
| `gonum` is **not** currently a dependency | absent from [go.mod](../backend/go.mod) | Net-new dependency, not a reuse |

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
- builds a one-BPM-resolution interval histogram, rounding each interval to an **integer** BPM before bucketing;
- searches 60–200 BPM (applied as an interval filter, not merely a display clamp);
- applies limited half/double weighting (neighbour buckets at ½× and 2× weighted 0.5);
- emits a confidence estimate derived from interval consistency within a 10% tolerance;
- falls back to 120 BPM with confidence 0 when analysis cannot establish a tempo — in three separate places, including the initial value of the histogram winner;
- can fold a detected tempo into a target range via `normalizeBPM()`;
- produces `generateBeatGrid()` by spacing beats uniformly from an offset.

Both detectors analyze **only the first 30 seconds**. For key this is an explicit caller option (`detectKey(url, { duration: 30 })`); for BPM it is structural — `ANALYSIS_DURATION = 30` sizes the `OfflineAudioContext` itself, so no caller can request more.

`hooks/useDJAudioEngine.ts` runs this analyzer after deck load and stores the result in deck state.

**Professional gap:** the current detector is a reasonable prototype, not a production DJ analyzer. It is sensitive to peak-selection errors, sees only the first 30 seconds (which for a great many tracks is an intro rather than representative material), lacks a robust multi-candidate tempo model, does not estimate tempo drift, and can turn an uncertain analysis into a misleading `120` fallback if downstream code ignores confidence. Its integer rounding is a hard ceiling rather than a tuning choice: fractional BPM is not merely unexposed by this design, it is unrepresentable within it.

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

### 2.3 Assumptions challenged

The original draft of this roadmap was largely accurate on DSP and licensing, and largely optimistic on infrastructure. These are the assumptions that changed after verification. Each one alters scope, so read them before estimating.

#### A. "WASM is a distant, heavyweight option." — **Wrong, and this is the most useful correction in the document.**

The draft placed a WASM codec pack at step 4 of the §7.4 fallback hierarchy, behind "benchmark, license, binary-size, and security review," implying a new runtime and a new class of risk.

ViiB already ships one. `go.senan.xyz/taglib` runs TagLib compiled to WebAssembly on `github.com/tetratelabs/wazero`, and it is not optional or peripheral — it is the primary metadata extractor, imported by `internal/audio`, `internal/scanner`, and `internal/spotify`. The runtime is pure Go, requires no CGO, and is already in every shipped binary.

**Consequence:** the marginal cost of a wazero-hosted decoder is one WASM artifact plus its license review — not a runtime adoption decision. This is the most credible path to AAC/M4A and Opus, the two codecs that otherwise have no acceptable pure-Go answer. It should be evaluated in Phase 0 as a *first-class* option alongside pure-Go decoders, not as a fallback.

**Consequence that cuts the other way:** wazero's interpreter is roughly an order of magnitude slower than native Go on tight numeric loops, and its optimizing compiler is unavailable on some targets. WASM is therefore acceptable at the **decode boundary**, where the work is bounded by the audio's real-time length and amortized against I/O, and unacceptable **inside the DSP loop**. Phase 0 must measure decode throughput specifically, because a 2× real-time WASM decoder would dominate a 10× real-time analyzer and invalidate §13.2's envelopes.

#### B. "Reuse the existing job infrastructure rather than invent a parallel framework." — **Half true. The state model is reusable; the scheduler does not exist.**

`operation_jobs` genuinely provides durable status, progress counters, typed JSON parameters/results, stable error codes, attempt counts, and restart recovery. That is real and worth building on.

But there is no queue. `createJobV2` writes a row with status `queued` and then immediately calls `go a.runJob(job.ID)`. Nothing ever selects `WHERE status = 'queued'` and dispatches it. Concretely absent:

- no worker pool or concurrency bound — N job creations produce N goroutines;
- no `priority` column and no ordering beyond `created_at`;
- no claim/lease, so two processes or a double-submit would both run;
- no `paused` status in the enum;
- no per-item state, so a 50,000-item job has one `progress_current` integer and no record of *which* items are done.

**Consequence:** Phase 4 is not "add a job type and a worker pool on top of existing infrastructure." It is "build the scheduler `operation_jobs` was always missing, and be the first consumer of it." That is a larger and riskier phase than the draft implied, and it is on the critical path to Phase 5. The upside is that fixing it also benefits the existing scan jobs, which today have the same unbounded-goroutine behavior.

#### C. "AIFF just needs a decoder." — **Wrong: it cannot be ingested at all.**

`scanner.supportedExtensions` is `{.mp3, .flac, .m4a, .aac, .ogg, .opus, .wav, .wma}`. There is no `.aiff` or `.aif`. An AIFF decoder in the analysis engine would never be reached, because no AIFF file can enter the `songs` catalog in the first place.

**Consequence:** AIFF support is a two-part change — scanner extension list *and* decoder — and the scanner half must land first or the work is untestable. The draft's codec matrix noted AIFF was absent from the scanner list but did not draw the prerequisite. Also worth deciding deliberately rather than by omission: AIFF is genuinely common in professional DJ libraries, so this may be worth doing early, but it is a catalog-ingestion change with its own metadata and duplicate-detection implications, not a pure analysis change.

#### D. "`songs.bpm` needs new provenance fields." — **True, but understated: the column is the wrong *type*.**

`songs.bpm` is `INTEGER` in SQLite and `int` in Go. Professional tempo is fractional — 127.99 and 128.00 are different tracks to a beatmatcher, and a rounding error of 0.5 BPM accumulates to a full beat of drift in about two minutes.

**Consequence:** this is not only a provenance problem solvable with a `bpm_source` column. Fractional BPM *requires* a new `REAL` field regardless of provenance policy, which independently justifies the separate `track_analysis` table and removes the "just add columns to `songs`" alternative on technical rather than stylistic grounds. It also means the eventual denormalization question (§8.5 step 6) cannot reuse the existing column as-is.

#### E. "Effective BPM has three tiers: manual, measured, legacy AI." — **There are four; the draft omitted one.**

`getSongBPM` in the AI DJ scorer falls back past `songs.bpm` to `TempoToBPM(song.Tempo)` — a BPM synthesized from a free-text descriptor like `"fast"` / `"medium"` / `"slow"`, which is itself AI-inferred.

**Consequence:** the §8.3 precedence ladder must name this tier explicitly, and it must rank *below* legacy numeric AI BPM. It is the weakest signal in the system and must never reach Sync. Leaving it unnamed is how it silently survives the migration.

#### F. "The JS detectors are a useful baseline." — **True, but the deck-load path has a race that will be inherited if it is ported carelessly.**

`setDeckAnalysis(deck, bpm, key, beatGrid)` is a single setter for all three values. BPM and key analysis are launched as independent async operations, and each call site passes a *captured* value for the field it did not compute — `setDeckAnalysis(deck, normalizedBpm, currentKey, beatGrid)` and `setDeckAnalysis(deck, currentBpm, keyResult.key, ...)`. Whichever resolves second overwrites the other's result with a stale snapshot.

**Consequence:** this is a pre-existing bug, not a blocker for this roadmap, but Phase 5 replaces exactly this code path when it hydrates decks from persistent analysis. Fix it by splitting the setter (or making it a partial patch) as part of Phase 5 rather than reproducing the shape. Note it here so the migration does not treat the current behavior as intended.

#### G. "Defer all accuracy thresholds to Phase 0." — **Defensible in principle, but it leaves Phase 0 with no exit condition.**

The draft is right that inventing a marketing accuracy number before measuring is worse than useless. But "Phase 0 establishes the thresholds" combined with "Phase 0 is a gate" means the gate has no definition, and a research phase without an exit condition does not end.

**Consequence:** Phase 0's acceptance criteria now include provisional numeric targets (see Phase 0) that serve as *tripwires*, not promises: if the Go detector cannot clear them, that is a signal to re-examine the approach rather than to lower the bar quietly. The targets are explicitly revisable once the corpus exists, but they must be written down *before* measurement so the comparison is honest.

#### H. Assumptions that survived challenge unchanged

For the record, these were checked and are correct as drafted:

- the canonical `song_id` / no-DJ-catalog architecture, and the Plex non-writeback rule;
- the licensing analysis, including the GPL/AGPL clean-implementation policy and the specific classifications of Mixxx, libkeyfinder, Essentia, and `benjojo/bpm`;
- the rejection of installed FFmpeg/SoX and of browser decoding as authoritative;
- the "uniform grid is not a beatgrid" distinction, which the code confirms exactly;
- the streaming/bounded-memory model and the row-per-beat rejection;
- the conservative same/adjacent/relative harmonic model, and the refusal to ship a composite transition score early;
- the observation that confidence must not be presented as calibrated probability.

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

### 5.1 Musical key detection

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

### 5.2 BPM / tempo detection

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

### 5.3 Beatgrid/downbeat research direction

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
| AIFF/AIFC PCM | **Not ingestible at all** — absent from `scanner.supportedExtensions` | Small in-house IFF/AIFF PCM reader for uncompressed/standard float subsets | In-house MIT; **Pure Go** | **Two-part change:** add `.aiff`/`.aif` to the scanner *first*, then decode. Common pro-DJ format; decoder alone is dead code (§2.3-C) |
| AAC ADTS | Scanner ingests `.aac`; no backend PCM decoder | `tphakala/go-aac` is pure Go AAC-LC but LGPL-2.1-or-later and profile-limited; **or** a wazero-hosted WASM decoder | LGPL-2.1+ Pure Go, **or** WASM (runtime already shipped) | **Gated evaluation of both paths.** WASM is now the likelier answer — no LGPL static-linking question, and the runtime is already present |
| M4A/AAC | Scanner ingests `.m4a`; metadata works via taglib; no backend PCM decode | MP4/ISO-BMFF demux (straightforward in pure Go) + AAC decoder (pure-Go LGPL or WASM) | mixed; demux can be in-house MIT | Phase 0 spike. Split the problem: demux in pure Go, decode via whichever AAC path clears review. Do not claim support until corpus passes |
| Opus in Ogg | Scanner ingests `.opus`; `oggvorbis` is **not** an Opus decoder, yet `dj_waveform.go` routes `.opus` into the ogg branch | evaluate a permissively licensed pure-Go Opus decoder, or a wazero-hosted `libopus`/`opus-decoder` WASM build | TBD; WASM path uses existing runtime | Explicit unsupported-analysis status until validated. Also fix the existing `.opus`/`.ogg` conflation so the failure is honest rather than mislabeled |
| WMA | Scanner ingests `.wma`; no pure-Go analysis path established | No pure-Go path identified; WASM conceivable but low value | TBD | Do not block engine; mark codec unsupported for analysis. Lowest priority — rare in DJ libraries |

### 7.3 Why browser decoding should not remain authoritative

Browser/WebView decoding has attractive codec coverage but creates problems for library preparation:

- analysis depends on WebView codec differences by OS;
- background bulk analysis is tied to renderer lifecycle;
- results are harder to reproduce in Go tests/CLI benchmarks;
- local/Plex handling is less controllable;
- analysis can compete with the same browser audio engine used for live DJ performance.

Browser analysis can remain a **temporary fallback/diagnostic comparator**, not the canonical persisted engine.

### 7.4 Fallback hierarchy

Recommended policy, reordered to reflect that a WASM runtime is already shipped (§2.3-A):

1. **Pure-Go decoder available and validated:** analyze normally. Always preferred — best throughput, simplest build.
2. **Pure-Go decoder available but license/profile uncertain:** feature-gate until approved. (`tphakala/go-aac` sits here.)
3. **WASM decoder on the existing wazero runtime:** a legitimate production path, not a last resort. Requires per-artifact license review, a profile-coverage corpus, binary-size accounting, and a decode-throughput benchmark — but *not* a new runtime, new build toolchain, or CGO. Confine WASM to decoding; never run DSP through it.
4. **No backend decoder:** persist `unsupported_codec` analysis state; do not fake values. This is an acceptable steady state for WMA and anything else with no viable path.
5. **Bundled FFmpeg/native helper:** rejected for initial roadmap. Consider only if codec coverage becomes a release blocker and redistribution/license/attack-surface costs are explicitly accepted. The existence of path 3 makes this substantially less likely to be needed.

No dependency on a user-installed FFmpeg/SoX executable.

**Sandboxing note.** wazero executes WASM without host filesystem or network access unless explicitly granted. Feeding it untrusted media is therefore a materially better security posture than linking a native C decoder into the process — a relevant point given that malformed-audio parsing is a classic memory-safety sink and that ViiB decodes files the user did not author.

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
    > legacy numeric AI/inferred songs.bpm      -- non-performance ranking only
    > tempo-descriptor-derived BPM               -- TempoToBPM(song.Tempo); weakest tier
    > unknown
```

The fifth tier is easy to miss and must be named explicitly. `getSongBPM` in the AI DJ scorer already falls back past `songs.bpm` to `TempoToBPM(song.Tempo)`, synthesizing a number from a free-text descriptor (`"fast"`, `"medium"`, `"slow"`) that was itself AI-inferred ([scoring.go:326-331](../backend/internal/dj/scoring.go#L326-L331)). It is a coarse bucket wearing the costume of a measurement. Keeping it is defensible for soft AI DJ flow ordering, where a wrong-by-20-BPM guess degrades gracefully; surfacing it anywhere a DJ might read it as tempo is not.

**Hard rules:**

- Do not use AI-estimated BPM for beat-phase Sync when measured/manual tempo is absent.
- Never use the tempo-descriptor tier for anything a user sees as a BPM value, or for any Sync, loop, or beat-jump decision.
- The resolver must return the tier alongside the value, so callers can refuse low tiers rather than having to know this ladder by heart. A resolver that returns a bare `float64` will be misused.

The same laddering applies to key, minus the inferred tiers: there is no AI-inferred key today, and none should be introduced — an unmeasured key must resolve to `unknown`, never to a guess.

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

**The column is `INTEGER`** ([db.go:231](../backend/internal/db/db.go#L231)), mapped to Go `int` ([db.go:115](../backend/internal/db/db.go#L115)). This is a type problem, not only a provenance problem. Professional tempo is fractional: a 0.5 BPM rounding error accumulates to roughly a full beat of drift within two minutes at 128 BPM, which is audible and unfixable by Sync. So a new `REAL` field is required no matter what policy is chosen for the old one — which independently kills the "just add provenance columns to `songs`" alternative on technical grounds, and constrains step 6 below.

Migration sequence:

1. introduce `track_analysis` with `bpm REAL`;
2. classify existing `songs.bpm` as legacy/inferred unless another verified source is available;
3. update AI DJ and DJv2 to call an effective-feature resolver that returns value **and** provenance tier;
4. measured BPM wins for DJ workflows;
5. legacy BPM remains a fallback for non-critical sequencing until sufficient library coverage exists;
6. later decide whether to add a *new* denormalized `REAL` effective-BPM cache column on `songs` or to deprecate `songs.bpm`. The existing integer column cannot itself become the effective cache.

**Do not write measured BPM back into `songs.bpm`.** It would round the value, destroy the provenance distinction the whole migration exists to create, and collide with the AI enrichment path that still writes that column via `UpdateSongMood`.

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

Use the existing durable `operation_jobs` record for batch jobs and keep per-track work state in `track_analysis`.

> **What must be built, not reused.** `operation_jobs` has no queue. Job creation dispatches immediately via `go a.runJob(job.ID)` ([v2_jobs.go:75](../backend/internal/api/v2_jobs.go#L75)), and no code path ever selects `WHERE status = 'queued'`. Phase 4 must add: a dispatcher that drains `queued` rows, a bounded worker pool, work ordering, single-flight claiming, and pause. See §2.3-B. Treat the following as a specification for new code.

**Required additions to the job layer:**

| Need | Current state | Phase 4 work |
|---|---|---|
| Dequeue loop | absent | dispatcher goroutine draining `queued` by priority then `created_at` |
| Concurrency bound | absent (goroutine per job) | worker pool sized per §9.4 worker policy |
| Priority | no column | add `priority INTEGER NOT NULL DEFAULT 0`; index `(status, priority, created_at)` |
| Single-flight claim | absent | conditional `UPDATE ... SET status='running' WHERE status='queued'` and check affected rows |
| Pause | no status | add `paused` status, or implement as "stop dispatching, let in-flight items finish" (preferred — see §9.5) |
| Per-item progress | one integer counter | authoritative per-track state in `track_analysis.status`; job row keeps only aggregate counts |
| Restart resume | queue is discarded | see below |

**Restart resume.** Existing recovery marks `queued`, `running`, and `canceling` rows `interrupted` ([jobs_schema.go:70-78](../backend/internal/db/jobs_schema.go#L70-L78)). That is correct for a scan and wrong for a multi-day analysis run, which it would silently abandon. Do not solve this by preserving `queued` job rows across restart — that reintroduces ghosts. Solve it by making the *work list derivable*: because `track_analysis` holds per-track status, version, and fingerprint, "what is left to do" can always be recomputed by re-expanding the selection and skipping valid rows. A resumed job is then a fresh job row over a shrinking work set, which is idempotent and needs no lease recovery. Record the original selection in the job's `parameters` so it can be re-expanded verbatim.

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

`operation_jobs` supports cancellation and interruption but has **no `paused` status** in its enum, and `V2JobRoutes()` exposes no pause/resume endpoint — only `cancel` and `retry`. Both are new.

**Recommended:** implement pause as “stop dispatching new track items; let the current item finish,” rather than serializing arbitrary DSP internals mid-track. Combined with the derivable-work-list model in §9.4, this makes pause/resume nearly free: pause stops the dispatcher, resume restarts it, and correctness comes from `track_analysis` rather than from suspended job state. Mid-track suspension buys a few seconds of latency at the cost of a substantially harder invariant, and is not worth it.

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

The drawer/browser is already virtualized (`TableVirtuoso` from `react-virtuoso`) and has a 12,000-track audit fixture in `scripts/dj-overlay-audit.mjs`. New compatibility computations must be O(rows) with tiny constants and memoized inputs; never run DSP in React.

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

The relevant state already exists: `keyLockA` / `keyLockB` in [djMixerSlice.ts:199-200](../slices/djMixerSlice.ts#L199-L200) ("preserve pitch when changing tempo"), alongside `originalBpm` and `effectiveBpm`. The effective-key derivation is therefore well-defined and cheap:

- **key lock on** — tempo changed, pitch preserved: effective key **equals** the original key;
- **key lock off** — tempo change shifts pitch: effective key is the original transposed by `12 * log2(effectiveBpm / originalBpm)` semitones, and only meaningful when that lands near a semitone. A 6% pitch change is roughly one semitone; a 2% change is about a third of one, which is audibly detuned rather than transposed into a new key.

That last case is the honest-answer case: at fractional-semitone shifts the deck is not in a *different* key, it is slightly out of tune with everything. Do not round it to the nearest Camelot slot and present that as truth — mark the effective key as approximate, or fall back to the original key with an indicator. Rounding here would produce confidently wrong compatibility advice, which is worse than none.

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

| Library | Audio duration | 1 worker @ 5× | 4 workers @ 5× | Notes |
|---|---:|---:|---:|---|
| 1,000 tracks | ~67 h | ~13 h (~0.6 days) | ~3 h | First-run overnight job; progress/resume important |
| 10,000 tracks | ~667 h | ~133 h (~5.6 days) | ~33 h (~1.4 days) | Concurrency is mandatory, not an optimization |
| 50,000 tracks | ~3,333 h | ~667 h (**~28 days**) | ~167 h (~7 days) | Weeks, not days. Incremental operation is the product, not a nicety |

Parallel workers reduce elapsed time only until CPU, disk, decoder, or remote-source contention becomes limiting. Benchmark rather than assuming linear scaling.

**Product consequences of this table.** These durations are large enough to shape the feature rather than merely inform it:

- A "analyze my library" button that implies completion in a session is dishonest at 10,000+ tracks. The UX must present analysis as an ongoing background process with useful partial results, not a task with an end.
- **Partial-library usefulness is a hard requirement.** DJv2 must be fully usable when 5% of the library is analyzed, which means unanalyzed tracks need a first-class display state (§10.3) and filters must not silently hide them.
- Prioritization matters more than throughput. Analyzing the 300 tracks a DJ is about to play beats analyzing 50,000 in catalog order, which is why `priority` is required scheduler scope (§9.4) rather than a later refinement.
- Analysis-on-load (§9.7) is not a fallback for edge cases — for a large library it is how most tracks get analyzed in practice, for months.
- This is also the strongest argument for eventually validating a representative-section fast mode (question #4). A 3× speedup here is worth weeks of wall time, but only if the accuracy cost is measured first.

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
- spike Gonum FFT vs small internal FFT (note: Gonum is a **net-new** dependency, not present in `go.mod` today);
- spike full-track streaming vs representative-section analysis;
- validate MP3/Vorbis/WAV/FLAC decoding;
- spike AAC-LC/M4A pure-Go paths with legal review flag;
- **spike a wazero-hosted WASM decoder for AAC and Opus**, measuring decode throughput separately from DSP (§2.3-A). The runtime is already a shipped dependency, so this spike is cheaper than the draft assumed and should be run in parallel with the pure-Go AAC spike rather than after it;
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

Phase 0 is a gate, so it needs an exit condition. The draft deferred *all* thresholds to Phase 0 itself, which left the gate undefined (§2.3-G). The numbers below are **provisional tripwires written down before measurement**, not promises: they exist so that failing them forces a visible decision instead of a quiet lowering of the bar. Revise them once the corpus exists, in a recorded amendment to this document.

Deliverables:

- documented decoder matrix with pass/fail corpus, including the wazero/WASM verdict for AAC and Opus;
- chosen FFT/resample approach, with allocations/op recorded;
- reproducible baseline metrics for the current JS BPM/key detectors;
- selected BPM and key algorithms with a quantified improvement target;
- license classification for every candidate;
- **a corpus of at least 200 tracks** with trustworthy labels, genre-stratified per §14.2, of which a held-out third is reserved for calibration and never used for tuning;
- no production feature exposed yet.

Provisional tripwires — measured on the held-out split:

| Metric | Provisional target | Rationale |
|---|---|---|
| BPM, strict (±0.5) on stable electronic material | ≥ 95% | If a Go detector cannot beat this on quantized 4/4, the pipeline is wrong, not the corpus |
| BPM, strict (±0.5) across the full stratified corpus | ≥ 85% | Includes live drums, acoustic, sparse material |
| BPM half/double error rate, Automatic mode | ≤ 5% | The single most damaging failure for DJ trust |
| BPM improvement over JS baseline, strict | ≥ 20 percentage points | Below this, the migration is not worth its cost |
| Key, exact tonic+mode | ≥ 65% | Consistent with published EDM key-detection results; do not accept vendor claims of 90%+ as a target |
| Key, Camelot-compatible (same/adjacent/relative counted correct) | ≥ 85% | The metric that actually matters for mixing |
| Key improvement over JS baseline, exact | ≥ 10 percentage points | Key is a harder problem; a smaller margin is still worth having |
| Confidence calibration | monotonic across ≥ 3 buckets | High-confidence results must be measurably more correct than low-confidence ones, or confidence is decoration |
| Deliberately unclassifiable fixtures returning `unknown` | ≥ 90% | Refusing to answer is a feature |
| Cross-platform determinism, same PCM | bit-identical scalars, or documented tolerance ≤ 1e-6 | Windows/macOS/Linux |
| Throughput, decode + BPM + key, one worker | ≥ 5× real-time | §13.2; measure decode separately so a slow WASM decoder is visible |

If a metric cannot be met, the recorded outcome must be one of: change the algorithm, change the target with a written justification, or descope the feature. "Ship it anyway" is not an option for the half/double and `unknown` rows — those two govern whether a DJ can trust the display at all.

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

## Phase 4 — Library analysis service, job scheduler, and professional lifecycle

### Objective

Turn analyzers into a dependable library-preparation workflow.

### User value

DJs can prepare thousands of tracks before a set, see progress, cancel, resume, and trust that only stale/missing work is repeated.

### Current-state gap

Analysis occurs opportunistically on deck load and has no durable batch lifecycle.

**Scope correction (§2.3-B).** The draft treated this phase as adding a job type and a worker pool on top of existing infrastructure. Verification shows there is no queue at all: `operation_jobs` is a durable *state* table whose `queued` status is written but never drained, because `createJobV2` dispatches immediately with `go a.runJob(job.ID)`. Phase 4 therefore builds the scheduler ViiB does not have and becomes its first consumer. That is a larger and riskier phase than originally scoped, and it sits on the critical path to Phase 5 — the first phase with user-visible value. Plan accordingly.

The compensation is that this work is not analysis-specific. The existing `full_scan`, `quick_scan`, and `refresh_genre_stats` jobs share the same unbounded-goroutine behavior and would inherit the fix. Build the scheduler as a general `operation_jobs` dispatcher, not an analysis-private one, and migrate the scan jobs onto it — otherwise ViiB ends up with exactly the parallel job framework this roadmap set out to avoid.

### Architecture

General-purpose durable job scheduler over `operation_jobs` (new), plus an analysis runner registered on it. Prioritized work queue, per-track validity checks, work list derivable from `track_analysis` rather than held in job state.

### Backend changes

Job-layer work (new infrastructure, per §9.4's table):

- queue dispatcher draining `queued` rows;
- `priority` column and `(status, priority, created_at)` index;
- single-flight claim via conditional status update;
- bounded worker pool;
- `paused` state or dispatch-gating equivalent;
- pause/resume endpoints on `V2JobRoutes()`;
- migrate existing scan jobs onto the dispatcher.

Analysis-specific work:

- job type(s) for track analysis registered on the `runJob` dispatch switch;
- selection expansion (IDs/playlist/missing/stale/all), recorded in job `parameters` so it can be re-expanded after restart;
- retry policy;
- auto-analyze integration after scan/Plex sync;
- performance-mode throttle signal;
- progress aggregation from per-track `track_analysis` state;
- stale/skip logic.

### Frontend changes

- Library Analysis panel/dialog;
- Analyze New Tracks setting;
- separate Plex auto-analysis setting;
- progress, pause/cancel, failed-item summary;
- selected/playlist/missing/all actions.

### Database changes

Reuse the `operation_jobs` table, but add the scheduling columns it lacks: `priority INTEGER NOT NULL DEFAULT 0` and an index on `(status, priority, created_at)`. Add a `paused` status constant if the gating approach in §9.5 is not used. No per-item job table — per-track state lives in `track_analysis`, which keeps the work list derivable and the row count bounded by the catalog rather than by job history.

### APIs/events

Extend the existing `V2JobRoutes()` surface rather than adding a parallel API. Present today: `GET /`, `POST /`, `GET /events` (SSE), `GET /{id}`, `POST /{id}/cancel`, `POST /{id}/retry`. To add: `POST /{id}/pause`, `POST /{id}/resume`, and the analysis job types in the `createJobV2` type allowlist (currently hardcoded to `full_scan`, `quick_scan`, `refresh_genre_stats`) and the `runJob` dispatch switch. Reuse `jobEventsV2` for progress rather than opening a second SSE stream.

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
- **restart does not lose pending work** — a job interrupted at 30% of 10,000 tracks can be resumed and re-dispatches only the ~7,000 unfinished tracks, verified by test rather than by inspection;
- **concurrency is bounded** — creating 50 analysis jobs at once produces the configured worker count, not 50 goroutines;
- **double-submit is single-flighted** — two concurrent create/dispatch attempts for the same work do not both analyze;
- already-valid tracks are skipped;
- algorithm/source changes make only relevant tracks stale;
- active DJ playback reduces analysis pressure without audio dropout;
- existing scan jobs continue to pass their current tests after migration onto the dispatcher.

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
- hydrate deck `originalBpm`/key from persistent analysis before falling back to on-load analysis;
- **split `setDeckAnalysis` into per-field updates (or make it a partial patch).** It currently takes `(deck, bpm, key, beatGrid)` as one setter, and the two independent async analyzers each pass a *captured* stale value for the field they did not compute — so whichever resolves second clobbers the other's result ([useDJAudioEngine.ts:312](../hooks/useDJAudioEngine.ts#L312), [:336](../hooks/useDJAudioEngine.ts#L336)). This is a pre-existing bug (§2.3-F), and Phase 5 rewrites exactly this code path. Fix the shape rather than porting it.

### Database changes

No new schema expected beyond indexes for BPM/key/status if server-side filtering is introduced.

### APIs/events

Catalog result updates when analysis completes; manual override update endpoint.

### Candidate Go packages

None.

### Licensing considerations

Camelot mapping can be implemented internally as music-theory data; no need for a Node dependency.

### Performance considerations

Compatibility computation is scalar and must not trigger deck/audio rerenders. Preserve `TableVirtuoso` row stability and the existing drawer memo boundaries.

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
| **Phase 4 underestimated** because the draft assumed a reusable scheduler | Critical-path slip into Phase 5, the first user-visible phase | scope the scheduler explicitly (§9.4 table); build it as a general dispatcher so the cost is shared with existing scan jobs; consider building it against a stub analyzer in parallel with Phases 2–3 |
| **Restart discards a multi-week analysis queue** | Users lose days of work and stop trusting the feature | derive the work list from `track_analysis` rather than job rows (§9.4); test resume explicitly at partial completion |
| **WASM decode too slow to hit throughput envelope** | Codec coverage won but §13.2 targets missed | benchmark decode separately from DSP in Phase 0 (question #18); keep WASM strictly at the decode boundary; prefer pure Go wherever it exists |
| Unbounded goroutines from job creation | Resource exhaustion under bulk operations | worker pool in Phase 4; this is a pre-existing defect in scan jobs, not new risk introduced here |
| **Analysis never completes for large libraries** | Feature perceived as broken rather than slow | partial-library usefulness as an acceptance requirement (§13.3); priority scheduling; honest progress UX that does not imply an end state |
| Fractional BPM silently rounded by a legacy write path | Beatmatch drift, provenance loss | never write measured BPM to the `INTEGER` `songs.bpm` (§8.5); resolver returns value plus provenance tier |

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

These should be answered in Phase 0, not guessed during later implementation. Each has an owner phase and a decision form, so none can be left open by default.

| # | Question | Answer in | Decision form |
|---|---|---|---|
| 1 | Which pure-Go FFT gives the best correctness/allocation tradeoff: Gonum or a small internal implementation? | Phase 0 | Benchmark table + ADR. Note Gonum is net-new |
| 2 | What analysis sample rate and STFT geometry best balance BPM and key reuse without materially degrading either? | Phase 0 | Chosen geometry + per-task accuracy delta |
| 3 | Is shared STFT practical for both onset and chroma, or should they share only decoded/resampled PCM? | Phase 0 | ADR; fall back to shared-PCM-only if geometries conflict |
| 4 | How much accuracy is lost by 60–90 s representative-section analysis versus full-track, by genre? | Phase 0 | Per-genre delta table; gates any future "fast mode" |
| 5 | Which chroma/HPCP profile combination wins on ViiB's corpus? | Phase 0/3 | Confusion matrices per profile |
| 6 | What confidence features best identify key uncertainty/modulation? | Phase 3 | Calibration curve per feature set |
| 7 | What candidate/prior policy minimizes half/double errors without genre hardcoding? | Phase 2 | Half/double rate against the ≤5% tripwire |
| 8 | **Reframed.** AAC/M4A: pure-Go LGPL decoder, or WASM decoder on the wazero runtime ViiB already ships? Split the container question (MP4 demux, straightforward in pure Go) from the codec question | Phase 0 | Head-to-head spike: profile coverage, decode throughput, license verdict. WASM is now the leading candidate (§2.3-A) |
| 9 | **Reframed.** Opus: is there a viable pure-Go decoder, or is a wazero-hosted `libopus` build the answer? Also fix the existing `.opus`→ogg misrouting in `dj_waveform.go` regardless of outcome | Phase 0 | Same head-to-head form as #8 |
| 10 | How should legacy AI-estimated `songs.bpm` be surfaced during migration — hidden provenance, explicit `Estimated` badge, or non-DJ fallback only? Note there are **two** inferred tiers to handle, including `TempoToBPM(song.Tempo)` (§2.3-E) | Phase 1/5 | Product decision; recommendation is explicit badge + never used for Sync |
| 11 | Should the existing `dj_waveform_cache` table be migrated into `track_analysis_artifacts`, or remain specialized and share only the decoder pipeline? | Phase 1 | ADR. Sharing the decoder is the clear win; sharing the table is optional |
| 12 | How should Plex source revision/fingerprint be computed without downloading media twice? | Phase 1 | Fingerprint spec per §8.6 |
| 13 | What worker count keeps Windows/macOS Wails deck playback glitch-free on 4/8/16-core machines? | Phase 4 | Measured default + throttle policy |
| 14 | At what library size does `DJLibraryBrowserV2` need server-side BPM/key filtering rather than client filtering? | Phase 5 | Profile against the existing 12,000-track fixture, extended to 50,000 |
| 15 | What user-facing threshold turns confidence into High/Medium/Low without implying probability? | Phase 0/3 | Bucket boundaries from the held-out split |
| 16 | **New.** Should the Phase 4 scheduler be built as a general `operation_jobs` dispatcher and the existing scan jobs migrated onto it? | Phase 4 | ADR. Recommendation is yes — otherwise ViiB gains the parallel job framework this roadmap set out to avoid (§2.3-B) |
| 17 | **New.** Is `.aiff`/`.aif` ingestion in scope, given it requires a scanner change with its own metadata and duplicate-detection implications? | Phase 0/1 | Product decision. Common in pro DJ libraries; currently impossible to ingest (§2.3-C) |
| 18 | **New.** What is the WASM decode throughput floor below which the §13.2 5×-real-time envelope becomes unreachable? | Phase 0 | Measured; decode must be benchmarked separately from DSP |

---

## 21. Dependency classification register

This register covers dependencies discussed by this roadmap. Implementation PRs must update it with exact versions.

Existing dependencies confirmed present in [`backend/go.mod`](../backend/go.mod) at the verification snapshot are marked **already shipped** — no new redistribution decision is required to use them.

| Dependency / project | Classification | License | Status | Proposed use |
|---|---|---|---|---|
| `github.com/hajimehoshi/go-mp3` v0.3.4 | Pure Go | Apache-2.0 | **already shipped** | Existing MP3 PCM decode; retain |
| `github.com/jfreymuth/oggvorbis` v1.0.5 | Pure Go | MIT | **already shipped** | Existing Ogg/Vorbis PCM decode; retain |
| `github.com/tetratelabs/wazero` v1.10.1 | Pure Go WASM runtime | Apache-2.0 | **already shipped** (indirect, via taglib) | Host for WASM codec decoders — AAC/M4A/Opus. Runtime cost already paid (§2.3-A) |
| `go.senan.xyz/taglib` v0.11.1 | Pure Go + embedded WASM | MIT (bindings); TagLib is LGPL-2.1/MPL-1.1 | **already shipped** | Existing metadata extraction; also the precedent proving the WASM path is viable |
| `modernc.org/sqlite` v1.56.0 | Pure Go | BSD-3-Clause | **already shipped** | Analysis persistence; confirms the no-CGO baseline |
| `gonum.org/v1/gonum/dsp/fourier` v0.17.0 | Pure Go | BSD-3-Clause | **net-new, Phase 0 benchmark-only** | Comparator for the internal radix-2 FFT. At 8192 points on the first measured Windows host it was slower/more allocating; keep only while benchmark evidence is being collected. |
| `github.com/mewkiz/flac` | Pure Go | Unlicense | **net-new** | Candidate FLAC decode |
| in-house WAV decoder | Pure Go | ViiB MIT | **net-new** | Preferred simple uncompressed PCM container support |
| in-house AIFF/AIFC decoder | Pure Go | ViiB MIT | **net-new** | Requires scanner extension-list change first (§2.3-C) |
| in-house MP4/ISO-BMFF demuxer | Pure Go | ViiB MIT | **net-new** | M4A container parsing, separable from the AAC decode decision |
| `github.com/tphakala/go-aac` | Pure Go | LGPL-2.1-or-later | **net-new, gated** | Technical codec spike only, pending legal/static-distribution review. Compare against the WASM AAC path, which avoids the LGPL static-linking question entirely |
| WASM AAC decoder artifact (TBD) | WASM on existing wazero | per-artifact; must be reviewed | **net-new, gated** | Leading candidate for AAC/M4A analysis decode |
| WASM Opus decoder artifact (TBD) | WASM on existing wazero | per-artifact; likely BSD (libopus) | **net-new, gated** | Leading candidate for `.opus` analysis decode |
| Essentia | CGO/native if embedded native; can be compiled to WASM | AGPLv3 principal license | reference only | Development/reference benchmark only; reject default production |
| Essentia.js | WASM + JavaScript | AGPLv3 principal license | reference only | Development/reference benchmark only |
| `libraz/libsonare` | CGO/native or WASM depending binding | Apache-2.0 per project | reference only | Benchmark/reference; possible future optional WASM path after validation |
| `mixxxdj/mixxx` | CGO/native application architecture | GPLv2-or-later | reference only | Architectural study only |
| `mixxxdj/libkeyfinder` | CGO/native C++ | GPLv3-or-later | reference only | Key algorithm study/benchmark only |
| `benjojo/bpm` | Pure Go code, but copyleft implementation | GPLv2 | reference only | Study only; no source reuse |
| `dlepaux/realtime-bpm-analyzer` | Browser JavaScript/TypeScript | Apache-2.0 | reference only | BPM algorithm reference/baseline only |
| `ifeelvoid/keyfinder` | Native Swift/macOS | MIT | reference only | Algorithm/reference comparison only |
| `libraz/bpm-detector` | Python + native/scientific stack | MIT project; dependency licenses vary | reference only | Algorithm/reference comparison only |
| `Rexaintreal/Resonate` | Browser JS + Python/Flask + cloud pieces | MIT | reference only | Low-priority conceptual reference |
| installed FFmpeg/SoX | External executable | mixed licenses | **rejected** | Rejected — see §18 |
| cloud audio-analysis API | Cloud/API dependency | provider-specific | **rejected** | Rejected for core/offline engine |

---

## 22. Recommended implementation PR decomposition

Keep implementation PRs reviewable. Suggested sequence after this roadmap:

| # | Branch | Phase | Notes |
|---|---|---|---|
| 1 | `analysis/phase0-benchmarks-and-codec-matrix` | 0 | Includes the wazero/WASM decode spike alongside the pure-Go AAC spike |
| 2 | `analysis/foundation-pcm-dsp-persistence` | 1 | Greenfield `backend/internal/analysis/`; schema + decoder registry + resolver |
| 3 | `jobs/durable-scheduler` | 4a | **Split out and land early.** General `operation_jobs` dispatcher: dequeue loop, worker pool, `priority`, single-flight claim, pause. Migrates existing scan jobs. Independent of any analyzer, so it can proceed in parallel with PRs 4–5 and de-risks the critical path |
| 4 | `analysis/tempo-v1` | 2 | Parallelizable with PR 5 |
| 5 | `analysis/key-v1` | 3 | Parallelizable with PR 4 |
| 6 | `analysis/library-jobs` | 4b | Analysis job types, selection expansion, auto-analyze hooks, throttle. Depends on PR 3 |
| 7 | `djv2/persistent-analysis-library-ux` | 5 | First user-visible value. Includes the `setDeckAnalysis` split (§2.3-F) |
| 8 | `analysis/beatgrid-downbeat-v1` | 6 | |
| 9 | `djv2/beatgrid-editor-sync-integration` | 6 | |
| 10 | `analysis/energy-structure-v1` | 7 | |
| 11 | `djv2/transition-recommendations` | 7 | |
| 12 | `ai-dj/measured-track-features` | 7 | |

Splitting the scheduler (PR 3) out of Phase 4 and landing it early is the single highest-leverage sequencing change available: it is the newly discovered scope, it blocks the first phase that delivers user value, it has no dependency on either analyzer, and it pays for itself by fixing the existing scan jobs.

Each phase PR should update this roadmap's status table and include benchmark deltas. PRs that add a dependency must also update §21 with the exact version and classification.

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
- `backend/internal/api/v2_jobs.go` — existing job API, SSE events, and the `runJob` dispatch switch to extend
- `backend/internal/dj/scoring.go` — `getSongBPM` provenance ladder
- `backend/internal/dj/sequencer.go`
- `backend/internal/gemini/gemini.go` — AI enrichment prompt that estimates BPM from genre conventions
- `scripts/dj-overlay-audit.mjs` — 12,000-track virtualization/performance audit to extend in Phase 5
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

### What verification changed

Three findings alter the plan rather than merely annotating it, and they are the reason to read §2.3 before estimating:

1. **The scheduler does not exist.** `operation_jobs` is a durable state table with no queue — job creation dispatches straight to a goroutine. Phase 4 must build the dispatcher, worker pool, priority, claiming, and pause from scratch, and it sits on the critical path to the first user-visible phase. Split it out as its own early PR (§22, PR 3), build it as a general dispatcher, and migrate the existing scan jobs onto it.
2. **A WASM runtime already ships.** wazero is already in every ViiB binary via `go.senan.xyz/taglib`. The codec strategy's worst gaps — AAC/M4A and Opus — become tractable at the cost of a license review and a throughput benchmark, not a runtime adoption. Evaluate it in Phase 0 as a first-class option, and keep it strictly at the decode boundary.
3. **Scale is weeks, not days.** 50,000 tracks is ~28 days of single-worker analysis. That makes partial-library usefulness, priority scheduling, restart resume, and honest progress UX into hard requirements rather than polish.

Two smaller corrections are worth carrying forward because they are easy to lose: `songs.bpm` is `INTEGER`, so fractional BPM needs a new column on technical grounds regardless of provenance policy; and the effective-BPM ladder has a fifth tier — `TempoToBPM(song.Tempo)` — that must be named to keep it out of Sync.

The rest of the original analysis held up. The DSP pipelines, the licensing discipline, the refusal to ship a composite transition score early, the "unknown beats fabricated certainty" principle, and the insistence that a uniform grid is not a beatgrid are all correct and should be preserved as written.

### Definition of ready

This document is ready to implement when a reader can start Phase 0 without asking a clarifying question. It now provides: verified current state with `file:line` evidence (§2.0); the assumptions that changed and why (§2.3); a critical path and a minimum shippable slice (§1); numeric Phase 0 exit tripwires (Phase 0); a specification for the missing scheduler (§9.4); and an owner phase and decision form for every open question (§20). The remaining unknowns are genuine research questions, not gaps in the plan — which is the correct state for a roadmap whose first phase is a research gate.
