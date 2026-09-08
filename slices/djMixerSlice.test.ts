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
