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
import { DJTempoSlider } from '../components/dj/v2/DJTempoSlider';

afterEach(() => vi.clearAllMocks());
function setup(container = document.createElement('div')) {
  (globalThis as any).IS_REACT_ACT_ENVIRONMENT = true;
  const deck = () => ({ track: { id: 'track' }, volume: 1, eq: { low: 0, mid: 0, high: 0 }, tempo: 1,
    originalBpm: 120, effectiveBpm: 128, cueEnabled: false, cuePoint: 0, isPlaying: false,
    position: 0.3, beatGrid: [0, 0.5, 1], beatGridSource: 'manual', beatGridLocked: true });
  mocks.state = { djDeckA: deck(), djDeckB: deck(), djMixer: { crossfader: 0, crossfaderCurve: 'constant-power',
    syncMode: 'bpm', masterVolume: 0.8, headphoneVolume: 1, headphoneMix: 0.5, beatFX: {} }, setDeckTempo: vi.fn((deck, tempo) => {
      const previous = mocks.state;
      const key = deck === 'A' ? 'djDeckA' : 'djDeckB';
      mocks.state = { ...previous, [key]: { ...previous[key], tempo } };
      mocks.listener?.(mocks.state, previous);
    }) };
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

it('groups pitch drag events by frame and applies the last pending value on release', async () => {
  const container = document.createElement('div');
  const root = setup(container);
  const update = vi.fn();
  vi.useFakeTimers({ toFake: ['requestAnimationFrame', 'cancelAnimationFrame', 'setTimeout', 'clearTimeout'] });
  try {
    await act(async () => root.render(<DJTempoSlider deck="B" value={1} onChange={update} />));
    const slider = container.querySelector('[role="slider"]')!;
    const measure = vi.spyOn(slider, 'getBoundingClientRect').mockReturnValue(new DOMRect(0, 100, 40, 100));
    const event = (target: EventTarget, type: string, clientY: number) =>
      target.dispatchEvent(new MouseEvent(type, { bubbles: true, button: 0, clientY }));

    await act(async () => { event(slider, 'mousedown', 150); });
    await act(async () => {
      for (let y = 130; y >= 125; y--) event(window, 'mousemove', y);
    });
    expect(update.mock.calls).toEqual([[1]]);
    expect(measure).toHaveBeenCalledTimes(1);

    await act(async () => { vi.advanceTimersByTime(24); });
    expect(update).toHaveBeenLastCalledWith(1.08);

    await act(async () => {
      event(window, 'mousemove', 175);
      event(window, 'mouseup', 175);
      vi.advanceTimersByTime(24);
    });
    expect(update.mock.calls).toEqual([[1], [1.08], [0.92]]);
    expect(measure).toHaveBeenCalledTimes(3);
  } finally {
    await act(async () => root.unmount());
    vi.useRealTimers();
  }
});

it('applies pitch while waiting on a delayed animation frame', async () => {
  const container = document.createElement('div');
  const root = setup(container);
  const update = vi.fn();
  vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] });
  vi.spyOn(window, 'requestAnimationFrame').mockReturnValue(1);
  vi.spyOn(window, 'cancelAnimationFrame').mockImplementation(() => {});
  try {
    await act(async () => root.render(<DJTempoSlider deck="A" value={1} onChange={update} />));
    const slider = container.querySelector('[role="slider"]')!;
    vi.spyOn(slider, 'getBoundingClientRect').mockReturnValue(new DOMRect(0, 100, 40, 100));
    await act(async () => {
      slider.dispatchEvent(new MouseEvent('mousedown', { bubbles: true, clientY: 150 }));
    });
    await act(async () => {
      window.dispatchEvent(new MouseEvent('mousemove', { clientY: 125 }));
      vi.advanceTimersByTime(16);
    });
    expect(update.mock.calls).toEqual([[1], [1.08]]);
    expect(window.cancelAnimationFrame).toHaveBeenCalledWith(1);
  } finally {
    await act(async () => root.unmount());
    vi.useRealTimers();
    vi.restoreAllMocks();
  }
});

it('deck SYNC disables unavailable modes and applies BPM or BPM plus phase when ready', async () => {
  const container = document.createElement('div'); const ui = setup(container);
  function Deck() {
    useDJAudioEngineSync();
    return <DJTransportButtons deck='A' console />;
  }
  const render = () => act(async () => ui.render(<Deck />));
  const button = () => container.querySelector<HTMLButtonElement>('[aria-label="Sync deck A to other deck"]')!;
  try {
    mocks.state.djMixer.syncMode = 'off'; await render(); expect(button().disabled).toBe(true);
    mocks.state.djMixer.syncMode = 'bpm'; mocks.state.djDeckB.effectiveBpm = 0;
    await render(); expect(button().disabled).toBe(true); expect(button().title).toContain('BPM analysis');
    mocks.state.djDeckB.effectiveBpm = 128; await render(); expect(button().disabled).toBe(false);
    mocks.tempo.mockClear();
    await act(async () => button().click());
    expect(mocks.tempo.mock.calls).toEqual([['A', 128 / 120]]);
    expect(mocks.phase).not.toHaveBeenCalled();
    mocks.state.djMixer.syncMode = 'beat-phase'; mocks.state.djDeckB.beatGridLocked = false;
    await render(); expect(button().disabled).toBe(true);
    mocks.state.djDeckB.beatGridLocked = true; await render(); await act(async () => button().click());
    expect(mocks.phase).toHaveBeenCalledWith('A', [0, 0.5, 1], [0, 0.5, 1], 0.3);
  } finally { await act(async () => ui.unmount()); }
});
