# DJv2 UI Refactor Engineering Plan

**Repository:** `ajbergh/ViiB-MediaHub`  
**Target:** `main` branch DJv2 UI  
**Primary entry point:** `pages/DJModeV2.tsx`  
**Status:** Proposed refactor plan  
**Scope:** UI composition, layout, responsive behavior, visual hierarchy, waveform presentation, and regression coverage. Audio-engine behavior should remain functionally unchanged unless explicitly called out.

---

## 1. Objective

Refactor DJv2 so the performance surface reads as a modern professional DJ workstation rather than a vertically stacked collection of independent widgets.

The visual target is a symmetric two-deck layout with:

- Deck A occupying the left side.
- Deck B occupying the right side.
- A fixed, visually centered mixer between them.
- The top waveform area split **50/50 horizontally**:
  - Deck A waveform in the left half.
  - Deck B waveform in the right half.
- Matching hierarchy and spacing between Deck A and Deck B.
- A library overlay that does not reflow or resize the deck workspace.
- A predictable geometry at supported desktop resolutions.
- No overlapping labels, controls, edit panels, or transport elements.

This plan deliberately preserves existing DJ state, audio-engine APIs, stems support, cue editing, BPM/key analysis, MIDI, recording, and virtualized library behavior.

---

## 2. Current implementation review

### 2.1 Overall shell

`pages/DJModeV2.tsx` is the orchestration surface. It currently:

- Mounts `DJTopBar`.
- Switches the upper area among timeline, scope, and racks.
- Renders `DJDualWaveform` or `DJWebGLWaveform` for timeline mode.
- Renders Deck A, a fixed-width center mixer, and Deck B in one flex row.
- Uses a fixed 1856×1090 authored canvas and scales the entire workstation to the viewport.
- Renders `DJLibraryDrawer` as an absolute overlay rather than a flex participant.

That architecture is viable and should be retained. The major problem is not the top-level state model; it is the composition and density of the visual regions.

### 2.2 Why the current layout becomes crowded

The current deck header is doing too much. Within the same vertical block it can include:

- track title/artist,
- horizontal VU,
- key,
- deck status,
- BPM,
- remaining and elapsed time,
- loop controls,
- beat jump,
- stems,
- beat-grid editing,
- energy insights,
- key verification,
- BPM editor,
- cue editor,
- and an overview waveform in racks mode.

This produces a tall, variable-content header that competes with the jog, EQ, mixer, and footer for the fixed design-canvas height.

The CSS also applies several global DJ-specific overrides after the component-level classes. Those overrides improve readability but make layout ownership harder to reason about because dimensions are split between JSX utility classes and late CSS selectors.

### 2.3 Current waveform model

Both waveform implementations are vertically stacked:

- `DJDualWaveform.tsx` renders a shared overview and then Deck A main waveform over Deck B main waveform.
- `DJWebGLWaveform.tsx` does the same using two WebGL canvases.

This is the largest structural mismatch with the requested design. The desired model is **left/right deck ownership**, not upper/lower deck ownership.

### 2.4 Existing strengths to preserve

The current code already has several strong foundations:

- self-subscribing mixer components to reduce unnecessary renders;
- store-to-engine synchronization outside the page render path;
- a virtualized DJ library;
- an overlay drawer that does not move the deck geometry;
- WebGL and Canvas waveform implementations with fallback;
- regression scripts for layout, virtualization, fullscreen, and playback continuity;
- explicit deck A/B identity colors;
- keyboard, MIDI, stems, cue, key, BPM, and analysis features already separated into reusable components.

The refactor should capitalize on those rather than replace them.

---

## 3. Target information architecture

The target page should have five vertically ordered regions:

```text
┌──────────────────────────────────────────────────────────────────────────┐
│ Top bar: workspace modes | current tracks | REC | AUDIO | MIDI | FS     │
├───────────────────────────────────┬──────────────────────────────────────┤
│ Deck A waveform (50%)             │ Deck B waveform (50%)                │
│ overview + main scrolling lane    │ overview + main scrolling lane       │
├───────────────────────┬───────────┴───────────┬──────────────────────────┤
│ Deck A                │ Mixer                 │ Deck B                   │
│ track header          │ channel A/B + master  │ track header             │
│ FX row                │ crossfader / cue mix  │ FX row                   │
│ jog + tempo + EQ      │ sampler               │ jog + tempo + EQ         │
│ hot cues / transport  │                       │ hot cues / transport      │
├───────────────────────┴───────────────────────┴──────────────────────────┤
│ Library affordance                                                        │
└──────────────────────────────────────────────────────────────────────────┘
```

