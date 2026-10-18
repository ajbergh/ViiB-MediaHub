# DJ auto-cue accuracy research

Research date: 2026-10-17. Scope: current implementation review, external research,
and a proposed improvement/evaluation plan. No production algorithm changes or new
real-audio accuracy measurements are included. Recommendations below are hypotheses
to test, not measured improvements.

## Recommendation

Generate cues from detected musical events, then align and rank them for DJ use.
Replace the current preference for evenly distributed timestamps with evidence for
first useful beat, major section changes, drops/choruses, breakdowns, vocal entrances,
and mixable intro/outro phrases. Allow fewer than eight cues when evidence is weak.

Develop two candidates against the same annotated tracks:

1. A lightweight native baseline using multi-feature structural novelty and existing
   beat evidence, with honest uncertainty about bar position and semantic labels.
2. A hybrid using qualified beat/downbeat predictions plus structural boundaries;
   compare Beat This! with All-In-One rhythm and structure outputs before selecting
   a runtime. Neither model alone determines whether a cue is useful to a DJ.

Start with a small reviewed cue corpus and replayable baseline outputs. Tuning the
existing percentage weights alone cannot fix missing musical-event evidence.

## What happens today

| Stage | Current behavior | Accuracy consequence |
| --- | --- | --- |
| Decode/analysis | One streaming decode feeds onset, key, phase, energy, waveform, three-band, and loudness accumulators | Useful signals exist, but several are not passed to cue selection |
| Structure | 500 ms RMS windows, track-maximum normalization; split after at least four seconds when adjacent energy differs by at least 0.30 | Coarse loudness jumps substitute for structural boundaries; gradual or equal-loudness changes can disappear |
| Native rhythm | Global BPM and one onset-derived phase; straight grid, four beats per bar | Beat timing does not establish musical bar one; drift and non-4/4 material need separate handling |
| Candidates | Section starts/ends plus every beat; qualified measured/manual downbeats get a higher score | Most candidates carry timing evidence without evidence of a key moment |
| Selection | Targets 1.5%, 10%, 25%, 40%, 55%, 70%, 84%, 96% of duration; score is 0.62 proximity + 0.23 confidence + 0.15 salience | Duration coverage dominates selection |
| Labels | Intro, Mix In, Section 1–4, Mix Out, Outro assigned by slot | Names do not prove those musical functions were detected |
| Validation | Tests cover ordering, policy, provenance, invalid inputs and cue lifecycle | Passing tests does not establish musical accuracy |

Source anchors:

- [Generator and scoring](../backend/internal/analysis/cues/generator.go), especially
  `slotTargets`, `Generate`, and `minimumSpacing`.
- [Energy segmentation and structure hints](../backend/internal/analysis/features/energy.go),
  `Accumulator`, `segment`, `AnnotateStructure`, and `AddCueSuggestions`.
- [Analysis orchestration and persistence](../backend/internal/analysis/track/track.go),
  `Analyze` path and `PersistWithAutoCueMode`.
- [Native phase detection](../backend/internal/analysis/beatgrid/detector.go) and
  [grid provenance](../backend/internal/analysis/beatgrid/grid.go).
- [Generator tests](../backend/internal/analysis/cues/generator_test.go).

For a four-minute track, the targets are 3.6, 24, 60, 96, 132, 168, 201.6 and
230.4 seconds, regardless of where its drops or vocals occur. This is an arithmetic
illustration of the policy, not an observed track result. Another issue is that
section salience uses absolute section energy rather than evidence of a meaningful
change. Loud sections and useful section entrances are different concepts.

`Generate` currently consumes section boundaries/energy and grid positions. It
does not consume `CueSuggestions`, semantic section labels/confidence, vocal/drum
activity, or the existing three-band overview. `AddCueSuggestions` is a separate
energy-boundary-to-grid path; improving only that function would not improve
persisted hot cues. Also, section-boundary candidates can remain off-grid: the
generator does not universally snap every selected boundary.

