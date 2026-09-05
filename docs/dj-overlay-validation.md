# DJ library overlay validation — 2026-09-05

This pass replaces the in-flow library with `DJLibraryDrawer` and preserves the existing `DJLibraryBrowserV2`. The drawer mounts its virtualized content on first use, preserves browser state while hidden, and owns its open state independently of the performance tree. It uses CSS positioning and a 40% height (280–640px, capped to available space). Manual panel resizing, page height/collapse state, drag listeners, mode-specific panel clamps, and the panel ResizeObserver are removed.

## Layout findings

| Browser viewport | Finding and resulting behavior | Open/close displacement |
| --- | --- | --- |
| 1920×1080 | Previously the 200px library plus 10px separator took 210px from the decks; overflowing sampler controls intercepted separator clicks. The new workspace fits the full-height deck controls and retains a bounded central mixer. | 0px |
| 2560×1440 | Mixer remains capped at 480px and jog wheels at 480px. Extra space surrounds the controls; drawer height is 576px. | 0px |
| 1840×960 | Models reduced usable space from desktop chrome. Full control columns remain reachable by scrolling the performance area. Drawer height is 384px. | 0px |
| 1600×900 | Previously the mixer body could collapse to zero and deck EQ could be clipped. Minimum 610px control-column height with scrolling preserves access; drawer height is 360px. | 0px |
| 1440×900 | Width-floor stress test; no document horizontal overflow. | 0px |

Two-row mirrored headers reserve track-title and performance-metadata space whether tracks are loaded or empty. The compact FX pad is slightly shorter and its target selector has sufficient width. FX readouts no longer expose floating-point strings such as `-13.200000000000001`. Performance and Browse share waveform geometry. Scope and timeline use the same waveform surface; FX/racks intentionally changes the primary view. Switching FX → Browse now restores timeline instead of leaving a mismatched rack/layout combination.

The fullscreen gate now observes the existing resolution/fullscreen hook and remembers admission. A later window resize above the 1440px floor does not re-gate and unmount the performance tree. Browser fullscreen passed. The existing below-1440 unsupported screen remains.

Screenshots (browser viewports, not native Wails window captures):

- [Before, 1080p](screenshots/dj-overlay/before-1080.png)
- 1080p: [closed](screenshots/dj-overlay/1920x1080-closed.png) / [open](screenshots/dj-overlay/1920x1080-open.png)
- 1440p: [closed](screenshots/dj-overlay/2560x1440-closed.png) / [open](screenshots/dj-overlay/2560x1440-open.png)
- [WebGL after capability fix](screenshots/dj-overlay/webgl-1080.png)
- Windowed-size simulation: [closed](screenshots/dj-overlay/1840x960-closed.png) / [open](screenshots/dj-overlay/1840x960-open.png)
- Constrained desktop: [closed](screenshots/dj-overlay/1600x900-closed.png) / [open](screenshots/dj-overlay/1600x900-open.png)

## Measured performance findings

Chrome/Vite CPU sampling during two-deck playback showed DJ page and hidden library React work despite unchanged track/key/loop selectors. Parent shell renders were crossing the page boundary. Memoizing `DJModeV2` removed these components from the subsequent sampled React hotspots; waveform drawing became the dominant DJ cost. Existing narrow leaf subscriptions and imperative store-to-engine sync remain unchanged.

In two local 300-frame stress samples (both decks playing, crossfader drag, EQ/tempo keyboard changes, four library open/search/scroll/close cycles):

| Measure | Before page memo boundary | After page memo boundary |
| --- | --- | --- |
| Median rAF interval | 16.8ms | 16.7ms |
| 95th percentile | 66.7ms | 33.4ms |
| Maximum | 133.4ms | 83.5ms |
| Observed long tasks (>50ms) | 38 | 0 |

These are directional development-browser samples, not controlled benchmarks, native WebView measurements, or proof of glitch-free audio. Both playback positions advanced without state resets during the stress samples. Some frame delays remain. Canvas drawing and application-shell work remain profiling candidates; this pass does not rewrite those systems.

Other concrete changes:

- Stable Virtuoso table/row and sort-header identities avoid remounts on browser updates. Column-resize listeners are cleaned up when the library hides or unmounts.
- Playlist membership uses a Set, replacing catalog-size × playlist-size membership scans.
- FX header selectors return enabled-effect counts, so parameter drags do not rerender that parent for an unchanged count.
- WebGL waveform surfaces follow the same CSS height as Canvas. Effects depend on stable renderer methods, avoiding texture uploads/readiness-timer restarts caused solely by newly allocated hook wrapper objects. The lazy Canvas fallback has a stable component identity.
- WebGL 2 float textures used LINEAR filtering without requesting `OES_texture_float_linear`. A live test had valid peaks but flat waveforms; enabling the capability restored the waveform. Allocation now requests it and uses NEAREST when absent; WebGL 1 byte textures retain LINEAR. Tests cover supported, unsupported, and WebGL 1 cases. Shared platform selection and fallback policy are unchanged.

## Interaction and catalog validation

`node scripts/dj-overlay-audit.mjs` uses an isolated browser context, intercepted 12,000-song catalog/playlist responses, and generated WAV audio. It checks all five geometry pairs, fewer than 80 mounted rows (9–13 observed), search focus, hidden state, Escape from search, column-menu priority, shortcut-modal priority, text-entry safety, retained search, sorting, playlist filtering, column visibility, load A/B, drag-to-deck, playback continuity, Browse, fullscreen, and the width floor. Synthetic records are never written to the real backend catalog.

The live 3,014-song catalog was also browsed, searched, sorted and loaded into both decks. Deck A alone, Deck B alone, and both-deck playback were exercised with crossfader/EQ/tempo changes and animated waveforms/meters. Local audio requests loaded successfully. Some server waveform endpoints returned 500; the existing client fallback produced waveforms. That server issue is not fixed here. The synthetic audit also exercises Deck B playback/load behavior; no native audible evaluation is claimed.

## Windows

- Windows host, Chromium Playwright, Vite at localhost:3000: tested as described above.
- Production Windows Wails executable: built with installed Wails CLI 2.15.0, repository runtime 2.11.0, Go/CGO and MinGW. The CLI reports this version mismatch; dependency versions are unchanged.
- An isolated native launch was attempted twice with a separate data directory and localhost debug port. Both exited at the Wails single-instance lock because an existing `ViiB-MediaHub` process was running. The existing application was not stopped. Therefore actual maximized/fullscreen WebView2 layout, native CPU/GPU behavior, and audible-glitch testing remain outstanding.

## macOS

No macOS machine is available; WKWebView and native macOS builds were not executed. Reviewed the platform policy, fallback paths, RAF/idle timer cleanup, renderer disposal, and Web Audio compatibility. Current main prefers WebGL 2 with WebGL 1/Canvas fallbacks even on macOS; a stale page comment claiming forced Canvas was corrected. This pass does not disable or weaken platform guards. Native WKWebView stability, device routing, GPU/CPU load, and audio continuity require a macOS validation run.

## Commands and remaining risks

- `npm run check`: passes palette audit (existing warnings), raw-color baseline check, TypeScript, 43 Vitest tests across 13 files, and production Vite build. Existing large-chunk warnings remain.
- `node scripts/dj-overlay-audit.mjs`: passes the browser regression suite described above.
- `cd backend; go test ./...`: passes.
- `cd backend; CGO_ENABLED=1 go test -race ./...` with MinGW on PATH: passes.
- `cd backend; go vet ./...`: existing `internal/scanner/journal_windows.go:306:53` unsafe.Pointer warning; not changed by this work.
- `staticcheck ./...` from the installed binary reported `"./..." matched no packages` in this execution environment, so no successful staticcheck coverage is claimed.
- Windows Wails production build: passes. Native execution limitations are above.

Hardware MIDI, external audio routing, sampler media/relinking, recording output, exhaustive hot-cue/loop/sync combinations, and Plex-specific playback were not manually revalidated. Their implementations remain unchanged. Opening/closing the overlay has no store or engine writes. Search/sort/category/columns/color/harmonic rendering continues through the existing browser; the audit does not claim an exhaustive harmonic or track-color test.

No current DJ V2 roadmap/suggestions/implementation-plan document was found in the checkout. References to the former bottom-panel plan are superseded by this report and `docs/dj-mode.md`; the unrelated AI DJ semantic-retrieval plan is unchanged.
