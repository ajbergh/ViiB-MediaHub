import { describe, expect, it } from 'vitest';
import type { DeckState } from '../slices/djMixerSlice';
import { hasSeparateHeadphoneRoute, isPreviewDeckOffAir, isPristineEmptyPreviewDeck, isRestorableOccupiedPreviewDeck, stillOwnsOccupiedPreviewBaseline, stillOwnsPreviewDeck, stillOwnsPreviewRoute, stillOwnsPreviewTransport } from './testMixPreviewGuard';

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
  it('allows only the opposite deck at the exact off-air endpoint and a dedicated headphone device', () => {
    expect(isPreviewDeckOffAir('A', 1)).toBe(true);
    expect(isPreviewDeckOffAir('A', 0.98)).toBe(false);
    expect(isPreviewDeckOffAir('A', -1)).toBe(false);
    expect(isPreviewDeckOffAir('B', -1)).toBe(true);
    expect(isPreviewDeckOffAir('B', -0.98)).toBe(false);
    expect(isPreviewDeckOffAir('B', 1)).toBe(false);
    expect(hasSeparateHeadphoneRoute('headphones-1', 'speakers-1')).toBe(true);
    expect(hasSeparateHeadphoneRoute('', 'speakers-1')).toBe(false);
    expect(hasSeparateHeadphoneRoute('default', 'default')).toBe(false);
    expect(hasSeparateHeadphoneRoute('speakers-1', 'speakers-1')).toBe(false);
  });

  it('keeps cleanup ownership only while the preview route remains unchanged and off-air', () => {
    const route = {
      deck: 'B' as const, crossfader: -1, startedAtCrossfader: -1,
      masterCueEnabled: false, autoGainEnabled: false, keyLockEnabled: true, startedAtKeyLock: true,
      headphoneDeviceId: 'headphones-1', startedAtHeadphoneDeviceId: 'headphones-1',
      masterDeviceId: 'speakers-1', startedAtMasterDeviceId: 'speakers-1',
    };
    expect(stillOwnsPreviewRoute(route)).toBe(true);
    expect(stillOwnsPreviewRoute({ ...route, crossfader: 0 })).toBe(false);
    expect(stillOwnsPreviewRoute({ ...route, masterCueEnabled: true })).toBe(false);
    expect(stillOwnsPreviewRoute({ ...route, autoGainEnabled: true })).toBe(false);
    expect(stillOwnsPreviewRoute({ ...route, keyLockEnabled: false })).toBe(false);
    expect(stillOwnsPreviewRoute({ ...route, headphoneDeviceId: 'headphones-2' })).toBe(false);
    expect(stillOwnsPreviewRoute({ ...route, masterDeviceId: 'speakers-2' })).toBe(false);
  });

  it('accepts only an empty, pristine deck for Test Mix audition', () => {
    expect(isPristineEmptyPreviewDeck(deck({ volume: 0.4, eq: { low: -2, mid: 1, high: 0 } }))).toBe(true);
    expect(isPristineEmptyPreviewDeck(deck({ loop: { enabled: false, start: 0, end: 0, pendingIn: 0 } }))).toBe(false);
    expect(isPristineEmptyPreviewDeck(deck({ track: { id: 'occupied' } as DeckState['track'] }))).toBe(false);
  });

  it('accepts occupied preview only for the verified loaded source and keeps ownership tied to controls', () => {
    const original = deck({ track: { id: 'original' } as DeckState['track'], isPlaying: true, position: 23, tempo: 1.04,
      cuePoint: 4, loop: { enabled: true, start: 8, end: 12, pendingIn: null },
      fx: { ...deck().fx, delay: { enabled: true, time: .25, feedback: .4, mix: .2 } } });
    expect(isRestorableOccupiedPreviewDeck(original, 'original')).toBe(true);
    expect(isRestorableOccupiedPreviewDeck(original, 'different-source')).toBe(false);
    expect(isRestorableOccupiedPreviewDeck({ ...original, cueEnabled: true }, 'original')).toBe(false);

    expect(stillOwnsOccupiedPreviewBaseline(original, original)).toBe(true);
    expect(stillOwnsOccupiedPreviewBaseline({ ...original, volume: .5 }, original)).toBe(false);
    expect(stillOwnsOccupiedPreviewBaseline({ ...original, loop: { ...original.loop, end: 16 } }, original)).toBe(false);
    expect(stillOwnsOccupiedPreviewBaseline({ ...original, fx: { ...original.fx, delay: { ...original.fx.delay, mix: .8 } } }, original)).toBe(false);
  });

  it('detects user changes before cleanup so preview does not overwrite their deck', () => {
    const baseline = { volume: 0.75, eq: { low: 0, mid: 0, high: 0 } };
    const previewDeck = deck({ track: { id: 'candidate' } as DeckState['track'], isPlaying: true, cueEnabled: true, duration: 120, position: 8 });
    expect(stillOwnsPreviewDeck(previewDeck, 'candidate', baseline)).toBe(true);
    expect(stillOwnsPreviewDeck(deck({ ...previewDeck, volume: 0.5 }), 'candidate', baseline)).toBe(false);
    expect(stillOwnsPreviewDeck(deck({ ...previewDeck, track: { id: 'user-track' } as DeckState['track'] }), 'candidate', baseline)).toBe(false);
    expect(stillOwnsPreviewDeck(deck({ ...previewDeck, tempo: 1.05 }), 'candidate', baseline)).toBe(false);
  });

  it('retains preview ownership only while no explicit transport command has changed', () => {
    expect(stillOwnsPreviewTransport(12, 12)).toBe(true);
    expect(stillOwnsPreviewTransport(12, 13)).toBe(false);
  });
});