The [cue API](../backend/internal/api/v2_analysis_cues.go) regenerates candidates
from stored analysis and the grid, checks source freshness, and applies merge
policies. [The cue editor](../components/dj/v2/DJAnalysisCueEditor.tsx) applies them;
[deck loading](../hooks/useDJAudioEngine.ts) loads persisted hot cues. The evidence
points primarily to analysis/selection limitations; it does not rule out timing
issues on particular files, which need audible replay against canonical audio.

## Define a useful cue before selecting a detector

| Cue role | Desired event | Evidence and exceptions |
| --- | --- | --- |
| Start | First useful stable beat after silence/pickup | Sustained rhythmic activity; preserve pickups rather than assuming time zero is bar one |
| Mix in | Start of a usable incoming phrase | Stable rhythm and enough runway before a disruptive change; low vocal activity only when measured |
| Drop / chorus | Arrival of a major payoff | Structural change, repetition/context, bass/drum return; a chorus is not automatically an EDM drop |
| Breakdown / reset | Start of a sustained reduction/change in arrangement | Texture/rhythm changes and context, not just one quiet transient |
| Vocal entrance | Beginning of a meaningful vocal passage | Vocal activity evidence; may precede a downbeat and should retain the actual event time |
| Mix out | Start of a usable outgoing phrase | Enough remaining bars and a compatible arrangement; avoid choosing the end of audible music |
| Outro | Start of the closing section | Structure plus remaining duration; never infer it solely from the 96% timestamp |

Separate an **event marker** at the musical arrival from an optional **preparation
cue** a chosen number of bars before it. A DJ may want both “Drop” and “16 bars to
drop”; these must not be evaluated against the same timestamp. Bar counts require
reliable meter/downbeats and must stay within the decoded track. These roles are
a proposed product policy, not universal genre rules.

## Research options

### Native multi-feature structural novelty

The FMP research notebook explains novelty from local self-similarity: compare the
musical context before and after a point rather than relying on a single loudness
jump. Kernel scale affects which changes are detected. This supports testing
multiple musical timescales. [FMP novelty segmentation][fmp]

Proposed native baseline: accumulate compact, time-local low/mid/high energy,
onset density/flux and timbral features; add time-local chroma when useful. The
existing whole-track key estimate is not a chroma timeline. Aggregate by beats
when reliable, compare context at several bar-scale hypotheses, and combine
change signals with persistence and local peak selection. Validate the resolution
of the existing three-band overview before reusing it for boundary timing.

Use bounded local context or sparse summaries for long tracks. A dense full-track
self-similarity matrix has quadratic storage cost. Reuse the shared decode; avoid
retaining full PCM just for cue ranking. This path is attractive for deployment,
but still cannot promise vocal identification or reliable functional labels.

### Beat This! for beat/downbeat evidence

Beat This! emits beats and downbeats, supports CPU/GPU inference, and offers full
and small checkpoints. Its repository lists MIT terms for code and published
weights and explicitly warns about evaluation on training material. Compare
`final0` and `small0` for timing, continuity, CPU latency and memory on target
hardware. It supplies rhythm evidence, not drop/chorus labels. [Official implementation][beatthis]

The paper reports remaining failure cases and continuity limitations, so a global
BPM score is insufficient qualification. [Beat This! paper][beatpaper]

### All-In-One for metrical and functional structure

All-In-One jointly predicts rhythm and functional segments such as intro, verse,
chorus, bridge and outro. Its label vocabulary does not directly establish
DJ-specific drop or mix-safe events; preserve native labels and assess mappings
against DJ annotations. [Official implementation][allinone], [paper][allpaper]

ViiB's existing adapter targets the inference fork `allin1_infer`, pinned to
`all-in-one-infer==3.0.0`, rather than the original `allin1` package. The fork's
current documentation describes different dependency/distribution arrangements
from older releases. Treat its runtime and outputs as a separately pinned
candidate; verify checkpoint identity, dependency terms, and Windows behavior.
Do not copy original-package installation instructions into this adapter.
[Inference fork][infer], [release history][inferreleases]

