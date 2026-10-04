# Spotify WebPlayer and Audio Analysis: Feasibility Review and Direct Implementation Plan

## Active objective: full cookie authentication parity

The user now requests all existing OAuth functionality through cookie-based authentication. This supersedes the original first-slice restriction against production routing/migration. The backend now has an encrypted cookie-session runtime and explicit all-purpose routing, connect/status/disconnect endpoints and shared media token injection. The renderer migration, complete account/queue lifecycle and live public API/expiry coverage remain required. Earlier progress entries and review decisions below are historical evidence, not a narrower completion target.

The normal frontend login and catalog/library/playback metadata calls now use backend cookie sessions. Shared login UI, redacted startup/enrichment state and account-media retirement are implemented and fixture-tested. Full frontend checks passed (284 tests and production build); focused backend lifecycle race checks passed. Live public API/expiry, complete app workflows and final audits are still required for parity.

Shared public API cooldowns now survive reconnect and restart. Rejected sessions propagate to the renderer reconnect state, and session generations fence queued work, pending responses, account views and background enrichment. Frontend checks and targeted lifecycle tests passed. Live catalog/library access, sustained playback, natural token expiry and final requirement auditing remain open.

Live Pathfinder research now verifies Web Player client-token acquisition, profileAttributes and findTopResults with HTTP 200 for the consenting account. A public search retry again returned HTTP 429. Complete cookie parity now requires a typed Web Player catalog/library adapter and production routing; successful token substitution alone has not provided working public API browsing. This new evidence changes the next implementation priority.

Production cookie profile routing now uses a runtime-owned typed Pathfinder client with memory-only client-token caching and account retirement. A live check of that production client passed identity/display-name normalization. Email/plan/country/followers remain unavailable and null; other catalog/library routes still require implementation and live verification.

Production search, individual/batch tracks, album, artist/top-track, playlist/content and saved album/playlist adapters are now implemented. Live production-client probes pass explicit renewal, search/category paging, album/playlist paging, artist top tracks and normal 20-item saved-library pages and a full 50-track metadata batch. Recently played remains an open parity gate: the documented route returned 429 in an isolated live check, while the internal recent-context route supplies one last-track reference per listening context and has not proven individual play-event equivalence. Production browser startup, saved-library views, album playback/seek/next, explicit renewal, single-track worker downloads/conversion, encrypted restart and logout/restart now have live evidence. Natural expiry, active-media account changes, grouped downloads/native UI and the remaining profile/market fields are still required. See the current validation record for coverage and limitations.

## Direct catalog parity implementation sequence

The successful Pathfinder probe establishes a separate Web Player protocol, not the app's existing REST response shapes. Implement this sequence while preserving the full parity goal:

1. Add backend/internal/spotify/catalog with fixed, reviewed operation contracts and a bounded HTTP client. Cache the provider-issued client token only in memory using its returned expiry, coalesce renewal, and retire it on account change. Never accept arbitrary GraphQL text, operation hashes or target URLs from the renderer. Keep token values and raw account payloads out of diagnostics.
2. Pin the source revision and license for each operation hash and inspect real response structures. Existing validated operations are profileAttributes and findTopResults. Full category search, album, track/batch-track, artist/top-track, playlist/content, library and recent-listening operations still need source and live evidence. Candidate implementation references are linked in the validation record; their current hashes are leads, not verified contracts.
3. Normalize each operation into the shapes already consumed by SpotifyService, detail pages, download metadata and playback queues. Preserve stable Spotify IDs/URIs, durations, artists, artwork, profile fields, playlist owners, market availability, unavailable entries, ordering, totals and pagination. Handle missing/nullable data explicitly. Generate bounded pagination links that route through the same backend adapter.
4. Route cookie-mode catalog requests to this adapter in the backend resource boundary; retain the OAuth compatibility owner for installations that have not selected cookie mode. Use existing account contexts and session generations so stale operations cannot write into a replacement account. Use persisted catalog cooldowns and one bounded renewal on authentication rejection. Schema/permission errors must not masquerade as expired login; never silently select OAuth.
5. Verify normalized profile and all search categories first, then album/artist/playlist details, full saved library and recent-listening pages, pagination, album/playlist play/shuffle/queue, metadata enrichment and grouped downloads. Use meaningful fixture tests for response conversion and pagination, plus live consenting-account checks. A 200 GraphQL envelope alone cannot prove field or workflow parity.
6. Verify complete application streaming/downloading with these normalized metadata routes, restart/reconnect/account rotation, sustained playback and natural token-expiry recovery. Run frontend/native UI workflows and the final requirement audit before claiming full parity. The original analysis work remains separately capability-gated because detailed analysis returned 404.

