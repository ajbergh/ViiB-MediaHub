# Roadmap easy-first implementation pass — October 20, 2026

**Status: implementation prepared for review; release qualification remains open.**
The [roadmap](ROADMAP.md) remains the source of truth. This audit records the
completed implementation slices and why remaining acceptance work is deferred
from this autonomous pass. Deferred work below is still required wherever the
roadmap names it as a stable 1.0 gate.

## Integration and evidence

The [PR #153](https://github.com/ajbergh/ViiB-MediaHub/pull/153) (`codex/roadmap-easy-pass-rollup`) branch targets `main` and includes every
source head from PRs #131–#152, followed by this documentation audit. The
[snapshot](evidence/roadmap-easy-pass-2026-10-20.json) records exact heads,
ancestry verification and selected hosted results. Main at the snapshot is
`c50e2b440996783a315be7150fce379fdd830c9d`; no implementation PR has been merged
and no release has been published by this pass.

Review the combined diff and fresh roll-up checks before integration through the
[normal protected PR process](repository-governance.md). The 16 required CI
contexts and conditional release validation must pass. A skipped publication
job on a PR is expected and supplies no live signing or OIDC evidence. Source
PRs remain open as detailed review history; after integration they can be
closed as superseded without merging their changes again.

Historical exception: PR #139's latest release preflight failed in the queued
scan test's cleanup race. PR #141 fixes that test isolation and is included in
the roll-up. The old failed head is preserved in the snapshot; its result must
not be presented as green. Fresh roll-up validation covers the combined code.

## Implemented slices

| Area | Review history | Result and evidence boundary |
| --- | --- | --- |
| CI and supply chain | [#131](https://github.com/ajbergh/ViiB-MediaHub/pull/131), [#133](https://github.com/ajbergh/ViiB-MediaHub/pull/133), [#139](https://github.com/ajbergh/ViiB-MediaHub/pull/139), [#141](https://github.com/ajbergh/ViiB-MediaHub/pull/141) | Pinned analysis tools, npm CycloneDX and normalized Go inventories, exact 13-payload checksum validation and isolated queued-scan fixture. Real candidate assets were assembled and verified; published RC7 was not changed. |
| Governance and release safeguards | [#137](https://github.com/ajbergh/ViiB-MediaHub/pull/137), [#138](https://github.com/ajbergh/ViiB-MediaHub/pull/138), [#140](https://github.com/ajbergh/ViiB-MediaHub/pull/140), [#142](https://github.com/ajbergh/ViiB-MediaHub/pull/142) | Owner-approved main-only protection is active. Stable publishing is guarded; exact-payload provenance generation/verification is proposed in workflows. Production credentials, live attestation and consumer verification remain open. |
| Accessibility and lifecycle | [#132](https://github.com/ajbergh/ViiB-MediaHub/pull/132), [#134](https://github.com/ajbergh/ViiB-MediaHub/pull/134), [#136](https://github.com/ajbergh/ViiB-MediaHub/pull/136), [#143](https://github.com/ajbergh/ViiB-MediaHub/pull/143), [#146](https://github.com/ajbergh/ViiB-MediaHub/pull/146) | Draft accessibility plan, named controls, confirmation focus/cancellation, consequence disclosures and dialog/maintenance async ownership. Browser and component regression coverage does not qualify assistive technology or supported hosts. |
| Recovery | [#135](https://github.com/ajbergh/ViiB-MediaHub/pull/135), [#147](https://github.com/ajbergh/ViiB-MediaHub/pull/147) | Backup creation preserves existing archives; synthetic API-to-offline restore checks catalog, ordered playlists, likes, detailed history, selected settings, credential exclusion and rollback. RC5, source ownership and real-host/crash acceptance remain open. |
| Support diagnostics | [#148](https://github.com/ajbergh/ViiB-MediaHub/pull/148), [#151](https://github.com/ajbergh/ViiB-MediaHub/pull/151) | Bounded allowlisted support records replace raw-log export. Server-generated v2 failure references reach Library Operations and support archives. Local raw logs and other error/provider boundaries still need review. |
| Frontend contracts | [#144](https://github.com/ajbergh/ViiB-MediaHub/pull/144), [#145](https://github.com/ajbergh/ViiB-MediaHub/pull/145), [#149](https://github.com/ajbergh/ViiB-MediaHub/pull/149), [#150](https://github.com/ajbergh/ViiB-MediaHub/pull/150) | Global TypeScript strict checking passes with concrete nullable/ref/callback fixes and JS boundary declarations. Existing explicit `any`, suppressions and unchecked JS bodies remain. |
| Browser journeys | [#152](https://github.com/ajbergh/ViiB-MediaHub/pull/152), plus earlier browser slices | Seven Chromium journeys include real four-format file upload, metadata/artwork worker execution, IndexedDB persistence and reload. No parser/store injection; provider/native playback and filesystem-handle acceptance remain open. |

Local validation includes 747 frontend unit tests, global strict checking and
production build, seven browser journeys, focused Go race checks and recovery/
support integration tests. Detailed commands and scope are recorded in the
roadmap progress log and source PRs. These results concern the implementation
branches, not a qualified merged main or stable release.

## Remaining work and deferral reasons

The final repository audit found no further small, self-contained change that
would close a listed acceptance requirement. Larger design, representative
measurements, supported-host/provider access and human sign-off are deferred
explicitly below. This is an execution disposition, not promotion to Qualified.

| ID | Deferred acceptance work | Dependency or next action |
| --- | --- | --- |
| P0.1 | Threat model, privileged mutation/CSRF and approved-client acceptance | Human security review and supported client/host execution |
| P0.2 | Install, relocation, upgrade, restart and credential rollback | Supported-host migration matrix with legacy secrets |
| P0.3 | Required-check behavior on integrated main | Review/integrate through protected PR; inspect merged-head CI |
| P0.4 | Mandatory production signatures, notarization, live OIDC and consumer verification | Signing credentials, release authority and real-host verification |
| P0.5 | Published assets, per-target/native inventories, attribution and advisory acceptance | Release artifacts and human licensing/security review |
| P1.1 | Full advertised media capability matrix | Fixtures and execution across supported runtimes/sources |
| P1.2 | Native/browser format acceptance beyond bounded upload fixtures | Host/codec coverage and unsupported-analysis notice review |
| P1.3 | Local/Plex/Spotify transitions, interruptions, crossfade and device changes | Combined provider and audio-device matrix |
| P1.4 | Native responsiveness, profile/Recently Played and market/account parity | Authorized live provider accounts and supported hosts |
| P1.5 | Large/interrupted/offline/auth-expired Plex and codec behavior | Real PMS libraries and explicit fallback qualification |
| P1.6 | RC5 migration, source/Plex ownership and crash continuity | Real data/hosts and controlled recovery failure exercises |
| P2.1 | Representative 1K/10K/100K percentile/resource benchmarks | Agree workloads/budgets and implement measurement harness |
| P2.2 | Decode/concurrency budgets and durable-job shutdown/recovery | Workload measurements and common lifecycle policy design |
| P2.3 | Quotas, eviction, disk budgets and WAL/restoration | Storage policy design and failure/continuity qualification |
| P2.4 | Integrated CI plus backend/provider/playback/recovery and browser profiles | Fresh roll-up CI, real services and broader runtime fixtures |
| P2.5 | Native long sessions, device changes and WebView matrix | Windows/macOS/Linux hardware and sustained execution |
| P2.6 | Local-log privacy and legacy/middleware/provider correlation | Broader diagnostic contract and native export/sharing review |
| P3.1 | Settings/API/DB/audio decomposition and coupling proof | Larger incremental architecture design with regression coverage |
| P3.2 | Explicit `any`/suppression cleanup, JS body verification and startup/bundle gains | Contract review and measured performance work |
| P3.3 | Assisted usability and source-repair qualification | Human participants and complete provider recovery journeys |
| P3.4 | Approved WCAG ledger, assistive technology and DJ alternatives | Human plan approval and supported-host accessibility execution |
| P3.5 | Full recovery outcomes and storage-quota UX | Recovery qualification and agreed storage policy |
| P4.1 | RC5-to-candidate migration of all relations/source ownership | Representative real upgrades and restore matrix |
| P4.2 | Install/upgrade/uninstall/signature matrix and web ARM64 support decision | Supported hardware, production signatures and owner support policy |
| P4.3 | Native/browser scan, offline playback, provider, queue/search and recovery smoke | Real-host/provider acceptance run |
| P4.4 | Security/privacy/licenses and optional AI-provider disclosure sign-off | Human review of trust, secret, attribution and data-sharing boundaries |
| P4.5 | Performance budgets, accessibility and long-session acceptance | Agreed profiles, measurements and usability review |
| P4.6 | Signed release, provenance/SBOM, release docs and rollback | Earlier release gates, credentials and publishing decision |
| P5.1 | Held-out BPM/key accuracy | Separate research scope, legal dataset and reserved evaluation |
| P5.2 | Beatgrid/downbeat/structure accuracy | Annotated labels and independent musical evaluation |
| P5.3 | Stem transport, controller and device reliability | Supported hardware and long-form host measurements |
| P5.4 | Explainable mix recommendation quality | Ranking/evaluation design and user assessment |
| P5.5 | Research governance and model/runtime packaging | Dataset/legal review and separate scope approval |

Phases 0–3 remain Partial or qualification-open; Phase 4 remains Planned and
Phase 5 remains Deferred. Phase 5 is not a blanket stable 1.0 dependency.
