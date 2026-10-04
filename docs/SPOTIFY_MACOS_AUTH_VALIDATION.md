# Spotify authentication on macOS

Validation date: 2026-10-04. This covers the current worktree on `spotify/webplayer-auth-analysis-provider`, based on `fed2571`, including uncommitted artist and download changes. Backend toolchain: Go 1.26.8. Execution host: Windows.

## Result and prerequisites

**Pre-native checks pass; native macOS sign-in is not yet confirmed.** No macOS-specific authentication compile blocker was found for Apple Silicon or Intel. Real Spotify credentials/MFA, visible browser launch, native WebKit integration, packaged-app restart, and cleanup still require macOS execution.

ViiB's Wails window uses the system WebKit/WKWebView engine. macOS supplies WebKit; users do not install Chromium to render the main application. The new Spotify sign-in uses a separate Chromium-based browser through Chrome DevTools Protocol (CDP), and does not use the Wails WebView's cookie store or the default-browser setting.

The current supported sign-in browsers are Chrome, Chromium, and Microsoft Edge. Discovery checks PATH and the executable inside each browser's `.app` under `/Applications` and `~/Applications`. Safari and Firefox cannot be substituted into this CDP implementation. The app neither bundles nor downloads Chromium; if none of the supported browsers is installed, sign-in reports that one must be installed. Credentials are entered on Spotify's page; no cookie field is presented by ViiB.

[Wails documents the built-in WebKit dependency](https://v3.wails.io/quick-start/installation/), and [Apple documents Safari as included with macOS](https://support.apple.com/en-us/102665). The WebKit dependency fact applies to this app's Wails v2 backend; the linked Wails page is v3 documentation, not this repository's build instructions. This branch's `backend/internal/spotify/auth/browser_login.go` defines its separate browser requirements.

Browser capture creates a non-default temporary profile. That matches [Chrome's remote-debugging requirement from Chrome 136 onward](https://developer.chrome.com/blog/remote-debugging-port). Cleanup stops the browser allocator before removing that profile.

Saved sessions are encrypted through `spotify_webplayer_session`. The existing key derivation includes the executable path and macOS user/machine inputs. Install the app in its final location before sign-in; moving the app or database can require reconnection. These credentials are not stored in macOS Keychain.

## Checks executed

| Check | Result | What it proves |
|---|---|---|
| Current Go suite and auth/API/Wails vet | Pass on Windows | Host regression coverage; not Mac execution |
| Fresh selected authentication, cancellation, replacement, cookie restoration/redaction, and encryption tests | 14 top-level tests pass on Windows | Authentication lifecycle and persistence fixtures |
| Real Chromium HttpOnly capture and cancellation/profile cleanup, with `VIIB_TEST_LOGIN_BROWSER=1` | Pass on Windows using a synthetic cookie and `about:blank` | CDP capture/cleanup mechanism; not Spotify credentials or Mac Chrome |
| Spotify connection UI/backend-client tests | 2 files, 19 tests pass | Browser-login polling/cancel and renderer session boundaries |
| Darwin `arm64` internal packages and plain Wails package | Compile pass with CGO disabled | Apple Silicon Go compatibility; not a production native desktop build |
| Darwin `amd64` internal packages and plain Wails package | Compile pass with CGO disabled | Intel Go compatibility; not a production native desktop build |
| Auth, API, and Wails test binaries for both Darwin architectures | Compile pass; all six verified as correct 64-bit Mach-O architecture | Tests compile for both Mac targets; none were executed on Windows |
| Native CGO/Wails packaging and real Spotify login | Not executed here | Requires an actual Mac |

Cross-compile commands, expressed with shell environment assignment:

```sh
# Run from backend; repeat for arm64 and amd64.
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build ./internal/... ./cmd/wails
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go test -c -o output/macos-auth-validation/auth-arm64.test ./internal/spotify/auth
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go test -c -o output/macos-auth-validation/api-arm64.test ./internal/api
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go test -c -o output/macos-auth-validation/wails-arm64.test ./cmd/wails
```

The production Wails build uses CGO and native frameworks. Plain package compilation above is deliberately not reported as a successful `.app` or DMG build. The browser-mode `cmd/viib` build also depends on the native tray implementation; disabling CGO is not a substitute for that macOS build.

## Native GitHub CI coverage

The existing release workflow has Apple Silicon `macos-15` and Intel `macos-15-intel` jobs. [GitHub documents these architectures](https://docs.github.com/en/actions/reference/runners/github-hosted-runners). Their current [Apple Silicon](https://github.com/actions/runner-images/blob/main/images/macos/macos-15-arm64-Readme.md) and [Intel](https://github.com/actions/runner-images/blob/main/images/macos/macos-15-Readme.md) image inventories list Chrome and Edge; browser availability is specific to the runner image, not every user's Mac.

This worktree adds `Validate Spotify browser authentication on macOS` to those jobs before the native Wails build:

```sh
# Run from backend on the Mac runner. No Spotify account secrets are needed.
CGO_ENABLED=1 VIIB_TEST_LOGIN_BROWSER=1 go test -count=1 -timeout=5m ./internal/spotify/auth ./internal/crypto ./internal/api
```

This enables the otherwise opt-in real-browser fixture, including HttpOnly capture, cancellation, and temporary-profile removal on macOS. The existing subsequent Wails build and DMG checks cover native compilation and packaging. The added step has been prepared locally but has not run in GitHub CI yet; the changed worktree must be committed/pushed before a run can validate it. Existing workflow triggers remain unchanged.

CI fixtures do not prove a real Spotify form/MFA/SSO submission or user-visible Wails behavior. Those remain hand-test gates. Do not put the supplied session cookie into CI to replace those gates.

## Mac hand-test gate

Test the packaged build installed in its final location, ideally once on Apple Silicon and once on Intel. Use Chrome first; repeat with Edge or Chromium if those browsers are claimed as supported for the release.

1. With Safari selected as the default browser and Chrome installed, click **Sign in with Spotify**. Verify a separate visible Chrome sign-in window opens, without a cookie input in ViiB.
2. Sign in directly on Spotify, including MFA/SSO if applicable. Verify ViiB reports connected, the temporary window closes, and no `viib-spotify-login-*` profile remains under the system temporary directory.
3. Search, open an artist and album, and play/seek/advance a track in the native Wails window. Record authentication, catalog, and audio outcomes separately.
4. Quit and relaunch the same installed app. Verify the session restores without asking for a cookie or another sign-in.
5. Disconnect, reconnect, and switch accounts. Verify prior-account results and active Spotify media are retired.
6. Test Cancel sign-in, closing the sign-in browser, and quitting ViiB during pending sign-in. Verify clean failure/cancellation, process exit, and temporary-profile removal.
7. On a separate test Mac/user without Chrome, Chromium, or Edge, verify the install-browser error. Safari-only sign-in is not a supported gate for this implementation.

Record macOS version, architecture, installed browser/version, app installation path, and pass/fail for each step. Exclude cookies, tokens, passwords, and raw cookie/network logs from the report.

See [the overall validation record](SPOTIFY_WEBPLAYER_VALIDATION.md) and [the parity audit](SPOTIFY_COOKIE_AUTH_PARITY_AUDIT.md).