Separation adds processing cost; measure that cost rather than assuming published
GPU throughput applies to this app. Existing `.viibstems` packages might eventually
provide vocal/drum activity, but only after validating stem identity and timeline
alignment. Cue improvements should also work when stems are unavailable.

## Proposed selection pipeline

1. **Establish audio time.** Use ViiB's canonical decoded timeline, source hash and
   decoder identity. Do not independently decode compressed audio in each model
   adapter. Validate any internal resampling/time conversion against that origin.
2. **Measure rhythm uncertainty.** Keep beat, downbeat, meter and phrase confidence
   separate. Preserve manual grids. Inferred every-fourth-beat positions remain
   inferred, even if tempo is accurate. A model prediction is not ground truth.
3. **Detect events independently of slots.** Propose sustained structural changes,
   repetition entrances, bass/drum returns, silence-to-rhythm starts and optional
   vocal events. Retain raw time, event type, evidence and uncertainty.
4. **Align conditionally.** Refine coarse boundaries with local onset evidence and
   snap to a nearby qualified downbeat only within a tuned tolerance. Record the
   offset. Fall back to beat alignment or an unsnapped event with lower confidence;
   never move a pickup/vocal onset far away merely to satisfy a grid.
5. **Rank for role and usefulness.** Combine structural salience, rhythmic certainty,
   sustained before/after contrast, role evidence, remaining runway and diversity.
   Use 4/8/16/32-bar phrase hypotheses as soft context, not a clock starting at zero.
   Learn/tune weights on reviewed data; keep ranking score separate from calibrated
   probability that a DJ accepts the cue.
6. **Select up to eight.** Prefer distinct useful events. Merge duplicate evidence
   for one event; do not create one cue per beat or force absent roles. Use
   beat/bar-aware spacing with exceptions for intentionally distinct events.
   Assign slots after selection in chronological order and label by evidence.
7. **Explain and preserve.** Show event reason, alignment quality and uncertainty.
   Preview suggestions before broad automatic rollout. Retain user/locked cues,
   suppressions, source freshness and existing apply modes.

Changing slot meaning can interact with slot/kind suppressions. Define a migration
policy before rollout; changing generator version alone does not establish user
intent. Fingerprints identify audio changes, while feature/model/generator versions
must identify analysis changes. Regenerate from compatible new artifacts and
recompute after manual grid edits; never silently replace protected cues.

## Evaluation that answers the actual complaint

Use the existing [rhythm/structure benchmark contract](REFERENCE_RHYTHM_STRUCTURE_BENCHMARK.md),
canonical WAV exporter and external adapters. The documented old BPM diagnostics
have training overlap and cannot establish independent cue accuracy. Preserve
reserved splits described in [deferred analysis work](LOCAL_ANALYSIS_FUTURE_BRANCH.md).
This research did not run models or consume reserved audio; historical environment
blockers in those documents were not reverified as current machine state.

Proposed first dataset: 60–100 full tracks, balanced across the collection's main
genres and arrangements. Include extended club mixes, radio edits, live/variable
tempo, half/double-time ambiguity, long silence, pickups, vocal-led openings,
quiet/ambient intros and short tracks. Split by recording family and artist where
feasible; audit training overlap and keep related edits/remasters together.

For each track, reviewers annotate event type/time, acceptable alternative cue
points, usable mixing intervals and minimum runway, plus verified beats/downbeats
around key events. Use two reviewers on a subset and adjudicate disagreement.
Allow “no useful cue” and multiple valid choices: DJ preparation has subjective
alternatives. Hide generator identity during listening review.

