# Wails v3 upgrade assessment and implementation plan

Research date: 2026-10-23. Reviewed the same day against the repository at `fbfaac1` and the Wails `v3.0.0-beta.34` tag (docs source and selected framework source). Status: proposed; no runtime migration implemented.

## Recommendation

Upgrade the existing v2 shell to the current v2 release first, then run a pinned-version v3 prototype, then migrate the desktop shell in stages. Keep the v2 release path until v3 passes native qualification.

The strongest v3-only benefits for ViiB are an integrated tray (including macOS, where ViiB has none today), cancellable close hooks, multiple windows, and global shortcuts. Several "improvements" commonly attributed to v3 already exist in v2.11 and should not be gated on the migration: a native folder dialog (`runtime.OpenDirectoryDialog`), native file drop (`EnableFileDrop`/`runtime.OnFileDrop`), and custom protocol registration (`wails.json` `protocols`).

Framework status at review time:

- Wails labels v3 **Beta**. Its compatibility contract states Beta APIs "aim for stability" but prerelease defects and announced changes may still land before v3.0.0. It lists Go 1.25+, Windows amd64/arm64 (WebView2), macOS Intel/Apple Silicon, and Linux amd64/arm64. Linux defaults to GTK4/WebKitGTK 6.0; GTK3/WebKit2GTK 4.1 is a `-tags gtk3` legacy option through v3.0.x, removed in v3.1. [Wails status](https://v3.wails.io/status/)
- v3 betas are automated nightly builds from `master` (beta.31–beta.34 shipped 2026-10-18 to 2026-10-20). Pinning is mandatory, and the plan needs an explicit re-pin cadence rather than tracking each nightly.
- v2 is still maintained: v2.17.0 (2026-10-18) follows ViiB's pinned v2.11.0 and includes `wails dev` IPC origin/DNS-rebinding hardening, a macOS/Linux C-string leak fix, the upstream `mimetype` replacement, and a macOS `EnableAutoplayWithoutUserAction` preference. [v2.17.0 release](https://github.com/wailsapp/wails/releases/tag/v2.17.0)

This assessment distinguishes documented framework capabilities from proposed ViiB uses. Benefits below are engineering expectations; performance, compatibility, and user experience improvements require measurement. An upgrade alone does not establish better audio latency, codec coverage, DJ timing, Spotify compatibility, or DSP accuracy.

## Current repository baseline

| Area | Current implementation and migration significance |
|---|---|
| Dependencies | [`backend/go.mod`](../backend/go.mod) pins Wails v2.11.0 and Go 1.26.10; CI and release install the v2.11.0 CLI. Go already exceeds the documented v3 minimum. |
| Native entrypoint | [`backend/cmd/wails/main.go`](../backend/cmd/wails/main.go) embeds React/Vite output, opens the database, binds the loopback API server, and configures one window through `wails.Run`. Shutdown cancels streams, closes the API, then shuts down HTTP before the deferred database close. Preserve that order and startup failure handling. |
| Browser distribution | [`backend/cmd/viib/main.go`](../backend/cmd/viib/main.go) is a separate executable sharing the HTTP backend and the same Vite bundle. Its browser UI must continue to work without a native runtime, and it keeps its own getlantern/systray tray. |
| Transport | Ordinary `/api` calls use the asset-server proxy. [`services/eventStreamURL.ts`](../services/eventStreamURL.ts) resolves direct loopback SSE/audio URLs because the native asset path buffers streaming responses. The native API port defaults to 34115 for the Spotify OAuth callback. |
| Bridge | Seven generated App methods exist (see [`App.d.ts`](../backend/cmd/wails/frontend/wailsjs/go/main/App.d.ts)). Only four have frontend callers: `GetServerURL`, `GenerateSpotifyCodeChallenge`, `SetWindowCloseAction`, and `SaveSupportBundle`. `ShowWindow` is called only from Go (tray, second instance); `GetVersion` and `GetDataDir` have no callers. |
| Native detection | [`utils.ts`](../utils.ts) `isWailsEnvironment` and [`externalNavigation.ts`](../services/externalNavigation.ts) check `wails.localhost`/`wails` hostnames, the `wails:` protocol, or the v2 `window.go`/`window.runtime` globals. `eventStreamURL.ts`, `getOAuthCallbackUrl` in `utils.ts`, and [`spotifyService.ts`](../services/spotifyService.ts) call bindings through `window.go.main.App` directly and fall back silently when it is absent. |
| Window controls | [`skinnyWindowService.ts`](../services/skinnyWindowService.ts), [`fullscreenService.ts`](../services/fullscreenService.ts), [`DesktopTitleBar.tsx`](../components/DesktopTitleBar.tsx), and [`Settings.tsx`](../pages/Settings.tsx) dynamically import the v2 `wailsjs` runtime/bindings. `DesktopTitleBar.tsx` and [`Player.tsx`](../components/Player.tsx) use `--wails-draggable`, which v3 retains. Windows/Linux are frameless with a custom title bar; macOS uses the native title bar. Skinny mode resizes the existing window. |
| External navigation | `externalNavigation.ts` opens system-browser URLs in native mode. Spotify PKCE has a Go fallback because WebKit may omit SubtleCrypto on the custom-scheme page. |
| Tray and close | [`tray_supported.go`](../backend/cmd/wails/tray_supported.go) runs getlantern/systray on Windows/Linux; [`tray_darwin.go`](../backend/cmd/wails/tray_darwin.go) disables both the tray and hide-on-close on macOS because of an AppDelegate conflict. The close preference defaults to hide and requires a restart to take effect. If the tray does not initialize within 5 s, the app quits. |
| Single instance | v2 evaluates `SingleInstanceLock` inside `wails.Run`, after `main` has opened `library.db` and bound `127.0.0.1:34115`. By code reading, a second launch on the default port exits on the bind error before it can signal the first instance, so `OnSecondInstanceLaunch` is effectively unreachable. Combined with hide-on-close, a hidden window whose tray icon is not visible (for example GNOME without an AppIndicator extension) may have no restore path. Confirm in package 1. |
| Trust boundary | [`backend/internal/server/server.go`](../backend/internal/server/server.go) validates Host/Origin and Sec-Fetch-Site. It trusts exactly `wails://wails` and `http://wails.localhost` as desktop origins. Preserve this contract for REST, SSE, OAuth, and media. |
| Delivery | Build scripts: [`build-wails.ps1`](../scripts/build-wails.ps1), [`build-wails-windows-arm64.ps1`](../scripts/build-wails-windows-arm64.ps1), [`build-wails-macos.sh`](../scripts/build-wails-macos.sh) (including `universal` via `lipo`), [`build-wails-linux.sh`](../scripts/build-wails-linux.sh), and [`dev-wails.ps1`](../scripts/dev-wails.ps1). Workflows: [CI](../.github/workflows/ci.yml), [release](../.github/workflows/release.yml), and [Windows build](../.github/workflows/build-windows.yml). All use v2 `wails build -platform … -ldflags "-s -w"` (Linux adds `-tags webkit2_41`; release adds `-skipbindings`). Native builds do not inject `main.Version`/`BuildTime`, so the native app reports `dev`. |
| Released native artifacts | Windows x64 NSIS installer (Authenticode-signed only when credentials exist), macOS arm64 and x64 DMGs (separate, ad-hoc signed, not notarized), and Linux amd64 `.deb` plus `.tar.gz`. The `.deb` depends on `libgtk-3-0, libwebkit2gtk-4.1-0, libayatana-appindicator3-1`. Windows arm64, Linux arm64, and macOS universal native builds exist only in local scripts; arm64 is released only for the browser build. |

## Improvements, capabilities, and ViiB benefits

Priority: **P0** migration requirement; **P1** first benefits after parity; **P2** separately scoped follow-up. The "v2.11?" column marks whether ViiB could adopt the capability without migrating.

| Priority | Documented capability | v2.11? | Proposed use and expected benefit | Constraint or acceptance evidence |
|---|---|---|---|---|
| P0/P1 | Integrated tray, theme-aware/template icons, menus, and attached windows. [Tray](https://v3.wails.io/features/menus/systray/) | No | Replace the native shell's external tray loop; add macOS menu-bar access; later expose Show, playback controls, and Quit. Fewer lifecycle workarounds and background access to music. | Verify Show/Quit and hide/restore on each desktop. Wails has no tray-support probe, and some Linux desktops show no tray, so hide-on-close must not strand the window there. Keep the browser executable's getlantern dependency while it still uses it. |
| P0 | Separate application/window construction, service registration, and object runtime calls. [Migration guide](https://v3.wails.io/migration/v2-to-v3/) | No | Inject app/window references into a small desktop service and centralize frontend native access. Easier testing and fewer scattered bridge assumptions. | Preserve the four frontend-called contracts through the adapter. Drop `GetVersion`/`GetDataDir` and stop exporting `ShowWindow` to JavaScript unless a consumer is found. Regenerate bindings rather than hand-editing them. |
| P0/P1 | Window listeners and cancellable hooks. [Window events](https://v3.wails.io/features/windows/events/) | No | Use a `WindowClosing` hook that reads the current preference so hide/quit changes take effect immediately; observe fullscreen/resize/focus for consistent controls. | Only `RegisterHook` can `e.Cancel()`; listeners cannot. Keep callbacks short and unsubscribe on disposal. Explicit Quit must bypass hide behavior. |
| P0 | Single instance enforced in `application.New`. [Single instance](https://v3.wails.io/guides/single-instance/) | Partly | Fix the unreachable second-launch path: call `application.New` before opening the database or binding port 34115. | On a second instance, `New` notifies the first and calls `os.Exit`, skipping deferred cleanup. It must run before any resource that needs closing. The asset proxy needs a late-bound target URL. Treat forwarded args as untrusted. |
| P1 | Multiple independently managed windows. [Multiple windows](https://v3.wails.io/features/windows/multiple/) | No | Add a mini-player while the main library stays open; later detach queue or visualizer. Avoid repeatedly resizing the main library to skinny dimensions. | Each window is a separate WebView and JS context. Share authoritative playback state; never mount a second independent audio engine. Test hide/close/reopen, focus, and multi-monitor restoration. |
| P0/P1 | Static binding analysis and generated wrappers preserving comments and parameter names. [What's new](https://v3.wails.io/whats-new/) | No | Generate a typed desktop bridge during builds and check generated output for drift. Improve discoverability and catch API mismatches earlier. | Bindings move from `wailsjs/go/main/App` to a module-path tree under `frontend/bindings`. Verify the pinned generator's output and `@wailsio/runtime` imports against Vite; v3 does not inject `window.go`. |
| P0 | Taskfile-based build orchestration with `build/config.yml`. [Build system](https://v3.wails.io/concepts/build-system/) | No | Required work, not a benefit by itself. Decide whether to adopt generated Taskfiles or keep ViiB scripts calling `wails3 generate bindings`, the syso/icon generators, and `go build`. Choose whichever duplicates less logic. | `wails3 build` forwards only `--tags`; it has no `-platform`, `-o`, `-skipbindings`, `-ldflags`, `-clean`, `-debug`, or `-icon`. It targets the host OS. Platform, universal, and package steps are `wails3 task darwin:…`/`windows:…`/`linux:…`. |
| P1 | Native open/save/folder dialogs. [File dialogs](https://v3.wails.io/features/dialogs/file/) | Yes | Preserve support ZIP export; add a native library-folder picker and explicit export destinations. Avoid manual path entry in desktop mode. | Can ship on v2 now. Cancellation must have no side effects. Check Unicode paths, overwrite behavior, permission failures, and window ownership; retain browser alternatives. |
| P1 | Native file drops (`EnableFileDrop`, `data-file-drop-target`). [File drop](https://v3.wails.io/features/drag-and-drop/files/) | Yes | Offer drop-to-import for supported music files or folders, feeding the existing scan/import pipeline. Reduce steps to add local music. | Can ship on v2 now, but drop-target markup and events differ in v3, so build it behind the adapter. Validate paths and formats; deduplicate and preview large imports. Native drops must remain distinct from in-app DJ drag gestures. |
| P1 | System-wide shortcuts distinct from focused-window key bindings. [Global shortcuts](https://v3.wails.io/features/keyboard/global-shortcuts/) | No | Optional Show player and play/pause shortcuts while another app has focus. | Configurable bindings. Conflicts differ by platform: Windows/X11 return an error, macOS silently co-registers, and Wayland's portal may assign different keys while `IsRegistered` reports the request. Keep DJ performance shortcuts focused by default. |
| P2 | Desktop notifications. [Notifications](https://v3.wails.io/features/notifications/overview/) | No | Notify when long scans, downloads, or enrichment finish while ViiB is hidden. | User preference; macOS requires authorization (verify with the ad-hoc-signed bundle); rate limiting; no sensitive paths or account data in notification text. |
| P2 | Custom URL protocols. [Custom protocols](https://v3.wails.io/guides/distribution/custom-protocols/) | Yes | Optional links opening a specific playlist or song in the existing instance. | Depends on the single-instance fix. Validate scheme, action, and identifiers; no arbitrary command/path execution. This does not replace Spotify's registered loopback callback. |
| P2 | In-app updater with pluggable providers and signature verification. [Updater](https://v3.wails.io/guides/updater/) | No | Reduce manual download/install effort after the release pipeline is qualified. | Separate project. The updater swaps the running binary, which conflicts with `.deb` package ownership and macOS bundle signatures and bypasses the NSIS installer. It also needs a signing trust anchor, channel selection, failed-install recovery, and restart policy. Keep existing provenance requirements. |

v3 also adds [Streams](https://v3.wails.io/guides/streams/), WebSocket-like channels over the asset server that avoid a TCP port. They could eventually replace native SSE, but the browser build still needs SSE, so adopting them means two transports; defer. WML is experimental and its documentation page is a placeholder; defer it. The [server build](https://v3.wails.io/guides/server-build/) would need a separate parity study before it could replace ViiB's browser executable. Mobile and experimental integrations are outside this desktop migration. No reduced binary size or faster application startup is assumed.

## Migration design and invariants

Use `application.New`, an explicitly retained main window, and a registered desktop service. The [v2-to-v3 guide](https://v3.wails.io/migration/v2-to-v3/) maps initialization (`OnStartup` becomes `ServiceStartup`; `OnShutdown`/`PostShutdown`/`ShouldQuit` live on application options), services, runtime, and frontend changes; apply them at the native boundary. Keep shared business logic in the existing HTTP/API packages. `WebviewUserDataPath` moves to application-level `Windows` options and must keep pointing at the existing data directory so WebView2 profiles persist.

Create a proposed `services/desktopBridge.ts` adapter for native capability detection, generated App calls, window controls, system-browser launch, and dialogs. Browser consumers receive explicit unavailable capabilities and existing browser behavior. Route every current native consumer through this adapter before replacing its implementation. The list is in the baseline table, including `utils.ts`, `spotifyService.ts`, `Settings.tsx`, and the tests that stub `window.go`. Because the browser and native builds share one Vite bundle, load `@wailsio/runtime` only by dynamic import after native detection, and verify it is never fetched or initialized in browser mode.

The adapter migration closes a silent-failure path. Without `window.go`, `eventStreamURL.ts` returns relative URLs, so SSE and audio would flow through the asset server. At beta.34 the Windows asset-server response writer buffers the whole response and does not implement `http.Flusher`; its source comment says streaming "cannot work on Windows today". Scans, downloads, and enrichment would then appear to hang without an error. The same absence would also skip the native PKCE hash on macOS. Adapter tests must fail when a native build cannot resolve the server URL, not fall back.

Retain the loopback server, direct SSE/audio transport, OAuth port contract, PKCE fallback, and restrictive Host/Origin checks. The direct-stream bypass is required on Windows, not merely a candidate for measurement. Re-check macOS/Linux, which write through without buffering in v3, before considering removal there. Never broaden trusted origins with a wildcard to make a prototype work.

v3 changes the desktop origin on macOS and Linux from `wails://wails` to `wails://localhost`; Windows stays `http://wails.localhost`. `IsTrustedOrigin` accepts the `wails` scheme only with hostname `wails`, so v3 cross-origin SSE and media requests to the loopback server would be rejected with 403 on macOS/Linux. Add exactly `wails://localhost` (no port) with tests. Keep rejecting look-alikes, and retire `wails://wails` only after v2 is withdrawn. Frontend detection still matches through the `wails:` protocol.

Retain application identity, data paths, WebView profile persistence, metadata, user settings, and the close-preference file. Order startup so `application.New` (single-instance check) runs before logger/database initialization and port binding. Then construct the API server, and give the asset proxy its target after the listener binds.

V3 configuration examples are not a complete build specification: the migration page still shows a `wails.json`, while the [build reference](https://v3.wails.io/concepts/build-system/) describes Taskfiles plus `build/config.yml` and says there is no `build/build.json`. Generate a disposable project using the pinned CLI and use its files and help output as the executable reference. Record discrepancies rather than copying incompatible examples. Run the [installation guidance](https://v3.wails.io/quick-start/installation/) and `wails3 doctor` on each build host.

Linux needs an explicit support-policy decision rather than a default. The GTK4 stack requires Ubuntu 24.04+ or Debian 13+. Ubuntu 22.04, Debian 12, Fedora ≤ 39, and RHEL 9 must use the `gtk3` legacy build, which ends at v3.1. The current `.deb` targets WebKit2GTK 4.1 and therefore installs on 22.04 and Debian 12. Choose one:

- **GTK4 at cutover (recommended):** raise the documented Linux floor to Ubuntu 24.04/Debian 13 in the release notes, and replace the `.deb` dependencies after confirming v3's Linux tray and runtime libraries.
- **Time-boxed GTK3:** ship a `-tags gtk3` build to hold the current floor. Add a dated task to drop it before adopting v3.1 or by Ubuntu 22.04's end of standard support (April 2027), whichever comes first.

## Implementation work packages

All checkboxes are open. Each package should be reviewable separately. Dependencies follow the order below; optional features can ship independently after parity.

### 0. Current v2 baseline (independent of v3)

- [ ] Upgrade the Go module and every CLI install (CI, release, Windows build workflow, local scripts) from v2.11.0 to the current v2 release (v2.17.0 at review time), then rerun native smoke tests.
- [ ] Fix second-launch activation on v2: check single-instance ownership before the 34115 bind, or treat a bind failure on the default port as "already running" and signal the first instance. Add a test for the occupied-port path.
- [ ] Inject `main.Version`/`BuildTime` in native builds so baselines and support bundles identify the build.
- [ ] Optionally ship the native folder picker and file drop on v2 behind a small adapter, so package 5 does not depend on the migration.

Exit: a current, qualified v2 release that serves as the rollback target and the comparison baseline for package 1.

### 1. Compatibility baseline and version spike

- [ ] Record a v2 native baseline for window behavior, tray, OAuth, bindings, streams, second launch, and package installation. Reuse existing external-navigation, skinny-window, PKCE, event-stream URL, and player-slice tests; add cases for missing native runtime and failures where coverage is absent.
- [ ] Select an exact v3 beta tag after reviewing its changelog, and define when to re-pin (for example, only for a needed fix or monthly). Pin CLI and Go module to that tag and a compatible `@wailsio/runtime` version in the lockfile. Record tag, commit, tool versions, platform libraries, and license/dependency changes. Avoid floating `latest` in CI.
- [ ] Build a disposable React/Vite prototype outside production entrypoints. Prove one generated method, one API request, one SSE stream and audio range/seek over the direct loopback path with the v3 origin, native save dialog, and single-instance reactivation with port 34115 held.
- [ ] Run Windows, macOS, and Linux probes. Record the Linux GTK decision from the design section; if GTK3 is used, add the dated removal task.

Exit: a recorded API/build mapping and working native prototype on all three OS families. Stop cutover if an essential platform behavior fails. This research has not chosen or tested a specific v3 tag.

### 2. Isolate and migrate the desktop bridge

- [ ] Introduce the adapter and move every native consumer behind it, including titlebar, fullscreen, skinny mode, external navigation, settings/support export, Spotify PKCE, OAuth callback URL, and streaming URL. Inventory all `wailsjs`, `window.go`, and `window.runtime` references, including tests and components. This step can land on v2 first.
- [ ] Convert `backend/cmd/wails/main.go` initialization, assets/API proxy (v3 `AssetOptions.Handler`/`Middleware`), lifecycle, single-instance ordering, window options, and App methods to v3 services/managers. Keep the main-window reference explicit.
- [ ] Add `wails://localhost` to the server trust boundary with positive and negative tests.
- [ ] Regenerate bindings, update imports/types and runtime package loading, and replace v2 detection. Ensure browser builds never initialize the native runtime.
- [ ] Preserve cancellation/error behavior and all existing settings/data. Confirm background workers and HTTP streams end before database closure; test startup failures and a busy OAuth port.

Exit: native feature parity and passing browser regressions, with no reliance on v2 globals. The package 0 v2 release remains reproducible from its recorded commit/tag.

### 3. Native lifecycle and tray consolidation

- [ ] Replace only the desktop target's external tray with Wails tray menus and platform icons (template icon on macOS). Prove macOS lifecycle compatibility before enabling hide-to-tray there; this is new behavior on macOS, not parity.
- [ ] Implement current-preference close hooks and an explicit quitting state. Specify macOS Dock/menu-bar behavior and leave `ApplicationShouldTerminateAfterLastWindowClosed` unset so hiding the last window keeps background playback alive when requested.
- [ ] Define the Linux fallback when no tray host is visible: default to quit-on-close or keep a reachable restore path. Remove the current "quit if the tray is not ready in 5 s" behavior only if a replacement exists.
- [ ] Test Quit, native close, custom close, second launch, restore from minimized/hidden/fullscreen, and shutdown during playback/download/scan. Tray callbacks must use the same actions as the UI.

Exit: lifecycle tests pass per platform, with no stranded process, double shutdown, or lost background controls. The [shortcut documentation](https://v3.wails.io/features/keyboard/global-shortcuts/) warns that on macOS `Hide()` counts as closing the last window, so `ApplicationShouldTerminateAfterLastWindowClosed: true` would quit a hidden-window workflow.

### 4. Build, CI, and release migration

- [ ] Port the four `build-wails*` scripts and `dev-wails.ps1` using the approach chosen in the capability table. Retain ViiB names, icons, frontend staging, output archives, installers, and architecture checks; add version/build-time injection if package 0 has not.
- [ ] Update all three workflows (`ci.yml`, `release.yml`, `build-windows.yml`): CLI install, module/runtime dependency inventories, caches, Linux packages and `.deb` `Depends`, the `webkit2_41` → GTK4/`gtk3` tag change, NSIS inputs, macOS ad-hoc signing and DMG creation, checksums, and provenance paths. Generate bindings before frontend compilation; fail CI on stale generated output.
- [ ] Keep v3 candidate builds distinct until qualified. Several native CI build steps are `continue-on-error`; do not let such a step count as a release gate. Preserve the browser build matrix.

Exit: clean-checkout builds and installable packages reproduce the released artifact contract: Windows x64 installer, macOS arm64 and x64 DMGs, and Linux amd64 `.deb`/`.tar.gz`. Windows arm64, Linux arm64, and macOS universal native builds are script-only today. Build them where runners allow, and record any untested target as script-only rather than qualified. Compilation alone does not qualify runtime behavior.

### 5. First product improvements

- [ ] Ship the native folder picker, then file-drop import, using the current import validation and progress UI (skip if delivered in package 0).
- [ ] Add optional global Show/playback shortcuts with conflict feedback and a disable setting.
- [ ] Prototype a mini-player route/window with one playback authority. The main playback owner must remain alive while hidden; reopening either view rehydrates state. If the owner closes, explicitly transfer ownership or stop playback. Events alone do not synchronize independent React stores or audio contexts.
- [ ] Measure mini-player idle CPU/memory and playback continuity, including WebView throttling of timers and `requestAnimationFrame` in the hidden main window. Make detached visualizer/queue work a later decision.

Exit: features meet their table criteria without duplicate audio, queue divergence, shortcut surprises, or browser regressions. Notifications, protocols, and updater stay separate follow-ups.

## Verification and rollout

| Gate | Required evidence |
|---|---|
| Automated contracts | `npm run check`, existing relevant Playwright flows, and applicable Go tests/race checks from CI. Add adapter tests for browser/native/error behavior, binding results, close preference, stream resolution (including failure when the native server URL is unavailable), v3 origin acceptance/rejection, and shutdown ordering. |
| Native playback | Local/Plex/Spotify as applicable: play, pause, seek/range requests, long streams, queue transitions, hide/restore, and stream reconnect. Exercise scan/enrichment/download SSE over multiple events, not just initial connection. |
| Native UI | Frameless titlebar dragging/buttons (Windows/Linux), native titlebar (macOS), minimum sizes, skinny restore, fullscreen, always-on-top, DPI/multiple monitors, external-browser auth, support ZIP save/cancel, and folder import. |
| Lifecycle/security | Second launch with port 34115 held, occupied port, close/hide/Quit, sleep/wake reconnect, WebView profile persistence, the actual v3 origins allowed, rejection of untrusted Host/Origin, and shutdown with work in progress. |
| Packaging | Install/launch/upgrade/uninstall on supported hosts, the Linux distribution floor chosen above, architecture verification, current signing policy (ad-hoc macOS, conditional Authenticode), checksum/inventory/provenance verification, and no missing assets/runtime package. |
| Measurements | Compare package 0 v2 and v3 cold start, idle RSS/CPU, import UI responsiveness, stream time-to-first-event, and playback dropouts using identical fixtures/hosts. Record values and agree acceptable regressions before cutover. |

Extend [`scripts/smoke-test.ps1`](../scripts/smoke-test.ps1), which currently checks process/window liveness and force-stops the app. Add binding/API/stream checks and graceful Quit verification, and provide equivalent macOS/Linux procedures. Store dated results with OS, architecture, display server, WebView/toolchain versions, fixtures, and package hash. Browser automation cannot prove native tray, dialogs, or WebView audio behavior.

Release a clearly marked v3 candidate only after packages 1–4 pass. Promote after native playback and lifecycle qualification, then enable package 5 features incrementally. Preserve the last qualified v2 artifacts and build instructions. Keep the migration free of required data-schema changes, and test returning to v2 against a copied profile. If cutover causes stream stalls, OAuth failure, data incompatibility, or loss of background control, withdraw the candidate and restore v2 distribution while retaining diagnostic evidence.

## Planning uncertainties

Prototype gates remain for the exact v3 tag/runtime pairing, Linux tray behavior across desktops and Wayland, macOS/Linux streaming characteristics, macOS background lifecycle, and macOS notification authorization with ad-hoc signing. The v3 origins and Windows buffering are confirmed from beta.34 source, not from a running build. The v2 second-launch defect is from code reading and needs a reproduction. This document reviews source and official documentation; it does not claim executed v3 builds, live OAuth, performance results, or platform qualification. Estimate schedule after package 1: the migration guide's "1-4 hours for typical applications" does not cover ViiB's multi-platform release and media contracts.
