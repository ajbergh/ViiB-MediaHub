# ViiB MediaHub - DJ Mode V2 Implementation Plan
**Date**: May 2, 2026  
**Scope**: DJ Mode V2 implementation guidance based on current repo state  
**Primary files**: `pages/DJModeV2.tsx`, `components/dj/v2/*`, `hooks/useDJAudioEngine.ts`, `lib/djAudio.ts`, `slices/djMixerSlice.ts`

---

## 0. Current Status

**Last updated**: May 2, 2026  
**Implementation started**: Yes  
**Verification status**: Phases 1-9 build verified with `npm run build`; `npm run typecheck` is blocked by Node/TypeScript heap OOM

| Phase | Status | Notes |
| --- | --- | --- |
| Phase 0 - Baseline And Safety Checks | Partial | Code reviewed against current repo. Runtime screenshots and manual smoke checks are still pending. |
| Phase 1 - Fix Nudge Shortcut Mismatch | Implemented, build verified | `Shift+ArrowLeft/Right` now calls active-deck nudge. `Alt+Shift+ArrowLeft/Right` adds fine nudge. Store position is updated immediately after nudge. |
| Phase 2 - Make FX Layout Behavior Coherent | Implemented, build verified | `FX` layout now switches to `racks`; leaving racks returns to `timeline`/`perf`. The placeholder text was removed and racks uses `--dj-fx-h`. |
| Phase 3 - Audio Setup Dialog | Implemented, build verified | Added `DJAudioSetup` dialog, `AUDIO` top-bar button, master/headphone output selectors, device enumeration, permission helper, support warnings, and main-output reapply during engine initialization. |
| Phase 4 - Library Drag-To-Deck Loading | Implemented, build verified | Library rows now emit a typed drag payload; Deck A/B panels accept drops, show deck-colored feedback, and call the existing `loadTrack(deck, song)` path. |
| Phase 5 - Tempo Accessibility And Visible Nudge Controls | Implemented, build verified | `DJTempoSlider` now has slider ARIA semantics and keyboard controls. `DJNudgeButtons` adds visible per-deck +/- nudge controls with Shift-click fine nudge. |
| Phase 6 - Beat FX Panel | Implemented, build verified | Added store-backed Beat FX state, a rack-mounted `DJBeatFXPanel`, store-to-engine sync, Deck A/B Beat FX reuse of existing FX nodes, and a dedicated master Beat FX chain after the mix bus and before the limiter. |
| Phase 7 - Master Cue | Implemented, build verified | Added store-backed `masterCueEnabled`, gated the audio engine headphone master send, wired sync/actions, and added a Master Cue toggle to `DJHeadphoneMix`. |
| Phase 8 - Sampler Persistence And Feedback | Implemented, build verified | Added sampler playback progress, clearer active/relink states, blob URL cleanup, and persisted sampler metadata with relink-required pads after reload. |
| Phase 9 - Library Table Refinements | Implemented, build verified | Added BROWSE-mode search autofocus, uncollapse-on-BROWSE behavior, persisted optional-column visibility, and persisted resize handles for Title/Artist/Album widths. |

### Progress Log