The key design rule is that the left and right halves must remain symmetrical. Deck-specific controls should not cause one side to grow independently.

---

## 4. Proposed component architecture

### 4.1 Extract a layout shell

Create:

`components/dj/v2/DJPerformanceWorkspace.tsx`

Responsibilities:

- render the upper waveform region;
- render the main 3-column deck/mixer/deck region;
- own only layout composition, not audio logic;
- receive renderable deck/mixer children or narrow props.

Suggested structure:

```tsx
<DJPerformanceWorkspace
  waveform={<DJSplitWaveform ... />}
  deckA={<DJDeckPanel deck="A" ... />}
  mixer={<DJMixerPanel ... />}
  deckB={<DJDeckPanel deck="B" ... />}
/>
```

This moves the large visual tree out of `DJModeV2.tsx` and leaves that file as page orchestration and callback wiring.

### 4.2 Extract one mirrored deck panel

Create:

`components/dj/v2/DJDeckPanel.tsx`

Instead of maintaining two large near-duplicate JSX sections in `DJModeV2.tsx`, render the same component for A and B.

Responsibilities:

- deck identity strip;
- compact track metadata header;
- deck performance toolbar;
- jog/tempo/EQ area;
- hot cues;
- transport/footer;
- drop target behavior.

Deck-specific alignment should be controlled by a `side`/deck prop, not duplicated markup.

This is important because the current UI drift between A and B is partly a maintenance problem: small fixes can land on one side and not the other.

### 4.3 Extract a dedicated mixer panel

Create:

`components/dj/v2/DJMixerPanel.tsx`

Compose existing:

- `DJChannelStrip`
- `DJMasterKnob`
- `DJStereoVUMeter`
- `DJCrossfaderSelfSub`
- `DJHeadphoneMix`
- `DJSamplerPads`

Keep the existing self-subscribing leaf components. The refactor should primarily reorganize them.

---

## 5. Waveform refactor: 50/50 horizontal split

This should be the first implementation milestone because it changes the visual frame for the entire page.

### 5.1 New public component

Create:

`components/dj/v2/DJSplitWaveform.tsx`

This should expose the same high-level behavior currently supplied by `DJDualWaveform` / `DJWebGLWaveform`, but compose two deck lanes side by side.

Target DOM:

```text
DJSplitWaveform
├── waveform toolbar
└── grid grid-cols-2
    ├── Deck A waveform lane
    │   ├── A label / zoom status
    │   ├── A overview
    │   └── A scrolling waveform
    └── Deck B waveform lane
        ├── B label / zoom status
        ├── B overview
        └── B scrolling waveform
```

There should be one clear vertical divider at the 50% boundary.

### 5.2 Canvas fallback

Refactor `DJDualWaveform.tsx` into reusable deck-oriented primitives rather than continuing the stacked layout.

Recommended split:

- `DJCanvasWaveformDeck.tsx` — one deck's overview + scrolling waveform.
- `DJSplitWaveform.tsx` — places A and B in a 2-column grid and provides shared toolbar state.

Move reusable drawing helpers into:

`components/dj/v2/waveform/canvasWaveformRenderer.ts`

The current drawing code for main waveform, overview, cues, loop shading, beat grid, playhead, and colors should be retained.

### 5.3 WebGL path

Refactor `DJWebGLWaveform.tsx` similarly.

Recommended:

- `DJWebGLWaveformDeck.tsx` owns one deck renderer/canvas.
- `DJSplitWebGLWaveform.tsx` owns shared zoom/color mode and renders two equal columns.
- Preserve the existing fallback chain.

The current WebGL code already has separate renderer instances for Deck A and Deck B. The structural change is therefore mostly DOM layout and overview ownership, not renderer replacement.

### 5.4 Overview behavior

Each deck should get its own overview strip. Do not keep one full-width shared overview.

This simplifies interaction:

- clicks in Deck A overview always seek Deck A;
- clicks in Deck B overview always seek Deck B;
- no midpoint deck detection is needed;
- deck identity is visually obvious.

### 5.5 Waveform sizing

At the authored 1856px canvas width:

- waveform region width: 1856px;
- Deck A lane: 928px;
- Deck B lane: 928px;
- 1px center divider can be accounted for via CSS grid gap/border.

Recommended authored height for performance mode: ~170–190px.

Each lane should contain:

- 24px overview;
- 4px spacing/divider;
- remaining height for the main waveform.

Avoid two stacked main waveforms inside one lane.

---

## 6. Deck header redesign

The deck header should become predictable and fixed-height.

