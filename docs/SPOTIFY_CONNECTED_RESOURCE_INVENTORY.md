# Connected resource inventory

> Scope update — 2026-10-12: Provider capture, source/account fencing and regression fixes remain in this PR. Further local scanning/DSP improvements are deferred. This inventory distinguishes fixture/public-contract evidence from live qualification. See [current branch scope](SPOTIFY_BRANCH_REMAINING_WORK.md) and [future local-analysis work](LOCAL_ANALYSIS_FUTURE_BRANCH.md).

The [2026-10-12 report](archive/spotify-metadata-foundation/evidence/spotify-connected-resource-inventory-2026-10-12.json) exercises the production Dev backend with one existing source-linked recording. Explicit refreshes for audio features, detailed analysis and extension-237 waveform all returned HTTP 200 with fresh results. Immediate repeated refreshes returned `fromCache: true` for each resource. This establishes availability and cache reuse for that sample; HTTP request and decoder counts were not instrumented.

The report classifies every observed stored-domain/API schema path without retaining scalar values, recording/song/account identifiers, fingerprints, titles, credentials or audio. It inventories 19 audio-feature paths, 78 detailed-analysis paths and 20 waveform response paths, including containers. No schema traversal was truncated. It covers sanitized stored domains rather than the upstream response before sanitization.

| Field family | Meaning and treatment |
| --- | --- |
| Seven feature scores | Provider scores; retain original values separately from local measurements. |
| Tempo, key/mode, meter, loudness, duration | Source-specific scalar evidence with original dimensions; milliseconds and seconds remain distinct fields. |
| Bars, beats, tatums, sections | Provider timing/section evidence, including confidence; availability does not qualify local meter or downbeats. |
| Segments, pitches, timbre, loudness envelopes | Detailed provider evidence; no local equivalence or model accuracy is implied. |
| Analysis provenance and audio descriptors | Preserve analyzer/context descriptors independently from effective musical scalars. |
| Provider identities, links and audio signatures | Catalog/recording evidence; values are omitted from this report. |
| Waveform bands, geometry and cache metadata | Adapter-native samples and timeline geometry. Band frequency boundaries and perceptual interpretation remain unqualified. |

All 17 observed effective-field selections agree between list and detail for value, units, source, stale state and endpoint where exposed. Nine scalar/readiness projections and five BPM-route scalar/sync projections also agree. The source-bound song waveform returns HTTP 200 and matches the list source fingerprint. Its duration differs from the local duration by 0.037551 seconds, within the existing UI tolerance. This does not qualify mixing consumers, browser rendering, accessibility, source/account races or a broad library rollout.

Partial, malformed, denied, missing and cooldown contracts still require their existing fixture evidence plus appropriate live evidence. The single successful recording is nonrandom, its automatic link is not independently reviewed, and it cannot establish complete provider coverage or independent audio-analysis accuracy. No broad acceptance gate is closed by this report.

## Documented REST artist-album contract

The [official Get Artist's Albums reference](https://developer.spotify.com/documentation/web-api/reference/get-an-artists-albums), reviewed 2026-10-12, specifies `/v1/artists/{id}/albums`, an offset-bearing page of simplified album objects, and membership filters `include_groups` and `market`. This is public contract evidence, separate from the connected one-recording observation above. Synthetic tests use this shape and invented identities; no account payload is incorporated.

REST capture now retains the requested artist as the page parent, with ordered `albums` relations at original offsets, while recursively retaining album/artist objects. Duplicate rows remain distinct positions. Unidentifiable rows remain unavailable album positions; valid objects of a different entity type, missing/null page arrays, missing/mismatched offsets and invalid parent identities are rejected. Original page and relation metadata retain sanitized siblings, including album-group and future fields.

Page keys distinguish offset, limit, group and market using a bounded digest of those known query fields. Arbitrary query parameters do not become catalog keys. The parent page is separate from artist detail and nested artist projections, and does not assert a completed discography traversal. Existing account contexts, publication fences and retention continue to apply. Request replay, SQLite reopen and account retirement are tested with synthetic responses. Live OAuth availability, additional response/error outcomes and complete graph/lifecycle qualification remain open; the active cookie GraphQL artist-discography route is separate.

Final merge review reproduced intermittent catalog loss when concurrent download-resource transactions encountered SQLite write contention. Catalog publication now uses bounded busy/locked retries, with cancellation and account/fence revalidation for every attempt. It does not repeat provider requests, retry authentication/fence failures or change the response body. Repeated fixture and focused race evidence is recorded in [PR closeout](SPOTIFY_BRANCH_CLOSEOUT_NEXT_STEPS.md).