- May 2, 2026: Reviewed plan against current code. Confirmed `useDJAudioEngineActions()` already exposes `nudgePosition`; no new engine API was needed for Phase 1.
- May 2, 2026: Implemented Phase 1 shortcut behavior in `components/dj/v2/hooks/useDJShortcuts.ts`, wired `nudgePosition` from `pages/DJModeV2.tsx`, updated shortcut overlay labels, and synchronized store position after nudge in `hooks/useDJAudioEngine.ts`.
- May 2, 2026: Implemented Phase 2 layout behavior in `pages/DJModeV2.tsx` and `index.css`; `FX` layout now opens `racks`, leaving racks returns to normal layout, and the expanded FX area uses `--dj-fx-h`.
- May 2, 2026: Ran `npm run build`; Vite production build passed. Existing warnings remain for chunk size and mixed static/dynamic imports.
- May 2, 2026: Ran `npm run typecheck`; `tsc --noEmit` crashed with Node/TypeScript heap out-of-memory. Retried with `NODE_OPTIONS=--max-old-space-size=4096`; it still crashed before producing diagnostics.
- May 2, 2026: Implemented Phase 3 Audio Setup in `components/dj/v2/DJAudioSetup.tsx`, wired an `AUDIO` button/dialog into `pages/DJModeV2.tsx`, exported the component from `components/dj/v2/index.ts`, and fixed `lib/djAudio.ts` so a pre-selected main output device is applied during audio engine initialization.
- May 2, 2026: Ran `npm run build` after Phase 3; Vite production build passed with the same existing chunk-size and mixed static/dynamic import warnings.
- May 2, 2026: Implemented Phase 4 drag-to-deck loading in `components/dj/v2/DJLibraryBrowserV2.tsx` and `pages/DJModeV2.tsx`; library rows are draggable and Deck A/B panels load dropped tracks through `loadTrack(deck, song)`.
- May 2, 2026: Ran `npm run build` after Phase 4; Vite production build passed with the same existing warnings.
- May 2, 2026: Implemented Phase 5 tempo/nudge accessibility in `components/dj/v2/DJTempoSlider.tsx`, `components/dj/v2/DJNudgeButtons.tsx`, `components/dj/v2/index.ts`, and `pages/DJModeV2.tsx`; tempo sliders now expose keyboard slider semantics and Deck A/B have visible nudge buttons using the same 20ms/5ms increment language as shortcuts.
- May 2, 2026: Ran `npm run build` after Phase 5; Vite production build passed with the same existing Browserslist, chunk-size, and mixed static/dynamic import warnings.
- May 2, 2026: Started Phase 7 Master Cue. Confirmed `lib/djAudio.ts` already has a headphone master send controlled by `updateHeadphoneMix`, so implementation should gate that send with a store-backed `masterCueEnabled` flag rather than creating a separate audio path.
- May 2, 2026: Implemented Phase 7 Master Cue in `slices/djMixerSlice.ts`, `lib/djAudio.ts`, `hooks/useDJAudioEngine.ts`, and `components/dj/v2/DJHeadphoneMix.tsx`; master cue now toggles only the headphone master feed while preserving independent per-deck PFL cue buttons.
- May 2, 2026: Ran `npm run build` after Phase 7; Vite production build passed with the same existing Browserslist, chunk-size, and mixed static/dynamic import warnings.
- May 2, 2026: Started Phase 6 Beat FX. Reviewed `DJFXSection`, `DJFXPad`, `slices/djMixerSlice.ts`, and `lib/djAudio.ts`; Deck A/B can reuse existing FX methods, while true Master target requires a dedicated master FX chain after the mix bus and before the limiter.
- May 2, 2026: Implemented Phase 6 Beat FX in `slices/djMixerSlice.ts`, `lib/djAudio.ts`, `hooks/useDJAudioEngine.ts`, `components/dj/v2/DJBeatFXPanel.tsx`, `components/dj/v2/DJFXSection.tsx`, and `components/dj/v2/index.ts`; the expanded racks view now exposes target/effect/fraction/depth/on-off controls.
- May 2, 2026: Ran `npm run build` after Phase 6; Vite production build passed with the same existing Browserslist, chunk-size, and mixed static/dynamic import warnings.
- May 2, 2026: Started Phase 9 Library Table Refinements. Confirmed `DJLibraryBrowserV2` already handles search/sort/load/drag; adding focused refinements for BROWSE auto-focus plus persisted optional-column visibility and core column widths.
- May 2, 2026: Implemented Phase 9 in `components/dj/v2/DJLibraryBrowserV2.tsx` and `pages/DJModeV2.tsx`; BROWSE layout now expands the library, focuses search, persists optional column visibility, and lets Title/Artist/Album widths be resized and persisted.
- May 2, 2026: Ran `npm run build` after Phase 9; Vite production build passed with the same existing Browserslist, chunk-size, and mixed static/dynamic import warnings.
- May 2, 2026: Started Phase 8 Sampler Persistence And Feedback. Confirmed sampler audio is loaded from object URLs, so durable audio persistence is not safe; implementing metadata persistence plus an explicit relink state, and adding runtime progress feedback for playing pads.
- May 2, 2026: Implemented Phase 8 in `slices/djMixerSlice.ts`, `lib/djSampler.ts`, and `components/dj/v2/DJSamplerPads.tsx`; sampler pads now show playback progress, retain metadata across reloads, expose relink-required state for non-durable local files, and revoke replaced/cleared blob URLs.
- May 2, 2026: Ran `npm run build` after Phase 8; Vite production build passed with the same existing Browserslist, chunk-size, and mixed static/dynamic import warnings.

