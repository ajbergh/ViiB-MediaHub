# DJ Mode - Feature Plan

> **Version:** 2.5  
> **Date:** January 26, 2026  
> **Status:** Phase 3 complete; Phase 4 partial (Headphone Cue + Beat-Phase Sync); polish pending  
> **Target Platform:** Windows (primary / validated); macOS/Linux (planned, not validated)

---

## Implementation Progress

### Phase 1: MVP Status ✅ COMPLETE

| Task | Status | Notes |
|------|--------|-------|
| Planning document | ✅ Complete | This document |
| Architecture review | ✅ Complete | Using Web Audio API (frontend-based) |
| DJ Mixer Zustand slice | ✅ Complete | `slices/djMixerSlice.ts` |
| Types & store integration | ✅ Complete | Updated `slices/types.ts` and `store.ts` |
| DJ Mode page/routing | ✅ Complete | `pages/DJMode.tsx`, route in `App.tsx` |
| Sidebar navigation | ✅ Complete | "DJ Mode" link in sidebar |
| Deck component | ✅ Complete | `components/dj/DeckView.tsx` |
| Waveform component | ✅ Complete | `components/dj/DJWaveform.tsx` |
| Mixer component | ✅ Complete | `components/dj/DJMixer.tsx` |
| Library browser | ✅ Complete | `components/dj/DJLibraryBrowser.tsx` |
| Component index | ✅ Complete | `components/dj/index.ts` |
| DJ Audio Engine | ✅ Complete | `lib/djAudio.ts` - Web Audio API dual-deck engine |
| DJ Audio Hook | ✅ Complete | `hooks/useDJAudioEngine.ts` - React integration |
| Backend waveform API | ✅ Complete | `internal/api/dj_waveform.go` - waveform generation & hot cues |
| Database schema | ✅ Complete | `dj_waveform_cache`, `dj_hot_cues` tables in `db.go` |
| Client-side waveform | ✅ Complete | `lib/clientWaveform.ts` - fallback for OGG/FLAC/AAC |
| Integration testing | ✅ Complete | Tracks load, play, crossfade, EQ, waveforms work |

### Phase 2: BPM + Sync + EQ Status 🔄 NEAR COMPLETE

| Task | Status | Notes |
|------|--------|-------|
| BPM detection library | ✅ Complete | `lib/bpmDetection.ts` - client-side Web Audio API |
| BPM auto-detection | ✅ Complete | Detected on track load, displayed in deck view |
| Tempo fader | ✅ Complete | ±50% range in DeckView, double-click to reset |
| SYNC button | ✅ Complete | Matches BPM to other deck's effective BPM |
| Beat grid detection | ✅ Complete | Generated from BPM, displayed on waveform |
| Beat grid on waveform | ✅ Complete | Downbeats (every 4) highlighted |
| Hot cue UI | ✅ Complete | `DJHotCues.tsx` - 8 slots, click/right-click/hold |
| Hot cue keyboard | ✅ Complete | 1-8 trigger, Shift+1-8 set hot cue |
| Track seeking | ✅ Complete | Progress bar click-to-seek; waveform drag-to-scratch (click-to-seek not implemented) |
| Waveform zoom | ✅ Complete | Shows ~10 seconds centered on playhead |
| Enhanced EQ visualization | ⏳ Not started | Visual feedback for EQ changes |

### Phase 3: FX + Looping + Key Detection Status ✅ COMPLETE

| Task | Status | Notes |
|------|--------|-------|
| **FX System** | | |
| FX type definitions | ✅ Complete | `FilterFX`, `DelayFX`, `ReverbFX`, `FlangerFX` in `djMixerSlice.ts` |
| FX state management | ✅ Complete | Actions: `setFilterFX`, `setDelayFX`, `setReverbFX`, `setFlangerFX`, `toggleFX` |
| FX audio nodes | ✅ Complete | Web Audio API nodes in `lib/djAudio.ts` |
| FX control methods | ✅ Complete | Methods in `DJAudioEngine` class |
| FX hook exposure | ✅ Complete | Methods exposed via `useDJAudioEngine` hook |
| FX Panel UI | ✅ Complete | `components/dj/DJFXPanel.tsx` - Filter/Delay/Reverb/Flanger controls |
| **Loop System** | | |
| Loop state types | ✅ Complete | Already existed in `djMixerSlice.ts` |
| Loop audio engine | ✅ Complete | Position tracking with automatic loop-back |
| Loop control methods | ✅ Complete | `setLoop`, `toggleLoop`, `clearLoop`, `setLoopIn/Out`, `setLoopBeats`, `doubleLoop`, `halveLoop` |
| Loop hook exposure | ✅ Complete | All methods in `useDJAudioEngine` hook |
| Loop Panel UI | ✅ Complete | `components/dj/DJLoopPanel.tsx` - IN/OUT/toggle, beat sizes (1/4-16), double/halve |
| **Key Detection** | | |
| Key detection algorithm | ✅ Complete | `lib/keyDetection.ts` - Krumhansl-Schmuckler algorithm |
| Chromagram analysis | ✅ Complete | FFT-based pitch class profile extraction |
| Camelot/Open Key | ✅ Complete | DJ-friendly key notation included |
| Key detection integration | ✅ Complete | Auto-runs on track load in `useDJAudioEngine` hook |
| Harmonic compatibility | ✅ Complete | `getKeyCompatibility()` and `getHarmonicSuggestions()` functions |

### Phase 3 FX Implementation Details

**Filter FX:**
- Types: Lowpass / Highpass
- Parameters: Frequency (20-20000 Hz), Resonance (0-30)
- Uses: BiquadFilterNode with wet/dry signal switching
- ✅ Fixed: Proper wet/dry gain control ensures filter completely engages/disengages

**Delay FX:**
- Parameters: Time (0.01-1s), Feedback (0-0.9), Mix (0-1)
- Uses: DelayNode with feedback loop
- ✅ Fixed: Feedback zeroed when disabled to stop echoes

**Flanger FX:**
- Parameters: Rate (0.1-5 Hz), Depth (0-1), Feedback (0-0.9)
- Uses: DelayNode with OscillatorNode LFO modulation
- ✅ Fixed: LFO gain, feedback, and wet signal all zeroed when disabled

**Reverb FX:**
- Parameters: Room Size (0-1), Damping (0-1), Mix (0-1)
- Uses: ConvolverNode with algorithmic impulse response
- ✅ Fixed: Impulse response cached - only regenerates when roomSize/damping changes (CPU optimization)

### Phase 3 Loop Implementation Details

**Loop Controls:**
- Loop In/Out: Manual loop point setting
- Beat-synced loops: 1/4, 1/2, 1, 2, 4, 8, 16 beats
- Loop size adjustment: Double (×2) and Halve (÷2)
- Loop toggle and clear

**Loop Behavior:**
- Position tracking checks for loop boundaries on each frame
- Seamless loop-back when playhead reaches loop end
- BPM-aware loop sizing

### Phase 3 Key Detection Implementation Details

**Algorithm:** Krumhansl-Schmuckler
- Computes chromagram (pitch class distribution) via FFT
- Correlates with major/minor key profiles
- Returns best matching key with confidence score

**Output Formats:**
- Standard: "Am", "C#m", "F"
- Camelot: "8A", "3B"
- Open Key: "1m", "8d"

**Integration:**
- Runs automatically on track load (first 30 seconds)
- Key displayed in deck info bar
- Harmonic mixing suggestions available

### Verified Working Features

From user testing:
- ✅ Tracks load on Deck A and B
- ✅ Cross-fade slider works
- ✅ Volume controls work
- ✅ Low/Mid/High EQ controls work
- ✅ BPM detection on track load (accuracy needs validation)
- ✅ SYNC button works
- ✅ Live tempo adjustment without audio distortion
- ✅ Waveforms load (OGG files via client-side generation)
- ✅ Progress bar click-to-seek
- ✅ **Vinyl scratch/jog wheel** - drag waveform to scratch with momentum
- ✅ **Hot cues working** - user confirmed
- ✅ **Scratching works at minimal level** - needs work to sound more authentic
- ✅ **FX controls working** - Filter, Delay, Reverb, Flanger all function correctly
- ✅ **FX panels collapsible** - Section-level collapse for FX & Loop panels
- ✅ **Hot cue persistence** - Hot cues save to and load from backend

### Performance Optimizations (v2.4)

**Position Update Throttling:**
- Reduced state updates from ~60 fps to ~15 fps
- Loop boundary checking still runs at full frame rate for tight loops
- Reduces React re-renders from 120+/sec to ~30/sec with both decks playing

**Component Subscription Optimization:**
- FX Panel: Only subscribes to `fx` state, not entire deck
- Loop Panel: Only subscribes to `loop`, `effectiveBpm`, `originalBpm`
- Hot Cues Panel: Only subscribes to `track`, `hotCues` (position read on-demand)
- Mixer Panel: Only subscribes to `eq`, `volume`, `isPlaying` per deck
- Library Browser: Only subscribes to `track` per deck
- **Result:** Controls remain responsive during playback

### Known Issues / Todo

- 🔧 BPM accuracy validation needed
- 🔧 EQ visualization enhancement pending
- 🔧 Scratch sound could be more authentic
- 🔧 Waveform click-to-seek not implemented (currently reserved for scratch drag)

### Phase 4: Advanced Features Status 🔄 PARTIAL

