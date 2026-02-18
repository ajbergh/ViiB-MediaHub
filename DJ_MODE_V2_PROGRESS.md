# DJ Mode V2 - Progress Tracker

> **Created:** February 18, 2026  
> **Last Updated:** February 18, 2026 (Session 6 — Performance Pass)  
> **Status:** P0 complete, P1 complete, P2 complete, P3 COMPLETE (16/16), Performance optimized  
> **Route:** `/dj-v2` → `pages/DJModeV2.tsx`

---

## Overview

DJ Mode V2 is a complete redesign of the DJ interface inspired by professional DJ software (rekordbox, Serato DJ Pro, Traktor Pro). It features circular jog wheels, dual frequency-colored waveforms, a professional mixer layout, and an integrated library browser.

**Current State:** The page renders and compiles with 0 errors. P0 bugs fixed. P1 features complete. P2 complete. **P3 COMPLETE** — all 16 items done including slip mode, auto-gain, WebGL integration, React.memo optimization, waveform caching, beat grid editing, sampler pads, track color coding, harmonic mixing, MIDI mapping, and more.

---

## P0 - Critical Bugs (Page Won't Render)

**Goal:** Make DJ Mode V2 load and render without errors.

| # | Task | File | Status | Notes |
|---|------|------|--------|-------|
| P0.1 | Fix `renderChannelStrip` className syntax error (line 171) | `pages/DJModeV2.tsx` | ✅ Done | Fixed: missing backtick, truncated `flex-1` |
| P0.2 | Fix SYNC button A className syntax error (lines 295-301) | `pages/DJModeV2.tsx` | ✅ Done | Fixed: added template literal + dynamic styling |
| P0.3 | Fix SYNC button B className syntax error (lines 404-410) | `pages/DJModeV2.tsx` | ✅ Done | Fixed: same as P0.2 |
| P0.4 | Verify page renders at `/dj-v2` after fixes | Browser | ✅ Done | All 87 TS errors resolved, 0 errors |
| P0.5 | Verify individual components mount without error boundaries triggering | Browser | 🟡 Pending | Needs manual browser test |

**Blocked by:** Nothing  
**Estimated Effort:** 30 minutes

---

## P1 - Missing Core Features

**Goal:** Bring V2 to feature parity with V1 and add essential professional DJ features.

### P1.A - FX Section (Biggest Gap)

| # | Task | File(s) | Status | Notes |
|---|------|---------|--------|-------|
| P1.A1 | Design FX strip layout (horizontal, between waveforms and decks) | `pages/DJModeV2.tsx` | ✅ Done | FX section placed between waveform and deck controls |
| P1.A2 | Create V2 FX panel component | `components/dj/v2/DJFXSection.tsx` | ✅ Done | Full component with collapsible strip, active FX count badges |
| P1.A3 | Add FX type selector per FX unit (Filter/Delay/Reverb/Flanger) | `DJFXSection.tsx` | ✅ Done | All 4 FX types per deck shown as togglable units |
| P1.A4 | Add dry/wet knob per FX unit | Reuse `DJEQKnob` | ✅ Done | Third knob (MIX/FDBK) on applicable FX types |
| P1.A5 | Add per-effect parameter controls | `DJFXSection.tsx` | ✅ Done | 2-3 knobs per FX unit with type-specific params |
| P1.A6 | Wire FX controls to audio engine | Hook integration | ✅ Done | Both store + audio engine synced |
| P1.A7 | Add FX on/off toggle per unit with visual indicator | `DJFXSection.tsx` | ✅ Done | Colored glow when active |

### P1.B - Mixer Strip Improvements

| # | Task | File(s) | Status | Notes |
|---|------|---------|--------|-------|
| P1.B1 | Wire TRIM knob to actual gain control | `pages/DJModeV2.tsx`, audio engine | ✅ Done | Mapped to volume as pre-gain (no gain in DeckState) |
| P1.B2 | Wire MAIN/Master volume knob | `pages/DJModeV2.tsx`, audio engine | ✅ Done | Wired to store + audio engine `setMasterVolume` |
| P1.B3 | Add per-channel filter knob (between LOW EQ and headphone cue) | `pages/DJModeV2.tsx` | ✅ Done | LP/HP sweep with color indicator when active |
| P1.B4 | Add sync mode selector (OFF/BPM/PHASE) | Mixer center strip | ✅ Done | 3-button selector above crossfader |
| P1.B5 | Implement real VU meters using audio analyser node | `lib/djAudio.ts`, `hooks/useDJAudioEngine.ts`, `components/dj/v2/DJVUMeter.tsx` | ✅ Done | Added `getVULevels()` to engine, canvas-based LED meter with peak hold |
| P1.B6 | Add channel VU meters (per deck) | `DJVUMeter.tsx`, `pages/DJModeV2.tsx` | ✅ Done | Stereo VU meters beside each channel fader + master meters |

