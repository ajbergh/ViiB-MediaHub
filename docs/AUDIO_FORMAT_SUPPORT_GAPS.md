# Audio Format Support Gaps: FLAC and M4A vs MP3 and OGG

**Original audit: 2026-10-07 · Updated implementation review: 2026-10-13**

## Summary

FLAC and M4A are accepted by the native library scanner and are included in the backend metadata extractor's documented format coverage. They are **not supported end-to-end at parity with MP3 and OGG**, because the backend DJ/track-analysis decoders have narrower format lists. Browser folder import now admits FLAC and M4A. The DJ browser itself does not filter tracks by extension; playback and client-side waveform generation depend on the WebView/browser being able to decode each file.

## Support matrix

The canonical machine-readable matrix is stored in [`backend/internal/audio/media-capabilities.json`](../backend/internal/audio/media-capabilities.json), embedded by Go and imported directly by [`services/mediaCapabilities.ts`](../services/mediaCapabilities.ts), and exposed at `GET /api/v2/capabilities/media`.

| Area | MP3 (`.mp3`) | OGG Vorbis (`.ogg`, `.oga`) | WAV (`.wav`, `.wave`) | FLAC (`.flac`) | M4A/AAC (`.m4a`, `.aac`) | Opus (`.opus`) | WMA (`.wma`) |
|---|---|---|---|---|---|---|---|
| Native library scan | Supported | Supported | Supported | Supported | Supported | Supported | Supported |
| Native metadata extraction | TagLib + `dhowden/tag` | TagLib + `dhowden/tag` | TagLib + filename | TagLib + `dhowden/tag` | TagLib + `dhowden/tag` | TagLib + `dhowden/tag` | TagLib + filename |
| Browser-only folder import | Supported | Supported | **Unsupported** | **Supported** (`.flac`) | **Supported** (`.m4a`); raw `.aac` excluded | **Unsupported** | **Unsupported** |
| Backend decoder ID | `go-mp3-v0.3.4` | `oggvorbis-v1.0.5` | `wav-pcm16-v1` | None | None | None | None |
| Backend DJ waveform | Supported | Supported | Supported | **Client fallback (`422`)** | **Client fallback (`422`)** | **Client fallback (`422`)** | **Client fallback (`422`)** |
| Client waveform fallback | Supported | Runtime-dependent | Supported | Runtime-dependent | Runtime-dependent | Runtime-dependent | Runtime-dependent |
| Backend track analysis (BPM/key/grid/LUFS) | Supported | Supported | Supported | **Unsupported (`unsupported_codec`)** | **Unsupported (`unsupported_codec`)** | **Unsupported (`unsupported_codec`)** | **Unsupported (`unsupported_codec`)** |
| Playback (`local` / `plex`) | Supported | Runtime-dependent | Supported | Runtime-dependent | Runtime-dependent | Runtime-dependent | Runtime-dependent |

“OGG” in the backend analysis table means Vorbis: `.opus` is not registered as an OGG/Vorbis decoder. Browser decode support is runtime-dependent and is not guaranteed by the extension/MIME mapping alone.

## Findings

1. **Browser-only import admits FLAC/M4A.** Both Settings import paths use the shared capability contract for `.mp3`, `.ogg`, `.oga`, `.flac`, and `.m4a`. Raw ADTS `.aac` remains excluded and has its own capability record. The metadata worker bundles the pinned `music-metadata` dependency instead of importing a parser from a CDN.
2. **Native scan accepts FLAC and M4A.** `backend/internal/scanner/scanner.go` includes `.flac` and `.m4a` in `supportedExtensions`; that common filter is also used by incremental scans/watchers. The scanner delegates extraction to `backend/internal/audio/metadata.go`, whose TagLib-first extractor documents FLAC Vorbis comments and M4A/AAC iTunes metadata, with `dhowden/tag` fallback.
3. **Real browser parser fixtures cover metadata.** [`lib/browserMetadata.test.ts`](../lib/browserMetadata.test.ts) parses original tagged MP3, Vorbis, FLAC, and M4A files and checks tags, duration, and embedded artwork bytes using the same parser as the bundled worker. Backend extraction has format fixtures in `backend/internal/audio/capabilities_test.go`. Runtime playback/decoding qualification remains separate.
4. **Backend DJ waveforms and analysis stop at MP3 and OGG Vorbis.** `backend/internal/analysis/wav.go` registers `.mp3`, `.ogg`, `.oga`, and WAV. The registry is shared by the server-side DJ waveform and track-analysis paths, so FLAC/M4A are marked unsupported there (including unsupported analysis status), rather than getting measured server-side waveform/BPM/key results. Phase 0 analysis benchmarking is also explicitly scoped to MP3/OGG.
5. **The DJ UI does not hide FLAC/M4A, but the fallback is conditional.** `components/dj/DJLibraryBrowser.tsx` lists the library without extension filtering and submits selected tracks to the deck. `hooks/useDJAudioEngine.ts` tries the backend waveform endpoint, then `lib/clientWaveform.ts` downloads and decodes in Web Audio. The playback audio URL is likewise format-agnostic, but successful playback/client waveform generation depends on the actual WebView2/browser decoder; it is not a native FLAC/M4A guarantee. Backend track analysis remains unsupported regardless of client waveform success.

## Remaining qualification

- Qualify playback and client waveform decoding in each supported browser/WebView; raw AAC browser import remains explicitly unsupported.
- Extend native TagLib and browser fixture coverage to additional containers, malformed files, and runtime versions as needed.
- The current policy explicitly discloses unsupported backend FLAC/M4A analysis, while preserving manual grids/scalars and conditional client waveform fallback. Backend decoder additions remain future work.
- Preserve the shared matrix and importer/decoder contract tests when adding formats.

## Code references

- `backend/internal/scanner/scanner.go` — scanned extension allowlist.
- `backend/internal/audio/metadata.go` — metadata extractor and documented formats.
- `pages/Settings.tsx` — browser-only import filters and supported-format label.
- `metadata.ts` / `metadata.test.ts` — browser metadata parser and current test scope.
- `backend/internal/analysis/wav.go` — production decoder registry.
- `backend/internal/api/dj_waveform.go` — server waveform unsupported-codec response and client-fallback contract.
- `hooks/useDJAudioEngine.ts`, `lib/clientWaveform.ts` — DJ waveform fallback and MIME mapping.
- `components/dj/DJLibraryBrowser.tsx` — DJ library listing/load behavior.