| Task | Status | Notes |
|------|--------|-------|
| **Headphone Cue System** | | |
| Headphone state types | ✅ Complete | `cueEnabled`, `headphoneVolume`, `headphoneMix` in `djMixerSlice.ts` |
| Headphone audio nodes | ✅ Complete | Separate headphone bus in `lib/djAudio.ts` |
| Cue routing per deck | ✅ Complete | `setCueEnabled()`, `updateHeadphoneMix()`, `setHeadphoneVolume()` |
| Headphone UI controls | ✅ Complete | CUE A/B buttons, mix slider, volume in `DJMixer.tsx` |
| **Beat-Phase Sync** | | |
| Sync mode types | ✅ Complete | `SyncMode = 'off' \| 'bpm' \| 'beat-phase'` in `djMixerSlice.ts` |
| Phase sync algorithm | ✅ Complete | `syncBeatPhase()` in `DJAudioEngine` using beat grids |
| Nudge position | ✅ Complete | `nudgePosition()` for manual beat alignment |
| Sync mode selector UI | ✅ Complete | OFF/BPM/PHASE buttons in mixer panel |
| SYNC button update | ✅ Complete | Shows SYNC+ in phase mode, amber color, performs phase align |
| **Not Implemented** | | |
| Beat grid editing | 🚧 Not started | Manual adjustment UI |
| Mix recording | 🚧 Not started | MediaRecorder API capture |
| Key lock | 🚧 Not started | Pitch-independent tempo |
| Slip mode | 🚧 Not started | Silent playback continue |

### Phase 4 Headphone Cue Implementation Details

**Audio Routing:**
- Separate headphone gain node for each deck (taps after EQ/FX, before crossfader)
- Headphone mix crossfader: 0 = cue only, 1 = master only (constant-power curve)
- Headphone volume control (0-100%)
- Note: In browser, headphone output goes to same device as master (hardware splitter needed)

**UI Controls (in Mixer Panel):**
- CUE A / CUE B buttons: Toggle deck routing to headphones
- MIX slider: Balance between cue signal and master output
- VOL slider: Headphone output volume

### Phase 4 Beat-Phase Sync Implementation Details

**Sync Modes:**
- **OFF:** SYNC button disabled
- **BPM:** Matches tempo only (original behavior)
- **BEAT-PHASE:** Matches tempo AND aligns beat phase (±50ms target)

**Phase Sync Algorithm:**
1. Find current beat index in source deck from beat grid
2. Calculate source's phase within beat (0-1)
3. Find current beat index in target deck
4. Calculate ideal position for target to match source phase
5. Nudge target position (limited to ±half beat to avoid jumping)

**UI Indicators:**
- SYNC button shows "SYNC+" when in beat-phase mode
- Amber color when beat-phase mode active
- Grayed out when sync mode is OFF

---

## Table of Contents

