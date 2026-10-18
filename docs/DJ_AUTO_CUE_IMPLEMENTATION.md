# Native event-based DJ cues

Implemented on `research/dj-auto-cue-accuracy` from the
[cue accuracy research](DJ_AUTO_CUE_ACCURACY_RESEARCH.md).

## Behavior

Automatic cues now select observed musical changes instead of targeting fixed
percentages of a track. The native analyzer measures broad-band energy and
frequency-balance changes in 50 ms frames during the existing decode pass. It
compares sustained context over two and four seconds on both sides, finds local
novelty peaks and removes nearby duplicates. Only compact event summaries are
stored alongside the existing energy artifact; PCM is not retained.

The opening event requires activity across time, including sparse percussion,
rather than treating leading silence as an intro. Sustained energy rises, falls,
and texture changes become candidates. Silence and isolated short fills can
produce no candidates; steady material may produce only an opening cue. At most
eight distinct events are selected, strongest first with the opening retained,
then assigned chronological pad slots. Unused pads stay empty.

Labels describe measured evidence: **First sound**, **Energy rise**, **Energy
fall**, and **Texture change**. They do not assert vocal entrances, choruses,
drops, or mix-safe phrases without supporting evidence. Native events are still
heuristics; genre-diverse DJ listening validation remains necessary to establish
real-world acceptance and semantic accuracy.

## Timing and preservation

Event timestamps remain in the production decoder's frame-zero timeline. The
artifact retains decoded duration so cue regeneration does not drift or truncate
events when catalog metadata has a different duration.

Generation aligns an event only within the smaller of 200 ms and 0.35 local
beats. Measured/manual downbeats take priority when close enough; other nearby
beats can align timing without claiming verified bar position. Distant anchors
leave the event at its observed time. Stored events remain unsnapped so a later
grid edit can regenerate candidates from the original evidence. Candidate spacing
uses four local beats, bounded to 1–8 seconds; extraction additionally merges
novelty peaks within four seconds.

User cues and locked cues retain their existing protections. Because event-based
selection changes each pad's role, **any existing deletion suppression at a pad
blocks v2 event cues in that pad**, including suppressions created under a
different old kind. This applies to fill-empty, refresh and selected-only modes.
An explicit suppression clear or saving a manual cue restores the pad under the
existing lifecycle. Old v1 cue application keeps its original slot/kind behavior.

## Existing libraries

The energy/structure algorithm is now `energy-structure-v2-native-novelty` and the
cue generator is `auto-cues-v2-events`. The gzip-JSON format stays backward
readable. The feature-version change makes older analysis eligible for repair by
the existing preparation workflow, while unrelated current artifacts are reused.

Run Track Analysis to prepare older tracks. **Fill empty slots** keeps existing
cues, including old generated cues. Use **Refresh generated** in the cue editor
to replace eligible unlocked generated cues after analysis. The `off` and
`suggest` automatic-cue preferences continue to retain artifacts without writing
new hot cues. This change adds no external model runtime or download.

## Verification

Tests cover off-percentage event selection, equal-loudness timbral changes,
gradual energy changes, silence, trailing silence, sparse percussion, transient
rejection, chunk invariance, artifact round trips, bounded snapping, inferred
downbeat honesty, deduplication and ranking, decoded duration, and preservation
of manual/locked/deleted cues. A decoded WAV integration test checks independently
constructed transitions against persisted and regenerated cue timestamps. A
preparation test checks migration from old feature artifacts in one decode.

The synthetic fixtures establish deterministic behavior on known signals; they
do not measure DJ acceptance on real recordings. Learned downbeat/structure
models and a reviewed real-audio cue corpus remain follow-up research from the
original report.

Validation on Windows, 2026-10-17:

```powershell
# From backend; a writable GOCACHE was used in the host temporary directory.
go test ./internal/analysis/... ./internal/api ./internal/db -skip '^TestDownloadRetention'
go vet ./internal/analysis/features ./internal/analysis/cues ./internal/analysis/track ./internal/api ./internal/db
go test ./internal/analysis/features -run '^$' -bench BenchmarkNativeCueFeatures -benchtime=1x -benchmem
```

The test command and vet passed. The unrestricted package run also encountered
nine download-retention failures; a Go build overlay restoring all changed Go
files to the original `HEAD` and excluding the new files reproduced those same
nine failures. The explicit skip above isolates that existing failure group.

The single-iteration feature benchmark on this host's Ryzen 5 3600 took about
104 ms for five minutes of generated 44.1 kHz audio, with 1,281,392 bytes allocated
and 38 allocations. This measures RMS/event extraction on synthetic audio only,
not decoding, tempo/key estimation, database work, or peak process memory.
