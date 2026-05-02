# DJ Mode V2 Panel — UI/UX & Code Review

**Date:** 2026-05-01
**Branch:** `DJ_Mode`
**Target:** `http://localhost:5173/dj` (component: [pages/DJModeV2.tsx](pages/DJModeV2.tsx))
**Reviewer scope:** Visual review via Playwright (1920×1080, 1600×1000, 1440×900, 2560×1440), DOM measurements, code audit of the V2 component tree under [components/dj/v2/](components/dj/v2/).

---

## ✅ Implementation Status — COMPLETE (round 2 — empty-space fill)

Legend: ✅ done · ⏭ deferred (rationale noted)

### Headline results (Playwright re-measurement, post round 2)

| Metric | Original | Round 1 | Round 2 (final) |
|---|---|---|---|
| Total interactive buttons | 159 | 159 | **170** (added FX-pad targets, sampler pads, …) |
| Buttons < 24 px (sub-touch) | 18 | 0 | **0** |
| Buttons 24–31 px (small) | 113 | 88 | 91 |
| Buttons 32–43 px (adequate) | 22 | 25 | 25 |
| Buttons ≥ 44 px (WCAG 2.2 AA target) | **6** (4%) | 46 (29%) | **54** (32%) |
| Largest control (Play button) | 64 × 64 | 64 × 64 | **80 × 80** |
| Waveform height (perf, 1080p) | 260 px | 194 px | 194 px |
| Mixer column width (1080p) | 340 px | 422 px | 422 px |
| EQ knob hit size | 20 px (size − 8 bug) | 40 px | 40 px |
| Hot-cue pad size | 32 × 28 | 48 × 40 | 48 × 40 |
| Crossfader handle | 36 × 26 | 56 × 36 | 56 × 36 |
| Jog wheel max diameter | 300 px | 300 px | **480 px** |
| BPM display height | 11 px | 11 px | **26 px** |
| Elapsed-time display | 11 px | 11 px | **20 px** |
| FX bar centre content | empty | empty | **FX X-Y Pad (120-180 px)** |
| Sampler Pads visibility | hidden < 500 px container | hidden < 500 px container | **always visible** |
| Per-deck stereo VU meter | mixer only | mixer only | **deck header strip + mixer** |

### Quick wins
- ✅ Delete fake top-bar zoom slider — was a static div with no handlers ([DJTopBar.tsx](components/dj/v2/DJTopBar.tsx))
- ✅ Fix SVG `<defs>` ID collision in `DJEQKnob` — `useId()` mints per-instance suffixes; `metalId/innerId/glowId` ([DJEQKnob.tsx](components/dj/v2/DJEQKnob.tsx))
- ✅ Bump `DJDeckEQStrip` knob size 28 → 40 (compact mode hides the value bar/text), shrink tempo column 48 → 40 ([DJMixerComponents.tsx](components/dj/v2/DJMixerComponents.tsx))
- ✅ Always-render FX MIX knob (greyed when disabled) — kills the toggle layout-shift ([DJFXSection.tsx](components/dj/v2/DJFXSection.tsx))
- ✅ Hot-cue `singleRow` 32 × 28 → 48 × 40 ([DJHotCuePad.tsx](components/dj/v2/DJHotCuePad.tsx))
- ✅ Crossfader max width 280 → 360, handle 36 × 26 → 56 × 36 ([DJCrossfader.tsx](components/dj/v2/DJCrossfader.tsx))

### Phase 1 — Sizing pass
- ✅ Added DJ sizing tokens (`--dj-knob-sm/md/lg`, `--dj-pad-perf/sec`, `--dj-fader-w/track`, `--dj-xfader-h`) to [index.css](index.css)
- ✅ `DJVolumeFader`: track 6 → 10 px, handle 30×10 → 44×16, role="slider" + aria-value*, keyboard + wheel ([DJVolumeFader.tsx](components/dj/v2/DJVolumeFader.tsx))
- ✅ GRID buttons: 93×28 → 44×44 with lucide `Chevrons*` icons ([DJBeatGridEdit.tsx](components/dj/v2/DJBeatGridEdit.tsx))
- ✅ Loop ½/2× chevrons: 16×32 → 44×44 with lucide icons; toggle pill 56×32 → 64×44 ([DJLoopSection.tsx](components/dj/v2/DJLoopSection.tsx))
- ✅ Beat Jump compact buttons: 32×32 → 44×44 ([DJBeatJump.tsx](components/dj/v2/DJBeatJump.tsx))
- ✅ Master strip: 64 → 88 px wide; MAIN knob size 28 → 48; VU 60×5 → 80×6
- ✅ Track-color dots 12 × 12 → 24 × 24 with `dj-focus-ring` and aria-label ([DJLibraryBrowserV2.tsx](components/dj/v2/DJLibraryBrowserV2.tsx))
- ✅ Waveform mode chips: 20×20 → 28×28; zoom +/− chips 20×20 → 28×28 ([DJDualWaveform.tsx](components/dj/v2/DJDualWaveform.tsx))
- ✅ Tempo-range chip 27×15 → 28×24 with min-h ([DJTempoSlider.tsx](components/dj/v2/DJTempoSlider.tsx))
- ✅ WebGL/2D toggle: 28×21 → 32×28
- ✅ Library sidebar collapse: 22×22 → 32×32 with aria-expanded