1. [Executive Summary](#executive-summary)
2. [User Stories](#user-stories)
3. [UX Overview](#ux-overview)
4. [Architecture Overview](#architecture-overview)
5. [Audio Engine Requirements](#audio-engine-requirements)
6. [Backend API Specification](#backend-api-specification)
7. [Data Model Changes](#data-model-changes)
8. [Feature Specifications](#feature-specifications)
9. [Metadata & Analysis Pipeline](#metadata--analysis-pipeline)
10. [Performance Requirements](#performance-requirements)
11. [Assumptions & Risks](#assumptions--risks)
12. [Implementation Roadmap](#implementation-roadmap)
13. [Testing Strategy](#testing-strategy)
14. [Appendix](#appendix)

---

## Executive Summary

DJ Mode transforms ViiB MediaHub from a standard media player into a powerful DJ mixing environment. Users can load two independent audio tracks ("decks"), manipulate tempo and pitch, apply real-time effects, and blend outputs via a crossfader—all using their local music library.

### Goals

- **Two-deck mixing** with independent transport controls
- **Real-time waveform visualization** with beat markers
- **BPM detection and sync** between decks
- **Professional mixer controls** (EQ, filters, crossfader)
- **Effects processing** (filters, modulation effects)
- **Seamless library integration** for track browsing and loading
- **Sub-20ms audio latency** for responsive mixing

### Non-Goals (Out of Scope)

- Streaming service integration (Spotify, Apple Music)
- Hardware DJ controller support (future consideration)
- Mix recording to file (Phase 4)
- Video mixing / VJ features
- Multi-deck (3+ decks)

---

## User Stories

### Core Mixing

| ID | As a... | I want to... | So that... | Priority |
|----|---------|--------------|------------|----------|
| US-01 | DJ | Load a track onto Deck A or B | I can prepare two songs for mixing | P0 |
| US-02 | DJ | Play, pause, and cue each deck independently | I have full control over playback timing | P0 |
| US-03 | DJ | See elapsed and remaining time per deck | I know when to transition | P0 |
| US-04 | DJ | Use a crossfader to blend between decks | I can smoothly transition between songs | P0 |
| US-05 | DJ | Adjust volume/gain per channel | I can balance levels before crossfading | P0 |
| US-06 | DJ | See a waveform for each deck | I can visually navigate the track | P0 |
| US-07 | DJ | Seek by clicking on the waveform | I can jump to specific parts quickly | P1 |

### Tempo & Sync

| ID | As a... | I want to... | So that... | Priority |
|----|---------|--------------|------------|----------|
| US-08 | DJ | See the detected BPM of each track | I know if tracks are compatible | P1 |
| US-09 | DJ | Adjust the tempo/pitch of a deck | I can match BPMs manually | P1 |
| US-10 | DJ | Press SYNC to auto-match BPM | Tempo matching is quick and easy | P1 |
| US-11 | DJ | See the musical key of each track | I can create harmonic mixes | P2 |

### EQ & Effects

| ID | As a... | I want to... | So that... | Priority |
|----|---------|--------------|------------|----------|
| US-12 | DJ | Adjust 3-band EQ (High/Mid/Low) per deck | I can shape the sound during transitions | P1 |
| US-13 | DJ | Apply a filter sweep per deck | I can create build-ups and transitions | P2 |
| US-14 | DJ | Enable an effect (e.g., Flanger) per deck | I can add creative elements to my mix | P2 |

### Looping & Performance

| ID | As a... | I want to... | So that... | Priority |
|----|---------|--------------|------------|----------|
| US-15 | DJ | Set a loop with beat-quantized length | I can extend sections of a track | P2 |
| US-16 | DJ | Use beat-jump buttons (1, 2, 4, 8 beats) | I can quickly navigate within a track | P2 |
| US-17 | DJ | Set and trigger hot cue points | I can jump to marked positions instantly | P3 |

### Library Integration

| ID | As a... | I want to... | So that... | Priority |
|----|---------|--------------|------------|----------|
| US-18 | DJ | Browse my library within DJ Mode | I don't have to leave the DJ interface | P0 |
| US-19 | DJ | Search/filter tracks by name, artist, BPM, key | I can find compatible tracks quickly | P1 |
| US-20 | DJ | Drag and drop tracks onto decks | Loading is intuitive | P1 |
| US-21 | DJ | See which tracks are currently loaded | I avoid accidentally loading duplicates | P1 |

---

## UX Overview

### Layout Structure

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              TOP BAR                                         │
│  [Time/BPM/Key - Deck A]     [Record] [Settings]     [Time/BPM/Key - Deck B] │
├─────────────────────────────────────────────────────────────────────────────┤
│                           WAVEFORM SECTION                                   │
│  ┌─────────────────────────────┐   ┌─────────────────────────────────────┐  │
│  │   Deck A Waveform           │   │   Deck B Waveform                   │  │
│  │   ▓▓▓▓▓▓░░░░░░░░░░░░░░░░░░  │   │   ░░░░░░░░░▓▓▓▓▓▓░░░░░░░░░░░░░░░░ │  │
│  │        ▲ playhead           │   │              ▲ playhead             │  │
│  └─────────────────────────────┘   └─────────────────────────────────────┘  │
├───────────────────────┬─────────────────────────┬───────────────────────────┤
│      DECK A           │        MIXER            │         DECK B            │
│  ┌─────────────────┐  │  ┌─────────────────┐   │  ┌─────────────────────┐  │
│  │  [Album Art]    │  │  │ HIGH  HIGH      │   │  │    [Album Art]      │  │
│  │                 │  │  │  ◯     ◯        │   │  │                     │  │
│  │  ┌───┐  ┌───┐  │  │  │ MID   MID       │   │  │  ┌───┐  ┌───┐      │  │
│  │  │ ▲ │  │FX │  │  │  │  ◯     ◯        │   │  │  │FX │  │ ▲ │      │  │
│  │  │ T │  └───┘  │  │  │ LOW   LOW       │   │  │  └───┘  │ T │      │  │
│  │  │ E │         │  │  │  ◯     ◯        │   │  │         │ E │      │  │
│  │  │ M │  [SYNC] │  │  │                 │   │  │  [SYNC] │ M │      │  │
│  │  │ P │         │  │  │ ┌─────────────┐ │   │  │         │ P │      │  │
│  │  │ O │  ▶ ⏸ ◀│ │  │ │ CROSSFADER  │ │   │  │ ▶ ⏸ ◀│ │ O │      │  │
│  │  └───┘         │  │  │ └─────────────┘ │   │  │         └───┘      │  │
│  │  [Loop/Pads]   │  │  │  Vol A   Vol B  │   │  │    [Loop/Pads]     │  │
│  └─────────────────┘  │  └─────────────────┘   │  └─────────────────────┘  │
├───────────────────────┴─────────────────────────┴───────────────────────────┤
│                          LIBRARY BROWSER                                     │
│  ┌───────────┬──────────────────────────────────────────────────────────┐   │
│  │ Playlists │  Name      │ Artist    │ Album  │ BPM │ Key │ Time │ ... │   │
│  │ ─────────│  ─────────│ ─────────│ ──────│ ───│ ───│ ────│     │   │
│  │ ▶ Music   │  Track 1   │ Artist A  │ Album  │ 128 │ Am  │ 3:45 │     │   │
│  │   Mixes   │  Track 2   │ Artist B  │ Album  │ 125 │ Cm  │ 4:12 │     │   │
│  │   Dance   │  Track 3   │ Artist C  │ Album  │ 130 │ Gm  │ 3:58 │     │   │
│  │   Chill   │  ...       │           │        │     │     │      │     │   │
│  └───────────┴──────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Key UI Components

#### 1. Waveform Display (per deck)

- **Scrolling waveform** with fixed playhead at center
- **Single-color amplitude waveform** (future: color-coded frequency bands)
- **Beat grid markers** as vertical lines
- **Scratch control** via click+drag on waveform
- **Waveform click-to-seek** (planned; currently not implemented)
- **Fixed zoom**: 10-second window centered on playhead

#### 2. Deck Controls

| Element | Description |
|---------|-------------|
| **Play/Pause** | Toggle playback |
| **Cue** | Return to cue point; hold to preview |
| **Tempo Fader** | Vertical slider, ±50% range, center = original |
| **BPM Display** | Shows current effective BPM (with adjustment) |
| **Key Display** | Musical key (e.g., "Am", "C#m") |
| **Sync Button** | Match BPM to other deck |
| **Album Art** | Visual track identification |
| **Time Display** | Elapsed / Remaining toggle |

#### 3. Mixer Section

| Element | Description |
|---------|-------------|
| **Crossfader** | Horizontal slider, Left = Deck A, Right = Deck B |
| **Channel Volume** | Per-deck vertical faders |
| **3-Band EQ** | High/Mid/Low rotary knobs per channel |
| **Filter Knob** | Low-pass/High-pass sweep per channel |
| **Headphone Cue** | (Phase 4) Route deck to headphone output |

#### 4. FX Section (per deck)

| Element | Description |
|---------|-------------|
| **FX Enable** | Toggle effect on/off |
| **FX Select** | Dropdown: Filter, Flanger, Echo, Reverb, etc. |
| **Wet/Dry** | Mix amount |
| **Parameter Knobs** | Effect-specific (Rate, Depth, Feedback) |

#### 5. Loop/Pad Section (per deck)

| Element | Description |
|---------|-------------|
| **Loop In/Out** | Manual loop point setting |
| **Loop Size Buttons** | 1/16, 1/8, 1/4, 1/2, 1, 2, 4, 8 beats |
| **Loop Enable** | Toggle loop on/off |
| **Beat Jump** | ◀◀ ◀ ▶ ▶▶ (1, 4, 16, 32 beats) |
| **Hot Cue Pads** | 4-8 trigger pads for saved positions |

#### 6. Library Browser

- **Playlist Sidebar**: Collapsible tree of playlists/folders
- **Track Table**: Sortable columns (Name, Artist, Album, Genre, BPM, Key, Time, Year)
- **Search Bar**: Real-time filtering
- **Load Actions**: Double-click, drag-drop, or "Load to A/B" buttons
- **Visual Indicators**: Currently loaded tracks highlighted
- **BPM/Key Filtering**: Quick filter by compatible BPM range or harmonic key

### Keyboard Shortcuts

| Key | Action |
|-----|--------|
| `Space` | Play/Pause active deck |
| `Shift+Space` | Play/Pause inactive deck |
| `Q` | Cue Deck A |
| `W` | Play/Pause Deck A |
| `E` | Sync Deck A |
| `O` | Cue Deck B |
| `P` | Play/Pause Deck B |
| `[` | Sync Deck B |
| `Z` | Crossfader full left |
| `X` | Crossfader center |
| `C` | Crossfader full right |
| `←` / `→` | Nudge crossfader |
| `1-8` | Trigger hot cues (active deck) |
| `L` | Toggle loop (active deck) |
| `Shift+L` | Halve loop length |
| `Ctrl+L` | Double loop length |
| `Tab` | Switch active deck |
| `Ctrl+F` | Focus library search |

### Error States

| Scenario | UI Response |
|----------|-------------|
| File not found | Toast: "Track unavailable - file missing or moved" + disable deck |
| Unsupported codec | Toast: "Format not supported: [codec]" + show in track list |
| Analysis unavailable | Show "?" for BPM/Key, disable sync, offer "Analyze" button |
| Audio device error | Modal: "Audio output error" with device selection |
| Deck load during playback | Confirm dialog: "Replace currently playing track?" |

### Accessibility Considerations

- **Focus management**: Tab navigation through all controls
- **ARIA labels**: All buttons, sliders, and interactive elements labeled
- **High contrast mode**: Ensure waveforms and beat grids are visible
- **Keyboard-only operation**: Full functionality without mouse
- **Screen reader announcements**: Track load, play state changes, BPM/key info
- **Non-color indicators**: Shape/pattern differentiation for deck states (not just color)

---

## Architecture Overview

> **Note:** This architecture uses **Web Audio API in the frontend** for audio playback, consistent with the existing ViiB MediaHub audio implementation. The backend handles analysis, waveform generation, and data persistence while the frontend manages real-time audio mixing.

### System Architecture Diagram

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         FRONTEND (React + Web Audio API)                     │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                         DJ Mode UI Components                        │   │
│  │  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐  │   │
│  │  │ DeckView │ │ Mixer    │ │ Waveform │ │ FXPanel  │ │ Library  │  │   │
│  │  │          │ │          │ │ Display  │ │          │ │ Browser  │  │   │
│  │  └──────────┘ └──────────┘ └──────────┘ └──────────┘ └──────────┘  │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                       Zustand Store (DJ Mixer Slice)                 │   │
│  │  • deckA/deckB state    • mixer state    • FX state    • UI state   │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                     Web Audio API Engine (DJAudioEngine)             │   │
│  │  ┌─────────────┐ ┌─────────────┐ ┌─────────────┐ ┌─────────────┐   │   │
│  │  │   Deck A    │ │   Deck B    │ │   Mixer     │ │ Master Out  │   │   │
│  │  │  ┌───────┐  │ │  ┌───────┐  │ │  ┌───────┐  │ │  ┌───────┐  │   │   │
│  │  │  │ Audio │  │ │  │ Audio │  │ │  │Crossfdr│ │ │  │Analyser│  │   │   │
│  │  │  │Element│  │ │  │Element│  │ │  │GainNode│ │ │  │Compres.│  │   │   │
│  │  │  └───┬───┘  │ │  └───┬───┘  │ │  └───────┘  │ │  └───────┘  │   │   │
│  │  │  ┌───▼───┐  │ │  ┌───▼───┐  │ │  ┌───────┐  │ └─────────────┘   │   │
│  │  │  │MediaEl│  │ │  │MediaEl│  │ │  │ EQ A  │  │                    │   │
│  │  │  │Source │  │ │  │Source │  │ │  │ EQ B  │  │                    │   │
│  │  │  └───┬───┘  │ │  └───┬───┘  │ │  │Biquad │  │                    │   │
│  │  │  ┌───▼───┐  │ │  ┌───▼───┐  │ │  └───────┘  │                    │   │
│  │  │  │GainNod│  │ │  │GainNod│  │ └─────────────┘                    │   │
│  │  │  │(Vol)  │  │ │  │(Vol)  │  │                                    │   │
│  │  │  └───────┘  │ │  └───────┘  │                                    │   │
│  │  └─────────────┘ └─────────────┘                                    │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                    Backend Service (REST/SSE Client)                 │   │
│  │  • Waveform fetching    • Analysis requests    • Track metadata     │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────────────┘
                                      │
                                      │ HTTP REST + SSE
                                      │ • GET /api/dj/waveform/{id}
                                      │ • GET /api/dj/hotcues/{id}
                                      │ • PUT /api/dj/hotcues/{id}
                                      │ • GET /api/dj/personas
                                      ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                              BACKEND (Go)                                    │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                         DJ API Handlers                              │   │
│  │  • Waveform endpoint    • Analysis endpoint    • Hot cue CRUD       │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │  • Waveform endpoint    • Hot cue CRUD    • Personas                │   │
│  │  • Waveform generation (MP3)  • Key detection (Phase 3)             │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │  • Waveform generation (MP3 only; others client-side fallback)      │   │
│  │  • Track metadata    • Analysis cache    • DJ settings/presets      │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────────────┘

> **Note:** BPM detection is performed client-side using Web Audio API for lower latency.

```

### Responsibility Division

#### Frontend (React/TypeScript + Web Audio API)

| Responsibility | Details |
|----------------|---------|
| **Audio Playback** | Two `<audio>` elements connected to Web Audio API graph |
| **DSP Processing** | EQ (BiquadFilterNodes), gain, crossfader via Web Audio |
| **BPM Detection** | Client-side peak detection using Web Audio API (`lib/bpmDetection.ts`) |
| **Waveform Fallback** | Client-side generation for OGG/FLAC/AAC (`lib/clientWaveform.ts`) |
| **UI Rendering** | Deck controls, mixer, waveforms, library browser |
| **Waveform Visualization** | Canvas rendering of peak data with beat grid overlay |
| **User Input Handling** | Mouse, keyboard, drag-drop interactions |
| **State Management** | Local UI state + Zustand `djMixerSlice` |
| **Animation** | 60fps render loop for waveforms, VU meters, playhead |

#### Backend (Go)

| Responsibility | Details |
|----------------|---------|
| **Audio Serving** | Stream files via `/api/audio/{id}` (existing) |
| **Waveform Generation** | Compute peak data from MP3 files (backend), fallback to client-side for OGG/FLAC |
| **Waveform Caching** | Store waveform peaks in SQLite (`dj_waveform_cache`) |
| **Hot Cues API** | Persist hot cue data in SQLite (`dj_hot_cues`) via `/api/dj/hotcues/{id}` |
| **Library Management** | Track database, playlist management |
| **Mix Recording** | (Phase 4) Capture master output to file |

> **Implementation Note:** BPM detection is now performed **client-side** using Web Audio API (`lib/bpmDetection.ts`) for lower latency and simpler architecture. Waveform generation uses a hybrid approach: backend for MP3 files, client-side fallback for OGG/FLAC/AAC.

### Communication Architecture

> **Note:** Unlike the original plan which considered WebSocket for real-time communication, the DJ Mode implementation uses a **frontend-centric architecture** where:
> - Audio processing happens entirely in the browser via Web Audio API
> - State management is handled by Zustand (djMixerSlice)
> - Backend is only consulted for:
>   - Audio file streaming (`/api/audio/{id}`)
>   - Waveform data (`/api/dj/waveform/{id}`)
>   - Hot cue persistence (`/api/dj/hotcues/{id}`) - ✅ wired

**Why Frontend Audio (Not Backend WebSocket):**

1. **Lower latency**: No network round-trip for audio processing
2. **Simpler architecture**: Consistent with existing ViiB audio playback
3. **Cross-platform**: Web Audio API works identically everywhere
4. **Real-time guaranteed**: No network jitter affects audio

**State Flow:**

```typescript
// User Action → Zustand → Web Audio API → Audio Output
//
// 1. User clicks Play on Deck A
// 2. djMixerSlice.togglePlayDeck('A') called
// 3. Zustand updates djDeckA.isPlaying = true
// 4. DJAudioEngine (lib/djAudio.ts) reacts to state change
// 5. Web Audio: audioElementA.play()
// 6. Audio plays through Web Audio graph
```

---

## Audio Engine Requirements (Web Audio API)

> **Implementation Note:** DJ Mode uses the browser's Web Audio API for audio processing, consistent with the existing ViiB MediaHub audio architecture. This provides low-latency mixing, EQ, and effects without requiring a Go-based audio engine.

### Core Capabilities

| Capability | Requirement | Web Audio Implementation |
|------------|-------------|--------------------------|
| **Simultaneous Playback** | 2 independent streams | Two `<audio>` elements + MediaElementSourceNodes |
| **Sample Rate** | System default (44.1/48kHz) | AudioContext.sampleRate |
| **Processing** | Real-time DSP | BiquadFilterNode, GainNode chains |
| **Latency** | <50ms perceived | Browser-managed buffering |
| **Channel Count** | Stereo per deck | ChannelMergerNode for mixing |

### Format Support (Browser-Native)

| Format | Support | Notes |
|--------|---------|-------|
| MP3 | ✅ Excellent | Universal browser support |
| WAV | ✅ Excellent | Universal browser support |
| FLAC | ✅ Good | Chrome, Firefox, Edge |
| AAC/M4A | ✅ Good | All modern browsers |
| OGG Vorbis | ✅ Good | Chrome, Firefox |
| Opus | ✅ Good | Chrome, Firefox |

### Web Audio DSP Chain (per Deck)

```
┌─────────────────────────────────────────────────────────────────┐
│                      DJ Audio Engine (TypeScript)               │
│                                                                 │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │                         Deck A                           │   │
│  │  <audio> ──► MediaElementSource ──► GainNode (volume)    │   │
│  │                        │                  │              │   │
│  │                        ▼                  │              │   │
│  │                 BiquadFilter (EQ High)    │              │   │
│  │                        │                  │              │   │
│  │                        ▼                  │              │   │
│  │                 BiquadFilter (EQ Mid)     │              │   │
│  │                        │                  │              │   │
│  │                        ▼                  │              │   │
│  │                 BiquadFilter (EQ Low)     │              │   │
│  │                        │                  │              │   │
│  │                        ▼                  │              │   │
│  │                 BiquadFilter (Filter)─────┘              │   │
│  │                        │                                 │   │
│  └────────────────────────┼─────────────────────────────────┘   │
│                           │                                      │
│  ┌────────────────────────┼─────────────────────────────────┐   │
│  │                        ▼         Mixer                   │   │
│  │  Deck A Out ──► GainNode (crossfader A) ──┐              │   │
│  │                                           │              │   │
│  │  Deck B Out ──► GainNode (crossfader B) ──┼──► GainNode  │   │
│  │                                           │    (master)  │   │
│  │                                           │       │      │   │
│  │                                           │       ▼      │   │
│  │                                      AnalyserNode (VU)   │   │
│  │                                           │              │   │
│  │                                           ▼              │   │
│  │                                 DynamicsCompressor       │   │
│  │                                    (limiter)             │   │
│  │                                           │              │   │
│  │                                           ▼              │   │
│  │                                   AudioDestination       │   │
│  └──────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────┘
```

### EQ Implementation

Using cascaded BiquadFilterNodes:

| Band | Filter Type | Frequency | Q |
|------|-------------|-----------|---|
| High | highshelf | 3000 Hz | 1.0 |
| Mid | peaking | 1000 Hz | 1.0 |
| Low | lowshelf | 200 Hz | 1.0 |

Gain range: -24dB to +12dB (with -Infinity for "kill")

### Crossfader Implementation

```typescript
// Constant-power crossfade curve
const position = crossfaderValue; // -1 to +1
const gainA = Math.cos((position + 1) * Math.PI / 4);
const gainB = Math.cos((1 - position) * Math.PI / 4);
```

### Tempo Adjustment (Phase 2+)

**MVP Approach:** Use `<audio>.playbackRate` property
- Pros: Simple, no additional libraries
- Cons: Changes pitch along with tempo

**Phase 3 Approach:** Integrate SoundTouchJS or similar
- Time-stretching with pitch preservation
- Requires AudioWorklet for real-time processing

### Vinyl Scratch / Jog Wheel (Phase 2)

> **IMPLEMENTED:** Vinyl scratch simulation via waveform drag.

**Implementation Details:**

The scratch feature in `lib/djAudio.ts` provides vinyl-style scrubbing:

1. **`startScratch(deck)`**: Captures current state (position, tempo)
2. **`updateScratch(deck, deltaTime, velocity)`**: 
   - Moves playhead based on drag movement
   - Adjusts playback rate based on drag velocity
   - Pauses audio during slow/reverse movement
3. **`endScratch(deck, finalVelocity, resumePlayback)`**:
   - Applies momentum effect (gradual slowdown to normal tempo)
   - Restores normal playback

**UI Interaction (DJWaveform.tsx):**
- **Mouse down**: Starts scratch mode, captures initial position
- **Mouse move**: Updates position based on drag delta, calculates velocity
- **Mouse up**: Ends scratch with momentum from final velocity
- **Mouse leave**: Ends scratch without momentum (safety)

**Momentum System:**
- Tracks last 5 velocity samples for averaging
- Decay animation from scratch velocity to normal tempo
- Ease-out curve over 300ms

**Visual Feedback:**
- Cursor changes to "grabbing" during scratch
- "SCRATCH" badge appears on waveform
- Playhead position updates in real-time

### Latency Characteristics

| Stage | Typical | Notes |
|-------|---------|-------|
| Audio element buffering | 10-50ms | Browser-controlled |
| Web Audio processing | <5ms | Per audio block |
| UI command dispatch | <1ms | JavaScript event |
| Visual feedback | 16ms | 60fps frame |
| **Total perceived** | **~50-100ms** | Acceptable for mixing |

---

## Backend API Specification

### REST Endpoints for DJ Mode

Since audio processing happens in the frontend, the backend provides data services:

| Endpoint | Method | Purpose |
|----------|--------|---------|
| `/api/audio/{id}` | GET | Audio file streaming (existing) |
| `/api/dj/personas` | GET | DJ persona definitions (used by AI DJ features) |
| `/api/dj/waveform/{id}` | GET | Precomputed waveform peaks |
| `/api/dj/hotcues/{id}` | GET | Get hot cue points |
| `/api/dj/hotcues/{id}` | PUT | Save hot cue points |

### Waveform Data Format

```typescript
interface WaveformResponse {
  trackId: string;
  duration: number;        // Total duration in seconds
  sampleRate: number;      // Source sample rate
  resolution: number;      // Samples per peak (e.g., 256)
  peaks: number[];         // Normalized peak values (0-1)
}
```

> **Note:** BPM detection, key detection, and beat grid generation are currently performed **client-side** (see `lib/bpmDetection.ts` and `lib/keyDetection.ts`).

### Analysis Endpoint

Not implemented as a backend endpoint. Analysis is computed client-side on track load.

### Hot Cue API

```typescript
// GET /api/dj/hotcues/{trackId}
interface HotCueResponse {
  trackId: string;
  hotCues: {
    slot: number;      // 1-8
    position: number;  // Seconds
    label?: string;
    color: string;     // Hex color
  }[];
}

// PUT /api/dj/hotcues/{trackId}
interface HotCueSaveRequest {
  hotCues: HotCueResponse['hotCues'];
}
```

> **Status:** Backend persistence endpoints exist; the current UI keeps hot cues in the Zustand store only.

### Zustand State Actions (Frontend)

Since audio processing happens in the frontend via Web Audio API, the following actions are managed by Zustand (djMixerSlice) rather than backend commands:

#### Deck Actions (djMixerSlice)

| Action | Parameters | Effect |
|--------|------------|--------|
| `loadTrackToDeck` | `deck: 'A'|'B', track: Song` | Load track to specified deck |
| `unloadDeck` | `deck: 'A'|'B'` | Clear deck and stop playback |
| `togglePlayDeck` | `deck: 'A'|'B'` | Toggle play/pause state |
| `seekDeck` | `deck: 'A'|'B', position: number` | Seek to position (seconds) |
| `cueDeck` | `deck: 'A'|'B'` | Return to cue point |
| `setCuePoint` | `deck: 'A'|'B', position: number` | Set cue point |
| `setDeckVolume` | `deck: 'A'|'B', volume: number` | Set volume (0-1) |
| `setDeckTempo` | `deck: 'A'|'B', tempo: number` | Set tempo (0.5-1.5) |

#### Mixer Actions (djMixerSlice)

| Action | Parameters | Effect |
|--------|------------|--------|
| `setCrossfader` | `position: number` | Set crossfader (-1 to 1) |
| `setMasterVolume` | `volume: number` | Set master volume (0-1) |
| `setDeckEQ` | `deck, band, gain` | Set EQ band gain |

#### Effect Actions (Phase 2+)

| Action | Parameters | Effect |
|--------|------------|--------|
| `setDeckFilter` | `deck, value: number` | Set filter (-1 to 1) |
| `setDeckEffect` | `deck, effect, enabled` | Toggle effect |
| `setEffectParam` | `deck, param, value` | Set effect parameter |

### REST Endpoints

Backend provides data services (not audio processing):

| Endpoint | Method | Purpose |
|----------|--------|---------|
| `/api/audio/{id}` | GET | Audio file streaming (existing) |
| `/api/dj/waveform/{trackId}` | GET | Precomputed waveform peaks |
| `/api/dj/analysis/{trackId}` | GET | BPM, key, analysis status |
| `/api/dj/analysis/{trackId}` | POST | Request analysis for track |
| `/api/dj/hotcues/{trackId}` | GET | Get hot cue points |
| `/api/dj/hotcues/{trackId}` | PUT | Save hot cue points |
| `/api/dj/settings` | GET/PUT | DJ mode preferences |

---

## Data Model Changes

### New Tables

#### `dj_analysis`

Stores computed audio analysis data per track.

| Column | Type | Description |
|--------|------|-------------|
| `id` | INTEGER PK | Auto-increment |
| `song_id` | INTEGER FK | Reference to songs table |
| `bpm` | REAL | Detected beats per minute |
| `bpm_confidence` | REAL | Confidence score (0-1) |
| `key` | TEXT | Musical key (e.g., "Am", "C#m") |
| `key_confidence` | REAL | Confidence score (0-1) |
| `waveform_peaks` | BLOB | Compressed peak data |
| `beat_grid` | BLOB | Beat position timestamps |
| `analyzed_at` | DATETIME | Analysis timestamp |
| `analyzer_version` | TEXT | Version for cache invalidation |

#### `dj_hot_cues`

Stores hot cue points per track.

| Column | Type | Description |
|--------|------|-------------|
| `id` | INTEGER PK | Auto-increment |
| `song_id` | INTEGER FK | Reference to songs table |
| `slot` | INTEGER | Hot cue slot (1-8) |
| `position` | REAL | Position in seconds |
| `label` | TEXT | Optional label |
| `color` | TEXT | Hex color code |
| `created_at` | DATETIME | Creation timestamp |

#### `dj_settings`

Stores DJ mode preferences.

| Column | Type | Description |
|--------|------|-------------|
| `id` | INTEGER PK | Auto-increment |
| `key` | TEXT UNIQUE | Setting key |
| `value` | TEXT | JSON-encoded value |
| `updated_at` | DATETIME | Last update |

**Settings Keys:**
- `crossfader_curve`: Linear, logarithmic, constant power
- `eq_preset`: Default EQ values
- `fx_defaults`: Default effect selections
- `tempo_range`: ±8%, ±16%, ±50%
- `sync_mode`: BPM only, beat phase, off
- `waveform_color`: Frequency-colored, mono, RGB

#### `dj_mix_recordings` (Phase 4)

| Column | Type | Description |
|--------|------|-------------|
| `id` | INTEGER PK | Auto-increment |
| `filename` | TEXT | Output file path |
| `duration` | REAL | Length in seconds |
| `format` | TEXT | WAV, FLAC, MP3 |
| `tracklist` | TEXT | JSON array of played tracks |
| `recorded_at` | DATETIME | Recording timestamp |

### Songs Table Extensions

Add columns to existing `songs` table:

| Column | Type | Description |
|--------|------|-------------|
| `bpm` | REAL | Cached BPM (from analysis) |
| `musical_key` | TEXT | Cached key (from analysis) |
| `energy` | REAL | Energy level (0-1) |
| `danceability` | REAL | Danceability score (0-1) |

### Schema Migration

```sql
-- Migration: Add DJ Mode tables

CREATE TABLE IF NOT EXISTS dj_analysis (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    song_id INTEGER NOT NULL UNIQUE,
    bpm REAL,
    bpm_confidence REAL,
    key TEXT,
    key_confidence REAL,
    waveform_peaks BLOB,
    beat_grid BLOB,
    analyzed_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    analyzer_version TEXT,
    FOREIGN KEY (song_id) REFERENCES songs(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS dj_hot_cues (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    song_id INTEGER NOT NULL,
    slot INTEGER NOT NULL,
    position REAL NOT NULL,
    label TEXT,
    color TEXT DEFAULT '#FF5500',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (song_id) REFERENCES songs(id) ON DELETE CASCADE,
    UNIQUE(song_id, slot)
);

CREATE TABLE IF NOT EXISTS dj_settings (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    key TEXT UNIQUE NOT NULL,
    value TEXT NOT NULL,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Add columns to songs table
ALTER TABLE songs ADD COLUMN bpm REAL;
ALTER TABLE songs ADD COLUMN musical_key TEXT;
ALTER TABLE songs ADD COLUMN energy REAL;
ALTER TABLE songs ADD COLUMN danceability REAL;

-- Indexes for DJ queries
CREATE INDEX IF NOT EXISTS idx_songs_bpm ON songs(bpm);
CREATE INDEX IF NOT EXISTS idx_songs_key ON songs(musical_key);
CREATE INDEX IF NOT EXISTS idx_dj_analysis_song ON dj_analysis(song_id);
```

---

## Feature Specifications

### 1. Two Decks + Transport

#### Functional Requirements

| ID | Requirement | Priority |
|----|-------------|----------|
| F1.1 | Load track from library to Deck A or B | P0 |
| F1.2 | Play/Pause independently per deck | P0 |
| F1.3 | Cue button: return to cue point, hold to preview | P0 |
| F1.4 | Display elapsed time (MM:SS.ss) | P0 |
| F1.5 | Display remaining time (toggle) | P1 |
| F1.6 | Visual indicator for playing/paused state | P0 |
| F1.7 | Track info display (title, artist, album art) | P0 |
| F1.8 | Prevent loading track to playing deck without confirmation | P1 |

#### Acceptance Criteria

- [ ] User can load different tracks to Deck A and Deck B simultaneously
- [ ] Play/Pause on Deck A does not affect Deck B
- [ ] Cue point is set at load position or last pause position
- [ ] Time display updates at minimum 10fps during playback
- [ ] Playing deck has clear visual distinction (glow, color, icon)

### 2. Waveforms + Beat Grid

#### Functional Requirements

| ID | Requirement | Priority |
|----|-------------|----------|
| F2.1 | Display scrolling waveform per deck | P0 |
| F2.2 | Playhead indicator (fixed center position) | P0 |
| F2.3 | Click-to-seek on waveform | P1 |
| F2.4 | Show BPM value per deck | P1 |
| F2.5 | Show musical key per deck | P2 |
| F2.6 | Beat grid markers on waveform | P2 |
| F2.7 | Color-coded frequency display | P2 |

#### Waveform Data Format

```typescript
interface WaveformData {
  trackId: string;
  sampleRate: number;        // Original sample rate
  duration: number;          // Total duration in seconds
  resolution: number;        // Samples per peak (e.g., 256)
  peaks: {
    min: Float32Array;       // Min values per segment
    max: Float32Array;       // Max values per segment
  };
  // Optional frequency bands for colored waveforms
  bands?: {
    low: Float32Array;       // Bass energy per segment
    mid: Float32Array;       // Mid energy per segment
    high: Float32Array;      // Treble energy per segment
  };
}
```

#### Acceptance Criteria

- [ ] Waveform renders smoothly at 60fps during playback
- [ ] Waveform loads within 500ms for cached tracks
- [ ] Clicking waveform seeks to correct position (±100ms accuracy)
- [ ] BPM displays to 1 decimal place (e.g., "128.0")
- [ ] Key displays in standard notation (e.g., "Am", "F#m")

### 3. Tempo, Sync, and Pitch Controls

#### Functional Requirements

| ID | Requirement | Priority |
|----|-------------|----------|
| F3.1 | Tempo fader per deck (±50% range) | P1 |
| F3.2 | Display effective BPM (original × tempo) | P1 |
| F3.3 | SYNC button: match BPM to other deck | P1 |
| F3.4 | Visual indication when tempo is adjusted | P1 |
| F3.5 | Reset tempo to original (double-click fader) | P1 |
| F3.6 | Pitch lock toggle (change tempo, keep pitch) | P2 |

#### Sync Behavior

**MVP Sync (Phase 2):**
- Match BPM only (no beat alignment)
- Target BPM = other deck's effective BPM
- Instant tempo change (no gradual transition)

**Advanced Sync (Phase 4):**
- Beat-phase alignment
- Downbeat detection and matching
- Gradual tempo transition option

#### Acceptance Criteria

- [ ] Tempo fader smoothly adjusts from 50% to 150%
- [ ] BPM display updates in real-time as fader moves
- [ ] SYNC sets deck BPM to match other deck within 0.1 BPM
- [ ] Tempo change does not cause audio glitches or dropouts
- [ ] CPU usage increase <5% when time-stretching active

### 4. Mixer: Crossfader + Volume + EQ

#### Functional Requirements

| ID | Requirement | Priority |
|----|-------------|----------|
| F4.1 | Horizontal crossfader control | P0 |
| F4.2 | Per-deck volume fader | P0 |
| F4.3 | Per-deck 3-band EQ (High/Mid/Low) | P1 |
| F4.4 | Per-deck filter (LP/HP) | P2 |
| F4.5 | Master limiter to prevent clipping | P1 |
| F4.6 | VU meters (per-deck and master) | P1 |

#### Crossfader Curves

| Curve | Behavior | Use Case |
|-------|----------|----------|
| Linear | Equal power across range | General mixing |
| Logarithmic | Fast cut at ends | Scratching |
| Constant Power | -3dB at center | Smooth transitions |

#### EQ Specifications

| Band | Center Frequency | Range |
|------|------------------|-------|
| High | 5 kHz (shelf) | ±12 dB |
| Mid | 1 kHz (peak) | ±12 dB |
| Low | 100 Hz (shelf) | ±12 dB |

#### Acceptance Criteria

- [ ] Crossfader moves smoothly, no stepping or zipper noise
- [ ] Full left = 100% Deck A, full right = 100% Deck B
- [ ] Volume faders provide smooth 0-100% range
- [ ] EQ "kill" (full cut) effectively silences that band
- [ ] Master output never exceeds 0 dBFS (limiter active)
- [ ] VU meters update at minimum 30fps

### 5. FX Section

#### Functional Requirements

| ID | Requirement | Priority |
|----|-------------|----------|
| F5.1 | FX enable/disable toggle per deck | P2 |
| F5.2 | Effect selection dropdown | P2 |
| F5.3 | Wet/dry mix control | P2 |
| F5.4 | Effect-specific parameter controls | P2 |
| F5.5 | Effect bypass (kill switch) | P3 |

#### MVP Effects

| Effect | Parameters | Description |
|--------|------------|-------------|
| **Filter** | Cutoff, Resonance | LP/HP/BP filter |
| **Flanger** | Rate, Depth, Feedback | Classic flanger |
| **Echo** | Time, Feedback, Mix | Tempo-synced delay |
| **Reverb** | Size, Decay, Mix | Room/hall reverb |

#### Acceptance Criteria

- [ ] Effects toggle on/off without pops or clicks
- [ ] Wet/dry smoothly blends from 0% to 100%
- [ ] Effect parameters respond in real-time (<20ms)
- [ ] CPU usage per effect <3% on mid-range hardware

### 6. Looping / Pads / Beat Jump

#### Functional Requirements

| ID | Requirement | Priority |
|----|-------------|----------|
| F6.1 | Loop enable/disable | P2 |
| F6.2 | Loop length selection (1/16 to 8 beats) | P2 |
| F6.3 | Loop in/out manual set | P2 |
| F6.4 | Beat jump forward/backward | P2 |
| F6.5 | Hot cue pads (4-8 slots) | P3 |
| F6.6 | Quantized loop to beat grid | P3 |

#### Loop Lengths (based on BPM)

| Button | Beats | At 120 BPM |
|--------|-------|------------|
| 1/16 | 0.0625 | 31ms |
| 1/8 | 0.125 | 62ms |
| 1/4 | 0.25 | 125ms |
| 1/2 | 0.5 | 250ms |
| 1 | 1 | 500ms |
| 2 | 2 | 1000ms |
| 4 | 4 | 2000ms |
| 8 | 8 | 4000ms |

#### Acceptance Criteria

- [ ] Loop plays seamlessly without audible gap
- [ ] Loop length buttons set correct duration based on current BPM
- [ ] Beat jump moves exactly N beats (±10ms accuracy)
- [ ] Hot cues trigger instantly (<20ms)
- [ ] Hot cue positions persist across sessions

### 7. Library Browser Integration

#### Functional Requirements

| ID | Requirement | Priority |
|----|-------------|----------|
| F7.1 | Display track list with DJ-relevant columns | P0 |
| F7.2 | Sort by any column | P1 |
| F7.3 | Filter/search by text | P1 |
| F7.4 | Filter by BPM range | P2 |
| F7.5 | Filter by compatible key | P2 |
| F7.6 | Drag-and-drop to decks | P1 |
| F7.7 | "Load to Deck A/B" context menu | P1 |
| F7.8 | Highlight currently loaded tracks | P0 |
| F7.9 | Show analysis status (analyzed/pending) | P1 |

#### DJ-Specific Columns

| Column | Source | Sortable |
|--------|--------|----------|
| Title | Metadata | Yes |
| Artist | Metadata | Yes |
| Album | Metadata | Yes |
| Genre | Metadata | Yes |
| Time | Metadata | Yes |
| BPM | Analysis | Yes |
| Key | Analysis | Yes |
| Year | Metadata | Yes |
| Rating | User data | Yes |
| Last Played | User data | Yes |
| Comments | Metadata | Yes |

#### Acceptance Criteria

- [ ] Library displays all tracks from main library
- [ ] Search returns results in <100ms for <10,000 tracks
- [ ] BPM filter shows tracks within ±3% of target
- [ ] Key filter shows harmonically compatible keys
- [ ] Drag-drop provides visual feedback during drag
- [ ] Currently loaded tracks are visually distinct

---

## Metadata & Analysis Pipeline

### Analysis Components

```
┌─────────────────────────────────────────────────────────────────┐
│                      Analysis Pipeline                          │
│                                                                 │
│  ┌──────────┐    ┌──────────┐    ┌──────────┐    ┌──────────┐ │
│  │  Audio   │    │  BPM     │    │   Key    │    │ Waveform │ │
│  │ Decoder  │───►│ Detector │───►│ Detector │───►│Generator │ │
│  └──────────┘    └──────────┘    └──────────┘    └──────────┘ │
│       │                                                │       │
│       │         ┌──────────────────────────────────────┘       │
│       │         │                                              │
│       ▼         ▼                                              │
│  ┌──────────────────┐    ┌──────────────────┐                 │
│  │  Analysis Cache  │    │   SQLite DB      │                 │
│  │  (Binary Files)  │    │ (BPM, Key, Meta) │                 │
│  └──────────────────┘    └──────────────────┘                 │
└─────────────────────────────────────────────────────────────────┘
```

### Analysis Strategies

| Strategy | When | Pros | Cons |
|----------|------|------|------|
| **On Import** | During library scan | Ready when needed | Slows import, wastes work on unplayed tracks |
| **On First Play** | When track loads to deck | Only analyze played tracks | Delay on first load |
| **Background Job** | Idle time after import | No UI blocking | May never complete |
| **On Demand** | User clicks "Analyze" | User control | Manual effort |

**Recommendation:** Hybrid approach:
1. **Quick scan on import**: Duration only (fast)
2. **Background analysis**: BPM, key, waveform during idle
3. **On-demand priority**: Bump priority when loading to deck
4. **Queue management**: Limit concurrent analysis (2-4 tracks)

### BPM Detection

> **IMPLEMENTED:** BPM detection is now client-side using Web Audio API (`lib/bpmDetection.ts`).

**Implementation Details:**

The BPM detection algorithm in `lib/bpmDetection.ts` uses:

1. **Audio decoding:** Web Audio API's `decodeAudioData()`
2. **Low-pass filtering:** BiquadFilterNode at 150Hz to isolate bass/kick
3. **Peak detection:** Find energy peaks in filtered audio
4. **Interval analysis:** Build histogram of intervals between peaks
5. **BPM calculation:** Most common interval converted to BPM
6. **Normalization:** Constrain to 70-170 BPM range (half/double if outside)

**Accuracy Targets:**
- Within ±1 BPM for 4/4 electronic music
- Within ±2 BPM for complex rhythms
- Confidence score for ambiguous cases

**Beat Grid Generation:**
- `generateBeatGrid(bpm, trackDuration)` creates array of beat timestamps
- Used for visual grid display on waveform
- Enables future auto-sync features

### Key Detection

**Algorithm:** Krumhansl-Schmuckler key-finding algorithm or similar.

**Libraries:**
- `essentia` (C++, comprehensive)
- `librosa` (Python, could pre-process)
- Custom FFT + pitch class histogram (Go native)

**Output:** Camelot wheel notation and standard notation:
- `8A` / `Am`
- `11B` / `G`

### Waveform Generation

**Hybrid Approach (Server + Client Fallback):**

The waveform generation uses a hybrid strategy to support all audio formats:

1. **Server-side (Go backend):**
   - MP3 files decoded using `go-mp3` library
   - Fast, cached in SQLite for reuse
   - Returns peak data at ~1200 samples resolution

2. **Client-side fallback (Web Audio API):**
   - Used for OGG/Vorbis, FLAC, WAV, AAC/M4A formats
   - Leverages browser's built-in audio decoders
   - Generated on-demand when server returns error
   - See `lib/clientWaveform.ts`

**Client-side Process:**
1. Fetch audio file via `/api/audio/{id}`
2. Decode using `AudioContext.decodeAudioData()`
3. Mix stereo to mono (if needed)
4. Compute RMS/peak values in windows
5. Normalize to 0-1 range
6. Return 1200 peak values

**Storage Format:**
- Binary blob in SQLite or separate `.waveform` files
- Compression: zstd or gzip (peaks are highly compressible)
- Resolution: ~1000 peaks per minute of audio

**Size Estimate:**
- 5-minute track at 1000 peaks/min = 5000 peaks
- 2 floats per peak (min/max) × 4 bytes = 40KB
- With 3 frequency bands = 120KB
- Compressed: ~20-40KB per track

### Cache Strategy

```
AppData/ViiB-MediaHub/
├── analysis/
│   ├── waveforms/
│   │   ├── {trackId}.waveform     # Binary peak data
│   │   └── ...
│   └── index.json                  # Analysis metadata
└── viib.db                         # BPM, key in dj_analysis table
```

**Cache Invalidation:**
- File modification time changed → re-analyze
- Analyzer version upgrade → re-analyze
- User-initiated re-analysis

---

## Performance Requirements

### Latency Targets

| Metric | Target | Maximum | Notes |
|--------|--------|---------|-------|
| Audio output latency | <15ms | 25ms | Button to sound |
| Control responsiveness | <10ms | 20ms | Slider to effect |
| Waveform scroll | 60fps | 30fps | During playback |
| Track load time | <500ms | 2s | With cached waveform |
| Analysis (full) | <30s | 60s | Per 5-minute track |

### CPU Usage Targets

| Component | Target | Maximum | Notes |
|-----------|--------|---------|-------|
| Idle (paused) | <1% | 2% | No playback |
| Single deck playback | <5% | 10% | No effects |
| Dual deck + effects | <15% | 25% | Full mixing |
| Waveform rendering | <5% | 10% | WebGL/Canvas |
| Analysis (background) | <20% | 40% | Single track |

### Memory Usage Targets

| Component | Target | Maximum |
|-----------|--------|---------|
| Base application | 100MB | 200MB |
| Per loaded deck | +50MB | +100MB |
| Waveform cache (memory) | +20MB | +50MB |
| Effect buffers | +10MB | +20MB |

### Disk Usage

| Data | Size Estimate |
|------|---------------|
| Analysis per track | ~50KB |
| 10,000 track library | ~500MB analysis data |
| Settings/presets | <1MB |

---

## Assumptions & Risks

### Assumptions

| ID | Assumption | Impact if Wrong |
|----|------------|-----------------|
| A1 | Users have modern hardware (4+ cores, 8GB RAM) | Performance issues on low-end machines |
| A2 | Audio files are local (not network/cloud) | Network latency causes playback issues |
| A3 | Common formats dominate (MP3, FLAC, AAC) | Decoder support gaps |
| A4 | Users accept analysis preprocessing time | Poor UX if analysis blocks workflow |
| A5 | Go audio libraries have acceptable latency | May need C/C++ for audio engine |
| A6 | Single audio output device is sufficient | Multi-output (cue/master) delayed |

### Technical Risks

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| **R1: Audio latency too high** | Medium | High | Prototype early with target hardware; fallback to native audio engine |
| **R2: Time-stretching quality poor** | Medium | Medium | Evaluate multiple libraries; accept quality tradeoffs |
| **R3: Cross-platform audio inconsistencies** | High | Medium | Extensive platform testing; abstract audio backend |
| **R4: BPM detection inaccurate** | Medium | Medium | Allow manual BPM correction; use multiple algorithms |
| **R5: Memory usage too high** | Low | Medium | Stream audio instead of full decode; limit cached waveforms |
| **R6: WebSocket latency spikes** | Low | Medium | Optimize message frequency; batch updates |
| **R7: CGO complicates builds** | Medium | Low | Minimize CGO dependencies; provide pre-built binaries |

### Licensing Risks

| Library | License | Risk | Mitigation |
|---------|---------|------|------------|
| Rubber Band | GPL | Requires commercial license or GPL release | Use SoundTouch (LGPL) for MVP |
| FFmpeg | LGPL/GPL | Complex licensing | Use only LGPL components |
| libmp3lame | LGPL | Must dynamically link | Dynamic linking or alternative |
| Essentia | AGPL | Requires open source | Pre-process analysis or alternative |

### Codec Support Risks

| Format | Support | Risk |
|--------|---------|------|
| MP3 | Excellent | Patent-free since 2017 |
| FLAC | Excellent | Open source, no issues |
| WAV | Excellent | Native support |
| AAC | Good | Licensing complex, use fdkaac |
| WMA | Poor | Windows-only, declining use |
| DSD | None | Specialized format, out of scope |

---

## Implementation Roadmap

### Phase 1: MVP - Basic Two-Deck Mixing

**Duration:** 6-8 weeks

**Goal:** Functional two-deck player with crossfader and basic waveforms

#### Engineering Tasks

| Task | Estimate | Dependencies | Status |
|------|----------|--------------|--------|
| **Frontend: DJ Mode Shell** | | | |
| Create DJ Mode route and layout | 2d | None | ✅ Complete |
| Implement Zustand DJ slice | 2d | Layout | ✅ Complete |
| Add sidebar navigation | 1d | DJ slice | ✅ Complete |
| **Frontend: Deck Components** | | | |
| Build DeckView component | 3d | DJ slice | ✅ Complete |
| Implement transport controls (play, pause, cue) | 2d | DeckView | ✅ Complete |
| Add time display | 1d | DeckView | ✅ Complete |
| Build waveform Canvas renderer | 4d | Waveform API | ✅ Complete |
| Implement waveform seeking | 2d | Canvas renderer | ✅ Complete |
| **Frontend: Mixer Components** | | | |
| Build crossfader control | 1d | DJ slice | ✅ Complete |
| Build volume fader controls | 1d | DJ slice | ✅ Complete |
| Build EQ knobs (3-band) | 2d | DJ slice | ✅ Complete |
| **Frontend: Library Integration** | | | |
| Build DJ library browser | 2d | Existing library | ✅ Complete |
| Add "Load to Deck A/B" actions | 1d | Library browser | ✅ Complete |
| Highlight loaded tracks | 1d | Load actions | ✅ Complete |
| **Frontend: Web Audio Engine** | | | |
| Create DJAudioEngine class | 3d | None | ✅ Complete |
| Implement dual-deck playback | 2d | DJAudioEngine | ✅ Complete |
| Add crossfader mixing (GainNodes) | 1d | Dual-deck | ✅ Complete |
| Implement per-deck volume control | 1d | Dual-deck | ✅ Complete |
| Implement 3-band EQ (BiquadFilters) | 2d | Dual-deck | ✅ Complete |
| Add master limiter (DynamicsCompressor) | 1d | Mixing | ✅ Complete |
| Connect UI to audio engine | 2d | All audio | ✅ Complete |
| **Backend: Waveform Generation** | | | |
| Implement peak extraction algorithm | 2d | Decoder | ✅ Complete |
| Add waveform caching | 1d | Peak extraction | ✅ Complete |
| Create waveform API endpoint | 1d | Caching | ✅ Complete |
| Add hot cue CRUD endpoints | 1d | Caching | ✅ Complete |
| **Integration & Testing** | | | |
| End-to-end integration | 3d | All components | ⬜ Not Started |
| Performance optimization | 2d | Integration | ⬜ Not Started |
| Bug fixing and polish | 3d | Testing | ⬜ Not Started |

**Completed:** ~35 days (UI + Audio Engine + Backend Waveform)  
**Remaining:** ~10 days (Integration & Testing)  
**Total Estimate:** ~45 engineering days

#### Acceptance Criteria (Phase 1)

- [x] Load local audio file to Deck A or B
- [x] Play, pause, and seek independently on each deck
- [x] Crossfader blends audio between decks smoothly
- [x] Volume faders control per-deck levels
- [x] Waveform displays with scrolling playhead
- [ ] Click on waveform seeks to position (not implemented; waveform reserved for scratch drag)
- [x] Library browser allows loading tracks to decks
- [ ] Audio latency <25ms (not measured)
- [ ] Waveform renders at 60fps (not measured)
- [x] Works on Windows (primary)
- [ ] Works on macOS, Linux (not validated)

#### Performance Targets (Phase 1)

| Metric | Target |
|--------|--------|
| Audio latency | <25ms |
| Waveform FPS | 60fps |
| Track load time | <2s (first load), <500ms (cached) |
| CPU (dual playback) | <15% |
| Memory | <300MB |

### Phase 2: BPM + Sync + EQ

**Duration:** 4-6 weeks

**Goal:** Tempo matching and professional EQ controls

**Status:** ✅ MOSTLY COMPLETE (see progress tracking above)

#### Completed Tasks

| Task | Status | Implementation |
|------|--------|----------------|
| **Client-side BPM Detection** | ✅ | `lib/bpmDetection.ts` |
| Implement peak-based BPM algorithm | ✅ | Web Audio API + low-pass filter |
| Auto-detect on track load | ✅ | `useDJAudioEngine.ts` |
| Generate beat grid | ✅ | `generateBeatGrid()` |
| Normalize BPM to 70-170 range | ✅ | `normalizeBPM()` |
| **Frontend: Tempo Controls** | ✅ | |
| Build tempo fader component | ✅ | `DeckView.tsx` |
| Display detected BPM | ✅ | BPM badge in deck view |
| Implement SYNC button | ✅ | Syncs Deck B to Deck A tempo |
| **Frontend: Beat Grid** | ✅ | |
| Display beat grid on waveform | ✅ | `DJWaveform.tsx` |
| **Frontend: Hot Cues** | ✅ | Complete with persistence |
| Hot cue UI (8 slots) | ✅ | `DJHotCues.tsx` |
| Keyboard shortcuts (1-8) | ✅ | Jump to cue |
| Set/delete cue functionality | ✅ | Persisted to backend on change |
| **Frontend: Seeking** | ✅ | |
| Click-to-seek on waveform | ⏳ | Not implemented (reserved for scratch drag) |
| Click-to-seek on progress bar | ✅ | `DeckView.tsx` |
| Waveform zoom (10s view) | ✅ | Centered on playhead |

#### Remaining Tasks

| Task | Estimate | Status |
|------|----------|--------|
| Hot cue persistence (wire UI to backend) | 1d | ✅ Complete |
| Enhanced EQ visualization | 1d | ⏳ Not Started |
| BPM accuracy validation | 1d | ⏳ Not Started |

#### Acceptance Criteria (Phase 2)

- [x] BPM detected and displayed for loaded tracks
- [x] Tempo fader adjusts playback speed ±50%
- [x] SYNC button matches Deck B tempo to Deck A
- [x] Tempo change does not cause audio glitches
- [x] 3-band EQ adjusts frequencies per deck
- [x] EQ "kill" effectively removes that band
- [x] Beat grid displayed on waveform
- [x] Click-to-seek on progress bar
- [x] Waveform shows 10-second window centered on playhead
- [x] Hot cues working (set/jump/delete) in-session
- [ ] Hot cues persisted per track (backend wired)
- [ ] BPM accuracy validated (±1 BPM for 4/4 music)

#### Performance Targets (Phase 2)

| Metric | Target | Status |
|--------|--------|--------|
| Tempo adjustment CPU | <10% per deck | ✅ Met |
| BPM detection time | <5s per track | ✅ Met (client-side) |
| BPM accuracy | ±1 BPM for 4/4 music | ⏳ Needs validation |

### Phase 3: FX + Looping + Key Detection

**Duration:** 5-7 weeks

**Goal:** Creative performance features

#### Engineering Tasks

| Task | Status | Implementation |
|------|--------|----------------|
| **Frontend: FX (Web Audio)** | ✅ Complete | `lib/djAudio.ts`, `components/dj/DJFXPanel.tsx` |
| Filter (LP/HP), Delay, Reverb, Flanger | ✅ Complete | Web Audio nodes per deck |
| **Frontend: Looping (Web Audio + position tracking)** | ✅ Complete | `lib/djAudio.ts`, `components/dj/DJLoopPanel.tsx` |
| Loop in/out, beat-sized loops, double/halve | ✅ Complete | Engine loop state + UI controls |
| **Frontend: Key Detection** | ✅ Complete | `lib/keyDetection.ts` (runs on load) |
| **Backend: Hot Cues persistence endpoints** | ✅ Complete | `backend/internal/api/dj_waveform.go`, `backend/internal/db/db.go` |
| **Frontend: Hot Cues persistence wiring** | ✅ Complete | Auto-loads on track load, auto-saves on change |
| **Library harmonic filter** | ⏳ Future | Not implemented |

**Total Estimate:** ~45 engineering days

#### Acceptance Criteria (Phase 3)

- [x] Filter effect sweeps smoothly across frequency range
- [x] Flanger produces audible modulation effect
- [x] Echo (Delay) with configurable time and feedback
- [x] Effects toggle on/off without clicks or pops
- [x] Loops play seamlessly without audible gap
- [x] Loop length changes in real-time (beat-synced sizes)
- [x] Hot cues trigger within 20ms
- [x] Musical key displayed for analyzed tracks
- [ ] Library filterable by harmonic compatibility (future enhancement)

#### Performance Targets (Phase 3)

| Metric | Target |
|--------|--------|
| Effect CPU (all active) | <10% (not measured) |
| Loop transition | seamless (inaudible) |
| Hot cue trigger | <20ms |
| Key detection accuracy | >80% for major/minor (not measured) |

### Phase 4: Advanced Features

**Duration:** 6-8 weeks

**Goal:** Professional-grade features for serious DJs

#### Features

1. **Beat Grid Editing**
   - Manual beat grid adjustment
   - Downbeat marking
   - BPM tap tempo

2. **Beat-Phase Sync** ✅ IMPLEMENTED
   - Align beats, not just BPM
   - Phase nudge via sync button
   - Sync mode selector (OFF/BPM/PHASE)

3. **Mix Recording**
   - Record master output to file (WAV, FLAC, MP3)
   - Track list metadata embedding
   - Real-time recording indicator

4. **Headphone Cue** ✅ IMPLEMENTED
   - Route individual decks to headphone bus
   - Cue/master mix slider
   - Per-deck CUE buttons in mixer

5. **Performance Enhancements**
   - Slip mode (silent continue)
   - Key lock (pitch independent of tempo)
   - Vinyl brake effect

#### Acceptance Criteria (Phase 4)

- [ ] Beat grid can be manually adjusted
- [x] Sync aligns beat phase (±50ms) - Phase sync algorithm implemented
- [ ] Mix recording captures full session to file
- [x] Headphone cue routes decks to separate bus - Implemented (requires hardware splitter)
- [ ] Key lock maintains pitch while tempo changes
- [ ] All features work cross-platform

---

## Testing Strategy

### Unit Testing

| Component | Framework | Coverage Target |
|-----------|-----------|-----------------|
| Go audio DSP | `testing` | 80% |
| Go API handlers | `testing` + `httptest` | 85% |
| React components | Vitest + Testing Library | 70% |
| Zustand stores | Vitest | 90% |

### Integration Testing

| Test Area | Approach |
|-----------|----------|
| REST API communication | Mock backend / real backend (waveform/hotcues/personas) |
| Audio pipeline | End-to-end with test audio files |
| State synchronization | Simulate user interactions |
| Multi-deck operations | Concurrent operations testing |

### Audio Regression Testing

**Approach:**
1. Create reference audio outputs for known inputs
2. Compare output to reference (waveform diff)
3. Tolerance for minor variations (float precision)

**Test Cases:**
- Crossfader positions produce expected mix
- EQ settings match expected frequency response
- Effects produce expected output
- Time-stretch maintains audio quality

### Performance Testing

| Test | Tool | Metric |
|------|------|--------|
| Latency | Custom measurement | Button-to-sound time |
| CPU profiling | `pprof` (Go), Chrome DevTools | CPU % by component |
| Memory profiling | `pprof`, Chrome DevTools | Memory allocation |
| Frame rate | Chrome DevTools | Waveform FPS |
| Load testing | Custom scripts | Track load times |

### Platform Testing Matrix

| Platform | Priority | CI | Manual |
|----------|----------|-----|--------|
| Windows 11 | P0 | ✓ | ✓ |
| Windows 10 | P1 | ✓ | ✓ |
| macOS 14 (ARM) | P1 | – | – |
| macOS 13 (Intel) | P2 | – | – |
| Ubuntu 22.04 | P2 | – | – |
| Fedora 39 | P3 | – | – |

### User Acceptance Testing

**Test Scenarios:**
1. **Basic Mix Session**: Load two tracks, crossfade, adjust EQ
2. **BPM Matching**: Sync tracks with different BPMs
3. **Effect Usage**: Apply effects during transition
4. **Loop Performance**: Set and manipulate loops
5. **Library Workflow**: Search, filter, load tracks efficiently
6. **Long Session**: 2+ hour mix without issues

---

## Appendix

### A. Glossary

| Term | Definition |
|------|------------|
| **BPM** | Beats Per Minute - tempo measurement |
| **Cue Point** | Saved position in a track for quick return |
| **Crossfader** | Control that blends audio between two sources |
| **Deck** | Virtual turntable/player for a single track |
| **EQ** | Equalizer - frequency adjustment |
| **Hot Cue** | Instantly triggerable saved position |
| **Key** | Musical key (e.g., Am, C, F#m) |
| **Loop** | Repeated section of audio |
| **Sync** | Automatic tempo matching |
| **Time-stretch** | Changing tempo without changing pitch |
| **Waveform** | Visual representation of audio amplitude |

### B. Harmonic Mixing Reference (Camelot Wheel)

```
        1A  1B
      /        \
   12A  12B  2A  2B
     |        |
   11A  11B  3A  3B
     |        |
   10A  10B  4A  4B
     |        |
    9A  9B  5A  5B
      \        /
        8A  8B
        /    \
      7A  7B  6A  6B
```

**Compatible keys (for harmonic mixing):**
- Same position (e.g., 8A ↔ 8A)
- Adjacent positions (e.g., 8A ↔ 7A, 8A ↔ 9A)
- Relative major/minor (e.g., 8A ↔ 8B)

### C. Reference DJ Software

| Software | Platform | Notable Features |
|----------|----------|------------------|
| Traktor | Win/Mac | 4 decks, Remix decks, Stems |
| Serato DJ | Win/Mac | Streaming integration, DVS |
| rekordbox | Win/Mac | Hardware integration, cloud |
| VirtualDJ | Win/Mac | AI mixing, video |
| Mixxx | All | Open source, extensive features |
| djay | All | AI, Spotify integration |

### D. Audio Library Evaluation

| Library | Language | License | Platforms | Notes |
|---------|----------|---------|-----------|-------|
| PortAudio | C | MIT | All | Mature, complex |
| miniaudio | C | Public Domain | All | Single-header, modern |
| Oto | Go | BSD | All | Simple, Go-native |
| Beep | Go | MIT | All | High-level, may have latency |
| SDL2 Audio | C | zlib | All | Part of SDL, stable |

### E. File Format Support Priority

| Format | Extension | Priority | Decoder |
|--------|-----------|----------|---------|
| MP3 | .mp3 | P0 | minimp3, mpg123 |
| FLAC | .flac | P0 | libflac |
| WAV | .wav | P0 | Native |
| AIFF | .aiff, .aif | P1 | Native |
| AAC | .m4a, .aac | P1 | fdkaac |
| OGG Vorbis | .ogg | P1 | libvorbis |
| ALAC | .m4a | P2 | FFmpeg |
| Opus | .opus | P2 | libopus |

---

## Document History

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2026-01-25 | AI Assistant | Initial planning document |
| 1.1 | 2026-01-25 | AI Assistant | Updated architecture to Web Audio API (frontend-based) |
| 1.2 | 2026-01-25 | AI Assistant | Updated Phase 1 roadmap with progress tracking; replaced WebSocket commands with Zustand actions; updated API spec for REST-only backend |
| 1.3 | 2026-01-25 | AI Assistant | Completed backend waveform API (`dj_waveform.go`), DB schema (`dj_waveform_cache`, `dj_hot_cues`), and route registration. Fixed audio engine integration in DeckView, DJMixer, DJLibraryBrowser, and DJMode to use useDJAudioEngine hook for proper audio control. Phase 1 core implementation complete. |
| 2.0 | 2026-01-25 | AI Assistant | Phase 2 implementation: BPM detection (client-side), tempo fader, SYNC button, beat grid on waveform, hot cue UI, OGG waveform support via client-side fallback |
| 2.1 | 2026-01-25 | AI Assistant | Fixed waveform zoom (10s visible), added click-to-seek on progress bar, added debug logging to hot cues, updated documentation for accuracy |
| 2.2 | 2026-01-25 | AI Assistant | Added vinyl scratch/jog wheel feature: drag waveform to scratch with velocity-based audio feedback and momentum effect on release |