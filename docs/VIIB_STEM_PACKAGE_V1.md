# ViiB Stem Package v1

**Status:** Initial MediaHub contract implementation  
**Owner boundary:** ViiB-StemLab (or another producer) creates packages; ViiB MediaHub discovers, validates, indexes and plays them. MediaHub does not run source separation.

This document is the v1 interchange contract. A package is a directory ending in `.viibstems`, with one `manifest.json` and the audio files named by that manifest. Readers must validate the package before marking it ready or serving any stem file.

## Compatibility

- `schemaVersion` is a required integer. V1 readers accept exactly `1` and reject every other major schema version.
- Unknown JSON fields are optional extensions and are ignored by v1 readers.
- A producer can list capabilities in `requiredFeatures`. V1 readers reject a package containing any required feature they do not implement.
- A v1 producer must write all fields shown below, including zero-valued timing compensation fields.
- Stem map keys and layout membership are canonical. A `four` package contains exactly `vocals`, `drums`, `bass`, and `other`; a `six` package contains exactly `vocals`, `drums`, `bass`, `guitar`, `piano`, and `other`.

StemLab now defaults to `htdemucs_ft` and writes the `four` layout. MediaHub accepts both layouts without a schema version change. Its standard DJ controls are VOCAL, DRUMS, BASS, and MUSIC: `other` feeds MUSIC for four-stem packages; `guitar + piano + other` feeds MUSIC for legacy six-stem packages. Guitar and piano files are not required for a four-stem package.

## Manifest

Example six-stem manifest (all example digests are placeholders):

```json
{
  "schemaVersion": 1,
  "source": {
    "filename": "Human.wav",
    "sha256": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
    "audioSha256": "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
    "duration": 355.21
  },
  "stemLayout": "six",
  "generator": { "name": "ViiB-StemLab", "version": "1.0.0" },
  "model": { "name": "htdemucs_6s", "version": "1" },
  "audio": { "sampleRate": 44100, "channels": 2, "frames": 15664761 },
  "timing": { "decoderDelayFrames": 0, "startTrimFrames": 0 },
  "stems": {
    "vocals": { "path": "vocals.wav", "sha256": "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc", "sizeBytes": 28212566, "sampleRate": 44100, "channels": 2, "frames": 15664761, "encoding": "pcm_s16le" },
    "drums":  { "path": "drums.wav",  "sha256": "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd", "sizeBytes": 28212566, "sampleRate": 44100, "channels": 2, "frames": 15664761, "encoding": "pcm_s16le" },
    "bass":   { "path": "bass.wav",   "sha256": "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee", "sizeBytes": 28212566, "sampleRate": 44100, "channels": 2, "frames": 15664761, "encoding": "pcm_s16le" },
    "guitar": { "path": "guitar.wav", "sha256": "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "sizeBytes": 28212566, "sampleRate": 44100, "channels": 2, "frames": 15664761, "encoding": "pcm_s16le" },
    "piano":  { "path": "piano.wav",  "sha256": "1111111111111111111111111111111111111111111111111111111111111111", "sizeBytes": 28212566, "sampleRate": 44100, "channels": 2, "frames": 15664761, "encoding": "pcm_s16le" },
    "other":  { "path": "other.wav",  "sha256": "2222222222222222222222222222222222222222222222222222222222222222", "sizeBytes": 28212566, "sampleRate": 44100, "channels": 2, "frames": 15664761, "encoding": "pcm_s16le" }
  }
}
```

A four-stem manifest uses `"stemLayout": "four"` and the four corresponding map entries. Stem files may be in a package subdirectory, provided paths follow the rules below.

### Field definitions