---

## 1. Goal

Turn DJ Mode V2 from a strong visual prototype into a more complete, hardware-aligned DJ workflow.

This plan focuses on the currently verified gaps:

- Audio output routing UI is missing, although engine methods already exist.
- FX layout behavior is confusing because `FX` layout and `racks` view are separate concepts.
- The shortcut overlay says `Shift+Left/Right` nudges, but the handler moves the crossfader.
- Library drag-to-deck loading is claimed in comments but not implemented.
- Tempo sliders need keyboard/fine-control accessibility.
- Hardware-style Beat FX and Master Cue are missing.
- Position smoothing and true vinyl scratch require careful technical follow-up.

Do not spend implementation time on stale items that are already done:

- Large color-coded transport buttons.
- Custom rotary EQ/filter knobs.
- Master volume as a rotary knob.
- Visible library search input.
- Loaded sampler names and per-pad volume controls.
- Red/high-contrast waveform playhead.

---

## 2. Working Principles

- Keep changes incremental. Each phase should be independently usable.
- Prefer existing store actions and audio-engine methods over new duplicate state.
- Keep render-loop work outside React state when possible, matching the current rAF pattern.
- Preserve existing DJ controls while adding hardware-aligned workflows.
- Update shortcut overlay text whenever shortcut behavior changes.
- Add minimal targeted tests or audit scripts for behavior that is easy to regress.

---

## 3. Phase 0 - Baseline And Safety Checks

**Purpose**: Establish a known baseline before changing DJ behavior.

### Files to inspect

- `pages/DJModeV2.tsx`
- `components/dj/v2/hooks/useDJShortcuts.ts`
- `components/dj/v2/DJTopBar.tsx`
- `components/dj/v2/DJFXSection.tsx`
- `components/dj/v2/DJLibraryBrowserV2.tsx`
- `components/dj/v2/DJTempoSlider.tsx`
- `hooks/useDJAudioEngine.ts`
- `lib/djAudio.ts`
- `slices/djMixerSlice.ts`

### Checks

- Run the app and verify DJ Mode V2 can mount at supported width.
- Verify existing controls still work: load to deck, play/pause, cue, crossfader, FX toggles, sampler load.
- Record screenshots for these layouts: `PERF`, `BROWSE`, `FX`, `timeline`, `scope`, `racks`.
- Confirm current audio engine initializes only after a user gesture.

### Acceptance criteria

- Baseline screenshots are captured.
- No unrelated code cleanup is mixed into later phases.
- Any pre-existing failures are noted before implementation begins.

---

## 4. Phase 1 - Fix Nudge Shortcut Mismatch

**Priority**: High  
**Reason**: The UI documents a performance shortcut that currently does something else.

### Current state

The shortcut overlay in `DJModeV2.tsx` lists:

- `Shift+Left`: Nudge Left
- `Shift+Right`: Nudge Right

But `components/dj/v2/hooks/useDJShortcuts.ts` currently handles these keys by changing `state.djMixer.crossfader`.

The audio nudge path already exists:

- `hooks/useDJAudioEngine.ts`: `nudgePosition(deck, offsetMs)`
- `lib/djAudio.ts`: `nudgePosition(deck, offsetMs)`

