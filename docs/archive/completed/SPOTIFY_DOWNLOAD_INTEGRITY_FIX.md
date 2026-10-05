# Spotify download integrity: boundary-safe copying

> Historical record archived on 2026-10-04. Status, branch names, and validation results below describe the recorded checkpoint; unresolved runtime/quality checks are not closed by archiving. See the [current documentation](../../index.md) and [archive index](../README.md).

## Confirmed source defect

The pinned librespot-go revision `0b9301b09744` implements
`assetReader.Read` by copying bytes from a chunk, advancing its cursor, and
then acquiring the next chunk if the caller's buffer is not full. If that
acquisition fails, it returns `0, err` instead of `bytesRead, err`. Bytes
already copied into the buffer are consequently discarded by the downloader.
An EOF from next-chunk acquisition can therefore turn a truncated candidate
into an apparently successful transfer. A boundary-aligned asset end can also
lead to requesting an unnecessary chunk beyond the actual file data.

The previous 64 KiB read loop did not constrain reads to the asset length or
the transport's 128 KiB chunk boundaries. Logical Ogg offsets are shifted by
the dependency's 167-byte Spotify prefix, so logical 64 KiB reads are not
aligned with transport chunks.

## Correction

`copySpotifyDownload` obtains the logical length using the same SeekEnd API
already used by playback, rewinds, and bounds every read by the remaining
length and the next raw transport chunk boundary. It stops at the advertised
length without a speculative EOF read. It preserves ordinary `(n, err)` reader
results, rejects premature EOF as a retryable integrity error, reports known
total progress, observes cancellation, and rejects zero-progress reads.

Ogg CRC/EOS validation and bounded zero-trailer handling remain in place, both
before and after metadata. Validation errors now include the page number and
byte offset for malformed/truncated pages and checksum failures, without
logging audio payloads or credentials.

## Evidence and limits

Deterministic fixtures reproduce the pinned reader's discarded tail and cover
short final reads, raw chunk boundaries, multiple chunks, a complete Ogg EOS
at a raw chunk-aligned end, premature EOF, cancellation, and no-progress reads.
The full backend suite and Spotify/API vet passed.

This establishes and avoids a concrete truncation defect, not the origin of
every historical integrity failure. No live authenticated download was used
for this correction. The bare reported error did not identify a particular
page/reason; earlier live middle-page CRC failures remain distinct evidence.
Strict validation and existing bounded retries still protect against actual
transport corruption. The pinned dependency's reader used by playback was
not modified by this download-only change.

## Follow-up: progress regression and three-byte tail

The reported `incomplete page header at page 1698 byte 7351878 (3 of 27
bytes)` is a post-transfer validation failure. Existing bounded retries can
restart that transfer, explaining repeated backward progress. The manager
previously ignored the copier's known size, estimated every track as 7 MiB,
and throttled away the last update from 95 to 99 percent. It now uses the
declared length, always emits 99 percent, and explicitly resets progress and
shows an attempt-count retry message without requeuing the owned worker.

Three trailing bytes are not sufficient evidence of harmless padding: the
validator already accepts up to fifteen zero bytes after a CRC-valid EOS page.
The new diagnostic reports whether the preceding page had EOS and whether
the tail was all zeros. Neither checksum checks nor EOS requirements were
relaxed. The exact reported artifact was not available for inspection, so its
tail contents and the cause of the missing EOS/nonzero trailer remain unproven.

API/Spotify tests and vet, both frontend typechecks, and diff checks passed.

## Follow-up: nonzero word padding after complete EOS

The subsequent screenshot reports three distinct recordings with
`preceding EOS=true; zero trailer=false`, each with exactly three bytes left
after all complete Ogg pages passed CRC. Their Ogg end offsets are 7,351,878,
10,660,722 and 8,710,962. In every case, adding the 167-byte Spotify prefix
and three trailer bytes yields a multiple of four. The pinned asset transport
advertises its byte length as a four-byte word count, not an exact Ogg length.
Requiring decrypted transport padding to be zero incorrectly rejects this
observed pattern; it is outside the complete EOS page's audio payload.

Raw Spotify candidates now allow exactly the one to three trailing bytes
needed for that word alignment, regardless of value, only at physical EOF
after checksum-valid EOS. The normalizer verifies the file size equals the
copied length and truncates just that trailer before metadata writing. Existing
bounded zero-padding support is retained and also normalized. Generic/local
and post-tag validators still reject arbitrary nonzero trailers.

No checksum is rewritten, page skipped, or missing EOS accepted. Tests verify
preservation of every page byte, all three padding lengths, and rejection
without mutation for bad CRC, missing EOS, incorrect alignment, four nonzero
bytes, truncated pages, canceled work and mismatched transfer sizes. The
affected live recordings have not been downloaded again in this session.