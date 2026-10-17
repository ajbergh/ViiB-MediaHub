import { describe, expect, it } from 'vitest';
import { beatPhaseCorrection, crossfaderGains, syncUnavailableReason } from './djMixerControls';
import type { DeckState, MixerState } from '../slices/djMixerSlice';

describe('audible crossfader curves', () => {
  it.each(['linear', 'constant-power', 'sharp'] as const)('%s mutes the opposite deck at either endpoint', curve => {
    expect(crossfaderGains(-1, curve).a).toBe(1);
    expect(crossfaderGains(-1, curve).b).toBe(0);
    expect(crossfaderGains(1, curve).a).toBeCloseTo(0);
    expect(crossfaderGains(1, curve).b).toBe(1);
  });
  it('distinguishes linear, constant-power and cut at center', () => {
    expect(crossfaderGains(0, 'linear')).toEqual({ a: 0.5, b: 0.5 });
    const cp = crossfaderGains(0, 'constant-power');
    expect(cp.a).toBeCloseTo(Math.SQRT1_2); expect(cp.b).toBeCloseTo(Math.SQRT1_2);
    expect(crossfaderGains(0, 'sharp')).toEqual({ a: 1, b: 1 });
    expect(crossfaderGains(-0.95, 'sharp').b).toBeCloseTo(0.5);
    expect(crossfaderGains(0.95, 'sharp').a).toBeCloseTo(0.5);
  });
  it('maintains constant power and bounded monotonic gains across the full travel', () => {
    for (let step = 0; step <= 100; step++) {
      const position = -1 + step / 50;
      const cp = crossfaderGains(position, 'constant-power');
      expect(cp.a ** 2 + cp.b ** 2).toBeCloseTo(1);
      for (const curve of ['linear', 'constant-power', 'sharp'] as const) {
        const current = crossfaderGains(position, curve), next = crossfaderGains(position + 0.01, curve);
        expect(next.a).toBeLessThanOrEqual(current.a); expect(next.b).toBeGreaterThanOrEqual(current.b);
        expect(current.a).toBeGreaterThanOrEqual(0); expect(current.b).toBeLessThanOrEqual(1);
      }
    }
  });
});

describe('deck sync prerequisites', () => {
  const deck = () => ({ track: { id: 'track' }, originalBpm: 120, effectiveBpm: 128,
    beatGrid: [0, 0.5, 1], beatGridOffset: 0, beatGridSource: 'manual', beatGridLocked: true } as DeckState);
  const state = () => ({ djDeckA: deck(), djDeckB: deck(), djMixer: { syncMode: 'bpm' } as MixerState });
  it('enables BPM with tracks and valid timing, and PHASE with reviewed grids', () => {
    const s = state(); expect(syncUnavailableReason(s, 'A')).toBeNull();
    s.djMixer.syncMode = 'beat-phase'; expect(syncUnavailableReason(s, 'B')).toBeNull();
    s.djDeckB.beatGridLocked = false; expect(syncUnavailableReason(s, 'A')).toContain('lock both');
  });
  it('blocks OFF, missing tracks, unknown BPM, and impossible tempo matching', () => {
    const s = state(); s.djMixer.syncMode = 'off'; expect(syncUnavailableReason(s, 'A')).toContain('disabled');
    s.djMixer.syncMode = 'bpm'; s.djDeckB.track = null; expect(syncUnavailableReason(s, 'A')).toContain('both decks');
    s.djDeckB = deck(); s.djDeckB.effectiveBpm = NaN; expect(syncUnavailableReason(s, 'A')).toContain('BPM analysis');
    s.djDeckB.effectiveBpm = 200; expect(syncUnavailableReason(s, 'A')).toContain('tempo range');
  });
});

describe('beat-phase alignment', () => {
  const deck = { beatGrid: [0, 0.5, 1], beatGridOffset: 0 };
  it('takes the short route across beat boundaries instead of refusing to align', () => {
    expect(beatPhaseCorrection(deck, deck, 0.05, 0.45)).toBeCloseTo(-0.1);
    expect(beatPhaseCorrection(deck, deck, 0.45, 0.05)).toBeCloseTo(0.1);
  });
  it('includes each deck’s reviewed grid offset', () => {
    expect(beatPhaseCorrection({ ...deck, beatGridOffset: 0.1 }, { ...deck, beatGridOffset: 0.2 }, 0.3, 0.4)).toBeCloseTo(0);
    expect(beatPhaseCorrection({ ...deck, beatGridOffset: 0.1 }, deck, 0.3, 0.3)).toBeCloseTo(0.1);
  });
  it('extrapolates the edge interval outside a finite grid', () => {
    expect(beatPhaseCorrection(deck, deck, 30.1, 40.2)).toBeCloseTo(0.1);
    expect(beatPhaseCorrection({ ...deck, beatGridOffset: 1 }, deck, 0.2, 0.3)).toBeCloseTo(0.1);
  });
});
