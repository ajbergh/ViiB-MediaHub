# DJv2 Track Analysis — Concise Context and Next Actions

**Source:** `DJV2_PROFESSIONAL_TRACK_ANALYSIS_ROADMAP.md`  
**Snapshot:** 2026-09-18

**Purpose:** Operational handoff of the active context, completed work, quality gate, and next actions. The source roadmap remains the detailed research, design, and evidence record.

## Current position

The engineering slices for Phases 0–7 have landed, including persistent analysis, beatgrid/downbeat artifacts and edits, energy/structure artifacts, advisory cues, deck energy UI, opt-in cue acceptance, and explainable local transition recommendations. Phase 5–7 work was opened under an explicit project-owner override.

The production defaults now use the measured half-BPM multi-feature tempo candidate (crest 15) and the 36-bin HPCP/Krumhansl key candidate (3500 Hz, flatness 0.98, minimum profile margin 0.01). This repairs an operational mismatch where the user-visible job still ran the original synthetic-fixture prototypes and rejected nearly all mastered tracks. The algorithm versions were advanced so those rejected rows are selectable as outdated. Analysis jobs and per-track failures also write structured `[Analysis]` entries to an append-only-across-restarts `viib.log`.

This **does not authorize a professional-accuracy claim**. The Phase 0 evidence gate remains open. The current work is primarily corpus expansion, measurement, tempo improvement, calibration, and cross-platform proof—not foundational feature construction.

The September 17 reliability pass advances tempo to `tempo-v3-multifeature-half-bpm`: tied alternative votes resolve deterministically, alternatives remain distinct after rounding, and confidence uses the actual runner-up rather than replacing it with a weaker displayed alternative. Primary BPM selection and refusal thresholds are unchanged. New benchmark artifacts preserve supplied analyzer options and alternate BPM; comparison/gate reports retain configuration. Determinism checks now include configuration, alternate BPM, stability, crest, flatness, and status, and reject invalid or empty result sets. Missing results and source failures no longer count as successful unknown refusals. Successfully analyzed tracks with neither reliable scalar use benchmark status `unknown`; catalog status semantics are unchanged. Legacy artifacts still load, but old `failed` rows without explicit refusal evidence do not earn unknown-refusal credit.

Validation of this pass: analysis, benchmark, DJ, and API tests passed, as did focused `go vet`. A fresh Windows run of all 127 tuning tracks preserved every primary BPM from `phase0-go-tuning-calibration-o-r2.json`: 103/127 strict matches (81.10%), two half/double errors, six unknown BPM results, and 57.91× real-time throughput. Confidence changed on 35 tracks; no reported alternate duplicated its primary. Two decoder failures remain explicit. Immutable local artifacts are `sample_media/Test Corpus/phase0-go-tuning-reliability-20260917-v3.json` and `phase0-tuning-reliability-comparison-20260917-v3.json`. These are tuning measurements, not new held-out qualification. Tempo confidence is still non-monotonic (low/medium/high bucket accuracy 88.14%/82.69%/80.00%); key confidence has no high-bucket observations. Fixing confidence arithmetic does not establish calibration.

The subsequent tuning-only clustered-vote experiment recovered two strict misses and lost one: 104/127 strict (81.89%), four half/double errors (3.15%), six unknowns, 62.96× real time. It remains an explicit `multifeature-clustered-half-bpm` benchmark method, not the production default; the small gain and added metrical errors do not justify promotion. Evidence is retained in `phase0-go-tuning-clustered-20260917-v1.json` and `phase0-tuning-clustered-comparison-20260917-v1.json` under the local corpus directory.

