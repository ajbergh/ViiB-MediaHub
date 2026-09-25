# DJv2 UI Refactor Engineering Plan

**Repository:** `ajbergh/ViiB-MediaHub`  
**Target:** `main` branch DJv2 UI  
**Primary entry point:** `pages/DJModeV2.tsx`  
**Status:** Proposed refactor plan  
**Scope:** UI composition, layout, responsive behavior, visual hierarchy, waveform presentation, and regression coverage. Audio-engine behavior should remain functionally unchanged unless explicitly called out.

## Design reference

The implementation target for this refactor is the approved DJv2 UI mock-up committed at:

`docs/assets/djv2-ui-target.png`

![DJv2 target UI mock-up](assets/djv2-ui-target.png)

**Reference image:** 1720×914 desktop composition. The image is a visual target for hierarchy, density, palette, component styling, and deck/mixer balance; it is not a literal pixel-coordinate specification. Structural acceptance criteria in this document take precedence when adapting the design to supported resolutions.

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

## 10A. Visual Design Specification

This section converts the approved mock-up into implementation-level visual rules. The goal is not to copy every rendered pixel; the goal is to reproduce the same visual hierarchy, color language, density, and professional DJ-console feel using reusable tokens and stateful components.

### 10A.1 Overall visual language

The target is a **dark cinematic / premium DJ workstation** rather than a flat admin dashboard.

Key characteristics visible in the mock-up:

- near-black application canvas;
- subtly lighter deck, mixer, waveform, and library surfaces;
- thin cool-gray borders instead of heavy card outlines;
- restrained radii, generally 4–7px;
- shallow inset/highlight treatment on buttons and panels;
- high-contrast white primary text;
- muted blue-gray secondary text;
- strong Deck A blue and Deck B violet identity colors;
- saturated state colors reserved for actions and live status;
- dense but ordered control spacing;
- minimal decorative gradients except where they communicate deck identity, waveform energy, or active state.

Avoid:
- large soft cards;
- oversized rounded corners;
- glassmorphism blur;
- large drop shadows;
- pastel surfaces;
- multiple unrelated accent colors competing inside the same control group.

### 10A.2 Core color tokens

The following tokens should be introduced or mapped onto the existing DJ token system. Values are derived from the approved mock-up and may be tuned slightly for contrast after implementation screenshots are compared against the reference.

```css
[data-dj-canvas] {
  /* Canvas and surfaces */
  --dj-bg: #090b0e;
  --dj-bg-elevated: #0e1116;
  --dj-surface-1: #10141a;
  --dj-surface-2: #161a22;
  --dj-surface-3: #22272e;
  --dj-border: #2b313a;
  --dj-border-subtle: #1c222b;

  /* Typography */
  --dj-text: #f3f6fb;
  --dj-text-secondary: #aab4c2;
  --dj-text-muted: #6f7b89;
  --dj-text-disabled: #4f5966;

  /* Deck identity */
  --dj-deck-a: #0868f8;
  --dj-deck-a-bright: #2088f8;
  --dj-deck-a-dim: #084098;

  --dj-deck-b: #8030f8;
  --dj-deck-b-bright: #8838f8;
  --dj-deck-b-dim: #53316b;

  /* Semantic/live states */
  --dj-play: #00c868;
  --dj-play-hover: #00d873;
  --dj-hotcue: #f85800;
  --dj-hotcue-hover: #ff6a12;
  --dj-key-active: #00b978;
  --dj-warning: #f0a000;
  --dj-danger: #ff3b45;

  /* Navigation / global accent */
  --dj-primary: #7b2cf5;
  --dj-primary-hover: #8b3cff;
}
```

Deck identity colors are not generic decoration. They should consistently identify which side owns a control, waveform, meter, jog ring, selected tab, or active indicator.

### 10A.3 Surface hierarchy

Use four visually distinct elevation levels:

1. **Application canvas** — `--dj-bg`
2. **Primary panels** — decks, mixer, library, waveform shell using `--dj-bg-elevated`
3. **Nested control groups** — FX modules, track header control rows, sampler using `--dj-surface-1` / `--dj-surface-2`
4. **Interactive controls** — buttons, select fields, segmented controls using `--dj-surface-3`

Panel separation should come primarily from border and luminance contrast, not shadow.

Recommended panel treatment:

```css
.dj-panel {
  background: var(--dj-bg-elevated);
  border: 1px solid var(--dj-border-subtle);
  border-radius: 6px;
}

.dj-control-group {
  background: var(--dj-surface-1);
  border: 1px solid var(--dj-border);
  border-radius: 5px;
}
```

### 10A.4 Top navigation

The mock-up establishes a compact global navigation strip.

Requirements:

- approximately 46–50px authored height;
- app mark at far left;
- mode buttons grouped immediately after the mark;
- utility controls aligned right;
- active mode uses the global violet accent rather than Deck A/B color;
- inactive buttons are dark with subtle borders;
- MIDI connected state may use a small green status dot;
- no full-height bright border around the entire header.

Mode button states:

- inactive: dark neutral surface;
- hover: one luminance step lighter;
- active: violet fill or violet-accented gradient with white text;
- focus-visible: 2px high-contrast focus ring that does not alter geometry.

### 10A.5 Waveforms

The waveform region is the strongest source of color in the upper workspace.

**Geometry**

- Deck A owns exactly the left 50%.
- Deck B owns exactly the right 50%.
- Each deck has one main scrolling waveform and one compact overview below it.
- A thin neutral divider marks the center boundary.
- Controls such as GRID / BEAT / PHRASE / zoom live within the corresponding deck lane, not across both decks.

**Deck A waveform palette**

Use a cool spectral progression dominated by:
- blue;
- cyan;
- teal;
- green;
- occasional yellow cue/energy highlights.

**Deck B waveform palette**

Use a warm/violet spectral progression dominated by:
- violet;
- magenta;
- pink;
- orange highlights.

The waveform should remain legible against near-black without using opaque rectangular fills behind every sample.

Playhead:
- bright red or high-contrast red-orange;
- 2–3px authored width;
- extends through the primary waveform lane;
- cue/beat markers remain visually secondary.

Overview:
- lower contrast than the primary waveform;
- selection/viewport region gets a deck-colored translucent outline/fill;
- overview height approximately 20–24px.

### 10A.6 Deck frames and identity

Each deck uses a narrow identity accent rather than flooding the whole panel with color.

Deck A:
- blue left edge/accent line;
- blue deck badge;
- blue jog ring;
- blue tempo/fader highlights;
- blue selected/active deck states.

Deck B:
- violet right edge/accent line;
- violet deck badge;
- violet jog ring;
- violet tempo/fader highlights;
- violet selected/active deck states.

Recommended deck treatment:

```css
.dj-deck[data-deck="A"] {
  border-left: 3px solid var(--dj-deck-a);
}

.dj-deck[data-deck="B"] {
  border-right: 3px solid var(--dj-deck-b);
}
```

Do not put a bright blue/purple border around every sub-panel. Accent color should communicate ownership and active state, not become background chrome.

### 10A.7 Track identity header

The mock-up uses a dense two-line metadata hierarchy:

Primary:
- title, semibold/bold, white;
- time remaining, monospaced or tabular numerals, white.

Secondary:
- artist, muted;
- BPM, Camelot/key, musical key, muted-to-medium contrast;
- elapsed/total time smaller than remaining time.

Artwork:
- approximately 52–58px square;
- 4px radius;
- no heavy shadow;
- preserve cover aspect ratio.

Deck badge:
- approximately 50–58px square;
- strong deck color;
- white A/B label;
- should remain visually aligned with artwork height.

Overflow actions should use a compact ellipsis button rather than adding permanent labels.

### 10A.8 Buttons and segmented controls

Buttons should feel like hardware-console controls translated to a desktop UI.

