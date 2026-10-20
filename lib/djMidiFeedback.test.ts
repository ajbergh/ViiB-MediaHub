import { describe, expect, it } from 'vitest';
import { buildDeckMidiFeedback, deckBeatPulse, mappedPadLEDValue, pulseDeckMidiFeedback, type DeckFeedback } from './djMidiFeedback';

function fixture(): DeckFeedback {
  return {
    deck: { track: { id: 'track' } as any, isPlaying: false, cueEnabled: false,
      originalBpm: 120, effectiveBpm: 120, hotCues: [{ slot: 2, position: 10, color: 'red' }],
      loop: { enabled: false, start: 0, end: 0 } },
    mode: 0, samples: [], cuePressed: false, syncPressed: false,
  };
}
const values = (feedback: DeckFeedback) => buildDeckMidiFeedback('A', feedback).slice(4, 8).map(message => message[2]);

const clock = () => ({ ...fixture().deck, position: 0, isPlaying: true, tempo: 1,
  beatGrid: null as number[] | null, beatGridOffset: 0, beatGridSource: 'unknown' as const, beatGridLocked: false });

describe('per-deck beat feedback', () => {
  it('follows each deck independently and immediately realigns after seeks and loop wraps', () => {
    const a = clock(); const b = { ...clock(), originalBpm: 90, effectiveBpm: 90, position: 1 / 3 };
    expect(deckBeatPulse(a)).toBe(true);
    expect(deckBeatPulse(b)).toBe(false);
    a.position = 0.3;
    b.position = 2 / 3;
    expect(deckBeatPulse(a)).toBe(false);
    expect(deckBeatPulse(b)).toBe(true);
    a.position = 8; expect(deckBeatPulse(a)).toBe(true);
    a.position = 2; expect(deckBeatPulse(a)).toBe(true);
  });

  it('does not apply tempo twice to source-track position', () => {
    const state = { ...clock(), position: 0.5, tempo: 1.5, effectiveBpm: 180 };
    expect(deckBeatPulse(state)).toBe(true);
    state.position = 0.75;
    expect(deckBeatPulse(state)).toBe(false);
    expect(deckBeatPulse({ ...state, originalBpm: null })).toBe(false);
  });

  it('uses reviewed grid intervals and offsets, including beyond finite grid edges', () => {
    const state = { ...clock(), beatGrid: [1, 1.5, 2.25], beatGridSource: 'manual' as const,
      beatGridLocked: true, beatGridOffset: 0.1, position: 1.6 };
    expect(deckBeatPulse(state)).toBe(true);
    state.position = 1.9; expect(deckBeatPulse(state)).toBe(false);
    state.position = 3.1; expect(deckBeatPulse(state)).toBe(true);
    state.position = 0.6; expect(deckBeatPulse(state)).toBe(true);
    // Unreviewed or malformed grids fall back to the source BPM.
    expect(deckBeatPulse({ ...state, position: 0.5, beatGridLocked: false })).toBe(false);
    expect(deckBeatPulse({ ...state, position: 0.5, beatGrid: [1, 1] })).toBe(false);
  });

  it('preserves pad meanings, other LEDs, and steady feedback when playback has no clock', () => {
    const state = { ...clock(), position: 0.3 };
    const messages = [[0x94, 0x14, 127], [0x94, 0x15, 0], [0x94, 0x16, 1],
      [0x94, 1, 127], [0x90, 0, 127], [0x95, 0x14, 127]] as const;
    const input = messages.map(message => [...message] as [number, number, number]);
    expect(pulseDeckMidiFeedback('A', state, input)).toEqual([
      [0x94, 0x14, 1], ...messages.slice(1),
    ]);
    expect(input[0][2]).toBe(127);
    for (const unavailable of [
      { ...state, isPlaying: false }, { ...state, track: null },
      { ...state, originalBpm: null, effectiveBpm: null }, { ...state, position: NaN },
    ]) {
      expect(deckBeatPulse(unavailable)).toBeNull();
      expect(pulseDeckMidiFeedback('A', unavailable, input)).toEqual(input);
    }
    expect(pulseDeckMidiFeedback('B', state, input).at(-1)).toEqual([0x95, 0x14, 1]);
  });
});