### Implementation guidance

1. Extend `UseDJShortcutsOptions` in `components/dj/v2/hooks/useDJShortcuts.ts`.

   Add:

   ```ts
   nudgePosition: (deck: DeckId, offsetMs: number) => void;
   ```

2. Pass `nudgePosition` from `DJModeV2.tsx`.

   `DJModeV2.tsx` already calls `useDJAudioEngineActions()`. Add `nudgePosition` to the destructured actions and pass it into `useDJShortcuts`.

3. Change shortcut behavior.

   Recommended mapping:

   - `Shift+ArrowLeft`: `nudgePosition(activeDeck, -20)`
   - `Shift+ArrowRight`: `nudgePosition(activeDeck, 20)`
   - Optional fine mode: `Alt+Shift+ArrowLeft/Right` uses `-/+5`

4. Preserve crossfader keyboard control with explicit keys.

   Existing `Z`, `X`, `C` already set hard-left, center, hard-right. If incremental crossfader movement is still needed, use non-conflicting shortcuts and document them.

5. Update shortcut overlay text in `DJModeV2.tsx`.

   Include the active deck behavior so users understand what will move.

### Technical notes

- Read `activeDeck` from the store snapshot inside the shortcut handler, as the hook already does.
- Prevent default browser behavior for handled arrow shortcuts.
- Keep the listener single-mount as currently designed.

### Acceptance criteria

- `Shift+Left/Right` nudges the active deck, not the crossfader.
- Overlay text matches behavior.
- `Z`, `X`, `C` still work for crossfader position.
- No text input interactions are intercepted.

---

## 5. Phase 2 - Make FX Layout Behavior Coherent

**Priority**: High  
**Reason**: Pressing `FX` should visibly switch the UI into an FX-focused workspace.

### Current state

- `DJTopBar.tsx` exposes layout mode buttons: `PERF`, `BROWSE`, `FX`.
- `DJModeV2.tsx` separately exposes view modes: `timeline`, `scope`, `racks`.
- `DJFXSection` expands based on `djMixer.djLayoutMode === 'fx'`.
- `DJModeV2.tsx` hides the normal `DJFXSection` when `djLayoutMode === 'fx'` unless `viewMode === 'racks'`.

This means selecting layout `FX` can reduce library height without making the expanded FX section obvious.

### Preferred implementation

Treat `FX` layout as a layout mode that automatically surfaces expanded FX controls.

1. In `DJModeV2.tsx`, wrap layout mode changes.

   Instead of passing `setDJLayoutMode` directly to `DJTopBar`, create:

   ```ts
   const handleLayoutModeChange = useCallback((mode: DJLayoutMode) => {
     setDJLayoutMode(mode);
     if (mode === 'fx') setViewMode('racks');
   }, [setDJLayoutMode]);
   ```

2. Pass `handleLayoutModeChange` to `DJTopBar`.

3. Make `racks` view real.

   Current `racks` view renders `DJFXSection` and a placeholder. Replace the placeholder with useful expanded content or remove it.

   Recommended expanded content:

   - Larger `DJFXPad`.
   - Per-deck active FX summary.
   - Space for Phase 6 Beat FX.

4. Avoid double rendering.

   Keep one `DJFXSection` visible at a time:

   - Timeline/scope with non-FX layout: compact `DJFXSection`.
   - Racks/FX layout: expanded `DJFXSection`.

### Alternative implementation

If `FX` should only be a layout mode and not change view mode, render `DJFXSection` directly when `djLayoutMode === 'fx'` even if `viewMode !== 'racks'`.

### Acceptance criteria

- Clicking `FX` visibly reveals expanded FX controls.
- Clicking `PERF` returns to the normal performance workspace.
- Clicking `BROWSE` expands the library and keeps the expected view.
- No duplicate FX sections appear.

---

## 6. Phase 3 - Audio Setup Dialog

**Priority**: High  
**Reason**: Separate master and headphone outputs are essential for DJ use.

