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