Base:
- height 30–34px for compact secondary controls;
- 38–44px for primary transport controls;
- radius 4–6px;
- 1px border;
- dark neutral fill;
- small vertical highlight or inset edge is acceptable;
- no pill-shaped controls except where semantically useful.

States:
- hover: surface lightens;
- pressed/selected: accent fill or accent border;
- disabled: reduced text contrast, no glow;
- focus-visible: explicit ring;
- toggles use `aria-pressed`.

Deck-colored selection:
- blue for A-owned controls;
- violet for B-owned controls.

Global selection:
- violet primary accent.

Semantic actions:
- Play = green;
- Hot Cue pads = orange;
- Key/analysis confirmed state = green;
- Record/critical destructive action = red.

### 10A.9 Transport and hot cues

The bottom of each deck should visually resemble the mock-up:

- eight hot-cue pads in one horizontal row;
- cue pads use saturated orange with white numerals;
- cue-pad spacing approximately 4–6px;
- transport row immediately below;
- Play is the strongest control and uses green;
- Cue / previous / Sync remain dark neutral unless active;
- settings and overflow controls remain visually secondary.

Primary transport target height: 42–46px.

Avoid using deck blue/violet as the Play color. The green Play state is intentionally cross-deck and semantic.

### 10A.10 Jog wheels

The jog wheels should become a visual anchor, but not exceed the deck's available height.

Target styling:

- dark platter body;
- concentric neutral rings;
- Deck A blue illuminated outer/progress ring;
- Deck B violet illuminated outer/progress ring;
- large centered BPM;
- smaller pitch percentage / beat-length line;
- elapsed/current time as a third hierarchy level;
- small deck-color position marker on the ring;
- subtle inner shadow for depth;
- no photorealistic metal texture.

The jog renderer/component should take deck color from tokens, not hard-code separate bespoke styling.

### 10A.11 Tempo, EQ, gain and meters

Faders:
- narrow dark track;
- light gray/silver handle;
- active fill uses deck identity color where appropriate;
- scale labels are subdued.

EQ/gain knobs:
- dark rotary body;
- thin silver/gray outer ring;
- white indicator line;
- deck accent may be used only for active/focused state.

Meters:
- preserve conventional green → yellow → orange/red level progression;
- background meter slots should remain near-black;
- labels use tabular numerals where practical.

### 10A.12 FX modules

Each deck-local FX row should match the mock-up's compact modular hardware aesthetic.

Core modules:
- Filter;
- Delay;
- Reverb;
- Flanger.

Each module:
- shared dark surface;
- small uppercase label;
- 2–3 rotary knobs;
- parameter labels beneath knobs;
- 1px separation between modules;
- selected/engaged effect may use a deck-colored top border or label accent.

Do not render every FX module as an independent floating card with large margins.

### 10A.13 Mixer styling

The mixer is a neutral center anchor between two colored decks.

Rules:

- primarily neutral dark surfaces;
- A channel labeling/highlights in blue;
- B channel labeling/highlights in violet;
- master controls remain neutral;
- meters supply their own semantic color;
- crossfader track visually transitions from A blue to B violet;
- cue-mix row uses A/MIX/B labeling and headphone icons;
- selected center mode/tab is clear but not brighter than Play;
- sampler pads are compact and visually subordinate to deck transport.

The mixer should never visually compete with the two jog wheels for dominance.

### 10A.14 Library

The mock-up's library is a dense professional media table, not a consumer card browser.

Target:

- very dark table background;
- left navigation rail;
- compact search bar;
- 30–34px row height;
- small artwork thumbnails;
- sortable column headers;
- subtle alternating/hover row state;
- selected row uses restrained deck/global accent treatment;
- BPM, key, Camelot, time aligned for scanning;
- stems represented by small colored availability indicators;
- library surface must overlay/open without moving deck geometry.

### 10A.15 Typography

Use the application's existing sans-serif stack unless a bundled UI font already exists. Do not introduce a new external runtime font dependency solely for this refactor.