### Current state

`lib/djAudio.ts` already has:

- `setMainOutputDevice(deviceId: string)`
- `setHeadphoneOutputDevice(deviceId: string)`
- `getMainOutputDeviceId()`
- `getHeadphoneOutputDeviceId()`

The UI exposes `MIDI`, but not audio routing.

### New component

Create:

- `components/dj/v2/DJAudioSetup.tsx`

### Suggested props

```ts
interface DJAudioSetupProps {
  onClose: () => void;
}
```

### Dialog responsibilities

- Request or detect media device permission.
- Enumerate audio output devices.
- Show separate selects:
  - Master Output
  - Headphone/Cue Output
- Apply selections through `getDJAudioEngine()`.
- Show support warnings for browsers that do not support output sink selection.
- Provide a default-device option.
- Handle errors without crashing DJ Mode.

### Browser API guidance

Use:

```ts
const devices = await navigator.mediaDevices.enumerateDevices();
const outputs = devices.filter(d => d.kind === 'audiooutput');
```

In some browsers, labels may be blank until permission is granted. If labels are blank, request microphone permission only if the product accepts that tradeoff:

```ts
await navigator.mediaDevices.getUserMedia({ audio: true });
```

Then stop tracks immediately:

```ts
stream.getTracks().forEach(track => track.stop());
```

Do not require microphone permission just to render the dialog. Offer it as a "Show device names" action if needed.

### UI integration

In `DJModeV2.tsx`:

- Add `showAudioSetup` state.
- Add an `AUDIO` button next to the existing `MIDI` button.
- Render `<DJAudioSetup onClose={() => setShowAudioSetup(false)} />`.

### Store persistence options

Short term:

- Keep selected device IDs inside the engine only.

Better follow-up:

- Add persisted settings in a settings slice or local storage helper:
  - `djMainOutputDeviceId`
  - `djHeadphoneOutputDeviceId`

### Failure states

Handle:

- `navigator.mediaDevices` missing.
- No `audiooutput` devices returned.
- `AudioContext.setSinkId` unsupported.
- `HTMLAudioElement.setSinkId` unsupported.
- Device selection rejected.
- Previously saved device no longer exists.

### Acceptance criteria

- User can open an Audio Setup dialog from DJ Mode.
- Dialog lists output devices where supported.
- Master and headphone output selections call the existing engine methods.
- Unsupported browsers show clear non-blocking messages.
- Closing dialog returns to DJ Mode without layout shift.

---

## 7. Phase 4 - Library Drag-To-Deck Loading

**Priority**: High  
**Reason**: This is a standard DJ software interaction and the component currently claims support that is not implemented.

### Current state

`DJLibraryBrowserV2.tsx` supports:

- Search.
- Sortable columns.
- Track color labels.
- Load-to-Deck A/B buttons.
- Double-click row to load Deck A.

It does not implement:

- `draggable`.
- `onDragStart`.
- Deck or waveform drop targets.

### Implementation guidance

1. Add a drag payload type.

   Use a small JSON payload:

   ```ts
   type DJTrackDragPayload = {
     type: 'viib-dj-track';
     songId: string;
   };
   ```

2. Add drag handlers to library rows.

   In `TableRow` inside `DJLibraryBrowserV2.tsx`:

   - Set `draggable={!!item}`.
   - On drag start, write payload to `dataTransfer`.
   - Also write `text/plain` fallback with the song ID.

3. Add drop handling in `DJModeV2.tsx`.

   Recommended helper:

   ```ts
   const handleTrackDrop = useCallback(async (event: React.DragEvent, deck: DeckId) => {
     event.preventDefault();
     // parse payload, find song, call loadTrack(deck, song)
   }, [songs, loadTrack]);
   ```

   `DJModeV2.tsx` currently uses `useDJAudioEngineActions()` but does not destructure `loadTrack`; add it.

4. Add drop targets.

   Good targets:

   - Entire Deck A panel.
   - Entire Deck B panel.
   - Optional: waveform rows for Deck A/B.