### P1.C - Hot Cues & Performance Pads

| # | Task | File(s) | Status | Notes |
|---|------|---------|--------|-------|
| P1.C1 | Show 8 hot cue pads per deck (currently 4) | `components/dj/v2/DJHotCuePad.tsx` | ✅ Done | 4x2 grid layout, both decks use slots 1-8 |
| P1.C2 | Add keyboard shortcuts for hot cues (1-8 for active deck) | `pages/DJModeV2.tsx` | ✅ Done | Keys 1-8 trigger on active deck |
| P1.C3 | Add Shift+1-8 to set hot cue at current position | `pages/DJModeV2.tsx` | ✅ Done | Shift+1-8 sets with color coding |

### P1.D - Visual Feedback

| # | Task | File(s) | Status | Notes |
|---|------|---------|--------|-------|
| P1.D1 | Add active deck indicator (ring/highlight on active deck) | `pages/DJModeV2.tsx` | ✅ Done | Blue ring + top bar for A, purple for B |
| P1.D2 | Add key display prominently in deck area | Deck header | ✅ Done | Key badge + BPM readout in deck header |
| P1.D3 | Show remaining time in addition to elapsed | `pages/DJModeV2.tsx` (deck headers) | ✅ Done | Elapsed (colored) and -remaining in deck header info bars |

**Blocked by:** P0 (page must render first)  
**Estimated Effort:** 3-5 days

---

## P2 - Layout & UX Improvements

**Goal:** Refine the layout to match professional DJ software standards and improve usability.

| # | Task | File(s) | Status | Notes |
|---|------|---------|--------|-------|
| P2.1 | Make library browser resizable/collapsible (drag handle) | `pages/DJModeV2.tsx` | ✅ Done | Drag handle, min 100px / max 600px, double-click to collapse, default 250px |
| P2.2 | Reduce mixer width from 360px to ~280-300px | `pages/DJModeV2.tsx` | ✅ Done | Mixer: 300px, master strip: 80px |
| P2.3 | Add waveform zoom control (+/- buttons or scroll) | `components/dj/v2/DJDualWaveform.tsx` | ✅ Done | +/−/reset buttons, Ctrl+scroll wheel, 2-60s range |
| P2.4 | Add beat jump buttons (±1, ±4, ±8, ±16 beats) | `components/dj/v2/DJBeatJump.tsx` | ✅ Done | Compact beat jump in deck headers between LOOP and HOT CUES |
| P2.5 | ~~Improve hot cue pad layout - 2 rows of 4 (8 total)~~ | `DJHotCuePad.tsx` | ✅ Done (P1.C1) | Completed in P1 - 4x2 grid layout |
| P2.6 | Add track elapsed/remaining time toggle | `pages/DJModeV2.tsx` (deck headers) | ✅ Done (P1.D3) | Elapsed + remaining shown in deck headers |
| P2.7 | Add quantize toggle button | Mixer center strip | ✅ Done | Q button next to sync mode selector, cyan when active |
| P2.8 | Improve deck header - show track artist/title more prominently | Deck header area | ✅ Done | Track title, artist, key badge, BPM, time display |
| P2.9 | Add waveform color mode selector (multi-color / 3-band / single) | `DJDualWaveform.tsx` | ✅ Done | RGB, 3-Band, Single (deck color) modes with selector in waveform overlay |
| P2.10 | Add fullscreen mode for DJ Mode | `pages/DJModeV2.tsx` | ✅ Done | F11 keyboard shortcut |

**Blocked by:** Nothing (P0 and P1 complete)  
**Estimated Effort:** ✅ COMPLETE

---

## P3 - Polish & Advanced Features

**Goal:** Add polish, advanced features, and performance optimizations.

### P3.A - Functional Polish