| Metric | Definition |
| --- | --- |
| Event precision/recall | One-to-one same-role matching to reference events; score at 0.5 beat and one beat tolerance; alternatives for one event count once |
| Placement error | Median and 90th-percentile absolute beat distance of matched cues, plus seconds; misses remain in recall |
| Alignment | Beat/downbeat F1 at declared timing tolerance, bar-one offset, and continuity/drift; do not hide wrong-bar errors behind BPM correctness |
| Structure | Boundary F1 at 0.5 s and 3 s, plus label macro-F1 where supported; these are diagnostic alongside tighter cue metrics |
| Mixing usefulness | Cue inside an annotated usable interval with the required runway; score mix-in and mix-out separately |
| DJ acceptance | Fraction kept unchanged after blind preview; report moved, deleted, relabeled and manually added cues |
| Coverage / restraint | Tracks with a useful cue by role; false semantic labels, duplicates, cues in silence, and abstention rate |
| Operational cost | Cold/warm CPU time, real-time factor, peak memory, cache size, cancellation and offline behavior on target platforms |

`mir_eval` provides established beat and segment evaluation tools, including
boundary detection and continuity metrics. Add custom role/window/alternative
matching for DJ cues; structural segmentation scores alone do not measure DJ
usefulness. [Beat metrics][evalbeat], [segment metrics][evalsegment]

Compare: (A) unchanged generator; (B) event selection on existing features;
(C) native richer features; (D) qualified rhythm plus native structure;
(E) learned joint structure/rhythm. Keep cue budget and review protocol comparable.
Ablations separate better timing from better event selection. Freeze thresholds,
weights and model/checkpoint identities before opening the held-out split.

Suggested provisional promotion gate, to agree before evaluation: at least 80%
unchanged DJ acceptance and a 15 percentage-point improvement over baseline;
no increase in false semantic labels or lost important events; no protected-cue
regressions. Report track-level uncertainty intervals and genre breakdowns. These
numbers are proposed product targets, not literature results or achieved scores;
expand the corpus if it cannot support a clear decision. Abstention must not
artificially improve acceptance by removing useful coverage.

## Implementation sequence and decision points

| Step | Deliverable | Decision enabled |
| --- | --- | --- |
| 1 | Annotated tuning set, stored baseline cues, listening review and failure taxonomy | Which errors actually dominate this library? |
| 2 | Event-first native prototype and replayable cue evaluator | How much improvement comes from selection alone? |
| 3 | Full/small Beat This and All-In-One comparisons using existing adapters | Does learned rhythm/structure improve downstream cues enough to justify runtime cost? |
| 4 | Freeze candidate; independent held-out run and platform/resource checks | Is automatic application justified? |
| 5 | Suggestion-first rollout with versioned artifacts and protected-cue regression checks | Can users review useful results and corrections safely? |

The existing [DJ roadmap](DJV2_PRO_STEMS_CUES_MIX_INTELLIGENCE_ROADMAP.md), sections
11.1, 12 and 36–37.1, already identifies the bar-one gap and richer cue inputs.
This proposal makes event selection, abstention, preparation-versus-event cues,
and downstream acceptance the concrete research focus. It recommends revising
the roadmap's optional phrase-fill behavior: weak fallback cues should stay
optional suggestions rather than fill pads automatically.

## Sources

External sources accessed 2026-10-17. Recheck exact commits, weights and dependency
licenses when freezing experiments; living README pages are not reproducibility pins.

[fmp]: https://www.audiolabs-erlangen.de/resources/MIR/FMP/C4/C4S4_NoveltySegmentation.html
[beatthis]: https://github.com/CPJKU/beat_this
[beatpaper]: https://arxiv.org/abs/2407.21658
[allinone]: https://github.com/mir-aidj/all-in-one
[allpaper]: https://arxiv.org/abs/2307.16425
[infer]: https://github.com/openmirlab/all-in-one-infer
[inferreleases]: https://github.com/openmirlab/all-in-one-infer/releases
[evalbeat]: https://mir-eval.readthedocs.io/latest/api/beat.html
[evalsegment]: https://mir-eval.readthedocs.io/latest/api/segment.html