5. Add visual feedback.

   Use local state:

   ```ts
   const [dragOverDeck, setDragOverDeck] = useState<DeckId | null>(null);
   ```

   Add a subtle deck-colored ring or overlay when a valid track is dragged over.

6. Avoid interfering with existing row buttons.

   Button clicks should continue to call load-to-deck directly.

### Edge cases

- Song no longer exists by drop time.
- Drag payload is malformed.
- Dropping on the same deck with same loaded song.
- Loading fails.

### Acceptance criteria

- Dragging a library row onto Deck A loads Deck A.
- Dragging a library row onto Deck B loads Deck B.
- Existing A/B load buttons still work.
- Double-click-to-Deck-A still works.
- Invalid drops do nothing and do not throw.

---

## 8. Phase 5 - Tempo Accessibility And Visible Nudge Controls

**Priority**: Medium-high  
**Reason**: Controller-free beatmatching needs precise tempo and nudge controls.

### Current state

`DJTempoSlider.tsx` has strong visual affordances but lacks slider keyboard semantics.

### Tempo slider implementation

In `components/dj/v2/DJTempoSlider.tsx`:

1. Add slider semantics to the track element:

   ```tsx
   role="slider"
   tabIndex={disabled ? -1 : 0}
   aria-label={`Deck ${deck} tempo`}
   aria-valuemin={1 - tempoRange / 100}
   aria-valuemax={1 + tempoRange / 100}
   aria-valuenow={value}
   aria-valuetext={`${percentDisplay}, ${bpmDisplay} BPM`}
   ```

2. Add keyboard handling:

   - ArrowUp: increase tempo by 0.001 or 0.002.
   - ArrowDown: decrease tempo by 0.001 or 0.002.
   - Shift+ArrowUp/Down: larger step.
   - Home, Enter, or Space: reset to 1.0.

3. Add pointer fine mode:

   During pointer drag, if `shiftKey` is held, reduce sensitivity or quantize to smaller changes.

### Visible nudge buttons

Add a small `DJNudgeButtons` component or extend the deck tempo edge area.

Suggested component:

- `components/dj/v2/DJNudgeButtons.tsx`

Props:

```ts
interface DJNudgeButtonsProps {
  deck: DeckId;
  onNudge: (deck: DeckId, offsetMs: number) => void;
  disabled?: boolean;
}
```

Controls:

- `-` button: `onNudge(deck, -20)`
- `+` button: `onNudge(deck, 20)`
- Optional fine buttons or Shift-click: `-/+5`

Place near each `DJTempoSliderSelfSub` in `DJModeV2.tsx`.

### Acceptance criteria

- Tempo slider can be focused and adjusted with keyboard.
- Reset-to-center works from keyboard.
- Visible nudge buttons adjust the correct deck.
- Shortcut overlay and visible controls use the same nudge increment language.

---

## 9. Phase 6 - Beat FX Panel

**Priority**: Medium  
**Reason**: Hardware parity with Pioneer-style controllers requires a centralized Beat FX workflow.

### Current state

The app has per-deck effects:

- Filter.
- Delay.
- Reverb.
- Flanger.
- X-Y filter pad.

It does not have a centralized target-selectable Beat FX module.

### Suggested state model

Add to `slices/djMixerSlice.ts`:

```ts
export type BeatFXTarget = 'A' | 'B' | 'master';
export type BeatFXType = 'delay' | 'echo' | 'reverb' | 'filter' | 'flanger';
export type BeatFraction = '1/4' | '1/2' | '1' | '2' | '4';

interface DJBeatFXState {
  enabled: boolean;
  target: BeatFXTarget;
  type: BeatFXType;
  fraction: BeatFraction;
  depth: number; // 0..1
}
```

Add actions:

- `setBeatFXEnabled(enabled: boolean)`
- `setBeatFXTarget(target: BeatFXTarget)`
- `setBeatFXType(type: BeatFXType)`
- `setBeatFXFraction(fraction: BeatFraction)`
- `setBeatFXDepth(depth: number)`