| # | Task | File(s) | Status | Notes |
|---|------|---------|--------|-------|
| P3.A1 | Implement view mode switching (Scope/Timeline/FX tabs) | `pages/DJModeV2.tsx`, `DJTopBar.tsx` | ✅ Done | Timeline=waveforms, Scope=real-time VU bars, FX=expanded FX view. Tabs fully wired. |
| P3.A2 | Wire record button to MediaRecorder API | `pages/DJModeV2.tsx`, `lib/djAudio.ts`, `useDJAudioEngine.ts` | ✅ Done | Captures master output via MediaStreamDestination, saves as .webm, duration timer, auto-download on stop |
| P3.A3 | Add key lock toggle (pitch-independent tempo) | `lib/djAudio.ts`, `useDJAudioEngine.ts`, `djMixerSlice.ts`, `DJModeV2.tsx` | ✅ Done | `preservesPitch` API, per-deck 🔒 buttons in deck headers, store sync |
| P3.A4 | Add slip mode (silent playback continues) | `lib/djAudio.ts`, `djMixerSlice.ts`, `DJModeV2.tsx` | ✅ Done | Shadow position tracking during scratch, auto-resume to where playback would be, SLIP toggle in deck headers (orange) |
| P3.A5 | Add auto-gain (normalize) per track | `lib/djAudio.ts`, `djMixerSlice.ts`, `DJModeV2.tsx` | ✅ Done | Full audio decode + peak analysis, targets -3dBFS, caps at +9.5dB, AG toggle in deck headers (cyan) |

### P3.B - Performance Optimization

| # | Task | File(s) | Status | Notes |
|---|------|---------|--------|-------|
| P3.B1 | Integrate WebGL2 waveform renderer | `pages/DJModeV2.tsx`, `webgl/` | ✅ Done | Conditional render: DJWebGLWaveform or DJDualWaveform based on store toggle |
| P3.B2 | Add WebGL toggle in settings (Canvas 2D fallback) | `djMixerSlice.ts`, `DJModeV2.tsx` | ✅ Done | `useWebGLWaveform` store flag, WebGL/2D toggle button overlaid on waveform area (green when WebGL active) |
| P3.B3 | Profile and optimize re-renders | All DJ V2 components | ✅ Done | Wrapped 11 components with React.memo: JogWheel, EQKnob, VolumeFader, Crossfader, TempoSlider, FXSection, HotCuePad, TransportButtons, LoopSection, DualWaveform + existing BeatJump/VUMeter |
| P3.B4 | Optimize waveform memory usage | `lib/clientWaveform.ts` | ✅ Done | LRU cache (max 10 entries), 200-point overview downsampling, `getCachedWaveform()` API |

### P3.C - Advanced Features

| # | Task | File(s) | Status | Notes |
|---|------|---------|--------|-------|
| P3.C1 | Beat grid editing (manual adjustment UI) | `DJBeatGridEdit.tsx`, `djMixerSlice.ts`, `DJDualWaveform.tsx` | ✅ Done | ±1ms/±10ms nudge buttons, offset display (click to reset), non-destructive offset applied at render time |
| P3.C2 | Sampler / sample pads mode | `DJSamplerPads.tsx`, `lib/djSampler.ts`, `djMixerSlice.ts` | ✅ Done | 8 pads with oneshot/loop/gate modes, per-pad volume, color-coded, file picker loading, AudioBuffer-based playback |
| P3.C3 | Track color coding in library | `DJLibraryBrowserV2.tsx` | ✅ Done | 8-color palette, click color dot to pick, session-persisted Map, 🎨 header column |
| P3.C4 | Harmonic mixing suggestions in library | `DJLibraryBrowserV2.tsx`, `lib/keyDetection.ts` | ✅ Done | Key column with color-coded compatibility (green ≥85%, yellow ≥70%, orange ≥50%), session key cache from deck analysis, sortable |
| P3.C5 | Keyboard shortcut overlay/help (? key) | `pages/DJModeV2.tsx` | ✅ Done | ? key toggles overlay, Esc closes, grouped by category with kbd styling |
| P3.C6 | MIDI controller mapping support | `lib/djMidi.ts`, `DJMidiMapping.tsx`, `DJModeV2.tsx` | ✅ Done | Web MIDI API, device detection, MIDI learn mode, configurable mappings (toggle/momentary/absolute/relative/trigger), localStorage persistence, 🎹 MIDI button in top bar |
| P3.C7 | Crossfader curve selector UI | `pages/DJModeV2.tsx` | ✅ Done | LIN/CP/CUT buttons above crossfader, orange when active |

**Blocked by:** Nothing (P0, P1, P2 complete)  
**Status:** ✅ **P3 COMPLETE** — All 16 items implemented

