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