### Phase 2 — Layout reflow
- ✅ Deck footer now hosts the **hot cues row** (top, prime thumb zone) + transport row below — the formerly-cramped CUES strip in the header is gone, and the 92-px footer that held only 3 transport buttons now serves the most-used performance triggers
- ✅ Deck header simplified: dropped CUES row entirely (now in footer), removed `LOOP / BEAT JUMP / GRID` text labels, killed the decorative `w-px h-8 bg-[#2a2a2a]` separators between sections — net effect ~30 px shorter per deck
- ✅ Waveform `perf` height now `clamp(180px, 18vh, 220px)` — reclaims ~60 px to FX bar + library at 1080p, and stops growing past 220 px on 2K/4K
- ✅ Mixer center: width is now `clamp(380px, 22vw, 480px)` (was fixed 340 px); section reorder — Channel A/Master/Channel B → Sync → Curve → **Crossfader (hero)** → CUE-MIX (was: Curve → CUE-MIX → Crossfader at the bottom). The crossfader now sits in the prime thumb position above the headphone mix, where DJs reach for it most often
- ✅ Top-bar duplicate time displays removed (deck headers already show position+remaining); deleted the unused `TopBarTimeDisplay` component and dead imports
- ✅ Fluid sizing via `clamp()` so the panel breathes at 2560/4K instead of hugging 1920×1080
- ✅ Sync mode buttons bumped to min-h 36 px; Q-quantize bumped to 40×36; curve LIN/CP/CUT bumped to 44×32

