# ViiB MediaHub — Product & Engineering Roadmap

> **Status:** Active / living plan  
> **Last reviewed:** 2026-10-13  
> **Target:** Reliable, secure, supportable **ViiB MediaHub 1.0** followed by qualified professional-DJ improvements  
> **Source of truth:** This file owns cross-project priorities and release gates. Domain-specific plans and dated evidence remain authoritative for the technical detail they cover.

## 1. Baseline and scope

ViiB MediaHub is a local-first music hub: React 19 / TypeScript frontend, Go / SQLite backend, Wails desktop builds, browser/server mode, a unified local/Plex music catalog, separate Spotify integration, AI-assisted library intelligence, and two-deck DJ functionality.

**Verified repository checkpoint (2026-10-13):**

- Latest code commit on `main`: [`6a76cafd`](https://github.com/ajbergh/ViiB-MediaHub/commit/6a76cafd0563253a4b00167180a940d0eb152b5b); later commits on `main` are documentation-only (this roadmap and its index link). [PR #117](https://github.com/ajbergh/ViiB-MediaHub/pull/117) merged Spotify metadata provenance, owner/source fencing, persistent observations, waveform fallback, and lifecycle fixes.
- The [`main` CI run for `6a76cafd`](https://github.com/ajbergh/ViiB-MediaHub/actions/runs/38343013399) completed **successfully**; runs for the later documentation commits were still in progress when this was checked. Passing CI does not establish real-device, provider, musical-accuracy, security-penetration, or production-release qualification.
- Latest published GitHub release at this checkpoint: [`v1.0.0-rc5`](https://github.com/ajbergh/ViiB-MediaHub/releases/tag/v1.0.0-rc5), published 2026-10-05. **Stable 1.0 is not declared complete.**
- No open issues or pull requests were returned by the GitHub open-item listing at this checkpoint. Roadmap rows below are **proposed work**, not already-created issues.
- `main` branch protection is **disabled** (the GitHub API reports "Branch not protected"). Do not assume required checks are enforced merely because CI exists.
- The repository contains extensive Go and frontend tests, native build workflows, analysis determinism checks, vulnerability scans, and developer documentation. Preserve those investments.
- **Release-pipeline baseline (from [`release.yml`](../.github/workflows/release.yml)):** the workflow builds a Windows x64 installer, macOS arm64/x64 DMGs, a Linux amd64 `.deb`/tarball, and web builds for Windows/macOS/Linux on x64 and arm64; it publishes `SHA256SUMS.txt` and marks RC tags as pre-releases. macOS bundles are **ad-hoc signed and not notarized**; Windows Authenticode signing runs **only when repository secrets exist** and otherwise continues unsigned. There is no SBOM or provenance attestation step.
- **Test-tooling baseline:** `@playwright/test` is a dev dependency and several Playwright-driven audit scripts exist under `scripts/` (for example `djv2-audit.mjs`, `mobile-audit.mjs`), but there is no Playwright config or browser end-to-end job in CI. TypeScript `strict` currently applies to only four files, listed in `tsconfig.boundaries.json`.

**Important continuity decision:** The owner previously deferred further local scanning/DSP estimator research from the Spotify metadata branch. PR #117 is merged; deferred work belongs in a **separate future scope** (Phase 5), not retroactively in that PR or as an undisclosed 1.0 dependency. See [Spotify scope](SPOTIFY_BRANCH_REMAINING_WORK.md), [closeout](SPOTIFY_BRANCH_CLOSEOUT_NEXT_STEPS.md), and [deferred local analysis](LOCAL_ANALYSIS_FUTURE_BRANCH.md).

### Outcome priorities

1. **P0: prevent security, credential, data-loss, and release-trust failures.**
2. **P1: make playback, catalog/source behavior, codec support, and recovery predictable.**
3. **P1: qualify performance and user-critical workflows against reproducible evidence.**
4. **P2: reduce coupling and modernize UX without disrupting the shipping application.**
5. **P2 / post-stabilization: expand professional DJ capability only after accuracy and platform qualification.**

### Status vocabulary

- **Baseline delivered:** already present or merged; listed as context, not new scope.
- **Planned:** accepted candidate work, not yet implemented or validated.
- **In progress:** an issue/branch/PR and evidence are linked.
- **Blocked:** dependency or external constraint named explicitly.
- **Qualified:** acceptance checks and evidence links recorded.
- **Deferred:** intentionally excluded from the current release scope.

Do not promote **Planned** to **Qualified** solely because a PR merged or CI passed.

## 2. Roadmap at a glance

| Phase | Priority | Goal | Depends on | Status | Release role |
| --- | --- | --- | --- | --- | --- |
| **0 — Security & release foundations** | P0 | Harden the local privileged API, credential storage, repository governance, and release authenticity | Current baseline | **Planned** | Required for stable 1.0 |
| **1 — Playback, media format & source reliability** | P1 | Eliminate inconsistent formats and strengthen local/Plex/Spotify failure behavior | Phase 0 security interfaces | **Planned** | Required for stable 1.0 |
| **2 — Performance & product-quality evidence** | P1 | Benchmark real workloads and automate critical user journeys | Instrumentation from Phases 0–1 | **Planned** | Required for stable 1.0 |
| **3 — Architecture & UX modernization** | P1/P2 | Improve maintainability, accessibility, and workflow clarity | Stable contracts from Phases 0–2 | **Planned** | Critical fixes before 1.0; larger refactors incremental |
| **4 — Stable 1.0 qualification** | P0 gate | Demonstrate signed, recoverable, usable builds across supported platforms | Phases 0–3 release-critical items | **Planned** | Final release gate |
| **5 — Professional DJ & advanced analysis** | P2 | Independently qualify musical accuracy and transport/device behavior | Separate approved future scope | **Deferred** | Not a blanket 1.0 blocker |

Work packages use stable IDs (`P0.1` etc.) for GitHub issues, PR titles, and progress reports. The number before the dot is the **phase**, not the priority: `P2.1` is Phase 2 work (priority P1), and priority labels (`priority/P0`–`P2`) are tracked separately. Dependencies should be rechecked before starting each item.

## 3. Phase 0 — Security and release foundations (P0)

**Rationale:** The backend listens on loopback, but is a privileged local HTTP service. The router restricts allowed CORS origins (with credentials allowed) and adds basic hardening headers, but it has no `Host`-header validation, and five SSE handlers in `api.go` independently set `Access-Control-Allow-Origin: *`. Separately, `enc:v1` encrypted-setting key derivation uses the hostname, executable path and home directory plus a static string in source, so moving the binary can strand stored secrets and the key is not bound to an OS-protected secret; and the current release candidates are not production-signed (see the release-pipeline baseline above). These require explicit threat modeling and qualification; an inconsistent header is **not** proof of an exploitable vulnerability.

| ID | Work package | Deliverables | Definition of done |
| --- | --- | --- | --- |
| **P0.1** | Loopback API trust boundary | Document trusted origins/hosts; centralized Origin/Host/CORS/SSE policy; inspect CSRF and privileged mutations; preserve Wails and local development flows | Negative tests for attacker-controlled origins/hosts (including DNS-rebinding-style `Host` values) and mutating requests, positive tests for approved desktop/browser clients, consistent SSE response headers; threat model reviewed |
| **P0.2** | Versioned credential key storage | Add `enc:v2` or equivalent for every setting stored via `crypto.Encrypt` (Plex, semantic and other secrets); use OS-protected secret material where available (DPAPI/Keychain/Linux secret service or documented secure fallback); migrate readable `enc:v1` entries, with rollback/reauth path | Install/relocate/upgrade/restart tests on supported OSes; readable old secrets migrate without disclosure or silent loss; unresolvable legacy secrets report a clear reauthentication path |
| **P0.3** | Governance and required checks | Protect `main` via branch ruleset; require frontend, Go test/race/vet/staticcheck, vulnerability and packaging checks; document emergency bypass | Normal PR cannot merge without passing required checks; explicit reviewed emergency process exists |
| **P0.4** | Release signing and provenance | Stable-release signing policy; macOS Developer ID + notarization, Windows Authenticode, checksums and verifiable build/source provenance | Production (non-RC) release workflow fails closed when mandatory signing/verification is unavailable (today Windows signing is skipped without secrets and macOS is ad-hoc signed only); install and signature-verification smoke tests pass |
| **P0.5** | Supply-chain and licensing review | Runtime/dev dependency inventory, pinned build tooling (CI currently installs `staticcheck` and `govulncheck` with `@latest`), SBOM/third-party notice review, especially bundled upstream code and Spotify-related dependencies | License obligations and security advisories are recorded; no unaccepted shipped critical/high vulnerability; reproducible dependency inputs |

**Phase exit:** P0.1/P0.2 data and threat-model regressions green on target platforms; repository policy active; release signing path demonstrated with staged artifacts. Certificate, Developer ID and notarization procurement has external lead time, so start it in Wave 1. Preserve the current security patches and binary vulnerability scans.

**Primary files:** [server middleware](../backend/internal/server/server.go), [API/SSE handlers](../backend/internal/api/api.go), [credential crypto](../backend/internal/crypto/crypto.go), [CI](../.github/workflows/ci.yml), [release workflow](../.github/workflows/release.yml).

## 4. Phase 1 — Playback, formats, and source reliability (P1)

**Rationale:** The scanner accepts FLAC/M4A, but browser-only import and backend DJ waveform/analysis do not provide equivalent support; browser/WebView playback remains decoder-dependent. The existing [format audit](AUDIO_FORMAT_SUPPORT_GAPS.md) is the starting contract, not a claim of parity. Spotify parity and cross-platform behavior also retain documented gaps.

| ID | Work package | Deliverables | Definition of done |
| --- | --- | --- | --- |
| **P1.1** | Single media-capability matrix | Explicit per-format/per-source support for import, metadata, playback, waveform, DJ analysis and fallback; reusable capability errors in UI/API | MP3, OGG Vorbis, FLAC, M4A/AAC, WAV, OPUS and other advertised formats have real fixtures or explicit unsupported state; documentation matches behavior |
| **P1.2** | FLAC/M4A import and analysis policy | Correct browser import filters; validate metadata/artwork/length; add backend decoder support **or** honest unsupported-analysis fallback | Native/browser import fixtures pass; tracks never silently claim BPM/key/waveform support unavailable in that runtime |
| **P1.3** | Playback state transitions | Source-neutral recovery for queue/seek/pause/restart/crossfade/session retirement and audio-device changes | Repeatable tests cover local → Plex → Spotify and back, logout during playback, range seeking, network interruption, and no unrelated local-playback teardown |
| **P1.4** | Spotify parity and resilience | Complete prioritized cookie-session, 429/rate-limit, unavailable-field, native search/buffering, download integrity and reconnect scenarios; provider/legal compatibility review | Contract/fixture cases pass; at least one real provider flow per supported host passes where feasible; unverified account/market cases remain visibly marked; no fabricated metadata |
| **P1.5** | Plex large-library and offline safety | Incremental sync/resume/error diagnostics; music-only scope; seek/artwork proxy/redirect validation; decide transcoding fallback policy | Interrupted sync never mass-deletes valid catalog rows; auth expiry/offline conditions recover; real-PMS smoke cases recorded; unsupported codecs clearly explained |
| **P1.6** | Data and media recovery | Backup/restore, duplicate/playlist integrity, job restart/idempotency and unavailable-source recovery scenarios | Restoring a tested backup preserves library/playlists/likes/history and consistent source links; unrecoverable changes are disclosed before destructive actions |

**Phase exit:** supported format matrix is factual; no known data-loss source-transition defect; critical playback and migration tests pass across Wails/browser profiles. Codec playback/analysis support must not be generalized from filename acceptance.

**Primary references:** [format gaps](AUDIO_FORMAT_SUPPORT_GAPS.md), [Spotify parity](SPOTIFY_COOKIE_AUTH_PARITY_AUDIT.md), [Plex contract](plex-music.md), [API contract](openapi-v2.yaml).

## 5. Phase 2 — Performance, tests, and observability (P1)

The current [preparation baseline](SPOTIFY_PREPARATION_PERFORMANCE.md) includes a **105-track offline Windows** experiment (Milestone 259 run): ~759 seconds fresh, ~1.5 seconds warm, about 100 MB of stored artifacts and a ~71 MiB process peak working set. It is not a connected-provider, native UI or multi-platform latency qualification. Avoid making performance promises based on it alone.

| ID | Work package | Deliverables | Definition of done |
| --- | --- | --- | --- |
| **P2.1** | Representative benchmark harness | Deterministic 1K / 10K / 100K catalog inventories; fresh/warm startup/search, scan, import, Plex sync, player, DJ-preparation benchmarks | CI or scheduled reports provide p50/p95/p99 where applicable, memory/CPU/disk/artifact totals, environment and reproducible inputs |
| **P2.2** | Bounded analysis and scanning | Measure decode reuse, concurrency, cancellation/retries, IO backpressure, connected/offline resource costs | No unbounded worker/provider storms; warm runs avoid unnecessary decodes; measured improvements do not regress accuracy or provenance |
| **P2.3** | Cache lifecycle | Global per-type quotas, eviction, repair/diagnostic UI, database WAL/checkpoint behavior and restoration tests | Predictable disk limits; cache eviction cannot destroy user data or invalidate retained provenance; reference workloads stay inside approved resource budgets |
| **P2.4** | Browser end-to-end gates | Promote the existing Playwright dependency and audit scripts into a configured, deterministic suite (no config or CI job exists today):  first launch, source setup, search, playlists, downloads, queue, DJ entry, offline/reconnect and error handling | Critical flows reliably pass in CI; failures retain screenshots/traces/logs without secrets |
| **P2.5** | Native system qualification | Windows/macOS/Linux long-session playback, WebView compatibility, audio device changes, startup/shutdown, search and source retirement | Tested platform/architecture matrix with explicit passes, failures and accepted exceptions |
| **P2.6** | Troubleshooting evidence | Correlation IDs, bounded structured logs, job diagnostics, privacy-safe opt-in support bundle and clear retry guidance | A user-visible failure can be traced to a sanitized diagnostic record; tokens, cookies, local-private data are excluded by default |

**Performance budgets:** Freeze representative measurements first; then agree latency/error/CPU/memory thresholds per workload and enforce a regression budget. Do **not** manufacture arbitrary p95 guarantees before benchmark measurements exist.

**Phase exit:** representative automated workflow suite and a versioned performance dashboard/report are attached to release candidates; regressions outside the approved budget block release.

## 6. Phase 3 — Architecture, maintainability, and UX (P1/P2)

This phase should be **incremental and behavior-preserving**, not a rewrite. Large modules observed at the checkpoint include `pages/Settings.tsx`, `backend/internal/db/db.go`, `lib/djAudio.ts`, `backend/internal/api/api.go`, `pages/Spotify.tsx`, and `services/api.ts`. Size is a refactoring signal, not proof of a bug.

| ID | Work package | Deliverables | Definition of done |
| --- | --- | --- | --- |
| **P3.1** | Domain boundaries | Split oversized files by settings section, persistence domain, audio transport, source adapter and typed API contract | Existing routes/storage schemas preserved or explicitly migrated; targeted regression suite green; coupling decreases without new circular dependencies |
| **P3.2** | Stronger frontend contracts | Expand TypeScript `strict` from the current limited boundary config; typed service DTOs; assess route-based code splitting | New code uses strict typing; targeted modules migrate with no `any` escape hatch proliferation; bundle/startup gain is measured |
| **P3.3** | First-run and library workflows | Source selection/onboarding, setup validation, transparent source badges, empty/offline/retry states, progress for background work | First-run and source-repair journeys pass assisted usability review and end-to-end tests |
| **P3.4** | Design consistency and accessibility | Design tokens, keyboard and focus/navigation coverage, scalable layout and DJ density, accessibility findings register | Standard app flows meet approved WCAG 2.2 AA test plan; specialized DJ controls have documented accessible alternatives where feasible |
| **P3.5** | Diagnostics and data operations UX | Safer bulk edits, duplicate management, backups, restore previews, storage usage and repair warnings | Destructive actions are reviewable; recovery progress and outcome are explicit; tests protect catalog continuity |

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
| **W1 — Foundations** | P0.1 local API security, P0.3 branch rules, P1.1 capability inventory | P0.5 supply-chain review, P2.1 benchmark specification, signing-credential procurement for P0.4 | Threat model and source/codec test fixtures approved |
| **W2 — Data safety** | P0.2 credential v2 migration | P1.2 format UI import parity, P2.4 browser test harness | Upgrade/relocation and playback/import behavior tested |
| **W3 — Media resilience** | P1.3–P1.6 local/Spotify/Plex recovery | P2.3 cache limits, P2.6 diagnostics | Failure scenarios and restoration tests green |
| **W4 — Measurement and polish** | P2.1–P2.5 baseline/qualification | P3.1–P3.5 *small* isolated changes | Real-browser/native matrix and performance budgets accepted |
| **W5 — Stable launch** | P0.4 signing pipeline completion (a staged-artifact demonstration is a Phase 0 exit item), P4.1–P4.6 release validation | Release documentation and support triage | Go/no-go for stable 1.0 |
| **W6 — Research/innovation** | P5 only after explicit scope approval | Subsequent safe architecture slices | Evidence-driven DJ roadmap, not hidden 1.0 scope |

Avoid a single PR for an entire phase. Prefer small diffs with tests, a migration/rollback note when state changes, and no unrelated estimator changes.

## 10. Initial GitHub issue backlog

These are **suggested issue titles**. They have not been created in GitHub.

1. `P0.1 Security: normalize loopback Host/Origin/CORS and SSE policy`
2. `P0.2 Credentials: versioned key storage with executable-relocation migration`
3. `P0.3 Governance: protect main and require critical CI checks`
4. `P0.4 Release: enforce production signing/notarization and artifact provenance`
5. `P0.5 Supply chain: SBOM, third-party notices and shipped dependency audit`
6. `P1.1 Media: canonical per-source codec capability matrix and test fixtures`
7. `P1.2 Media: FLAC/M4A browser import and backend analysis policy`
8. `P1.3 Player: source-neutral playback and session-transition regression suite`
9. `P1.4 Spotify: native buffering/search and authenticated lifecycle qualification`
10. `P1.5 Plex: interrupted synchronization and direct-play codec recovery`
11. `P1.6 Recovery: RC5 upgrade and backup/restore integrity test cases`
12. `P2.1 Performance: 1K/10K/100K reproducible catalog baselines`
13. `P2.2 Performance: bounded analysis/scan concurrency and decode reuse measurements`
14. `P2.3 Storage: global artifact/cache quotas and safe eviction`
15. `P2.4 QA: Playwright critical user journeys in CI`
16. `P2.5 Native: Windows/macOS/Linux long-session and WebView qualification matrix`
17. `P2.6 Support: correlation IDs, bounded logs and privacy-safe support bundle`
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
go install honnef.co/go/tools/cmd/staticcheck@latest   # if not installed
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

### Baseline evidence and implementation references

- [Documentation index](index.md)
- [Architecture](architecture.md) and [versioned API](openapi-v2.yaml)
- [Spotify scope](SPOTIFY_BRANCH_REMAINING_WORK.md), [closeout evidence](SPOTIFY_BRANCH_CLOSEOUT_NEXT_STEPS.md), [parity audit](SPOTIFY_COOKIE_AUTH_PARITY_AUDIT.md)
- [Codec limitations](AUDIO_FORMAT_SUPPORT_GAPS.md), [preparation measurements](SPOTIFY_PREPARATION_PERFORMANCE.md)
- [Plex behavior and limitations](plex-music.md)
- [Professional track-analysis roadmap](DJV2_PROFESSIONAL_TRACK_ANALYSIS_ROADMAP.md), [stems/cue/mix roadmap](DJV2_PRO_STEMS_CUES_MIX_INTELLIGENCE_ROADMAP.md), [deferred local research](LOCAL_ANALYSIS_FUTURE_BRANCH.md)
- [CI workflow](../.github/workflows/ci.yml), [release workflow](../.github/workflows/release.yml)

---

*Last updated: 2026-10-13. This file establishes work priorities and acceptance criteria; it does not certify unfinished functionality or authorize merging, releasing, changing credentials, or expanding DJ research scope without the corresponding review.*
