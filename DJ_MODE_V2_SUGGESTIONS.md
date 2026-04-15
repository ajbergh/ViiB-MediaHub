# DJ Mode V2: Review & Suggestions for Improvement

This document outlines a review of the current state of the DJ Mode V2 interface (`DJModeV2.tsx` and related components) and provides actionable suggestions for improving both layout/UX and rendering performance.

---

## Implementation Status

| # | Suggestion | Status | Notes |
|---|-----------|--------|-------|
| 1.1 | Responsive Scaling / Container Queries | ⏳ Pending | Hard `min-w-[1152px]` remains; `@container` on decks not yet done |
| 1.2 | Compact Mixer Mode | ⏳ Pending | Mixer still requires scrolling on short screens |
| 1.3 | Touch Target Sizing | ⏳ Pending | No Touch Mode toggle yet |
| 1.4 | Side-by-Side Library Layout | ⏳ Pending | Bottom drawer only |
| 2.1 | Zustand Selector Optimization | ✅ Already done | All inline components were already using granular primitive selectors (`eqHigh`, `eqMid`, `filterValue`, etc.) and `!!track` guards. No action needed. |
| 2.2 | Decouple Keyboard Shortcuts | ✅ **Implemented** | Extracted to `components/dj/v2/hooks/useDJShortcuts.ts`. Listener now attaches once on mount via stable `useRef` — no more dep-array churn. |
| 2.3 | ResizeObserver vs Window Resize | ✅ **Implemented** | `window.addEventListener('resize')` replaced with `ResizeObserver` attached to the DJ Mode root `containerRef`. Now reacts to sidebar open/close and any layout shift. |
| 2.4 | Waveform Rendering Offloading | ✅ Already done | WebGL waveform (`DJWebGLWaveform`) exists and is togglable. Canvas 2D fallback is also available. |
| 2.5 | Component Extraction | ✅ **Implemented** | `DJModeV2.tsx` reduced from **1,139 → ~843 lines**. Extracted: `DeckTimeDisplay`, `DeckHasTrack`, `DeckBpmBadge` → `DJDeckComponents.tsx`; `DJChannelStrip`, `DJMasterKnob`, `DJCrossfaderSelfSub`, `DJTempoSliderSelfSub` → `DJMixerComponents.tsx`. |

---

## 1. Layout & UX Improvements

### 1.1. Responsive Scaling & Graceful Degradation
*   **Current State:** The app relies on a hard `min-w-[1152px]` to prevent horizontal crushing, and uses `overflow-y-auto` on the mixer to handle vertical constraints.
*   **Suggestion:** Implement more aggressive Container Queries (`@container`) on the Decks themselves. 
    *   If a deck's width drops below a certain threshold, automatically hide secondary controls (like the Beat Grid editor or Beat Jump) into a collapsible "Advanced" menu.
    *   This would allow the `min-w` to be lowered (e.g., to `900px`), making the app usable on smaller laptop screens or split-screen setups.

### 1.2. Mixer Density & "Compact" Mode
*   **Current State:** The mixer strip contains Trim, 3-band EQ, Filter, and Volume faders. On shorter screens, this requires scrolling (`overflow-y-auto`), which is not ideal for live performance where controls need to be instantly accessible.
*   **Suggestion:** Introduce a "Compact Mixer" layout mode. When vertical space is `< 700px`, automatically collapse the 3-band EQ into a single "Color/Filter" knob, or hide the Trim knobs, ensuring the Volume faders and Crossfader remain visible without scrolling.

### 1.3. Touch Target Sizing
*   **Current State:** Many buttons (Loop halve/double, Grid adjustments) use very small text (`text-[9px]`) and small heights (`h-7`).
*   **Suggestion:** Since ViiB compiles to a Windows desktop app (which may be used on Surface tablets or touch-enabled laptops), consider adding a "Touch Mode" toggle that increases minimum button heights to `44px` (standard touch target size) and uses icons instead of dense text.

### 1.4. Library Browser Integration
*   **Current State:** The library is a bottom drawer that can be resized vertically.
*   **Suggestion:** Offer a "Side-by-Side" layout option (similar to Serato's Browse mode) where the library takes up the left or right half of the screen, and the decks are stacked vertically. This utilizes widescreen monitors much more efficiently than a bottom drawer.

---

## 2. Performance & Architecture Improvements

### 2.1. Zustand Selector Optimization
*   **Current State:** Components use selectors like `const track = useStore(state => deck === 'A' ? state.djDeckA.track : state.djDeckB.track);`.
*   **Suggestion:** If `track` is an object, any update to the track object (even if it's just a play count increment) will cause a re-render. 
    *   **Fix:** Select primitive values where possible. For example, if a component only needs to know *if* a track is loaded, use `const hasTrack = useStore(state => !!state.djDeckA.track?.id);`.
    *   Use `useShallow` from `zustand/react/shallow` if selecting multiple properties from the store to prevent unnecessary renders when unrelated properties change.

### 2.2. Decoupling Keyboard Shortcuts
*   **Current State:** `DJModeV2.tsx` contains a massive `useEffect` (lines ~500-580) handling all keyboard shortcuts. Every time one of the dependencies (like `togglePlay`, `seek`, `handleSync`) changes, the event listener is removed and re-added.
*   **Suggestion:** Extract this into a dedicated hook: `useDJShortcuts()`. Use a `useRef` to hold the latest state/callbacks so the `keydown` event listener only needs to be attached once on mount, reducing overhead and potential memory leaks.

### 2.3. ResizeObserver vs. Window Resize
*   **Current State:** The library height clamping uses `window.addEventListener('resize', ...)`.
*   **Suggestion:** Use a `ResizeObserver` attached to the main DJ Mode container. This ensures the layout reacts to *any* layout shift (e.g., a sidebar opening/closing) rather than just the browser window resizing.

### 2.4. Waveform Rendering Offloading
*   **Current State:** Dual waveforms are rendered in the UI thread (or via WebGL, depending on the implementation state).
*   **Suggestion:** Ensure that waveform peak data calculation and rendering are fully offloaded to a Web Worker or an `OffscreenCanvas`. The main thread should only be responsible for passing the current `currentTime` to the worker, ensuring the UI remains 60fps even during heavy track analysis.

### 2.5. Component Extraction
*   **Current State:** `DJModeV2.tsx` is over 1,100 lines long and contains several inline components (`DeckTimeDisplay`, `DJChannelStrip`, etc.).
*   **Suggestion:** Move these self-subscribing components into their own files within `components/dj/v2/`. This will make `DJModeV2.tsx` much easier to read, maintain, and test.

---

---

## 3. Remaining Next Steps

1.  **Implement Container Queries** (§1.1): Replace the hard `min-w-[1152px]` with `@container` rules on the decks to collapse the Beat Grid and Beat Jump sections on smaller screens.
2.  **Compact Mixer Mode** (§1.2): Auto-collapse the 3-band EQ to a single Color/Filter knob when vertical space is `< 700px`.
3.  **Touch Mode Toggle** (§1.3): Add a button that bumps minimum touch targets to `44px` and switches to icon-only labels.
4.  **Side-by-Side Library Layout** (§1.4): Offer a layout where the library takes up a left/right half and decks stack vertically (Serato Browse-style).