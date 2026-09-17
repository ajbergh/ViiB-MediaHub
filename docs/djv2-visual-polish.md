# DJv2 visual polish

The default Racks + FX layout now includes a compact whole-track waveform on each deck. The overview uses real peak data, numbered hot cues and a white playhead; it shows an explicit empty/preparing state when data is unavailable. It is a visual overview, not a seek control.

Deck headers prioritize the title, BPM and remaining time. Essential labels have higher contrast and a 12px reference size inside the proportionally scaled canvas. Deck B's jog now matches the purple mixer/transport accent.

The upper mixer follows its content height, with reduced spacing before sync, crossfader and headphone controls. FX racks are shorter, values use Hz, milliseconds, percentages and resonance Q, and zero-valued parameters no longer substitute a default. The filter knobs use the inverse of their existing frequency/resonance mapping for consistent indicator positions.

Channel and master volume readouts show dB of gain. Channel faders mark unity, half amplitude (approximately −6 dB) and silence. VU references remain normalized 0–1 to match the existing engine signal; a red cap holds for 1.2 seconds when the reported level reaches full scale. These are not calibrated loudness meters.

Edit Grid expands an anchored panel beside the loop/beat-jump controls. The Library affordance is taller and more prominent. Layout scaling still applies to the complete design canvas.

Validation: `npm run typecheck`, `npm run build`, and `node scripts/dj-overlay-audit.mjs`. The browser audit covers empty and loaded decks at 1470×825, 1920×1080, 2560×1440 and 3840×2160, grid disclosure, all three display modes, library behavior and playback continuity. Screenshots are written to `output/playwright/dj-overlay/`.
