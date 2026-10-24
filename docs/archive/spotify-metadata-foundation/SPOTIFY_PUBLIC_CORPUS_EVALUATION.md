# Public corpus evaluation

> Archived on 2026-10-12. This is a historical checkpoint, not the current merge checklist. Further local scanning/DSP improvements and quality qualification are deferred to a future branch. See [Spotify behavior and limitations](../../spotify.md) and [deferred work](../../LOCAL_ANALYSIS_FUTURE_BRANCH.md). Original results and unchecked gates below remain historical evidence.

The existing external Beat This `final0`/minimal/span16 reference also ran on canonical ViiB exports of all 308 FSLD tuning recordings. It gives 66 strict BPM matches, 22 half/double errors and 152 unknowns, with zero source failures. In 142 unknown cases, ticks exist but the fixed 16-beat derivation lacks sufficient ticks. CPU process wall time is 208.589s and peak RSS 1,380,028 KiB; these measurements include initialization/inference and exclude canonical export/installation. The [reference report](evidence/spotify-fsld-beat-this-reference-2026-10-12.json) contains hashes, full-denominator metrics and resource limitations. It is not a promotion target. No shorter-span derivation was substituted, no product dependency was added and reserved recordings remain unanalyzed.

Training overlap is separate from the public audio license. [Upstream's versioned training-source inventory](https://github.com/CPJKU/beat_this_annotations/tree/v1.0) includes GuitarSet, so no GuitarSet model qualification was attempted. FSLD is not a named source in that inventory, but recording-level overlap has not been reviewed: all reference rows explicitly retain `overlap-unreviewed`. The [model guidance](https://github.com/CPJKU/beat_this#available-models) warns against using training material for fair evaluation. Existing canonical input, checkpoint and environment requirements remain documented in [the reference adapter guide](../../REFERENCE_BPM_BENCHMARK.md).

Latest tempo experiment: explicit 40–240 BPM tuning runs fail qualification. A wider refined search gives 37/308 strict matches with 98 half/double errors; the new benchmark-only phase-support candidate gives 86/308 with 94 half/double errors, versus production's 87/308 with 14. Synthetic success does not establish musical metrical selection. Neither candidate was promoted. [Configurations, source/result hashes and cohorts](evidence/spotify-fsld-phase-support-experiment-2026-10-12.json) retain the complete-denominator evidence. All source hashes remain unchanged and reserved recordings remain unanalyzed.

Reproduce from `backend`, using new non-overwriting output names:

```powershell
go run ./cmd/analysisbench -analyze ../.tmp-review/public-evaluation/fsld-assembled/manifest.json -analyze-split tuning -tempo-method phase-support -tempo-min-bpm 40 -tempo-max-bpm 240 -out ../.tmp-review/public-evaluation/fsld-assembled/go-tuning-phase-support-reproduction.json
go run ./cmd/analysisbench -manifest ../.tmp-review/public-evaluation/fsld-assembled/manifest.json -results ../.tmp-review/public-evaluation/fsld-assembled/go-tuning-phase-support-reproduction.json -split tuning
```

For the control, select `multifeature-refined-half-bpm` with the same bounds and a distinct output path. The paired range flags require finite bounds satisfying `20 <= min < max <= 400`, apply only to explicit benchmark analysis, and record the actual custom range in results. Production defaults remain 90–180 BPM with `multifeature-half-bpm` and crest 15. The phase-support candidate requires observed amplitude/flux periodicity and at least four peaks spanning three periods; it does not measure meter or downbeats. Further work must address real-audio metrical selection before a held-out evaluation is warranted.

The owner authorized public evaluation data on 2026-10-12. The first independent-label run uses the official [Schubert Winterreise Dataset v2.1](https://doi.org/10.5281/zenodo.10839767), by Christof Weiß, Frank Zalkow, Vlora Arifi-Müller, Meinard Müller, Hendrik Vincent Koops, Anja Volk and Harald G. Grohganz (ACM JOCCH, 2021). The release declares CC BY 3.0 for the dataset; its HU33 audio notice identifies Public Domain Mark 1.0. Audio remains in ignored evaluation storage and is not redistributed or added to the live library.

The official archive MD5 is `591c377c6d3db522fd159b8b70180978`; it was verified before importing. All 24 HU33 performances were reserved for evaluation before running production defaults. Performance-specific global keys come from `02_Annotations/ann_audio_globalkey.csv`, rather than score keys or automatically estimated tuning metadata. The release's main audio/annotations are used together, including its artificial repeat in song 01. Per-file hashes remained unchanged after analysis. No tuning options were passed.

| Measurement | Result |
| --- | --- |
| Successful analyses | 24/24, zero source errors |
| Exact global tonic and mode | 14/24 (58.33%) |
| Camelot-compatible global key | 19/24 (79.17%) |
| Key abstentions | 0/24 |
| BPM accuracy | Unscored: no global BPM ground truth declared |
| Structure boundaries, 0.5-second window | 9/153 references matched; micro F1 5.73% |
| Structure boundaries, 3-second window | 33/153 references matched; micro F1 21.02% |

See [key results and provenance](evidence/spotify-winterreise-key-evaluation-2026-10-12.json) and [structure results](evidence/spotify-winterreise-structure-evaluation-2026-10-12.json). Key accuracy on this corpus is below the plan's 65% exact / 85% compatible thresholds. This narrow corpus cannot establish the plan's stratified accuracy gate. The analyzer produced estimates for every key, so the misses cannot be explained by abstentions.

Structure scoring uses one-to-one matches between interior annotated section starts and predicted energy-region starts, at two declared tolerances. It clips predictions to each annotated interior and excludes annotation endpoints. Missing results count as missed references. The low boundary scores do not qualify local musical-form inference: energy regions and musical-form sections have different semantics, and their labels are not equated. Measure annotations are preserved in the upstream archive, not relabeled as beat ticks or global BPM.

Reproduce from the repository root, using new output paths:

```powershell
python scripts/import-winterreise-evaluation.py .tmp-review/public-evaluation/Schubert_Winterreise_Dataset_v2-1.zip .tmp-review/public-evaluation/swd-hu33
```

From `backend`:

```powershell
go run ./cmd/analysisbench -analyze ../.tmp-review/public-evaluation/swd-hu33/manifest.json -out ../.tmp-review/public-evaluation/swd-hu33/results.json -analyze-split held_out
go run ./cmd/analysisbench -manifest ../.tmp-review/public-evaluation/swd-hu33/manifest.json -results ../.tmp-review/public-evaluation/swd-hu33/results.json -split held_out
```

From the root:

```powershell
python scripts/evaluate-structure-boundaries.py .tmp-review/public-evaluation/swd-hu33/structure-reference.json .tmp-review/public-evaluation/swd-hu33/results.json .tmp-review/public-evaluation/swd-hu33/structure-report.json
python scripts/test-public-evaluation.py
```

The importer verifies the official archive and exact performance identities, copies only fixed HU33 members, preserves license/README notices, and leaves BPM labels absent. The benchmark now admits formats supported by its production decoder registry, including WAV, and exports structure separately from beats. Tests cover performance-specific key selection, duplicate labels, interval semantics, one-to-one maximal matching, missing outputs and invalid boundaries.

This is one classical art-song performance, not the required 200-track stratified corpus. Browser-baseline improvement, stable electronic material, tempo/meter/downbeat labels, perceptual attributes, supported-platform determinism and a reviewed go/no-go decision remain open. Analyzer-only throughput excludes database/artifact publication and GUI work. Preserve this evaluated set from tuning; later algorithm work needs separate tuning data and fresh qualification evidence. No acceptance gate is closed by these results.

## GuitarSet tuning diagnostics

The second corpus is [GuitarSet v1.1.0](https://doi.org/10.5281/zenodo.3371780), Q. Xi, R. Bittner, J. Pauwels, X. Ye and J. P. Bello, ISMIR 2018, declared CC BY 4.0. Only the mono microphone archive is used. The importer verifies the official annotation/audio MD5 checksums before copying fixed recording members. JAMS `tempo` and `key_mode` supply global labels; `beat_position` supplies a backing-track-derived beat/downbeat/meter grid. These are independent of Spotify, but are not manually labeled performed onsets. The acoustic guitar style names do not establish live-drum or electronic genre coverage.

Before any analysis, lead-sheet index 3 was reserved across all styles, both tempi, every player and comp/solo pairs. Indices 1 and 2 form tuning data. The [split inventory](evidence/spotify-guitarset-reserved-split-2026-10-12.json) records public recording IDs, group isolation and audio hashes. The importer excludes both recordings with [reported timing offsets](https://github.com/marl/GuitarSet/issues/5) and the recording with a [reported duplicated MIDI note](https://github.com/marl/GuitarSet/issues/4), without silently correcting the release. This leaves 238 tuning and 119 reserved recordings in 15 groups. Shared harmonic vocabulary can remain across groups; composition independence is not claimed.

The unchanged production-default tuning run has zero source errors, 47/238 BPM estimates within 0.5 BPM (19.75%), 137/238 exact keys (57.56%) and 177/238 compatible keys (74.37%). Four tempo and eleven key results abstain. Tempo confidence is nonmonotonic; key confidence lacks observations in the highest bucket. Within the default 90–180 BPM range, strict tempo accuracy is 47/179 (26.26%); all 59 outside-range cases miss. Range restriction alone does not explain the poor accuracy. See [tuning report](evidence/spotify-guitarset-tuning-default-2026-10-12.json).

At a fixed 70 ms, exported beat ticks match 4,753/13,328 reference beats with micro F1 33.89%. [Grid diagnostics](evidence/spotify-guitarset-grid-tuning-default-2026-10-12.json) count missing outputs as misses, use one-to-one matching, and fit no phase offset, tempo rescaling or metrical alternatives. Downbeats and meter remain unscored because the benchmark does not export those predictions. This diagnostic does not qualify continuity or perceptual alignment. These tuning measurements are not release-facing accuracy evidence.

```powershell
python scripts/import-guitarset-evaluation.py .tmp-review/public-evaluation/guitarset-annotation.zip .tmp-review/public-evaluation/guitarset-audio-mono-mic.zip .tmp-review/public-evaluation/guitarset
# From backend, use -analyze-split tuning before any calibration:
go run ./cmd/analysisbench -analyze ../.tmp-review/public-evaluation/guitarset/manifest.json -out ../.tmp-review/public-evaluation/guitarset/go-tuning-default.json -analyze-split tuning
# From root:
python scripts/evaluate-beat-grid.py .tmp-review/public-evaluation/guitarset/grid-reference.json .tmp-review/public-evaluation/guitarset/go-tuning-default.json .tmp-review/public-evaluation/guitarset/grid-tuning-default.json --split tuning
```

Use new output paths when reproducing. The browser baseline exporter now admits WAV/OGA filename extensions in its picker; running it still requires a functioning browser and Web Audio. No browser measurement is inferred from Go analysis.

Three existing tuning configurations were also measured, with all other options left at defaults:

| Tempo method / key configuration | Strict BPM | Exact key | Compatible key |
| --- | --- | --- | --- |
| Default half-BPM / HPCP, Krumhansl, 3500 Hz | 47/238 (19.75%) | 137/238 (57.56%) | 177/238 (74.37%) |
| Refined half-BPM / HPCP, Temperley, 3500 Hz | 80/238 (33.61%) | 131/238 (55.04%) | 177/238 (74.37%) |
| Beat interval consistency / HPCP, Temperley, 2000 Hz | 43/238 (18.07%) | 135/238 (56.72%) | 177/238 (74.37%) |
| Onset autocorrelation / direct chroma, Krumhansl, 2093 Hz | 18/238 (7.56%) | 93/238 (39.08%) | 145/238 (60.92%) |

All four runs have zero source errors. The refined candidate has 19 half/double errors (7.98%), above the plan's 5% ceiling, and remains far below its broad BPM requirement. Alternative key configurations do not improve on the default. [Candidate evidence](evidence/spotify-guitarset-tuning-candidates-2026-10-12.json) retains exact options, result hashes, abstentions and calibration. No candidate was promoted and no production default changed. The 119 reserved GuitarSet recordings remain unanalyzed. Candidate runs overlapped in time, so their wall times are not comparable performance evidence.

## Electronic-loop acquisition and tuning diagnostics

The [Freesound Loop Dataset](https://doi.org/10.5281/zenodo.3967852), A. Ramires et al., ISMIR 2020, provides expert annotations separately from automatic metadata. Its annotation archive passed official MD5 `3920ee437802cf047a990b2968fa066c`. Per-sound metadata was obtained from the official audio archive using bounded HTTPS byte ranges and ZIP member CRC validation, transferring 4,203,332 bytes rather than downloading the entire 8.8 GB archive. The full archive MD5 was not verified. The source member SHA256 is recorded.

Before any FSLD audio analysis, the selector requires at least two annotations, no discarded/deferred votes, unanimous positive defined BPM, unanimous `well_cut`, and a per-sound CC0 1.0 or CC BY 3.0 license. All sounds from an uploader share a deterministic split. This yields [487 candidates](evidence/spotify-fsld-public-selection-2026-10-12.json): 308 tuning and 179 reserved across 212 uploader groups. Exact audio duplicate isolation remains required after acquisition. The agreed labels include 156 electronic, 57 house/techno, 85 bass music and 22 hip-hop cases; labels overlap. Broad combined genres are preserved, not converted into separate required strata. Unanimous meter is 4/4 on 482 candidates; five have no agreed meter.

All 487 selected recordings have now been acquired and admitted to the evaluation manifest after integrity, attribution and global duplicate checks. No exact source-byte duplicates or conflicting duplicate labels were found. All 308 tuning recordings decode successfully through the production registry; the 179 reserved recordings remain unanalyzed. Many key labels are `none`; these are scored as dimension-specific abstentions without discarding their defined BPM labels. This manifest is evaluation input, not a qualified release corpus.

Reproduce selection with the same acquired metadata and a new output path:

```powershell
python scripts/select-fsld-evaluation.py .tmp-review/public-evaluation/fsld-annotations.zip .tmp-review/public-evaluation/fsld-metadata.json .tmp-review/public-evaluation/fsld-selection.json
```

Twelve public importer/selection/scoring tests pass. FSLD tuning measurements remain diagnostic evidence, not acceptance evidence.

The acquisition tool now binds the archive's actual `audio/wav/<sound>_<uploader>.<source-extension>.wav` members (including `.wav.wav`) to exact sound IDs. Bounded HTTPS range reads require HTTP 206, an exact Content-Range and exact response length. Truncated transfers retry without admitting partial bytes; a server that ignores ranges is rejected before reading its body. ZIP CRC, per-file SHA256 and adjacent per-sound attribution are retained. Atomic checkpoints have bounded retries for transient Windows sharing/access errors. The full remote archive checksum is still unverified.

Acquisition completed in four disjoint batches derived from the original reserved selection, reusing 161 verified files from the initial sequential download. Batch partitions did not alter tuning/held-out assignments. Before model analysis, the assembler verified every batch label/split against the original selection, audio hashes/attribution, and source-byte duplicates globally. Every duplicate spanning splits would be excluded; within-split copies with conflicting BPM/key labels would all be excluded. Otherwise one deterministic copy remains. [Final acquisition evidence](evidence/spotify-fsld-acquisition-2026-10-12.json) records 487 selected, available and admitted recordings, all with PCM16 WAV headers, and zero exact duplicates. Transformed or near duplicates are not proven absent. Header validation does not establish decoding of the reserved split.

```powershell
# Sequential acquisition is also reproducible from the full selection:
python scripts/import-fsld-evaluation.py docs/spotify-fsld-public-selection-2026-10-12.json .tmp-review/public-evaluation/fsld-metadata.json .tmp-review/public-evaluation/fsld
# Assemble the completed four batches:
python scripts/assemble-fsld-evaluation.py docs/spotify-fsld-public-selection-2026-10-12.json .tmp-review/public-evaluation/fsld-assembled .tmp-review/public-evaluation/fsld-batch-0 .tmp-review/public-evaluation/fsld-batch-1 .tmp-review/public-evaluation/fsld-batch-2 .tmp-review/public-evaluation/fsld-batch-3
```

Benchmark manifests now accept `expectedBpmUnknown` and `expectedKeyUnknown` independently. A defined BPM with an expert key-none label remains BPM-labeled and is separately scored for key refusal. A dimension cannot have both a value and a refusal label. Per-field refusals require an explicit successful/partial/unknown analysis status and no errors; missing, failed and untagged outputs are not successful refusals. Additional ≥90% per-field tripwires supplement the original both-unknown requirement, which remains mandatory. Failed source outputs also cannot inflate positive accuracy or confidence calibration. Frozen Spotify comparisons clear human refusal labels in their separate provider projection without changing the original corpus. Twelve public-tool tests and affected Go suites/vet pass.

GuitarSet's intended/backing-track tempo and grid are not manually measured performed guitar timing. Performance variation can contribute to strict BPM error. A three-recording tuning-only diagnostic recomputed refined period scores but changed no selected BPM. [Diagnostic evidence](evidence/spotify-tempo-rescore-diagnostic-2026-10-12.json) records the cases, options and source hashes. The experimental method was removed, with snapshots retained in ignored evaluation storage; no production estimator/default changed. These selected cases do not prove full-corpus equivalence or a universal cause of misses.

The [FSLD default tuning report](evidence/spotify-fsld-tuning-default-2026-10-12.json) retains all 308 recordings: 87 strict BPM matches (28.25%), 14 half/double errors (4.55%), 88 tempo abstentions and zero source errors. Among 62 agreed major/minor key labels, 31 are exact (50%), 42 compatible (67.74%) and two unknown. Only 35/124 agreed key-none labels are correctly refused (28.23%); 89 receive false-positive keys. Tempo confidence is nonmonotonic and no positive-key result reaches the medium/high confidence buckets.

Durations range from 0.484 to 80 seconds. Post-run duration diagnostics show strict BPM matches of 39/136 below eight seconds, 27/84 from eight to sixteen seconds, and 21/88 at sixteen seconds or longer. Short single-pass loops offer limited periodicity evidence, but length alone does not explain the misses. All 41 labels outside the default 90–180 BPM bounds miss. These diagnostic cohorts do not replace the full denominator or establish a new acceptance population. Analyzer-only throughput is 51.173 seconds for 3,902.645 seconds of audio, excluding publication and GUI work.

A reproduced tuning-only candidate uses the existing HPCP/Krumhansl options with a 2500 Hz maximum and 0.95 chroma-flatness ceiling. It improves exact keys to 33/62 and compatible keys to 43/62, with five unknowns; correct key-none refusals improve to 74/124, leaving 50 false positives. [Candidate evidence](evidence/spotify-fsld-tuning-key-candidate-2026-10-12.json) retains options and hashes. Neither accuracy nor refusal is sufficient for promotion. No production default changed and no FSLD held-out recording was analyzed.

On the separate GuitarSet tuning population, the same candidate gives 135/238 exact keys, 178/238 compatible and eight unknowns, versus default 137 exact, 177 compatible and eleven unknowns. The [acoustic regression report](evidence/spotify-guitarset-tuning-key-candidate-2026-10-12.json) preserves this tradeoff. FSLD improvement does not establish a general accuracy improvement. All 119 reserved GuitarSet recordings remain unanalyzed. The FSLD source hashes were reverified after analysis for all 487 recordings. Its readiness report also conservatively flags repeated uploader groups for identity review; uploader isolation and zero byte-identical files do not prove recording or near-duplicate independence.

## Windows/Linux determinism

An actual Debian/WSL run of the Linux amd64 benchmark matches Windows on all 24 frozen HU33 recordings, including scalar/key/status/error/configuration, beat positions, structure availability, duration and boundaries at the comparator's 1e-6 tolerance. All input hashes still match the reference captured before the original Windows evaluation. [Execution evidence](evidence/spotify-winterreise-windows-linux-determinism-2026-10-12.json) records source/result/executable hashes and environments. The Linux executable was cross-compiled with CGO disabled and then executed on Linux; this establishes decoder/DSP agreement on these inputs, without qualifying SQLite, desktop workflows or native Linux installation. The determinism gate remains unproven because macOS execution is missing. Acceptance remains 2/12 checked and 10/12 partial.

## Tempo refusal and refinement ablations

The [nine-recording diagnostic](evidence/spotify-fsld-tempo-diagnostic-2026-10-12.json) selects default tuning failures to investigate near misses, half/double errors and abstentions. Its selected-case results are not population accuracy evidence. Eight existing-option ablations show that lowering onset crest to 10 recovers one correct long-loop tempo, and refined periodicity corrects one near miss. Range presets force some answers into the expected band but introduce other folds and errors; no preset was selected. Range runs used a temporary package-local producer with explicit options; the retained public result configurations are authoritative, and no production source/default was retained from the diagnostic.

The two positive signals were then tested across all 308 tuning recordings, followed by separate full-population ablations:

| Configuration | Strict BPM | Half/double errors | Unknown |
| --- | --- | --- | --- |
| Default | 87/308 (28.25%) | 14/308 (4.55%) | 88 |
| Refined periodicity, crest 15 | 106/308 (34.42%) | 19/308 (6.17%) | 87 |
| Default periodicity, crest 10 | 100/308 (32.47%) | 17/308 (5.52%) | 38 |
| Refined periodicity, crest 10 | 123/308 (39.94%) | 22/308 (7.14%) | 37 |

[Full ablation evidence](evidence/spotify-fsld-tempo-tuning-ablations-2026-10-12.json) retains configurations, source/result hashes and complete comparisons. All runs have zero source errors and unchanged key outputs. All 487 input hashes were reverified afterward, and no reserved ID appears in any candidate output. The combined candidate has monotonic tempo confidence buckets, but every candidate exceeds the 5% half/double ceiling and remains far below broad tempo accuracy requirements. None was promoted. The single-factor runs overlapped, so their timing is not comparative performance evidence.

Temporary component diagnostics further distinguish honest refusal causes. A 66-second loop with crest 10.936 has candidates in both onset representations and is recovered at crest 10. A 48-second loop with crest 22.108 has no amplitude-onset candidates and only weak flux candidates (maximum recorded score 0.0137); removing the two-representation guard is not justified by that case. A 0.484-second loop yields no candidates from either representation and contains about one labeled beat period; it cannot establish repeated periodicity from a single pass. The raw diagnostic has 480 energy and 40 flux envelope samples, so the generic fewer-than-eight-samples guard is not a demonstrated cause for this recording. Detailed component output is retained in ignored evaluation storage; temporary source files were removed. Missing BPM is not universally a scan failure, and these cases do not justify fabricating a tempo or weakening retry settlement.