| Field | Requirement and meaning |
| --- | --- |
| `schemaVersion` | Required. Integer `1` for this contract. |
| `requiredFeatures` | Optional list of feature identifiers needed to consume the package. Unknown entries make a package unsupported. |
| `source.filename` | Required basename for display/provenance only. It is never resolved as a path. |
| `source.sha256` | Required SHA-256, lowercase or uppercase hexadecimal, of the complete source file byte sequence. This changes after tag/artwork edits. |
| `source.audioSha256` | Optional SHA-256 of canonical decoded audio samples. Producers must use the algorithm below if they provide it. |
| `source.duration` | Required finite positive duration in seconds. Must agree with `audio.frames / audio.sampleRate` within one sample frame plus 1 ms. |
| `stemLayout` | Required `four` or `six`, exactly matching the stem keys. |
| `generator`, `model` | Required non-empty `name` and `version` provenance strings. They identify production software/model, not a MediaHub dependency. |
| `audio` | Required common PCM output geometry. `channels` is 1 or 2; rate and frames are positive. |
| `timing.decoderDelayFrames` | Required non-negative number of frames of decoder delay compensated by the producer. Zero means no compensation was applied. |
| `timing.startTrimFrames` | Required non-negative number of frames trimmed at the start by the producer. Zero means no trim was applied. |
| `stems.<name>.path` | Required relative `/`-separated path to one audio file. |
| `stems.<name>.sha256` | Required SHA-256 over the complete encoded stem file bytes. |
| `stems.<name>.sizeBytes` | Required positive encoded file size in bytes. |
| `stems.<name>.sampleRate`, `channels`, `frames` | Required decoded PCM geometry; must equal `audio` and the file header/data geometry. |
| `stems.<name>.encoding` | Required `pcm_s16le` or `float32le` for WAV. |

## Source identity and hash rules

`source.sha256` is SHA-256 over every byte of the source file, including tags and embedded artwork. MediaHub must not substitute the catalog's partial file hash for this identity.

`source.audioSha256` is optional in v1. If supplied, it is SHA-256 over canonical decoded PCM in this exact byte representation:

1. Decode the complete source to interleaved signed 32-bit little-endian PCM integers at the package `audio.sampleRate` and `audio.channels`.
2. For each channel sample in frame order, clamp to `[-1, 1]`, multiply by `2147483647`, round to the nearest integer with ties away from zero, and encode as signed 32-bit little-endian.
3. Hash the concatenated bytes. Exclude container metadata, tags, and artwork. Do not dither.

This definition is intended to permit reproducible decoded-audio identity across retagging. A future decoder/canonicalization revision that changes bytes must use a new required feature or schema version rather than silently changing v1 semantics.

MediaHub compares a package with its source using these rules:

- Full-file SHA matches: source identity matches.
- Full-file SHA differs and both available audio SHA values match: keep the package ready and record a metadata-only source change.
- Both identities differ, or the package audio hash differs: mark the package stale.
- If the package omits `audioSha256`, a full-file mismatch is stale.

### MediaHub source-hash coverage

MediaHub computes the v1 audio digest with its backend decoder only when the
decoded source sample rate and channel count exactly match the package audio
geometry. The current decoder registry covers WAV/WAVE PCM16 or float32, MP3,
and Ogg/Vorbis. It does not resample or change channel count for this hash:
there is no v1 resampling filter or channel-map rule to reproduce safely. If
the source decoder is unavailable, decoding is incomplete, or native geometry
does not match, MediaHub cannot compare `audioSha256`; a full-file hash
mismatch therefore remains stale. Plex streams are outside this local-file
hash path. V1 also does not record a source decoder identity, so matching
digests across different decoder implementations for lossy formats are not
guaranteed.

## Audio support in v1

**Default encodings are WAV PCM16 and WAV IEEE float32**, mono or stereo. WAV extensions are `.wav` and `.wave`; the manifest encoding values are `pcm_s16le` and `float32le`. The WAV header format, bit depth, sample rate, channels, block alignment, data length, declared frame count, and checksum must all agree.

FLAC is not allowed by the current v1 MediaHub validator. It may be enabled only after MediaHub ships and qualifies a backend FLAC decoder; that change must update this contract/validator together. MP3 and Ogg/Vorbis are accepted with the `compressed-stems-v1` capability described below. AAC/M4A, Opus, 24-bit PCM and other WAV subtypes remain rejected.

