# External BPM reference benchmark

This development-only harness compares established tempo tools with ViiB on
the same locally authorized corpus. It never adds an analyzer to the desktop
application, bundles a runtime, or copies source from a reference project.

The initial adapter uses `librosa.beat.beat_track`: an ISC-licensed reference
implementation of onset-strength, tempo estimation, and dynamic-programming
beat tracking. FFmpeg is used only to decode local audio into mono PCM for the
reference process. Neither belongs to the ViiB production dependency graph.

Run it from `backend`, so the relative paths in the local manifest resolve:

```powershell
python -m venv ..\.reference-bpm-venv
..\.reference-bpm-venv\Scripts\python.exe -m pip install --upgrade pip
..\.reference-bpm-venv\Scripts\python.exe -m pip install -r ..\tools\reference-bpm-requirements.txt

..\.reference-bpm-venv\Scripts\python.exe ..\scripts\reference_bpm_benchmark.py `
  --manifest '..\sample_media\Test Corpus\phase0-spotify-manifest-r5.json' `
  --split tuning `
  --out '..\sample_media\Test Corpus\reference-librosa-tuning-YYYYMMDD-v1.json'

go run ./cmd/analysisbench `
  -manifest '..\sample_media\Test Corpus\phase0-spotify-manifest-r5.json' `
  -results '..\sample_media\Test Corpus\reference-librosa-tuning-YYYYMMDD-v1.json' `
  -split tuning > '..\sample_media\Test Corpus\reference-librosa-tuning-comparison-YYYYMMDD-v1.json'
```

The adapter refuses to overwrite a result artifact, records tool/decoder
versions and parameters, includes failed tracks, and defaults to tuning only.
After selecting and freezing a configuration from tuning, a held-out run needs
the explicit `--allow-held-out` acknowledgement and a new output filename.

Keep the private audio and generated local artifacts out of Git. Compare BPM
metrics only for a BPM-only reference: absent key output is expected and must
not be used to judge ViiB key detection.

Mixxx/Queen Mary and Essentia remain valuable external comparators. Their GPL
and AGPL terms respectively make them development/reference tools under the
current project rules; do not copy, translate, link, or ship their code. Add
their adapters only as separate processes that emit the same result schema,
with their exact version, settings, command, and license recorded in the
artifact.

## Essentia reference adapter

`scripts/essentia_bpm_benchmark.py` runs Essentia's
`standard.RhythmExtractor2013` in a separate AGPL development environment. It
emits BPM plus ascending beat positions in seconds using the same result schema;
beat positions are evidence for a later grid-quality evaluation and are not
currently part of the Phase 0 BPM score.

On Debian/WSL, create an isolated environment outside the repository and run
the adapter from `backend`:

```bash
python3 -m venv /tmp/viib-essentia-venv
/tmp/viib-essentia-venv/bin/pip install -r ../tools/reference-essentia-requirements.txt

/tmp/viib-essentia-venv/bin/python ../scripts/essentia_bpm_benchmark.py \
  --manifest '../sample_media/Test Corpus/phase0-spotify-manifest-r5.json' \
  --split tuning --sample-rate 44100 --min-tempo 90 --max-tempo 180 \
  --out '../sample_media/Test Corpus/reference-essentia-multifeature-90-180-tuning-YYYYMMDD-v1.json'

go run ./cmd/analysisbench \
  -manifest '../sample_media/Test Corpus/phase0-spotify-manifest-r5.json' \
  -results '../sample_media/Test Corpus/reference-essentia-multifeature-90-180-tuning-YYYYMMDD-v1.json' \
  -split tuning > '../sample_media/Test Corpus/reference-essentia-multifeature-90-180-tuning-comparison-YYYYMMDD-v1.json'
```

The adapter records the tool version, method, sample rate, tempo range, full
track policy, manifest hash, beat-position units, and wall time. For
`multifeature`, it maps Essentia's documented 0–5.32 raw confidence into the
benchmark schema's 0–1 range by dividing by 5.32 and clamping. It defaults to
tuning; a held-out run requires `--allow-held-out` after configuration freeze.

## Beat This reference adapter

`scripts/beat_this_bpm_benchmark.py` runs the MIT-licensed Beat This `final0`
transformer model as a separate CPU-only development process. It emits the
model's ascending beat timestamps in seconds and derives BPM as `60 / median`
of 16-beat periods. This avoids the model's 50 FPS single-tick quantization
while rejecting isolated missing or spurious ticks. This is an external
comparator, not a ViiB dependency or production candidate.

Install its pinned CPU environment outside the repository, download the
checkpoint once, verify its SHA-256, and then run from `backend`:

```bash
python3 -m venv /tmp/viib-beat-this-venv
/tmp/viib-beat-this-venv/bin/pip install -r ../tools/reference-beat-this-requirements.txt

/tmp/viib-beat-this-venv/bin/python ../scripts/beat_this_bpm_benchmark.py \
  --manifest '/private/research/canonical-r5.json' \
  --path-base '/private/research' \
  --checkpoint ~/.cache/torch/hub/checkpoints/beat_this-final0.ckpt \
  --overlap-audit '/private/research/beat-this-r5-overlap-audit.json' \
  --split tuning \
  --out '/private/research/reference-beat-this-final0-tuning-YYYYMMDD-v1.json'
```

