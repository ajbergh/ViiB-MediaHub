# Wails v3 upgrade assessment and implementation plan

Research date: 2026-10-23. Status: proposed; no runtime migration implemented.

## Recommendation

Proceed with a pinned-version prototype, then migrate the desktop shell in stages. The strongest benefits for ViiB are integrated tray support, explicit window management, and a cleaner native bridge. Keep the current v2 release path until v3 passes native qualification. Wails currently labels v3 **Beta**, with desktop API stability goals rather than a final-release guarantee. Its compatibility contract lists Go 1.25+, Windows amd64/arm64, macOS Intel/Apple Silicon, and Linux amd64/arm64. Linux defaults to GTK4/WebKitGTK 6; GTK3/WebKit2GTK 4.1 is a legacy option through v3.0.x, removed in v3.1. [Wails status](https://v3.wails.io/status/)

This assessment distinguishes documented framework capabilities from proposed ViiB uses. Benefits below are engineering expectations; performance, compatibility, and user experience improvements require measurement. An upgrade alone does not establish better audio latency, codec coverage, DJ timing, Spotify compatibility, or DSP accuracy.

## Current repository baseline

| Area | Current implementation and migration significance |
|---|---|
| Dependencies | [`backend/go.mod`](../backend/go.mod) pins Wails v2.11.0 and Go 1.26.10. Go already exceeds the documented v3 minimum. |
| Native entrypoint | [`backend/cmd/wails/main.go`](../backend/cmd/wails/main.go) embeds React/Vite output, creates the API server, and configures one Wails window through `wails.Run`. Preserve startup failure handling and orderly HTTP/API/database shutdown. |
| Browser distribution | [`backend/cmd/viib/main.go`](../backend/cmd/viib/main.go) is a separate executable sharing the HTTP backend. Its browser UI must continue to work without a native runtime. |
| Transport | Ordinary `/api` calls use the asset proxy. [`services/eventStreamURL.ts`](../services/eventStreamURL.ts) resolves direct loopback SSE/audio URLs because the existing native asset path buffers streaming responses. Native default port is 34115 for Spotify OAuth. |
| Bridge | The seven generated App methods are `GetServerURL`, `GetVersion`, `GetDataDir`, `GenerateSpotifyCodeChallenge`, `SetWindowCloseAction`, `SaveSupportBundle`, and `ShowWindow`. See [`App.d.ts`](../backend/cmd/wails/frontend/wailsjs/go/main/App.d.ts). |
| Window controls | [`skinnyWindowService.ts`](../services/skinnyWindowService.ts), [`fullscreenService.ts`](../services/fullscreenService.ts), and [`DesktopTitleBar.tsx`](../components/DesktopTitleBar.tsx) depend on v2 runtime calls. Skinny mode resizes the existing window. |
| External navigation | [`externalNavigation.ts`](../services/externalNavigation.ts) detects native mode and opens system-browser URLs. Spotify PKCE has a Go fallback for WebKit custom-scheme crypto limitations. |
| Tray and close | [`tray_supported.go`](../backend/cmd/wails/tray_supported.go) runs getlantern/systray on Windows/Linux; macOS excludes it because of an AppDelegate conflict. Hide/quit preference currently requires restart to affect native close handling. |
| Trust boundary | [`backend/internal/server/server.go`](../backend/internal/server/server.go) validates Host/Origin and permits specific Wails origins. Preserve this contract for REST, SSE, OAuth, and media. |
| Delivery | [`scripts/build-wails.ps1`](../scripts/build-wails.ps1), [macOS script](../scripts/build-wails-macos.sh), [Linux script](../scripts/build-wails-linux.sh), [CI](../.github/workflows/ci.yml), and [release workflow](../.github/workflows/release.yml) encode v2 commands, platform dependencies, artifact layouts, and version injection. |

## Improvements, capabilities, and ViiB benefits

Priority: **P0** migration requirement; **P1** first benefits after parity; **P2** separately scoped follow-up. Some APIs below also existed in v2: the benefit can be cleaner integration or adopting an unused capability, rather than a newly invented feature.

| Priority | Documented capability | Proposed use and expected benefit | Constraint or acceptance evidence |
|---|---|---|---|
| P0/P1 | Integrated tray, theme-aware icons, menus, and attached windows. [What's new](https://v3.wails.io/whats-new/) | Replace the native shell's external tray loop; add macOS menu-bar access; later expose Show, playback controls, and Quit. Fewer lifecycle workarounds and background access to music. | Verify Show/Quit and hide/restore on each desktop. Keep the browser executable's systray dependency while it still uses it. |
| P0 | Separate application/window construction, service registration, and object runtime calls. [Migration guide](https://v3.wails.io/migration/v2-to-v3/) | Inject app/window references into a small desktop service and centralize frontend native access. Easier testing and fewer scattered bridge assumptions. | Preserve all seven method contracts through adapters; regenerate bindings rather than hand-edit them. |
| P0/P1 | Window listeners and cancellable hooks. [Window events](https://v3.wails.io/features/windows/events/) | Use a close hook reading the current preference to make hide/quit changes take effect immediately; observe fullscreen/resize/focus for consistent controls. | Register hooks before relevant events; cancel in hooks, not listeners. Keep callbacks short and unsubscribe on disposal. Explicit Quit must bypass hide behavior. |
| P1 | Multiple independently managed windows. [Multiple windows](https://v3.wails.io/features/windows/multiple/) | Add a mini-player while the main library stays open; later detach queue or visualizer. Avoid repeatedly resizing the main library to skinny dimensions. | Share authoritative playback state; never mount a second independent audio engine. Test hide/close/reopen, focus, and multi-monitor restoration. |
| P0/P1 | Static binding analysis and generated wrappers preserving useful source metadata. [What's new](https://v3.wails.io/whats-new/) | Generate a typed desktop bridge during builds and check generated output for drift. Improve discoverability and catch API mismatches earlier. | Verify the pinned generator's full module-path output and runtime imports against Vite; do not assume v2 globals exist. |
| P0 | Customizable Taskfile-based build orchestration. [Build system](https://v3.wails.io/concepts/build-system/) | Consolidate frontend staging, icons, metadata, architecture builds, and packaging while retaining existing script entrypoints. Reduce duplicate build logic. | Port each v2 flag explicitly into tasks/configuration. `wails3 build` does not accept v2's platform/output/skipbindings/ldflags flags. |
| P1 | Native open/save/folder dialogs. [File dialogs](https://v3.wails.io/features/dialogs/file/) | Preserve support ZIP export; add a native library-folder picker and explicit export destinations. Avoid manual path entry in desktop mode. | Cancellation must have no side effects. Check Unicode paths, overwrite behavior, permission failures, and window ownership; retain browser alternatives. |
| P1 | Native file drops. [File drop](https://v3.wails.io/features/drag-and-drop/files/) | Offer drop-to-import for supported music files or folders, feeding the existing scan/import pipeline. Reduce steps to add local music. | Validate paths and supported formats; deduplicate and preview large imports. Native drops must remain distinct from in-app DJ drag gestures. |
| P1 | System-wide shortcuts distinct from focused-window key bindings. [Global shortcuts](https://v3.wails.io/features/keyboard/global-shortcuts/) | Optional Show player and play/pause shortcuts while another app has focus. | Make bindings configurable; handle conflicts and registration errors. Wayland depends on the desktop portal and may assign different keys. Keep DJ performance shortcuts focused by default. |
| P2 | Desktop notifications. [Notifications](https://v3.wails.io/features/notifications/overview/) | Notify when long scans, downloads, or enrichment finish while ViiB is hidden. | User preference, permission/platform checks, rate limiting, and no sensitive paths or account data in notification text. |
| P2 | Custom URL protocols. [Custom protocols](https://v3.wails.io/guides/distribution/custom-protocols/) | Optional links opening a specific playlist or song in the existing instance. | Validate scheme, action, and identifiers; no arbitrary command/path execution. This does not automatically replace Spotify's registered loopback callback. |
| P2 | In-app update facilities. [Updater](https://v3.wails.io/guides/updater/) | Reduce manual download/install effort after the release pipeline is qualified. | Separate project: signing trust anchor, correct platform/channel selection, verified artifacts, failed-install recovery, and restart policy. Keep existing signing/provenance requirements. |

WML is experimental and offers little immediate value for an established React UI; defer it. Wails also documents a [server build](https://v3.wails.io/guides/server-build/), but replacing ViiB's existing browser executable requires a separate parity study. Mobile and experimental integrations are outside this desktop migration. No reduced binary size or faster application startup is assumed.

## Migration design and invariants

Use `application.New`, an explicitly retained main window, and a registered desktop service. The [v2-to-v3 guide](https://v3.wails.io/migration/v2-to-v3/) maps the initialization, services, runtime, and frontend changes; apply them at the native boundary. Keep shared business logic in the existing HTTP/API packages.

Create a proposed `services/desktopBridge.ts` adapter for native capability detection, generated App calls, window controls, system-browser launch, and dialogs. Browser consumers receive explicit unavailable capabilities and existing browser behavior. Route current native consumers through this adapter before replacing its implementation. Verify generated imports and `@wailsio/runtime` against the selected release, including production asset loading.

Retain the loopback server, direct SSE/audio transport, OAuth port contract, PKCE fallback, and restrictive Host/Origin checks for initial parity. Measure v3 streaming before considering removal of the bypass. Never broaden trusted origins with a wildcard to make a prototype work. Retain application identity, data paths, WebView profile persistence, metadata, user settings, and single-instance activation. Register/check single-instance ownership early enough that a second launch does not fail merely because the first owns port 34115.

V3 configuration examples are not a complete build specification: the migration page shows `wails.json`, while the [build reference](https://v3.wails.io/concepts/build-system/) describes Taskfiles plus `build/config.yml`. Generate a disposable project using the pinned CLI and use its files and help output as the executable reference. Record discrepancies rather than copying incompatible examples. Use [installation guidance](https://v3.wails.io/quick-start/installation/) and `wails3 doctor` on each build host; verify actual minimum OS versions against that release before changing ViiB's support policy.

## Implementation work packages

All checkboxes are open. Each package should be reviewable separately. Dependencies follow the order below; optional features can ship independently after parity.

### 1. Compatibility baseline and version spike

- [ ] Record a v2 native baseline for window behavior, tray, OAuth, bindings, streams, and package installation. Reuse existing external-navigation, skinny-window, PKCE, and event-stream URL tests; add cases for missing native runtime and failures where coverage is absent.
- [ ] Select an exact v3 tag after reviewing its changelog. Pin CLI and Go module to that tag and a compatible frontend runtime in the lockfile. Record tag, commit, tool versions, platform libraries, and license/dependency changes. Avoid floating `latest` in CI.
- [ ] Build a disposable React/Vite prototype outside production entrypoints. Prove one generated method, one API request, one SSE stream, audio range/seek, native save dialog, and single-instance reactivation.
- [ ] Run Windows, macOS, and Linux probes. Choose GTK4 as the intended Linux target; use GTK3 only as a documented temporary fallback with an explicit v3.1 removal task.

Exit: a recorded API/build mapping and working native prototype on all three OS families. Stop cutover if an essential platform behavior fails. This research has not chosen or tested a specific v3 tag.

### 2. Isolate and migrate the desktop bridge

- [ ] Introduce the adapter and move titlebar, fullscreen, skinny mode, external navigation, settings/support export, Spotify PKCE, and streaming URL consumers behind it. Inventory all `wailsjs`, `window.go`, and `window.runtime` references, including tests and components.
- [ ] Convert `backend/cmd/wails/main.go` initialization, assets/API proxy, lifecycle, single-instance handling, window options, and App methods to v3 services/managers. Keep the main-window reference explicit.
- [ ] Regenerate bindings, update imports/types and runtime package loading, and replace v2 detection. Ensure browser builds never initialize the native runtime accidentally.
- [ ] Preserve cancellation/error behavior and all existing settings/data. Confirm background workers and HTTP streams end before database closure; test startup failures and a busy OAuth port.

Exit: native feature parity and passing browser regressions, with no reliance on v2 globals. The prior v2 release remains reproducible from its recorded commit/tag.

### 3. Native lifecycle and tray consolidation

- [ ] Replace only the desktop target's external tray with Wails tray menus and platform icons. Prove macOS lifecycle compatibility before enabling hide-to-tray there.
- [ ] Implement current-preference close hooks and an explicit quitting state. Specify macOS Dock/menu-bar behavior so hiding the last window keeps background playback alive when requested.
- [ ] Test Quit, native close, custom close, second launch, restore from minimized/hidden/fullscreen, and shutdown during playback/download/scan. Tray callbacks must use the same actions as the UI.

Exit: lifecycle tests pass per platform, no stranded process, double shutdown, or lost background controls. [Shortcut documentation](https://v3.wails.io/features/keyboard/global-shortcuts/) specifically warns that macOS terminate-after-last-window can also terminate a hidden window workflow.

### 4. Build, CI, and release migration

- [ ] Port the three build scripts to generated/custom Taskfiles and `build/config.yml`; retain ViiB names, version/build-time injection, icons, frontend staging, output archives, installers, and architecture checks.
- [ ] Update both GitHub workflows, module/runtime dependency inventories, caches, Linux packages/tags, signing/notarization inputs, checksums, and provenance paths. Generate bindings before frontend compilation; fail CI on stale generated output.
- [ ] Keep v3 candidate builds distinct until qualified; do not let an optional/continue-on-error native job count as a release gate. Preserve the browser build matrix.

Exit: clean-checkout builds and installable packages reproduce the existing artifact contract. Qualify Windows amd64/arm64, macOS amd64/arm64 plus universal packaging, and Linux amd64/arm64. Record unsupported/test-unavailable cells explicitly; compilation alone does not qualify runtime behavior.

### 5. First product improvements

- [ ] Ship the native folder picker, then file-drop import, using the current import validation and progress UI.
- [ ] Add optional global Show/playback shortcuts with conflict feedback and a disable setting.
- [ ] Prototype a mini-player route/window with one playback authority. The main playback owner must remain alive while hidden; reopening either view rehydrates state. If the owner closes, explicitly transfer ownership or stop playback. Events alone do not synchronize independent React stores or audio contexts.
- [ ] Measure mini-player idle CPU/memory and playback continuity; make detached visualizer/queue work a later decision.

Exit: features meet their table criteria without duplicate audio, queue divergence, shortcut surprises, or browser regressions. Notifications, protocols, and updater stay separate follow-ups.

## Verification and rollout

| Gate | Required evidence |
|---|---|
| Automated contracts | `npm run check`, existing relevant Playwright flows, and applicable Go tests/race checks from CI; new adapter tests for browser/native/error behavior, binding results, close preference, stream resolution, and shutdown ordering. |
| Native playback | Local/Plex/Spotify as applicable: play, pause, seek/range requests, long streams, queue transitions, hide/restore, and stream reconnect. Exercise scan/enrichment/download SSE over multiple events, not just initial connection. |
| Native UI | Frameless titlebar dragging/buttons, minimum sizes, skinny restore, fullscreen, always-on-top, DPI/multiple monitors, external-browser auth, support ZIP save/cancel, and folder import. |
| Lifecycle/security | Second launch, occupied port, close/hide/Quit, sleep/wake reconnect, WebView profile persistence, allowed actual v3 origins, rejection of untrusted Host/Origin, and shutdown with work in progress. |
| Packaging | Install/launch/upgrade/uninstall on supported hosts, architecture verification, signing/notarization policy, checksum/inventory/provenance verification, and no missing assets/runtime package. |
| Measurements | Compare v2/v3 cold start, idle RSS/CPU, import UI responsiveness, stream time-to-first-event, and playback dropouts using identical fixtures/hosts. Record values and agree acceptable regressions before cutover. |

Extend [`scripts/smoke-test.ps1`](../scripts/smoke-test.ps1): it currently checks process/window liveness and force-stops the app. Add binding/API/stream checks and graceful Quit verification; provide equivalent macOS/Linux procedures. Store dated results with OS, architecture, display server, WebView/toolchain versions, fixtures, and package hash. Browser automation cannot prove native tray, dialogs, or WebView audio behavior.

Release a clearly marked v3 candidate only after packages 1–4 pass. Promote after native playback and lifecycle qualification, then enable package 5 features incrementally. Preserve the last qualified v2 artifacts and build instructions. Keep the migration free of required data-schema changes; test returning to v2 against a copied profile. If cutover causes stream stalls, OAuth failure, data incompatibility, or loss of background control, withdraw the candidate and restore v2 distribution while retaining diagnostic evidence.

## Planning uncertainties

The exact v3 tag/runtime pairing, minimum OS support, actual WebView origins, Linux tray/Wayland behavior, streaming characteristics, and macOS background lifecycle remain prototype gates. This document reviews source and official documentation; it does not claim executed v3 builds, live OAuth, performance results, or platform qualification. Estimate schedule after package 1: the migration guide's small-app time estimate does not cover ViiB's multi-platform release and media contracts.