## Implementation progress (2026-10-02)

Steps 1–3 now have purpose-separated authentication, a fixture-tested WebPlayer provider, bounded analysis/features clients, and a build-tagged live research probe. The consenting live probe verified authentication and explicit renewal; detailed analysis returned 404, while scalar audio features succeeded for the supplied recording. These are separate capabilities.

The next increment adds Step 4 observation/failure storage and manually confirmed recording identities, plus the cache-only subset of Step 5. Recording links use current analysis-source fingerprints. A further increment implements injected explicit refresh, request coalescing, a global provider slot, persisted cooldowns, cancellation and session write fencing, plus refresh/disconnect routes. Normal application startup still leaves the service uninstalled. Completed-download automatic linkage, production sign-in/profile lifecycle, renderer reference display, retention scheduling, and production permission remain open. Normal startup performs no WebPlayer network work.

An authorized audio compatibility probe now also passed fresh librespot login and a 4096-byte Ogg read with both the initial and explicitly renewed WebPlayer token. This is one account/recording and a short read; production playback/download routing remains OAuth while sustained playback, complete downloads and public API coverage are tested.

A fixed read-only public API matrix is also implemented. Two live attempts authenticated/renewed successfully but stopped on profile HTTP 429 (Retry-After 23 and 39 seconds, with the first delay honored). Search/library/catalog access is therefore still unassessed. Normal OAuth routing remains intact; no automatic live retry was added.

An additional authorized probe completed a full temporary download and MP3 conversion using only cookie-derived authentication. The Ogg fully decoded to 382.4 seconds and the MP3 to 382.380408 seconds; temporary audio and session artifacts were removed. Complete download/conversion is therefore verified for one account/recording on Windows. Public API coverage, elapsed-expiry/reconnect, sustained real-time playback and production session capture remain open; the app still uses OAuth for production playback/downloads.

The research probe now supports independently selected public API groups and provider reconnect. Deterministic fixtures passed automatic near-expiry/expired-token renewal and disconnect/reconstruction. A live search-only run passed reconnect and explicit renewal, then stopped on HTTP 429 (Retry-After 28 seconds); search remains unassessed. This is provider lifecycle evidence, not elapsed-expiry or public API compatibility proof.

See [validation and live evidence](SPOTIFY_WEBPLAYER_VALIDATION.md) for exact implemented routes, tests, and remaining gates. Review-time statements below describe the original baseline; they do not override these subsequent results.


**Reviewed:** 2026-10-02

**Status:** Backend auth/client/research-probe slice implemented; live scalar features verified for one account/track; detailed analysis returned HTTP 404

**Input:** [Original research proposal](SPOTIFY_WEBPLAYER_AUTH_AUDIO_ANALYSIS_PLAN.md)

**Scope:** Review and planning only. This deliverable does not implement authentication or authorize live-account experiments.

Implementation progress and current test evidence are recorded in [Spotify WebPlayer Validation](SPOTIFY_WEBPLAYER_VALIDATION.md). Identity/storage, API/UI, production login, and live compatibility remain later increments.

**Live research correction (2026-10-02):** Authenticated token acquisition and explicit renewal passed. Detailed analysis returned HTTP 404 for two tracks; single and batch feature routes returned HTTP 200 for the supplied track. The explicit feature client retrieved BPM/key successfully. Prioritize scalar features with endpoint provenance; do not promise detailed analysis arrays or confidence. See the validation record for scope and values.

