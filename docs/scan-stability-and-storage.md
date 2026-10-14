# Scan stability and analysis storage

Analysis jobs are now serialized at the atomic database claim boundary. A running
or canceling `analyze_tracks` job blocks other analysis jobs while unrelated job
types remain eligible. Queued follow-ups and restart recovery remain intact.
Per-track leases still protect source changes and publication.

All scanner TagLib calls, including embedded artwork, share a bounded executor.
A parser timeout returns an error, records a file retry delay for metadata, and
keeps its slot until the underlying operation exits. Parser panics are recovered
and logged with a stack trace. Waiting for capacity is bounded to one second;
a full-folder traversal stops safely if capacity is exhausted. Incomplete roots
remain ineligible for destructive reconciliation. TagLib cannot be forcibly
canceled: if every slot stays stuck, restarting the app is required. This bounds
abandoned work but cannot recover a fatal runtime out-of-memory termination.

Whole-library genre statistics are refreshed after the full scan/enrichment,
rather than after every 50-song batch. Direct directory updates use the existing
debounced refresh; API enrichment refreshes once after its workers finish,
including cancellation after partially committed batches.

New local three-band waveforms use a 24-byte binary header and three arrays of
float32 peaks (`local-threeband-f32-le-v1`). Existing JSON artifacts remain
readable and count as prepared, avoiding audio reanalysis. Logical format and
algorithm versions stay unchanged; the encoding field identifies the bytes.
The precision change is limited to rounding waveform peaks to float32.

Successful full and incremental scan boundaries convert up to 128 legacy
artifacts with a two-second maintenance budget. Conversion validates each old
payload and compares the old bytes before updating, preserves source/provenance
and timestamps, and leaves invalid artifacts for the repair path. No audio is
opened for conversion. Maintenance skips oversized legacy payloads.

Conversion frees SQLite pages for reuse; it does not automatically shrink the
physical database file. Existing Spotify caches and amplitude waveforms remain
significant storage consumers. No live database is changed just by checking out
this branch; migration runs when the updated application scans.

Validation covers concurrent analysis claims, cancellation, parser panic and
timeout ownership, full-scan quarantine, binary bounds and round trips, legacy
API/preparation reads, and bounded/idempotent database conversion.
