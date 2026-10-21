# Browser metadata fixtures

These original one-second, 440 Hz sine waves contain title, artist, album,
album artist, track/disc number, year, genre, and an embedded one-pixel PNG.
They contain no sampled or downloaded music.

Regenerate with `python scripts/generate-browser-metadata-fixtures.py` (requires
FFmpeg). The parser tests execute the same bundled parser as the browser worker
and inspect actual tags, duration, and artwork bytes.

The production-preview browser import E2E also selects this directory through
the standard upload fallback. It checks worker parsing, IndexedDB tags/artwork
and reconstructed artwork after reload; it does not qualify audio output or
native file-handle permissions.