Recommended hierarchy at authored size:

- top navigation: 12–13px / 600;
- track title: 18–20px / 700;
- artist: 12–13px / 500;
- remaining time: 18–20px / 700, tabular numerals;
- deck metadata: 12–13px / 500;
- control labels: 10–12px / 600;
- FX parameter labels: 9–10px / 600;
- mixer labels: 10–12px / 600;
- library rows: 11–12px / 400–500.

Use:
```css
font-variant-numeric: tabular-nums;
```
for BPM, time, meter scales, tempo values, and other rapidly changing numeric readouts.

### 10A.16 Spacing and density

Use a 4px base spacing unit for DJv2.

Recommended authored spacing:
- 4px: internal micro-gap;
- 6–8px: control-to-control;
- 8px: panel padding in dense regions;
- 10–12px: track identity padding;
- 12px: major component group separation.

Do not use generic application spacing such as 20–24px inside performance controls unless required for touch/accessibility.

### 10A.17 Borders, shadows and glow

Borders:
- 1px neutral borders for most components;
- 2–3px deck accent only on major identity edges/active controls.

Shadows:
- shallow inset or 1–2px depth cues only;
- avoid large blurred floating-card shadows.

Glow:
- reserve for small active indicators, jog rings, and focus/selected state;
- avoid persistent neon glow around whole panels.

### 10A.18 Interaction animation

Keep motion short and functional:

- hover/press transitions: 80–120ms;
- drawer/popover: 120–180ms;
- no spring/bounce animation on primary DJ controls;
- meters, playheads and waveform animation follow real-time engine state;
- respect `prefers-reduced-motion`.

### 10A.19 Visual acceptance criteria

A visual-regression pass should compare implementation screenshots to `docs/assets/djv2-ui-target.png`.

The implementation should be considered visually converged when:

- canvas remains near-black, with clearly tiered dark surfaces;
- Deck A reads blue and Deck B reads violet without oversaturating entire panels;
- the top waveform region is visually split at exactly the mixer centerline;
- the waveform is the dominant color field above the decks;
- Play controls are green and hot cues orange on both decks;
- track headers, FX racks, jogs, mixer and library have the same relative hierarchy as the reference;
- controls look compact and hardware-inspired rather than generic web cards;
- mixer remains neutral and narrower than either deck;
- both decks are visually symmetric;
- the library remains dense and table-oriented;
- no legacy styling override introduces incompatible radii, spacing, background colors, or typography.

### 10A.20 Implementation guidance

Prefer implementing these rules through shared variables and primitives rather than one-off utility-class overrides.

Recommended additions:

- `DJButton` variants: neutral, global-primary, deck, play, hotcue, danger;
- `DJPanel` / shared panel utility;
- `DJKnob` token-driven deck accent;
- `DJFader` token-driven accent;
- `DJDeckThemeProvider` or simple CSS data-attribute inheritance if needed;
- waveform palette constants shared by Canvas and WebGL renderers.

The visual token layer should live close to the existing DJ tokens in `index.css`. Renderer-specific waveform colors should reference a shared TypeScript palette module when CSS variables cannot be consumed efficiently from the render loop.

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

Implement against the committed reference image and Section 10A, not against ad-hoc component defaults.

1. Introduce/migrate the shared DJ color tokens.
2. Normalize typography and tabular numeric readouts.
3. Normalize button heights, borders, radii, hover, active and focus states.
4. Apply consistent Deck A blue / Deck B violet ownership styling.
5. Apply green Play and orange Hot Cue semantic styling.
6. Align waveform palettes, jog rings, tempo/fader accents, and mixer channel identity.
7. Normalize dark-surface elevation across decks, mixer, FX and library.
8. Remove obsolete/conflicting global CSS overrides.
9. Tighten mixer layout and crossfader settings.
10. Capture screenshots against `docs/assets/djv2-ui-target.png` at supported resolutions.

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