Every stem in one package must have identical rate, channel count, and frame count. Frame zero of each stem must correspond to frame zero of the package's canonical source decode after the declared producer compensation. `audio.frames / audio.sampleRate` is the package duration. No tolerance is allowed between stem frame counts.

## Path and package security

Treat the directory, manifest, and all manifest strings as untrusted input:

- Limit `manifest.json` to 1 MiB and require exactly one JSON value.
- Reject absolute stem paths, drive/UNC paths, backslashes, NUL, empty path segments, `.` and `..` segments. Use `/` as the manifest separator on every platform.
- Resolve each path and reject symlinks whose final target escapes the resolved package root. Only regular files are accepted.
- Allow `.wav`/`.wave` for PCM16/float32, `.mp3` for MP3, and `.ogg` for Vorbis; compressed stems require `compressed-stems-v1`.
- Verify each file's byte size and SHA-256 before the package becomes ready. Inspect its WAV header and frame geometry before indexing it.
- Reject unsupported schema versions, unsupported required features, missing/extra layout stems, malformed hashes, invalid geometry, and unsupported codecs.
- Do not serve paths from a manifest that has not passed validation. A later registry may cache validation by file size/mtime, but explicit revalidation must recompute hashes.

The Go implementation lives in `backend/internal/stems/manifest.go`. It parses a bounded manifest, validates the v1 schema, resolves safe in-package paths, verifies complete-file hashes/sizes, and accepts WAV PCM16/float32 and feature-gated gapless MP3/Vorbis geometry. It does not discover packages, persist registry state, hash source tracks, serve audio, or generate stems; those belong to later roadmap slices.

## Deterministic fixture packages

Tests construct tiny PCM16 WAV files with fixed sample values and derive their manifest checksums from the exact file bytes. These fixtures test format and validation behavior; they are not generated stem examples and make no separation-quality claim.

## Compressed stems capability

WAV remains the default and existing WAV packages remain valid. Compressed-only
packages keep `schemaVersion: 1` and declare `requiredFeatures: ["compressed-stems-v1"]`.
Readers that do not understand this capability reject them rather than attempting playback.

- `.mp3` uses `encoding: "mp3"`; `.ogg` uses `encoding: "vorbis"` (Opus is rejected).
- Every stem in a package has the same encoding and exact decoded geometry.
- Only the selected format is stored at package root; no WAV listening copies or `exports/` directory is written.
- Hashes and sizes cover the encoded files. Readers fully decode and verify frame counts during validation.
- `frames` is the gapless source timeline. Vorbis uses its final Ogg granule; MP3 requires valid Xing/Info frame counts and LAME-compatible delay/end-padding metadata. Untagged MP3 is rejected because its alignment cannot be established.
- MP3 rates are 16,000, 22,050, 24,000, 32,000, 44,100 or 48,000 Hz (MPEG-1/2 Layer III). MPEG-2.5 and free-format MP3 are not supported; choose WAV or Vorbis for lower rates.
- MP3 gapless length is `XingFrames * samplesPerMPEGFrame - encoderDelay - endPadding`. `XingFrames` excludes the Xing/Info frame. The MediaHub go-mp3 path skips that metadata frame and `encoderDelay + 529` synthesis-delay samples; it excludes trailing padding. Require endPadding >= 529. Do not put codec padding into package `timing`: its existing fields still describe producer source compensation, and StemLab writes zero.
- MediaHub decodes compressed stems directly to floating-point frames for the DJ worklet, with bounded preroll on MP3 seeks. It saves no decoded WAV in the stem package. Preview serves the encoded artifact using `audio/mpeg` or `audio/ogg`; browser Vorbis preview support can vary, while DJ playback uses the backend decoder.
- FLAC, AAC and other codecs remain unsupported.

## Shared compressed fixtures

The [codec interoperability fixtures](../fixtures/stems/README.md) are shared with StemLab and exercise mono/stereo files, gapless frame counts, impulse alignment, seeking, corruption, and cancellation.
