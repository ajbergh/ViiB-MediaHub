# Frozen Spotify reference comparisons

**Updated:** 2026-10-04. Implementation checkpoint: `8187104`.

Current status: comparator/exporter fixtures pass; a reviewed real-audio comparison remains open. Overall application gates are tracked in the [parity audit](SPOTIFY_COOKIE_AUTH_PARITY_AUDIT.md), with dated proof in the [validation record](SPOTIFY_WEBPLAYER_VALIDATION.md).

`analysisbench` can compare local analyzer output against an offline, normalized Spotify reference snapshot while retaining the ordinary local-label comparison as a separate report. This implements the frozen comparator portion of implementation-plan Step 7. It does not prove real-audio benchmark quality. A read-only cache-to-snapshot exporter is also available and has database/command fixture coverage.

From `backend`:

```powershell
go run ./cmd/analysisbench -manifest corpus.json -results local-results.json -spotify-reference-snapshot references.json -split held_out
```

The command reads JSON only. It makes no Spotify request and does not read, download, decode or modify audio. `comparison` scores the original corpus labels. `spotifyReference` scores the frozen provider values independently and preserves evidence class, license, label source, adapter revision, nullable upstream analyzer version and snapshot hash. Local analyzer identity/configuration remains in each comparison.

A corpus entry needs a `spotifyRecording` object before provider values can be scored:

```json
{
  "recordingId": "AAAAAAAAAAAAAAAAAAAAAA",
  "recordingVersion": "synthetic-recording-v1",
  "audioSha256": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
  "confirmed": true
}
```

These illustrative identities are synthetic. Real entries must come from explicit recording confirmation and the exact local audio fingerprint; filename/title similarity is insufficient. Existing CSV imports omit this binding and therefore cannot silently become identity-confirmed comparisons. The comparator checks agreement between manifest and snapshot. The exporter independently verifies the current source revision and full file SHA-256 against the stored manual confirmation and manifest binding.

The snapshot schema is `analysisbench.SpotifyReferenceSnapshot` (`version: spotify-reference-v1`). Required provenance includes `evidenceClass`, `license`, `labelSource`, `adapterRevision`, nullable `analyzerVersion`, `cacheSnapshotHash`, and `entries`. Each entry contains `trackId`, the same `recording` object, `endpoint`, `schemaVersion`, RFC3339 `retrievedAt`, nullable `bpm`, nullable integer `key`/`mode`, and nullable tempo/key confidence. Key uses pitch classes 0–11; mode is 0 minor or 1 major. Nullable key and mode are preserved independently; key accuracy requires both values, so a partial pair remains unavailable for scoring. Confidence is bounded to [0,1]. No token, cookie, account identifier or raw provider body belongs in this format.

To seal a snapshot, use `SpotifyReferenceSnapshot.Hash()`, assign its return value to `CacheSnapshotHash`, then serialize the snapshot. The hash is lowercase SHA-256 of Go's JSON encoding of the typed snapshot with `cacheSnapshotHash` set to an empty string. Field order follows the struct, entry order is retained, and JSON whitespace is irrelevant after parsing. Changing values or provenance invalidates the hash. The loader rejects unknown fields, trailing JSON, duplicate track IDs, missing provenance, invalid identity/scalars and inputs larger than 16 MiB. This hash detects changed inputs; it is not a signature proving a source's authenticity.

Coverage reports all selected-split positions, matched identities, missing references, unconfirmed local identities, unavailable BPM/key and completely unlabelled references. An identity mismatch or unknown manifest track fails the comparison. Null provider values are excluded from that dimension's label denominator and counted as unavailable; they are never manufactured expected-unknown labels. Missing local detector results remain unknown failures where a provider label exists. A snapshot with no usable labels produces coverage without an accuracy comparison.

The report reuses strict BPM (±0.5), metrical/half-double, exact key/mode, harmonic compatibility, unknown-rate and local confidence-calibration metrics. Relative major/minor matches and same-mode harmonic-neighbor matches are also reported separately. Synthetic fixtures retain `synthetic-ci` and cannot pass the real-audio qualification gate.