## 1. Feasibility verdict

The dual-provider architecture is technically credible. Backend token isolation, typed remote observations, conservative identity mapping, and optional enrichment fit the current repository. However, the proposal is not yet a production-ready plan: standalone WebPlayer authentication, current private endpoint access, cross-platform login, and permitted use remain unresolved.

Proceed first with a small, disabled-by-default backend integration using synthetic fixtures. Retain OAuth PKCE for public API and playback consumers. Keep local analysis and locked manual values as the effective DJ metadata. Do not make removal of the Developer application requirement an initial delivery target.

| Area | Assessment | Required evidence or change |
| --- | --- | --- |
| Purpose-aware backend authentication | Feasible now | Adapt the existing OAuth refresh boundary without moving frontend PKCE |
| Typed analysis client and normalization | Feasible with fixtures | Validate missing fields, unknown key, ranges, size limits, and error handling |
| WebPlayer session-to-bearer derivation | Conditional | Reproduce the current contract in standalone Go; pin evidence and implementation version |
| Private audio-analysis availability | Unverified for ViiB | A permitted live probe must succeed over token renewal, not just one request |
| Remote observation storage | Feasible | Add separate tables; do not overload local analysis source fields |
| Local/Plex track matching | Missing prerequisite | Persist explicit identity and recording-version evidence |
| Production session capture | Significant unresolved work | Prove one chosen browser approach on each supported desktop OS |
| OAuth replacement | Speculative | Independently prove every consumer, token renewal, account changes, and operational support |
| Spotify benchmarking / DJ use | Material policy conflict | Resolve applicable permissions before live use; technical access does not establish permission |

### Corrections to the original proposal

1. **Private endpoint success is a hypothesis to reproduce.** The original document reports September research and third-party verification, but does not pin exact source revisions, sanitized request traces, account context, or renewal results. A Spicetify extension operating inside Spotify has a different authentication context from a standalone Go application. This review did not obtain a Spotify session or call the private endpoint.
2. **The TOTP specification is incomplete.** The document gives parameter names but no complete algorithm, versioned derivation, test vectors, or reproducible source revision. Do not invent a secret/version or silently download executable provider code. Record and review the exact current contract before implementing the live adapter.
3. **Backend ownership is partial today.** Renderer code also performs OAuth refresh and direct Web API calls. Backend token-purpose work cannot by itself centralize all Spotify authentication. Preserve that arrangement in the first slice and document it explicitly.
4. **403 is not necessarily expired authentication.** An authorization, account, region, policy, or client-context denial must not cause refresh loops. Refresh once after 401; retry 403 only if a verified response classification identifies token expiry. Persistent denial ends the request.
5. **Typed numeric zero values lose information.** A missing key must not decode to C, and missing mode must not decode to minor. Use nullable fields and preserve zero confidence as a real value.
6. **Spotify identity is not persisted on songs.** Download records and frontend Spotify objects have IDs, but scanner-created songs do not. Identity linkage is a separate work item before automatic enrichment of local/Plex media.
7. **Production permission is an early gate.** Spotify's policy expressly addresses benchmarking and prohibits mixing/overlapping Spotify content. Moving a request to a private endpoint does not resolve those issues. Resolve intended metadata, comparator, retention, and DJ use separately before live work or release. This is a feasibility inference from the published policy, not a legal determination. [Spotify Developer Policy](https://developer.spotify.com/policy)

