# Spotify authentication on macOS

> Scope update — 2026-10-12: Historical compiler/fixture evidence is distinct from macOS host execution. Further local scanning/DSP improvements are deferred; unverified platform behavior is not newly qualified by this documentation review. See [Spotify behavior and limitations](spotify.md) and [future local-analysis work](LOCAL_ANALYSIS_FUTURE_BRANCH.md).

The Web Player baseline below was recorded on 2026-10-04 on `spotify/webplayer-auth-analysis-provider`, based on `fed2571` with additional worktree changes. The baseline used Go 1.26.8 on Windows. The default-browser sign-in section records a separate validation gate.

## Web Player baseline and prerequisites

The historical pre-native checks passed for Apple Silicon and Intel. Live Spotify sign-in, native WebKit integration, packaged-app restart, and process/profile cleanup remain manual macOS checks.

ViiB's Wails window uses the system WebKit/WKWebView engine. Web Player sign-in uses a separate Chromium-based browser through Chrome DevTools Protocol (CDP) and an isolated browser profile.

Web Player sign-in supports Chrome, Chromium, and Microsoft Edge. Discovery checks PATH and browser executables under `/Applications` and `~/Applications`. If discovery finds no supported browser, ViiB prompts the user to install one. Users sign in on Spotify's page.

[Wails documents the built-in WebKit dependency](https://v3.wails.io/quick-start/installation/), and [Apple documents Safari as included with macOS](https://support.apple.com/en-us/102665). The WebKit dependency fact applies to this app's Wails v2 backend; the linked Wails page is v3 documentation, not this repository's build instructions. This branch's `backend/internal/spotify/auth/browser_login.go` defines its separate browser requirements.

Browser capture creates a non-default temporary profile. That matches [Chrome's remote-debugging requirement from Chrome 136 onward](https://developer.chrome.com/blog/remote-debugging-port). Cleanup stops the browser allocator before removing that profile.

The backend encrypts Web Player sessions in `spotify_webplayer_session` and desktop OAuth credentials in `spotify_credentials`. Keep the application data and its credential key together when using an existing installation.

## Historical Web Player checks

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

The release workflow runs `Validate Spotify browser authentication on macOS` in those jobs before the native Wails build:

```sh
# Run from backend on the Mac runner. No Spotify account secrets are needed.
CGO_ENABLED=1 VIIB_TEST_LOGIN_BROWSER=1 go test -count=1 -timeout=5m ./internal/spotify/auth ./internal/crypto
CGO_ENABLED=1 go test -count=1 -timeout=5m ./internal/api -run '^Test(BrowserLogin|Cookie|DesktopSpotifyOAuth|SpotifySessionExcluded|SpotifyCredentialsExcluded)'
```

This enables the otherwise opt-in real-browser fixture, including HttpOnly capture, cancellation, and temporary-profile removal on macOS. The existing subsequent Wails build and DMG checks cover native compilation and packaging. The added step has now run in GitHub CI; see the initial findings below. Existing workflow triggers remain unchanged.

CI fixtures do not prove a real Spotify form/MFA/SSO submission or user-visible Wails behavior. Those remain hand-test gates. Do not put the supplied session cookie into CI to replace those gates.

## Web Player hand-test gate

Test the packaged build installed in its final location, ideally once on Apple Silicon and once on Intel. Use Chrome first; repeat with Edge or Chromium if those browsers are claimed as supported for the release.

1. With Safari selected as the default browser and Chrome installed, click **Connect Web Player**. Verify a separate visible Chrome sign-in window opens.
2. Sign in directly on Spotify, including MFA/SSO if applicable. Verify ViiB reports connected, the temporary window closes, and no `viib-spotify-login-*` profile remains under the system temporary directory.
3. Search, open an artist and album, and play/seek/advance a track in the native Wails window. Record authentication, catalog, and audio outcomes separately.
4. Quit and relaunch the same installed app. Verify the session restores without asking for a cookie or another sign-in.
5. Disconnect, reconnect, and switch accounts. Verify prior-account results and active Spotify media are retired.
6. Test Cancel sign-in, closing the sign-in browser, and quitting ViiB during pending sign-in. Verify clean failure/cancellation, process exit, and temporary-profile removal.
7. On a separate test Mac/user without Chrome, Chromium, or Edge, verify the Web Player install-browser error and link.

Record macOS version, architecture, installed browser/version, app installation path, and pass/fail for each step. Exclude cookies, tokens, passwords, and raw cookie/network logs from the report.

See [the overall validation record](SPOTIFY_WEBPLAYER_VALIDATION.md) and [the parity audit](SPOTIFY_COOKIE_AUTH_PARITY_AUDIT.md).

## Default-browser sign-in hand-test gate

The desktop implementation uses Spotify PKCE and accepts Spotify's redirect through the loopback listener. The Windows callback fixture checks state validation, cancellation, replay rejection, credential handling, and media capability gating. The API package compiled for Apple Silicon. A packaged Mac run with live Safari authorization remains unverified.

1. Install the packaged app on an Apple Silicon Mac with Safari as the default browser. Register the URI shown in ViiB for an authorized Spotify Developer app, as described in the [Spotify setup guide](spotify.md#default-browser-sign-in-on-desktop).
2. Select **Sign in with your default browser**, authorize access in Safari, and verify that the confirmation page and ViiB both show success.
3. Verify Web API browsing and account views. Confirm that ViiB directs users to Web Player for in-app streaming and downloads.
4. Relaunch and verify that the connection restores. Test declined consent, **Cancel sign-in**, and **Disconnect**. A callback arriving after cancellation should leave the connection unchanged.

## Initial native CI findings (2026-10-04)

[PR #115](https://github.com/ajbergh/ViiB-MediaHub/pull/115) triggered both validation workflows. Standard CI passed on `74df803`, including native desktop builds. The [release validation run](https://github.com/ajbergh/ViiB-MediaHub/actions/runs/37220767061) exposed temporary Chrome-profile cleanup failures in the browser fixture on Apple Silicon and Intel. It also exposed unrelated stem-library API tests rejecting the macOS `/var` symlink alias.

The follow-up stops/waits for the allocator before the browser-context wait and retries removal of the exact capture profile for a bounded 1.9-second retry window, checking that the profile root is absent. It still refuses to return a credential if cleanup fails. Deterministic retry/exhaustion tests and the Windows real-browser fixture pass. Native revalidation is pending for this fix.

The Mac CI step now runs the full auth/crypto suites and the Spotify browser-login, cookie lifecycle/catalog, and export-isolation API tests. It does not claim to validate unrelated stem-library behavior; the `/var` alias failures remain a separate known Mac testing issue.
