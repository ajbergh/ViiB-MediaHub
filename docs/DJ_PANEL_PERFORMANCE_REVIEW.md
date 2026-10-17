# DJ panel performance review

Reviewed October 17, 2026 against the running `/dj` page and current branch.

## Findings and fixes

| Priority | Finding | Evidence | Result |
| --- | --- | --- | --- |
| High | Closed inspectors rendered on playback ticks | `DJEnergyInsights` subscribed to both complete deck objects; `DJBeatGridEdit` subscribed to a complete deck. Inspectors stay mounted while hidden. | Subscribe to relevant fields with shallow equality and to the pristine-deck guard boolean. Keep mounted state and action-time safety checks. |
| High | Transport updates serialized and wrote unrelated preferences | A browser probe of 200 `setDeckPosition` calls caused 200 localStorage writes and 252,600 serialized characters. | Deduplicate the persisted subset before JSON serialization. Repeating the probe produces zero writes/characters; preference changes still save immediately. |
| Medium | GPU path was opt-in | New mixer defaults set `useWebGLWaveform` to false, although the renderer already supported GPU rendering. | New settings prefer WebGL. Detect support on the rendering canvas: WebGL2, then WebGL1, then Canvas. Preserve saved Canvas choices and merge missing mixer defaults. |
| Medium | Some remaining work wakes up independently | Six VU canvas loops; a 5 ms loop-worker ticker after audio initialization; separate waveform, jog and engine loops. | Candidates for the next measured optimization, not changed in this pass. |

The early `shouldUseAdvancedWebGL` check is browser eligibility, not a capability guarantee. The actual renderer uses a tested context-creation helper which handles null returns and exceptions without creating an additional probe context. Shader initialization failure also retains Canvas fallback. At the initial review, three-band and vertical waveforms used Canvas; the follow-up below adds horizontal three-band GPU rendering and a Canvas worker path.

## Measurements

Foreground Chromium via Playwright/CDP, Vite development build, 1440×1000 viewport, two loaded and playing decks using WebGL. Tracks: “¿Por Qué Les Mientes?” and “…And Justice For All”. Closed inspectors remained mounted. A five-second `requestAnimationFrame` sampler ran alongside the CPU sampler.

| Metric | Before inspector subscription fix | After |
| --- | ---: | ---: |
| Observed animation callbacks in ~5 seconds | 110 | 277 |
| 95th-percentile callback interval | 150 ms | 16.8 ms |
| Intervals over 33.4 ms | 48 | 3 |

Before the fix, sampled execution was dominated by development React JSX creation, validation and DOM updates, despite both waveforms already using WebGL. After narrowing inspector subscriptions, those costs fell substantially. A React Profiler regression verifies that 200 deck position updates produce no inspector commits after the relevant pristine-state transition; editing a beat grid still commits an update.

These are diagnostic observations, not a controlled production speedup claim. The development server, CPU sampling, background application work and concurrent validation can influence results. Initial background-tab measurements were discarded; foreground runs explicitly checked `document.hidden === false`. This pass does not quantify GPU time, native audio glitches, WebView performance, or low-end hardware behavior.

A separate synthetic Canvas draw probe used a 30-minute duration, 100,000 peak samples, a stress grid of 30,000 markers, a 1856×72 canvas, a ten-second window, ten warmup draws and 100 timed draws. Mean CPU submission/drawing time was 0.584 ms, p95 1.0 ms on this environment. It excludes compositing/GPU completion and audio processing. Canvas is therefore a valid fallback here; expensive hidden React work was the stronger measured bottleneck.

## Technique plan from the initial review

Items 1–5 are implemented in the follow-up below, covering the four grouped recommendations presented to the user. Item 6 remains conditional future GPU profiling work.