The `multifeature-refined-half-bpm` experiment adds padded boundary-peak measurements and refinement across two to four beat periods before clustered voting. It passes synthetic 90/90.5/128.3/179.5/180 BPM checks at 22.05/44.1 kHz. On the original tuning split it reaches 109/127 strict (85.83%), 5/127 half/double (3.94%), and monotonic tempo-confidence buckets (50%/76.47%/95.83%). Frozen in `phase0-refined-selection-20260918-v1.json`, it was evaluated once on the overlap-free r5 held-out subset: **43/54 strict (79.63%), 3/54 half/double (5.56%), four unknown BPM results, 51.44× real time**. Confidence remains monotonic (0%/80%/92.86%), but accuracy fails qualification. Key on that subset is 38/54 exact (70.37%) and 45/54 compatible (83.33%), with no high-confidence observations. The method remains benchmark-only; do not promote it or retune against these held-out errors. Raw and comparison artifacts are `phase0-go-tuning-refined-20260918-v1.json`, `phase0-tuning-refined-comparison-20260918-v1.json`, `phase0-go-heldout-refined-r5-20260918-v1.json`, and `phase0-heldout-refined-comparison-r5-20260918-v1.json`.

Native Linux tempo/benchmark tests passed under Debian WSL using Linux binaries cross-compiled with Go 1.26.8. All 33 comparable generated fixtures and the 127-track real-audio tuning replay agree with Windows within the recorded scalar tolerance (1e-6); native three-platform generated-fixture evidence now passes in [CI run 35352456861](https://github.com/ajbergh/ViiB-MediaHub/actions/runs/35352456861) at commit `cf17fdd`. Its report is preserved in `sample_media/analysis-platform/ci-35352456861/report.json`. The corpus comparison is preserved in `sample_media/analysis-platform/corpus-windows-linux-determinism-20260918-v3.json`. `analysisbench -synthetic-results-out <new.json>` runs generated WAV fixtures through the actual decoder and combined production pipeline. `-determinism-results <comma-separated-files>` now supports standalone comparison and exposes measured differences even when one required platform is missing. A CI matrix runs the same generated fixtures on Windows, Linux, and macOS and publishes a scalar determinism report; this is regression evidence and does not replace lawful real-audio qualification.

The development-only external reference adapters normalize established BPM tools into the existing benchmark schema without becoming ViiB dependencies. The first whole-track `librosa` 0.11.0 control on the r5 tuning split used FFmpeg PCM at 22050 Hz, a 512-sample hop, `startBPM=120`, and `tightness=100`: 13/122 strict BPM (10.66%), one half/double error, no source failures, 157.99 seconds. The same-manifest production Go replay is 98/122 strict (80.33%), two half/double errors, and six unknown BPM results. `scripts/essentia_bpm_benchmark.py` then ran Essentia `RhythmExtractor2013` `multifeature` at 44.1 kHz as an AGPL external process, recording BPM and beat positions. Its frozen production-aligned 90–180 BPM tuning configuration reached 99/122 strict (81.15%), three half/double errors, and no unknowns in 461.66 seconds; the broad 40–208 range was 98/122. The frozen configuration then reached only **39/54 strict (72.22%)** on independent r5 held-out data, with two half/double errors and no unknowns in 199.72 seconds, so it is not a promotion target. It remains stronger beat-aware diagnostic evidence. See `docs/REFERENCE_BPM_BENCHMARK.md`; raw local artifacts remain under `sample_media/Test Corpus/`. Mixxx 2.3.3 cannot yet produce reliable output in this WSL environment because it consistently segfaults while setting up audio devices before its asynchronous scan commits a library row; it remains a separate GPL reference to revisit on a native audio-capable runner.

Benchmark outputs now retain the native post-selection beat-grid ticks, with finite/ascending validation and cross-platform tick determinism checks. This is diagnostic evidence only because the grid is constructed after BPM selection. The native `beat-interval-consistency` candidate instead scored observed onset-peak intervals with a bounded 100 ms refractory peak selector across existing periodicity hypotheses. It passed generated static-tempo/refusal fixtures but reached only **64/122 strict BPM (52.46%)**, one half/double error, six unknowns, and 62.58× real time on r5 tuning. It is rejected without a held-out run; production remains `tempo-v3-multifeature-half-bpm`.

The development-only MIT Beat This `final0` transformer reference produces timestamped beat grids rather than a native scalar BPM. Its CPU r5 tuning run took 1,429.42 seconds. The first single-tick median export was invalid for ±0.5 BPM scoring because 50 FPS tick quantization turns a 140 BPM interval into 142.86 BPM. The corrected fixed 16-beat median derivation over the same immutable ticks reaches **111/122 strict BPM (90.98%)**, three half/double errors, and no unknowns. This is diagnostic reference evidence only. The 2026-09-18 overlap audit against the upstream v1.0 annotations found direct Harmonix title matches in r5, including Darude — "Sandstorm", Alice Deejay — "Better Off Alone", Swedish House Mafia — "Don't You Worry Child", Cascada — "Everytime We Touch", Benny Benassi — "Satisfaction", Calvin Harris — "Feel So Close", and Michael Jackson — "Beat It"; the upstream split marks most matched entries as training data. Do not run `final0` on r5 held-out or use it for qualification or promotion. Production remains `tempo-v3-multifeature-half-bpm`.

## Delivered capabilities

- A backend-owned, local-first Go analysis stack with durable persistence and job lifecycle support.
- MP3 and Ogg/Vorbis analysis measurement support; decoder failures are retained as explicit source errors.
- `analysisbench`: validated corpus/result schemas, codec matrix, synthetic fixtures, browser-baseline exporter, Go result producer, CSV corpus importer, gate report, timing, and confidence-calibration reporting.
- Persisted BPM/key/beatgrid and shared energy/structure artifacts; beatgrid edits survive reloads.
- DJ UI for deck energy, advisory cues with opt-in acceptance, and local explainable transition recommendations.
- A held-out key candidate that meets the roadmap’s key tripwires on the current sample: 71.93% exact and 85.96% Camelot-compatible (49/57).
- Versioned production defaults based on the best measured candidates, with durable job/per-track diagnostics for incomplete, decoding, source, claim, and persistence failures.

## Quality gate: still open

The latest recorded held-out gate remains **do not claim professional readiness**. The figures below are historical measurements on the original manifest; the September 18 identity audit found four artist/title groups crossing tuning and held-out splits, so these figures cannot establish independent release qualification. The prior selected tempo candidate recorded:

| Measure | Current held-out evidence | Provisional gate |
|---|---:|---:|
| Strict BPM (±0.5) | 77.19% (44/57) | >= 85% full corpus; >= 95% stable electronic |
| Half/double BPM errors | 3.51% | <= 5% |
| Exact key | 71.93% | >= 65% |
| Camelot-compatible key | 85.96% | >= 85% |
| BPM improvement vs browser | 71.93 percentage points | >= 20 pp |
| Exact-key improvement vs browser | 29.82 percentage points | >= 10 pp |
| One-worker throughput | 57.40x real time | >= 5x real time |

The importer now resolves six additional artist/title matches while preserving all 184 existing labels, provenance declarations, and split assignments. Its explicit artist/title matching admits short titles only with artist identity and resolves duplicate titles; CSV truncation requires an explicit ellipsis and a matching artist/title prefix. The new raw manifest has 190 entries, but its 14 repeated label-derived recording groups include four cross-split groups. All 190 file SHA-256 values differ; differing encoded bytes do not prove distinct recordings.

The conservative `phase0-spotify-manifest-r5.json` subset retains one representative per declared recording group, preferring a tuning representative if a group crosses splits, without relabeling or reassigning any retained track. It has **176 groups: 122 tuning and 54 held out**, zero missing group identities, and zero declared overlap. It needs at least **24 additional independent recordings, including 13 new held-out recordings**, to reach 200/67. Label-derived groups still require review; this metadata audit cannot prove acoustic uniqueness. Required genre/case tags, real-audio refusal evidence, confidence calibration, and stable-electronic coverage remain incomplete. Three-platform generated-fixture determinism passes; this does not establish real-corpus decoder equivalence on macOS. Playlist names are not genre annotations.

Local audit artifacts: `phase0-spotify-manifest-r4.json`, `phase0-spotify-import-r4.json`, `phase0-corpus-content-hashes-r3.json`, `phase0-identity-subset-audit-r5.json`, and `phase0-corpus-coverage-r5.json` under `sample_media/Test Corpus/`. The gate now requires recording-group identity and blocks repeated/cross-split groups rather than treating copies as independent evidence. Legacy manifests still load, but missing identity is an explicit readiness deficit.

## Prioritized next actions

1. **Expand and tag the independent corpus.** Start from the conservative r5 subset; review recording identities and add at least 24 independently labeled `.mp3`/`.ogg` recordings, reserving at least 13 additions for held-out evaluation to reach 67/200. Use one `recordingGroup` for every copy/encoding of the same recording; do not move a previously tuned recording into held-out. Tag every required genre/case, especially `stable-electronic`; do not guess missing CSV labels or derive genres from playlist names. Preserve authoritative label source and license provenance.
2. **Improve tempo—not thresholds alone.** Use only the tuning split for candidate work, preserve each result in a new immutable artifact, then evaluate the chosen configuration once on the held-out split. The remaining gap is strict BPM accuracy; half/double performance already passes.
3. **Calibrate real-audio refusal and confidence behavior.** Measure unknown rates separately on real and synthetic material. Recalibrate `tempo.minOnsetCrestFactor` and `key.maxTonalChromaFlatness` from corpus evidence, and investigate the major/minor key-confidence asymmetry before fixing confidence buckets.
4. **Maintain cross-platform evidence.** Generated production fixtures now pass on Windows, macOS, and Linux within `1e-6`; CI preserves the report. Extend evidence to the same real-audio corpus on macOS before claiming real-corpus platform equivalence.
5. **Run and preserve the final gate report.** Once corpus coverage, candidate results, browser results, calibration, and platform evidence are available, use the gate report to make an explicit go/no-go decision. Do not silently lower a target; record a justified amendment, algorithm change, or feature descoping.

## Product and engineering guardrails

- Prefer `unknown` over a plausible but unsupported BPM/key result.
- Keep analysis backend-owned, local-first, cross-platform, and predominantly pure Go; do not make browser detection authoritative.
- Use one canonical local/Plex `song_id`; do not create a DJ-only catalog.
- Preserve provenance and confidence. Manual values must never be silently overwritten; AI-inferred metadata must stay distinct from measured analysis.
- A uniform BPM-derived grid is not a beatgrid. Sync, loops, and beat jumps should consume the persisted beatgrid only when appropriate.
- Recommendations and cues remain advisory, explainable, and user-overridable; no mandatory cloud service.
- Keep GPL/AGPL source and runtime dependencies out of production implementation. AAC/M4A and Opus remain gated by codec, legal, and throughput decisions.

## Deferred / owner decisions

| Decision | Why it matters |
|---|---|
| AAC/M4A decoder path | Requires a distribution/license decision between LGPL pure-Go and reviewed WASM artifacts. |
| Opus, FLAC, and AIFF scope | Focused evidence currently covers MP3/Ogg. AIFF also needs scanner, metadata, and duplicate-detection work. |
| Corpus provenance | Engineering cannot decide whether audio is lawfully held or labels are authoritative. |
| Professional threshold changes | Any change to a provisional tripwire requires a recorded justification, never an implicit relaxation. |

## Practical references

- Benchmark harness and reports: `backend/internal/analysisbench/`, `backend/cmd/analysisbench/`
- Analysis implementation: `backend/internal/analysis/`
- DJ API and behavior: `backend/internal/api/`, `backend/internal/dj/`
- DJ UI: `pages/DJModeV2.tsx`, `components/dj/v2/`, `hooks/useDJAudioEngine.ts`
- Full architecture, acceptance criteria, dependency register, and research citations: `docs/DJV2_PROFESSIONAL_TRACK_ANALYSIS_ROADMAP.md`

## Definition of done for the evidence gate

Call the system professionally ready only after a lawful, genre-stratified corpus of at least 200 tracks with a held-out third demonstrates the recorded BPM, key, half/double, unknown/refusal, confidence, throughput, codec, and Windows/macOS/Linux determinism targets. Until then, shipped features should be presented as advisory analysis rather than professionally accurate measurement.