## Real-corpus verification workflow

1. Prepare a reviewed manifest with `evidenceClass: lawful-real-audio`, per-track audio/license and label declarations, tuning/held-out split, and confirmed Spotify recording IDs/versions bound to exact full-file SHA-256 values. Synthetic placeholders and filename matches do not qualify.
2. Manually confirm those recordings against the current local sources in ViiB. Automatic download-origin links alone are not eligible for export.
3. Obtain eligible normalized cached observations through the explicit optional reference workflow when the provider is available. Export does not refresh a cache, authenticate or contact Spotify; missing, expired or failed observations remain coverage gaps.
4. Run the read-only exporter below, then the comparator command above with independently produced local analyzer results. Keep the ordinary local-label report separate from the frozen Spotify comparison.
5. Review identity coverage, unavailable dimensions, licensing/provenance, split separation and the actual metrics before claiming real-audio evidence. Retain the unchanged reviewed manifest, frozen snapshot/hash, local results and comparison report for reproduction. A declaration or snapshot hash alone does not prove recording correctness, source authenticity or lawful use.

## Export from the local cache

From `backend`, with an existing identity-bound manifest:

```powershell
go run ./cmd/spotifyreferenceexport -database library.db -manifest corpus.json -out references.json -endpoint audio_features -reference-license "reviewed reference retention declaration" -reference-label-source "normalized cached Spotify scalar observations"
```

`-endpoint` explicitly chooses `audio_features` or `audio_analysis`; the exporter never substitutes the other endpoint. The output is exclusively created with owner read/write permissions and never overwrites a file. Standard output contains aggregate eligibility counts. No provider request, identity inference or database write occurs. An export with no eligible entries returns an error and leaves no output file. Cancellation and a ten-minute bound apply while hashing audio.

The database is opened with SQLite `mode=ro`/`query_only` in a single read transaction. Existing reference schemas must already be installed; no migrations, runtime WAL pragmas or credential initialization run. Corpus paths resolve against the command's working directory; absolute paths are recommended. A normalized path locates a candidate DB row but cannot establish confirmation. Duplicate DB/corpus path candidates are reported ambiguous. Legacy manifest entries without explicit recording confirmation remain unconfirmed.

Each eligible entry needs a current manual DB confirmation matching the manifest recording ID, a current size/mtime source revision, and a freshly recomputed full-file SHA-256 matching the manifest's `audioSha256`. Here SHA-256 covers the complete container bytes, not decoded PCM, `Song.FileHash` or the cheap source token. Same-size/mtime content edits are therefore detected. File identity/size/mtime are checked while hashing and the source revision is checked again afterward. The manifest's explicit `recordingVersion` is retained; neither a timestamp nor a filename becomes a recording version. Automatic download-origin links are excluded until manually confirmed.

Only normalized validated cache values are read. Missing/expired cache rows, future observations and failure statuses at or after the observation time are excluded and counted. Mixed adapter or upstream analyzer versions require separate snapshots and fail before output creation. Adapter revision, nullable analyzer version, schema version, endpoint and retrieval time come from the cached observation rather than command defaults. Reference license/label declarations are explicit command inputs; corpus evidence class and local-audio licensing remain in the unchanged manifest.

Fixture verification covers database byte preservation, rejected row/schema writes, consistent reads during a concurrent writer update, current/hash-bound identity checks, missing/ambiguous/unavailable coverage, cache expiry/failure policy, partial nullable key/mode, provenance conflicts, deterministic hashing, and command output preservation. These fixtures use synthetic container bytes and synthetic scalar cache entries; they do not establish audio decoding or real Spotify accuracy.

Remaining proof: run the export/comparator against an independently reviewed lawful-audio corpus with real confirmed recording versions and current cached observations. No live reference corpus was used for this synthetic verification. Native responsiveness, history/profile parity and the other app gates remain separate.
