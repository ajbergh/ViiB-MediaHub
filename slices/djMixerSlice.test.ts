import { describe, expect, it } from 'vitest';
import { createDJMixerSlice, type DJMixerSlice } from './djMixerSlice';

function createTestMixerState(): DJMixerSlice {
  let state: Record<string, unknown> = {};
  const set = (update: unknown) => {
    const patch = typeof update === 'function'
      ? (update as (current: Record<string, unknown>) => Record<string, unknown>)(state)
      : update;
    Object.assign(state, patch);
  };
  const slice = createDJMixerSlice(set as never, (() => state) as never, {} as never);
  state = { ...slice };
  return state as unknown as DJMixerSlice;
}

describe('deck analysis patches', () => {
  it('keeps independently resolved tempo, key, and grid fields intact', () => {
    const state = createTestMixerState();
    state.setDeckAnalysis('A', { bpm: 128, beatGrid: [0, 0.46875] });
    state.setDeckTempo('A', 1.05);
    state.shiftBeatGrid('A', 0.03);

    state.setDeckAnalysis('A', { key: '8A' });
    expect(state.djDeckA.originalBpm).toBe(128);
    expect(state.djDeckA.effectiveBpm).toBe(134.4);
    expect(state.djDeckA.key).toBe('8A');
    expect(state.djDeckA.beatGrid).toEqual([0, 0.46875]);
    expect(state.djDeckA.beatGridOffset).toBe(0.03);

    state.setDeckAnalysis('A', { bpm: 130 });
    expect(state.djDeckA.key).toBe('8A');
    expect(state.djDeckA.beatGrid).toEqual([0, 0.46875]);
    expect(state.djDeckA.beatGridOffset).toBe(0.03);
    expect(state.djDeckA.effectiveBpm).toBe(136.5);
  });
});

describe('guarded deck snapshot restoration', () => {
  it('restores the complete deck and key-lock value in one store action', () => {
    const state = createTestMixerState();
    const snapshot = {
      ...state.djDeckB,
      track: { id: 'original', title: 'Original' } as DJMixerSlice['djDeckB']['track'],
      position: 42,
      duration: 180,
      isPlaying: true,
      cuePoint: 8,
      tempo: 1.06,
      loop: { enabled: true, start: 32, end: 48, pendingIn: null },
      hotCues: [{ slot: 1, position: 64, label: 'drop', color: '#f97316' }],
      fx: { ...state.djDeckB.fx, reverb: { ...state.djDeckB.fx.reverb, enabled: true, mix: .6 } },
    };
    state.restoreDeckSnapshot('B', snapshot, true);
    expect(state.djDeckB).toEqual(snapshot);
    expect(state.djMixer.keyLockB).toBe(true);
  });
});


describe('reviewed beat grids', () => {
  it('invalidates stale evidence after edits and protects reviewed evidence from automatic updates', () => {
    const state = createTestMixerState();
    const evidence = { source: 'browser' as const, alternateBpm: 60, stability: 1 };
    state.setDeckAnalysis('A', { bpm: 120, beatGrid: [0, .5, 1], tempoEvidence: evidence });
    state.setDeckAnalysis('A', { key: '8A' });
    expect(state.djDeckA.tempoEvidence).toEqual(evidence);
    state.setDeckAnalysis('A', { beatGrid: [.1, .6, 1.1], beatGridSource: 'manual', beatGridLocked: true });
    expect(state.djDeckA.tempoEvidence).toBeNull();
    state.setDeckAnalysis('A', { tempoEvidence: evidence });
    state.setDeckAnalysis('A', { automatic: true, bpm: 130, beatGrid: [0, .46, .92], tempoEvidence: null });
    expect(state.djDeckA.tempoEvidence).toEqual(evidence);
    expect(state.djDeckA.originalBpm).toBe(120);
  });
  it('preserves manual timing through background results, including missing analysis', () => {
    const state = createTestMixerState();
    state.setDeckAnalysis('A', { bpm: 120, beatGrid: [0.1, 0.6, 1.1], beatGridSource: 'manual', beatGridLocked: true });
    state.setDeckAnalysis('A', { automatic: true, bpm: 130, beatGrid: [0, 0.46], beatGridSource: 'generated', beatGridLocked: false, key: '8A' });
    state.setDeckAnalysis('A', { automatic: true, bpm: null, beatGrid: null, beatGridSource: 'unknown', beatGridLocked: false });
    expect(state.djDeckA.beatGrid).toEqual([0.1, 0.6, 1.1]);
    expect(state.djDeckA.beatGridSource).toBe('manual');
    expect(state.djDeckA.originalBpm).toBe(120);
    expect(state.djDeckA.key).toBe('8A');
    state.setDeckAnalysis('A', { beatGrid: null, beatGridSource: 'unknown', beatGridLocked: false });
    expect(state.djDeckA.beatGrid).toBeNull();
    expect(state.djDeckA.beatGridLocked).toBe(false);
  });
  it('blocks phase alignment for an estimate but permits two reviewed grids', () => {
    const state = createTestMixerState();
    for (const deck of ['A', 'B'] as const) state.setDeckAnalysis(deck, { bpm: 120, beatGrid: [0, 0.5, 1, 1.5], beatGridSource: 'generated' });
    state.setDeckPosition('A', 0.2);
    state.setDeckPosition('B', 0.4);
    state.syncBeatPhase('A');
    expect(state.djDeckA.position).toBe(0.2);
    for (const deck of ['A', 'B'] as const) state.setDeckAnalysis(deck, { beatGrid: [0, 0.5, 1, 1.5], beatGridSource: 'manual', beatGridLocked: true });
    state.syncBeatPhase('A');
    expect(state.djDeckA.position).toBeCloseTo(0.4);
  });
});