describe('MixTrack control LED state', () => {
  it.each(['A', 'B'] as const)('lights dedicated stem presets from the live mix on deck %s in every pad bank', deck => {
    const feedback = fixture();
    feedback.stems = {
      status: { available: true, mode: 'stems' } as any,
      state: {
        vocals: { muted: false, solo: false, gain: 1 }, music: { muted: true, solo: false, gain: 1 },
        bass: { muted: true, solo: false, gain: 1 }, drums: { muted: true, solo: false, gain: 1 },
      },
    };
    const status = deck === 'A' ? 0x90 : 0x91;
    const leds = () => buildDeckMidiFeedback(deck, feedback).slice(-2);
    for (const mode of [0, 1, 2, 3] as const) {
      feedback.mode = mode;
      expect(leds()).toEqual([[status, 0x46, 127], [status, 0x48, 0]]);
    }
    feedback.stems.state.vocals.muted = true;
    for (const bus of ['music', 'bass', 'drums'] as const) feedback.stems.state[bus].muted = false;
    expect(leds()).toEqual([[status, 0x46, 0], [status, 0x48, 127]]);
    feedback.stems.state.vocals.muted = false; // Full mix / preset off.
    expect(leds().map(message => message[2])).toEqual([0, 0]);
    feedback.stems.state.vocals.solo = true; // UI solo makes the mix vocals-only.
    expect(leds().map(message => message[2])).toEqual([127, 0]);
    feedback.stems.state.vocals.gain = 0; // Silence is not an active preset.
    expect(leds().map(message => message[2])).toEqual([0, 0]);
    feedback.stems.state.vocals.gain = 1;
    feedback.stems.status.available = false;
    expect(leds().map(message => message[2])).toEqual([0, 0]);
    feedback.stems.status.available = true;
    feedback.stems.status.mode = 'fallback';
    expect(leds().map(message => message[2])).toEqual([0, 0]);
    feedback.stems.status.mode = 'stems';
    feedback.deck.track = null;
    expect(leds().map(message => message[2])).toEqual([0, 0]);
  });
  it('follows the target cue/sample slot when a physical pad is remapped', () => {
    const decks = { A: fixture(), B: fixture() };
    decks.B.samples = [{ url: 'sample', isPlaying: true }];
    expect(mappedPadLEDValue('deckA.hotCue2', decks)).toBe(127);
    expect(mappedPadLEDValue('deckA.hotCue1', decks)).toBe(0);
    expect(mappedPadLEDValue('sampler.pad5', decks)).toBe(127);
    expect(mappedPadLEDValue('deckA.performancePad4', decks)).toBe(0);
  });
  it('uses authoritative transport and PFL state, with held Cue/Sync indications', () => {
    const feedback = fixture();
    feedback.deck.isPlaying = true;
    feedback.deck.cueEnabled = true;
    feedback.cuePressed = true;
    feedback.syncPressed = true;
    expect(buildDeckMidiFeedback('B', feedback).slice(0, 4)).toEqual([
      [0x91, 0, 127], [0x91, 1, 127], [0x91, 2, 127], [0x81, 0x1B, 127],
    ]);
    feedback.deck.isPlaying = false;
    feedback.deck.cueEnabled = false;
    feedback.cuePressed = false;
    feedback.syncPressed = false;
    expect(buildDeckMidiFeedback('A', feedback).slice(0, 4)).toEqual([
      [0x90, 0, 1], [0x90, 1, 1], [0x90, 2, 1], [0x80, 0x1B, 1],
    ]);
  });

  it('shows saved cue slots, and clears them when a deck is unloaded', () => {
    const feedback = fixture();
    expect(values(feedback)).toEqual([0, 127, 0, 0]);
    feedback.deck.track = null;
    expect(values(feedback)).toEqual([0, 0, 0, 0]);
  });

  it('shows only the matching active loop, clearing unsupported lengths', () => {
    const feedback = fixture(); feedback.mode = 1;
    feedback.deck.loop = { enabled: true, start: 0, end: 2 };
    expect(values(feedback)).toEqual([0, 0, 127, 0]); // 4 beats at 120 BPM.
    feedback.deck.effectiveBpm = 150; // Pitch changes playback rate, not source loop boundaries.
    expect(values(feedback)).toEqual([0, 0, 127, 0]);
    feedback.deck.loop.enabled = false;
    expect(values(feedback)).toEqual([0, 0, 0, 0]);
    feedback.deck.loop.enabled = true;
    feedback.deck.loop.end = 1.5;
    expect(values(feedback)).toEqual([0, 0, 0, 0]); // 3 beats is not a pad preset.
  });

  it('keeps the requested loop pad lit when quantization or clipping changes its duration', () => {
    const feedback = fixture(); feedback.mode = 1;
    feedback.deck.loop = { enabled: true, start: 10, end: 10.3, presetBeats: 8 };
    expect(values(feedback)).toEqual([0, 0, 0, 127]);
    feedback.deck.loop.enabled = false;
    expect(values(feedback)).toEqual([0, 0, 0, 0]);
  });

  it('distinguishes empty, loaded and playing sampler pads', () => {
    const feedback = fixture(); feedback.mode = 2;
    feedback.samples = [{ url: null, isPlaying: false }, { url: 'sample', isPlaying: false }, { url: 'sample', isPlaying: true }];
    expect(values(feedback)).toEqual([0, 1, 127, 0]);
    feedback.samples[2].isPlaying = false;
    expect(values(feedback)).toEqual([0, 1, 1, 0]);
  });

  it('reflects stem audibility including mute, solo, zero gain and unavailable stems', () => {
    const feedback = fixture(); feedback.mode = 3;
    feedback.stems = {
      status: { available: true, mode: 'stems' } as any,
      state: {
        vocals: { muted: false, solo: false, gain: 1 }, music: { muted: true, solo: false, gain: 1 },
        bass: { muted: false, solo: false, gain: 0 }, drums: { muted: false, solo: false, gain: 1 },
      },
    };
    expect(values(feedback)).toEqual([127, 1, 1, 127]);
    feedback.stems.state.drums.solo = true;
    expect(values(feedback)).toEqual([1, 1, 1, 127]);
    feedback.stems.status.mode = 'full';
    expect(values(feedback)).toEqual([0, 0, 0, 0]);
  });
});
