# Mixer control behavior review

## Controls

| Button | Actual behavior |
| --- | --- |
| OFF | Disables subsequent deck SYNC actions, including keyboard/MIDI. Does not reset the current tempo. |
| BPM | Selects one-shot tempo matching. Press SYNC on the deck that should follow the other deck. Does not continuously follow future tempo changes. |
| PHASE | Selects BPM matching plus beat-phase alignment when deck SYNC is pressed. Both decks need valid, reviewed, locked manual grids. This aligns beats, not phrase/downbeat structure. |
| Q | Toggles beat-loop button quantization to a reviewed, locked manual grid. Without one, loops start at the playhead. Does not quantize hot-cue triggering, manual loop IN/OUT, or the cue editor (which has its own quantization selector). |
| LIN | Linear crossfade: A/B each have gain 0.5 at center. |
| CP | Constant-power crossfade: A/B each have gain approximately 0.707 at center. |
| CUT | Sharp curve: both channels have full gain through the middle; each fades over the outermost 5% of travel. |

The center sync row selects a mode; selecting BPM or PHASE does not itself
seek or change either deck's tempo. Tooltips now state this explicitly.

## Defects corrected

- The curve setting previously only changed store state. The engine always used
  constant power. It now applies the selected gain law, including immediately
  when the curve changes without moving the crossfader and when restoring settings.
- Keyboard SYNC bypassed OFF. UI, keyboard, and MIDI now share prerequisite
  checks for mode, loaded tracks, BPM, supported tempo range, and reviewed grids.
- A deck SYNC button could be enabled without usable BPM on both decks. It now
  disables with a reason in its tooltip, and its active indicator follows readiness.
- Engine PHASE sync ignored grid offsets, used stale source position, and could
  skip alignment across a beat boundary. It now reads live transport position,
  accounts for both offsets, and takes the shortest correction within half a beat.
- Q's old tooltip implied global action quantization. It now describes the
  existing beat-loop scope instead.

## Verification

Regression tests cover gain-node values for all curves, curve changes without
fader movement, monotonic gain laws/endpoints, BPM/PHASE eligibility and dispatch,
grid offsets and boundary wrapping, and Q-on/Q-off loop placement. The full
frontend check passed with 580 tests across 133 files, TypeScript, style checks
and production build. Playwright verified all seven controls in the production
preview, their tooltips/selected states, and disabled deck SYNC indicators with
no loaded tracks. There were no page exceptions. The development server stalled
loading modules, so browser validation used port 4173. No listening test or
native hardware qualification is claimed.
