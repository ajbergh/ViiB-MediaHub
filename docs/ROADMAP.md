# ViiB MediaHub — Product & Engineering Roadmap

> **Status:** Active / living plan  
> **Last reviewed:** 2026-10-20<br>
> **Target:** Reliable, secure, supportable **ViiB MediaHub 1.0** followed by qualified professional-DJ improvements  
> **Source of truth:** This file owns cross-project priorities and release gates. Domain-specific plans and dated evidence remain authoritative for the technical detail they cover.

## 1. Baseline and scope

ViiB MediaHub is a local-first music hub: React 19 / TypeScript frontend, Go / SQLite backend, Wails desktop builds, browser/server mode, a unified local/Plex music catalog, separate Spotify integration, AI-assisted library intelligence, and two-deck DJ functionality.

**Historical repository checkpoint (2026-10-15, local `main` at `08322ba`):**

- Since the October 13 checkpoint, `main` includes API trust-boundary hardening ([#118](https://github.com/ajbergh/ViiB-MediaHub/pull/118)), versioned credential storage ([#119](https://github.com/ajbergh/ViiB-MediaHub/pull/119)), media capabilities ([#120](https://github.com/ajbergh/ViiB-MediaHub/pull/120)), FLAC/M4A import and unsupported-analysis handling ([#121](https://github.com/ajbergh/ViiB-MediaHub/pull/121)), UI/library-performance and Spotify surface work ([#122](https://github.com/ajbergh/ViiB-MediaHub/pull/122)), scan/analysis stability and storage work ([#123](https://github.com/ajbergh/ViiB-MediaHub/pull/123)), and managed local semantic matching plus Spotify account-plan gates ([#124](https://github.com/ajbergh/ViiB-MediaHub/pull/124)). The latest checkpoint commit is [`08322ba`](https://github.com/ajbergh/ViiB-MediaHub/commit/08322ba).
- **October 20 refresh:** local/hosted `main` is now `c50e2b4`, including MIDI/recovery work ([#128](https://github.com/ajbergh/ViiB-MediaHub/pull/128)), experimental controller presets ([#129](https://github.com/ajbergh/ViiB-MediaHub/pull/129)), and four-channel routing ([#130](https://github.com/ajbergh/ViiB-MediaHub/pull/130)). Implementation does not qualify Phase 5 hardware/accuracy gates.
- Hosted [CI for `c50e2b4`](https://github.com/ajbergh/ViiB-MediaHub/actions/runs/39435304421) passed; the preceding [CI for `41b9d4e`](https://github.com/ajbergh/ViiB-MediaHub/actions/runs/39435237378) also passed. Passing CI alone does not establish real-device, provider, musical-accuracy, or release qualification.
- Latest hosted release: [`v1.0.0-rc7`](https://github.com/ajbergh/ViiB-MediaHub/releases/tag/v1.0.0-rc7), published October 17 America/Chicago (October 18 UTC). Local tags include RC7. **Stable 1.0 is not declared complete.**
- GitHub reports `main` unprotected and no repository rulesets on October 20. No open PRs existed before this branch. Suggested follow-up titles below are not a live issue inventory.
- The repository contains extensive Go and frontend tests, native build workflows, analysis determinism checks, vulnerability scans, and developer documentation. Preserve those investments.
- **Release-pipeline baseline (from [`release.yml`](../.github/workflows/release.yml)):** the workflow builds a Windows x64 installer, macOS arm64/x64 DMGs, a Linux amd64 `.deb`/tarball, and web builds for Windows/macOS/Linux on x64 and arm64; it publishes `SHA256SUMS.txt` and marks RC tags as pre-releases. macOS bundles are **ad-hoc signed and not notarized**; Windows Authenticode signing runs **only when repository secrets exist** and otherwise continues unsigned. The October 20 branch adds an npm CI SBOM; release-artifact SBOMs and provenance remain open.
- **Proposed tooling update (October 20):** a deterministic Chromium browser-only suite/config and CI job are implemented on `codex/roadmap-easy-foundations`. Strict TypeScript coverage expands from four to twelve selected files. Local checks pass; PR/merged-head acceptance remains open.

**Important continuity decision:** The owner previously deferred further local DSP estimator research from the Spotify metadata branch. PR #117 is merged; deferred work belongs in a **separate future scope** (Phase 5), not retroactively in that PR or as an undisclosed 1.0 dependency. Subsequent scan lifecycle/storage fixes (#123) and local semantic matching (#124) are delivered maintenance/product work; they do not close deferred estimator accuracy gates. See [Spotify scope](SPOTIFY_BRANCH_REMAINING_WORK.md), [closeout](SPOTIFY_BRANCH_CLOSEOUT_NEXT_STEPS.md), and [deferred local analysis](LOCAL_ANALYSIS_FUTURE_BRANCH.md).

### Outcome priorities

1. **P0: prevent security, credential, data-loss, and release-trust failures.**
2. **P1: make playback, catalog/source behavior, codec support, and recovery predictable.**
3. **P1: qualify performance and user-critical workflows against reproducible evidence.**
4. **P2: reduce coupling and modernize UX without disrupting the shipping application.**
5. **P2 / post-stabilization: expand professional DJ capability only after accuracy and platform qualification.**

### Status vocabulary

- **Baseline delivered:** already present or merged; listed as context, not new scope.
- **Planned:** accepted candidate work, not yet implemented or validated.
- **Partial:** useful implementation slices are present; the row names the remaining scope or acceptance evidence.
- **In progress:** active work is linked to an issue/branch/PR and evidence. A merged implementation alone is recorded as delivered or partial.
- **Blocked:** dependency or external constraint named explicitly.
- **Qualified:** acceptance checks and evidence links recorded.
- **Deferred:** intentionally excluded from the current release scope.

Do not promote **Planned** to **Qualified** solely because a PR merged or CI passed.

## 2. Roadmap at a glance

| Phase | Priority | Goal | Depends on | Status | Release role |
| --- | --- | --- | --- | --- | --- |
| **0 — Security & release foundations** | P0 | Harden the local privileged API, credential storage, repository governance, and release authenticity | Current baseline | **Partial** | Required for stable 1.0 |
| **1 — Playback, media format & source reliability** | P1 | Eliminate inconsistent formats and strengthen local/Plex/Spotify failure behavior | Phase 0 security interfaces | **Partial** | Required for stable 1.0 |
| **2 — Performance & product-quality evidence** | P1 | Benchmark real workloads and automate critical user journeys | Instrumentation from Phases 0–1 | **Partial** | Required for stable 1.0 |
| **3 — Architecture & UX modernization** | P1/P2 | Improve maintainability, accessibility, and workflow clarity | Stable contracts from Phases 0–2 | **Partial** | Critical fixes before 1.0; larger refactors incremental |
| **4 — Stable 1.0 qualification** | P0 gate | Demonstrate signed, recoverable, usable builds across supported platforms | Phases 0–3 release-critical items | **Planned** | Final release gate |
| **5 — Professional DJ & advanced analysis** | P2 | Independently qualify musical accuracy and transport/device behavior | Separate approved future scope | **Deferred** | Not a blanket 1.0 blocker |

Work packages use stable IDs (`P0.1` etc.) for GitHub issues, PR titles, and progress reports. The number before the dot is the **phase**, not the priority: `P2.1` is Phase 2 work (priority P1), and priority labels (`priority/P0`–`P2`) are tracked separately. Dependencies should be rechecked before starting each item.

## 3. Phase 0 — Security and release foundations (P0)

**Current position:** The privileged loopback service now has centralized Host/Origin/CORS/SSE checks and regression tests. Credential storage uses `enc:v2` with an installation key, Windows DPAPI protection, and a documented non-Windows keywrap fallback. The former wildcard SSE headers and executable-path-dependent encryption are legacy/migration concerns, not the current default implementation. Threat-model review, target-host migration/relocation evidence, governance enforcement, and production release signing remain open. The non-Windows fallback is not Keychain or Linux secret-service integration.

| ID | Work package | Status | Deliverables | Definition of done |
| --- | --- | --- | --- | --- |
| **P0.1** | Loopback API trust boundary | **Baseline delivered; qualification open** ([PR #118](https://github.com/ajbergh/ViiB-MediaHub/pull/118)) | Centralized Host/Origin/CORS/SSE policy and negative/positive tests merged. Remaining: threat-model review, privileged-mutation/CSRF acceptance and target-client qualification | Negative tests for attacker-controlled origins/hosts (including DNS-rebinding-style `Host` values) and mutating requests, positive tests for approved desktop/browser clients, consistent SSE response headers; threat model reviewed |
| **P0.2** | Versioned credential key storage | **Baseline delivered; qualification open** ([PR #119](https://github.com/ajbergh/ViiB-MediaHub/pull/119)) | `enc:v2`, Windows DPAPI, non-Windows keywrap fallback, readable legacy migration and reauthentication handling merged. Remaining: supported-host install/relocation/upgrade/restart and rollback qualification | Install/relocate/upgrade/restart tests on supported OSes; readable old secrets migrate without disclosure or silent loss; unresolvable legacy secrets report a clear reauthentication path |
| **P0.3** | Governance and required checks | **Planned** | Protect `main` via branch ruleset; require frontend, Go test/race/vet/staticcheck, vulnerability and packaging checks; document emergency bypass | Normal PR cannot merge without passing required checks; explicit reviewed emergency process exists |
| **P0.4** | Release signing and provenance | **Planned** | Stable-release signing policy; macOS Developer ID + notarization, Windows Authenticode, checksums and verifiable build/source provenance | Production (non-RC) release workflow fails closed when mandatory signing/verification is unavailable (today Windows signing is skipped without secrets and macOS is ad-hoc signed only); install and signature-verification smoke tests pass |
| **P0.5** | Supply-chain and licensing review | **Partial; proposed implementation** | Exact Staticcheck/govulncheck pins across workflows and npm CI CycloneDX inventory. Remaining: hosted validation, Go/release SBOMs, attribution/license review and shipped advisory acceptance | License obligations and security advisories are recorded; no unaccepted shipped critical/high vulnerability; reproducible dependency inputs |

**Progress & Evidence Log (Phase 0):**
- **2026-10-13 (`P0.1`, [PR #118](https://github.com/ajbergh/ViiB-MediaHub/pull/118), branch `codex/p0-1-loopback-api-trust-boundary`):** Implemented centralized `loopbackTrustBoundary` middleware (`IsTrustedHost`, `IsTrustedOrigin`, and `Sec-Fetch-Site: cross-site` enforcement) in [`backend/internal/server/server.go`](../backend/internal/server/server.go), removed wildcard `Access-Control-Allow-Origin: *` overrides across all five legacy SSE handlers in [`backend/internal/api/api.go`](../backend/internal/api/api.go), and added negative/positive unit and handler tests. Merged to `main`; remaining acceptance evidence is tracked in the row above.
- **2026-10-13 (`P0.2`, [PR #119](https://github.com/ajbergh/ViiB-MediaHub/pull/119), branch `codex/p0-2-versioned-credential-key-storage`):** Implemented `enc:v2:` credential encryption in [`backend/internal/crypto/crypto.go`](../backend/internal/crypto/crypto.go) using a per-installation 256-bit master key (`credentials.key`, mode `0600`) protected via Windows DPAPI ([`backend/internal/crypto/keyprotect_windows.go`](../backend/internal/crypto/keyprotect_windows.go)) with a relocation-safe AES-256-GCM keywrap fallback on non-Windows hosts ([`backend/internal/crypto/keyprotect_other.go`](../backend/internal/crypto/keyprotect_other.go)). Updated [`backend/internal/db/db.go`](../backend/internal/db/db.go) to migrate all `crypto.SensitiveKeys()` (including `plex_credentials` and `semantic_embedding_api_key`) from plaintext/readable `enc:v1:` to `enc:v2:`, and added `crypto.ErrReauthRequired` handling for unreadable legacy secrets. The later managed semantic service ignores legacy embedding settings/keys; cloud embedding-provider setup is no longer current scope. Merged to `main`; remaining acceptance evidence is tracked in the row above.

**Phase exit:** P0.1/P0.2 data and threat-model regressions green on target platforms; repository policy active; release signing path demonstrated with staged artifacts. Certificate, Developer ID and notarization procurement has external lead time, so start it in Wave 1. Preserve the current security patches and binary vulnerability scans.

**Primary files:** [server middleware](../backend/internal/server/server.go), [API/SSE handlers](../backend/internal/api/api.go), [credential crypto](../backend/internal/crypto/crypto.go), [CI](../.github/workflows/ci.yml), [release workflow](../.github/workflows/release.yml).

## 4. Phase 1 — Playback, formats, and source reliability (P1)

**Current position:** Canonical capabilities, browser FLAC/M4A import filters, fixtures, and explicit unsupported DJ-analysis states are merged. Import acceptance still does not imply backend waveform/analysis support; browser/WebView playback remains decoder-dependent. The existing [format audit](AUDIO_FORMAT_SUPPORT_GAPS.md) is the starting contract, not a claim of parity. Spotify parity and cross-platform behavior also retain documented gaps.

| ID | Work package | Status | Deliverables | Definition of done |
| --- | --- | --- | --- | --- |
| **P1.1** | Single media-capability matrix | **Baseline delivered; qualification open** ([PR #120](https://github.com/ajbergh/ViiB-MediaHub/pull/120)) | Canonical format/source inventory, capability endpoint and structured errors merged. Remaining: verify advertised import/playback/waveform/analysis behavior across target runtimes and sources | MP3, OGG Vorbis, FLAC, M4A/AAC, WAV, OPUS and other advertised formats have real fixtures or explicit unsupported state; documentation matches behavior |
| **P1.2** | FLAC/M4A import and analysis policy | **Baseline delivered; qualification open** ([PR #121](https://github.com/ajbergh/ViiB-MediaHub/pull/121)) | Browser FLAC/M4A import filters, metadata fixtures and explicit unsupported-analysis fallback merged. Remaining: native/browser runtime acceptance for metadata/artwork/length and notices | Native/browser import fixtures pass; tracks never silently claim BPM/key/waveform support unavailable in that runtime |
| **P1.3** | Playback state transitions | **Partial** | Spotify retirement/local-playback preservation and range/lifecycle regressions exist; the combined local → Plex → Spotify matrix, interruptions, crossfade, and device-change qualification remain open.| Repeatable tests cover local → Plex → Spotify and back, logout during playback, range seeking, network interruption, and no unrelated local-playback teardown |
| **P1.4** | Spotify parity and resilience | **Partial** | Cookie sign-in/session renewal, browsing, Premium media gates, integrity retries, and reconnect flows are implemented. Native responsiveness/search, other-host execution, Recently Played/profile parity, broader markets/accounts, and compatibility review remain open.| Contract/fixture cases pass; at least one real provider flow per supported host passes where feasible; unverified account/market cases remain visibly marked; no fabricated metadata |
| **P1.5** | Plex large-library and offline safety | **Partial** | Music-only synchronization, offline catalog retention, authenticated audio/artwork proxying, and tests exist. Real-PMS interrupted/large-library/auth-expiry/codec qualification and fallback policy remain open.| Interrupted sync never mass-deletes valid catalog rows; auth expiry/offline conditions recover; real-PMS smoke cases recorded; unsupported codecs clearly explained |
| **P1.6** | Data and media recovery | **Partial** | Library Operations backup/staged restore, duplicate review, restart/idempotency and source-bound retention exist. Full RC5 upgrade and backup/restore continuity evidence remains open.| Restoring a tested backup preserves library/playlists/likes/history and consistent source links; unrecoverable changes are disclosed before destructive actions |

**Progress & Evidence Log (Phase 1):**
- **2026-10-13 (`P1.1`, [PR #120](https://github.com/ajbergh/ViiB-MediaHub/pull/120), branch `codex/p1-1-media-capability-matrix`):** Added canonical per-format/per-source capability inventory and reusable `CapabilityError` types in [`backend/internal/audio/capabilities.go`](../backend/internal/audio/capabilities.go) and [`services/mediaCapabilities.ts`](../services/mediaCapabilities.ts), wired scanner extension allowlist to `audio.ScannerSupportedExtensions()`, exposed `GET /api/v2/capabilities/media`, attached structured capability metadata to `GET /api/dj/waveform/{id}` `422` responses, and added cross-format metadata/decoder alignment tests in [`backend/internal/audio/capabilities_test.go`](../backend/internal/audio/capabilities_test.go), [`backend/internal/api/media_capabilities_test.go`](../backend/internal/api/media_capabilities_test.go), and [`services/mediaCapabilities.test.ts`](../services/mediaCapabilities.test.ts). Merged to `main`; remaining acceptance evidence is tracked in the row above.
- **2026-10-13 (`P1.2`, [PR #121](https://github.com/ajbergh/ViiB-MediaHub/pull/121), branch `codex/p1-2-flac-m4a-import-analysis-policy`):** Unified browser-only directory and legacy folder import filters in [`pages/Settings.tsx`](../pages/Settings.tsx) with `isBrowserImportSupportedFile` (`.mp3`, `.ogg`, `.oga`, `.flac`, `.m4a`) from [`services/mediaCapabilities.ts`](../services/mediaCapabilities.ts); added explicit `unsupported` deck analysis status and UI notices across [`hooks/useDJAudioEngine.ts`](../hooks/useDJAudioEngine.ts), [`components/dj/v2/DJDeckFacts.tsx`](../components/dj/v2/DJDeckFacts.tsx), [`components/dj/v2/DJDeckComponents.tsx`](../components/dj/v2/DJDeckComponents.tsx), [`components/dj/v2/DJEnergyInsights.tsx`](../components/dj/v2/DJEnergyInsights.tsx), and [`components/SongAudioMetadata.tsx`](../components/SongAudioMetadata.tsx); and added multi-format browser import and unsupported-analysis tests. Merged to `main`; remaining acceptance evidence is tracked in the row above.

**Phase exit:** supported format matrix is factual; no known data-loss source-transition defect; critical playback and migration tests pass across Wails/browser profiles. Codec playback/analysis support must not be generalized from filename acceptance.

**Primary references:** [format gaps](AUDIO_FORMAT_SUPPORT_GAPS.md), [Spotify parity](SPOTIFY_COOKIE_AUTH_PARITY_AUDIT.md), [Plex contract](plex-music.md), [API contract](openapi-v2.yaml).

## 5. Phase 2 — Performance, tests, and observability (P1)

The current [preparation baseline](SPOTIFY_PREPARATION_PERFORMANCE.md) includes a **105-track offline Windows** experiment (Milestone 259 run): ~759 seconds fresh, ~1.5 seconds warm, about 100 MB of stored artifacts and a ~71 MiB process peak working set. It is not a connected-provider, native UI or multi-platform latency qualification. Avoid making performance promises based on it alone.

| ID | Work package | Status | Current scope and deliverables | Definition of done |
| --- | --- | --- | --- | --- |
| **P2.1** | Representative benchmark harness | **Planned** | The 105-track offline preparation experiment and library performance changes are useful baselines; no representative 1K/10K/100K percentile/resource reporting harness is established.| CI or scheduled reports provide p50/p95/p99 where applicable, memory/CPU/disk/artifact totals, environment and reproducible inputs |
| **P2.2** | Bounded analysis and scanning | **Partial** | Atomic analysis claims, bounded scanner TagLib execution, timeout/retry ownership, and cancellation/restart tests are delivered in #123. Workload-level decode/concurrency/resource measurements remain open.| No unbounded worker/provider storms; warm runs avoid unnecessary decodes; measured improvements do not regress accuracy or provenance |
| **P2.3** | Cache lifecycle | **Partial** | Binary three-band storage and bounded legacy compaction are delivered in #123. Global quotas, safe eviction, disk-budget enforcement, and WAL/restoration qualification remain open.| Predictable disk limits; cache eviction cannot destroy user data or invalidate retained provenance; reference workloads stay inside approved resource budgets |
| **P2.4** | Browser end-to-end gates | **Partial; proposed implementation** | Three deterministic Chromium browser-only tests, config and CI job with failure screenshots/traces/report. Remaining: hosted validation, backend/import/playback/recovery/provider flows and other browser profiles.| Critical flows reliably pass in CI; failures retain screenshots/traces/logs without secrets |
| **P2.5** | Native system qualification | **Partial** | Native builds and bounded Windows user-reported functional samples exist. Long-session/device/WebView coverage and macOS/Linux host execution remain open.| Tested platform/architecture matrix with explicit passes, failures and accepted exceptions |
| **P2.6** | Troubleshooting evidence | **Partial** | Request IDs, job diagnostics and an opt-in bounded/redacted support archive with tests exist. End-to-end failure correlation, broader log/privacy review, and troubleshooting qualification remain open.| A user-visible failure can be traced to a sanitized diagnostic record; tokens, cookies, local-private data are excluded by default |

**Progress & Evidence Log (Phase 2, October 15):**
- [PR #123](https://github.com/ajbergh/ViiB-MediaHub/pull/123) (`79a04eb`) delivers the bounded analysis/scanner and binary waveform conversion described in [scan stability and storage](scan-stability-and-storage.md). Compaction reuses SQLite pages; it does not automatically shrink the database file. TagLib cannot be forcibly canceled; fully stuck slots require restart.
- The existing [support archive handler](../backend/internal/api/support_bundle.go), [redaction/bounds tests](../backend/internal/api/support_bundle_test.go), and **Settings → System & Logs → Create Support Bundle** implement a bounded export of diagnostics and sanitized logs. This is partial P2.6 delivery, not proof every failure is fully correlated or every sensitive log shape is covered.

**Performance budgets:** Freeze representative measurements first; then agree latency/error/CPU/memory thresholds per workload and enforce a regression budget. Do **not** manufacture arbitrary p95 guarantees before benchmark measurements exist.

**Phase exit:** representative automated workflow suite and a versioned performance dashboard/report are attached to release candidates; regressions outside the approved budget block release.

## 6. Phase 3 — Architecture, maintainability, and UX (P1/P2)

This phase should be **incremental and behavior-preserving**, not a rewrite. Large modules observed at the checkpoint include `pages/Settings.tsx`, `backend/internal/db/db.go`, `lib/djAudio.ts`, `backend/internal/api/api.go`, `pages/Spotify.tsx`, and `services/api.ts`. Size is a refactoring signal, not proof of a bug.

| ID | Work package | Status | Current scope and deliverables | Definition of done |
| --- | --- | --- | --- | --- |
| **P3.1** | Domain boundaries | **Partial** | Dedicated source adapters, media-capability and semantic modules exist. Oversized Settings/API/DB/audio modules still need incremental decomposition and coupling verification.| Existing routes/storage schemas preserved or explicitly migrated; targeted regression suite green; coupling decreases without new circular dependencies |
| **P3.2** | Stronger frontend contracts | **Partial** | Typed capability/service boundaries and library/Spotify surface performance work are delivered. Proposed strict coverage grows from four to twelve selected modules without source changes or new escape hatches; broader migration and measured code-splitting benefits remain open.| New code uses strict typing; targeted modules migrate with no `any` escape hatch proliferation; bundle/startup gain is measured |
| **P3.3** | First-run and library workflows | **Partial** | First-launch source setup, tabbed Settings, loading/empty/retry states, Spotify surface persistence and local preparation status exist. Proposed browser-only source-settings and playlist persistence E2E coverage is in #131. Assisted usability and end-to-end source-repair qualification remain open.| First-run and source-repair journeys pass assisted usability review and end-to-end tests |
| **P3.4** | Design consistency and accessibility | **Partial** | Design tokens, shared UI components, responsive layouts and DJ keyboard/focus work are delivered. Proposed playlist-name accessible labeling is in #131. A draft [WCAG 2.2 AA plan and findings register](ACCESSIBILITY_QUALIFICATION.md) now covers standard workflows and candidate DJ alternatives. Plan approval, full criterion ledger, host execution and accessible-alternative qualification remain open.| Standard app flows meet approved WCAG 2.2 AA test plan; specialized DJ controls have documented accessible alternatives where feasible |
| **P3.5** | Diagnostics and data operations UX | **Partial** | Duplicate review, diagnostics/repair, backup/staged restore and support export UI exist. Full destructive-action/recovery continuity acceptance and storage-quota UX remain open.| Destructive actions are reviewable; recovery progress and outcome are explicit; tests protect catalog continuity |

**Progress & Evidence Log (Phase 3, October 15):**
- [PR #122](https://github.com/ajbergh/ViiB-MediaHub/pull/122) (`4b86824`) delivers the shared visual refresh, library-performance changes and Spotify surface persistence, with targeted frontend tests. These changes do not establish representative workload percentiles or WCAG acceptance.
- [PR #124](https://github.com/ajbergh/ViiB-MediaHub/pull/124) (`59c1970`) adds the managed local semantic library, independent chat-provider configuration and Spotify Premium media gates. [Architecture](architecture.md#semantic-retrieval-subsystem) and [AI Smart Mix](smart-playlists.md) explain local matching, cloud prompt/candidate disclosure and strict audio constraints. Professional musical-analysis qualification remains deferred.
- `08322ba` refreshes UI/documentation and screenshots; the [documentation review](DOCUMENTATION_REVIEW_2026-10-15.md) and [capture workflow](screenshots.md) record that work. Documentation/screenshots are product evidence, not acceptance tests.

**Phase exit:** no large cross-cutting rewrite is required for 1.0. Release-critical usability and accessibility bugs are closed; remaining modularization can continue as small post-1.0 slices with measured benefit.

## 7. Phase 4 — 1.0 stable-release qualification (P0 gate)

This phase **validates** earlier work; it does not substitute a green compile for real-host acceptance.

| ID | Release gate | Required evidence |
| --- | --- | --- |
| **P4.1** | Source/data migration | Upgrade from RC5 to candidate 1.0; migrate/read credentials; verify catalog, playlists, history, likes and source ownership; restore from backup |
| **P4.2** | Multi-platform installers | Install, run, upgrade, uninstall and verify signature on Windows x64 (desktop installer), macOS Intel/Apple Silicon (DMG), and Linux amd64 (`.deb`/tarball); decide and document whether the web-only Windows/Linux arm64 builds (there are no arm64 desktop builds for Windows or Linux) are supported; disclose any architecture not actually qualified |
| **P4.3** | Desktop and browser smoke | Launch/shutdown, library scan, offline song playback, Plex music sync, Spotify login/stream/download (where authorized), queue/search and error recovery |
| **P4.4** | Security/privacy review | P0 threat model, credential migration, dependency/attribution scan, backup secret handling and privacy-safe logs signed off; documented disclosure of what library data is sent to optional third-party AI/metadata providers |
| **P4.5** | Performance and usability | Baseline budgets met for agreed profiles; accessibility and long-session playback known-critical defects closed |
| **P4.6** | Release publishing | Signed/notarized artifacts where required, checksums, provenance/SBOM, known limitations, changelog, troubleshooting guide and rollback instructions |

**Stable 1.0 decision rule:** release only if all P0 gates pass and critical P1 reliability issues are closed or consciously re-scoped with clear user-facing limitations. **Professional musical-analysis accuracy is not an automatic 1.0 release blocker** if existing outputs remain advisory, uncertainty is honest, and playback/recovery contracts hold.

## 8. Phase 5 — Professional DJ and advanced intelligence (deferred separate scope)

Existing DJv2 stems/cue/mix functionality is substantial, but independent benchmark and native-hardware qualification remain open. Keep the existing implementation and verified reference baseline intact.

| ID | Future work | Qualification required |
| --- | --- | --- |
| **P5.1** | BPM and key improvements | Independent, held-out audio; half/double-time metrics, exact/Camelot-compatible key accuracy, calibrated unknown/refusal rates |
| **P5.2** | Beatgrid, meter, downbeat and structure | Phase accuracy, bar-one/intro drift, continuity, cue usefulness and per-genre failure analysis with annotated labels |
| **P5.3** | Stem transport and device reliability | Long-form deck playback, timing, resampling, scratch/slip, fallback, underrun, MIDI/controller and device-change host evidence |
| **P5.4** | Mix planning and explainable recommendations | Ranking quality, harmonic compatibility, phrase/energy flow, provenance, user overrides, and reproducible evaluation |
| **P5.5** | Research-governance and packaging | Legal dataset/source review, fixed train/tuning/reserved splits, overlap audit; separately approve model/runtime packaging only after evidence |

**Do not** promote Beat This, learned downbeats, new estimator thresholds, Python/Torch runtime, or experimental parameter candidates into production defaults without independent qualification. Follow [deferred work](LOCAL_ANALYSIS_FUTURE_BRANCH.md), [DJ analysis roadmap](DJV2_PROFESSIONAL_TRACK_ANALYSIS_ROADMAP.md), and [DJ stems/cues/mix roadmap](DJV2_PRO_STEMS_CUES_MIX_INTELLIGENCE_ROADMAP.md).

## 9. Recommended implementation waves and dependencies

| Wave | Start with | Can parallelize | Exit decision |
| --- | --- | --- | --- |
| **W1 — Foundations** | Qualify delivered P0.1/P1.1; implement P0.3 branch rules | P0.5 supply-chain review, P2.1 benchmark specification, signing-credential procurement for P0.4 | Threat model and source/codec test fixtures approved |
| **W2 — Data safety** | Qualify merged P0.2 migration on target hosts | Qualify delivered P1.2; implement P2.4 browser test harness | Upgrade/relocation and playback/import behavior tested |
| **W3 — Media resilience** | P1.3–P1.6 local/Spotify/Plex recovery | P2.3 cache limits, P2.6 diagnostics | Failure scenarios and restoration tests green |
| **W4 — Measurement and polish** | P2.1–P2.5 baseline/qualification | P3.1–P3.5 *small* isolated changes | Real-browser/native matrix and performance budgets accepted |
| **W5 — Stable launch** | P0.4 signing pipeline completion (a staged-artifact demonstration is a Phase 0 exit item), P4.1–P4.6 release validation | Release documentation and support triage | Go/no-go for stable 1.0 |
| **W6 — Research/innovation** | P5 only after explicit scope approval | Subsequent safe architecture slices | Evidence-driven DJ roadmap, not hidden 1.0 scope |

Avoid a single PR for an entire phase. Prefer small diffs with tests, a migration/rollback note when state changes, and no unrelated estimator changes.

## 10. Suggested remaining GitHub follow-ups

These are **suggested follow-up titles**, not a live GitHub issue listing. P0.1/P0.2/P1.1/P1.2 implementation PRs are merged; use qualification follow-ups rather than reopening their delivered implementation scope. Partial rows need only their remaining scope.

1. `P0.1 Security: qualify merged loopback trust boundary and review threat model`
2. `P0.2 Credentials: qualify v2 upgrade/relocation/restart across supported hosts`
3. `P0.3 Governance: protect main and require critical CI checks`
4. `P0.4 Release: enforce production signing/notarization and artifact provenance`
5. `P0.5 Supply chain: SBOM, third-party notices and shipped dependency audit`
6. `P1.1 Media: qualify delivered capability matrix across runtimes and sources`
7. `P1.2 Media: qualify delivered FLAC/M4A import and unsupported-analysis policy`
8. `P1.3 Player: source-neutral playback and session-transition regression suite`
9. `P1.4 Spotify: native buffering/search and authenticated lifecycle qualification`
10. `P1.5 Plex: interrupted synchronization and direct-play codec recovery`
11. `P1.6 Recovery: RC5 upgrade and backup/restore integrity test cases`
12. `P2.1 Performance: 1K/10K/100K reproducible catalog baselines`
13. `P2.2 Performance: measure bounded scan/analysis workload costs and decode reuse`
14. `P2.3 Storage: global artifact/cache quotas and safe eviction`
15. `P2.4 QA: Playwright critical user journeys in CI`
16. `P2.5 Native: Windows/macOS/Linux long-session and WebView qualification matrix`
17. `P2.6 Support: qualify failure correlation and extend support/log privacy coverage`
18. `P3.1 Architecture: incremental Settings/API/DB/audio module decomposition`
19. `P3.2 TypeScript: gradual strict-mode contract migration`
20. `P3.3–P3.5 UX: first-run workflows, accessibility register and data-operations UX`
21. `P4.1–P4.6 Release: stable 1.0 cross-platform qualification tracker`

Recommended labels: `priority/P0`, `priority/P1`, `priority/P2`; `phase/0`…`phase/5`; `area/security`, `area/media`, `area/spotify`, `area/plex`, `area/dj`, `area/frontend`, `area/backend`, `area/release`; and `status/blocked`.

## 11. Common definition of done

A roadmap item can be marked **Qualified** only when it has:

1. A linked issue/PR and a concise description of behavior changed and compatibility risks.
2. Tests at the appropriate layer: unit, integration, E2E, native host, or measured corpus; **unsupported test modes disclosed**.
3. CI passing on the merged head; an explicit manual qualification record where CI cannot reach the scenario.
4. Migration, rollback, security and log-redaction review if persisted or sensitive data is affected.
5. Updated OpenAPI/product documentation and release notes where user-facing behavior changes.
6. A measurable acceptance result with evidence links, failure boundaries and the reviewer/date.

**Standard available validations:**

```bash
npm ci
npm run check
python3 scripts/check_documentation_links.py
cd backend
go test ./...
go test -race ./... -timeout 25m
go vet ./...
go install honnef.co/go/tools/cmd/staticcheck@v0.8.1   # match CI
staticcheck ./...
```

CI additionally runs `npm audit --omit=dev --audit-level=high`, binary `govulncheck -mode binary` on the built web and Wails executables, a three-platform analysis determinism comparison, and native desktop builds. Also run release pipeline builds and relevant platform/corpus/Playwright checks before claiming qualifications. Tests are not run by publishing this roadmap alone.

## 12. Maintenance and status updates

- **This document is the roll-up;** specialist documents retain detailed benchmark histories, audit evidence and active technical contracts.
- On every work-item PR: update the relevant row status, link issue/PR/commit and append a dated evidence note. No status change based on expectations.
- At each milestone: record resolved issues, new risks, CI/platform evidence, performance results, deferred work and the next exit gate.
- Preserve dates and failed experiments; moving documents to the archive never means unresolved quality gates passed.
- Rebaseline this document after releases or major integration changes; distinguish **verified on main** from **proposed** or **host-untested**.
- Avoid publishing credentials, local-library paths, proprietary media, account identifiers or uncensored diagnostic artifacts in GitHub.

### October 20 implementation and deferrals

[PR #131](https://github.com/ajbergh/ViiB-MediaHub/pull/131), branch `codex/roadmap-easy-foundations` (proposed; not merged or Qualified). These are bounded implementation slices; work packages retain their remaining acceptance scope. [PR CI](https://github.com/ajbergh/ViiB-MediaHub/pull/131/checks) is pending at publication; merged-head qualification remains open.

- **P0.5:** pin Staticcheck `v0.8.1` and govulncheck `v1.6.0` in all six workflow installs. `npm sbom --package-lock-only --sbom-format cyclonedx` generated a valid CycloneDX 1.5 inventory of **287 npm components** locally; CI retains it for 14 days. Go/native inventories, licenses and security acceptance remain open.
- **P2.4:** `npm run test:e2e` passed **3/3 Chromium tests**: fresh IndexedDB empty library to Add Music to Settings; legacy DJ redirect to workstation and reload; and create an IndexedDB playlist, reload, then open its detail with backend unavailable. Tests intercept all API/external requests, use no private data/accounts, and exercise the production preview. CI consumes the frontend artifact and retains synthetic failure evidence for seven days. Audio output, import, providers, native WebViews and recovery integrity remain unqualified. See [developer instructions](development.md#browser-smoke-gate).
- **P3.2:** strict include list grows **4 to 12** modules, adding capabilities, imported catalog, jobs, recovery API, backend conversion, three-band data and DJ output services/hooks. No runtime changes or typing suppressions. `npm run check` passed palette/raw-color checks, TypeScript, **148 files / 725 Vitest tests**, and production build. Existing large-chunk warnings remain; no bundle/startup gain is claimed.
- **P3.3/P3.4:** browser-only source-settings navigation and playlist creation/reload now have E2E coverage. The playlist-name textbox now has an explicit accessible name. Assisted first-run/source-repair review, full accessibility test plan/register and assistive-technology qualification remain open.
- **Deferred for complexity:** P2.1 representative 1K/10K/100K benchmark design, P2.3 cache quota/eviction policy, P3.1 major decomposition and Phase 5 research need dedicated scopes and evidence. P0.1/P0.2 and P1 target-host qualification remain open.
- **Human/external follow-up:** review/enable main protection and emergency bypass policy (P0.3); signing/notarization credentials and signed-installer tests (P0.4); attribution/licensing acceptance (P0.5); authenticated providers, native hosts, assistive-technology review and stable-release signoff (P4). No release qualification is claimed.

### P3.4 accessibility planning follow-up (October 20)

Branch `codex/p3-4-accessibility-qualification-plan`, based on PR #131, adds the
[accessibility qualification plan and findings register](ACCESSIBILITY_QUALIFICATION.md).
The plan defines host/input coverage, workflow checks, evidence/result rules,
DJ alternative qualification tasks and initial source observations. It is a draft
for review, not WCAG conformance or manual/assistive-technology acceptance.
Validation: documentation links/evidence JSON and diff whitespace checks. No
runtime changes or new host audit. Human execution/approval remain deferred.

### Baseline evidence and implementation references

- [Documentation index](index.md)
- [Architecture](architecture.md) and [versioned API](openapi-v2.yaml)
- [Spotify scope](SPOTIFY_BRANCH_REMAINING_WORK.md), [closeout evidence](SPOTIFY_BRANCH_CLOSEOUT_NEXT_STEPS.md), [parity audit](SPOTIFY_COOKIE_AUTH_PARITY_AUDIT.md)
- [Scan stability and analysis storage](scan-stability-and-storage.md), [October 15 documentation review](DOCUMENTATION_REVIEW_2026-10-15.md)
- [Codec limitations](AUDIO_FORMAT_SUPPORT_GAPS.md), [preparation measurements](SPOTIFY_PREPARATION_PERFORMANCE.md)
- [Plex behavior and limitations](plex-music.md)
- [Professional track-analysis roadmap](DJV2_PROFESSIONAL_TRACK_ANALYSIS_ROADMAP.md), [stems/cue/mix roadmap](DJV2_PRO_STEMS_CUES_MIX_INTELLIGENCE_ROADMAP.md), [deferred local research](LOCAL_ANALYSIS_FUTURE_BRANCH.md)
- [CI workflow](../.github/workflows/ci.yml), [release workflow](../.github/workflows/release.yml)

---

*Last updated: 2026-10-20 against local/hosted main `c50e2b4` plus proposed `codex/roadmap-easy-foundations` changes. This file establishes work priorities and acceptance criteria; it does not certify unfinished functionality or authorize merging, releasing, changing credentials, or expanding DJ research scope without the corresponding review.*
