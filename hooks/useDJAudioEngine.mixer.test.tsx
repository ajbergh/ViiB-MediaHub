// @vitest-environment jsdom
import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { afterEach, expect, it, vi } from 'vitest';

const mocks = vi.hoisted(() => ({ state: {} as any, listener: null as any,
  crossfader: vi.fn(), tempo: vi.fn(), phase: vi.fn() }));
vi.mock('../store', () => {
  const useStore = (select: any) => select(mocks.state);
  Object.assign(useStore, { getState: () => mocks.state, subscribe: (fn: any) => {
    mocks.listener = fn; return () => { mocks.listener = null; };
  } });
  return { useStore };
});
vi.mock('../lib/djAudio', () => ({ getDJAudioEngine: () => new Proxy({ initialized: true,
  setCrossfader: mocks.crossfader, setTempo: mocks.tempo, syncBeatPhase: mocks.phase },
  { get: (target, key) => key in target ? (target as any)[key] : vi.fn() }), disposeDJAudioEngine: vi.fn() }));
vi.mock('../services/api', () => ({ api: {} }));
vi.mock('../lib/clientWaveform', () => ({ generateClientWaveform: vi.fn() }));
import { useDJAudioEngineSync } from './useDJAudioEngine';
import { DJTransportButtons } from '../components/dj/v2/DJTransportButtons';

afterEach(() => vi.clearAllMocks());
function setup(container = document.createElement('div')) {
  (globalThis as any).IS_REACT_ACT_ENVIRONMENT = true;
  const deck = () => ({ track: { id: 'track' }, volume: 1, eq: { low: 0, mid: 0, high: 0 }, tempo: 1,
    originalBpm: 120, effectiveBpm: 128, cueEnabled: false, cuePoint: 0, isPlaying: false,
    position: 0.3, beatGrid: [0, 0.5, 1], beatGridSource: 'manual', beatGridLocked: true });
  mocks.state = { djDeckA: deck(), djDeckB: deck(), djMixer: { crossfader: 0, crossfaderCurve: 'constant-power',
    syncMode: 'bpm', masterVolume: 0.8, headphoneVolume: 1, headphoneMix: 0.5, beatFX: {} }, setDeckTempo: vi.fn() };
  return createRoot(container);
}

it('reapplies a changed curve immediately without moving the crossfader', async () => {
  const root = setup();
  function Sync() { useDJAudioEngineSync(); return null; }
  try {
    await act(async () => root.render(<Sync />)); mocks.crossfader.mockClear();
    for (const curve of ['linear', 'sharp', 'constant-power']) {
      const previous = mocks.state;
      mocks.state = { ...previous, djMixer: { ...previous.djMixer, crossfaderCurve: curve } };
      mocks.listener(mocks.state, previous);
    }
    expect(mocks.crossfader.mock.calls).toEqual([[0], [0], [0]]);
  } finally { await act(async () => root.unmount()); }
  expect(mocks.listener).toBeNull();
});

it('deck SYNC disables unavailable modes and applies BPM or BPM plus phase when ready', async () => {
  const container = document.createElement('div'); const ui = setup(container);
  const render = () => act(async () => ui.render(<DJTransportButtons deck='A' console />));
  const button = () => container.querySelector<HTMLButtonElement>('[aria-label="Sync deck A to other deck"]')!;
  try {
    mocks.state.djMixer.syncMode = 'off'; await render(); expect(button().disabled).toBe(true);
    mocks.state.djMixer.syncMode = 'bpm'; mocks.state.djDeckB.effectiveBpm = 0;
    await render(); expect(button().disabled).toBe(true); expect(button().title).toContain('BPM analysis');
    mocks.state.djDeckB.effectiveBpm = 128; await render(); expect(button().disabled).toBe(false);
    await act(async () => button().click()); expect(mocks.tempo).toHaveBeenLastCalledWith('A', 128 / 120);
    expect(mocks.phase).not.toHaveBeenCalled();
    mocks.state.djMixer.syncMode = 'beat-phase'; mocks.state.djDeckB.beatGridLocked = false;
    await render(); expect(button().disabled).toBe(true);
    mocks.state.djDeckB.beatGridLocked = true; await render(); await act(async () => button().click());
    expect(mocks.phase).toHaveBeenCalledWith('A', [0, 0.5, 1], [0, 0.5, 1], 0.3);
  } finally { await act(async () => ui.unmount()); }
});