The adapter refuses model downloads and artifact overwrites. It requires a
ViiB-decoded canonical WAV and matching timing metadata for each manifest
track; it does not independently decode the original source. It records the
canonical PCM/WAV hashes, checkpoint hash, Beat This/Torch/Torchaudio versions,
CPU device, `dbn=false`, minimal postprocessor, BPM derivation, manifest hash,
and wall time. The normalized output includes raw beat and downbeat timestamps.
Use the companion [rhythm/structure benchmark contract](REFERENCE_RHYTHM_STRUCTURE_BENCHMARK.md)
for the manifest timing fields and overlap audit. Its first run is tuning only;
use `--allow-held-out` only after freezing this exact configuration. Keep model
files and generated artifacts out of Git.

`final0` was trained on most data used by the upstream project. Its authors
warn that measurements can be unfairly high on training material. The required
overlap review was completed against version `v1.0` of the published
annotations (commit `c3c47fd37d3074d9f8119f18bbf460f909609f22`) on
2026-09-18. It found direct Harmonix title matches for corpus recordings
including Darude — "Sandstorm", Alice Deejay — "Better Off Alone", Swedish
House Mafia — "Don't You Worry Child", Cascada — "Everytime We Touch", Benny
Benassi — "Satisfaction", Calvin Harris — "Feel So Close", and Michael
Jackson — "Beat It". The published `single.split` marks most of those
annotation entries as training data. The r5 corpus is therefore not independent
of `final0`; do not run it on r5 held-out or use any `final0` result as
qualification evidence. Both tuning artifacts remain diagnostic external
reference evidence only. See the [upstream model guidance](https://github.com/CPJKU/beat_this#available-models)
and [versioned annotations](https://github.com/CPJKU/beat_this_annotations/tree/v1.0).

## Beat This tuning diagnostic

On 2026-09-18, Beat This `1.1.0` `final0` processed all 122 r5 tuning tracks
on CPU in 1,429.42 seconds. Its initial single-interval BPM export is retained
as raw tick evidence but is not a valid scalar comparator: 50 FPS timestamps
quantize a 140 BPM single interval to 142.86 BPM. Re-deriving BPM from the
same immutable ticks with the documented 16-beat median gives **111/122 strict
BPM matches (90.98%)**, with three half/double errors and no unknowns.

The artifacts are `reference-beat-this-final0-tuning-20260918-v1.json` and
`reference-beat-this-final0-span16-tuning-20260918-v1.json`, plus their local
comparison reports under `sample_media/Test Corpus/`. The latter remains
tuning-only diagnostic evidence, not a production candidate or a basis for an
r5 held-out qualification run because the overlap review found direct training
set matches.

## First reference measurement

On 2026-09-18, `librosa` 0.11.0 ran the full 122-track r5 tuning split through
FFmpeg 2025-01-30, mono 22050 Hz PCM, `hopLength=512`, `startBPM=120`, and
`tightness=100`. It completed without source failures in 157.99 seconds, but
only reached **13/122 strict BPM matches (10.66%)**, with one half/double
error. The corresponding ViiB production replay on the same manifest reached
**98/122 (80.33%)**, with two half/double errors and six unknown BPM results.

The raw local artifacts are
`reference-librosa-tuning-20260918-v1.json`,
`reference-librosa-tuning-comparison-20260918-v1.json`,
`phase0-go-tuning-r5-production-20260918-v1.json`, and
`phase0-go-tuning-r5-production-comparison-20260918-v1.json` under
`sample_media/Test Corpus/`. This establishes librosa's default tracker as a
reproducible control, not a candidate for production adoption. It does not
evaluate or diminish the value of the stronger Essentia or Queen Mary
references, which require separate development-only environments.

## Essentia tuning measurements

On 2026-09-18, Essentia `2.1b6.dev1389` `RhythmExtractor2013` in
`multifeature` mode processed all 122 r5 tuning tracks at 44.1 kHz. The broad
40–208 BPM configuration reached **98/122 strict BPM matches (80.33%)**, with
four half/double errors and no unknowns in 479.92 seconds. Freezing the
production-aligned 90–180 BPM range reached **99/122 (81.15%)**, with three
half/double errors and no unknowns in 461.66 seconds.

These results are tuning evidence only; they do not qualify a production
change. The production-aligned result narrowly exceeds the existing Go tuning
replay's 98/122 strict matches, while the two tools make different errors.
The local artifacts are `reference-essentia-multifeature-default-tuning-20260918-v1.json`,
`reference-essentia-multifeature-90-180-tuning-20260918-v1.json`, and their
corresponding comparison reports under `sample_media/Test Corpus/`.

The 90–180 BPM configuration was then frozen and evaluated once on the
independent 54-track r5 held-out split. It reached **39/54 strict BPM matches
(72.22%)**, with two half/double errors and no unknowns in 199.72 seconds.
This does not qualify it as a production target. The held-out artifacts are
`reference-essentia-multifeature-90-180-heldout-r5-20260918-v1.json` and
`reference-essentia-multifeature-90-180-heldout-r5-comparison-20260918-v1.json`
under `sample_media/Test Corpus/`; they remain local and ignored.

## Rejected native interval candidate

`beat-interval-consistency` is a benchmark-only native method that combines
existing periodicity hypotheses with the consistency of observed onset-peak
intervals. It does not score the generated `BeatGrid`, because that grid is
created only after tempo selection. Its peak search uses a 100 ms refractory
period to bound full-track work, and benchmark results now expose the native
grid ticks for diagnostics beside external reference ticks.

It passes the generated static-tempo and unknown-audio fixtures, but its first
r5 tuning evaluation reached only **64/122 strict BPM matches (52.46%)**, with
one half/double error and six unknowns at 62.58× real time. It is rejected and
will not receive a held-out evaluation or replace the production method. The
ignored local artifacts are `phase0-go-tuning-beat-interval-consistency-20260918-v2.json`
and `phase0-tuning-beat-interval-consistency-comparison-20260918-v2.json`.