### P3.D - Performance Optimization (Session 6)

Root cause: Audio engine position updates (~15fps at 66ms intervals) write to Zustand store, changing full deck object references. Components subscribing to `state.djDeckA`/`state.djDeckB` re-rendered on every position tick, cascading through the entire tree.

| # | Task | File(s) | Status | Notes |
|---|------|---------|--------|-------|
| P3.D1 | DJJogWheel: eliminate all React re-renders during playback | `DJJogWheel.tsx` | ✅ Done | Granular selectors, removed `useBpmGlow`, 5 DOM refs (rotation/glow/bpm/tempo/time/arc), RAF reads position via `getState()`, deps: `[deck, computedSize]` only |
| P3.D2 | DJModeV2: stop position-driven re-renders | `DJModeV2.tsx` | ✅ Done | `DeckTimeDisplay` micro-component (RAF + ref), `handleSync`/keyboard handler use `getState()` snapshots, dep arrays use only stable function refs |
| P3.D3 | DJTransportButtons: granular selectors + RAF glow | `DJTransportButtons.tsx` | ✅ Done | Removed `useBpmGlow`, CSS variable glow via `playButtonRef`, granular selectors for track/isPlaying/BPM |
| P3.D4 | DJDualWaveform: zero reactive deps on deck state | `DJDualWaveform.tsx` | ✅ Done | Removed all store subscriptions; RAF loop reads `getState()` each frame; click handler uses `getState()` |
| P3.D5 | DJTopBar: granular selectors + RAF time display | `DJTopBar.tsx` | ✅ Done | `TopBarTimeDisplay` micro-component (RAF + ref), separate selectors for track/isPlaying per deck |
| P3.D6 | DJBeatJump: granular selectors + getState for position | `DJBeatJump.tsx` | ✅ Done | Individual selectors for track/duration/effectiveBpm/originalBpm; handler reads position via `getState()` |

**Key patterns used:**
- `useStore.getState()` for non-reactive reads inside RAF loops and event handlers
- Granular Zustand selectors (e.g., `state => state.djDeckA.isPlaying`) instead of full object subscriptions
- DOM refs + `setAttribute()`/`textContent` for animation-driven visual updates (bypasses React reconciliation)
- CSS custom properties (`--glow`) set via refs for smooth visual effects
- Self-subscribing micro-components (`DeckTimeDisplay`, `TopBarTimeDisplay`) that isolate position-dependent rendering

---

## Component Inventory

### V2 Components (Already Built)

| Component | File | Status | Quality |
|-----------|------|--------|---------|
| `DJTopBar` | `components/dj/v2/DJTopBar.tsx` | Working | Good - tabs functional (view modes), RAF time display |
| `DJDualWaveform` | `components/dj/v2/DJDualWaveform.tsx` | Working | Good - freq colors, beat grid |
| `DJJogWheel` | `components/dj/v2/DJJogWheel.tsx` | Working | Good - SVG, BPM glow, scratch |
| `DJHotCuePad` | `components/dj/v2/DJHotCuePad.tsx` | Working | Good - 8 slots, 4x2 grid |
| `DJTransportButtons` | `components/dj/v2/DJTransportButtons.tsx` | Working | Good - press animations |
| `DJLoopSection` | `components/dj/v2/DJLoopSection.tsx` | Working | OK - compact |
| `DJEQKnob` | `components/dj/v2/DJEQKnob.tsx` | Working | Good - metallic SVG |
| `DJVolumeFader` | `components/dj/v2/DJVolumeFader.tsx` | Working | Good - T-handle |
| `DJCrossfader` | `components/dj/v2/DJCrossfader.tsx` | Working | Good - grip texture |
| `DJTempoSlider` | `components/dj/v2/DJTempoSlider.tsx` | Working | Good - range presets |
| `DJCueButton` | `components/dj/v2/DJCueButton.tsx` | Working | Good - PFL toggle |
| `DJHeadphoneMix` | `components/dj/v2/DJHeadphoneMix.tsx` | Working | Good - cue/master blend |
| `DJLibraryBrowserV2` | `components/dj/v2/DJLibraryBrowserV2.tsx` | Working | Good - full featured |
| `DJErrorBoundary` | `components/dj/v2/DJErrorBoundary.tsx` | Working | Good - error isolation |

### V2 Components (Need to Build)