### 6.1 Row 1: track identity

Height target: 56–64px.

Left-to-right for Deck A:

- deck badge;
- optional artwork;
- title and artist;
- key / Camelot;
- BPM;
- remaining time.

Deck B should mirror the same information without reversing arbitrary DOM order that complicates overflow.

Use a consistent grid rather than a free-growing flex row.

Suggested grid:

```css
grid-template-columns:
  auto       /* deck badge */
  auto       /* artwork */
  minmax(0, 1fr) /* title/artist */
  auto       /* key */
  auto       /* bpm */
  auto;      /* time */
```

### 6.2 Row 2: performance controls

Height target: 40–48px.

Keep only controls that are needed continuously:

- loop;
- beat jump;
- stems;
- key lock / slip / auto gain state.

Move editing/analysis tools out of the permanent header.

### 6.3 Move editor-heavy tools into a popover / inspector

The following should not permanently consume deck height:

- `DJEnergyInsights`
- `DJKeyVerificationKeyboard`
- `DJBpmEditor`
- `DJAnalysisCueEditor`
- `DJBeatGridEdit`

Create:

`DJDeckInspector.tsx`

Open it from a compact “Analysis/Edit” button in the deck header.

Possible implementation:

- anchored popover inside each deck;
- tabs: Analysis | Key | BPM | Cues | Grid;
- max-height with internal scroll;
- z-index above decks but below global modal dialogs.

This is the most important fix for vertical density after the waveform change.

---

## 7. FX layout

The generated mock-up works visually because effects are compact and deck-local.

The existing `DJFXSection` can remain, but it should be changed from a page-wide block to two deck FX racks when in the default performance view.

Recommended new component:

`DJDeckFXRack.tsx`

Render one instance for A and one for B directly beneath each deck header.

Each rack should show the core effects in a compact horizontal row:

- Filter
- Delay
- Reverb
- Flanger

Secondary/Beat FX can live behind an expand control.

This avoids the present situation where `viewMode === 'racks'` consumes the entire upper page region and forces the waveform out of view.

### Proposed view semantics

Simplify the view model:

- **Performance**: split waveforms + compact per-deck FX + full decks.
- **Browse**: same performance geometry with library overlay opened.
- **FX**: optionally expand the compact FX area, but do not replace the waveform.
- **Scope**: optional alternate diagnostic view.

The waveform should remain visible in the normal DJ workflow.

---

## 8. Main deck body

The jog section should use an explicit 3-column grid rather than nested flexible width guesses.

Deck A:

```text
tempo | EQ | jog
```

Deck B can either mirror physically:

```text
jog | EQ | tempo
```

or retain the same logical order for code simplicity. The preferred professional-console visual is mirrored physically.

Suggested CSS:

```css
.dj-deck-performance {
  display: grid;
  grid-template-columns: 64px 72px minmax(0, 1fr);
  min-height: 0;
}
```

For Deck B:

```css
.dj-deck-performance[data-deck="B"] {
  grid-template-columns: minmax(0, 1fr) 72px 64px;
}
```

Do not allow the jog wheel component to define the row height. The containing region defines height; jog fills the available square.

---

## 9. Mixer redesign

The mixer should remain fixed-width but become visually less dominant.

Current authored width: 422px.

Target: 300–340px at the 1856px design canvas.

Rationale:

- With split waveforms and symmetric decks, more horizontal space should go to the decks.
- The current mixer width amplifies the cramped feeling.

Suggested authored grid:

```text
Deck A  | Mixer | Deck B
758px   | 340px | 758px
```

for a 1856px canvas.

The mixer can keep:

- channel A/B faders and meters;
- master meter/volume;
- sync and quantize;
- crossfader;
- cue mix;
- compact sampler.

Move rarely changed crossfader curve settings to a small popover opened by a settings icon rather than permanently using vertical space.

---

## 10. CSS ownership cleanup

The current UI mixes Tailwind utility sizes with late global overrides in `index.css`.

During this refactor, introduce explicit DJ layout classes and reduce selector-based overrides.

Recommended classes:

- `.dj-workstation`
- `.dj-waveform-split`
- `.dj-waveform-lane`
- `.dj-main-grid`
- `.dj-deck`
- `.dj-deck-header`
- `.dj-deck-toolbar`
- `.dj-deck-performance`
- `.dj-deck-footer`
- `.dj-mixer`

Keep design tokens in `:root`, but put structural dimensions in one section.

Example:

```css
[data-dj-canvas] {
  --dj-canvas-w: 1856px;
  --dj-canvas-h: 1090px;
  --dj-mixer-w: 340px;
  --dj-waveform-h: 184px;
  --dj-track-header-h: 62px;
  --dj-toolbar-h: 46px;
  --dj-footer-h: 132px;
}

.dj-main-grid {
  display: grid;
  grid-template-columns:
    minmax(0, 1fr)
    var(--dj-mixer-w)
    minmax(0, 1fr);
  min-height: 0;
  overflow: hidden;
}
```

Avoid broad selectors such as `.dj-deck-info + div` for critical layout behavior.

---

## 11. Responsive and resolution behavior

Keep the existing fixed authored canvas and proportional scaling approach for now. It is already regression-tested and provides predictable geometry.

Supported target geometries:

- 1470×825 minimum audit size;
- 1920×1080;
- 2560×1440;
- 3840×2160.

The width gate below the supported threshold can remain.

### Acceptance criteria

At every supported geometry:

- no body overflow;
- no internal main-workspace scrollbars;
- no overlap between deck header controls;
- no overlap between transport and hot cues;
- no waveform clipping;
- Deck A waveform consumes exactly the left half of the waveform region;
- Deck B waveform consumes exactly the right half;
- mixer centerline matches waveform 50/50 divider;
- opening the library causes zero deck/mixer displacement;
- popovers remain inside viewport/canvas bounds.

---

## 12. Accessibility

Preserve or improve the current focus-visible behavior.

Requirements:

- 44px primary performance controls where practical;
- at least 32px secondary controls;
- all icon-only controls require `aria-label`;
- `aria-pressed` for toggles;
- keyboard focus cannot be hidden behind library or inspector overlays;
- Escape closes the topmost transient surface first;
- reduced motion remains respected.

The library's existing focus-return behavior should be preserved.

---

## 13. Performance constraints

The refactor must not regress real-time audio/UI performance.

Preserve these patterns:

- leaf-level Zustand subscriptions;
- direct `useStore.getState()` reads inside waveform RAF loops;
- no position subscription in `DJModeV2`;
- WebGL/Canvas fallback;
- virtualized library;
- lazy mounting of library contents;
- existing engine synchronization hook.

### New constraints

- `DJDeckPanel` should subscribe only to low-frequency header state.
- waveform lanes should not re-render on audio position ticks.
- opening a deck inspector must not remount the jog or waveform.
- keep animation loops isolated from React rendering.

---

## 14. Suggested implementation sequence

### Phase 1 — Layout foundation

1. Create `DJPerformanceWorkspace.tsx`.
2. Create `DJDeckPanel.tsx`.
3. Create `DJMixerPanel.tsx`.
4. Move duplicated Deck A/B JSX out of `DJModeV2.tsx`.
5. Convert the main workspace from flex to 3-column CSS grid.
6. Reduce mixer authored width to ~340px.
7. Keep behavior identical.

**Definition of done:** existing UI functions the same but component ownership is clean and A/B share one implementation.

### Phase 2 — Split waveforms

1. Introduce `DJSplitWaveform.tsx`.
2. Create one-deck Canvas waveform primitive.
3. Create one-deck WebGL waveform primitive.
4. Give A and B independent overview strips.
5. Move color/zoom controls to one shared toolbar.
6. Ensure 50/50 horizontal geometry.
7. Update timeline tests.

**Definition of done:** Deck A waveform is only in the left 50%; Deck B is only in the right 50%, in both WebGL and Canvas paths.

### Phase 3 — Header density reduction

1. Implement fixed-height track header.
2. Implement compact performance toolbar.
3. Create `DJDeckInspector.tsx`.
4. Move energy/key/BPM/cue/grid editing into inspector.
5. Remove variable-height analysis widgets from deck flow.

**Definition of done:** loaded and unloaded deck headers have identical structural height.

### Phase 4 — FX integration

1. Create `DJDeckFXRack.tsx`.
2. Place a compact FX rack in each deck.
3. Keep waveform visible in performance mode.
4. Rework the current racks/FX view into an expanded state instead of a mutually exclusive waveform replacement.

**Definition of done:** normal DJ performance view contains waveform + FX + deck controls simultaneously without overflow.

### Phase 5 — Visual polish

1. Normalize typography.
2. Normalize button heights and radii.
3. Simplify color hierarchy.
4. Align jog, hot cues, transport, and deck identity colors.
5. Remove obsolete global CSS overrides.
6. Tighten mixer layout and crossfader settings.

### Phase 6 — Regression hardening

