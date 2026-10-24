# Wails v2.17 upgrade plan

Research date: 2026-10-23. Base: `main` at `0d6b04a`. Status: proposed; no dependency change implemented.

## Decision

Stay on Wails v2 and upgrade from v2.11.0 to v2.17.0 (released 2026-10-18). Defer Wails v3: it is still a nightly-released Beta, and its main benefits (macOS tray, multiple windows, global shortcuts) are features rather than fixes for ViiB's 1.0 release. The v3 assessment is kept on the `docs/wails-v3-upgrade-plan` branch. Revisit v3 when v3.0.0 is tagged stable, when a v3-only feature becomes a product priority, or when v2 moves to security-only maintenance.

The upgrade is deliberately small: one dependency/tooling PR with no application behavior changes. Fixes that v2 already makes possible are separate follow-up PRs, so a regression can be traced to the framework bump or to ViiB code, not both.

## Relevant changes from v2.11.0 to v2.17.0

Sources: the [v2 changelog](https://github.com/wailsapp/wails/blob/v2.17.0/website/src/pages/changelog.mdx) and release notes for [v2.12.0](https://github.com/wailsapp/wails/releases/tag/v2.12.0), [v2.13.0](https://github.com/wailsapp/wails/releases/tag/v2.13.0), [v2.14.0](https://github.com/wailsapp/wails/releases/tag/v2.14.0), and [v2.17.0](https://github.com/wailsapp/wails/releases/tag/v2.17.0), which also covers v2.15.0 and v2.16.0 (tagged without GitHub releases).

| Version | Change | ViiB relevance |
|---|---|---|
| v2.12.0 | Fixed WebView crash on macOS 26 (Tahoe) during rapid UI updates | High: DJ mode, visualizer, and scan progress update rapidly; users on macOS 26 are exposed today |
| v2.12.0 | Fixed Linux crash on panic in JS-bound Go methods (WebKit overrode signal handlers) | Medium: a panic in a bound App method currently crashes the Linux app instead of returning an error |
| v2.12.0 | Fixed macOS clipboard mojibake (pbpaste/pbcopy `LANG`) | Low: affects any non-ASCII clipboard text |
| Between tags | Notifications runtime API. The current changelog files it under v2.11.0, but the v2.11.0 tag's runtime wrapper does not contain it and v2.17.0's does | None now; additive generated-wrapper change (see below) |
| v2.13.0 | Fixed nil-pointer crash in `Quit` when called before `Run` or after an early return | Low: relevant to the tray quit path during failed startup |
| v2.14.0 | Fixed nil-pointer crash and swallowed network error when downloading the WebView2 bootstrapper | Medium: Windows hosts without WebView2 exited instead of showing the install dialog |
| v2.15.0 | Added macOS `EnableAutoplayWithoutUserAction` webview preference | None by default; do not enable without a product reason |
| v2.16.0 | Fixed macOS/Linux memory leak: C strings for dialogs, menus, and window setup were never freed | Low: small, but affects every support-bundle save dialog |
| v2.17.0 | Hardened `wails dev` IPC against cross-origin and DNS-rebinding attacks (Host allowlist, same-origin upgrade, capability cookie) | Medium for contributors: before this, a malicious page in the developer's browser could call the bound Go API during `wails dev` |
| v2.17.0 | `wails init` warns that remote templates run code | None: ViiB does not use `wails init` |
| v2.17.0 | Windows notifications no longer fall back to PowerShell | None: ViiB does not send notifications |
| v2.17.0 | Replaced the `wailsapp/mimetype` fork with upstream `gabriel-vasile/mimetype` v1.4.16 | Low: asset MIME detection for the embedded frontend; verify fonts, WASM, and audio assets still get correct types |

## Compatibility review

Compared at the source level between the two tags:

- **Go API:** `pkg/options/options.go` and `pkg/options/windows` are byte-identical. `pkg/options/mac` only adds `DisableEscapeExitsFullscreen`. `pkg/options/linux` reorders the `WebviewGpuPolicy` constants so the values match WebKit (`OnDemand`=0, `Always`=1); `Never` stays 2. ViiB sets no Linux options, so both versions use `Never` (the [#2977](https://github.com/wailsapp/wails/issues/2977) workaround) and nothing changes. Note this if Linux options are ever added.
- **Webview origins:** unchanged. macOS/Linux still use `wails://wails/` and Windows still uses `http://wails.localhost/`, so the trust boundary in [`server.go`](../backend/internal/server/server.go) and native detection in [`utils.ts`](../utils.ts) and [`externalNavigation.ts`](../services/externalNavigation.ts) need no change.
- **Go version:** the Wails module's `go` directive rises from 1.22.0 to 1.25.0. ViiB's 1.26.10 already satisfies it.
- **WebView2 binding:** `github.com/wailsapp/go-webview2` stays at v1.0.22.
- **Module graph:** expect `wailsapp/mimetype` to be replaced by `gabriel-vasile/mimetype`. Expect `git.sr.ht/~jackmordaunt/go-toast/v2` (Windows toasts) to be added, and `labstack/echo`/`gommon` (dev server) to be bumped. ViiB already requires newer `golang.org/x/{crypto,sys,text,net}` than Wails does, so those should not move. Confirm with `go mod graph` and the release Go-module inventory rather than assuming.
- **Generated runtime wrapper:** the v2.17 `wailsjs/runtime` adds only notification functions and types. ViiB's committed [`runtime.js`](../backend/cmd/wails/frontend/wailsjs/runtime/runtime.js)/`.d.ts` are consumed by the Vite build, while CI and release build with `-skipbindings`. Regenerate and commit them so the committed wrapper matches the module version. `App.js`/`App.d.ts` should not change.
- **Close and quit semantics:** unchanged for the current configuration. In v2.17 the Windows frontend's `Quit()` calls `OnBeforeClose` and aborts the quit if it returns true. ViiB does not set `OnBeforeClose` today, so this matters only for the close-preference follow-up below.

## Implementation (one PR)

- [ ] In `backend/`, run `go get github.com/wailsapp/wails/v2@v2.17.0` and `go mod tidy`. Review the `go.mod`/`go.sum` diff against the module-graph expectations above.
- [ ] Update every CLI pin from v2.11.0 to v2.17.0:
  - `.github/workflows/release.yml` (`WAILS_VERSION`)
  - `.github/workflows/ci.yml` (two `go install` steps)
  - `.github/workflows/build-windows.yml`
  - prerequisite comments in `scripts/build-wails.ps1`, `scripts/build-wails-windows-arm64.ps1`, `scripts/build-wails-macos.sh`, `scripts/build-wails-linux.sh`, and `scripts/dev-wails.ps1`
  - [`docs/development.md`](development.md) install command
- [ ] In `backend/cmd/wails`, regenerate the bindings with the v2.17.0 CLI (`wails generate module`) and commit the result. Expected diff: notification additions in `wailsjs/runtime/` only.
- [ ] Update the toolchain baseline line in [`DJV2_PROFESSIONAL_TRACK_ANALYSIS_ROADMAP.md`](DJV2_PROFESSIONAL_TRACK_ANALYSIS_ROADMAP.md). Leave dated reviews and archived evidence that cite v2.11.0 unchanged, because they record what was tested.
- [ ] Add a candidate entry to [`releases/UNRELEASED.md`](releases/UNRELEASED.md): the Wails v2.17.0 upgrade and the user-visible fixes (macOS 26 WebView crash, Linux bound-method panic, WebView2 bootstrapper error dialog).
- [ ] Run `govulncheck` (CI's `check-windows-govuln.ps1` covers the Windows binary) and review license/inventory output for the new modules.

Out of scope for this PR: any change to `main.go` options, the close hook, single-instance handling, version injection, or Linux packaging.

## Verification

| Gate | Required evidence |
|---|---|
| Automated | `npm run check`, `go test ./...` (with `-race` where CI runs it), and all CI desktop build jobs green. Treat `continue-on-error` native build steps as passing only when the logs show a successful build, not just a green job. |
| Build output | Windows amd64 and Linux amd64 builds embed v2.17.0 (`go version -m <binary>`). Release build steps still accept `-skipbindings`, `-platform`, `-tags webkit2_41`, and `-ldflags`. The Windows govulncheck gate passes. |
| Windows x64 | Install from the NSIS installer, launch, frameless title-bar drag/minimize/maximize/close, skinny mode, fullscreen, tray Show/Quit, hide-on-close, support-bundle save and cancel, external-browser Spotify sign-in, local playback with seek, and scan progress (SSE) over several events. |
| macOS arm64 and x64 | DMG install and first launch (ad-hoc signature), native title bar, fullscreen, DJ mode and visualizer under sustained updates (on macOS 26 if available, to confirm the Tahoe fix), PKCE login, support-bundle dialog, and quit from the menu. |
| Linux amd64 | `.deb` install on Ubuntu 24.04 plus the `.tar.gz` on one other distribution, tray Show/Quit, hide-on-close, playback, SSE, and support-bundle dialog. |
| Embedded assets | No MIME regressions after the `mimetype` swap: fonts, images, WASM/worker files, and audio fixtures load with correct `Content-Type` in the native app. |
| Developer workflow | `scripts/dev-wails.ps1` still starts and the app loads under `wails dev` with the hardened IPC checks. Confirm no Host/Origin rejection from the dev server. |
| Smoke test | [`scripts/smoke-test.ps1`](../scripts/smoke-test.ps1) passes on Windows. Record results with OS, architecture, and package hash. |

## Rollout and rollback

Ship the upgrade in the next release candidate (after RC8), with no other framework changes in that candidate. The change touches no data schema, settings file, WebView profile path, or origin, so rollback is reverting the PR and rebuilding. Keep the RC8 artifacts as the last v2.11.0 reference until the new candidate passes the verification table.

## Follow-ups that v2 already enables (separate PRs)

These were listed in the v3 assessment as reasons to migrate. All are achievable on v2.17.

1. **Close preference without restart.** Set `HideWindowOnClose: false` and add `OnBeforeClose`. On close it reads `window-preferences.json`, calls `runtime.WindowHide` and returns true when the preference is hide and a tray exists (Windows/Linux), and otherwise returns false. Add an explicit `quitting` flag, set before tray Quit and `runtime.Quit`, so `OnBeforeClose` lets those through; on Windows, `Quit()` itself invokes `OnBeforeClose`. Remove the restart notice from `SetWindowCloseAction`. Test native close, Alt+F4, custom close, tray Quit, and changing the preference while running on each platform.
2. **Second-launch activation.** With the default port, a second launch fails to bind `127.0.0.1:34115` and exits before Wails' `SingleInstanceLock` can signal the first instance (by code reading; reproduce first). Fix this by treating a bind failure on the default port as "already running" and signalling the existing instance. Alternatively, acquire a single-instance lock before opening the database and binding the port. Add a test for the occupied-port path.
3. **Native version injection.** Native builds pass only `-ldflags "-s -w"`, so `main.Version` and `BuildTime` stay `dev`/`unknown` in logs and support bundles. Add `-X main.Version=… -X main.BuildTime=…` to the scripts and workflows.
4. **Linux hidden-window trap.** If the tray initializes but no tray host is visible (for example GNOME without an AppIndicator extension), hide-on-close leaves no restore path. Consider defaulting Linux to quit-on-close, or rely on follow-up 2 so a relaunch restores the window.
5. **Native folder picker and file drop.** Both exist in v2 (`runtime.OpenDirectoryDialog`, `EnableFileDrop`/`runtime.OnFileDrop`) and can ship without v3.
6. **Optional macOS options.** Evaluate `DisableEscapeExitsFullscreen` only if Escape in fullscreen conflicts with in-app modals. Leave `EnableAutoplayWithoutUserAction` off unless resume-on-launch playback is a requirement.
7. **Desktop bridge adapter.** Route `window.go`/`wailsjs` consumers through one module. This is useful on v2 for testing, and it is the main preparation for an eventual v3 migration.

## Planning uncertainties

This plan is based on release notes, the changelog, and a source diff of the two tags. It does not claim an executed v2.17 build, native qualification, or a measured fix for the macOS 26 crash. The module-graph changes are expectations until `go mod tidy` runs. The second-launch defect comes from code reading and needs a reproduction before follow-up 2 is scheduled.
