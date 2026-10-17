# macOS build review

Reviewed 2026-10-17: Wails/Darwin configuration, Intel and Apple Silicon packaging,
native window behavior, and the DJ renderer/worker/audio changes.

## Findings corrected

1. **High: native macOS shell was not recognized.** `isMacOSWails` only accepted
   `wails.localhost`. Wails v2.11.0's Darwin frontend uses `wails://wails/`
   (`internal/frontend/desktop/darwin/frontend.go`, `startURL`). This could show
   the custom title bar alongside the native title bar and select `WindowHide`
   instead of Dock minimization, despite macOS having no system tray in this app.
   Runtime detection now reads the protocol and accepts the actual Darwin
   scheme/host, preserving the existing legacy host. Tests cover Darwin,
   ordinary browser origins, Windows, and the actual window service action.

2. **Medium: DJ meters required `CanvasRenderingContext2D.roundRect`.** The bundle
   declares macOS 12.0 as its minimum, so meter initialization must tolerate
   older WKWebView canvas implementations. Both background and active segments
   now use a shared rounded-path helper with a quadratic-curve fallback.
   Tests cover native and fallback geometry. A Playwright run with `roundRect`
   removed rendered all three stereo meters with no page errors.

## Build and release coverage

- CI builds native CGO/Wails applications on separate `macos-15` ARM and
  `macos-15-intel` runners, validates executable architecture with `lipo`, and
  archives the application bundle. Release CI produces separate DMGs and verifies
  ad-hoc signatures. This review did not run or observe those remote jobs.
- The release workflow explicitly produces ad-hoc signed, non-notarized RC
  packages. Apple Developer signing and notarization remain distribution work;
  successful compilation does not establish Gatekeeper acceptance.
- WebGL detection tries WebGL 2, then WebGL 1, then Canvas. Worker startup/error
  and timeout paths retain a main-thread Canvas fallback. Audio loop worker
  failures retain the timer fallback. No unconditional requirement for WebGL 2
  or OffscreenCanvas was found in the reviewed DJ paths.

## Verification limits

Local production build, TypeScript and style checks passed, along with all 566 tests across 131
files, including the new runtime/window-action and canvas fallback regressions.
The missing-`roundRect` Playwright smoke check passed in Chromium; it is a
feature-fallback check, not a WKWebView compatibility certification.

Local validation runs on Windows. Frontend checks and simulated missing browser
APIs cannot validate Cocoa linking or WKWebView behavior. Darwin Go cross-compilation
also cannot substitute for launching the native application.

Before qualifying a macOS release, run the actual Intel and Apple Silicon
packages, including the oldest supported OS: launch/quit and Dock recovery;
dual-deck playback, seeking and loops; WebGL rendering and Canvas fallback;
custom-scheme module-worker and AudioWorklet loading; audio device switching;
and sustained playback/background/resume. Existing build/architecture checks
do not exercise these native DJ flows.
