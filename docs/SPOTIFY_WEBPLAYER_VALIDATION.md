# Spotify WebPlayer Validation and Research Record

**Record started:** 2026-10-02

**Evidence through:** 2026-10-04

**Implementation checkpoint:** `8187104`

**Branch:** spotify/webplayer-auth-analysis-provider

**Related:** [Implementation plan](SPOTIFY_WEBPLAYER_AUTH_AUDIO_ANALYSIS_IMPLEMENTATION_PLAN.md)

## Current evidence summary

The [parity audit](SPOTIFY_COOKIE_AUTH_PARITY_AUDIT.md) is the current feature/gate checklist. This file retains the dated experiments, including failures and superseded intermediate results. Earlier open-gate statements apply to their own experiment date; the summary below reflects the latest recorded evidence. This documentation pass did not rerun application tests or add live results.

| Area | Latest recorded result | Still unverified |
| --- | --- | --- |
| Sign-in and session lifecycle | Real Spotify-owned credentials login without cookie paste, encrypted same-path restart, logout/reconnect and browser playback through natural token expiry pass. | MFA/SSO variants and other-platform login. |
| Catalog and library | Production adapters and bounded live samples pass; UI traversed all 63 album cards and 229 playlist positions, retaining four unavailable positions. | Universal account/market/content coverage; Recent individual play events and missing profile fields. |
| Audio and native UX | Browser controls and direct-loopback transport fixtures pass; warm HTTP reads improve with asset reuse. Windows native functional controls and latest clean closure pass. | Updated native buffering/search feedback and shutdown while enrichment is active. Browser timing is not WebView timing. |
| Downloads and reference identity | Album/playlist worker samples fully decode; fresh Ogg/MP3 links persist after queue clearing and restart. | Broader quality/mixed/relinked content and repeated multi-track stress. |
| Analysis and benchmark | Scalar reference, local-value preservation, frozen comparator and read-only exporter have separate live/fixture evidence. | Detailed analysis after 404 samples; reviewed real-audio benchmark corpus/results. |
| Credentials and failure paths | Encrypted storage, generic-setting exclusion, support redaction, sanitized new backup copies, bounded proxy failures and shutdown-drain fixtures pass. | Full live generic write-route compatibility is not established. |
| Platforms | Core backend compiles for Windows amd64/arm64, Linux amd64 and macOS amd64/arm64 with CGO disabled. | Native packages and macOS/Linux interactive login/playback/shutdown; race testing unavailable on the recorded Windows toolchain. |

Latest normal-app Recent request: HTTP 429 with Retry-After 27; session status remained connected. No individual-event parity is claimed. Profile email/product/country/followers were absent from the inspected response, not silently dropped by the DTO in that sample.

