# DJv2 Track Analysis — Concise Context and Next Actions

**Source:** `DJV2_PROFESSIONAL_TRACK_ANALYSIS_ROADMAP.md`  
**Snapshot:** 2026-09-09  
**Purpose:** Operational handoff of the active context, completed work, quality gate, and next actions. The source roadmap remains the detailed research, design, and evidence record.

## Current position

The engineering slices for Phases 0–7 have landed, including persistent analysis, beatgrid/downbeat artifacts and edits, energy/structure artifacts, advisory cues, deck energy UI, opt-in cue acceptance, and explainable local transition recommendations. Phase 5–7 work was opened under an explicit project-owner override.

The production defaults now use the measured half-BPM multi-feature tempo candidate (crest 15) and the 36-bin HPCP/Krumhansl key candidate (3500 Hz, flatness 0.98, minimum profile margin 0.01). This repairs an operational mismatch where the user-visible job still ran the original synthetic-fixture prototypes and rejected nearly all mastered tracks. The algorithm versions were advanced so those rejected rows are selectable as outdated. Analysis jobs and per-track failures also write structured `[Analysis]` entries to an append-only-across-restarts `viib.log`.

This **does not authorize a professional-accuracy claim**. The Phase 0 evidence gate remains open. The current work is primarily corpus expansion, measurement, tempo improvement, calibration, and cross-platform proof—not foundational feature construction.

## Delivered capabilities

- A backend-owned, local-first Go analysis stack with durable persistence and job lifecycle support.
- MP3 and Ogg/Vorbis analysis measurement support; decoder failures are retained as explicit source errors.
- `analysisbench`: validated corpus/result schemas, codec matrix, synthetic fixtures, browser-baseline exporter, Go result producer, CSV corpus importer, gate report, timing, and confidence-calibration reporting.
- Persisted BPM/key/beatgrid and shared energy/structure artifacts; beatgrid edits survive reloads.
- DJ UI for deck energy, advisory cues with opt-in acceptance, and local explainable transition recommendations.
- A held-out key candidate that meets the roadmap’s key tripwires on the current sample: 71.93% exact and 85.96% Camelot-compatible (49/57).
- Versioned production defaults based on the best measured candidates, with durable job/per-track diagnostics for incomplete, decoding, source, claim, and persistence failures.

## Quality gate: still open

The latest recorded held-out gate remains **do not claim professional readiness**. The current best tempo candidate has:

| Measure | Current held-out evidence | Provisional gate |
|---|---:|---:|
| Strict BPM (±0.5) | 77.19% (44/57) | >= 85% full corpus; >= 95% stable electronic |
| Half/double BPM errors | 3.51% | <= 5% |
| Exact key | 71.93% | >= 65% |
| Camelot-compatible key | 85.96% | >= 85% |
| BPM improvement vs browser | 71.93 percentage points | >= 20 pp |
| Exact-key improvement vs browser | 29.82 percentage points | >= 10 pp |
| One-worker throughput | 57.40x real time | >= 5x real time |

Evidence is also incomplete: the lawful corpus has 184 unambiguous labeled MP3/Ogg tracks, requiring at least 16 more tracks and five additional held-out tracks, and is missing required genre/case tags. Confidence calibration, expected-unknown behavior on real audio, stable-electronic coverage, and macOS/Linux determinism are not yet demonstrated.

## Prioritized next actions

1. **Expand and tag the lawful corpus.** Add at least 16 unambiguous labeled `.mp3`/`.ogg` tracks; reserve at least five more for held-out evaluation. Tag every required genre/case, especially `stable-electronic`; do not guess ambiguous filename matches. Preserve authoritative label source and license provenance.
2. **Improve tempo—not thresholds alone.** Use only the tuning split for candidate work, preserve each result in a new immutable artifact, then evaluate the chosen configuration once on the held-out split. The remaining gap is strict BPM accuracy; half/double performance already passes.
3. **Calibrate real-audio refusal and confidence behavior.** Measure unknown rates separately on real and synthetic material. Recalibrate `tempo.minOnsetCrestFactor` and `key.maxTonalChromaFlatness` from corpus evidence, and investigate the major/minor key-confidence asymmetry before fixing confidence buckets.
4. **Produce cross-platform evidence.** Run the same fixtures/results on Windows, macOS, and Linux; record scalar determinism (or a documented tolerance no greater than `1e-6`) in a gate artifact.
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
