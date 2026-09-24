# DJ Panel Review: Loops, PERF Waveforms, and Controls

Date: 2026-09-23

## Fix Progress

Current status: Fixes for findings 1–26, including the DJ load crash regression, are on `djv2/panel-review-fixes` and [PR #59](https://github.com/ajbergh/ViiB-MediaHub/pull/59) is open. Analysis state, MIDI mapping behavior/status, WebGL overview loop shading, and superseded track loads have been corrected. Minimized-window loop behavior still needs Wails runtime confirmation.

| Finding | Status | Progress |
|---|---|---|
| 1–5 | Fixed | Loop creation now has visible presets; invalid loops auto-create safely and cannot be halved/doubled; set operations preserve enabled state; loop-in is pending until loop-out. |
| 6–9 | Partial | Overshoot is preserved, beat loops use quantize/grid timing, and displays are consistent. Loop boundaries use a 5 ms worker ticker plus media `timeupdate` events, independent of requestAnimationFrame; minimized Wails behavior still needs runtime confirmation. |
| 10–12 | Fixed | Canvas idle invalidation observes waveform, duration, grid, cues, loop state, position, and backing sizes; centered mapping and max-pool sampling are used in both lanes and overview. |
| 13–18 | Fixed | WebGL keeps the full peak series in a multirow texture with max-pooled per-pixel sampling, shares zoom/color controls, tracks renderer invalidation state, draws loop bands/edges, and marks overview regions. Both palettes are accurately named; controls, labels, overview seeking, and lane sizes are separated. |
| 19 | Fixed | DJ page mounts MIDI dispatch, loads persisted mappings, routes transport/mixer/EQ/tempo/jog/hot-cue/FX/loop/headphone/sampler actions, and clears the handler on unmount. |
| 20 | Fixed | Deck loading stops after a missing feature record, marks analysis status, and shows a clear status near BPM and energy insights. Non-404 failures remain distinguishable as unavailable. |
| 21 | Fixed | DJ page crashed on load because each deck BPM badge returned a newly allocated object from its Zustand selector. Split it into primitive `hasTrack` and `analysisStatus` selectors to keep snapshots stable. |
| 22 | Fixed | The DJ v2 library loader now resolves analysis status to `available`, `not_analyzed`, or `error`; it requests the beat grid only after finding the feature record. |
| 23 | Fixed | MIDI sync now follows the UI's BPM and beat-phase rules; headphone cue mappings honor toggle, momentary, and value modes; jog wheels are available to learn as relative mappings. |
| 24 | Fixed | The DJ toolbar now labels MIDI as ON/OFF and explains that controller input is inactive until the user enables Web MIDI access. |
| 25 | Fixed | WebGL overview loop ranges are tinted in the overview shader so the waveform remains visible; removed the scissored framebuffer clear that erased pixels. |
| 26 | Fixed | A newer deck load cancels and settles the previous pending load with `AbortError`; hook callers swallow only this superseded-load signal and do not install stale deck state. |

Implementation log:

- 2026-09-23: Started implementation from the findings below. The findings are the acceptance checklist; their remediation suggestions are guidance, not separate instructions.
- 2026-09-23: Fixed findings 1–5: added selectable beat-loop presets in v2, safe empty-loop toggle behavior, valid-loop guards and a beat-based minimum, explicit loop enabled state, and pending loop-in state. Updated v2 loop status to display beat lengths.
- 2026-09-23: Fixed findings 6, 8, and 9 in source: loop seeks retain overshoot, quantized beat loops snap to the nearest adjusted grid beat and derive length from grid timing, and the button/footer show beat size with a 120 BPM fallback. Finding 7 now uses a dedicated 5 ms Web Worker ticker outside rAF, with a timer fallback if the worker fails; hidden/minimized Wails runtime behavior remains to be confirmed.
- 2026-09-23: Fixed findings 10–12 in Canvas 2D: idle redraw invalidation now compares all rendered deck state and canvas sizes, waveform sampling is centered on the playhead, and peaks are max-pooled per pixel. Added loop shading/markers to the 2D timeline and overview, renamed palette controls to describe their amplitude-based behavior, and made overview clicks seek the selected deck. The toolbar has a reserved strip and lane heights use whole CSS pixels.
- 2026-09-23: Fixed findings 13–18: WebGL uploads complete peak data to a resolution-aware multirow texture (max-pooling only if the device texture limit is exceeded) and max-pools per rendered pixel, shares zoom/palette controls, invalidates on peaks/duration/grid offset/loop/cues/view changes, and shades loop ranges with boundary markers. The overview shows loop ranges and supports click-to-seek. Both renderer UIs describe amplitude palettes accurately.
- 2026-09-23: Fixed finding 19 by mounting a page-level MIDI action handler, loading saved mappings during its lifecycle, dispatching all defined mapping categories to engine/store/sampler APIs, and clearing the handler when the DJ page unmounts.
- 2026-09-23: Fixed finding 20 by recording per-deck analysis status, stopping beat-grid/energy/recommendation requests when the base feature record is missing, and showing “Not analysed” or “Unavailable” feedback beside BPM and in the energy panel.
- 2026-09-23: `npm run typecheck` passed and `git diff --check` is clean. The automated test suite was not run. `gh auth status` reports the saved GitHub token is invalid, so push/PR creation is blocked until GitHub is reauthenticated. Finding 7 still needs confirmation in a minimized Wails window.
- 2026-09-23: Pushed implementation commit `833ca11` to `origin/djv2/panel-review-fixes`. `gh auth status` reports an invalid saved token, and Git Credential Manager returned no reusable credential even with the repository marked safe, so I could not create the PR. `git push` printed the branch's GitHub PR creation link.
- 2026-09-23: Tightened finding 7's worker/fallback interval to 5 ms and added fallback on worker errors. Pushed commit `2a3d864`; `npm run typecheck` passed after the change. Wails minimized-window runtime confirmation remains outstanding.
- 2026-09-23: Added media `timeupdate` listeners as a second loop-wrap trigger alongside the worker ticker, removed on engine disposal. `npm run typecheck` passed, and commit `9365a40` is pushed. Wails minimized-window behavior still requires runtime confirmation.
- 2026-09-23: Created [PR #59](https://github.com/ajbergh/ViiB-MediaHub/pull/59) using the host GitHub CLI after confirming host authentication. This supersedes the earlier sandbox authentication blockage; minimized-window Wails runtime confirmation remains outstanding.
- 2026-09-23: Fixed the DJ load crash reported after opening PR #59. The shared BPM badge selector created a new object on every Zustand snapshot, triggering React's infinite update-depth error on both decks. Replaced it with primitive selectors and added finding 21 to this log. User-reported crash diagnosis was reproduced by repository investigation; unrelated existing edits in `backend/go.mod` and `backend/go.sum` were left untouched.
- 2026-09-23: Fixed additional DJ issues 22–26 found in the follow-up audit: v2 analysis requests now update status and skip the grid request when the feature is missing; MIDI sync/headphone cue semantics match the UI and jog wheel can be learned; MIDI enabled state is visible; WebGL loop shading blends with overview pixels; and superseded track loads reject promptly without allowing stale callers to replace the active deck. Pushed commit `9817d70` to PR #59. Existing edits in `backend/go.mod` and `backend/go.sum` remain untouched.

## Scope

This review covers DJ Mode v2 (`/dj`) with a focus on:

- Loop controls: the v2 loop section, the engine's loop actions, and loop playback.
- The timeline waveforms in the PERF layout, in both the Canvas 2D and WebGL renderers.
- Other controls found along the way (MIDI, analysis requests).

Method:

- Live testing with Playwright (Chromium 1920×1080, headless) against `http://127.0.0.1:34115/dj`.
- Code reading of the files listed under each finding.

The server was serving the production bundle in `dist/`. It was built at 14:22 CDT on 2026-09-23, after `dc7edb8`; the later `d02ad01` does not touch DJ files. The tested build therefore matches the current source for DJ code.

Test tracks were loaded through the library drawer's "Load to Deck A/B" buttons:

| Deck | Track | Duration | Server peaks |
|------|-------|----------|--------------|
| A | ¿Por Qué Les Mientes? — Tito "El Bambino" | 213.3 s | 36,746 (~172/s) |
| B | ...And Justice For All — Metallica | 585.5 s | 100,863 (~172/s) |

Neither track had been analysed. Every `/api/v2/analysis/{id}` request returned 404, so BPM showed `--.--` and no beat grid was drawn. Loop tests therefore ran on the engine's 120 BPM fallback, and beat-grid rendering was not exercised.

### How the tests observed state

The production bundle doesn't expose the Zustand store, so the test uses a Playwright init script instead:

- It wraps `window.Audio` to capture the engine's deck `HTMLAudioElement`s (created at `lib/djAudio.ts:259-260`).
- It instruments the `HTMLMediaElement.prototype.currentTime` setter, which timestamps every seek. Each loop wrap is one seek.
- UI state was read from the DOM: the loop button text, the `aria-pressed` attribute, the footer `LOOP` label, and canvas pixel coverage.

The scripts are kept outside the repo. They can be turned into a committed e2e spec (see [Testing Plan](#testing-plan)).

## Summary

| # | Area | Finding | Severity | Evidence |
|---|------|---------|----------|----------|
| 1 | Loops | LOOP with no loop set turns on a zero-length loop | High | Reproduced |
| 2 | Loops | Halve/double on that empty loop jumps playback to 0:00 | **Critical** | Reproduced |
| 3 | Loops | The v2 panel has no control that creates a loop | High | Code + UI |
| 4 | Loops | `setLoopBeats` / `setLoopOut` switch the loop off after creating it | High | Code |
| 5 | Loops | `setLoopIn` starts an active 4 s loop immediately | Medium | Code |
| 6 | Loops | Loop wrap jitters ±9 ms per cycle and discards the overshoot | High | Reproduced |
| 7 | Loops | Loop wrap depends on `requestAnimationFrame` (stops in hidden/minimised pages) | High | Code (not reproduced) |
| 8 | Loops | Loops ignore quantize and the beat grid | Medium | Code |
| 9 | Loops | Loop size display shows `---` without a BPM; footer shows seconds | Low | Reproduced |
| 10 | Waveform 2D | Paused decks never draw waveform data after load | High | Reproduced |
| 11 | Waveform 2D | Waveform is offset from the playhead for the first half-window of a track | High | Reproduced |
| 12 | Waveform 2D | Single-sample peak lookup aliases at wide zoom | Low | Code |
| 13 | Waveform GL | Fixed 4096-texel texture with point sampling gives low-detail waveforms | High | Reproduced |
| 14 | Waveform GL | No zoom or colour-mode controls in WebGL mode | Medium | Reproduced |
| 15 | Waveform GL | Idle-skip check omits peaks, duration, loop and grid offset | Low | Code (not reproduced) |
| 16 | Waveform both | Loop region is not drawn | High | Reproduced |
| 17 | Waveform both | "RGB" / "3B" colouring isn't frequency data | Medium | Code + UI |
| 18 | Waveform both | Toggle and toolbar overlap canvas labels; overview looks clickable but isn't | Low | Reproduced |
| 19 | MIDI | Learned MIDI mappings never fire any action | **Critical** | Code |
| 20 | Analysis | Unanalysed tracks cause 8 console 404s per load, and nothing in the UI explains it | Low | Reproduced |

No JavaScript errors were thrown during testing. These worked as expected:

- Zoom limits (2–60 s).
- Colour-mode switching in 2D.
- Re-enabling a loop after playing past it jumps back into the loop ("reloop").
- Track loading from the library.
- Play/pause.

---

## Findings: Loops

### 1. LOOP with no loop set turns on a zero-length loop

Files: `components/dj/v2/DJLoopSection.tsx:78`, `slices/djMixerSlice.ts:765-773`

The size button calls `toggleLoop`, which just flips `enabled` on the default `{ start: 0, end: 0 }`.

Observed at 62.8 s into deck A:

- The button turned green with `aria-pressed="true"` and the label `---`.
- The footer read `0.00s`.
- Halve and double became enabled.
- Nothing looped, because the engine requires `end > start`.

### 2. Halve/double on the empty loop jumps playback to 0:00

File: `lib/djAudio.ts:2256-2265` (and `doubleLoop` at `2241-2251`)

`halveLoop` only checks `loop.enabled`, not whether the loop has any length. It computes `start + max(0 / 2, 0.1)`, which gives a 0.1 s loop at **0:00**. The next position tick sees `currentTime >= loop.end` and seeks to `loop.start`.

| Step | Result |
|------|--------|
| Before halve | pos 62.777 s |
| After halve | pos 0.078 s, footer `0.10s` |
| Double ×3 | footer `0.80s`, stuck looping 0.0–0.8 s |

In a live set this sends the playing track to its first beat with no way back except turning the loop off and seeking.

### 3. The v2 panel has no control that creates a loop

File: `components/dj/v2/DJLoopSection.tsx:19-39`

`LOOP_SIZES` and `handleSetLoop` → `setLoopBeats` are defined but not rendered. The only controls are toggle, halve and double, and all three act on an existing loop. The v1 `components/dj/DJLoopPanel.tsx` still wires loop in/out, beat presets and clear, but v2 does not.

### 4. `setLoopBeats` / `setLoopOut` switch the loop off after creating it

Files: `lib/djAudio.ts:2207-2236`, `lib/djAudio.ts:2184-2202`, `slices/djMixerSlice.ts:755-763`

The store's `setLoop` always writes `enabled: true`. Both engine methods capture `deckState` before calling it, then run `if (!deckState.loop.enabled) toggleLoop(deck)`. With the loop previously off, the snapshot is stale: `setLoop` enables it and `toggleLoop` immediately disables it. Setting a beat loop or loop-out from an "off" state leaves the loop **off**.

Affected callers:

- The v1 loop panel.
- The MIDI `loopOut` action, once MIDI is wired up (see #19).
- Any future v2 preset button.

### 5. `setLoopIn` starts an active 4 s loop immediately

File: `lib/djAudio.ts:2164-2179`

With no existing end point, `setLoopIn` calls the store's `setLoop(deck, pos, pos + 4)`, which enables it. Loop-in should only mark a pending start point until loop-out is pressed.

### 6. Loop wrap jitters and discards the overshoot

File: `lib/djAudio.ts:2285-2291` (deck B at `2305-2311`)

The wrap check runs once per animation frame and seeks to exactly `loop.start` when `currentTime >= loop.end`. Measured over 4 s on a 0.8 s loop:

- Wrap intervals: 817, 800, 817, 800 ms.
- Position before the wrap, relative to the first wrap: 0.0, −0.5, −9.2, +8.2, −9.2 ms.
- Every wrap landed at exactly `loop.start + 0`.

Frame-quantised wrapping produces up to ~17 ms of spread between iterations, and the overshoot is thrown away rather than carried into the next pass. Seeking an `HTMLMediaElement` isn't sample-accurate either and can cause a short gap on each wrap. This is audible on ¼- and ½-beat loops (117 ms and 234 ms at 128 BPM).

### 7. Loop wrap depends on `requestAnimationFrame`

File: `lib/djAudio.ts:2272-2336`

`startPositionTracking` is driven by `requestAnimationFrame`. Browsers pause rAF for hidden tabs, and WebView2 does the same when its window is minimised or occluded. Audio keeps playing, so an active loop would stop wrapping and the track would play past the loop end.

This comes from the code and was not reproduced: headless Chromium does not throttle rAF.

### 8. Loops ignore quantize and the beat grid

File: `lib/djAudio.ts:2226`

`setLoopBeats` starts at `audioElement.currentTime` and sizes the loop as `beats × 60 / bpm`. It ignores `djMixer.quantize`, `beatGrid` and `beatGridOffset`, so loops start off-beat and drift on tracks with variable-tempo grids.

### 9. Loop size display

File: `components/dj/v2/DJLoopSection.tsx:29-54`, `pages/DJModeV2.tsx:720-725`

- The button computes beats from `effectiveBpm || originalBpm`. With no BPM it shows `---` even while a loop runs, because the engine uses a 120 BPM fallback that the display doesn't know about.
- The footer `LOOP` label shows seconds (`0.80s`) while the button shows beats, so the two disagree.

---

## Findings: PERF Waveforms

PERF uses the `timeline` view. It renders `DJDualWaveform` (Canvas 2D) by default, because `useWebGLWaveform` defaults to `false` (`slices/djMixerSlice.ts:441`). Otherwise it renders `DJWebGLWaveform`.

### 10. Paused decks never draw waveform data after load (2D)

File: `components/dj/v2/DJDualWaveform.tsx:489-502`

The frame loop skips drawing when both decks are idle and `posA`/`posB` haven't changed. The first frame after mount draws the placeholder with the position at 0. Loading a track leaves the position at 0, so the arrival of waveform peaks, duration, cue points, hot cues or grid edits never causes a redraw.

Observed:

- Five seconds after loading both decks, both lanes still showed "No waveform data".
- The canvases' lit-pixel coverage was identical to the empty state (0.19% / 0.24%).
- Both lanes only rendered once deck A started playing.

Setting `canvas.width` on resize also clears the canvas, and the same skip would leave it blank while paused.

### 11. Waveform is offset from the playhead near the start of a track (2D)

File: `components/dj/v2/DJDualWaveform.tsx:283-331`

`visibleStartTime = Math.max(0, position - halfWindow)` is then used as the pixel-0 time: `pixelTime = visibleStartTime + x × secondsPerPixel`. When `position < visibleSeconds / 2`, the waveform shifts left, and the playhead shows audio from `visibleSeconds / 2` instead of `position`. That covers the first 5 s at the default 10 s zoom and the first 30 s at 60 s zoom.

- Beat-grid lines, the cue marker and double-click seek all use the correct mapping, `playheadX + (t − position) / secondsPerPixel`, so they don't line up with the drawn waveform in that region.
- In the screenshot, deck B at 0:00 draws from the left edge instead of starting at the centre playhead.
- The WebGL shader maps correctly (`DJWaveformShaders.ts:114`), so this bug is 2D only.

### 12. Single-sample peak lookup aliases at wide zoom (2D)

File: `components/dj/v2/DJDualWaveform.tsx:312, 328`

Each pixel reads one peak: `peaks[floor(pixelTime × peaksPerSecond)]`. At 172 peaks/s:

- At 10 s zoom this is about 1:1 and fine.
- At 60 s zoom about 5.5 peaks map to each pixel, and all but one are skipped, so transients flicker as the window scrolls.

### 13. Low-detail WebGL waveform

File: `components/dj/v2/webgl/DJWebGLRenderer.ts:102, 428-435`

Every track is resampled into a fixed 4096-texel texture by point sampling (`peaks[floor(i × step)]`):

| Track | Server peaks | Texels/s | Texels in 10 s window (1856 px) |
|-------|--------------|----------|---------------------------------|
| A (213 s) | 36,746 | 19.2 | ~192 |
| B (585 s) | 100,863 | 7.0 | ~70 |

The result is the smeared, blobby waveform in the WebGL screenshots. Point decimation (9:1 and 25:1 here) also drops most peaks outright, so kicks and snares disappear. `lib/clientWaveform.ts` has a similar ceiling of 1200 peaks per track. That fallback path wasn't used in this test, but it would make the 2D renderer blocky too.

### 14. No zoom or colour-mode controls in WebGL mode

Files: `components/dj/v2/webgl/DJWebGLWaveform.tsx:41-45`, `pages/DJModeV2.tsx:582-586`

- `visibleSeconds` is a prop that defaults to 10 and is never passed.
- The zoom and colour toolbar lives inside `DJDualWaveform`.
- Switching to WebGL loses zoom, colour modes and Ctrl+scroll. Playwright found 0 zoom and 0 colour buttons in WebGL mode.

### 15. WebGL idle-skip check omits some state

File: `components/dj/v2/webgl/DJWebGLWaveform.tsx:146-160`

The skip condition compares position, track id, cue, hot cues and beat grid. It does not compare `waveformPeaks`, `duration`, `beatGridOffset` or `loop`. Peaks that arrive after the track-id-change frame, or a grid-offset edit while paused, would not repaint. This was not reproduced: peaks arrived in time during the test.

### 16. Loop region is not drawn (both renderers)

Neither `drawMainWaveform`, the overview, nor any WebGL shader reads `deck.loop`. An active loop has no visual indication on the waveform. This is the primary loop feedback in every DJ application.

### 17. "RGB" / "3B" colouring isn't frequency data

Files: `components/dj/v2/DJDualWaveform.tsx:291-318`, `components/dj/v2/webgl/DJWaveformShaders.ts`

- **RGB** is a vertical gradient: red at the edges, green at the centre. The code comment says: "For now, simulate frequency bands with amplitude zones".
- **3B** colours bars by loudness.
- The WebGL shader uses the same amplitude-based palette.

Both are labelled as frequency waveforms. The backend waveform (`backend/internal/api/dj_waveform.go`) returns a single amplitude series, so no frequency data exists to render.

### 18. Layout overlaps and a fake affordance

Files: `pages/DJModeV2.tsx:566-580`, `components/dj/v2/DJDualWaveform.tsx:407-410, 620-624, 680-723`

- The `2D`/`WEBGL` toggle at `top-1 left-1` covers the canvas-drawn `OVERVIEW` label and the `DECK A` lane label.
- The zoom/colour toolbar at `top-1 right-2` covers the right end of the overview strip and the top-right of deck A's lane.
- The overview canvas has `cursor-pointer` but no click handler.
- Minor: lane height is fractional (`63.5px` CSS vs a 63 px backing store), which causes slight vertical resampling.

---

## Findings: Other

### 19. MIDI mappings never fire

Files: `lib/djMidi.ts:100, 233, 299`, `components/dj/v2/DJMidiMapping.tsx`

`DJMidiService.handleMessage` returns early when `actionHandler` is null, and nothing in the codebase calls `setActionHandler`. The only consumer of `getDJMidiService()` is the mapping dialog. Users can learn and save mappings, but no MIDI action ever runs: play, cue, loops, EQ, crossfader or anything else.

### 20. Analysis 404s with no UI indication

Each deck load requests `analysis`, `beatgrid`, `energy` and `recommendations`, and all four 404 for an unanalysed track. That's 8 console errors for two decks. The deck shows `BPM --.--` with no hint that analysis is missing, or that sync, beat jump and beat loops are degraded as a result.

---

## Remediation Plan

The phases are ordered by risk to a live set. Each phase can ship on its own.

### Phase 1: Stop loops from breaking playback (findings 1, 2, 4, 5)

1. **Make loop state writes explicit.**
   - Replace the implicit `enabled: true` in the store's `setLoop` with an explicit write, for example `setLoopState(deck, { start, end, enabled })`. Alternatively, keep `setLoop` but give it an `enabled` parameter.
   - Remove every "read snapshot → `setLoop` → `toggleLoop`" sequence in `lib/djAudio.ts`. The engine should compute the final loop state and write it once.
   - Update `slices/djMixerSlice.test.ts` to match.
2. **Validate loops in every action.** Add `hasValidLoop(loop) = loop.end > loop.start`:
   - `halveLoop` / `doubleLoop` do nothing when the loop is invalid.
   - `toggleLoop` on an invalid loop creates a default auto-loop (4 beats at the current position, quantised in Phase 3) instead of enabling an empty one. This matches Rekordbox and Serato "Auto Loop" behaviour.
   - Express the halve floor in beats (for example 1/32 beat) rather than 0.1 s. Clamp double to `duration`.
3. **Make loop-in pending.** Add a pending loop-in point (for example `loop.pendingIn: number | null`) that doesn't enable anything. `setLoopOut` then creates and enables the loop from `pendingIn`, or from `loop.start` when adjusting an existing loop.
4. **Disable halve/double in the UI** unless a valid loop exists, not just `loop.enabled`.
5. **Tests.** Add unit tests for the engine's loop math (extract the pure computations into `lib/djLoop.ts`) and store tests covering off→set, on→halve, empty→halve and in→out.

### Phase 2: Waveform correctness in PERF (findings 10, 11, 16, 18)

1. **2D redraw invalidation.**
   - Replace the position-only idle skip in `DJDualWaveform.tsx` with a dirty check over everything that affects the picture: `waveformPeaks`, `duration`, `beatGrid`, `beatGridOffset`, `cuePoint`, `hotCues`, `loop`, `visibleSeconds`, `colorMode` and canvas size.
   - A simple approach is a `useStore.subscribe` that sets `needsRedraw = true` whenever any of those references change.
   - Set `needsRedraw` after `resizeCanvases`.
2. **Apply the same dirty check in the WebGL renderer** (finding 15).
3. **Share the time↔pixel mapping.** Add a single `timeToX` / `xToTime` helper centred on the playhead, `x = playheadX + (t − position) / secondsPerPixel`. Use it for waveform bars, beat grid, cue, hot cues, loop band and click/scratch seek. Drop the clamped `visibleStartTime` as a mapping origin, and skip pixels where `t < 0 || t > duration`.
4. **Draw the loop region.**
   - Draw a translucent band from `loop.start` to `loop.end` with edge markers.
   - Use a brighter colour when `enabled` and a dim outline when set but disabled.
   - Add a matching marker on the overview.
   - In WebGL, add a loop uniform pair to the cue-point pass rather than a new program.
5. **Fix the layout.**
   - Move the 2D/WebGL toggle into the waveform toolbar.
   - Inset canvas labels so they clear the toolbar.
   - Either implement overview click-to-seek (left half = deck A, right half = deck B) or remove `cursor-pointer`.
   - Round lane height to whole pixels.

### Phase 3: Loop precision and musical behaviour (findings 3, 6, 7, 8, 9)

1. **Wrap without losing the overshoot (short term).** Seek to `start + ((currentTime − start) mod length)` instead of `start`, so timing error doesn't accumulate.
2. **Move the loop check off rAF.**
   - Drive the wrap check from a timer that keeps running in hidden and minimised windows, for example a Worker-based ticker at ~5 ms.
   - Keep rAF only for UI position updates.
   - Verify in the Wails build with the window minimised.
3. **Sample-accurate loops (long term).**
   - `HTMLMediaElement` seeking can't be sample-accurate.
   - Evaluate running loops inside the existing `vinyl-scratch` AudioWorklet, which already holds decoded audio, or on an `AudioBufferSourceNode` using `loopStart`/`loopEnd`.
   - Treat this as its own design task, because it changes how decks play back.
4. **Quantize.** When `djMixer.quantize` is on and a grid exists, snap the loop start to the nearest beat (applying `beatGridOffset`). Take the loop length from grid beats rather than `60 / bpm`.
5. **v2 loop controls.**
   - Tapping the size button creates or toggles an auto-loop of the selected size.
   - Halve/double change the selected size when no loop is active, and resize the loop when one is.
   - Optionally add loop-in/out buttons for parity with v1 and MIDI.
6. **Consistent display.**
   - Add a shared `getDeckLoopBpm(deck)` that includes the 120 BPM fallback, so the button always shows a size.
   - Show beats in the footer, with seconds only as a tooltip.

### Phase 4: Waveform fidelity (findings 12, 13, 14, 17)

1. **Max-pool decimation.**
   - Wherever peaks are reduced, take the max over each bucket rather than a single sample: 2D at wide zoom, WebGL texture upload, and the overview cache.
2. **WebGL resolution.**
   - Upload the full peak series.
   - Use a 2D texture layout (`width = min(n, 4096)`, `rows = ceil(n / width)`), and compute `(u, v)` in the shader from the sample index.
   - Check against `MAX_TEXTURE_SIZE`. The 585 s track needs 100,863 texels, which is 25 rows at 4096.
3. **Shared waveform view state.**
   - Move `visibleSeconds` and `colorMode` into the `djMixer` slice, persisted like the other mixer preferences.
   - Render one toolbar component in `DJModeV2.tsx` above whichever renderer is active.
   - Pass `visibleSeconds` to `DJWebGLWaveform` and the scratch hooks.
4. **Frequency colouring.**
   - Short term: relabel the modes to describe what they actually show, for example Gradient, Level and Solid.
   - Long term: extend `analysis.GenerateWaveformOverview` / `db.DJWaveform` to store low, mid and high band peaks (for example three biquad-filtered envelopes computed in the same decode pass), and colour bars from those bands in both renderers.
5. **Client fallback resolution.** Size the `lib/clientWaveform.ts` output by duration (for example 150 peaks/s) instead of a fixed 1200.

### Phase 5: MIDI and analysis UX (findings 19, 20)

1. **Wire MIDI.**
   - Add a `useDJMidiActions()` hook mounted by `DJModeV2Inner`.
   - It calls `getDJMidiService().setActionHandler(...)` and maps each `MidiAction` to the corresponding engine or store action (`useDJAudioEngineActions`, `setCrossfader`, EQ, loops, hot cues).
   - Clear the handler on unmount.
   - Add a unit test that dispatches a fake message through a stored mapping.
   - Depends on Phase 1 for the loop actions to behave.
2. **Analysis state.**
   - Have the backend return a typed "not analysed" response (200 with `status: "not_analyzed"`, or a single status endpoint) instead of 404s. Alternatively, have the frontend skip the dependent requests once the feature request says the track has no analysis.
   - Show a "Not analysed — Analyse" badge in place of `BPM --.--`.

## Testing Plan

There is no committed e2e setup yet, although `@playwright/test` 1.59 is already a dependency.

1. **Unit tests (Vitest).**
   - Loop math in the new `lib/djLoop.ts`: create, halve and double with floors, quantised start, overshoot-preserving wrap.
   - Store loop transitions.
   - The `timeToX`/`xToTime` helper, including `position < visibleSeconds / 2`.
   - WebGL max-pool resampling (extend `DJWebGLRenderer.test.ts`).
   - MIDI dispatch.
2. **E2E spec (`e2e/dj-panel.spec.ts`)**, using the same init-script approach as this review: wrap `window.Audio` and instrument the `currentTime` setter.
   - Load two tracks → both lanes have waveform pixels while paused (finding 10).
   - At position 0, the first non-silent waveform column is at or right of the playhead (finding 11).
   - LOOP with no loop set → a valid loop at the current position, with no jump to 0 (findings 1, 2).
   - Halve/double keep `start` and change the length by 2×.
   - Wrap timing: over 4 s, the spread of wrap positions stays within a set tolerance (finding 6).
   - The loop band is visible on the waveform while enabled (finding 16).
   - WebGL mode shows zoom controls and follows zoom changes (finding 14).
3. **Manual verification in the Wails desktop build.**
   - WebGL on WebView2.
   - Loop behaviour with the window minimised (finding 7).
   - A physical MIDI controller after Phase 5.