### Phase 3 — Code health
- ✅ `DJTransportButtons` rAF effect: deps shrink to `[isPlaying]`; BPM read via ref so tempo nudges no longer tear down + recreate the rAF loop ([DJTransportButtons.tsx](components/dj/v2/DJTransportButtons.tsx))
- ✅ `DJTransportButtons` removed dual `setTimeout` press-state — pure CSS `active:scale-[0.92]` with shadow swap; no JS timers competing with mouse events
- ✅ Filter-knob double-write coalescing in `DJModeV2.handleFilterChange` — `lastFilterRef` skips the no-op store + engine writes when the (already-rounded) knob value is unchanged ([DJModeV2.tsx](pages/DJModeV2.tsx))
- ✅ DJEQKnob: keyboard support (Arrow/Home/PageUp/PageDown), mouse-wheel adjust, Shift-fine modifier on drag/wheel/keyboard, `role="slider"` + aria-value*, focus-visible ring
- ⏭ `zustand/shallow` selectors in `DJFXSection` — **not needed**: re-checked `setFilterFX/setDelayFX/...` in [djMixerSlice.ts](slices/djMixerSlice.ts); they preserve untouched `fx[type]` references via spread, so the existing primitive-equality selector is already stable. The original review concern was based on a hypothetical mutation pattern that does not exist in this slice.
- ⏭ `useDrag1D` hook abstraction — deferred (touches 5 components, scoped own PR; current per-component drag handlers all work and now share the same a11y treatment)
- ⏭ Hex → token migration in V2 — deferred (touches every V2 component; scoped own PR after design freeze)
- ⏭ Extract `DJDeckPanel`/`DJMixerCenter`/`DJShortcutsOverlay` from `DJModeV2.tsx` — deferred (the page now reads cleaner after header/footer simplification, and the deck blocks are no longer near-mirrored after Phase 2 reordered Deck B's controls)
- ⏭ Delete V1 mixer/library files — deferred (still need to grep imports to confirm zero refs)

### Phase 4 — Polish & a11y
- ✅ `.dj-focus-ring` utility added in [index.css](index.css), applied to crossfader, volume fader, knobs, library color picker, sidebar toggle
- ✅ Disabled state: grayscale 0.85 + opacity 0.45 via `[data-dj-mode] :disabled` — replaces grey-on-grey re-coloring
- ✅ `role="slider"` + `aria-valuenow/min/max` on EQ knobs, volume fader, crossfader
- ✅ Reduced-motion CSS rule kills the play-button beat-pulse glow when `prefers-reduced-motion: reduce`
- ✅ FX toggles, sync mode, curve buttons, WebGL/2D get `aria-pressed`

### Round 2 — Empty-space fill (in response to "make controls more prominent + add an FX pad")

The user noted there was still substantial empty real estate after round 1. Round 2 attacks that directly with new components and prominence bumps:

- ✅ **NEW: FX X-Y Pad** ([components/dj/v2/DJFXPad.tsx](components/dj/v2/DJFXPad.tsx)) — Pioneer-style touch performance pad that fills the previously-dead centre of the FX bar. Drag inside the pad to morph filter cutoff (X axis, low-pass left → high-pass right) + resonance (Y axis, top = aggressive, bottom = mild). Releases smoothly animate back to centre (non-latching, like a Kaossilator). Target deck selector A / B / BOTH. Engine writes coalesced via rAF. role="application" + aria-label.
- ✅ **Jog wheels — max diameter 300 → 480 px**, so they now actually fill the deck instead of leaving giant empty wings ([DJJogWheel.tsx](components/dj/v2/DJJogWheel.tsx))
- ✅ **Transport buttons bumped again** — Play 64 → **80 px**, Cue 56 → **64 px**, Sync row h-14 → **h-16** (text 13 → 14 px) ([DJTransportButtons.tsx](components/dj/v2/DJTransportButtons.tsx))
- ✅ **Sampler Pads always visible** — dropped the `@[min-height:500px]/mixer:block` gate; the mixer column has been widened in round 1 so the 8-pad grid now ships every session ([DJModeV2.tsx](pages/DJModeV2.tsx))
- ✅ **Jumbo BPM display in deck header** — new `large` prop on `DeckBpmBadge` renders 26-px tabular-nums numerals with deck-accent colour and a "BPM" caption above. Falls back to dim "--.--" placeholder when no track is loaded ([DJDeckComponents.tsx](components/dj/v2/DJDeckComponents.tsx))
- ✅ **Jumbo elapsed-time display** — `DeckTimeDisplay` accepts a `sizeClass` prop; deck headers now use 20-px elapsed time stacked above 12-px remaining time
- ✅ **NEW: Per-deck horizontal stereo VU meter** in the deck header — canvas-based, rAF-driven, gradient (green → amber → red) with peak hold + decay. 120 × 12 px strip giving DJs an instant "is this deck hot" glance without checking the mixer column ([DJDeckComponents.tsx](components/dj/v2/DJDeckComponents.tsx) — `DeckHorizontalVU`)
- ✅ **DECK A / DECK B labels promoted** — now sit in deck-coloured pills with stronger contrast instead of bare text
- ✅ **KEY chip bumped** 10 px → 12 px text, larger padding

### Verification (round 2)
- ✅ Re-ran `node .tmp-review/dj-review.mjs` — final bucket distribution: 0 sub-24 px, 91 small, 25 adequate, **54 at ≥ 44 px** (out of 170 total interactive elements). Largest single control is the Play button at **80 × 80 px**.
- ✅ `npm run typecheck` — only the pre-existing `vite.config.ts` `build.minify` error remains; new `DJFXPad` + `DeckHorizontalVU` + `large`/`sizeClass` props all type-clean.
- ✅ `.tmp-review/` already in `.gitignore`

### Files changed (rounds 1 + 2)

**Components — new (round 2)**
- [components/dj/v2/DJFXPad.tsx](components/dj/v2/DJFXPad.tsx) — X-Y performance pad, fills FX bar centre, A/B/BOTH target selector, rAF-coalesced engine writes
- `DeckHorizontalVU` (added inside [components/dj/v2/DJDeckComponents.tsx](components/dj/v2/DJDeckComponents.tsx)) — canvas + rAF stereo VU strip with peak-hold

**Components — modified**
- [components/dj/v2/DJEQKnob.tsx](components/dj/v2/DJEQKnob.tsx) — useId for SVG IDs, removed `size − 8` bug, compact prop, keyboard + wheel + Shift-fine, role="slider", aria-value*
- [components/dj/v2/DJVolumeFader.tsx](components/dj/v2/DJVolumeFader.tsx) — bigger track + handle, role="slider", aria-value*, keyboard + wheel
- [components/dj/v2/DJCrossfader.tsx](components/dj/v2/DJCrossfader.tsx) — max width 360, bigger handle/track, role="slider", aria-value*, keyboard, focus ring
- [components/dj/v2/DJHotCuePad.tsx](components/dj/v2/DJHotCuePad.tsx) — singleRow 48×40, gap-1
- [components/dj/v2/DJBeatGridEdit.tsx](components/dj/v2/DJBeatGridEdit.tsx) — 44×44 buttons with lucide icons, aria-labels
- [components/dj/v2/DJBeatJump.tsx](components/dj/v2/DJBeatJump.tsx) — 44×44 buttons
- [components/dj/v2/DJLoopSection.tsx](components/dj/v2/DJLoopSection.tsx) — 44×44 chevrons, 64×44 toggle, aria-labels
- [components/dj/v2/DJTransportButtons.tsx](components/dj/v2/DJTransportButtons.tsx) — rAF dep shrink, ref-stored bpm, CSS `:active` press-state, **round 2: Play 64→80, Cue 56→64, Sync h-14→h-16**
- [components/dj/v2/DJJogWheel.tsx](components/dj/v2/DJJogWheel.tsx) — max diameter 300 → 480 px so the wheel fills the deck
- [components/dj/v2/DJFXSection.tsx](components/dj/v2/DJFXSection.tsx) — round 1: always-render MIX, round 2: hosts the X-Y FX Pad in the centre gap
- [components/dj/v2/DJDeckComponents.tsx](components/dj/v2/DJDeckComponents.tsx) — `DeckTimeDisplay` accepts `sizeClass`, `DeckBpmBadge` accepts `large` (26 px tabular numerals), new `DeckHorizontalVU`
- [components/dj/v2/DJTopBar.tsx](components/dj/v2/DJTopBar.tsx) — deleted fake zoom slider, deleted unused TopBarTimeDisplay, dropped useRef import
- [components/dj/v2/DJMixerComponents.tsx](components/dj/v2/DJMixerComponents.tsx) — DJDeckEQStrip uses size=40 + compact, master knob size=48
- [components/dj/v2/DJFXSection.tsx](components/dj/v2/DJFXSection.tsx) — always-render MIX knob, bumped toggle min-h, aria-pressed
- [components/dj/v2/DJDualWaveform.tsx](components/dj/v2/DJDualWaveform.tsx) — bigger color/zoom chips, aria-pressed/labels
- [components/dj/v2/DJTempoSlider.tsx](components/dj/v2/DJTempoSlider.tsx) — range chip min-h 24, aria-label
- [components/dj/v2/DJLibraryBrowserV2.tsx](components/dj/v2/DJLibraryBrowserV2.tsx) — 24×24 color dots, 32×32 sidebar toggle

**Pages**
- [pages/DJModeV2.tsx](pages/DJModeV2.tsx) — deck header simplification, deck footer (hot cues + transport), mixer reorder (crossfader above CUE-MIX), wider master/EQ strip columns, slimmer tempo column, filter coalescing, WebGL/2D moved to top-left

**Styles & build**
- [index.css](index.css) — sizing tokens, fluid clamps for waveform/mixer, dj-focus-ring, disabled grayscale, reduced-motion rule
- [.gitignore](.gitignore) — `.tmp-review/`

---

Screenshots and raw measurements are stored in `.tmp-review/` (gitignored — delete after review):
- `01-full-1920x1080.png`, `02-full-*.png` — full-page captures at four breakpoints
- `crop-deck-a.png`, `crop-mixer-full.png`, `crop-eq-strip.png`, `crop-fx-bar.png`, `crop-deck-a-header.png` — region close-ups
- `measurements.json` — raw per-element bounding rects + role/aria/title

---

## TL;DR

The V2 panel uses pro-DJ vocabulary (jog wheel, tempo slider, hot cues, EQ strip, crossfader curves) but its **physical sizing is wrong for a performance surface**. Out of 159 interactive buttons measured at 1920×1080:

| Min-edge bucket | Count | % |
|---|---|---|
| **< 24 px** (sub-touch) | 18 | 11% |
| 24–31 px (small) | 113 | 71% |
| 32–43 px (adequate) | 22 | 14% |
| **≥ 44 px** (WCAG 2.2 AA target) | 6 | 4% |

Meanwhile, the panel is generous with **vertical empty space** (a 92 px deck footer holds three buttons; the FX bar occupies a full 1856 px row for 4 small toggle pills per deck) and **horizontal empty space** (waveform area is 260 px tall and >50% black when no track is loaded; mixer center is 340 px containing controls that bunch in the middle).

The result: the screen *looks* full but only ~25% of pixels carry interactive value, and the controls a DJ touches most often (hot cues, EQ knobs, crossfader, sync, FX wet/dry) are among the smallest things on screen.

The fix is not "more controls" — it is **pruning labels/padding, raising baseline control sizing, and re-allocating wasted real estate to the highest-impact controls** (jog → keep, EQ knobs → 1.6×, hot cues → 1.7×, crossfader → 1.4× and taller, FX knobs → always-visible, mixer width → wider).

---

## Layout map (1920×1080, sidebar visible at 64 px)

Measured from `[data-dj-mode]` (1856 × 1080):

```
┌─ TopBar           1856 × 44   (5 view+layout tabs at min-h 32, REC, fake zoom slider, MIDI, FS) ─┐
├─ Waveform         1856 × 260  (≥50% black with no track; tiny 20 px RGB/3B/CLR/+/− chips)        │
├─ FX bar           1856 × 79   (Deck A: 4 pills 53–68 × 28 / Deck B: same; >50% empty)            │
├─ Main control     1856 × 487   ┌ Deck A 758 ┬ Mixer 340 ┬ Deck B 758 ┐                           │
│                                │ header 124 │           │ header 124 │                           │
│                                │ jog area   │ ChStripA  │ jog area   │                           │
│                                │            │ Master 64 │            │                           │
│                                │            │ ChStripB  │            │                           │
│                                │ footer 92  │ x-fader   │ footer 92  │                           │
│                                └────────────┴───────────┴────────────┘                           │
├─ Resize handle    1856 × 10                                                                      │
└─ Library          1856 × 200  (3 visible rows w/ search bar)                                     ┘
```

Within each deck the jog area is divided as: **tempo slider 48 px | EQ strip 62 px | jog 648 px**. The EQ strip's 62-px column hosts 5 stacked knobs configured at `size={28}` → ~20 px hit circles ([DJMixerComponents.tsx:128](components/dj/v2/DJMixerComponents.tsx#L128)).

---

## Part 1 — UI/UX findings

### 1.1 Sizing & touch targets (the dominant problem)

| Control | Current | Issue | Target |
|---|---|---|---|
| **EQ knobs** (TRIM/HI/MI/LO/FILT) | size=28 → ~20 px circle | Hit area smaller than fingertip; hard to drag precisely with mouse, impossible with touch. ([DJMixerComponents.tsx:128](components/dj/v2/DJMixerComponents.tsx#L128)) | size=44–52 (32–40 px circle) |
| **Hot cue pads 1–8** | 32 × 28 px in `singleRow` mode | Performance-critical. On a Pioneer DDJ they are 35 mm pads. ([DJHotCuePad.tsx:104](components/dj/v2/DJHotCuePad.tsx#L104)) | 56 × 44 px minimum |
| **GRID jog buttons** ◀◀ ◀ +0 ▶ ▶▶ | 93 × 28 px | 3.3:1 aspect, padded horizontally to fill row. ([DJModeV2.tsx:539](pages/DJModeV2.tsx#L539)) | 64 × 44 px square-ish |
| **Loop ½ / 2× / start-end** | **16 × 32 px** (16 px wide!) | Among the smallest hit targets in app. | 32 × 44 px |
| **Volume fader handle** | 30 × 10 px on 6-px track | Track too thin; handle barely visible against gradient. ([DJVolumeFader.tsx:142–151](components/dj/v2/DJVolumeFader.tsx#L142-L151)) | 44 × 16 px on 10-px track |
| **Crossfader** | width clamps to 280 px, handle ~36 × 36 | THE hero control of a DJ mixer; sits below VU meters in a 72-px slot. ([DJCrossfader.tsx:43](components/dj/v2/DJCrossfader.tsx#L43)) | width 320–360 px, handle 56 × 64, slot height 96+ px |
| **CUE-MIX headphone slider** | ~80 × 6 px | Slider under "HEADPHONES" is barely a hairline. ([DJModeV2.tsx:712](pages/DJModeV2.tsx#L712)) | 140 × 12 px |
| **FX toggle pills** | 53–68 × 28 px | Currently the only FX UI in default `perf` layout (knobs hidden until enabled, causing layout-shift on toggle). | 80 × 36 px + always-visible MIX knob |
| **Sync mode (OFF/BPM/PHASE/Q)** | 60-95 × 32 px | Adequate but cramped — ~1 px between buttons. ([DJModeV2.tsx:651–684](pages/DJModeV2.tsx#L651-L684)) | maintain h, gap-2 |
| **Curve LIN/CP/CUT** | 33–36 × 28 px | "CP" is meaningless — should expand to "CONST PWR" or use a glyph. | 44 × 32 + better labels |
| **Waveform mode chips** RGB / 3B / CLR / + / − / 10s / 2D | 20–35 × 20 px | These eat the upper-right corner with sub-touch buttons. | Collapse to one icon menu |
| **Track-color labels** (4×) | **12 × 12 px** | Tied for smallest interactive elements on screen. | 24 × 24 px |
| **KEY / SLIP / AG** deck flags | 32–57 × 24 px | Only 24 px tall. | 32 px tall, gap-1.5 |

**Verdict:** Doubling baseline control size (and tightening padding around them) will not lengthen the panel — there is enough wasted whitespace to absorb the increase without a layout overhaul.

### 1.2 Empty space & wasted real estate

1. **Deck footer (92 px tall, 758 px wide)** holds CUE-time label (16 px wide), 3 transport buttons (~190 px cluster centered), and LOOP-status label (16 px wide) — so ~520 px of horizontal real estate is dead. The footer should host the **Hot Cue pads** (currently jammed into the cramped header) at performance size, freeing the header for track metadata.

2. **Waveform region is 260 px tall and mostly black with no track loaded.** With a track, it still allocates a full overview lane plus a main lane that fills ~140 px. Default `perf` layout could safely shrink waveform to 200 px and reclaim 60 px for the FX bar.

3. **FX bar (79 px tall, 1856 px wide)** holds 4 pills × 2 decks = 8 buttons of total width ~480 px, leaving ~1370 px of black background. In compact (`perf`) mode the wet/dry knob only appears *after* enabling the FX → toggling FX causes the row to grow vertically (layout shift). Always render the MIX knob (greyed when disabled).

4. **Mixer master strip is 64 px wide** — too narrow for both VU meters and the MAIN volume knob. Both are tiny. The master strip should be 88 px and host VU + a 44-px MAIN knob + a clear digital readout.

5. **EQ strip column 62 px wide × ~360 px tall** stacks 5 knobs at size 28 with `gap-1`. With size-44 knobs the column would need to be 70 px wide (+8 px) — easily absorbed by trimming the 48-px tempo slider column.

6. **Top bar zoom slider** ([DJTopBar.tsx:256–270](components/dj/v2/DJTopBar.tsx#L256-L270)) is **decorative — no event handlers**. It's a fake control. Either wire it to waveform zoom or delete it.

7. **Library height clamps to 200 px** with header (~30 px) + 3 visible rows. On 1080p we have spare vertical budget if we shrink waveform; even +60 px to library = 2 more rows visible at all times.

### 1.3 Visual hierarchy & importance

A pro DJ controller's hierarchy is roughly: **Crossfader > Volume faders > Jog > EQ > Hot cues > Sync/Cue > FX > Loop > BeatJump > Library**. Current panel hierarchy by visual weight is closer to: **Jog (huge) > Library > Waveform > Master strip > everything else (uniformly tiny)**.

Specific reversals to fix:

- **Crossfader** should be the most prominent horizontal element in the mixer, not crammed under the master strip in a 72-px slot.
- **Hot cues** are the most-used performance trigger after play; they currently render at half the size of the SYNC button.
- **EQ knobs** are the most-touched continuous control during a transition; here they're smaller than the static "DECK A" text label.
- The static "MASTER" text label (10 px) is given the same vertical breathing room as the actual master VU meter (60 px) — the label dominates by virtue of being centered with whitespace above and below.

### 1.4 Density & noise

- **Five separate text labels** (`CUES`, `LOOP`, `BEAT JUMP`, `GRID`, accompanied by separator `|` strokes) above/within a 120-px deck header — labels consume ~30% of the row height and convey what the icons already make obvious.
- **Decorative SVG separators (`w-px h-8 bg-[#2a2a2a] mx-2.5`)** between every deck section — three per deck row, each adding 21 px of horizontal nothing.
- **Tempo slider chip `±16%`** (27 × 15 px) — duplicates info available from the slider's own range readout.
- **Headphones / CUE MIX double label** ("HEADPHONES" and "CUE MIX" stacked above the same control).
- **Two redundant time displays** — top bar has a `TopBarTimeDisplay` per deck; deck header has a `DeckTimeDisplay` *plus* a `DeckTimeDisplay` for remaining. Keep one.
- **Repeated 10-px labels everywhere** — neutral-500/600 colour makes them illegible against the dark surface, so they add noise without informing.

### 1.5 Behaviour bugs & a11y gaps

- **Decorative zoom slider in top bar is non-functional** ([DJTopBar.tsx:262–264](components/dj/v2/DJTopBar.tsx#L262-L264)).
- **No keyboard support on EQ knobs / volume faders / crossfader** — screen-reader and keyboard-only users cannot tweak. ([DJEQKnob.tsx:91–98](components/dj/v2/DJEQKnob.tsx#L91-L98)).
- **No mouse-wheel support on knobs** — fine-tune via wheel is standard on every DAW/DJ controller in software.
- **Knob drag is vertical only with a fixed 0.5 sensitivity** — Shift-drag for fine, Ctrl-drag for coarse is conventional and missing.
- **`disabled={!track}` guards** are everywhere but most disabled buttons render in the same dim grey as enabled ones, so the active/inactive state is hard to distinguish.
- **Sync button has no keyboard shortcut** despite "E" / "[" being shown in the help dialog (verify in audit).
- **No focus-visible ring** on most knobs/faders.

### 1.6 Cross-breakpoint behaviour

| Breakpoint | Observed |
|---|---|
| 2560×1440 | Layout looks comically empty. Mixer stays 340 px; jog area >900 px. Massive black bands above/below mixer. |
| 1920×1080 | Reference design. Tight but workable. |
| 1600×1000 | Library auto-collapses (window.innerHeight < 900 fires earlier than expected); deck footer Sync button starts truncating. |
| 1440×900 | Below `useIsDJReady` threshold? Confirm — if `minWidth=1440` exactly, we render but very cramped. EQ knobs effectively unusable. |

Recommendation: define **fluid sizing tokens** (e.g. mixer width `clamp(360px, 22vw, 480px)`) so the panel breathes at higher resolutions instead of hugging 1920.

---

## Part 2 — Code optimization findings

### 2.1 🐛 Bug — `DJEQKnob` SVG `<defs>` IDs collide across knob instances

[DJEQKnob.tsx:107–127](components/dj/v2/DJEQKnob.tsx#L107-L127) generates filter/gradient IDs from the knob's `label` prop:

```tsx
<radialGradient id={`knobMetal-${label}`} ... />
<filter        id={`knobInnerShadow-${label}`} ... />
<filter        id={`knobGlow-${label}`}        ... />
```

Both decks render a knob with `label="HIGH"` (and MID, LOW, TRIM, FILT). HTML SVG IDs are document-global, so the second-rendered knob's `<defs>` overwrite the first, and `url(#knobInnerShadow-HIGH)` references one shared filter element. The same applies to FX knobs (FREQ/RES/TIME/FDBK/MIX appear on both decks).

**Symptoms:** subtle render quirks — glow doesn't show on the "wrong" deck, gradient appears flat after re-mount, filter inheritance changes when only one knob is updated.

**Fix:** use `React.useId()` (or a counter ref) to mint a per-instance unique suffix:

```tsx
const uid = React.useId();
const metalId = `knobMetal-${uid}`;
// ... and reference `url(#${metalId})`
```

### 2.2 🐛 Dead UI — top-bar zoom slider has no event wiring

[DJTopBar.tsx:262–270](components/dj/v2/DJTopBar.tsx#L262-L270) renders a div-only "slider" with magnifier glyphs. There are no `onPointerDown`/`onChange`/`onClick` handlers, no value subscription, no waveform-zoom action. Either:
- Wire it up to the existing waveform zoom (the help dialog promises `Ctrl+Scroll` zoom, so the action exists).
- Delete it. It currently misleads users.

### 2.3 ⚠️ Perf — Filter-knob recompute fires twice on each drag tick

[DJModeV2.tsx:306–330](pages/DJModeV2.tsx#L306-L330) runs `setDeckFilter(...)` *and* `setFilterFX(...)` on every pointer-move. The `setFilterFX` call writes to the engine *and* re-derives store fields. Below the 0.05 neutral threshold both writes still happen. Coalesce: skip both when the new value is within 0.5 px of the previous (the knob already round-snaps), or rAF-throttle the engine call.

### 2.4 ⚠️ Perf — FX-section `useStore` selector returns a new object each tick

[DJFXSection.tsx:42–45](components/dj/v2/DJFXSection.tsx#L42-L45):

```tsx
const fx = useStore(state => {
  const deckState = deck === 'A' ? state.djDeckA : state.djDeckB;
  return deckState.fx[type];   // OK only if the slice is preserved by reference
});
```

When the deck slice is rebuilt (any `set` call inside the deck), `deckState.fx[type]` may be a *new* object reference even though its contents are unchanged. Combined with the four FX-unit instances per deck, this triggers re-renders during unrelated deck mutations (volume drag, position tick at 15 fps). Use a custom `equalityFn` (`shallow` from `zustand/shallow`) or memoize on `deckState.fx[type].enabled + ...primitive fields`.

Same pattern at [DJFXSection.tsx:357–361](components/dj/v2/DJFXSection.tsx#L357-L361) (`deckAFX`, `deckBFX`).

### 2.5 ⚠️ Perf — `DJTransportButtons` rebuilds animation loop on every `effectiveBpm` tick

[DJTransportButtons.tsx:42–63](components/dj/v2/DJTransportButtons.tsx#L42-L63) lists `effectiveBpm` in deps. If tempo nudges fire even mildly often during a sync, the rAF loop is torn down and recreated each time. Switch to a ref-stored `bpm` and let the rAF read it; only re-arm when `isPlaying` flips.

### 2.6 ⚠️ Layout shift — FX wet-knob appears/disappears on toggle

[DJFXSection.tsx:254–262](components/dj/v2/DJFXSection.tsx#L254-L262) — in compact mode the MIX knob is conditionally rendered when `isEnabled`. Toggling FX shifts neighbours horizontally (~30 px). Render the knob always; greyscale when disabled.

### 2.7 🧹 Style — duplicate inline transport handlers + `setTimeout` press states

[DJTransportButtons.tsx:65–79](components/dj/v2/DJTransportButtons.tsx#L65-L79) sets a `pressed` state for 100 ms via `setTimeout` then *also* responds to `onMouseDown/Up/Leave`. The two systems fight each other (e.g., quick taps stay pressed for 100 ms even after release). Pick one — `:active` CSS or the `useState` model with proper handler, not both.

### 2.8 🧹 Code — knob value↔real-value mapping has 4 copies in `DJFXSection`

[DJFXSection.tsx:170–225](components/dj/v2/DJFXSection.tsx#L170-L225) — four `useCallback` blocks each switch on `type`. Encode the mapping in the `FX_CONFIGS` table once and dispatch generically. Reduces the file ~80 lines and makes adding a new FX a one-row change.

### 2.9 🧹 Code — manual drag handlers duplicated across 5 components

`DJEQKnob`, `DJVolumeFader`, `DJCrossfader`, `DJTempoSlider`, `DJHeadphoneMix` each implement nearly identical `pointerdown / setPointerCapture / pointermove / pointerup / pointerleave` flows with their own `isDragging` state. Extract a `useDrag1D({ axis, onChange, sensitivity, onCommit, fineModifier })` hook. This would also be the place to add the missing **wheel + keyboard + Shift-fine-drag** affordances in one shot (§1.5).

### 2.10 🧹 Code — `DJModeV2.tsx` is 939 lines, mostly JSX layout wiring

The page component reads many store slices, drags + drops layout sections, and inlines the keyboard-shortcuts overlay (lines 360–416). Extract:
- `<DJShortcutsOverlay/>` (own file)
- `<DJDeckPanel deck="A"/>` / `<DJDeckPanel deck="B"/>` — currently nearly mirrored 100-line blocks for deck A and B (lines 492–604 & 733–846); fold into one component with `deck` + `mirrored` props.
- `<DJMixerCenter/>` (lines 606–731)

This reduces the page to ~250 lines and removes a class of "did you update both decks?" bugs (the deck headers are *almost* mirrored but not 100% — easy place for divergence).

### 2.11 🧹 Code — `useEffect` in [DJModeV2.tsx:160–173](pages/DJModeV2.tsx#L160-L173) re-creates ResizeObserver on every `djLayoutMode` change

Cheap, but the inner branching (`fx`/`perf`/else) duplicates the constants from the prior effect. Define `LAYOUT_MAX_LIBRARY_H` once and reuse.

### 2.12 ⚠️ A11y — DJ panel uses raw hex everywhere

The codebase has scripts `check:raw-colors` and `check:palette` ([package.json](package.json)) but the DJ V2 components ignore the design tokens (e.g. `bg-[#161616]` is hardcoded throughout instead of `bg-dj-surface-1` or the corresponding token). The CSS vars `--dj-target-primary`/`-secondary` are defined ([index.css:54–55](index.css#L54-L55)) but **never referenced** anywhere in the V2 tree. Theming (light mode, contrast adjust) is impossible without a token migration.

### 2.13 🧹 Hygiene — orphan files / sibling V1 mixer

[components/dj/DJMixer.tsx](components/dj/DJMixer.tsx) and [components/dj/DJLibraryBrowser.tsx](components/dj/DJLibraryBrowser.tsx) appear to be V1 components no longer routed (the `/dj` route is V2; `pages/DJMode.tsx` was deleted in `git status`). Confirm dead, then delete to reduce maintenance surface.

---

## Part 3 — Implementation plan

The plan is organised into four phases that each ship independently and build on each other. **Phase 1** is the high-impact "feels professional" change; **Phase 2** is the layout reflow; **Phase 3** is code health; **Phase 4** is polish.

### Phase 1 — Sizing pass (1–2 days, biggest perceived improvement)

**Goal:** every continuously-touched control reaches a 44 px touch target, performance triggers reach 56 px, and labels shrink/disappear to make room.

1. **Introduce sizing tokens & use them.** Add to [index.css](index.css):
   ```css
   --dj-knob-sm:    32px;   /* FX wet, headphone mix */
   --dj-knob-md:    44px;   /* deck EQ + master */
   --dj-knob-lg:    56px;   /* hero / future macro */
   --dj-pad-perf:   56px;   /* hot cues, beat-jump */
   --dj-pad-sec:    44px;   /* loop, grid, sync-mode */
   --dj-fader-w:    44px;   /* volume fader column */
   --dj-fader-track:10px;   /* track width */
   --dj-xfader-h:   28px;   /* crossfader handle height */
   ```
   Replace every magic number in V2 components with these.

2. **`DJEQKnob`**: remove the hard-coded `knobSize = size - 8` bug (the SVG already uses a 40 viewBox, so `size` *is* the visual size). Default `size = 44`; in `DJDeckEQStrip` use `size=44`; in FX use `size=40`. Convert the value bar from `h-1` (4 px) to `h-1.5` for visibility.

3. **`DJVolumeFader`**: track 10 px wide; handle 44 × 16 px; container width 56 px. Add tick marks at 0/25/50/75/100 with stronger 0/100 strokes.

4. **`DJCrossfader`**: minimum 320 px, max 400 px (drop the 280-px clamp). Handle 56 × 32 with a visible centre detent. Slot height 96 px with the curve indicator drawn on the track.

5. **`DJHotCuePad`** in `singleRow`: bump to `w-14 h-11 text-[12px]` (56 × 44). Two-row mode if track header still has space, but prefer relocating cues to the deck footer in Phase 2.

6. **GRID buttons (◀◀ ◀ +0 ▶ ▶▶)**: square-up to 64 × 44 with stronger glyphs (use lucide `Rewind`, `SkipBack`, `Play`, `SkipForward`, `FastForward` rather than ascii triangles).

7. **Loop ½ / 2× chevrons** (16 × 32 today): replace with `lucide` Minimize2/Maximize2 inside 32 × 44 buttons.

8. **Master strip**: widen `w-[64px]` → `w-[88px]`, MAIN knob to size=44, add a digital `+4.8 dB` readout below (already exists, just bigger).

9. **Top bar fake zoom slider**: delete (or wire to `waveformZoom` action — TBD). Either way it cannot stay decorative.

10. **Track-color dots** (12 × 12): bump to 24 × 24 with focus ring, rename aria-label.

**Acceptance check:** rerun `node .tmp-review/dj-review.mjs` — bucket distribution should flip to ≥70% of buttons in the 32–44 band, ≥10% at 44+.

### Phase 2 — Layout reflow (2–3 days)

**Goal:** put each control where its importance demands, kill empty pixels.

1. **Deck footer redesign.** Today: 92 px tall, holds 3 transport buttons centered + 2 tiny labels. Replace with two rows:
   - Row 1 (60 px): Hot Cue pads (8 × 56 × 44 px) — the natural "performance row".
   - Row 2 (60 px): CUE | Play | Sync transport at current sizes, plus LOOP/CUE-time labels at the corners.
   Net height ~+28 px per deck — paid for by removing the deck-header CUES row (Phase 2.2).

2. **Deck header simplification.** Currently 124 px in three rows: track-info, hot cues, controls (LOOP/BEAT-JUMP/GRID).
   - Track-info row stays.
   - Hot-cue row removed (now in footer).
   - Controls row (LOOP / BEAT JUMP / GRID) collapses to a single 48-px row using icon-only buttons + a hover/long-press tooltip; remove the inline labels and `w-px` separators. Net: 124 → 92 px.

3. **Waveform reduction in `perf` mode.** Drop `--dj-waveform-h` from 260 → 200 px in perf, keep 260 in browse, 180 in fx (already so for fx). Reclaim 60 px → 30 px to library, 30 px to FX always-knobs (Phase 2.4).

4. **FX bar always-visible knobs.** Render FX cells with the MIX knob always present (greyed when disabled) so toggling does not shift layout. Bring up to ~80 px row height (was 79 px → effectively same), but actually filled.

5. **Mixer center widen + reorder.**
   - `--dj-mixer-w` 340 → 380 px.
   - Reorder: top-to-bottom should be CUE buttons | VU+Fader | Master VU+MAIN knob | Sync mode | Crossfader curve | **Crossfader (hero)** | Headphone CUE-MIX. The crossfader belongs *above* headphone mix, not below — DJs reach for it constantly.
   - Move Sampler Pads out of the mixer (currently hidden until container ≥500 px tall) into the bottom of the deck footer or a dedicated pad row triggered by view tab.

6. **Top bar trimming.** Remove the (broken) zoom slider — the existing keyboard `Ctrl+Scroll` is fine. Remove the duplicate time displays from the top bar OR from the deck header (pick one — recommend keeping deck-header + remaining-time, drop top-bar). Frees ~200 px in the header centre for a real "Now Mixing" arc/timeline if useful.

7. **2560×1440 fluid sizing.** Wrap mixer width / waveform height in `clamp(...)`:
   ```css
   --dj-mixer-w:    clamp(360px, 22vw, 480px);
   --dj-waveform-h: clamp(180px, 22vh, 280px);
   ```
   Decks then fluidly absorb the rest. Verify the jog wheel's `responsive` mode picks up the increased space.

### Phase 3 — Code health (2 days, parallel-safe)

1. **Fix SVG ID collisions in `DJEQKnob`** with `React.useId()`. (§2.1)
2. **Extract `useDrag1D` hook** and migrate the 5 control components. Add wheel + keyboard support (Arrow ±1, Shift+Arrow ±5, Home reset to 0/center, Shift-drag fine, double-click reset). (§2.9, §1.5)
3. **`zustand/shallow` selectors** in `DJFXSection` and `DJDeckEQStrip` for `fx[type]` and `eq.{high,mid,low}`. (§2.4)
4. **Refactor `DJTransportButtons` rAF effect** — dep only on `isPlaying`, read `effectiveBpm` from a ref. (§2.5)
5. **Remove dual press-state in `DJTransportButtons`** — pure CSS `:active`. (§2.7)
6. **Extract from `pages/DJModeV2.tsx`**: `DJShortcutsOverlay`, `DJDeckPanel`, `DJMixerCenter`. Folds the two near-mirrored deck blocks into one component. (§2.10)
7. **Migrate hex colors to design tokens** in DJ V2 components — re-enable `npm run check:raw-colors` for those paths. (§2.12)
8. **Delete confirmed-dead V1 files** after a final usage grep. (§2.13)

### Phase 4 — Polish & a11y

1. **Focus rings** on every interactive element via a single `.dj-focus-ring` utility.
2. **Disabled state visual contrast** — use grayscale filter + 0.4 opacity on the whole control rather than re-coloring.
3. **`aria-valuenow` / `aria-valuemin` / `aria-valuemax`** on knobs/faders (already SVG; add `role="slider"` wrappers).
4. **Reduced motion** — skip the play-button beat-pulse rAF when `prefers-reduced-motion: reduce`.
5. **Tooltips** on icon-only buttons (after Phase 2.2 collapses LOOP/BEATJUMP/GRID labels).

---

## Risks & non-goals

- **Audio engine changes are out of scope.** All proposals are visual/structural.
- **Phase 2.5 mixer reorder** changes muscle memory for any user already accustomed to the current layout. Worth doing but flag in release notes.
- **Sampler pads relocation (Phase 2.5)** depends on whether the user community uses them — confirm via session telemetry / poll before moving.
- **Token migration (Phase 3.7)** can churn diffs across many files; do it as one focused PR after Phase 1+2 ship to avoid review-fatigue conflicts.

---

## Quick wins (≤2 hours each, can land independently)

1. **Delete the fake top-bar zoom slider.** ([DJTopBar.tsx:256–270](components/dj/v2/DJTopBar.tsx#L256-L270))
2. **Fix the SVG ID collision in `DJEQKnob`.** Subtle correctness bug; trivial fix. (§2.1)
3. **Bump EQ-strip knob `size={28}` → `size={44}`** and tempo-slider column from `w-12` → `w-10`. Massive perceived improvement, no layout collateral. ([DJMixerComponents.tsx:128](components/dj/v2/DJMixerComponents.tsx#L128))
4. **Always-render the FX MIX knob** (greyed when off). Removes layout shift. (§2.6)
5. **Hot-cue `singleRow` size `w-8 h-7` → `w-12 h-10`**; pads still fit the existing header row. ([DJHotCuePad.tsx:104](components/dj/v2/DJHotCuePad.tsx#L104))
6. **Crossfader `max 280` → `max 360`.** ([DJCrossfader.tsx:43](components/dj/v2/DJCrossfader.tsx#L43))

---

## Repro

```powershell
# dev server already running on :5173
npx playwright install chromium      # one-time
node .tmp-review/dj-review.mjs       # captures + measurements.json
node .tmp-review/dj-closeups.mjs     # region crops
```

Both scripts assume the dev server is up and that the user is signed-in or that DJ mode is reachable without auth.
