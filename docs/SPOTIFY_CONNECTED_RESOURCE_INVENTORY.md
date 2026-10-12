# Connected resource inventory

The [2026-10-12 report](spotify-connected-resource-inventory-2026-10-12.json) exercises the production Dev backend with one existing source-linked recording. Explicit refreshes for audio features, detailed analysis and extension-237 waveform all returned HTTP 200 with fresh results. Immediate repeated refreshes returned `fromCache: true` for each resource. This establishes availability and cache reuse for that sample; HTTP request and decoder counts were not instrumented.

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
