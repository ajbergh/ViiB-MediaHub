# Wails v2.17 upgrade plan

Research date: 2026-10-23. Base: `main` at `0d6b04a`. Status: implemented in this worktree; validation results and remaining release gates are recorded below.

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
- **Module graph:** `wailsapp/mimetype` is replaced by `gabriel-vasile/mimetype`. Resolved changes: `git.sr.ht/~jackmordaunt/go-toast/v2` v2.0.3 and `gabriel-vasile/mimetype` v1.4.16 are added; `labstack/echo/v4` rises to v4.15.3, `labstack/gommon` to v0.5.0, and `golang.org/x/time` to v0.15.0. ViiB already requires newer `golang.org/x/{crypto,sys,text,net}` than Wails does; those versions remain unchanged in the resolved graph and inventory.
- **Generated runtime wrapper:** the v2.17 `wailsjs/runtime` adds only notification functions and types. ViiB's committed [`runtime.js`](../backend/cmd/wails/frontend/wailsjs/runtime/runtime.js)/`.d.ts` are consumed by the Vite build, while Unix CI and release build with `-skipbindings`; Windows CI also exercises regeneration. Regenerate to verify the committed wrapper matches the module version. The current committed wrappers already include these notification additions, so no runtime diff is expected. `App.js`/`App.d.ts` should not change.
- **Close and quit semantics:** unchanged for the current configuration. In v2.17 the Windows frontend's `Quit()` calls `OnBeforeClose` and aborts the quit if it returns true. ViiB does not set `OnBeforeClose` today, so this matters only for the close-preference follow-up below.

## Implementation (one PR)

- [x] In `backend/`, run `go get github.com/wailsapp/wails/v2@v2.17.0` and `go mod tidy`. Review the `go.mod`/`go.sum` diff against the module-graph expectations above.
- [x] Update every CLI pin from v2.11.0 to v2.17.0:
  - `.github/workflows/release.yml` (`WAILS_VERSION`)
  - `.github/workflows/ci.yml` (two `go install` steps)
  - `.github/workflows/build-windows.yml`
  - prerequisite comments in `scripts/build-wails.ps1`, `scripts/build-wails-windows-arm64.ps1`, `scripts/build-wails-macos.sh`, `scripts/build-wails-linux.sh`, and `scripts/dev-wails.ps1`
  - [`docs/development.md`](development.md) install command
- [x] In `backend/cmd/wails`, regenerate the bindings with the v2.17.0 CLI (`wails generate module`) and commit the result. Expected result: no diff, because the committed runtime already contains the notification additions.
- [x] Update the toolchain baseline line in [`DJV2_PROFESSIONAL_TRACK_ANALYSIS_ROADMAP.md`](DJV2_PROFESSIONAL_TRACK_ANALYSIS_ROADMAP.md). Leave dated reviews and archived evidence that cite v2.11.0 unchanged, because they record what was tested.
- [x] Add a candidate entry to [`releases/UNRELEASED.md`](releases/UNRELEASED.md): the Wails v2.17.0 upgrade and the user-visible fixes (macOS 26 WebView crash, Linux bound-method panic, WebView2 bootstrapper error dialog).
- [x] Run `govulncheck` (CI's `check-windows-govuln.ps1` covers the Windows binary) and review license/inventory output for the new modules.

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

The original plan was based on release notes, the changelog, and a source diff of the two tags. Implementation evidence is recorded below. Windows compilation and a basic launch test do not establish full native qualification or a measured fix for the macOS 26 crash. The second-launch defect comes from code reading and needs a reproduction before follow-up 2 is scheduled.

## Implementation review (2026-10-23)

The dependency/tooling upgrade is implemented without changing application options
or the separate behavior follow-ups. Installation hints now pin v2.17.0 instead
of `@latest`. Windows CI retains binding generation; release and Unix CI retain
`-skipbindings`.

- `go get` and `go mod tidy` resolved the changes listed above. WebView2 remains
  v1.0.22 and x/crypto, x/sys, x/text and x/net remain at their existing versions.
- The pinned v2.17.0 CLI regenerated all five committed binding/runtime files
  with no content changes (only local line-ending normalization).
- The release inventory script successfully exported 303 resolved modules to
  the local validation artifact `.tmp/wails-v2.17-go-modules.json`.
- License files in the downloaded module sources were reviewed: go-toast
  v2.0.3 offers `Unlicense OR MIT`; mimetype v1.4.16, Echo v4.15.3 and Gommon
  v0.5.0 use MIT; x/time v0.15.0 uses the Go BSD license. These require retaining
  their applicable notices. This focused review does not qualify the complete
  release license inventory or native-library licenses.
- `npm audit --omit=dev --audit-level=high` reports no vulnerabilities. The full
  install audit reports eight development-dependency advisories; the npm lockfile
  is unchanged by this upgrade.

Native macOS/Linux builds, installer qualification, the full native interaction
matrix, asset MIME checks in native WebViews, and hardened `wails dev` IPC
qualification remain release gates requiring the corresponding environments.

### Executed checks

Validation host: Windows amd64; the backend selected Go 1.26.10 via its module
baseline. Logs and scratch outputs are in the ignored `.tmp/` directory.

| Check | Result |
|---|---|
| `npm run check` | Passed: palette guards, both TypeScript checks, 154 test files / 756 tests, production Vite build |
| `go test ./... -timeout 10m` | Passed with the MinGW compiler available and embedded assets prepared |
| `go test -race ./... -timeout 25m` | Passed on Windows amd64 with MinGW; no race reports |
| `go vet ./...` | Passed |
| Dependency inventory unit tests | 4 passed |
| `govulncheck -tags desktop,production ./cmd/wails` | Passed: no reachable vulnerabilities |
| Windows release command | Passed: v2.17 CLI `build -skipbindings -platform windows/amd64 -ldflags '-s -w'` |
| `go version -m` | Executable embeds Go 1.26.10 and Wails v2.17.0 |
| `check-windows-govuln.ps1` | Passed under the existing policy: GO-2026-5932 binary-mode overmatch excluded after the production graph confirmed no openpgp imports |
| Basic Windows smoke | Passed using a temporary copy of `smoke-test.ps1`, with hidden launch, isolated `-data` directory and port 34125; window detected after 2 seconds and process stopped |

Windows executable SHA-256:
`D067685FB6AB3557C013288E4835A181294B801109721DD854893B2658B4320D`.
This identifies the local executable, not an installed NSIS package. The initial
smoke attempt used an incorrect harness argument (`-data-dir`); correcting it
to the existing `-data` flag resolved the launch failure without app changes.