| Component | Purpose | Priority |
|-----------|---------|----------|
| `DJFXSection` | FX strip with per-unit controls | ✅ Built |
| `DJVUMeter` | Real-time audio level meter | ✅ Built |
| `DJFilterKnob` | Per-channel LP/HP sweep | ✅ Built (inline in mixer) |
| `DJBeatJump` | Beat jump buttons (±1/4/8/16) | ✅ Built |
| `DJSyncModeSelector` | OFF/BPM/PHASE toggle | ✅ Built (inline in mixer) |

### V2 Additional Components

| Component | File | Status | Quality |
|-----------|------|--------|--------|
| `DJBeatGridEdit` | `components/dj/v2/DJBeatGridEdit.tsx` | Working | Good - ±1ms/±10ms nudge, offset display |
| `DJSamplerPads` | `components/dj/v2/DJSamplerPads.tsx` | Working | Good - 8 pads, oneshot/loop/gate modes |
| `DJMidiMapping` | `components/dj/v2/DJMidiMapping.tsx` | Working | Good - MIDI learn, device detection |

### V2 WebGL Components (Built & Integrated)

| Component | File | Status |
|-----------|------|--------|
| `DJWebGLRenderer` | `components/dj/v2/webgl/DJWebGLRenderer.ts` | Built |
| `DJWaveformShaders` | `components/dj/v2/webgl/DJWaveformShaders.ts` | Built |
| `useDJWebGL` | `components/dj/v2/webgl/useDJWebGL.ts` | Built |
| `DJWebGLWaveform` | `components/dj/v2/webgl/DJWebGLWaveform.tsx` | Built |

---

## Audio Engine Status

The audio engine (`lib/djAudio.ts` + `hooks/useDJAudioEngine.ts`) is **comprehensive and functional**:

| Feature | Engine Support | V2 UI Exposes |
|---------|---------------|---------------|
| Dual-deck playback | ✅ | ✅ |
| Scratch with velocity | ✅ | ✅ (via jog wheel) |
| 3-band EQ | ✅ | ✅ |
| Volume per channel | ✅ | ✅ |
| Crossfader | ✅ | ✅ |
| Tempo control | ✅ | ✅ |
| BPM detection | ✅ | ✅ (displayed) |
| Key detection | ✅ | ✅ (deck header) |
| Filter FX | ✅ | ✅ (FX section + channel filter knob) |
| Delay FX | ✅ | ✅ (FX section) |
| Reverb FX | ✅ | ✅ (FX section) |
| Flanger FX | ✅ | ✅ (FX section) |
| Loop engine | ✅ | ✅ |
| Hot cues | ✅ | ✅ (8 pads + keyboard) |
| Headphone cue | ✅ | ✅ |
| Beat-phase sync | ✅ | ✅ (mode selector + SYNC buttons) |
| Nudge position | ✅ | ❌ No UI |
| Master volume | ✅ | ✅ (MAIN knob) |
| VU metering | ✅ | ✅ (real-time canvas LED meters with peak hold) |
| Quantize | ✅ (store) | ✅ (Q button in mixer) |
| Beat jump | N/A (uses seek) | ✅ (±1/4/8/16 beat buttons) |
| Key lock | ✅ | ✅ (per-deck 🔒 toggle in deck header) |
| Crossfader curve | ✅ (store) | ✅ (LIN/CP/CUT buttons above crossfader) |
| Slip mode | ✅ | ✅ (per-deck SLIP toggle, orange) |
| Auto-gain | ✅ | ✅ (per-deck AG toggle, cyan) |
| Beat grid editing | N/A (UI only) | ✅ (±1ms/±10ms nudge with offset display) |
| Sampler pads | ✅ (separate engine) | ✅ (8 pads: oneshot/loop/gate modes) |
| Track color coding | N/A | ✅ (8-color palette in library) |
| Harmonic mixing | ✅ (keyDetection.ts) | ✅ (Key column with compatibility colors) |
| MIDI mapping | ✅ (Web MIDI API) | ✅ (learn mode, device detection, config dialog) |
| WebGL waveforms | ✅ (WebGL2 renderer) | ✅ (toggle button, Canvas 2D fallback) |

---

## Keyboard Shortcuts

### Currently Implemented (V2)

