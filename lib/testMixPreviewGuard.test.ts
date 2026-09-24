import { describe, expect, it } from 'vitest';
import type { DeckState } from '../slices/djMixerSlice';
import { hasSeparateHeadphoneRoute, isPreviewDeckOffAir, isPristineEmptyPreviewDeck, stillOwnsPreviewDeck } from './testMixPreviewGuard';

function deck(overrides: Partial<DeckState> = {}): DeckState {
  return {
    track: null, analysisStatus: 'not_analyzed', isPlaying: false, position: 0, duration: 0, cuePoint: 0,
    volume: 0.75, eq: { low: 0, mid: 0, high: 0 }, filter: { enabled: false, value: 0 }, tempo: 1,
    originalBpm: null, effectiveBpm: null, key: null, waveformPeaks: null, beatGrid: null, downbeatIndices: null,
    beatGridLocked: false, beatGridSource: 'unknown', bpmConfidence: null, tempoEvidence: null, beatGridOffset: 0,
    loop: { enabled: false, start: 0, end: 0, pendingIn: null }, hotCues: [],
    fx: {
      filter: { enabled: false, type: 'lowpass', frequency: 1000, resonance: 1 },
      delay: { enabled: false, time: 0.375, feedback: 0.3, mix: 0.3 },
      reverb: { enabled: false, roomSize: 0.5, damping: 0.5, mix: 0.3 },
      flanger: { enabled: false, rate: 0.5, depth: 0.5, feedback: 0.3 },
    },
    cueEnabled: false,
    ...overrides,
  } as DeckState;
}

describe('Test Mix preview guards', () => {
  it('allows only the opposite deck at the fully off-air crossfader endpoint and a dedicated headphone device', () => {
    expect(isPreviewDeckOffAir('A', 1)).toBe(true);
    expect(isPreviewDeckOffAir('A', 0.97)).toBe(false);
    expect(isPreviewDeckOffAir('B', -1)).toBe(true);
    expect(isPreviewDeckOffAir('B', -0.97)).toBe(false);
    expect(hasSeparateHeadphoneRoute('headphones-1', 'speakers-1')).toBe(true);
    expect(hasSeparateHeadphoneRoute('', 'speakers-1')).toBe(false);
    expect(hasSeparateHeadphoneRoute('default', 'default')).toBe(false);
    expect(hasSeparateHeadphoneRoute('speakers-1', 'speakers-1')).toBe(false);
  });

  it('accepts only an empty, pristine deck for Test Mix audition', () => {
    expect(isPristineEmptyPreviewDeck(deck({ volume: 0.4, eq: { low: -2, mid: 1, high: 0 } }))).toBe(true);
    expect(isPristineEmptyPreviewDeck(deck({ loop: { enabled: false, start: 0, end: 0, pendingIn: 0 } }))).toBe(false);
    expect(isPristineEmptyPreviewDeck(deck({ track: { id: 'occupied' } as DeckState['track'] }))).toBe(false);
  });

  it('detects user changes before cleanup so preview does not overwrite their deck', () => {
    const baseline = { volume: 0.75, eq: { low: 0, mid: 0, high: 0 } };
    const previewDeck = deck({ track: { id: 'candidate' } as DeckState['track'], isPlaying: true, cueEnabled: true, duration: 120, position: 8 });
    expect(stillOwnsPreviewDeck(previewDeck, 'candidate', baseline)).toBe(true);
    expect(stillOwnsPreviewDeck(deck({ ...previewDeck, volume: 0.5 }), 'candidate', baseline)).toBe(false);
    expect(stillOwnsPreviewDeck(deck({ ...previewDeck, track: { id: 'user-track' } as DeckState['track'] }), 'candidate', baseline)).toBe(false);
    expect(stillOwnsPreviewDeck(deck({ ...previewDeck, tempo: 1.05 }), 'candidate', baseline)).toBe(false);
  });
});