Spotify's November 2024 announcement confirms the restriction of Audio Features and Audio Analysis for affected apps; it does not remove every existing app's access. The public analysis reference now labels the endpoint deprecated. [2024 announcement](https://developer.spotify.com/blog/2024-11-27-changes-to-the-web-api), [analysis reference](https://developer.spotify.com/documentation/web-api/reference/get-audio-analysis)

Development Mode also changed in 2026. The announcement's March 9 update postpones endpoint changes for existing integrations, despite the migration guide's broader timeline; account restrictions remain. Inventory actual app cohort and supported endpoints rather than assuming a uniform migration date. [2026 announcement and update](https://developer.spotify.com/blog/2026-02-06-update-on-developer-access-and-platform-security)

## 2. Verified repository constraints

| Existing location | Behavior | Implementation consequence |
| --- | --- | --- |
| [spotify_token.go](../backend/internal/api/spotify_token.go) | Backend OAuth refresh, mutex, persistence, one retry after 401 | Wrap this behavior first; retain the existing refresh/persistence semantics |
| [spotify.go](../backend/internal/api/spotify.go) | OAuth credentials endpoint and public API/download helpers | Do not add WebPlayer secrets to the renderer-facing OAuth credential shape |
| [api.go](../backend/internal/api/api.go) | Spotify routes and generic settings routes | Register narrow analysis routes; block WebPlayer secret access through generic settings |
| [spotifyService.ts](../services/spotifyService.ts) | Renderer PKCE, token refresh, direct public Web API calls | Include direct frontend consumers in the inventory; preserve current login |
| [App.tsx](../App.tsx), [spotifySlice.ts](../slices/spotifySlice.ts), [store.ts](../store.ts) | Startup restoration, renderer token state, persistence exclusions | Only a redacted WebPlayer status DTO may enter renderer state |
| [session.go](../backend/internal/spotify/session.go) | OAuth bearer supplied to librespot/respot login | Playback retains OAuth; do not alter streaming/download token selection |
| [db.go](../backend/internal/db/db.go) | Song model lacks Spotify ID; sensitive-setting encryption/migration | Add identity linkage and explicitly register any new sensitive setting |
| [crypto.go](../backend/internal/crypto/crypto.go) | Sensitive-key classification and encryption helpers | Audit both storage and read surfaces; encrypted storage alone does not prevent disclosure |
| [track_analysis_schema.go](../backend/internal/db/track_analysis_schema.go), [track_analysis_repository.go](../backend/internal/db/track_analysis_repository.go) | Local analysis sources and manual/measured effective-value resolution | Remote values belong in separate observations |
| [v2_analysis_features.go](../backend/internal/api/v2_analysis_features.go) | Local effective analysis and source fingerprints | Expose remote reference data separately from effective BPM/key |
| [spotify_corpus.go](../backend/internal/analysisbench/spotify_corpus.go) | Conservative CSV/media matching, evidence/license/label-source requirements | Extend provenance explicitly; existing Spotify-labeled CSVs are not proof of private API access |

## 3. Fixed decisions for the first delivery

- Keep the existing OAuth settings format and frontend PKCE flow. Avoid a combined credential-schema migration.
- Introduce three backend purposes: WebAPI, Playback, InternalAnalysis. Route the first two to OAuth and the last only to the optional WebPlayer provider. No automatic token-class fallback.
- Create packages under backend/internal/spotify/auth and backend/internal/spotify/analysis. Inject credential loading and HTTP/time dependencies; neither package imports internal/api. Compose adapters in the API layer to avoid import cycles.
- First implement developer credentials in memory via a dedicated opt-in probe. Do not persist sp_dc until production session capture is selected.
- Add external observations and explicit identity links as separate tables. Preserve current effective-value resolution without change.
- The initial UI, if the live gate passes, offers reference metadata only. Remote beat arrays do not drive sync, cue generation, beat grids, or local waveform timing.
- Exclude ReccoBeats, automatic catalog-wide retrieval, playback migration, public API migration, and OAuth retirement from the initial implementation.
- Following live evidence, support explicit scalar FetchFeatures alongside detailed Fetch. Preserve sourceEndpoint and nullable confidence. Detailed analysis is currently unavailable for the tested recordings; no automatic fallback or app enablement is implied.

## 4. Ordered implementation work

### Step 0 — establish evidence and live-use gates

**Deliverable:** docs/SPOTIFY_WEBPLAYER_VALIDATION.md, created during implementation.

Record a consumer matrix for backend public API helpers/proxy, renderer direct calls, startup refresh, download manager, and streaming session. Each row records purpose, existing credential owner, endpoint category, tests, and manual coverage.

Record the intended use and applicable permission decision for session capture, private API requests, metadata display, storage, benchmarking, and any DJ interaction. Do not assume permission for one use covers the others. If live use cannot be cleared, complete fixture-backed components and continue local analysis; omit the Spotify live adapter from release.

For a permitted probe, capture exact upstream source file/revision, license, TOTP version/derivation, timestamp units, required cookies/headers, token response expiry units, and sanitized result evidence. A repository root URL or an illustrative payload is insufficient. Verify whether a client token or additional client context is required; do not add guessed headers.

**Exit:** consumer matrix complete; live work has a documented go/no-go decision; unknown protocol details remain explicitly unknown.

### Step 1 — introduce the authentication seam

**Files:** new auth/manager.go and auth/token.go; adapt backend/internal/api/spotify_token.go and the existing playback credential handoff.

Define Token(ctx, purpose), Invalidate(purpose, tokenGeneration), and Disconnect(provider). Return token kind, expiry, and generation internally; do not expose bearer values in status DTOs or formatted errors.

Wrap current OAuth refresh through an injected adapter. Keep its five-minute refresh window and one-401 retry behavior. Preserve OAuth credential storage and rotation. Keep frontend refresh logic unchanged in this slice; document its separate ownership.

For WebPlayer, coalesce concurrent refreshes, check cancellation, and use a session generation counter. Disconnect increments the generation and cancels in-flight work so a late token response cannot recreate a disconnected session. Never hold a global lock through unrelated provider operations.

**Exit:** purpose routing is tested; existing OAuth consumers continue working; concurrent refresh produces one token request per generation.

### Step 2 — implement a developer-only WebPlayer provider

**Files:** new auth/webplayer.go, auth/webplayer_totp.go, auth/credentials.go and focused tests; new backend/cmd/spotify-analysis-probe/main.go.

Build this adapter only after Step 0 supplies a reproducible protocol contract. Inject an HTTP client and clock. Retrieve Spotify server time, apply the verified derivation, request the bearer, validate authenticated/non-anonymous response and future expiry, and cache it only in memory. Use a 60-second expiry margin as an initial configurable default. Fail closed on missing or changed provider contract.

The probe requires an explicit feature opt-in and a known track ID. Read session material through a protected local input mechanism, never a command-line argument, checked-in file, normal settings form, or diagnostics output. If environment input is used for a developer experiment, do not print it and remove it after loading. The release application must not enable this credential input path.

Use fixed HTTPS origins. Reject redirects on credential-bearing requests and prevent cookie/header propagation to other hosts. Send session cookies only to the verified token endpoints. Return redacted error categories rather than upstream body dumps.

**Exit:** deterministic protocol/time fixtures pass; concurrent refresh, expired/revoked session, anonymous token, malformed expiry, logout race, timeout, and redaction tests pass. A permitted live probe must show initial token acquisition and successful renewal; otherwise mark the live adapter unproven.

### Step 3 — add the audio-analysis client

**Files:** new analysis/client.go, models.go, normalize.go and tests.

Accept a bare 22-character base62 Spotify track ID. Reject URLs, URIs, episodes, and malformed input at the boundary; syntax alone cannot prove media type, so validate response shape too. Call the fixed audio-analysis endpoint from the original proposal only after the live contract is confirmed.

Use a 20-second overall timeout, request context cancellation, and an initial 8 MiB decompressed response cap. Permit unknown JSON fields for compatibility, require a usable track object, and bound structural array sizes. Do not persist upstream HTML, malformed bodies, or error documents.

Normalize nullable tempo, key, mode, confidences, loudness, time signature, duration, and provider analyzer version. Key -1 means unknown; valid pitch classes are 0–11 and modes 0/1. Preserve zero confidence; reject non-finite values, confidence outside 0–1, and invalid numeric types. Tempo must be positive for a usable BPM; do not impose local DJ BPM heuristics on provider data. Missing tempo/key may produce a valid partial result; absent usable scalar data is unavailable analysis. Reuse the canonical key/Camelot conversion and test all 24 keys.

Validate intervals for finite nonnegative start/duration and track-duration bounds where available. Retain provider timing for reference only: another master, trim, silence offset, or sample-rate conversion can prevent alignment with local bytes.

| Result | Action |
| --- | --- |
| 200 valid | Return normalized observation |
| 401 | Invalidate rejected token and refresh/retry once |
| 403 | End as access_denied unless verified expiry classification permits the one auth retry |
| 404 | Record not_found/unavailable; do not claim syntax proves a supported track |
| 429 | Honor Retry-After; no immediate retry or auth refresh |
| 5xx/network/offline | Return temporarily_unavailable; defer retry to bounded worker policy |
| HTML or incompatible schema | Return provider_changed; preserve no raw sensitive body |
| Timeout/cancel | Stop; no background retry for a canceled request |

**Exit:** fixture tests cover each row, malformed/large payloads, nullable zero values, unknown key, partial results, interval validation, and cancellation. No caller can exceed one auth refresh per request.

### Step 4 — persist recording identity and external observations

**Files:** new db/track_external_identity.go, db/external_track_analysis_schema.go, db/external_track_analysis_repository.go and migration/repository tests; integrate with existing migration conventions.

Create track_external_identity with song ID, provider, external ID, link origin, recording evidence, confirmed-at timestamp, and local source fingerprint. Enforce uniqueness of a song/provider/ID link and one active confirmed Spotify recording link per song. Allow several local songs to reference the same Spotify ID. Invalidate or require reconfirmation when the local source changes.

Initially link only from an explicitly confirmed Spotify ID or an unambiguous completed download record tied to the exact saved file and song. Verify that download-to-file linkage is present; do not infer it from title alone. Treat ISRC/catalog matches as candidates requiring recording-version checks. Store unresolved and ambiguous matches outside the confirmed linkage used for retrieval or comparisons. No automatic title/artist fuzzy matching in the first delivery.

Create external_track_analysis keyed by provider, external track ID, and normalization schema version. Store nullable normalized scalars, all individual confidences, retrieved/expiry timestamps, provider analyzer version, adapter revision, and sanitized payload hash. Keep request failures/cache cooldown in a separate status record so failed refreshes do not replace good observations. Song linkage references provider identity; a remote observation is not a local source fingerprint.

Persist scalars initially. Keep structural arrays in bounded memory for the probe; add versioned artifacts later only if a concrete permitted use requires them. Do not store full token/analysis exchange traces. Make cache retention comply with the Step 0 decision.

Initial configurable engineering defaults: 7-day successful cache, 1-hour unavailable-track cache, at most one in-flight fetch per provider/ID and one concurrent provider request globally. These are scheduling defaults, not claimed Spotify limits or permitted retention periods. Respect longer Retry-After values and never mass-fetch on startup.

**Exit:** migrations are idempotent; clean/existing DB tests pass; observations survive restart; local/manual effective values are identical before and after insertion; wrong/ambiguous recording links cannot trigger retrieval.

### Step 5 — expose narrow APIs and optional reference UI

**Files:** new api/spotify_analysis.go; register in api.go; add service DTOs in services/api.ts; extend the existing track-analysis detail UI after locating its current component.

Proposed routes under the existing API prefix:

| Route | Contract |
| --- | --- |
| GET /spotify/analysis/status | Redacted configured/connected/provider state; no credential values |
| GET /spotify/analysis/{trackID} | Cache-only normalized result with source, age, stale flag, nullable fields |
| POST /spotify/analysis/{trackID}/refresh | Explicit bounded retrieval; obey feature flag and provider cooldown |
| DELETE /spotify/analysis/session | Disconnect optional analysis session and cancel pending work |

Use existing local API authentication/authorization middleware. Do not create a generic private-Spotify proxy. Expose no raw provider JSON. Status states include disabled, authentication_required, available, access_denied, rate_limited, temporarily_unavailable, not_found, and provider_changed. Report normalized cooldown information without secret-bearing URLs/errors.

Show local effective BPM/key as today. If enabled and available, show a separate Spotify reference with provenance, retrieval age, partial/unknown values, and link to the Spotify track. A provider failure does not blank local values. Avoid automatic fallback to another remote provider.

Production credential persistence, if later approved, uses a separate sensitive setting such as spotify_webplayer_session. Register it in crypto.IsSensitiveKey and the DB migration list; explicitly deny reads/writes through generic settings, backup/export, support bundles, and OAuth credential endpoints. Status is a separate safe DTO. Test encrypted bytes and every disclosure surface before enabling persistence.

**Exit:** cache reads perform no remote I/O; feature-disabled startup performs no Spotify work; secrets never enter renderer state; UI partial/offline states work; existing analysis contracts remain compatible.

### Step 6 — implement lifecycle and production login only after the probe passes

Time-box a dedicated-profile browser spike before committing to UX. Test Spotify login, MFA, cancellation, browser availability, native cookie access, reauthentication, and profile cleanup on Windows/macOS/Linux. A generic external browser callback cannot transfer Spotify cookies by itself; do not assume OAuth handoff provides a WebPlayer session.

Prefer one supported isolated browser mechanism if evidence supports it. Do not read users' ordinary browser profiles or passwords. A browser profile is itself credential storage: protect filesystem permissions, exclude it from support bundles, and purge cookies/session storage on disconnect. Encrypting only the copied sp_dc does not protect remaining profile cookies. Define browser lifetime, profile locking, crash recovery, and installation/dependency behavior.

Implement a separate analysis-session disconnect plus an explicit whole-Spotify logout. Whole logout clears OAuth credentials, optional session material/profile cookies, cached tokens, renderer state, and active playback session. Cancel pending fetches and prevent generation-stale writes. Purge account-associated cached observations according to the retention decision. Switching account never reuses the prior session/cache authorization context.

**Exit:** supported-OS evidence exists; login requires no DevTools/cookie copying; backend-only session storage is demonstrated; disconnect remains effective across restart and concurrent requests. If this spike fails, keep the integration developer-only.

### Step 7 — comparator and future compatibility work

Only after permission for benchmarking is resolved, extend analysisbench using explicitly confirmed recording IDs and independently lawful local audio. Preserve corpus evidence class, license, label source, adapter/analyzer versions, cache snapshot hash, and recording-version identity. Freeze fixtures so rerunning a comparison does not depend on live access.

Report strict BPM differences, half/double tempo separately, exact key+mode, relative keys, harmonic neighbors, unknown values, confidence, and missing/ambiguous coverage. Do not call Spotify ground truth or tune the analyzer from unverified matches. Synthetic fixtures can exercise report generation before the live gate.

OAuth retirement is a separate future proposal. If pursued, test the pinned librespot dependency and every backend/frontend public API consumer with explicit permitted scope, account/app cohort, renewal, and failure recovery. One successful spclient request is insufficient. Compatibility alone does not establish a supported or permitted production contract. Do not expand downloads or change playback in this initiative.

## 5. Validation and delivery boundaries

For code implementation, run focused auth/client/storage/API tests after each corresponding change. Tests use httptest, synthetic JSON, and injected clocks; CI never needs a real Spotify cookie. Run race checks for refresh, logout, and cache coalescing. UI tests cover source labeling, missing fields, cooldown, stale cache, and offline behavior.

Final implementation checks:

~~~powershell
npm run check
Set-Location backend
go test ./...
go test -race ./...
~~~

Use a Go race-supported toolchain on each validation host. Record unsupported tooling or environmental failures explicitly. Separately perform a permitted manual smoke test of current OAuth login, startup restoration, browsing/library, streaming, download behavior, and local/manual DJ metadata. Passing unit tests does not establish live compatibility.

During this review, RepoTracer reported passing focused Spotify frontend tests, backend Spotify API tests, and analysisbench/track/db tests. A broader backend test attempt was interrupted and is not a pass. No full check, live token derivation, private endpoint probe, or cross-platform login validation was completed. This task changes documentation only.

Recommended reviewable increments:

1. Authentication seam and fixture-backed client (Steps 0–3).
2. Identity/observation migrations and cache-only API (Steps 4–5).
3. Permitted live adapter evidence and explicit refresh path.
4. Production session capture/lifecycle, only after its own go/no-go evidence.
5. Permitted comparator tooling; OAuth replacement remains separately scoped.

Each increment must leave the optional provider disabled without affecting local analysis or current OAuth consumers. Rollback disables the adapter/UI, cancels workers, disconnects its session, and leaves additive tables inert; avoid destructive rollback migrations.

## 6. First implementation milestone acceptance checklist

- [ ] Existing OAuth WebAPI/Playback behavior is preserved and purpose routing is explicit.
- [ ] Standalone provider protocol evidence is pinned, or the live adapter is clearly unproven and excluded.
- [ ] Fixture tests validate renewal, concurrency, denial/rate limits, payload limits, nullability, and normalization.
- [ ] Session/token material is absent from renderer DTOs, logs, diagnostics, generic settings, and exports.
- [ ] Confirmed recording links and remote observations retain independent provenance.
- [ ] Local/manual effective BPM/key and DJ timing behavior remain unchanged.
- [ ] Optional analysis fails gracefully and performs no work when disabled.
- [ ] Live-use and distribution decisions address the actual policy conflicts.
- [ ] Validation records distinguish fixture success, live evidence, and unresolved experiments.

## 7. Sources and evidence limits

The original proposal and current repository investigation supply code context. The paths above are existing integration points; files named as new work are proposed additions. The original proposal is retained unchanged and its embedded decisions are treated as review material.

Primary external sources checked on 2026-10-02:

- [Spotify Web API changes, November 2024](https://developer.spotify.com/blog/2024-11-27-changes-to-the-web-api): affected app classes and audio endpoint restrictions.
- [Spotify audio-analysis reference](https://developer.spotify.com/documentation/web-api/reference/get-audio-analysis): deprecated public contract and data shape; it does not document the private endpoint.
- [Developer access update, February 2026, including March 9 update](https://developer.spotify.com/blog/2026-02-06-update-on-developer-access-and-platform-security): account restrictions and postponed endpoint migration for existing integrations.
- [February 2026 migration guide](https://developer.spotify.com/documentation/web-api/tutorials/february-2026-migration-guide): endpoint/schema migration inventory; consult the dated announcement for the postponement.
- [Spotify Developer Policy](https://developer.spotify.com/policy): applicable use, benchmarking, mixing, and disconnect restrictions.

Opening the third-party repository references did not establish a pinned current standalone protocol or live endpoint success. Obtain that evidence in Step 0 rather than treating the original research claims as verified implementation facts.

### Production integration update (2026-10-03)

The API now installs the inert reference client/service on connected-cookie startup and successful cookie connection, retires it with the account, and preserves cache across a normal same-account restart. Dedicated services/spotifyReference.ts DTOs and the BPM-editor SpotifyReference panel implement explicit recording confirmation, cache-only reads and explicit endpoint refresh without modifying local analysis. Real production API and browser verification are recorded in SPOTIFY_WEBPLAYER_VALIDATION.md. Detailed 404 remains explicit; no scalar fallback is invented. Exact completed-download linkage now uses durable final-file SHA-256/size/mtime evidence and source fingerprints; queue cleanup/restart, manual-link preservation, explicit unlink and account-cancelled conversion have checks. A retained-artifact normal-app sample passes. HTTP shutdown with four event streams also passes. Comparison/benchmark work and the remaining parity/lifecycle/platform gates remain required.