### Audio engine guidance

Short term:

- Reuse existing delay/reverb/flanger paths where possible.
- For Deck A/B target, call the existing per-deck FX methods.
- For Master target, add a master FX chain after the mix bus and before master output.

Better long term:

- Add a dedicated beat FX graph:
  - Input bus.
  - Dry gain.
  - Wet gain.
  - Effect nodes.
  - Output merge.
- Calculate delay times from BPM and selected beat fraction:

```ts
const beatSeconds = 60 / bpm;
const delaySeconds = beatSeconds * fractionMultiplier;
```

Choose BPM:

- Target Deck A: Deck A effective/original BPM.
- Target Deck B: Deck B effective/original BPM.
- Master: active deck BPM, or playing deck BPM, fallback to 120.

### UI component

Create:

- `components/dj/v2/DJBeatFXPanel.tsx`

Controls:

- Target segmented control: A, B, MST.
- Effect select/menu.
- Beat fraction buttons.
- Depth knob.
- Large ON/OFF toggle.

Place it in:

- Expanded `racks`/`FX` view first.
- Later, a compact version can live in the normal FX strip.

### Acceptance criteria

- Beat FX can target Deck A, Deck B, or Master.
- Beat fraction changes affect time-based effects.
- ON/OFF toggle is large and visible.
- Existing per-deck FX still work.

---

## 10. Phase 7 - Master Cue

**Priority**: Medium  
**Reason**: DJs often need one-touch monitoring of the exact master mix.

### Current state

`DJHeadphoneMix` blends CUE to MST, but there is no Master Cue button.

### Implementation options

Option A - Store-backed toggle:

- Add `masterCueEnabled: boolean` to `djMixer`.
- Add `toggleMasterCue()`.
- In the audio engine, include/exclude master feed in headphone output based on the toggle.

Option B - UI shortcut to force mix:

- A Master Cue button sets `headphoneMix` to `1`.
- This is simpler but less expressive than true PFL/master-cue behavior.

Preferred: Option A.

### UI placement

Place near:

- Master level knob.
- Headphone cue mix section.

### Acceptance criteria

- Master Cue button has a clear active state.
- It affects headphone monitoring only, not master output.
- Existing per-deck cue buttons remain independent.

---

## 11. Phase 8 - Sampler Persistence And Feedback

**Priority**: Lower  
**Reason**: Current sampler is usable but session-oriented and visually minimal.

### Current state

Loaded sampler pads display:

- Sample name.
- Pad number.
- Mode.
- Volume range control.

Empty pads show `+`.

### Improvements

- Add loaded sample progress feedback while playing.
- Add looped-pad playing state that is unmistakable.
- Persist pad assignments if local file access model allows it.
- Consider waveform thumbnails after sample decode.

### Technical notes

Object URLs are not durable across sessions. Persistence requires either:

- Storing file handles, if the app supports File System Access API.
- Importing/copying samples into an app-managed storage location.
- Storing only metadata and requiring relink on restart.

### Acceptance criteria

- Playing state is obvious for one-shot and looped samples.
- Persistence behavior is explicit, even if persistence is deferred.

---

## 12. Phase 9 - Library Table Refinements

**Priority**: Lower  
**Reason**: The library is already functional but not as configurable as pro DJ libraries.

### Improvements

- Resizable columns for Title, Artist, Album.
- Column visibility menu for BPM, Key, Album, Time, and Genre.
- Persist column widths and visibility.
- Auto-focus search input when entering `BROWSE` layout.

### Implementation guidance

For search auto-focus:

- Add a ref or query hook in `DJLibraryBrowserV2.tsx`.
- Expose `autoFocusSearch?: boolean`, or handle it from layout mode.
- When `djLayoutMode === 'browse'`, focus the input on the next frame.

Avoid focusing search if:

- A modal is open.
- User is dragging/resizing.
- A text field already has focus.

### Acceptance criteria

