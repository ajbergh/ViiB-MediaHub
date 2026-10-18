# DJ cue real-audio validation

Run date: 2026-10-17. This is production-path engineering validation on real
recordings, not an annotated musical-accuracy or listening-acceptance study.

## Selection and provenance

24 local Ogg files were selected from the existing private-local tuning manifest,
round-robin across three collections (eight files each). The source manifest
contains 130 tuning and 60 held-out rows. The runner filters to tuning **before
opening or hashing audio**. No held-out rows were analyzed. The source declaration
is the manifest owner's `private-local`/`lawful-real-audio` declaration; music is
not copied into this branch or redistributed. BPM/key labels are not cue labels.

There are 24 distinct source file hashes and 22 manifest recording groups. Some titles occur in multiple collection
edits/encodings, including One More Time and Clarity, so these are not 24
independent songs. This sample is collection-balanced, not genre-diverse or a
release qualification corpus. Results were not used to tune detector thresholds.

## Results

- All 24 files decoded, produced valid current event artifacts, and generated cues.
- 98.08 minutes of decoded audio; 157 cues, ranging from one to eight per file.
- All 24 passed regeneration after feature and beatgrid serialization. Cue
  positions matched within one microsecond (the grid encoding's precision).
- The test deliberately supplied a wrong catalog duration during regeneration;
  cues still used decoded duration.
- Zero out-of-bounds/ordering failures and zero false verified-downbeat flags.
- Maximum observed event-to-cue snap: 0.186592 seconds (below the 0.2 second cap).

These checks verify file handling, event schema, cue invariants and persistence
consistency. They do **not** determine whether a cue marks the desired chorus,
drop, phrase, or mix-safe moment. No human listening review or independently
annotated cue/section reference was available for this run. Do not report a
musical accuracy or acceptance percentage from these results.

Sparse coverage deserves attention: Bad has only an opening cue; Billie Jean has
two. Remember the Time, Rock with You and Human Nature each have four. These can
reflect the detector's limited evidence on steady/compressed arrangements; the
run does not establish whether the omitted events were unimportant. Review these
first, alongside busy EDM results that fill eight pads, before judging DJ quality.

## Cue review list

Times below are seconds from the production decoder's frame zero. Use the raw
JSON for full precision, feature evidence, source hashes and individual timings.
The CSV has blank review columns intentionally: they are not fabricated labels.

| Track | Cue count | Cue positions (seconds) |
| --- | ---: | --- |
| 100 Most Iconic EDM Songs/001-Avicii-Levels - Radio Edit | 8 | 0.09, 22.94, 53.42, 78.85, 83.65, 91.99, 160.09, 191.52 |
| EDM Hits/007-David Guetta-Titanium (feat. Sia) | 8 | 0.59, 15.05, 46.78, 76.78, 107.25, 122.97, 152.97, 231.06 |
| This Is Michael Jackson/001-Michael Jackson-Billie Jean | 2 | 0.05, 284.45 |
| 100 Most Iconic EDM Songs/002-Darude-Sandstorm - Radio Edit | 8 | 0.44, 14.35, 21.62, 87.36, 93.09, 97.06, 113.10, 127.30 |
| EDM Hits/008-Pitbull-Timber | 8 | 0.20, 7.60, 22.50, 37.98, 81.70, 97.28, 155.66, 170.95 |
| This Is Michael Jackson/004-Michael Jackson-Remember the Time | 4 | 0.27, 73.25, 90.83, 233.05 |
| 100 Most Iconic EDM Songs/003-Alice Deejay-Better Off Alone | 6 | 0.00, 30.11, 57.81, 128.14, 132.25, 154.07 |
| EDM Hits/010-Calvin Harris-Summer | 8 | 0.53, 4.26, 10.33, 17.80, 48.45, 123.33, 154.15, 214.10 |
| This Is Michael Jackson/006-Michael Jackson-Bad - 2012 Remaster | 1 | 0.61 |
| 100 Most Iconic EDM Songs/004-Swedish House Mafia-Don't You Worry Child - Radio Edit | 8 | 0.15, 5.45, 13.35, 66.66, 92.90, 97.15, 174.85, 205.90 |
| EDM Hits/012-OMI-Cheerleader (Felix Jaehn Remix) - Radio Edit | 8 | 0.00, 14.25, 72.55, 113.63, 122.50, 126.65, 145.29, 174.65 |
| This Is Michael Jackson/007-Michael Jackson-Rock with You - Single Version | 4 | 0.21, 43.18, 168.95, 213.49 |
| 100 Most Iconic EDM Songs/005-Daft Punk-One More Time | 8 | 0.05, 30.30, 58.82, 93.20, 141.26, 257.84, 285.65, 313.65 |
| EDM Hits/017-Daft Punk-One More Time | 8 | 0.05, 30.30, 58.82, 93.20, 141.26, 257.84, 285.65, 313.65 |
| This Is Michael Jackson/008-Michael Jackson-They Don't Care About Us | 6 | 3.13, 13.63, 17.63, 23.35, 130.40, 251.13 |
| 100 Most Iconic EDM Songs/006-Skrillex-Scary Monsters and Nice Sprites | 8 | 0.05, 28.05, 41.53, 107.10, 120.39, 185.35, 199.24, 225.81 |
| EDM Hits/019-The Chainsmokers-Roses | 8 | 0.55, 9.95, 48.55, 82.31, 86.54, 122.11, 178.19, 182.41 |
| This Is Michael Jackson/009-Michael Jackson-Human Nature | 4 | 0.31, 186.35, 196.65, 238.37 |
| 100 Most Iconic EDM Songs/007-Zedd-Clarity | 8 | 1.85, 8.69, 39.28, 116.75, 120.69, 151.75, 211.99, 259.04 |
| EDM Hits/021-Flo Rida-Club Can't Handle Me (feat. David Guetta) | 8 | 1.94, 47.23, 61.71, 65.91, 107.00, 114.47, 167.90, 228.40 |
| This Is Michael Jackson/010-Michael Jackson-Smooth Criminal - 2012 Remaster | 5 | 0.89, 14.62, 239.37, 244.45, 248.70 |
| 100 Most Iconic EDM Songs/008-deadmau5-Strobe - Radio Edit | 6 | 0.00, 29.61, 76.20, 82.50, 199.96, 204.00 |
| EDM Hits/022-Zedd-Clarity | 8 | 1.85, 8.69, 39.28, 116.75, 120.69, 151.75, 211.99, 259.04 |
| This Is Michael Jackson/011-Michael Jackson-Heaven Can Wait | 7 | 0.23, 15.55, 52.82, 150.35, 219.80, 270.86, 283.90 |

## Reproduction and artifacts

From `backend`, with access to the same local corpus:

```powershell
go run ./cmd/cuebench -manifest '../sample_media/Test Corpus/phase0-spotify-manifest-r6-import-audit-20260918-v1.json' -out '../docs/DJ_CUE_REAL_AUDIO_RESULTS-new.json' -limit 24
```

The output path must be new. `-limit 0` means all tuning rows, never held-out rows.
The runner uses default production tempo/key/grid/features options and no database.
It writes failed rows and exits unsuccessfully if any invariant fails. Its tests
check tuning-only filtering and collection balancing without opening media.

- [Raw run results](DJ_CUE_REAL_AUDIO_RESULTS.json)
- [Cue review CSV](DJ_CUE_REAL_AUDIO_REVIEW.csv)
- [Implementation and PR validation notes](DJ_AUTO_CUE_IMPLEMENTATION.md)

An earlier eight-track comparison with the percentage generator confirmed that
positions changed, and the older generator always filled eight slots. That
comparison was exploratory and did not have independent cue annotations; it is
not used as evidence of higher musical accuracy here.

## PR regression checks after review fixes

The following passed after the schema validation, suppression-family and atomic
artifact-retirement fixes:

```powershell
go test ./cmd/cuebench ./internal/analysis/... ./internal/api ./internal/db -skip '^TestDownloadRetention'
go vet ./cmd/cuebench ./internal/analysis/features ./internal/analysis/cues ./internal/analysis/localcontract ./internal/analysis/track ./internal/api ./internal/db
```

The nine existing download-retention failures are excluded as documented in the
implementation report; they were previously reproduced on the unchanged base.
Tests cover missing/unknown native schemas, invalid durations/event evidence,
mislabeled-current artifact discovery and repair, cue API conflict responses,
deletion policy across future generator versions, and source-fenced artifact
cleanup with rollback on a late publication failure.