1. **Move three-band rendering onto the GPU.** Upload three amplitude textures when track data changes; select a band through uniforms and draw its full-height geometry. Keep the provider/source/timeline validation and existing Canvas path. This is the largest feature gap in the current GPU route; benchmark large local/provider artifacts before committing to the implementation.
2. **Use multiresolution peak data and cached drawing tiles for Canvas.** Precompute max-pooled levels once per track/source and choose the resolution matching the visible pixel density. Cache static waveform tiles, then composite moving windows and dynamic cue/grid overlays. Maintain peak maxima, original time mapping and scratch/seek behavior. MDN recommends caching repeated drawing in an offscreen canvas: [Optimizing canvas](https://developer.mozilla.org/en-US/docs/Web/API/Canvas_API/Tutorial/Optimizing_canvas).
3. **Sleep the loop ticker when no fallback loop needs it.** The 5 ms ticker currently checks both decks even without active loops. Start/stop it based on playback and loop eligibility, preserving sample-accurate AudioWorklet/source looping. Regression-test looping, scratching, track replacement and background playback first.
4. **Consolidate meter scheduling and cache meter artwork.** Use a shared visual frame scheduler and one canvas per stereo meter; sample analyser levels once, and redraw only changed segments/peak holds. Keep audio transport timing independent of visual frames.
5. **Move heavy fallback drawing to a worker where supported.** Feature-detect `OffscreenCanvas` and transferable canvas support; retain main-thread Canvas fallback. Send immutable peak data once and small position/control messages afterward. Measure worker messaging and memory costs first: [OffscreenCanvas](https://developer.mozilla.org/en-US/docs/Web/API/OffscreenCanvas).
6. **Reduce GPU passes and adapt drawing resolution only if GPU profiling justifies it.** The amplitude renderer uses up to five passes per lane. Consider combining compatible overlays, retaining textures/buffers, and selecting an appropriate device-pixel ratio. Avoid synchronous GPU queries in animation loops and preserve context restoration: [WebGL best practices](https://developer.mozilla.org/en-US/docs/Web/API/WebGL_API/WebGL_best_practices).

The audio engine already uses AudioWorklets for key transport/scratch processing, and band loaders already coalesce concurrent source/session reads. Replacing these with new machinery without measurement is not a priority. Library-derived work such as `computeArtists` also appeared in samples, but its contribution needs separate attribution before changing library state behavior.

## Validation and reproduction

- Full `npm run check`: style checks, TypeScript, 123 test files / 548 tests, production build. Existing bundle-size/mixed-import warnings remain.
- Regression coverage: no position-triggered persistence, immediate preference persistence, failed-write retries, WebGL2/WebGL1/null/exception paths, default GPU preference and user toggle, and inspector render counts.
- Browser checks: fresh settings render both amplitude lanes with WebGL; an existing Canvas preference renders both with Canvas; simulated unavailable WebGL produces usable Canvas lanes despite the GPU preference.
- Playback was paused after profiling.

For a repeatable production benchmark, serve the production build, load the same tracks, fix viewport/DPR and zoom, warm up, then compare multiple five-second foreground runs without running tests simultaneously. Record frame intervals, long tasks, CPU profiles, storage calls and audio underruns separately. Repeat on the target Wails WebViews and an integrated GPU before choosing a worker/GPU-resolution strategy.

## Implementation follow-up

Implemented the four requested improvement groups:

| Improvement | Implementation |
| --- | --- |
| GPU three-band drawing | Packed RGBA byte textures work in both WebGL2 and WebGL1 without float-texture extensions. Low/Mid/High preserve common scaling, channel colors and full-height isolation. A shared loader validates track identity, session and duration; animation callbacks recheck ownership before drawing. |
| Multiresolution peaks and Canvas tiles | Immutable arrays have exact max-pooling pyramids. A bounded LRU stores 512-CSS-pixel static timeline tiles; moving frames composite tiles and redraw dynamic grid/loop/cue/playhead overlays. Zoom, resize, source, band, palette and orientation changes invalidate the cache. The limit is 24 tiles with an approximate 16 MiB target and a two-tile minimum per renderer. |
| Shared meters and sleeping loop checks | Three stereo canvases replace six channel canvases, sharing one visual scheduler, cached segment backgrounds and change-based repainting. Idle/hidden visual clocks sleep. The 5 ms audio loop ticker starts only for playing, enabled fallback loops and stops for pause, loop-off, unload or scratching. Native sample-accurate worklet loops remain independent. |
| Worker drawing | The worker owns an OffscreenCanvas and returns transferable ImageBitmaps. Main-thread pointer/transport logic is unchanged. One outstanding frame plus one latest request bounds queue growth. Delta state messages avoid repeatedly cloning unchanged peaks, bands, grids and cues; generations reject stale results. Startup failure, timeout, missing OffscreenCanvas or worker failure retain a usable main-thread Canvas renderer. The DOM canvas is never irreversibly transferred. |

Browser checks with real loaded tracks confirmed WebGL three-band drawing, isolated bands, vertical worker drawing, and identical horizontal playhead coordinates (754.739 px at the checked 1440 px viewport). A forced WebGL1 context rendered real three-band data successfully. Simulated unavailable WebGL plus failed Worker construction or absent OffscreenCanvas produced usable Canvas lanes. A one-second double-click seek offset moved the source by 0.995 seconds. A real fallback loop created its ticker while playing and removed it on pause.

A same-run synthetic comparison (30-minute duration, 100k peaks, 30k grid markers, 1856×72 pixels, ten-second zoom, 120 moving frames after warmup) measured direct drawing at 0.691 ms mean / 1.4 ms p95 and cached drawing at 0.449 ms mean / 0.9 ms p95. The cache built five tiles and reused tiles 619 times including warmup. These are CPU submission/drawing observations, not end-to-end production or GPU-completion measurements.

Additional regression tests cover exact range maxima including unpaired trailing peaks, cache reuse/invalidation in both orientations, worker backpressure/state deltas/stale results/timeouts, texture channel packing and silent data, current-source ownership, shared scheduling, fallback loop lifecycle, and native-loop exclusion. Target Wails/WKWebView hardware qualification remains separate from browser verification.

Final follow-up validation: `npm run check` passed with 129 test files / 561 tests, TypeScript, both style checks and the production build. The captured log is `output/dj-performance-check.log`. Existing mixed-import and bundle-size warnings remain. The preferred renderer was restored to WebGL and playback was left paused after browser checks.