- BROWSE mode focuses search when appropriate.
- Column changes persist across route changes.
- Table remains readable at compact heights.

---

## 13. Technical Follow-Up - Position Smoothing

**Priority**: Investigate before implementing  
**Reason**: A smoothing fix should be centralized and data-driven.

### Current state

These components read `engine.getPosition(deck)` in rAF loops:

- `DJJogWheel.tsx`
- `DJDualWaveform.tsx`
- `DJWebGLWaveform.tsx`

`lib/djAudio.ts` currently returns `audioElement.currentTime`.

### Measurement first

Before changing behavior:

- Log frame-to-frame deltas from `engine.getPosition(deck)` during playback.
- Compare against expected deltas from `performance.now()` and `playbackRate`.
- Test Chrome/Edge and Wails WebView if applicable.

### If smoothing is needed

Implement one central display getter in `lib/djAudio.ts`:

```ts
getSmoothedPosition(deck: DeckId): number
```

Track per deck:

- Last media time.
- Last `performance.now()`.
- Last playback rate.
- Playing state.
- Scratch state.
- Seek/version counter to reset interpolation on jumps.

Use raw position when:

- Paused.
- Scratching.
- Seeking.
- Track not ready.

Use interpolated position when:

- Playing normally.
- Playback rate is stable.

### Acceptance criteria

- rAF visuals use the smoothed getter only after measurement confirms a benefit.
- Store updates remain throttled.
- Seeking and loop jumps do not visibly overshoot.

---

## 14. Technical Follow-Up - True Vinyl Scratch

**Priority**: Long-term  
**Reason**: True reverse scratch audio is not possible with the current deck playback path.

### Current state

`updateScratch()` manipulates:

- `HTMLAudioElement.currentTime`
- `HTMLAudioElement.playbackRate`

Reverse scratch pauses forward playback and seeks backward. This can move the UI position backward, but it cannot produce real reverse audio.

### Long-term architecture

Move deck playback to a custom player:

- Decode tracks into `AudioBuffer`, or stream through `AudioWorklet`.
- Use a playhead controlled by velocity.
- Support positive and negative read speeds.
- Interpolate samples for smooth pitch/scratch sound.
- Keep Web Audio effect graph after the source.

### Migration strategy

1. Keep existing `HTMLAudioElement` playback as fallback.
2. Add an experimental buffer-backed deck source for local files.
3. Implement forward playback parity first.
4. Add reverse playback and scratch velocity control.
5. Reconnect existing EQ/FX/mixer graph.

### Acceptance criteria

- Forward playback matches current behavior.
- Reverse scratch produces audible reverse audio.
- Effects and mixer routing still work.
- Large tracks do not cause unacceptable memory pressure.

---

## 15. Suggested Implementation Order

1. Phase 1: Fix nudge shortcut mismatch.
2. Phase 2: Make FX layout behavior coherent.
3. Phase 3: Add Audio Setup dialog.
4. Phase 4: Implement library drag-to-deck loading.
5. Phase 5: Add tempo keyboard control and visible nudge buttons.
6. Phase 7: Add Master Cue.
7. Phase 6: Add Beat FX panel.
8. Phase 9: Improve library table and BROWSE focus.
9. Phase 8: Improve sampler persistence and feedback.
10. Investigate position smoothing.
11. Plan true vinyl scratch as a separate architecture project.

---

## 16. Verification Checklist

Use this checklist after each phase:

- DJ Mode V2 mounts at supported widths.
- No console errors on mount.
- Deck A and Deck B can each load and play tracks.
- CUE and PLAY buttons still work.
- Crossfader still affects output.
- Per-deck cue buttons and headphone mix still work.
- Waveform view still renders in Canvas 2D and WebGL modes.
- Scope view still renders VU bars.
- Racks/FX view still renders controls.
- Library search, sort, and load buttons still work.
- Keyboard shortcuts match overlay documentation.
- No modal traps focus permanently after close.
- No drag/drop operation throws on invalid payload.