1. Update `scripts/dj-overlay-audit.mjs`.
2. Update `scripts/djv2-audit.mjs`.
3. Add waveform 50/50 assertions.
4. Add A/B symmetry assertions.
5. Add overlap detection for critical controls.
6. Run WebGL and Canvas fallback audits.
7. Capture visual baselines at all supported resolutions.

---

## 15. Testing changes

### 15.1 Add explicit waveform geometry assertions

In the Playwright audit, collect:

- full waveform region rect;
- Deck A waveform lane rect;
- Deck B waveform lane rect.

Assert:

```text
A.x == waveform.x
A.width ≈ waveform.width / 2
B.x ≈ waveform.x + waveform.width / 2
B.width ≈ waveform.width / 2
A.height == B.height
```

Use a tolerance of 1 CSS pixel after scale normalization.

### 15.2 Add structural data attributes

Recommended:

- `data-dj-waveform-split`
- `data-dj-waveform-deck="A"`
- `data-dj-waveform-deck="B"`
- `data-dj-deck="A"`
- `data-dj-deck="B"`
- `data-dj-mixer`
- `data-dj-deck-inspector`

These will make regression tests resilient to styling changes.

### 15.3 Overlap detection

For critical UI elements, compute DOMRects and assert they do not intersect unexpectedly.

Targets:

- deck title vs BPM/time;
- deck toolbar vs editor button;
- hot cues vs transport;
- mixer sync row vs crossfader;
- waveform toolbar vs deck lanes.

---

## 16. Files expected to change

Primary:

- `pages/DJModeV2.tsx`
- `index.css`
- `components/dj/v2/DJDualWaveform.tsx`
- `components/dj/v2/webgl/DJWebGLWaveform.tsx`
- `components/dj/v2/DJFXSection.tsx`
- `components/dj/v2/DJTopBar.tsx`
- `scripts/dj-overlay-audit.mjs`
- `scripts/djv2-audit.mjs`

New files likely:

- `components/dj/v2/DJPerformanceWorkspace.tsx`
- `components/dj/v2/DJDeckPanel.tsx`
- `components/dj/v2/DJMixerPanel.tsx`
- `components/dj/v2/DJSplitWaveform.tsx`
- `components/dj/v2/DJDeckInspector.tsx`
- `components/dj/v2/DJDeckFXRack.tsx`
- `components/dj/v2/waveform/DJCanvasWaveformDeck.tsx`
- `components/dj/v2/webgl/DJWebGLWaveformDeck.tsx`

Potentially reusable drawing utilities:

- `components/dj/v2/waveform/canvasWaveformRenderer.ts`

---

## 17. Non-goals

This refactor should not initially change:

- audio graph design;
- beat-sync algorithms;
- BPM/key detection algorithms;
- stem generation or stem file format;
- persistent database schema;
- MIDI mapping model;
- sampler audio engine;
- library data model;
- routing/output device logic.

Those can be addressed separately without coupling them to the UI restructuring.

---

## 18. Risks and mitigations

### Risk: waveform split duplicates render work

Mitigation: the code already renders two separate deck waveform canvases. The refactor changes their placement, not the number of main renderers.

### Risk: deck extraction causes excessive Zustand subscriptions

Mitigation: follow the existing self-subscribing leaf pattern. Keep position out of parent components.

### Risk: inspector popovers cover performance controls

Mitigation: anchor inspector to the header, constrain height, and close on Escape/outside interaction. Add Playwright bounds tests.

### Risk: fixed-canvas scaling hides layout flaws

Mitigation: continue normalized-geometry assertions at multiple resolutions and add explicit overlap tests.

### Risk: WebGL and Canvas implementations diverge visually

Mitigation: make `DJSplitWaveform` own shared layout/toolbar semantics and keep render-specific code below it.

---

## 19. Recommended first PR

**Title:** `refactor(dj): establish symmetric workspace and split A/B waveforms`

Scope only:

- extract workspace/deck/mixer shells;
- convert main deck area to 3-column grid;
- implement horizontal 50/50 waveform split for Canvas and WebGL;
- preserve all existing controls and behavior;
- update Playwright geometry assertions;
- no analysis-editor relocation yet;
- no large visual redesign beyond the structural split.

This provides a stable foundation for subsequent UI-density and polish PRs without combining too many behavioral changes in one review.

---

## 20. Final design principle

Every permanent element on the performance surface should answer one of two questions:

1. **Do I need this while actively mixing?**
2. **Does it need to be visible for both decks at the same time?**

If the answer to the first is no, move it into an inspector/popover. If the answer to the second is yes, give Deck A and Deck B the same geometry.

That rule will eliminate most of the overlap and density problems visible in the current DJv2 interface.
