# Support bundles

Create a support ZIP in **Settings → System & Logs → Create Support Bundle**.
Review it before sharing. Browser download and native save use the same exporter.

## Schema 2 contents

- `manifest.json`: bundle schema, app/Go versions, platform and creation time.
- `diagnostics.json`: debug-logging state, database/log existence and sizes,
  integration-configured booleans, projected byte count and omission statistics.
- `logs/diagnostics.jsonl`: allowlisted diagnostic records described below.
- `README.txt`: scope and privacy information.

The archive excludes the database, media/artwork, playlists, scan-folder lists,
`scan.log` and the original `viib.log`. Free-form messages, titles, resource IDs,
URLs, paths, prompts, provider bodies, stack traces and raw HTTP request IDs do
not pass through. Unknown formats/fields are omitted rather than partially
redacted. Schema 1 archives used regex-sanitized free text and do not have this
stronger export boundary; review older archives accordingly.

## Retained records

The [projector](../backend/internal/api/support_projection.go) requires a valid
local timestamp and an exact recognized record grammar. It constructs new JSON
objects from typed fields, without copying source messages or arbitrary data.

| Event | Allowed details |
| --- | --- |
| `v2_error` | Server-generated UUID-v4 diagnostic token and HTTP error status 400–599; no route/code/message/details/request ID |
| `audio_read_failed` | Fixed operation/category enums and optional HTTP status 100–599; unknown JSON fields reject the record |
| `spotify_catalog_failed` | Fixed classifier code/stage enums and upstream status (0 means unavailable, otherwise 100–599) |
| `analysis_started` | Version-4 UUID job ID, fixed selection-mode enum and track count; the source algorithm must match the current build |
| `analysis_canceled` | Job ID/mode and processed/total counts |
| `analysis_completed` | Job ID/mode and total/analyzed/skipped/failed counts |

All counts are nonnegative and at most one million. Algorithm strings are not
exported. Job IDs permit grouping these local reports; they do not authenticate
the log or establish a complete job history. Unsupported failure messages and
per-track analysis lines are deliberately absent.

The timestamp is local time without a timezone. Raw request IDs are excluded
because current IDs may contain hostnames or caller-supplied strings. For handler-generated structured v2 errors, a separate server-generated
`diagnosticToken` is returned in the error body and `X-ViiB-Diagnostic-Token`
response header. It is independent of incoming headers and the legacy request
ID. Library Operations displays a validated support reference and copies only
that token. Match it against a `v2_error` record in the archive. Projection
version 2 adds these records; bundle schema remains 2.

This bounded path excludes legacy/middleware/trust-boundary errors and does not
make job IDs into request references. Tail/projection limits can omit a record;
absence from an archive does not invalidate a displayed reference.

## Limits and omissions

The exporter reads at most the final **4 MiB** of the application log, discarding
an incomplete leading line. Projection rejects lines over **8 KiB**, emits at
most **2,000 records / 256 KiB**, and stops accepting records when the record or
output budget is reached. Unknown entries also count as omitted records.

`includedRecords` and `droppedRecords` describe nonempty lines in the read tail,
not the whole log. `projectionVersion` identifies the grammar. The separate
`logWasSizeLimited` and `projectionWasSizeLimited` flags identify input-tail and
line/record/output limits. Missing logs produce an empty JSONL entry. An empty
projection or missing event is not evidence that no failure occurred.

## Remaining privacy and qualification scope

This changes support **export**, not local logging: the on-disk log and in-app
debug log can still contain private details. Do not attach raw logs by default.
New exported event types need a reviewed fixed-field contract and privacy/bounds
regressions. This format trades raw error text and stacks for a smaller set of
safe classifications; complete troubleshooting coverage across legacy/middleware/provider flows
and supported-host save/sharing qualification remain roadmap work.

Run the synthetic archive/projection regressions from `backend`:

```bash
go test ./internal/api -run '^TestSupport' -count=1
```