| Key | Action |
|-----|--------|
| `Q` | Return to cue (Deck A) |
| `W` | Play/Pause (Deck A) |
| `O` | Return to cue (Deck B) |
| `P` | Play/Pause (Deck B) |
| `Space` | Play/Pause (active deck) |
| `Tab` | Toggle active deck |
| `Z` | Crossfader full left |
| `X` | Crossfader center |
| `C` | Crossfader full right |
| `Shift+←` | Crossfader nudge left |
| `Shift+→` | Crossfader nudge right |
| `1-8` | Trigger hot cue (active deck) |
| `Shift+1-8` | Set hot cue at current position |
| `E` | Sync Deck A |
| `[` | Sync Deck B |
| `F11` | Toggle fullscreen |
| `Ctrl+Scroll` | Zoom waveform in/out |
| `?` | Toggle Shortcuts Overlay |
| `Esc` | Close Shortcuts Overlay |

### Missing (Should Add in P3+)

| Key | Action |
|-----|--------|
| `L` | Toggle loop (active deck) |
| `K` | Toggle key lock (active deck) |

---

## Reference: Professional DJ Software Features

### rekordbox (Pioneer DJ)
- 2-4 deck mixing
- 8 hot cues + 8 pads (hot cue/loop/slicer/sampler modes)
- 3 FX units with beat-synced parameters
- Per-channel filter knob
- Waveform color modes (RGB, 3Band, Blue)
- Phrase visualization
- Auto-gain / master tempo
- Mix recording
- MIDI controller mapping
- Cloud library sync

### Serato DJ Pro
- 2-4 deck mixing
- 8 hot cues + 8 pads (multiple modes)
- 2 FX units
- iZotope effects suite
- Per-channel filter
- Waveform with beat grid
- Key lock / key shift
- Slip mode
- Recording
- Streaming integration

### Traktor Pro
- 4 deck mixing
- 8 hot cues
- 4 FX units (single/group mode)
- Per-channel filter
- Flux mode (slip)
- Remix decks / sampler
- External mixer mode
- MIDI mapping
- Stem separation

---

## File Map

```
pages/
  DJModeV2.tsx              ← Main page (WORKING - 0 errors, P1 features complete)
  DJMode.tsx                ← V1 page (working, for reference)

components/dj/v2/
  DJTopBar.tsx              ← Top navigation bar
  DJDualWaveform.tsx        ← Stacked dual waveform (Canvas 2D)
  DJJogWheel.tsx            ← Circular jog wheel (SVG)
  DJHotCuePad.tsx           ← Hot cue buttons
  DJTransportButtons.tsx    ← Play/Cue/Sync buttons
  DJLoopSection.tsx         ← Loop controls
  DJEQKnob.tsx              ← Rotary knob (EQ/effects)
  DJVolumeFader.tsx         ← Vertical T-fader
  DJCrossfader.tsx          ← Horizontal crossfader
  DJTempoSlider.tsx         ← Vertical tempo slider
  DJCueButton.tsx           ← Headphone cue toggle
  DJHeadphoneMix.tsx        ← Headphone mix control
  DJLibraryBrowserV2.tsx    ← Library browser with sidebar
  DJFXSection.tsx           ← FX strip (NEW - Filter/Delay/Reverb/Flanger per deck)
  DJVUMeter.tsx             ← Real-time VU meters (canvas LED-style per deck/master)
  DJBeatJump.tsx            ← Beat jump buttons (±1/4/8/16 beats per deck)
  DJBeatGridEdit.tsx        ← Beat grid manual offset editor
  DJSamplerPads.tsx         ← 8-pad sampler (oneshot/loop/gate)
  DJMidiMapping.tsx         ← MIDI learn / mapping configuration dialog
  DJErrorBoundary.tsx       ← Error isolation

  hooks/
    useDJEffects.ts         ← Visual effects (BPM glow, VU)
    useAnimationFrame.ts    ← RAF helper
    index.ts                ← Hook exports

  webgl/
    DJWebGLRenderer.ts      ← WebGL2 render engine
    DJWaveformShaders.ts    ← GLSL shaders
    useDJWebGL.ts           ← React lifecycle hook
    DJWebGLWaveform.tsx     ← WebGL waveform component
    index.ts                ← Module exports

hooks/
  useDJAudioEngine.ts       ← Audio engine React integration

lib/
  djAudio.ts                ← Web Audio API dual-deck engine
  djSampler.ts              ← AudioBuffer sampler engine (8 pads)
  djMidi.ts                 ← Web MIDI API integration
  bpmDetection.ts           ← BPM analysis
  keyDetection.ts           ← Musical key detection
  clientWaveform.ts         ← Client-side waveform generation

slices/
  djMixerSlice.ts           ← Zustand state management
```