For detailed follow-ups, see [stream reuse](#stream-asset-reuse-and-byte-ranges-2026-10-04), [shutdown ownership](#2026-10-04-enrichment-and-reference-shutdown-ownership), [credential exports/proxy](#2026-10-04-generic-proxy-and-credential-export-regression-gates), [platform checks](#2026-10-04-cross-platform-core-compilation-and-browser-discovery) and [the final gate recheck](#2026-10-04-remaining-external-gates-rechecked).

Evidence artifacts referenced under `output/playwright` and temporary audio/test roots are local validation output, not committed fixtures or distributable datasets. Their absence from another checkout does not create new passing evidence; use the documented result and a fresh authorized run when reproduction is required. The original account cookie and live tokens are not part of this record.

## Dated validation entries

## First-search responsiveness follow-up — 2026-10-03

Reviewed the pending cookie-auth request path with emphasis on opening the UI, restoring the session, entering Spotify and searching. This increment removes application-owned head-of-line waits; it does not establish live latency or complete OAuth parity.

- Interactive search bypasses the renderer's serial profile/enrichment FIFO. Background enrichment retains its existing spacing.
- Catalog results are published immediately after their response is parsed, before optional first-party playlist scraping completes. The final response still merges unique scraped playlists, preserving the existing fallback behavior.
- Typing debounce is reduced from 500 ms to 150 ms; Enter submits without waiting for debounce. Feedback starts while typing. Search runs only on the Search tab, and changing the input, leaving the tab or unmounting aborts old requests and suppresses late results.
- The profile timeout now aborts actual transport work. Catalog cancellation remains attached through JSON body consumption, not just response headers. Renderer session-generation fencing remains in place.
- Cookie catalog transactions are bounded to four concurrent operations, with at most three non-search transactions to reserve search capacity. Shared persisted Retry-After enforcement is checked before upstream dispatch, including nested hydration and client-token minting. Already-dispatched work may overlap a subsequently received 429; new dispatches are suppressed.
- Retirement hooks are installed once rather than acquiring the account-transition mutex on each request. Token fetch/refresh runs outside the runtime read lock and revalidates the captured account lifetime before accepting success or rejection, allowing logout to cancel cold acquisition.
- OAuth's serialized public-API request/retry path is unchanged. No new endpoint contracts or synthetic history data were introduced.

Validation for this increment:

- PASS: `npm run typecheck` (both TypeScript configurations).
- PASS: `npm test`: 77 files / 325 tests, including seven new Spotify page regressions and four new service/transport latency/cancellation regressions.
- PASS: `npm run build`. Existing Browserslist age, mixed dynamic/static import and large-chunk warnings remain.
- PASS: `go test ./... -count=1 -timeout=240s` from backend.
- PASS: eight targeted admission/cooldown/retirement regressions repeated 25 times.
- PASS: race-enabled API/auth/catalog/refresh package suites, using the existing MSYS2 compiler with its bin directory temporarily added to PATH. The initial attempt without that PATH addition failed at linking; environment settings were restored after both attempts.

Remaining performance evidence: a restored cold session still requires server-time/access-token acquisition and a client token before catalog data; these upstream waits are not removed or claimed instantaneous. Token acquisition coalesces, but cancellation of a flight leader while another consumer needs the same cold token remains a separate review item. Library hydration remains N+1, now isolated from monopolizing search admission. Live cold/warm search timing and native Wails behavior must be measured independently before calling the user-visible performance goal verified. This increment did not access real cookies or live Spotify accounts.

## Live first-search follow-up — 2026-10-03

Built the current production frontend and browser-mode `cmd/viib` executable. Ran two app lifetimes in a unique temporary root with isolated APPDATA, encrypted database, SDK/download paths and logs, using a separate loopback origin. No normal library or settings were changed. The user-authorized session was submitted directly to the real backend connection endpoint without printing its request body. Connection returned HTTP 200 in 322 ms. This tests the backend connection endpoint, not another cookie-form submission or automatic browser login UX.

| Sample | Browser measurement | Interpretation |
|---|---|---|
| First search after profile had completed | Catalog HTTP 780 ms; first result controls observed at 878 ms from fill/Enter automation | Successful four-category catalog response rendered before fallback completion. This is not a cold-token sample. |
| Warm second query | Catalog HTTP 779 ms; dispatch 26 ms after the pre-fill timing marker | No 150 ms debounce wait after Enter. Existing results were already visible, so this sample records transport, not a new-result DOM latency. |
| Restart → Home → Spotify → immediate search | Profile HTTP 560 ms; search dispatched 56 ms after profile started, catalog HTTP 1,381 ms; result controls observed at 1,425 ms from the pre-fill marker | Encrypted session restored with cold in-memory token/client-token caches. Search entered while profile was pending, and results rendered successfully. Shared token acquisition remains part of this duration. |
| Search while saved albums loaded | Saved-album HTTP 4,424 ms; new-query search HTTP 848 ms, dispatched 87 ms after the library request began | Search finished before ongoing 20-item library hydration, demonstrating removal of backend head-of-line blocking in this sample. |

HTTP durations and dispatch timings came from browser Resource Timing, cross-checked against backend request status/timing logs. First-result observations are automation/DOM mutation timings, including fill/Enter scheduling, not high-precision input-to-paint measurements. Playlist scraping took 5,347 ms and 5,632 ms for the first two queries and completed later, updating playlist counts without delaying initial catalog results. Every assessed catalog/profile/library response returned HTTP 200. A superseded query dispatched during the library-to-search switch was cancelled when the input changed; the backend logged HTTP 503/session-unavailable for that cancelled request. The subsequent query and account remained valid; this is not counted as a successful catalog response or an account rejection.

The legacy credentials endpoint returned an empty object and final status remained connected/not-auth-required. The run used no developer app credentials. These few samples support responsiveness in this account/network/browser context, not a latency SLA, OAuth speed comparison, repeated percentile benchmark, complete account/market parity or native Wails proof. The 1.43-second cold result is not literally instantaneous. Natural elapsed expiry, cold-flight leader cancellation and Recently Played parity remain open.

Cleanup: issued the real disconnect endpoint (HTTP 200, connected=false), cleared this test origin's renderer storage, navigated the browser away, and interrupted both app lifetimes. Both logs confirmed HTTP shutdown completion and graceful worker stop; shell exit 1 reflects Ctrl+C. Verified zero validation processes and deleted the exact temporary root, including encrypted session database, executable, caches and logs. No credential was added to repository files or diagnostic reports. Because the credential was supplied in chat, it should still be revoked by the user after testing.

## Implemented first slice

Backend Web API requests through doSpotifyRequest and the download/stream session handoff now request explicit WebAPI/Playback purposes from an OAuth adapter. The adapter retains the existing credential JSON, refresh mutex, five-minute refresh window, rotated-token persistence, and one-401 retry behavior.

New auth and analysis packages have no dependency on internal/api or internal/db. InternalAnalysis requires its own WebPlayer provider; it cannot fall back to OAuth. The normal app composes OAuth only.

The WebPlayer provider keeps cookies and derived tokens in memory, coalesces token acquisition, reuses a replacement when an older rejected token is reported, uses separate client/server TOTP values, rejects anonymous/malformed token responses, and cancels/invalidates acquisition on disconnect. Errors and standard token/provider formatting redact session material.

The optional analysis client has a 20-second overall context deadline, an 8 MiB decompressed body cap, no redirects/cookie jar, one refresh after 401, no refresh after 403, and Retry-After parsing for 429. It validates scalar/interval data, preserves unknown versus zero values, and uses the existing Camelot conversion. It returns separate Spotify observations; it changes no local analysis or effective DJ metadata.

A separate command, backend/cmd/spotify-analysis-probe, requires both the spotify_research build tag and --enable. Normal app builds do not construct this provider, register analysis routes, accept WebPlayer credentials, or contact private analysis endpoints.

An explicit FetchFeatures method now retrieves scalar audio features with sourceEndpoint=audio_features, validates the returned recording ID, and leaves absent confidence values null. Detailed observations carry sourceEndpoint=audio_analysis. No automatic fallback was added.

Storage, identity linkage, cache scheduling, API/UI exposure, durable session lifecycle, and production browser login remain later implementation increments.

## Token consumer inventory

| Consumer | Purpose | Credential owner / refresh behavior | Current handling and coverage |
| --- | --- | --- | --- |
| spotify_token.go: doSpotifyRequest | WebAPI | Backend encrypted OAuth record; backend serialized refresh | Routed through purpose adapter; existing PKCE/401 tests pass |
| spotify.go: search, /me, generic proxy | WebAPI | doSpotifyRequest | Uses explicit adapter transitively; manual live browse coverage pending |
| spotify.go: downloadTrackByID, paginated album/playlist metadata helpers | WebAPI for metadata | doSpotifyRequest | Uses adapter transitively; download behavior itself unchanged |
| spotify.go: downloadAlbumByID, downloadPlaylistByID, downloadPlaylistByScraping | WebAPI for metadata | Caller-supplied OAuth accessToken; direct HTTP clients | Legacy direct consumers retained; no WebPlayer substitution |
| spotify.go: fetchSpotifyTrack | WebAPI | Reads OAuth setting directly; direct HTTP client | Legacy consumer retained; future centralization work |
| download_manager.go: ensureSession | Playback | Backend OAuth adapter then UpdateAccessToken/Initialize | Purpose is now explicit; no change to librespot token class |
| spotify.go: streamSpotifyTrack | Playback | EnsureSession and existing SessionManager | Shares the Playback path; real-account streaming test pending |
| spotifyService.ts: generateAuthUrl/exchangeCode/getAccessToken | OAuth authorization/refresh | Renderer PKCE; persists through backend endpoint | Preserved; frontend tests pass |
| spotifyService.ts: profile, artist/album search, search, recently played, saved albums/playlists, artist top tracks/detail | WebAPI | Renderer OAuth bearer; direct Web API calls | Preserved; backend manager does not own these requests |
| App.tsx / SpotifyCallback / spotifySlice | OAuth startup/callback/state | Restores existing backend OAuth record to renderer memory | Preserved; no WebPlayer secrets added |
| New analysis.Client | InternalAnalysis | Separate backend memory-only provider | Fixture coverage; not composed into normal API/app |
| Research probe | InternalAnalysis | Developer-local in-memory session | Built/tested separately; live requests not run |

Existing OAuth and librespot tests prove regression behavior with fixtures; they do not establish live service availability or authorization.

## Pinned protocol research

The original proposal's broad repository references are now supplemented by a specific source snapshot:

- [stephancill/stupid-social probe at f0f8c219e43d394c84516a3bcc7af7c9fd41f713](https://github.com/stephancill/stupid-social/blob/f0f8c219e43d394c84516a3bcc7af7c9fd41f713/scripts/spotify-web-client.py): source contract used for the research adapter; repository commit dated September 29, 2026. Its license is Apache-2.0 and retained in the probe directory.
- [mirrorfm wrapper at 7f0ac21eb95c0c592394fdb2529d9f6671b8aef4](https://github.com/mirrorfm/spotify-webplayer-token/blob/7f0ac21eb95c0c592394fdb2529d9f6671b8aef4/app/app.go): independent evidence of /api/token and derivation; its mutable secret-download mechanism is not adopted.
- [RFC 6238](https://www.rfc-editor.org/rfc/rfc6238): standard TOTP primitive, verified using its SHA1 vectors reduced to six digits.

The pinned probe uses version 61, a 30-second HMAC-SHA1 code, and provider-owned client material. Its own vector at Unix time 1777993436 produces 031750. ViiB's test reproduces that value.

The source separates /api/server-time from /api/token and supplies local and server codes independently. Its internal audio-analysis path uses bearer authentication and WebPlayer client headers. A separate client-token is used for other surfaces such as Pathfinder; that does not establish a need for it on this analysis route. Source describes behavior; successful live ViiB calls remain unverified.

Older Spotcast code found during research uses /server-time, /get_access_token, and version 5. These divergent examples support keeping the contract pinned and isolated rather than treating parameter names as permanent API guarantees.

ViiB does not execute upstream code or fetch replacement protocol secrets at runtime. Updating the research contract requires an explicit source revision and deterministic vector review.

The upstream supports optional sp_t/sp_key cookies as well; the initial ViiB probe supplies sp_dc only. If live evidence establishes that additional session material is necessary, expand the credential model deliberately.

Differences deliberately adopted by ViiB include failing when server time is unavailable, requiring authenticated token state and future expiry, rejecting redirects, bounding payloads, and not retrying generic 403 denials. These stricter choices may reveal missing live contract details; they are not evidence that ViiB is live-compatible yet.

## Validation results

- PASS: go test ./... from backend. Full backend regression suite completed.
- PASS: focused final auth/analysis/API tests, including purpose routing, unchanged OAuth credentials, nullable fields, all 24 keys, one-401 retry, 403 denial, 429 cooldown, bounded payloads, refresh coalescing, disconnect during acquisition, and credential redaction.
- PASS: go test -tags spotify_research ./cmd/spotify-analysis-probe. Pinned contract vector verified.
- PASS: go vet ./internal/spotify/auth ./internal/spotify/analysis.
- PASS: npm run check. Palette/color checks, type checks, 68 test files / 279 tests, and production build completed. Existing bundle size/dynamic import warnings remain.
- PASS: focused race checks for auth, analysis, and OAuth API regressions. Used CGO_ENABLED=1 with C:/msys64/mingw64/bin/gcc.exe for this command only; default environment remains unchanged. PASS: full go test -race ./... completed with the same compiler configuration.
- Live evidence: the first user-run probe reached analysis after token acquisition and returned an ambiguous not_found. See the user-run result below. Agent-run live requests, account switching, real expiry-duration renewal, streaming/download compatibility, production browser login, and external benchmark retrieval remain untested.

Fixture tests use synthetic credentials and local HTTP servers. For the later authorized live comparison, the user supplied a session cookie. It was passed through a one-use local memory handoff to the network-enabled process, cleared after each run, and was not saved in repository files or diagnostic output. No Spotify password was used.

## Developer-only local probe

The probe exists to collect the missing standalone evidence. An authenticated Spotify web session is needed for live analysis testing. The analysis route's Premium requirement is not established by this research; Premium is relevant to later streaming compatibility tests.

Do not send passwords or session cookies through chat. Normal users should not need cookie-copy instructions; this environment input is a research-only shortcut, not the production login design.

From backend, first verify the contract without credentials:

~~~powershell
go test -tags spotify_research ./cmd/spotify-analysis-probe
~~~

For an authorized local experiment, supply only the session value through masked local input. This avoids storing it in shell history or a credential file:

~~~powershell
$probeSecureSession = Read-Host 'Research session value (sp_dc; keep local)' -AsSecureString
try {
    $env:VIIB_SPOTIFY_SP_DC = [System.Net.NetworkCredential]::new('', $probeSecureSession).Password
    go run -tags spotify_research ./cmd/spotify-analysis-probe --enable --track-id 11dFghVXANMlKmJXsNCbNl
} finally {
    Remove-Item Env:\VIIB_SPOTIFY_SP_DC -ErrorAction SilentlyContinue
    $probeSecureSession = $null
}
~~~

The probe removes the variable from its own process after loading, keeps credentials in memory, and disconnects when done. The parent shell's variable is cleared by finally. This does not promise secure memory erasure or hide the environment from privileged local processes.

Default behavior obtains an authenticated token, explicitly renews it, then requests analysis. Success prints the source revision, renewalVerified, and normalized metadata. The field confirms the explicit renewal request succeeded; it does not prove renewal after a full expiry window. Failure prints a redacted JSON report with the failing stage, authentication/renewal milestones, error category, and HTTP status when available. HTTP 404 is not_found; an HTTP 200 response without usable tempo/key is analysis_unavailable. Do not add HTTP trace logging or share raw headers.

Record only date, source revision, account tier (not identifier), region if relevant, success/error category, HTTP status, renewal outcome, and scalar availability. No benchmark bulk retrieval or playback test is performed by this command.

## Remaining evidence and next implementation work

1. Run the local probe against a consenting account and record sanitized outcomes. Follow up with an elapsed-expiry test. Fix protocol differences only from measured, reviewed evidence.
2. Before live benchmarking or product enablement, resolve intended-use permission and retention policy. The published [Spotify Developer Policy](https://developer.spotify.com/policy) identifies benchmarking and mixing restrictions; a private endpoint does not resolve them.
3. Implement confirmed recording identity and separate observation tables, with migration tests proving local/manual effective values remain unchanged.
4. Add cache-only/status APIs and explicit retrieval scheduling behind a feature flag. No catalog-wide automatic fetch.
5. Spike production browser session capture across supported operating systems. Persist no session until generic settings/export/support-bundle disclosure surfaces are covered.
6. Treat playback/public API token replacement as separate future work. Retain OAuth until its complete consumer matrix and permitted-use requirements are resolved.

The first slice is reviewable and usable through fixtures today. Live availability, permission, and production onboarding remain explicit open results.

## First user-run result (2026-10-02)

The user ran the initial probe for track 11dFghVXANMlKmJXsNCbNl and received spotify analysis: not_found. This implies initial authenticated token acquisition passed under the provider validation, but the original diagnostic did not distinguish HTTP 404 from an empty usable analysis payload and stopped before renewal. It does not prove the analysis route is available.

The updated probe independently records authentication and explicit renewal before analysis, preserves HTTP status on payload-validation failures, and separates analysis_unavailable from not_found. A repeat local run is needed; no additional account credentials are requested in chat.

Diagnostic follow-up checks passed: go test ./internal/spotify/analysis and go test -tags spotify_research ./cmd/spotify-analysis-probe, including 404 versus HTTP-200-without-analysis classification and redaction of unexpected errors.

## Second user-run result (2026-10-02)

The updated probe returned stage=audio_analysis, authenticationVerified=true, renewalVerified=true, code=not_found, httpStatus=404 for 11dFghVXANMlKmJXsNCbNl. Initial authenticated token acquisition and explicit renewal therefore passed; the private analysis route did not return data for this ID/account context. A 404 cannot distinguish missing per-track analysis from route/context availability.

The sample ID is still present in Spotify public metadata ([track page](https://open.spotify.com/track/11dFghVXANMlKmJXsNCbNl), [Get Track documentation](https://developer.spotify.com/documentation/web-api/reference/get-track)); this does not prove playability or analysis availability in the user account. Next test: a song the user can currently play, using its Share > Copy Song Link. The probe now accepts that link and validates it locally before authentication. It sends only the extracted ID to the fixed analysis endpoint.

Additional pinned evidence: [L3-N0X metadata client at d8220d2e48d5d28aa3cb19f20577dec4ac6066d5](https://github.com/L3-N0X/spicetify-dj-info/blob/d8220d2e48d5d28aa3cb19f20577dec4ac6066d5/src/api/metadata.mjs) uses audio-features batch/per-ID routes with an in-client token. It does not verify the analysis route for a standalone ViiB WebPlayer session; no automatic endpoint fallback was added.

## Authorized live comparison (2026-10-02)

Both the original sample track and user-supplied track 5r9W9MJLvHk83fcZSPQ8SE returned HTTP 404 from the detailed analysis route, after successful authenticated token acquisition and explicit renewal. A sandboxed runner first failed before authentication because network access was unavailable; that result was excluded from service compatibility evidence. The network-enabled runner produced the results below.

| Route for supplied track | HTTP result | Observed fields |
| --- | --- | --- |
| audio-analysis/{id} | 404 | No analysis track/tempo/key payload |
| audio-features/{id}?format=json | 200 | Tempo and key present |
| audio-features?ids={id} | 200 | Tempo and key present |

The explicit normalized feature probe then succeeded for this recording, with successful renewal: BPM 108.022, Spotify key 6, mode 1 (F-sharp major), Camelot 2B, loudness -13.212 dB, time signature 4, duration 382.4 seconds. Confidence fields were absent and remain null. No beat/bar/segment arrays or analyzer version were invented.

Implementation now provides --features for normalized scalar retrieval and --compare-features for bounded fixed-route availability diagnostics. These remain research-only and do not enable an application fallback. Track links are accepted only after validation and conversion to a bare ID; requests always use fixed Spotify origins.

This is evidence for one account/session and one feature recording, not general account/market coverage or production permission. Immediate engineering priority is explicit feature enrichment with provenance; detailed analysis must remain a separately unavailable capability until additional evidence resolves the 404s.

Follow-up tests verify that analysis 404 does not trigger feature fallback, audio_features provenance is retained, recording-ID/type/range mismatches are rejected, and absent confidence is not fabricated.

## Recording identity and cache increment (2026-10-02)

Implemented additive SQLite storage installed by the normal database migration. One confirmed Spotify recording is allowed per local song; multiple local songs may reference the same recording. Confirmation requires an explicit true flag and the current local/Plex analysis source fingerprint. An unavailable or changed source yields no active link. Song deletion cascades its link; globally recording-keyed observations are independent of local files.

Only manual confirmation is enabled. Automatic completed-download linking remains pending stronger evidence binding the completed output to the current file revision. Historical path equality alone must not confirm a replacement file. No title/artist matching is implemented.

Observations are keyed by Spotify recording, endpoint, and schema version. The cache stores bounded normalized scalar JSON, nulls, retrieval/expiry timestamps, adapter revision, and a SHA-256 hash of normalized bytes (not raw provider JSON). Successful observations and failure/cooldown records are separate. Older results cannot overwrite newer entries. Failure writes retain good data; cache reads suppress failure state older than the successful observation. No raw response, session cookie, or token is stored.

Implemented routes:

| Route | Behavior |
| --- | --- |
| GET /api/spotify/analysis/status | Reports disabled, unconfigured, disconnected, cache-only |
| GET /api/spotify/analysis/{trackID}?endpoint=audio_features | Reads normalized cached scalars, age, stale flag and newer failure; no remote I/O |
| GET /api/spotify/analysis/{trackID}?endpoint=audio_analysis | Reads the separate detailed-analysis cache; no feature fallback |
| GET /api/v2/analysis/{songID}/external/spotify | Returns source fingerprint and currently valid confirmed link, or null |
| PUT /api/v2/analysis/{songID}/external/spotify | Requires trackId, sourceFingerprint and confirmed=true; rejects stale/unavailable source with 409 |
| DELETE /api/v2/analysis/{songID}/external/spotify | Removes that song's recording link |

The default cache endpoint is explicitly audio_features, matching the measured working scalar route. At this storage milestone no refresh/session endpoints or renderer integration were present; the following section records subsequent additions. Missing cached data is not_cached, distinct from a measured remote not_found. Disabled status does not prevent reading already persisted reference data.

TTL is supplied explicitly by the storage caller; storage does not schedule retrieval or silently authorize a retention period. An explicit repository purge clears remote observations and failure state while preserving local analysis, manual overrides and recording links. Expired entries remain marked stale until explicitly purged/replaced; production expiry/retention scheduling and account authorization context remain pending before live application writes.

Tests cover schema idempotence, reopening the DB, null versus zero confidence, endpoint isolation, failures retaining good observations, older writes/cooldowns, invalid scalars/provenance, explicit source confirmation, changed/unavailable audio, duplicate local copies, deletion cascade, purge isolation, and cache reads with no provider configured.

Planned after this increment (implemented below): a bounded explicit refresh service with injected provider, one in-flight operation per recording/endpoint, global provider concurrency and cooldown, account/session-generation write guards, and cancellation. Keep normal application retrieval disabled until session onboarding and permitted retention/use are resolved. No Premium credentials are needed for fixture-backed storage/service work.

Validation for this increment passed: go test ./..., go test -tags spotify_research ./cmd/spotify-analysis-probe, go vet ./internal/db ./internal/api ./internal/spotify/analysis, focused race tests for the new DB/API lifecycle cases, the full Spotify analysis package under race, and git diff --check. Race checks used CGO_ENABLED=1 with the installed MinGW GCC. No frontend source changed; the previously recorded npm run check result was not rerun for this backend increment.

## Explicit refresh and session lifecycle increment (2026-10-02)

Added internal/spotify/refresh as an injected orchestration service. Constructing it performs no remote work. API.New leaves it uninstalled; no environment variable, generic setting, cookie, or WebPlayer contract is loaded by normal startup. The explicit composition seam is InstallSpotifyAnalysisService, using a separately constructed provider and the API's database. The installer purges prior account reference data before publishing the service; callers own services that fail installation.

Implemented API additions:

| Route | Behavior |
| --- | --- |
| POST /api/spotify/analysis/{trackID}/refresh?endpoint=audio_features | Explicit scalar retrieval through an installed service; otherwise 503 disabled |
| POST /api/spotify/analysis/{trackID}/refresh?endpoint=audio_analysis | Explicit detailed retrieval, with no features fallback |
| DELETE /api/spotify/analysis/session | Cancels/fences the service, invokes its provider cleanup callback, and purges reference observations and failures |
| GET /api/spotify/analysis/status | Reports redacted injected-service state; still disabled in normal application startup |

Refresh honors a fresh successful cache instead of forcing another request. Duplicate requests for a recording/endpoint share one flight. Canceling a caller cancels only its wait; the last departing waiter cancels the operation. At most 32 distinct flights are pending per service, with a 20-second bound including queue time. A single provider slot is shared across service/session lifetimes, so a retiring request cannot overlap a new session's provider request.

Cooldown is rechecked after obtaining that slot. Rate limiting applies across recordings and endpoints, and the maximum persisted rate-limit retry timestamp is read after restart. Retry-After can extend the default one-minute backoff. Unavailable/not_found results use a configurable one-hour default; successful observations use a configurable seven-day default. These are engineering defaults for fixtures, not permission for production retention. No timer, startup fetch, background retry, force option, or bulk endpoint is introduced.

The service validates returned recording identity, endpoint, source, timestamp and normalized scalar values before storage. It holds its lifecycle mutex through each database write; disconnect cannot race past a completed generation check into a late commit. Canceled/obsolete operations write neither observations nor failure state. Safe failure codes preserve existing good data; arbitrary upstream error strings never reach storage or the HTTP response. Disabled adapters do not write failure cache.

API failures return structured safe codes/results and normalized Retry-After; a good prior observation can remain in the response beside a failed refresh. API shutdown retires the service and invokes session cleanup. Explicit session disconnect also purges reference data while preserving recording links and local/manual analysis. Shutdown preserves completed reference cache; any later explicit installation purges it before a new session can fetch. The owning production composition must supply the in-memory provider disconnect callback; browser/profile cleanup is still future work.

Fixture and actual-router tests cover inert construction/default startup, explicit POST, fresh cache reuse, coalescing, independent/last-waiter cancellation, queue limits, global serialization across session replacement, no endpoint fallback, persisted cooldown and queued requests, cooldown expiry, negative caching, invalid observations, safe error redaction, disconnect fencing, cache purge, and shutdown rejection of new installation.

Production session capture, encrypted credential/profile persistence, permitted-use/retention decisions, elapsed-expiry live validation and renderer reference UI remain open. No live credentials were used for this increment.

Validation passed: full backend go test ./..., research-tag probe tests, go vet for refresh/API/DB, and git diff --check. Final focused race checks passed for the refresh service, storage cases and registered API lifecycle routes after the disabled-adapter/index additions. No frontend source changed and no live Spotify request was needed.

## WebPlayer-token audio compatibility result (2026-10-02)

Implemented --stream-read-bytes 4096 (allowed range 4096..16384) in the research-only probe. It is mutually exclusive with --features and --compare-features, disabled by default, and restricted to one validated track. Production OAuth routing and application streaming/download behavior remain unchanged.

A parent process runs an isolated worker with a 75-second watchdog, because the pinned librespot metadata/audio-key APIs do not accept a context. Worker stdout/stderr are discarded at the process boundary rather than captured. Only safe milestone JSON is written into a temporary directory; no cookie, bearer, raw upstream error, or audio bytes are written there. Parent cleanup removes this directory on success/failure/timeout.

The worker uses the existing SessionManager and Streamer at low quality (OGG_VORBIS_96 only), reads a bounded sample into memory, checks the OggS header, closes the stream/session, explicitly renews the WebPlayer token, and repeats using a freshly created session even if the returned bearer is unchanged. The application read is bounded; the dependency may request larger chunks or prefetch internally, so the byte count is not a network-transfer cap.

The authorized live run used the previously supplied session and recording 5r9W9MJLvHk83fcZSPQ8SE:

| Milestone | Initial token | After explicit renewal |
| --- | --- | --- |
| Authenticated WebPlayer token | Passed | Passed |
| Fresh librespot session login | Passed | Passed |
| Stream creation | Passed | Passed |
| Application audio bytes read | 4096 | 4096 |
| OggS header | Verified | Verified |

The pinned dependency is librespot-go v0.0.0-20251013184957-0b9301b09744 with the existing amp.SDK replacement. This demonstrates session and short audio retrieval compatibility for this account/recording without a developer-app OAuth token. It is stronger than token issuance alone. It does not establish complete playable decoding, sustained playback, full-file download/transcoding, higher qualities, elapsed-expiry recovery, or other accounts/markets.

Cleanup verification found no remaining probe executable or probe cache/report directories; the one-use local credential handoff was closed and cleared. Only the redacted outcome above is retained.

Example local rerun from backend (keep the session value local):

~~~powershell
$probeSecureSession = Read-Host 'Paste sp_dc (hidden)' -AsSecureString
try {
    $env:VIIB_SPOTIFY_SP_DC = [System.Net.NetworkCredential]::new('', $probeSecureSession).Password
    go run -tags spotify_research ./cmd/spotify-analysis-probe --enable --track-id 5r9W9MJLvHk83fcZSPQ8SE --stream-read-bytes 4096
} finally {
    Remove-Item Env:\VIIB_SPOTIFY_SP_DC -ErrorAction SilentlyContinue
    $probeSecureSession = $null
}
~~~

Fixture tests cover byte bounds, incompatible flags, login/stream/read distinction, Ogg header rejection, reader cleanup, milestone persistence, unsafe output rejection, raw dependency output suppression, and forced termination of a hung child. Research probe tests, race checks and vet passed.

Next decisive evidence: bounded read-only search/profile/library endpoint coverage using this token; then elapsed-expiry/reconnect and sustained playback checks. Full downloads/transcoding need their own test. Only after that matrix passes should the application adopt a single WebPlayer login. Production browser/session capture and permitted distribution/use remain separate gates.

## Public Web API compatibility probe (2026-10-02)

Added --web-api as a mutually exclusive, research-only mode. It obtains an authenticated WebPlayer bearer, performs the existing optional explicit renewal, then checks a fixed read-only matrix. The current live runs use the renewed token; they do not compare each route before and after renewal.

The matrix contains at most eleven GET requests: profile, supplied track, fixed four-category search, saved albums, saved playlists, recently played, and dependent album, artist, artist top tracks, playlist metadata and playlist-track checks. Dependent IDs come only from validated response objects and must be bare 22-character IDs; there is no arbitrary URL/path/query option. Artist top tracks requires a validated profile market. Missing dependencies are reported as skipped, never searched or guessed.

Paged search/library/playlist-track requests use limit=1. Playlist metadata requests select fields without expanding its tracks. Album details and artist top tracks can contain provider-bounded arrays within the one-megabyte response cap; they are not paginated. Redirects are rejected, no cookie jar is used, and only the bearer is sent to fixed api.spotify.com GET routes. Each request is bounded to ten seconds and the matrix to sixty seconds. It never follows next links.

Only route labels, HTTP status, compatibility/classification, constant missing-field labels and normalized Retry-After are output. IDs, country, email, names, library contents, bodies, headers, cookies and bearer values stay out of diagnostics/storage. Shape-compatible is null when a response was not assessed, rather than falsely reporting incompatibility on throttling. Coverage distinguishes sampled-route compatibility, partial coverage, rejected token, incomplete/incompatible responses, and inconclusive rate limiting.

The probe checks the application's typed profile fields, usable track identity/name/duration/artists/album, result-page envelopes and representative items, dependent catalog object identity, and playlist track wrappers. It accepts genuinely empty libraries and skips null/local playlist entries in the same manner as existing consumers. It does not cover mutation/player-control APIs, full pagination, every optional UI field, or all account/market cohorts. Frontend direct OAuth ownership remains an additional migration task even if read compatibility eventually passes.

Authorized live outcomes:

| Attempt | WebPlayer authentication / renewal | First route | Result |
| --- | --- | --- | --- |
| Initial | Both passed | profile (/v1/me) | HTTP 429, Retry-After 23 seconds |
| One delayed follow-up | Both passed | profile (/v1/me) | HTTP 429, Retry-After 39 seconds |

The follow-up waited beyond the first delay. Both matrices stopped immediately at the rate limit; search, library and catalog compatibility were not assessed. No conclusion about token rejection, scope availability, or successful public API access can be drawn from these outcomes. Do not replace OAuth based on this result or keep retrying the matrix automatically.

The session handoffs were closed/cleared, parent environment variables cleared, and both temporary executables removed. No new credentials were requested and no raw account/catalog data retained.

Example later local run from backend, after service cooldown:

~~~powershell
$probeSecureSession = Read-Host 'Paste sp_dc (hidden)' -AsSecureString
try {
    $env:VIIB_SPOTIFY_SP_DC = [System.Net.NetworkCredential]::new('', $probeSecureSession).Password
    go run -tags spotify_research ./cmd/spotify-analysis-probe --enable --track-id 5r9W9MJLvHk83fcZSPQ8SE --web-api
} finally {
    Remove-Item Env:\VIIB_SPOTIFY_SP_DC -ErrorAction SilentlyContinue
    $probeSecureSession = $null
}
~~~

Synthetic tests cover fixed origin/read-only requests, cookie isolation, bounded page size/request count, no pagination following, redaction, denial versus missing objects/schema/market results, empty libraries, missing dependencies, redirect blocking, token kind/mode guards, payload bounds, and immediate stop on 401/429. Research probe tests, race checks and vet passed. Final reporting refinements were rechecked without further live requests.

Current conclusion: short audio works with the WebPlayer token for the tested account/recording; public API coverage is inconclusive due to rate limiting. A later bounded matrix attempt, elapsed-expiry/sustained audio tests, and complete download/transcoding validation remain before application login unification.

## Complete download and MP3 conversion (2026-10-02)

Added `--download` to the build-tagged research probe. It is opt-in, accepts one validated recording, and is mutually exclusive with feature, Web API and short-stream modes. A parent process enforces a 180-second watchdog and removes the worker's temporary audio, reports and session cache on success, failure or timeout. Worker output is discarded; only validated, redacted milestone results are returned. Files are confined to the temporary directory, limited to 128 MiB each, and decoded audio is limited to 30 minutes.

The worker authenticates using the cookie-derived WebPlayer bearer, explicitly renews it by default, then uses the existing SessionManager, Downloader and pure-Go Ogg-to-MP3 converter. Neutral labels and nil optional metadata avoid artwork and public Web API requests. The downloader requests its existing highest Ogg quality with fallback; this probe does not independently establish which quality Spotify supplied.

The authorized live test for recording `5r9W9MJLvHk83fcZSPQ8SE` passed without a developer-app OAuth token:

| Verification | Downloaded Ogg | Converted MP3 |
| --- | --- | --- |
| File size | 13,666,762 bytes | 15,296,337 bytes |
| Decoded sample rate | 44,100 Hz | 44,100 Hz |
| Decoded channels | 2 | 2 |
| Decoded frames | 16,863,840 | 16,862,976 |
| Decoded duration | 382.4 seconds | 382.380408 seconds |

The Ogg decoded through completion and matched all 16,863,840 declared frames. The MP3 also decoded through completion, with a duration difference of approximately 19.6 ms. Conversion removed the source Ogg as expected. The probe permits at most a one-second duration difference and rejects malformed, incomplete, oversized or escaped files.

Authentication and explicit renewal both passed before the single complete download. This establishes full-file retrieval and conversion for this account/recording on Windows; it does not establish elapsed-expiry recovery, continuous real-time playback, every quality, other accounts/markets or cross-platform session capture. Earlier short-stream tests separately exercised fresh sessions before and after explicit renewal.

Cleanup verification found no remaining probe executable or temporary probe directories. The one-use credential handoff was closed and cleared, and the shell environment variable was removed. No credential or audio artifact is retained in the repository.

Example local rerun from backend:

~~~powershell
$probeSecureSession = Read-Host 'Paste sp_dc (hidden)' -AsSecureString
try {
    $env:VIIB_SPOTIFY_SP_DC = [System.Net.NetworkCredential]::new('', $probeSecureSession).Password
    go run -tags spotify_research ./cmd/spotify-analysis-probe --enable --track-id 5r9W9MJLvHk83fcZSPQ8SE --download
} finally {
    Remove-Item Env:\VIIB_SPOTIFY_SP_DC -ErrorAction SilentlyContinue
    $probeSecureSession = $null
}
~~~

Fixture tests cover pipeline milestones and stage failures, failed-conversion source preservation, file confinement and size limits, invalid PCM geometry/durations, corrupt inputs and cancellation. Validation passed: research-tag tests for the probe, audio and Spotify packages; probe race tests; probe vet; and git diff --check. These checks supplement the successful live full-decoding test.

Current conclusion: scalar features, short audio retrieval, and complete download/MP3 conversion work with cookie-derived authentication for the tested recording. Public Web API browsing/library coverage remains inconclusive after HTTP 429 responses. Production OAuth routing remains unchanged. The next gates are a later bounded public API matrix, elapsed-expiry/reconnect validation, and production session capture/account lifecycle before login unification.

## Independent public API groups and reconnect (2026-10-02)

Added `--web-api-surface all|profile|track|search|library|catalog` (requires `--web-api`). The default remains the original full matrix. Profile, track and search each make one fixed request. Library checks saved albums, saved playlists and recently played, with two dependent playlist reads when a validated playlist exists (at most five requests). Catalog checks the supplied track and its validated album/artist (at most three requests). Catalog intentionally omits profile-dependent artist top tracks; the full matrix retains that check. All groups retain fixed origins, small pages, bounded responses, no pagination following, redacted diagnostics and immediate stop on 401/429. Reports include the selected group and actual request count.

Added research-only `--reconnect`, restricted to Web API mode. It acquires a token, disconnects the provider, constructs a new provider using the same memory-only session, acquires a token again, and optionally performs the existing explicit renewal before exactly one selected group. Authentication/reconnect/renewal plus the group have a 120-second total bound. A `reconnectVerified` result proves provider reconstruction and authenticated token acquisition; HTTP compatibility remains a separate result and is not established by reconnect alone.

Provider fixtures now advance an injected clock into the one-minute renewal window and beyond token expiry. They verify fresh-token reuse, automatic acquisition of a replacement, rejection of a disconnected provider, and successful acquisition by a new provider. This verifies local expiry handling; actual elapsed-expiry recovery against Spotify is still a live gate.

One authorized live run used the previously supplied session:
`--web-api --web-api-surface search --reconnect`.

| Milestone | Outcome |
| --- | --- |
| Initial authentication | Passed |
| Disconnect and new-provider authentication | Passed |
| Explicit renewal | Passed |
| Search requests | One |
| Search response | HTTP 429, Retry-After 28 seconds |
| Search compatibility | Inconclusive; response shape not assessed |

The run stopped immediately without retry or further route requests. The one-use local session handoff was closed/cleared, environment removed and temporary executable deleted. The result shows that throttling also affects search directly; it does not demonstrate rejection or availability of browsing/library access.

Tests cover every group's route count, profile independence, fixed origins/read-only requests, immediate 401/429 termination, arbitrary group rejection, successful reconnect/renewal and distinction between lifecycle success and HTTP throttling. Auth/analysis fixtures and all research-probe tests passed; focused probe/auth race checks and vet passed. The successful matrix path now uses the same coverage classifier as early-stop reports, so completed runs also have an explicit coverage state.

Production OAuth routing is unchanged. Remaining work includes later live API coverage after throttling clears, actual elapsed-expiry recovery, production browser/session capture and account lifecycle. Do not run groups sequentially to evade a provider cooldown.

## Production cookie session foundation and parity goal (2026-10-02)

The user expanded the objective to full functional parity with cookie-based authentication. Earlier decisions to keep this permanently research-only are superseded by that objective; compatibility evidence is still required before claiming parity.

Implemented a shared API-owned auth runtime. Normal startup restores a dedicated encrypted `spotify_webplayer_session` record without a token-network request during construction. With a selected cookie session, WebAPI, Playback and InternalAnalysis explicitly use the same WebPlayer provider; missing, disconnected or unreadable cookie records never silently select saved OAuth credentials. Existing installations without a cookie record retain their current OAuth behavior while the renderer migration is completed.

The pinned reviewed WebPlayer contract now lives in the production auth package and is reused by the research command. Its original source license is retained there. The new session setting is registered for encryption and legacy plaintext migration, and remains excluded from generic settings routes. Bearers remain memory-only.

New routes:

| Route | Contract |
| --- | --- |
| POST /api/spotify/auth/session | Bounded JSON containing spDC; authenticates before encrypted persistence and returns status only |
| DELETE /api/spotify/auth/session | Removes the cookie, retains disconnected cookie mode, cancels account requests/media work and purges reference observations |
| GET /api/spotify/auth/status | Redacted provider, connected/authRequired and message fields |
| POST /api/spotify/auth/refresh | Cookie mode performs an actual provider renewal; OAuth compatibility retains its existing notification behavior |

The existing credentials GET returns no OAuth payload in cookie mode. Public API helpers now use the shared selected token source and refresh once on HTTP 401. Cookie mode sends bearers only to HTTPS api.spotify.com, disables cookie jars and redirects, and returns HTTP 429 immediately rather than retrying it automatically. Proxy/profile responses preserve Retry-After. Account changes/disconnect/shutdown cancel in-flight API requests through a shared lifetime; the response body owns request cancellation until closed.

The DownloadManager receives this same runtime for Playback. Its session/downloader media layer is retained, and API shutdown now explicitly closes its SessionManager. Account connect/disconnect closes streams, cancels active download jobs and retires/purges account-specific analysis references. Production analysis composition, complete media account-rotation behavior and queue/session lifecycle remain part of the completion audit.

New fixtures verify encrypted raw SQLite storage, restart restoration, all-purpose WebPlayer routing, token reuse, no OAuth fallback after logout/restart, credentials/status redaction, fixed bearer origins, one-401 renewal, immediate 429 return, bounded session input, shutdown rejection and in-flight request cancellation on logout. Focused API/auth race checks and vet passed; auth, existing API, crypto/validation and research-probe tests passed. No live account request or user-cookie persistence was needed for this implementation increment.

Full parity remains incomplete. The renderer still owns OAuth tokens and direct Spotify calls. Remaining work is to migrate its service, page calls, login/settings/startup/logout and enrichment gates to backend session/status/proxy APIs; verify every existing browse/library/catalog/download/stream workflow; finish account/session rotation and analysis composition; and obtain live public API plus elapsed-expiry evidence. Previous profile/search HTTP 429 results remain inconclusive. Browser session capture UX and supported platform validation also need completion.

Final integration check: the complete backend go test ./... suite passed after these changes; git diff --check passed.

## Frontend cookie migration and media account retirement (2026-10-02)

Normal frontend Spotify requests now use a backend resource-path helper. Profile, general/artist/album search, album metadata, artist/top-track metadata, recent listening and saved albums/playlists preserve their existing data shapes and request queue while removing renderer bearer headers and token gates. Album/playlist detail, play, shuffle and queue handlers also use backend paths; the existing first-party playlist scraping fallback remains. A repository audit found no direct api.spotify.com calls or bearer headers in frontend pages/services. Legacy OAuth helper methods/types remain unused by normal login and will be considered in the final cleanup audit.

The Spotify page, Settings and first-launch setup share a masked cookie connection form. Submission sends spDC only to the dedicated backend session endpoint, clears the input, and stores only connection state in the renderer. Disconnect calls the backend before clearing local account views. First-launch download-path setup remains available. Startup and background metadata enrichment use redacted backend status instead of renderer tokens. Profile retrieval failure alone does not hide browsing for an authenticated session. Persisted profiles and legacy tokens are removed during the store's version-five migration; profile/connected state is not persisted.

Frontend tests verify resource/query routing without Authorization headers, session connect/status/disconnect, cookie-input clearing and absence from localStorage, catalog/library calls without OAuth credentials, reconnect/rate-limit errors and playlist denial fallback. The full npm run check passed: palette checks, typecheck, 70 Vitest files / 284 tests, and production build. Backend proxy/profile requests now preserve Retry-After and map provider errors through safe session codes.

A subsequent account-lifecycle audit found a token-to-session race: a worker could acquire an old token and initialize media after a provider change. The runtime now serializes media preparation with account retirement. Retirement cancels the account lifetime and pauses/cancels active media before taking a write gate; old preparation must leave that gate before the SessionManager is cleared and a new provider is published. Token/request lookup rejects a retiring lifetime. Normal renewal retains lease-based rotation.

Download dispatch and stream HTTP requests now inherit the account lifetime. Session setup uses the caller/job context and a serialized preparation path. Streams reject canceled contexts before acquiring/pinning media. Account-specific cancellation remains distinguishable from user cancellation: delayed cleanup safely requeues unfinished downloading rows even if a new login has already cleared the auth flag. Persistent queued work remains intact and resumes with a fresh context/token; it cannot reuse the retired session. Saturated auth-event buffers cannot block retirement.

SessionManager account retirement waits for active leases, clears the session and cached bearer, and shutdown also erases the bearer. New regressions verify retirement ordering against blocked old preparation, replacement/logout, cancellation of dispatched work, safe late requeue/resumption and lease-held credential clearing. Focused API/media tests, race checks and vet passed.

Full parity is still unproven. Required remaining evidence includes live public API search/profile/library/catalog/pagination, complete application streaming/download workflows and expiry recovery, browser/native UI validation and remaining startup/error/queue audits. Detailed analysis remains unavailable for the tested recordings. Previous public API HTTP 429 results are still inconclusive; the successful audio-only research path does not prove browsing/library functionality.

Final lifecycle integration verification: go test ./... passed for the complete backend; git diff --check passed after the renderer whitespace fix.

## Shared public API cooldown and rejected-session state (2026-10-02)

Cookie-mode public API requests now share a cancellable request gate. A 429 records a deadline in the database; queued requests recheck it before sending, and later requests return 429 with the remaining Retry-After without contacting Spotify. Delay-seconds and HTTP-date headers are supported; absent or unusable values use a one-minute cooldown. The deadline survives disconnect, reconnect and restart. The gate applies to public Web API calls, leaving Playback token/session preparation available.

A rejected cookie token endpoint or a public API 401 repeated after one renewal marks the session disconnected, cancels its account lifetime and removes the rejected persisted cookie. Explicit connection with a newly validated cookie restores the account. Public 403, 404 and 429 do not invalidate the session. Search now uses the same safe provider-error mapping as profile/proxy, and frontend permission denials no longer become authentication errors.

Fixtures cover zero-network suppression across routes, queueing behind an upstream 429, HTTP-date handling, persistence/reconnect/restart, deadline expiry, independent playback, token-endpoint revocation, repeated 401 and non-authentication statuses. No live request or real cookie persistence was used for this increment. Live public API compatibility remains unresolved because the previous profile/search probes were throttled.

Verification for this increment passed: complete backend go test ./..., focused cookie API race tests, go vet ./internal/api, and the full npm run check (70 files / 284 tests plus typecheck and production build). git diff --check passed.

## Renderer reconnect and account response fencing (2026-10-02)

The renderer now has transient session-generation and reconnect-required state. The shared backend Spotify fetch boundary handles a current-account 401 by clearing connected/profile/search state and showing the reconnect form. It preserves 403 playlist fallback behavior. Account changes abort pending fetches; generation checks reject stale headers and JSON bodies without letting an old-account 401 disconnect a replacement account.

Queued catalog/library work captures its generation and rejects before network use or after completion when the account changes. Profile/search/library/detail effects guard state commits; startup status probes cannot overwrite a newer connection. Pagination now resolves its result updater before publishing to the store, avoiding a function being stored as search data. Account generation changes clear local library/search/loading views.

Background enrichment checks the current account before scheduling more work. Metadata enrichment also checks generation before Spotify calls and before saving results. Tests verify current-account rejection, stale 401 isolation, stale JSON rejection, queued-work suppression, reconnect UI and stopping background album scheduling during a pending request.

Full npm run check passed for the implementation (70 files / 289 tests, typecheck and production build); the separately added background hook test and reconnect component tests also passed. Live browsing/library compatibility and elapsed-expiry recovery remain unverified; no real session was used by these renderer tests.

Final renderer corrections passed typecheck and all 11 targeted session, reconnect UI and background lifecycle tests. The last full frontend suite passed 289 tests; the new hook regression passed separately.

## Live public search retry and Pathfinder research (2026-10-02)

A bounded public search retry used the consenting supplied account after prior cooldowns had elapsed. Authentication succeeded; exactly one api.spotify.com search returned HTTP 429 with Retry-After 52 seconds. No renewal or retry was requested, and the run stopped immediately. This repeated outcome leaves public search compatibility inconclusive and does not prove the cause of throttling.

Reviewed the same pinned source's Web Player client-token and Pathfinder protocol. Added an explicit research-only --pathfinder mode: fixed client-token acquisition, profileAttributes, and findTopResults for the fixed query music. It is mutually exclusive with all other probe modes, accepts no arbitrary operation or endpoint, bounds each HTTP request to ten seconds and the account run to ninety seconds, caps JSON at 1 MiB, disables redirects/cookie jars, and stops after any denied or incompatible operation. It is never an automatic fallback from HTTP 429.

The provider now retains the public clientId supplied by /api/token as private token metadata for backend consumers. This is Spotify's own Web Player client context; no user developer credentials are needed. Token and client-token values remain absent from reports. Client-token acquisition receives no cookie or bearer; Pathfinder receives the bearer and client-token, but no session cookie.

After the observed public cooldown had elapsed, one authorized Pathfinder run succeeded:

| Stage | HTTP | Evidence |
| --- | --- | --- |
| Cookie authentication | Successful | Existing authenticated token provider |
| Client-token acquisition | 200 | granted_token.token present |
| profileAttributes | 200 | data.me.profile.username present; value redacted |
| findTopResults | 200 | data.searchV2.topResultsV2.itemsV2 is an array; content redacted |

This proves the fixed protocol and basic response shapes for this account, rather than complete Web API response parity or coverage of all search categories/library operations. Production browsing still uses the public API. The next implementation step is a fixed-operation Web Player catalog adapter with typed normalization for the app's profile/search/album/artist/playlist/library/recent-listening surfaces, then production routing and full workflow validation. Retain public API throttling controls without attempting automatic 429 fallback.

Probe/auth tests passed, including fixed origins, bearer/client-token separation, provider clientId retention, redacted reporting, and immediate stop on 401/403/429. Vet passed for the research command and auth package. Both one-use pipe handoffs and environment variables were cleared; no real cookie or bearer was persisted.

Further primary-source leads for catalog operation research: [Spotui's Spotify adapter](https://github.com/Spotui/Spotui/blob/main/spotify/src/main/kotlin/com/metrolist/spotify/Spotify.kt), [ArchiveTune's adapter](https://github.com/rukamori/ArchiveTune/blob/main/spotifycore/src/moe/rukamori/archivetune/spotify/Spotify.kt), and [Spicetify's GraphQL API documentation](https://spicetify.app/docs/development/api-wrapper/methods/graphql). These show libraryV3 and other GraphQL integration patterns. They have not yet been pinned or validated for ViiB; retry/hash-refresh/debug-token behavior from third-party clients must not be adopted without review.

## Production Web Player profile adapter (2026-10-02)

Added backend/internal/spotify/catalog as a fixed-operation Web Player client. It mints client tokens using the provider's public clientId, coalesces acquisition, uses returned expiry/refresh timing with a one-day local reuse cap, retains credentials only in memory, and erases them on close/account retirement. Missing token expiry prevents cache reuse. Requests have fixed origins, no cookie jar or redirects, bounded timeouts and a 1 MiB response cap. Diagnostics redact client state; schema and network errors contain no response bodies.

Cookie GET /v1/me requests now use profileAttributes through the runtime-owned catalog client rather than api.spotify.com. Both /spotify/me and proxy path me use this seam. The shared persisted catalog cooldown and account lifetime remain authoritative. A Pathfinder 401 renews account/client context once; repeated rejection requires reconnect. Client-token acquisition failures do not by themselves invalidate the account cookie. Permission errors and throttling preserve connected state; a catalog 429 suppresses later public/catalog calls while playback remains available.

The live schema diagnostic confirmed profile username, uri, name and nullable avatar fields. Typed normalization returns the current identity, display name, images and Spotify user URL. Email, product, country and followers remain null because this operation does not supply them; the renderer omits those unavailable labels. These missing fields are an explicit parity gap, not inferred account values. Non-null avatar behavior still needs live coverage.

The first live production-client check exposed an overly strict upper bound on provider client-token lifetimes. It was corrected to accept positive provider lifetimes while capping local cache reuse. The corrected --catalog-profile check passed against the consenting account: authenticated=true, profileNormalized=true, identityAvailable=true, displayNameAvailable=true, imageCount=0, and country/product/followersAvailable=false. Only flags/counts were emitted. The probe uses the actual production catalog client and does not persist the real cookie. One-use session handoffs and environment variables were cleared.

Fixtures passed profile handler/proxy normalization, no public-API profile request, credential separation/redaction, client-token reuse/expiry, acceptance of long lifetimes, account replacement closing the old catalog client, one-renewal rejection, permission handling and shared 429 suppression. Frontend typecheck passed with nullable profile fields. Search, album, artist, playlist, saved-library and recent-listening routing still use the public API and remain required implementation work. Full profile field parity, elapsed token expiry and complete application workflows remain open.

Final checks passed: complete backend go test ./..., focused cookie lifecycle race checks, research command/auth tests and vet, frontend typecheck, and git diff --check. Additional schema/oversized-payload/identity-mismatch and pending-profile logout regressions passed under the race detector.

## Production Web Player search adapter (2026-10-02)

Cookie GET /v1/search now uses the fixed searchDesktop operation through the same runtime-owned catalog client as profile. Both the direct search handler and generic proxy normalize tracks, albums, artists and playlists into the fields consumed by browsing, playback/download actions and metadata enrichment. Protocol facts are pinned to Spotui commit f9d05b6450e730469d15f30f9dd4bc790db794db; see backend/internal/spotify/catalog/SOURCES.md. No upstream implementation code or mutable hash registry is used.

Queries validate term, requested categories, limit and offset. Typed conversion rejects incompatible schemas and missing playable track identity/duration. Recognized unavailable rows retain their cursor positions, while renderer/enrichment consumers skip them. Missing followers and other unavailable values remain null. Pages supply explicit items/next/previous fields. The UI now advances using server offsets rather than item counts, which differ across categories and can be augmented by playlist fallback results.

The production --catalog-search probe passed authentication, explicit renewal and two fixed music pages at limit 1. Available record counts were tracks/albums/artists/playlists = 1/1/1/1 at offset 0, then 1/0/1/1 at offset 1. Next-page bucket counts were 4 and 3 respectively. The report contained counts and flags only; no titles, IDs, account values or credentials. The real session remained in memory; one-use handoff and environment variables were cleared. This verifies the fixed live query and typed normalization, not advanced artist/album query matching or every unavailable-content variant.

Fixture checks passed all-category action fields, nullable availability, encoded paging, invalid queries, handler/proxy routing, shared client-token reuse, one-time renewal, repeated-401 retirement and shared 429 cooldown suppression. Full backend go test ./..., targeted catalog/API race tests, catalog/API vet and research probe tests passed. Full npm run check passed tests, typecheck, color checks and production build; final enrichment null handling also passed typecheck. Existing build chunk warnings remain.

Remaining parity work includes full album/artist/playlist detail routes, saved library and recent listening, profile fields absent from profileAttributes, natural elapsed-expiry recovery and complete app workflows. Album consumers currently assume a complete embedded track list; the next adapter must cover pagination before play/shuffle/queue/download actions can be considered equivalent.

## Production Web Player album adapter (2026-10-02)

Cookie album detail and track-page reads now use the pinned getAlbum Pathfinder operation. Normalized responses preserve identity, artist names, artwork, release date, label, copyright, duration, explicit availability and track/disc numbers. Unavailable track rows retain their pagination positions and are skipped for playback/download queueing. Genres and popularity are unavailable from this operation; they are not inferred.

The first live run authenticated/renewed but failed schema conversion. A field/type-only diagnostic established that copyright is an object with items, and album tracks omit __typename while supplying trackNumber, discNumber, duration.totalMilliseconds, artist and URI fields. The adapter was corrected for this fixed operation. A subsequent actual production-client --catalog-album run passed authentication, renewal and normalized offset 0/1 pages at limit 1 (one available track each). All real account/session values stayed in memory; the one-use pipe and environment were cleared. This is bounded catalog evidence, not grouped download completion or exhaustive regional/relinking coverage.

Renderer album play/shuffle/queue actions and the album detail page now collect every track page before using the album. Paging validates origin, album path, advancing offset and consistent total; account generation fences the entire operation. Duplicate track occurrences are preserved. Grouped backend album downloads already followed tracks.next through doSpotifyRequest and therefore now use this adapter too. Added an account lifetime for metadata collection and an originating-account check around album queue commits so an old request cannot queue after account replacement.

Fixtures cover full album fields and normalized proxy pages, client-token reuse, malformed inputs without upstream calls, renewal/repeated rejection, permission errors, schema failures and shared cooldown. A 51-track grouped metadata fixture verifies two pages, release/disc/track metadata and skipped unavailable rows. Renderer tests cover all pages, duplicate occurrences, unsafe/nonadvancing cursors, truncated results and replacement-account fencing. Grouped audio completion remains a required live app validation.

Validation passed: complete backend go test ./..., targeted album/catalog race checks, catalog/API vet, research probe tests, full npm run check, and final focused album/session frontend tests plus typecheck. The production build retains the existing chunk-size warning. Next required adapters are artist/top tracks, playlist detail/contents and saved/recent library surfaces; no claim of full parity or grouped live audio completion is made.

## Production Web Player artist adapter (2026-10-02)

Cookie artist profile and top-track reads now route to queryArtistOverview through the runtime-owned catalog client. Requested identity is validated against returned URI or ID; inconsistent identities fail schema validation. Recognized unavailable artists return a sanitized 404 while keeping the account connected. Profile normalization supplies identity, display name, avatar images, Spotify URL and follower count when provided. Genres and popularity are unavailable from this operation and are not inferred.

Top tracks preserve playable IDs, duration, artists, explicit availability and album name/artwork. The first live response omitted some top-track album names. The adapter resolves those via the already verified getAlbum operation, reuses results for repeated album IDs within the request, and keeps all related reads inside the same thirty-second transaction/account context. Unavailable tracks are skipped; malformed playable metadata fails conversion. Album HTTP rejection participates in the same one-renewal/shared-cooldown path.

After the schema corrections, the actual production-client --catalog-artist probe passed authentication, explicit renewal, normalized artist profile (three images and a follower count available), and ten available top tracks with normalized album metadata. Reports contained only counts/flags; no names, IDs or credentials. One-use handoff and environment variables were cleared. Country-specific top-track market behavior is not established: this web operation uses the connected account context, and independent market selection remains a parity gap.

Fixtures cover artist/profile and top-track proxy routing, origin/credential separation, missing-URI identity normalization, album-name resolution/reuse, requested-only profile behavior, duplicate occurrences, unavailable tracks, invalid input suppression, schema failures, one-renewal rejection and shared cooldown. Remaining implementations are playlist detail/contents, saved library and recent listening, followed by full app playback/download/lifecycle validation.

## Production Web Player playlist adapter (2026-10-02)

Cookie playlist detail and content routes now use the fixed fetchPlaylist operation. Responses normalize playlist identity/name/description, owner, artwork, follower count, revision and paged track metadata. Public/private status is nullable because this response does not directly supply the prior REST boolean; it is not inferred from capabilities. Unavailable/local/episode entries retain page positions with a null track and are skipped by music playback/download consumers. Unrecognized playable schemas fail conversion.

The first live run authenticated and renewed but exposed two wire differences: followers is a number on playlistV2, and playlist track duration is trackDuration.totalMilliseconds. A field/type-only diagnostic confirmed them. After correction, the actual production-client probe passed authentication, explicit renewal and offset 0/1 normalization at limit 1. A later fixed run additionally passed limit 100 and normalized all 50 available tracks of the first-party test playlist. Reports contain only flags/counts. The real cookie stayed in memory and one-use handoffs/environment variables were cleared. Private playlists, mixed content variants and very large live playlists still require coverage.

Playlist detail and play/shuffle/queue actions now load every track page. Validation checks origin/path, advancing offsets, total consistency and final completeness. Duplicate occurrences are preserved. Synthetic cookie track pages carry revision metadata so the renderer and grouped metadata collector reject changes between pages. Added outer account contexts and originating-account queue checks for grouped playlist downloads. Nested requests now retain the original account lifetime, preventing adoption of a replacement account between pages.

Fixtures verify normalized proxy detail/content fields, client-token reuse, two grouped metadata pages with 102 source positions and an unavailable entry, complete renderer pages beyond 100, duplicate preservation, changed revisions, truncated results, account replacement, invalid queries, permission/schema failures, one-renewal rejection and shared persisted cooldown. This does not establish grouped live audio completion, independent market selection, complete profile fields, saved library or recent-listening parity.

Final artist/playlist increment checks passed: complete backend go test ./..., catalog/API race tests, catalog/API vet, research probe tests, full npm run check and git diff --check. Final incomplete-playlist-page regression passed separately. Existing frontend chunk-size warnings remain. Full cookie/OAuth parity is still unproven and the goal remains active.

## Production Web Player saved-library adapter (2026-10-03)

Cookie /me/albums and /me/playlists now use the pinned libraryV3 operation through the account-owned catalog client. Requests use type filters, offset/limit, flattened folders and no optional synthetic collection features. Root item identities are validated before the verified album/playlist adapters resolve full display metadata and track counts. Related reads share the thirty-second transaction and account lifetime. Duplicate metadata lookups are reused within each page. Unavailable positions remain null and the renderer skips them; duplicate occurrences remain intact. Missing saved timestamps stay null rather than being invented.

The actual production-client --catalog-library probe passed authentication and explicit renewal. Albums reported total 63 and playlists total 228. Both types passed offset 0 and 1 at limit 1, plus offset 0 at the app's normal limit 20 with 20 available items. Reports contained counts/flags only. Cookies remained in memory; one-use handoffs and environment values were cleared. This proves sampled saved-library metadata browsing, not all-account contents, all nested album/playlist track fields, or complete playback/download workflows. Nested metadata track pages are intentionally small; existing detail/action loaders retrieve every needed track page separately.

Proxy fixtures cover both library types, pagination, query validation, client-token reuse, one-renewal rejection, non-auth failures and shared cooldown. Additional edge fixtures cover empty/end pages, unavailable positions, repeated IDs with one metadata lookup, conflicting/wrong-kind identities and truncated pages. The renderer preserves its existing first-page account browsing behavior; this increment does not add an unrelated load-more feature.

## Recently played protocol feasibility (2026-10-03)

Added an independent --web-api --web-api-surface recent mode to isolate the documented GET /v1/me/player/recently-played?limit=1 route. The live run passed authentication and explicit renewal, then returned HTTP 429 with Retry-After 12 seconds. Only one history request was sent. No track metadata or cursor contract could be verified from that response. Production retains the existing public-history path and shared cooldown behavior while a true equivalent is investigated.

Primary-source lead: [openclaw/spogo 243315d1e7c518e9abac35d4322ffcf3461d775d connect_user.go](https://github.com/openclaw/spogo/blob/243315d1e7c518e9abac35d4322ffcf3461d775d/internal/spotify/connect_user.go). It uses the internal recently-played/v3 route and playContexts[].uri/lastPlayedTime/lastPlayedTrackUri. A [desktop request trace](https://github.com/abba23/spotify-adblock/issues/37) supplies the default,track filter. The upstream implementation maps each context's last track into a recent-track row; that mapping is not evidence that individual/repeated track plays are preserved. Only independently written code using the protocol facts is included here.

Added --history-contexts as a build-tagged research-only diagnostic: validated account identity from profileAttributes, fixed internal HTTPS GET, at most two 50-item pages, bounded response/time limits, no redirects/cookie jar, no retry or production fallback, and count-only reports. Live authentication and explicit renewal passed. Offset 0 returned 50 contexts, zero track contexts, 37 last-track references and 50 timestamps. Offset 50 returned 50 contexts, zero track contexts, 33 last-track references and 50 timestamps. No context identity repeated across these sampled pages. This establishes a context-oriented response, not Web API track-event/cursor parity. The report explicitly sets trackEventParityVerified=false. No production substitution was made.

The diagnostic fixtures verify fixed route/filter/page bounds, bearer/client-token separation, redacted output, stop on rejection, malformed/oversized response rejection and invalid identity suppression. The [documented Web API contract](https://developer.spotify.com/documentation/web-api/reference/get-recently-played) remains the parity target. A direct track-event history contract, album artwork, repeated-play preservation and after/before cursor behavior still require verified source and live evidence. Undocumented listening-history/v2 mentions are leads only, not an implemented contract.

Saved-library/history increment validation passed: complete backend go test ./..., research probe tests, targeted library/catalog race tests, API/catalog/research vet, full npm run check (302 tests, typechecks, palette/color checks and production build), and git diff --check. Nested library fixtures additionally verify successful child-read renewal, repeated child rejection and shared child-read cooldown. Existing Vite import/chunk and Browserslist-age warnings remain. All live research handoffs were closed and credential/environment values cleared. Full parity remains unproven; recent track events, remaining profile/market fields, individual track metadata routes and full app playback/download/expiry/lifecycle checks are open.

## Production individual and batch track metadata (2026-10-03)

Cookie /tracks/{id} and /tracks?ids=... now route to a fixed getTrack operation through the runtime-owned catalog client. The source/hash and live transport evidence are recorded in backend/internal/spotify/catalog/SOURCES.md. Typed conversion validates recording identity, duration and artists, accepts verified ID-based identity where URI is omitted, resolves exact album artwork/release dates with the existing getAlbum adapter, and does not guess track or album names. Metadata failure remains a typed schema/provider failure. Market arguments are validated but independent country selection remains unverified; the web operation uses the account context.

Batch requests accept at most 50 validated IDs, preserve input order and duplicate/null positions, reuse track and album lookups within the request, and remain inside one thirty-second account transaction. Recognized unavailable types/playability and track HTTP 404 return null batch entries; individual unavailable tracks return sanitized 404. Requests stop on auth/throttle/schema failures and use the existing one-renewal/shared-cooldown behavior. This is a composition of verified single-ID reads, not an invented batch hash.

The actual production-client --catalog-track probe passed authentication, explicit renewal, single-track normalization with artwork and exact release date, a three-position/two-ID batch with a repeated ID preserved, and a normal 50-position metadata batch with all 50 available tracks. The 50 IDs came in memory from the fixed first-party playlist already used for catalog validation. No identities, names, timestamps or credentials were emitted by the reports. One-use handoffs/environment values were cleared.

Direct track-URL downloads now use the same cookie adapter and preserve the original account lifetime through metadata collection and queue commit. A stale request cannot queue under a replacement account. Authentication rejection and rate-limit errors use the shared redacted session error response, preserving 401 and Retry-After. The existing single-track endpoint that accepts renderer metadata continues to queue the supplied metadata without adding a new lookup.

Fixtures cover exact operation/credential separation, single-track playback metadata, batch order/null/duplicate behavior, track and album reuse, invalid queries without upstream calls, rejection/schema/cooldown behavior, direct URL queued metadata and account-change cancellation without queued rows. Generic public-request lifetime/cooldown tests now use the still-public recent-listening route so they remain distinct from catalog-specific dispatch coverage. No live full download-worker completion or frontend/native workflow is claimed by metadata/queue fixtures.

Track URL error fixtures exposed a subsequent-request classification issue after cookie rejection: the canceled session lifetime could reach the request gate before the authentication state check and return a generic service error. The shared boundary now checks disconnected cookie state before that gate and returns authentication-required. Focused track/session/cooldown checks pass; the final full backend and race checks are recorded below when complete. Independent market selection, true recent play events, complete profile fields and full app playback/download/expiry/lifecycle validation remain open.

Final track increment checks passed: complete backend go test ./..., research probe tests, focused track/account/session/cooldown race tests, API/catalog vet and git diff --check. No frontend code changed in this increment; the previous full npm check remains the latest frontend result (302 passing tests and build). Full metadata batch coverage is now live-verified for the fixed 50-track sample; complete application audio/lifecycle verification and the unresolved recent-history/profile/market gates remain required before goal completion.

## Production browser app workflow (2026-10-03)

Built the actual browser-mode cmd/viib executable with the current production frontend bundle. Used a unique temporary viib-cookie-app-* root for APPDATA, the -data database/log/cover directory, default Spotify downloads and SDK cache. The authenticated run used loopback port 34159. The user's original library/configuration was not used. Connected through the production POST /api/spotify/auth/session endpoint; the real cookie was delivered through a one-use memory pipe and stored only by the existing encrypted setting boundary. GET /api/spotify/credentials returned an empty object. Raw isolated database inspection found the encrypted-value marker and no cookie prefix, and subsequent restart successfully restored profile access.

Chrome/Playwright exercised the actual production frontend: startup restored the account page, Saved Albums rendered 20 cards with artwork/year/track counts, Saved Playlists rendered 20 cards, and the fixed public album detail rendered its available track list. Clicked Play album, observed a SPOTIFY/Streaming source, Pause control, advancing seek position (45+ seconds on the first observed track), queue state and subsequent streamed tracks. Playback continued after explicit POST /api/spotify/auth/refresh and Next track; a later seek jumped to about 117 seconds in the new track while playback remained active. This verifies real browser playback controls and explicitly renewed media acquisition for this sample, not elapsed token expiry or all playback engines. Visual evidence: [production album playback](../output/playwright/spotify-cookie-album-playback.png).

The production streaming handler returned HTTP 206, audio/ogg, Content-Range bytes 0-4095/13666697, 4096 bytes and an OggS header for the supplied recording. A direct track-URL download completed through the real automatically started DownloadManager, produced an Ogg file under the isolated download root, and entered the local library. Full read-only PCM verification reported 13,666,755 bytes, 44,100 Hz, two channels, declared/decoded frames 16,863,840 and 382.4 seconds. This is a production worker result, beyond the earlier isolated SDK probe.

Enabled the existing automatic Ogg-to-MP3 setting and queued the second supplied track through the same URL download endpoint. It completed with an MP3 file; full decoding reported 8,318,527 bytes, 44,100 Hz stereo, 9,169,920 frames and 207.934693877551 seconds. Both completed tracks appeared in the local library after restart. Artwork/metadata cache-miss requests produced expected local 404 console entries while enrichment ran; these were not authentication failures. This sample does not establish every grouped download, tag/cover variant or quality negotiation.

Stopped the app after closing the browser; logs confirmed HTTP shutdown completion and all download workers stopped gracefully. Restarted with the same isolated encrypted database: status remained connected in webplayer mode, a live profile read succeeded, and both local tracks remained available. Used the actual browser Disconnect control: the login form returned, status became disconnected/authRequired, and profile reads returned 401. Restarted once more: webplayer selection stayed disconnected, profile reads remained 401 and the credential response stayed empty. No OAuth fallback occurred. The browser and all validation app processes were closed. Temporary database, logs, SDK cache, executable and audio files were removed after final verification.

Before authenticated testing, a startup on port 34115 encountered an already-open client tab. It was stopped and the authenticated run moved to a separate unused port. That initial shutdown hit the thirty-second HTTP deadline while the client SSE connection remained open; this is retained as an observed shutdown limitation, not counted as a clean shutdown. Authenticated runs with the test browser closed shut down cleanly. Live shutdown with active clients/media still needs coverage.

Added build-tagged --verify-app-downloads ROOT for reproducible read-only full PCM validation. It accepts only an immediate temporary-directory child named viib-cookie-app-*, excludes SDK cache, rejects symlinks, bounds eight audio files and decoding time/size/duration, and emits format/PCM metrics without file paths, identities, metadata or credentials. Research fixtures verify arbitrary-root rejection, empty-artifact rejection and corruption rejection; research tests and vet pass. No normal application code changed in this increment, so the preceding full backend/frontend regression results remain applicable.

Remaining full-parity gates include natural elapsed token renewal, account changes/logout during active media, grouped album/playlist worker completion, native Wails playback/UI, complete profile/market semantics, individual play-event recent history, and the final original-plan/functional requirement audit. The successful production browser workflow does not close those gates.

## Renderer playback retirement (2026-10-03)

Review of active-media logout found that the backend retired streams and workers, but renderer logout left the current Spotify song playing and eligible for network retries. Logout and authentication rejection now retire Spotify playback: pending song resolution and delayed retries are invalidated, remote Spotify queue entries are removed, buffering/error state is reset when the active stream stops, and downloaded/local/Plex playback and duplicate queue positions are preserved.

The audio hook releases both active and preloaded media sources when playback is cleared. On Spotify disconnection it also releases outgoing/gapless Spotify sources while preserving local audio. Hidden Spotify preloads now have explicit retirement cleanup, remove listeners/timers and release their network source; stale completion cannot restore the preload state.

Fixtures cover disconnect and authentication rejection with an active stream/delayed retry, preservation of downloaded/Plex queue entries, cancellation of hidden preload, and an in-flight Spotify selection completing after disconnect. These are renderer lifecycle fixtures, not live active-media logout/reconnect evidence. Grouped real worker completion, natural expiry, native playback and the other full-parity gates remain open.

Validation passed: full npm run check (74 test files, 307 tests, both TypeScript checks and production build), focused player/session tests and git diff --check. The existing production chunk-size warning remains. No live cookie credentials were needed for this increment.

## Grouped download failure and cancellation handling (2026-10-03)

The four active album/playlist download entry points (direct POST and URL dispatch) previously converted typed cookie authentication/rate-limit failures into generic 500/502 responses. They now preserve 401 and 429 with Retry-After; rejected metadata reads never admit a queue batch. Queue commits also preserve account-retirement authentication errors. Other download failures retain their endpoint-specific failure status with a sanitized message rather than an upstream response body.

Album/playlist page collectors and scraped track-batch reads now retain status-bearing upstream errors. This also preserves final OAuth HTTP failure statuses without changing the existing OAuth token-renewal/retry owner. Cookie catalog shared cooldown and exactly-one authentication renewal remain unchanged.

The active grouped playlist fallback now calls ScrapePlaylistContext with its originating account/request context. The embed fetch uses NewRequestWithContext, so logout, replacement and client cancellation interrupt the request instead of waiting for the thirty-second timeout. The old no-context public function remains a compatibility wrapper for existing callers. This does not establish that embed data is complete for every long playlist; the separate detail scraper and unused legacy download helpers remain outside this cancellation change.

Regression fixtures cover each of the four cookie grouped entry points with exhausted 401 and 429, renewal/read counts, shared later failures, Retry-After preservation, disconnected-versus-retained account state, zero queue rows and upstream-body redaction. Additional fixtures exercise status-bearing raw HTTP failures and an actual in-flight transport canceled through the scraper context. No live Spotify requests or credentials were used in this increment.

Validation passed: full backend go test ./..., focused grouped/lifecycle/scraper race checks with MinGW, API/Spotify go vet, research probe tests, and git diff --check. The previous full frontend check remains applicable because this increment changed only Go sources. Grouped production audio completion, live active-media logout/reconnect and natural elapsed renewal remain required; full OAuth functionality parity is still unproven.

## Functional parity audit and individual history lead (2026-10-03)

Added SPOTIFY_COOKIE_AUTH_PARITY_AUDIT.md to distinguish actual reachable renderer features, implemented adapters, live samples and unmet original-plan requirements. Recently played is an active UI feature still lacking verified cookie support. No active SpotifyService/UI caller was found for Spotify discography, recommendations or write/library-edit actions; generic proxy compatibility remains a separate open audit item.

Inspected pinned librespot protocol schemas (939dc5ee9d833e1980f9495241219d9d4868a061) and Spotify's public desktop bundle/Recently Played route without authentication. Context schemas and player-local history do not establish the individual played_at/cursor contract required by the existing Recent tab. Main-bundle absence alone does not prove that no track-history service exists.

Added research-only --history-events, mutually exclusive with other catalog/research modes. It makes one fixed GET to the community-reported https://spclient.wg.spotify.com/listening-history/v2 root, with derived bearer and independently minted internal client token; no cookie reaches that origin. Requests use the bounded research client, no redirects or retries, a one-MiB response limit and a sixty-second workflow limit. Successful diagnostics expose only root type, byte count and counts for a fixed allowlist of containers, never arbitrary keys or scalar account values. This is explicitly not a production adapter or a track-event parity assertion.

Actual run: authenticationVerified=true, renewalVerified=true, failure stage history_events, HTTP 404. No endpoint variants were guessed or requested. The real session stayed in a one-use memory pipe/environment boundary; the environment, memory cookie and handoff were cleared. This closes only the fixed-root endpoint lead, not recently-played parity.

Research tests and vet pass. Fixtures cover fixed origin/method/no query, credential separation, one-request rejection stopping, private key/value redaction, malformed payloads and response size. Normal application code did not change in this increment. Required next evidence remains true individual-event history, real grouped worker completion, active-media retirement/reconnect and natural expiry.

## Real grouped album workers and empty selection (2026-10-03)

Built the current production browser-mode executable/frontend and started a new isolated viib-cookie-app-* root on loopback port 34169. APPDATA, database, downloads, SDK cache and logs stayed in that root. Connected through the real cookie session endpoint using a one-use memory pipe. Credentials remained an empty object; no developer application credentials were supplied.

Resolved the album of the supplied recording through the production track/album adapters. POST /api/spotify/download/album queued ten tracks as one group. All ten real workers completed (three download workers configured), produced ten Ogg files and entered the local library. After app restart the ten completed rows remained.

Full read-only decoding passed all ten files: each was stereo 44,100 Hz and each decoded frame count exactly matched its declared frame count. Durations ranged from 130.4 to 382.4 seconds. Independent music-metadata parsing verified essential title/artist/album tags, album artist/year, disc 1 and track numbers 1 through 10. All files had numbered album paths. One cover.jpg sidecar was present and all ten download rows retained artwork URLs; embedded Ogg picture tags were absent in this sample. This proves a bounded grouped album workflow, not every album/playlist/tag/quality variant.

A small saved playlist contained two source positions with no downloadable music IDs. Its grouped request originally returned a generic 500 because QueueDownloads rejected an empty request as an ordinary internal error. Added a typed no-downloadable-tracks error and mapped it to HTTP 422 with a clear unavailable-selection message across the download error boundary. After rebuilding/restarting, the actual same request returned 422 and inserted no rows; the completed row count stayed ten. Fixtures cover all four direct/URL album/playlist entry points with only unavailable rows, zero insertion and retained connected account state.

A separate small saved-playlist full-content request returned a catalog/schema failure (actual proxy HTTP 503). This is retained as an open production compatibility defect, not counted as successful playlist grouping. The initial small-playlist checks also used the detail response's limited first page before requesting its complete track page; the final empty-selection evidence uses both source positions and no next page.

Extended the isolated read-only audio verifier's bounded artifact count from eight to thirty-two to validate a complete ten-track album; its root/symlink/cache/size/duration/120-second decoding restrictions remain. Research tests pass. Full backend tests, API/Spotify vet, focused grouped race tests and diff checks pass.

Both validation app runs shut down with HTTP shutdown complete and workers stopped gracefully (PTY shell exit 1 reflects Ctrl+C). Disconnected before final shutdown, confirmed zero app processes, and deleted the exact verified temporary root including its database/audio/cache/logs. Cookie memory/handoff was cleared. No browser was opened in this increment, and active-media logout/reconnect and natural elapsed token expiry were not verified.

## 2026-10-03: active playlist retirement, reconnect streaming, and Ogg integrity

A bounded isolated production app queued a seven-track saved playlist with cookie authentication and empty developer credentials. Three workers were active when logout was issued. All seven rows returned to queued state; session status became disconnected/auth-required. An open HTTP audio response stopped after 753,664 bytes of its declared 13,666,697-byte body. Reconnect resumed the workers and all seven rows became completed. Tags were present and numbered playlist filenames were retained.

This test revealed a separate streaming lifecycle defect: account retirement permanently closes Streamer, while the API previously reused it when SessionManager's pointer was unchanged. Retirement now clears the API-owned streamer/session pointers under the existing mutex. A focused same-manager regression verifies that the retired object rejects preparation and a new shared streamer is created. A rebuilt production app, in one process, returned HTTP 200 before logout, then successfully returned HTTP 206 with exactly 4,096 bytes and Content-Range bytes 0-4095/13666697 after reconnect.

Completed worker rows alone did not prove playable artifacts. Full strict decoding found one bad Ogg page checksum among the seven files. A bounded page scan found exactly one bad page; correcting only the checksum on a scratch copy allowed decoding, but this is diagnostic evidence, not a production repair. A fresh untagged HTTP stream of that recording had 1,832 pages and zero bad checksums. Source/capture corruption versus an earlier write is not conclusively established. Focused TagLib experiments on a repository fixture preserved valid page checksums with the production tags and large metadata; TagLib also accepted and preserved a deliberately corrupted untouched middle-page checksum.

The downloader now verifies each complete Ogg page using the non-reflected Ogg CRC polynomial, bounded page buffers and cancellation checks. Raw bytes are checked before tag writing; tagged files and existing local downloads are checked before accepting success. Corruption is rejected rather than checksum-repaired. Tests cover valid pages, middle-page payload/checksum corruption, incomplete/trailing bytes and cancellation. Cancellation during existing-file validation is checked before removing the destination.

The subsequent real group run is **not a passing parity result**: one new artifact completed and fully decoded, one raw temp file was rejected before tagging, and five workers received audio-key errors (invalid AES key size 0). Older completed rows remained in the database, so aggregate completed-row counts are not artifact completeness. The underlying corruption/audio-key failures and reliable recovery remain active work. The isolated app data is retained temporarily for diagnosis.

A research-only saved-playlist probe now supports a validated playlist ID through a transient environment value, or bounded saved-library selection, without outputting private IDs. Allowlisted shape diagnostics report row/type/required-field counts without arbitrary response keys/scalars. The same seven-track target normalized successfully; the prior production HTTP 503 was not reproduced and should not be described as a proven schema defect.

Validation: backend suite, focused API/Spotify race checks, vet and research-build tests passed for the lifecycle/integrity changes. The final cancellation guard also passed the targeted API/Spotify rerun and vet. Frontend code was not changed in this step.

## 2026-10-03: preserve replacement destinations and distinguish recovery failures

The next bounded retry run completed one of six failed rows; the other five exhausted their three attempts at raw Ogg validation. This run no longer reproduced the prior audio-key failures. A fresh production HTTP stream from one of those five recordings returned 12,800,893 bytes: all 2,951 parsed Ogg pages had valid CRCs, the final page had EOS, and exactly three zero bytes followed it. Thus CRC-valid recordings can be rejected by treating a short zero trailer as an incomplete next page. The earlier EOS-absence hypothesis was not established.

Validation now accepts at most 15 zero trailer bytes after a complete CRC-valid EOS page. It still rejects missing EOS, nonzero trailers, partial pages, oversized zero trailers and checksum mismatch. Regression cases cover 1/3/15 accepted bytes and 16-byte/nonzero/missing-EOS rejection. This bounded trailer tolerance is based on the observed sample; it does not establish every possible Spotify asset format.

Replacement downloads now preserve the existing destination through authentication, transfer, tagging and validation. Only the final rename of a validated candidate replaces it. Go's local Windows implementation uses MoveFileEx with MOVEFILE_REPLACE_EXISTING; a platform regression exercised replacement of an existing destination. A session-failure regression verifies that a prior file survives a failed replacement. The old implementation deleted it before any network success. The scanner only reconciles missing-file records and did not account for disappearing media.

Ogg errors now retain a typed sentinel and diagnostic checksum/missing-EOS reasons; the existing bounded three-attempt worker retry recognizes candidate integrity errors. Post-tag validation preserves the actual error instead of collapsing it to a boolean. Arbitrary checksum repair remains excluded.

A concurrent worker reset exposed another production bug: HTTP playback returned 401 with "Spotify session expired" for the actual error "failed to get session: session not initialized", while auth status remained connected and not auth-required. Both playback preparation/error paths now use typed auth/upstream classification and return 503 for temporary session failures. This prevents renderer logout from a media-session recovery race. A regression verifies temporary session errors produce 503 while a typed authentication rejection remains 401.

The isolated app was rebuilt and the remaining five failed rows were requeued with the zero-trailer correction. Final production completion/artifact verification follows below.

Final trailer-corrected live result: all five remaining rows completed. The seven playlist artifacts fully decoded as Ogg, 44.1 kHz stereo; every declared frame count exactly matched its decoded count (durations 138.427–287.387 seconds). Essential title/artist/album/track tags, 001–007 filename numbering and cover.jpg were present. One new raw page-checksum mismatch was observed on the first attempt and the bounded retry produced a valid final recording. This exercises recovery of real corruption rather than merely accepting zero trailers. It proves this seven-track playlist sample, not every account/market/quality variant.

Final checks: complete backend suite, research-build tests/vet, API/Spotify race suites and vet passed. No frontend changes were needed in this step.

A fresh normal-app Recent-feature request through the selected cookie context returned 429 with Retry-After 42 seconds. No play-event payload was returned; Recent parity remains unverified. The observed shared cooldown was honored without an immediate repeat.

## 2026-10-03: production reference composition and separate renderer panel

Normal API construction now composes the existing inert analysis client/refresh service only for a selected connected WebPlayer cookie. Successful cookie connection installs a new service after the provider commit, under the serialized account transition. Reconnection closes and purges the old account service/cache before composing its replacement. OAuth-only/disconnected startup remains unconfigured; explicit optional-analysis disconnect remains cache-purging. Construction performs no remote reference fetches. Tests verify inert composition, preserved same-account startup cache, replacement-service identity, account-cache purge, logout and API-closed rejection.

The actual normal-app scalar refresh route succeeded for the supplied recording: HTTP 200, BPM 108.022, Camelot 2B, null confidence values, source audio_features. An explicit detailed-analysis refresh returned HTTP 404/not_found and left the separate scalar cache intact. No endpoint fallback was added.

The BPM editor now includes a separate Spotify reference panel using typed services/spotifyReference DTOs. It resolves source-fingerprint-valid recording identity, offers explicit same-recording/version confirmation from a Spotify track URL or ID, and reads cache without provider I/O. Users explicitly choose audio features or detailed analysis before refreshing. It shows nullable BPM/key/individual confidence, endpoint provenance, retrieval timestamp, stale status, cached data alongside refresh failures, and an offline refresh state. This component never writes effective BPM/key, grids, cues or sync state.

Renderer request results are fenced against song/source/session changes. A current provider 401 retires the selected Spotify renderer session; an old response cannot retire a replacement session. Tests cover nullable/stale cache, explicit detailed refresh, offline disabling, late-source-result rejection, current authentication retirement and stale-session rejection.

Using the Playwright skill/CLI in an isolated production browser, the actual downloaded recording was loaded into Deck A. The manual confirmation UI linked its exact completed-download Spotify ID; explicit refresh rendered Spotify BPM 109.724/Camelot 12A with unknown BPM/key confidence. The local measured BPM field stayed 110. After restarting the backend and reloading/reselecting the same recording, both identity and cached reference remained available without another provider refresh. Rendered artifact: output/playwright/spotify-reference-panel.png (ignored local artifact). The dark dropdown contrast was corrected and the final render inspected.

Validation: full backend suite; API/all Spotify package race suites and vet; full frontend check (palette/raw-color/type/test/build), final type/build and 314 frontend tests in 76 files passed. Existing bundle-size warnings remain.

An app restart while the browser's event streams were still open reached the HTTP server shutdown deadline; worker shutdown completed afterwards. Closing the isolated browser before final shutdown avoided that deadline. This is a remaining lifecycle investigation, not a passing open-browser shutdown claim. The isolated browser and app were stopped; diagnostic app data remains retained.

Still open: automatic exact completed-download identity retention, reference comparisons/benchmark integration, wider original-plan gates, Recent play-event parity, natural expiry, native/cross-platform tests and broader playlist variants. This production increment does not claim full goal completion.

## 2026-10-03: event-stream shutdown and durable download recording evidence

Browser-mode HTTP requests now share a cancellable server base context. Shutdown cancels that context before waiting for HTTP handlers, allowing open event streams to exit. A real TCP regression verifies an open flushed stream closes cleanly. In the isolated production app, all four open library/download/job event streams returned HTTP 200 and ended without body errors when the app stopped; logs show HTTP shutdown completed in approximately 9 ms. This closes the observed four-stream shutdown timeout. Native Wails and an active enrichment stream remain separate unverified cases.

Successful download completion now verifies the exact final tagged/converted regular file with a streaming full SHA-256, size, nanosecond mtime and before/after file identity/stat checks. A context-aware SQL transaction records both conditional queue completion and durable recording evidence. Evidence is independent of the queue and contains no session credentials. Historical path-only completed rows are not automatically trusted.

Completion and scanner upserts reconcile the canonical persisted song by exact path, verify its bytes/revision against that evidence, and link only an unambiguous recording. Local analysis and links share the same source-fingerprint helper. Manual identities are preserved; explicit removal records a source-revision suppression so later scans cannot silently restore it. Automatically established identities can follow a newly verified source revision. No title/artist matching or automatic provider analysis fetch was introduced. The identity-table constraint migration preserves existing manual rows and foreign-key deletion behavior.

Conversion now detaches from the finishing worker while retaining the original account lifetime. Retirement cancels conversion and requeues its conditional converting row. Completion commits under the runtime account gate, preventing a retired/replaced account from completing work after its lifetime ends.

Tests cover queue clearing and database reopening, canonical path upsert identity, same-size/same-mtime content replacement, ambiguous recording evidence, historical path-only rejection, manual-link preservation, explicit unlink suppression/reconfirmation, cancellation rollback, schema migration and account-bound conversion cancellation.

The retained isolated production app authenticated without new credentials, verified an existing valid downloaded Ogg through the normal worker, and exposed a matching download_completion recording link through the normal local analysis API. All 14 completed queue rows were cleared in this isolated test database. The automatic link remained available after queue clearing and a complete app restart, with zero queue rows. This is one retained-artifact/recompletion sample, not a new network transfer or every conversion format. The diagnostic app was stopped afterward.

The full backend suite and vet passed. No frontend behavior changed in this increment. Remaining parity work includes individual Recent play-event history, natural elapsed bearer renewal, broader group/account/market variants, native renderer lifecycle, comparison/benchmark integration and the original-plan final audit.


Final verification for this increment: full backend tests/vet passed before the last ambiguity guard, and DB/analysis/API tests plus vet passed after it. Focused race checks covering Spotify, downloads, conversion, recording evidence/schema migration and HTTP shutdown passed in DB/API/viib packages. The broad API race attempt exceeded its 120-second limit while the existing 1,000-track analysis scale test was still doing analysis/database work; it is not recorded as a full-suite race pass. Conflicting newly completed evidence now also retires a prior automatic link, while preserving manual confirmation. Formatting and diff checks pass.

## 2026-10-03: actual browser account retirement, form reconnect and natural-expiry observation

Using the Playwright skill/CLI with the real isolated production app, Spotify search loaded a recording and active browser audio advanced with readyState 4. Clicking the actual Disconnect button returned the UI to the cookie form, paused both audio elements, reset their time and removed their source attributes. A later observation still showed both paused/source-free. Backend status reported the selected cookie account disconnected/auth-required.

Two actual browser form submissions used a one-use memory-only localhost handoff for the already-authorized cookie. The form cleared/unmounted its credential field; no cookie was written to a script, artifact, command argument, browser trace or source repository. Both handoff listeners closed and the in-memory credential variable was cleared. Reconnection did not auto-resume old playback.

The first post-reconnect search returned HTTP 503 while the account remained connected. Its cause was not recorded by the then-generic error mapping. Added a redacted catalog diagnostic with fixed error codes, an allowlisted stage and an upstream status number; it excludes error strings, resource/query/account IDs, tokens and provider bodies. Tests reject arbitrary stage/error strings. The same exact search later returned 200 after restart and after a repeated same-process form reconnect, so a consistent reconnect failure was not established. The original isolated 503 remains unclassified.

With diagnostics installed, the repeated real-browser cycle passed: active Spotify audio advanced, UI disconnect retired it, form reconnect succeeded, the exact catalog search returned 200, and new browser Spotify playback advanced beyond seven seconds with readyState 4 in the same application process. This closes the browser sample gap, not native Wails or all media/session variants.

The local copy was then played through the actual library UI. Its /api/audio source advanced from about 30 seconds before Spotify disconnect to 52 seconds afterward, remained unpaused/readyState 4, and retained its source. The hidden Spotify preload was paused/source-free/readyState 0. Local queue playback continued into another local song. The inspected screenshot output/playwright/spotify-local-playback-after-logout.png shows the disconnected Spotify form beside the active local Ogg player. The test browser was closed afterward.

Auth status now exposes only cached tokenExpiresAt and tokenIssueCount metadata. Reading it performs no provider I/O; it contains no cookie, bearer or client context and hides cached lifetime after disconnection. The provider count increments only when a successful exchange is committed. Tests verify non-minting reads and credential-free serialization.

The natural-expiry observation completed successfully. output/playwright/spotify-natural-renewal-result.json records issue count 1 to 2, expiry advancing from 2026-10-03T23:49:20.194Z to 2026-10-04T00:44:41.495Z, verified metadata, and HTTP 206 with 4,096 audio bytes. The monitor waited until twenty seconds after expiry, polling only cached status before requesting metadata and audio without explicit refresh. This proves automatic recovery after idle expiry. It did not measure uninterrupted active playback across expiry; that remains a separate gate. The completed artifact does not establish that the app or monitor is still running.

Targeted API/auth tests, vet, and race checks passed. The full backend suite and vet also passed. Full goal parity remains unproven: Recent track-event history, active-playback expiry continuity, native/cross-platform lifecycle, broader account/market/group variants, production session acquisition UX and original-plan comparison/final audit still remain.

## 2026-10-03: continuation review and remaining test gates

Reviewed the untracked parity audit, implementation plan, validation log and catalog SOURCES.md. Historical implementation-plan entries describe incremental progress; use the latest validation entries and parity audit for outstanding gates.

Repository-investigator verification passed: npm run check (77 test files, 325 tests, type checks and production build); go test ./... -count=1 -timeout=240s; targeted API/Spotify/browser-entrypoint vet; research-probe fixtures; native-shell package tests; and five focused frontend Spotify test files (32 tests). Parent git diff --check also passed. Current-environment race execution was unavailable because CGO was disabled and no GCC toolchain was installed. Earlier recorded race passes remain historical evidence, not a new race pass. Known build warnings remain.

| Priority | Gate | Required passing evidence |
|---|---|---|
| High | Spotify sign-in without cookie pasting | User completes Spotify's own login in a controlled browser surface; backend captures only the resulting session and validates/stores it through the existing encrypted boundary. Verify cancellation, browser closure, timeout, failed validation, restart restoration, logout and reconnect. ViiB must not collect passwords. A CDP HttpOnly-cookie fixture passed, but actual Spotify login compatibility remains unproven. |
| High | Recently Played individual events | Test the actual /spotify Recently Played tab and backend proxy request after the shared cooldown clears. Require successful track-event data, timestamps/order and agreement with rendered rows. HTTP 429 remains blocked evidence; listening-context summaries are not event parity. |
| High | Complete saved library | Follow album and playlist pagination to exhaustion; verify terminal totals, page boundaries and identities without silent omission. Existing bounded page probes do not prove traversal. |
| High | Private and large playlists | Use an explicitly selected consenting private playlist and a playlist larger than a page; verify complete ordered items, valid duplicate preservation, unavailable rows, stable revision and matching total. The default small saved-playlist probe is insufficient. |
| High | Active playback across natural expiry | Observe advancing currentTime, readyState, paused state and source before and after expiry, then seek/next. The completed idle-renewal monitor does not establish continuity. |
| Medium | Native Wails and cross-platform lifecycle | In isolated app data, verify login, playback/seek/next, account retirement/reconnect and clean shutdown, including open event/enrichment streams. Native-shell package tests do not prove the native UI workflow. |
| Medium | Broader parity and final audit | Cover relinked/unavailable/market-specific content, profile-field differences, repeated grouped-download integrity, supported proxy transport, and original-plan comparison gates. Record account/platform limits explicitly. |

No new live request using the newly supplied cookie was made during this review. Production automatic login remains unimplemented: SpotifySessionConnect still presents the manual cookie form and the backend has no login-capture route. The user requires Spotify sign-in without cookie pasting as a completion gate.

## 2026-10-03: browser sign-in implementation and full-library gate

Normal SpotifySessionConnect now offers Sign in with Spotify and Cancel sign-in rather than a cookie field. Backend-only capture opens an installed Chrome/Chromium/Edge in a unique temporary profile, navigates to Spotify's own login page, reads only the resulting Spotify-scoped sp_dc through CDP, closes the browser and removes its profile before validation/commit. Cookie/password material is absent from operation DTOs. Browser login is bounded to ten minutes, cancellable, fenced against replacement/logout/close, and uses the existing encrypted persistence/account-retirement path. Launch/cancel require JSON and a reviewed local/native origin; macOS/Linux wails://wails is covered by source-backed origin fixtures.

A replacement-session storage failure previously retired the current lifetime before the write failed. Persistence now precedes retirement; a meaningful regression verifies storage failure preserves the old connected account and usable token. Retirement failure restores the prior persisted record and creates a fresh lifetime for the retained provider. Renderer polling handles failure, cancels on unmount, and rejects late connection results after renderer generation changes.

New API/renderer tests passed. Opt-in real Chromium fixtures passed HttpOnly cookie capture and profile removal after success/cancellation (VIIB_TEST_LOGIN_BROWSER=1). Actual production browser UI opened a visible Spotify login window; the no-credentials attempt timed out with a usable retry state and profile removal. A second real UI attempt was canceled through Cancel sign-in and its profile was removed. The no-cookie screenshot output/playwright/spotify-browser-login.png was visually inspected. Successful credentials/MFA/SSO login remains unverified and requires the user completing Spotify's own login; no password was requested in chat or inspected.

Live Recent again returned HTTP 429, Retry-After 51 seconds; the cooldown was honored. A separate supplied-cookie test connection used hidden stdin and backend-only encrypted storage, with no plaintext credential in source/scripts/artifacts. Retained-session live checks traversed all 63 saved albums and two large playlists (239 items/five pages; 380 items/eight pages), with stable reported revisions/totals. Private status was not established by these samples.

Saved-playlist traversal initially failed after 100 of 228 positions. Redacted full-library research diagnosed a child playlist with a present empty name. normalizePlaylist now distinguishes a missing/null name from a valid empty string, preserving the latter without fabricating a title. Regression fixtures passed. The fixed production catalog client then traversed all 63 album positions and 228 playlist positions, including four explicit unavailable playlist positions, to terminal pages. Credential-free result: output/playwright/spotify-full-library-result.json. This is full traversal for one account, not every market/account or a production-app rerun after installing the final catalog fix.

Checks: npm run check passed after the login UI changes (77 files/328 tests, types and build); full backend suite and targeted vet passed before the final empty-name/native-origin changes; catalog and research-probe tests passed after the empty-name fix. New browser-capture fixtures passed after the capture test hooks were added. Race execution remains unavailable without a CGO/GCC toolchain.

Live continuation handles: browser-mode isolated app port 34221 (tool session 31158), Playwright session spotify-expiry, and window.__spotifyExpiryContinuity are observing real album playback across the original expiry 1791085423858; poll these before changing/restarting the app/browser. The last verified observation had 80 samples, 78 advancing samples and no missing-active samples; no final expiry pass is claimed. Playback observation was explicitly started after the supplied-cookie session validation, with no explicit refresh. Original retained diagnostic app port 34179 is confirmed live. Separate login test app port 34219 (session 77131) remains live with no pending login. Native Windows app port 34223 (session 90439) built/launched and served its real Wails WebView; CDP attachment failed because the installed go-webview2 loader clears external debugging arguments. Native playback/shutdown and other-platform proof remain open; native launch alone is not those gates.


## 2026-10-03: real login, full UI traversal, active expiry and native feedback

The user completed Spotify's own sign-in twice. The first attempt reached a redacted failure after the window closed/profile removal; its underlying error was not logged, so rate limiting was not established. Browser-login errors now distinguish provider rejection, rate limiting, protocol change, transport unavailability and profile cleanup using fixed messages and credential-free stage/category diagnostics. The second attempt connected successfully without cookie pasting. Its encrypted session restored after a controlled test-process restart, and a profile request returned HTTP 200. No temporary `viib-spotify-login-*` profiles remained. Credential-free artifact: `output/playwright/spotify-real-login-result.json`.

Actual browser audio crossed the original expiry `1791085423858` continuously. The resident monitor recorded 301 samples, 295 advancing samples, five track changes, zero missing-active samples, and readyState 4 throughout the 90 seconds on either side of expiry. Metadata returned 200; token issue count advanced 1 to 2. Seeking to 60 seconds and next-track playback worked afterward, with advancing remote audio. Artifact: `output/playwright/spotify-active-expiry-result.json`. This is one account/browser sample, not a native performance result.

The user-supplied private-playlist link normalized completely: one position, one page, stable reported revision. The link was supplied in response to the private-playlist gate question; provider metadata alone does not establish privacy. This small sample does not cover mixed/episode/unavailable items. Artifact: `output/playwright/spotify-private-playlist-result.json`.

The final production API traversed 63 album positions and 228 playlist positions across two/five pages before the new private playlist appeared. The renderer previously had no saved-library pagination beyond 20. It now supports explicit Load more, preserves provider positions and duplicate resources, retains loaded cards on inconsistent totals/pages, fences late results across account/tab changes, and offers retry/refresh errors. An actual UI traversal through the browser-acquired session loaded album card counts 20/40/60/63 and playlist card counts 20/40/60/80/98/118/137/156/176/196/216/225: 229 provider playlist positions including four unavailable rows. No Load more remained. Artifact: `output/playwright/spotify-ui-pagination-result.json`.

An intervening provider response reported total zero after earlier playlist pages reported 228; the append guard rejected it and preserved cards. Later both old-cookie and new-browser-cookie instances reported 229 positions and matching account identity. Do not infer authentication rejection from this transient zero response or silently weaken pagination validation. Initial Recent/library failures now show errors rather than claiming an empty collection. A fresh-browser-cookie request to Recent still returned HTTP 429, Retry-After 35; no track-event parity is claimed.

The user tested actual Windows Wails playback, seek, next, logout and browser-login reconnect. Functional actions worked, but startup/seek/next buffering was slow and search sometimes showed no results. These remain performance/reliability gates. Native logs exposed an initial 9.64 MB range taking roughly 20 seconds and repeated large ranges. A specific installed dependency check found the Wails Windows response writer buffers bytes and calls PutByteContent only when Finish executes. Native Spotify audio now resolves the existing direct-loopback transport already used for SSE, including hidden/gapless preloads and late-retirement fences. Browser URLs remain relative. The updated native performance result still requires verification; no speculative asset-cache rewrite or partial-range semantics change was made.

Search now displays current-query error/retry state and clears prior-query results. Catalog-first publication remains. Playlist fallback now receives request cancellation through the Chromium context; removed process-global log.SetOutput(io.Discard), which previously suppressed unrelated diagnostics. Three normal production catalog searches returned HTTP 200 with nonempty requested categories in 995/701/902 ms. This bounded sample does not explain every reported empty response or prove percentile latency. Artifact: `output/playwright/spotify-search-consistency-result.json`.

The first native shutdown timed out while SSE remained open. Native HTTP requests now inherit a cancellable server context; request cancellation and once-only API retirement precede Shutdown. The user's subsequent actual native close logged workers stopping and HTTP shutdown completion in about 1 ms. Both native/browser entrypoints have an actual open-event-stream shutdown regression. Other operating systems and active enrichment shutdown remain open.

The persistent goal remains active. Remaining gates include updated native responsiveness/search verification, Recent individual events, broader market/relink/mixed/quality/repeated-transfer variants, other supported operating systems, and original-plan comparator/final requirements. Tests and runtime evidence must remain distinct.


Final checks after the transport/search changes: npm run check passed all 77 files/337 tests, TypeScript and production build; full Go suite and targeted API/Spotify/native/browser vet passed. Native open-event-stream regression passed; the real native shutdown had already passed. New fixtures prove direct native audio URL selection, rejection of a late native URL after logout, current-query failure/retry, and canceled playlist search without launching Chromium. Wails custom-scheme CORS is explicitly permitted for direct transport; other-platform runtime proof remains pending.

A production browser fixture using the same native GetServerURL resolution verified direct loopback audio, then play/seek/next at 5,742/4,584/4,652 ms. These are one browser fixture's action-to-advancing-audio samples, not actual Windows WebView timings or parity with Spotify's Web Player. They demonstrate correct transport and retained controls while indicating remaining startup/seek optimization may be needed. Artifact: `output/playwright/spotify-native-transport-fixture-result.json`. The updated native app was opened for the user to retest; its result remains pending.


Fresh restored-session profile availability was checked without recording values: id/display_name/images were present; email/product/country/followers were null. `output/playwright/spotify-profile-availability-result.json` records only booleans. These OAuth field differences remain a concrete parity limit, not a successful profile-field gate.


## Frozen reference comparator (2026-10-04)

Added `analysisbench -spotify-reference-snapshot` alongside the existing manifest/results inputs. The ordinary local-label report remains separate. Frozen references require explicit manifest recording confirmation, matching recording ID/version/audio SHA-256, normalized nullable scalars and retrieval provenance. Snapshot version, evidence class, license, label source, adapter/upstream analyzer versions and a validated canonical snapshot hash are retained. Missing/unconfirmed/unavailable coverage is explicit; absent provider values do not become expected-unknown labels. Changed recording/fingerprint/version, foreign/duplicate IDs, tampering, incomplete provenance, invalid scalars and mismatched evidence classes fail closed. The loader is bounded, rejects unknown fields/trailing JSON, and performs no network/audio I/O.

Regression checks cover deterministic reruns, local-label preservation, half/double BPM, relative major/minor and same-mode harmonic neighbors, absent scalar coverage and invalid inputs. Corrected the shared harmonic compatibility helper: relative major/minor matching now respects which member is minor, rather than accepting either pitch-class direction. C major/A minor passes; C major/D# minor is rejected in both orders.

A synthetic one-recording CLI run is retained in ignored `output/playwright/spotify-reference-offline/`: local 60 BPM/A minor remains correct against the local labels, while the separate synthetic 120 BPM/C major reference reports the half-time difference and relative key match. This is regression evidence only (`synthetic-ci`), not a real Spotify corpus or real-audio gate pass. Confirmed current-file database/cache export and independently reviewed real-audio evidence remain open. The contract and rerun command are documented in `SPOTIFY_REFERENCE_BENCHMARK.md`.

Validation: `go test ./... -count=1 -timeout=240s` passed. After the final optional manifest-identity validation change, focused analysisbench/command tests and `go vet` passed. Two actual CLI reruns produced identical JSON; assertions verified local strict BPM success, separate reference half/double error, relative-key count, confirmed coverage and non-ready synthetic evidence.


## Read-only reference cache export (2026-10-04)

Added `cmd/spotifyreferenceexport` to create a non-overwriting frozen snapshot from an existing identity-bound corpus manifest and the application's normalized SQLite reference cache. It performs no provider request, database migration, credential initialization, identity confirmation or cache mutation. The DB opener uses a consistent read-only transaction and rejects absent/incomplete schemas; fixtures demonstrate rejected row/schema writes, unchanged closed-database bytes and a retained read snapshot while a writer adds a song.

The exporter requires current manual recording confirmation, matching manifest recording ID/version and a separately recomputed full-container SHA-256. The cheap source revision is checked before/after hashing; same-size/mtime byte changes are independently detected. Corpus/DB path ambiguity, absent local source, stale confirmation, missing/expired cache and newer provider failures are counted and omitted. Endpoint selection is explicit; mixed adapter/analyzer provenance fails before output creation. Cache-derived endpoint/schema/retrieval provenance and nullable BPM/key/mode/confidence are retained. Partial key/mode fields now remain valid frozen evidence but cannot produce exact key/mode labels. No defaults are invented.

Focused analysisbench, DB and export-command tests pass, as does `go vet`. The full `go test ./... -count=1 -timeout=240s` passed before the final partial-key/mode handling change; affected packages were rerun successfully afterward. Tests exercise an actual temporary SQLite library and command runner, load the exported snapshot through the strict comparator loader, verify deterministic reruns, preserve local labels, reject identity/provenance contradictions, and verify an existing output is untouched and an empty export creates no file. Fixtures use generated container bytes and normalized synthetic scalars only; this does not establish decoder quality or a real-audio comparison gate. Native retest remains pending, with its existing process confirmed alive during this work.


## Stream asset reuse and byte ranges (2026-10-04)

Direct-loopback native transport removed Windows asset-server whole-body buffering, but each HTTP range still repeated librespot metadata/audio-key/asset startup and the first chunk. Added a Streamer-owned asset pool keyed by track/quality/session generation, retaining fresh independent readers and the existing per-request capacity/session leases. Concurrent preparation is coalesced. Idle assets hold no session lease. Reuse is bounded to three entries, 32 MiB aggregate known asset size, 16 MiB per reusable asset and 30 seconds idle time; oversized/budget-exceeding active assets close after their readers drain. Generation changes, failures, eviction, logout and streamer shutdown retire resources. The dependency's nominal resident limit is not relied on as an eviction guarantee. Total size is discovered once during preparation and reused by the HTTP handler.

Fixed Range handling: bytes=0-0 is now exactly one byte, suffix ranges are supported, explicit/open ranges are bounded, and invalid/multiple/unsatisfiable ranges return 416 with bytes */total. They no longer silently expand to full content. Unit checks cover preparation sharing, independent positions, canceled waiters, quality/generation isolation, active reader lifetimes, entry/byte limits, TTL, errors, late preparation after retirement and range edge cases. The pool checks passed 100 repeated runs. Full Go suite, focused Spotify/API suites, go vet and browser/native builds pass. A final additional active-reader byte-budget test was rerun in the focused suite.

Matched bounded production reads on the same isolated browser instance/recording before and after the rebuild are retained in output/playwright/spotify-stream-cache-before.json and spotify-stream-cache-after.json. Before: 4,096-byte reads took 433 ms cold, 116 ms repeated, and 232 ms at byte 1,000,000. After: 935 ms cold, 4 ms repeated, and 83 ms at byte 1,000,000, with matching audio hashes and Content-Range. Cold time did not improve in this sample. One-byte and suffix reads returned exactly 1 and 4,096 bytes; an out-of-bounds range returned 416. These are one-recording samples, not percentiles or universal latency guarantees.

The production browser fixture using the native direct-URL resolver now measured play 1,190 ms, seek 1,058 ms and next 1,007 ms, including the fixture's requirement for at least half a second of audio advancement. Earlier same-fixture readings were 5,742/4,584/4,652 ms at a different time; network conditions and cache warmth differ, so the change alone cannot be credited with that entire difference. This is browser evidence, not actual Windows WebView responsiveness. Its two console errors are fixture-server POST/listen-event 501 failures; audio controls passed. The result is retained in spotify-stream-cache-ui-result.txt.

Normal-app explicit Web API refresh completed HTTP 200 in 216 ms with token issue count 1 -> 2. Audio range reads before/after succeeded with matching bytes (196 ms/3 ms). This proves refresh and continued range access, not a playback-session generation reset or another natural-expiry cycle. Generation isolation and idle lease behavior retain fixture coverage.

The isolated native executable was rebuilt/replaced at the same path and reopened at 08:33 Central, preserving its encrypted sign-in. Auth status is connected. Its test-copy startup automatically began analysis of 3,388 local tracks; the isolated queue was paused and that automatic job was canceled (terminal canceled verified) to avoid CPU contention during the pending playback/search retest. This does not alter the user's main application database. The native window now includes both direct HTTP transport and asset reuse. Its responsiveness/search result is still awaiting the user; no native UX gate is declared passed.


## Fresh grouped transfer/conversion retention (2026-10-04)

One normal-app Recent request after the previous cooldown still returned HTTP 429 with Retry-After 32 seconds while the restored WebPlayer session was connected. No immediate retry was made and no event/cursor parity was inferred. Count-only evidence is retained in output/playwright/spotify-recent-after-cache-result.json.

Queued the full user-selected one-track private playlist twice through the production playlist API on the isolated authenticated browser app: fresh Ogg and fresh Ogg-to-MP3 passes, each in a new empty immediate-child Temp root with the existing viib-cookie-app-* verifier layout. Queue ID differences and final paths prove these were new jobs in distinct empty destinations, rather than completed-row or valid-file reuse. Concurrency was limited to one. Ogg completed at 100% in 10,067 ms; MP3 completed at 100% in 14,063 ms. Both matched the expected recording, retained sidecar artwork and imported into the local library; the MP3 pass removed its intermediate Ogg.

Independent whole-file decode passed: Ogg 9,005,143 bytes, stereo 44.1 kHz, declared/decoded 9,102,240 frames, 206.4 seconds. MP3 8,256,924 bytes, stereo 44.1 kHz, 9,103,104 decoded frames, 206.41959 seconds. Independent tag reads verified exact title/artist match against the queue metadata, playlist Album/AlbumArtist, track order 1, disc 1 and numbered filenames. Both use sidecar artwork; neither has embedded artwork, so embedded-cover variants are not claimed. Current-source DB reads verified automatic download-completion recording links and library numbering for both formats.

The initial read-only verifier invocations stopped at track_input because the CLI parsed a recording ID before entering offline verification. Moved the offline branch ahead of recording parsing and protocol/authentication self-test. Explicit enable, mode exclusivity, path confinement, symlink checks, file count/size/time limits and full decoding remain in force. A regression invokes offline mode without a track ID and proves it reaches the corrupt-audio rejection. Research-tagged probe tests and rebuild pass; actual Ogg/MP3 verifier runs now pass without cookie or track input.

Cleared only the two completed test queue rows after checking that the isolated app had no other completed history. Both media files remained. Same-path app restart restored cookie sign-in, both library songs and both current-source recording links after queue cleanup; media SHA-256 values were unchanged. The new fresh Ogg/conversion/link-retention sample extends the previous retained-artifact sample, but does not establish repeated multi-track stress, account/market/quality or all metadata variants.

Settings cleanup restored the prior download root and conversion setting. The previous concurrency setting was empty; the setting API rejects an empty concurrency value, so the effective default 3 was restored explicitly. The script now normalizes that default before cleanup. This change is confined to the isolated test database. No user-main-library setting was touched. Temporary audio is retained for review; the verifier and metadata/link checks are read-only. The task-owned browser fixture was closed after checks to stop its background metadata polling. The native test window remains open for the pending responsiveness/search retest.

Artifacts: spotify-repeat-downloads-result.json, spotify-repeat-ogg-decode.json, spotify-repeat-mp3-decode.json, spotify-repeat-metadata-link-result.json, spotify-repeat-queue-cleanup-result.json, spotify-repeat-restart-link-result.json and spotify-repeat-restart-result.json under ignored output/playwright. Auxiliary state contains test paths/queue IDs/recording IDs, never credentials.


## Missing profile fields: wire evidence (2026-10-04)

A targeted source investigation confirmed the pinned librespot SessionInfo exposes username/auth blob/country only. APWelcome's Spotify account-info protobuf has no email, plan or follower fields. PacketProductInfo is received but discarded; its comment about XML product/A-B configuration is not a verified Premium/free accessor. Country is assigned asynchronously without a safe synchronized accessor. No production field was inferred from bitrate, device capability or artist metadata, and profile reads do not initialize playback.

Added a research-only observer around the already-fixed profileAttributes operation. It retains only fixed email/product/country/followers field types and aggregate me/profile field counts; no scalar values, arbitrary field names, credentials or raw response bytes are written. Fixtures prove the downstream decoder sees the unchanged body and private values/unknown keys cannot escape the report. Unrelated operations are ignored. Full research-tagged probe tests and build pass.

The supplied test cookie authenticated and explicitly renewed, then the fixed operation normalized successfully. Live wire shape: data.me had one field; its profile had five fields. Email/product/country/followers and followers.total were absent at both checked levels. The adapter is capable of ignoring unknown JSON fields in general, but this sample proves those four null DTO fields were not caused by silently dropping present wire fields. This does not establish absence across all accounts or all other Spotify operations. Artifact: output/playwright/spotify-profile-wire-result.json; only types/counts/availability are retained.

One separate, bounded direct documented GET /v1/me was attempted with the WebPlayer token after the previous cooldown. Authentication succeeded; exactly one profile request returned HTTP 429/Retry-After 9 seconds, and the probe stopped. No immediate retry, fallback or production change was made. Direct profile compatibility remains inconclusive, not a successful full-profile gate. Artifact: spotify-profile-direct-result.json. The missing-field implementation gap still requires a verified accessible source. The initial local wrapper used an incorrect CLI flag and failed before provider dispatch; it was corrected to --track-id before these successful diagnostics.


## 2026-10-04: enrichment and reference shutdown ownership

The original-plan audit found that detached mood/year and Last.fm jobs could continue after API shutdown, and unified AI workers could outlive their SSE handler. API enrichment admission now shares a mutex with shutdown, every admitted job has application cancellation and a drain counter, and shutdown rejects new admission. Detached jobs retain their existing behavior across client disconnects but stop on application shutdown. Unified workers and the keepalive writer drain before the response handler returns; canceled provider results are discarded before persistence. Provider cleanup completes before the job releases its drain registration. Last.fm checks cancellation before applying fetched data and only records last-sync time after a successful uncanceled run.

The optional Spotify reference service retains nonblocking Disconnect fencing/cancellation. Close now additionally waits for provider operations to drain, including work retired by an earlier Disconnect. Blocking-provider fixtures verify cancellation, waiting for cleanup, rejected admission, repeated cleanup/Close, unchanged metadata and absence of late reference-cache commits. A real local HTTP provider fixture verifies the unified handler's cancellation path and a detached mood job's client-disconnect/application-shutdown distinction. The first HTTP fixture stalled because it did not consume the request body; after correcting the fixture, the bounded tests passed.

The full backend Go suite passed after API ownership changes. Focused API/Last.fm/reference tests and vet were run after reference draining was added. These are local fixture results. A Windows native shutdown while enrichment is active is still unverified, and this does not close Recent/profile parity, real-corpus benchmarking or other-platform interactive gates. The parity audit now maps the original-plan requirements to evidence and their remaining limits.


## 2026-10-04: generic proxy and credential-export regression gates

Source inspection confirmed generic settings already exclude the dedicated session key, but no explicit route regression test existed. New GET/POST fixtures verify rejection, unchanged stored session and successful normal theme read/write. Support-log redaction now recognizes Spotify cookie names (`spDC`/`sp_dc`, `sp_key`), cookie headers and the session-setting name. Actual support ZIP fixtures verify stored session values and cookie examples do not appear in any exported entry.

New backup creation sanitizes only its consistent temporary database copy, removing Spotify cookie-session and OAuth-credential settings before hashing and archiving. SQLite secure_delete and VACUUM remove the deleted bytes too. The actual backup ZIP fixture verifies absent credential rows, absent original encrypted cookie bytes/raw fixture values, preserved ordinary settings and unchanged live credentials. The backup documentation now explains that new archives require Spotify sign-in after restore, while older archives and other integrations keep their previous behavior.

Generic Spotify proxy fixtures cover oversized input (413 without dispatch), bounded malformed/empty/trailing/oversized successful response rejection (502), unchanged valid JSON, bodyless 204, redacted 403/404/429 with preserved Retry-After and a still-connected cookie session, and cancellation of an active HTTP request (503 without secret details). The handler caps request input at 2 MiB and successful response JSON at 8 MiB. These local failure-contract tests do not claim live write-route parity. Focused tests passed; full Go tests, vet and diff checks were run for this increment.


## 2026-10-04: cross-platform core compilation and browser discovery

After the credential-export/proxy increment, RepoTracer checked platform closures and found that backend internal packages compile with CGO disabled across Windows amd64/arm64, Linux amd64 and macOS amd64/arm64. The root repeated those five backend checks after the browser-discovery update; all passed, with explicit target/scope/CGO/result records in `output/playwright/spotify-platform-compile.json`. Those are compiler results, not host execution or native packaging results. RepoTracer's plain Wails-package checks do not replace production GUI packaging; the Linux CGO-free native closure fails on systray's native dependencies. Current CI already defines Linux/macOS host package jobs with GTK/WebKit/appindicator dependencies for Linux. Those jobs were inspected but not run on this uncommitted worktree. Wails CLI and GCC are absent from this host PATH.

The discovery helper now adds macOS per-user Applications bundles and Linux stable Chrome/Edge `/opt` binaries after existing PATH candidates. Those Linux stable-channel locations match [Microsoft's Playwright registry](https://github.com/microsoft/playwright/blob/main/packages/playwright-core/src/server/registry/index.ts), checked today. Platform fixtures verify found/missing cases and existing PATH selection priority without trusting renderer-supplied executable paths. Focused auth/API/Wails tests and auth vet pass. The opt-in real Windows Chromium capture fixture also passes both synthetic secure HttpOnly-cookie capture and cancellation/profile cleanup (about:blank, no live Spotify request). Other-platform interactive Spotify sign-in, playback/reconnect and active shutdown remain unverified.


## 2026-10-04: remaining external gates rechecked

At 14:55 UTC, the retained browser-test application's auth status returned HTTP 200 with provider webplayer, connected true and authRequired false. One fresh normal application Recent request returned HTTP 429, Retry-After 27; no retry or alternate history endpoint was attempted. This is not individual play-event parity and does not justify replacing unavailable events with context history.

The updated asset-cache native test executable was absent from the process inventory. Its exact retained session completed with exit code 0 and a completed HTTP-shutdown message. This adds actual clean-closure evidence for that native build, but does not establish buffering/search responsiveness or shutdown during active enrichment. No new window was launched. Safe typed recheck results are retained in `output/playwright/spotify-remaining-gate-recheck.json`.

The remaining completion requirements need external evidence: native performance/search feedback, provider support for Recent and missing profile fields, a reviewed recording-identity/audio-license corpus manifest for the real benchmark, and macOS/Linux runtime hosts. The user was asked for the concrete feedback, corpus path and available test machines. Passing local compiler/fixture checks do not close these requirements.
