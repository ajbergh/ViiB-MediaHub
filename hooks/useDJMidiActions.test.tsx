// @vitest-environment jsdom
import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { expect, it, vi } from 'vitest';

const mocks = vi.hoisted(() => ({
  handler: null as any, state: {} as any, listener: null as any,
  seek: vi.fn(), loop: vi.fn(), toggleLoop: vi.fn(), volume: vi.fn(), trigger: vi.fn(), stop: vi.fn(), led: vi.fn(),
  stemMute: vi.fn(), stemStatus: { available: true, mode: 'stems' }, modeProvider: null as any,
  feedbackProvider: null as any, refresh: vi.fn(), storeListener: null as any, storeUnsubscribe: vi.fn(),
  headphones: vi.fn(), filter: vi.fn(), filterValue: vi.fn(), tempo: vi.fn(),
  scratchStart: vi.fn(), scratchEnd: vi.fn(), scratchMove: vi.fn(), nudge: vi.fn(),
  browse: vi.fn(), load: vi.fn(), open: vi.fn(),
  stemStates: null as any,
}));
vi.mock('../store', () => ({ useStore: {
  getState: () => mocks.state,
  subscribe: (listener: any) => { mocks.storeListener = listener; return mocks.storeUnsubscribe; },
} }));
vi.mock('./useDJAudioEngine', () => ({ useDJAudioEngineActions: () => ({
  seek: mocks.seek, setLoopBeats: mocks.loop, toggleLoop: mocks.toggleLoop, setVolume: mocks.volume,
  getStemStatus: () => mocks.stemStatus,
  getStemState: (deck: string) => mocks.stemStates?.[deck] ?? ({ vocals: { muted: false, gain: 1 }, music: { muted: false, gain: 1 }, bass: { muted: false, gain: 1 }, drums: { muted: false, gain: 1 } }),
  setStemMuted: mocks.stemMute,
  returnToCue: vi.fn(), togglePlay: vi.fn().mockResolvedValue(undefined),
  setHeadphoneVolume: mocks.headphones, setFilterFX: mocks.filter, setTempo: mocks.tempo,
  startScratch: mocks.scratchStart, endScratch: mocks.scratchEnd, updateScratch: mocks.scratchMove, nudgePosition: mocks.nudge,
}) }));
vi.mock('../lib/djMidiLibrary', () => ({ browseDJMidiLibrary: mocks.browse, loadDJMidiLibrary: mocks.load, toggleDJMidiLibrary: mocks.open }));
vi.mock('../lib/djSampler', () => ({ getDJSamplerEngine: () => ({ triggerPad: mocks.trigger, stopPad: mocks.stop }) }));
vi.mock('../lib/djMidi', () => ({ getDJMidiService: () => ({
  loadMappings: vi.fn(), isEnabled: () => true, getDevices: () => [],
  subscribe: (listener: any) => { mocks.listener = listener; return vi.fn(); },
  setActionHandler: (handler: any) => { mocks.handler = handler; }, sendPadModeFeedback: mocks.led,
  setPadModeProvider: (provider: any) => { mocks.modeProvider = provider; },
  setFeedbackProvider: (provider: any) => { mocks.feedbackProvider = provider; }, refreshFeedback: mocks.refresh,
}) }));
import { useDJMidiActions } from './useDJMidiActions';

it.each([
  ['A', 0.5], ['B', 0.5], ['A', 0.63], ['B', 0.63], ['A', 0.1], ['B', 0.1],
] as const)('toggles the active loop pad off on deck %s with source seconds per beat %s', async (deck, secondsPerBeat) => {
  (globalThis as any).IS_REACT_ACT_ENVIRONMENT = true;
  const deckState = () => ({ track: { id: 'song' }, position: 10, tempo: 1.25,
    isPlaying: false, cueEnabled: false, originalBpm: 120, effectiveBpm: 150,
    beatGrid: [10, 10 + secondsPerBeat, 10 + secondsPerBeat * 2], beatGridLocked: secondsPerBeat !== 0.5,
    beatGridSource: 'manual', beatGridOffset: 0,
    hotCues: [], loop: { enabled: false, start: 0, end: 0 } });
  mocks.state = { djDeckA: deckState(), djDeckB: deckState(), djSampler: [] };
  const target = mocks.state[deck === 'A' ? 'djDeckA' : 'djDeckB'];
  mocks.loop.mockImplementation((id: string, beats: number) => {
    const state = mocks.state[id === 'A' ? 'djDeckA' : 'djDeckB'];
    state.loop = { enabled: true, start: 10, end: 10 + beats * secondsPerBeat, presetBeats: beats };
  });
  mocks.toggleLoop.mockImplementation((id: string) => {
    const state = mocks.state[id === 'A' ? 'djDeckA' : 'djDeckB'];
    state.loop.enabled = !state.loop.enabled;
  });
  const root = createRoot(document.createElement('div'));
  function Harness() { useDJMidiActions(); return null; }
  const button = (kind: string, down: boolean) => mocks.handler(`deck${deck}.${kind}`, down ? 1 : 0,
    down ? 'press' : 'release', { deviceId: 'controller' });
  const tap = (kind: string) => { button(kind, true); button(kind, false); };
  try {
    await act(async () => root.render(<Harness />));
    tap('padMode');
    for (const [index, beats] of [1, 2, 4, 8].entries()) {
      const pad = `performancePad${index + 1}`;
      tap(pad);
      expect(mocks.loop).toHaveBeenLastCalledWith(deck, beats);
      expect(target.loop.enabled).toBe(true);
      const toggles = mocks.toggleLoop.mock.calls.length;
      button(pad, true); button(pad, true); // Repeated down must not re-enable.
      expect(mocks.toggleLoop.mock.calls.length).toBe(toggles + 1);
      expect(target.loop.enabled).toBe(false);
      button(pad, false);
      expect(target.loop.enabled).toBe(false);
      tap(pad); // Disabled loop can be started again.
      expect(target.loop.enabled).toBe(true);
      tap(pad);
    }
    tap('performancePad1');
    const toggles = mocks.toggleLoop.mock.calls.length;
    tap('performancePad4');
    expect(mocks.toggleLoop.mock.calls.length).toBe(toggles);
    expect(target.loop.end - target.loop.start).toBeCloseTo(8 * secondsPerBeat);
    expect(mocks.state[deck === 'A' ? 'djDeckB' : 'djDeckA'].loop.enabled).toBe(false);
    expect(mocks.feedbackProvider().find((message: number[]) => message[0] === (deck === 'A' ? 0x94 : 0x95)
      && message[1] === 0x17)?.[2]).toBe(127);
    tap('performancePad4');
    expect(mocks.feedbackProvider().find((message: number[]) => message[0] === (deck === 'A' ? 0x94 : 0x95)
      && message[1] === 0x17)?.[2]).toBe(0);
  } finally {
    await act(async () => root.unmount());
    mocks.loop.mockReset(); mocks.toggleLoop.mockReset();
    vi.clearAllMocks();
  }
});

it('connects the printed pad modes to deck audio, sampler banks, stem mutes, and LED feedback', async () => {
  vi.useFakeTimers();
  (globalThis as any).IS_REACT_ACT_ENVIRONMENT = true;
  const deck = () => ({ track: { id: 'song' }, position: 10, volume: 0.8,
    isPlaying: false, cueEnabled: false, originalBpm: 120, loop: { enabled: false, start: 0, end: 0 },
    hotCues: [{ slot: 1, position: 5 }] });
  mocks.state = { djDeckA: deck(), djDeckB: deck(), djSampler: Array.from({ length: 8 }, () => ({ mode: 'gate' })), setDeckFilter: mocks.filterValue };
  mocks.volume.mockImplementation((deck: string, value: number) => {
    mocks.state[deck === 'A' ? 'djDeckA' : 'djDeckB'].volume = value;
  });
  const root = createRoot(document.createElement('div'));
  function Harness() { useDJMidiActions(); return null; }
  const button = (deck: string, control: string, down: boolean) => mocks.handler(
    `deck${deck}.${control}`, down ? 1 : 0, down ? 'press' : 'release', { deviceId: 'controller' },
  );
  const cycle = (deck = 'A') => { button(deck, 'padMode', true); button(deck, 'padMode', false); };
  const pad = (deck: string, number: number) => {
    button(deck, `performancePad${number}`, true); button(deck, `performancePad${number}`, false);
  };
  try {
    await act(async () => root.render(<Harness />));
    expect(mocks.modeProvider('A')).toBe(0);
    expect(mocks.modeProvider('B')).toBe(0);
    const value = (action: string, amount: number) => mocks.handler(action, amount, 'value', { deviceId: 'controller', builtIn: true });
    value('headphoneVolume', 0.4);
    expect(mocks.headphones).toHaveBeenCalledWith(0.4);
    value('deckA.filter', 0);
    expect(mocks.filter).toHaveBeenLastCalledWith('A', true, 'lowpass', 200, 2);
    value('deckA.filter', 0.5);
    expect(mocks.filter).toHaveBeenLastCalledWith('A', false, 'lowpass', 20000, 0.5);
    value('deckA.tempo', 0.5);
    expect(mocks.tempo).toHaveBeenCalledWith('A', 1);
    button('A', 'jogTouch', true);
    value('deckA.jogWheel', 1 / 63);
    expect(mocks.scratchStart).toHaveBeenCalledWith('A');
    expect(mocks.scratchMove).toHaveBeenCalledOnce();
    button('A', 'jogTouch', false);
    expect(mocks.scratchEnd).toHaveBeenCalledWith('A');
    value('library.browse', 1 / 63); button('B', 'load', true);
    expect(mocks.browse).toHaveBeenCalledWith(1);
    expect(mocks.load).toHaveBeenCalledWith('B');
    button('A', 'load', true);
    expect(mocks.load).toHaveBeenLastCalledWith('A');
    button('A', 'load', false);
    for (let press = 0; press < 2; press++) mocks.handler('library.open', 1, 'press', { deviceId: 'controller' });
    mocks.handler('library.open', 0, 'release', { deviceId: 'controller' });
    expect(mocks.open).toHaveBeenCalledTimes(2);
    button('A', 'stemAcapella', true);
    expect(mocks.stemMute).toHaveBeenCalledWith('A', 'vocals', false);
    expect(mocks.stemMute).toHaveBeenCalledWith('A', 'drums', true);
    mocks.stemMute.mockClear();
    const ledValue = (status: number, note: number) => mocks.feedbackProvider().find((message: number[]) => message[0] === status && message[1] === note)?.[2];
    expect(ledValue(0x94, 0x14)).toBe(127); // Saved hot cue.
    expect(ledValue(0x94, 0x15)).toBe(0);
    button('A', 'cue', true);
    expect(ledValue(0x90, 1)).toBe(127);
    button('A', 'cue', false);
    expect(ledValue(0x90, 1)).toBe(1);
    mocks.refresh.mockClear();
    const before = mocks.state;
    mocks.state = { ...before, djDeckA: { ...before.djDeckA, position: 11 } };
    mocks.storeListener(mocks.state, before);
    expect(mocks.refresh).not.toHaveBeenCalled();
    const paused = mocks.state;
    mocks.state = { ...paused, djDeckA: { ...paused.djDeckA, isPlaying: true, cueEnabled: true } };
    mocks.storeListener(mocks.state, paused);
    expect(mocks.refresh).toHaveBeenCalledOnce();
    expect(ledValue(0x90, 0)).toBe(127);
    expect(ledValue(0x80, 0x1B)).toBe(127);
    mocks.state.djDeckA.position = 11.3;
    mocks.state.djDeckB.isPlaying = true;
    mocks.state.djDeckB.position = 12;
    mocks.refresh.mockClear();
    vi.advanceTimersByTime(50);
    expect(mocks.refresh).toHaveBeenCalledOnce();
    expect(ledValue(0x94, 0x14)).toBe(1); // A is between beats.
    expect(ledValue(0x95, 0x14)).toBe(127); // B is on its own beat.
    expect(ledValue(0x94, 0x15)).toBe(0); // Empty pads remain dark.
    mocks.state.djDeckA.position = 11;
    mocks.state.djDeckB.isPlaying = false;
    pad('A', 1);
    expect(mocks.seek).toHaveBeenCalledWith('A', 5);
    cycle(); pad('A', 3);
    expect(mocks.loop).toHaveBeenCalledWith('A', 4);
    cycle(); button('A', 'performancePad2', true);
    expect(mocks.trigger).toHaveBeenLastCalledWith(1);
    cycle(); // Stop the held gate before changing to Stems mode.
    expect(mocks.stop).toHaveBeenLastCalledWith(1);
    button('A', 'performancePad2', false); // Physical release of the old sampler pad.
    expect(mocks.led).toHaveBeenLastCalledWith('controller', 'A', 3);
    expect(mocks.modeProvider('A')).toBe(3);
    for (const [index, bus] of ['vocals', 'music', 'bass', 'drums'].entries()) {
      pad('A', index + 1);
      expect(mocks.stemMute).toHaveBeenLastCalledWith('A', bus, true);
    }
    mocks.stemStatus.mode = 'full';
    mocks.stemMute.mockClear(); pad('A', 1);
    expect(mocks.stemMute).not.toHaveBeenCalled();
    cycle();
    expect(mocks.led).toHaveBeenLastCalledWith('controller', 'A', 0);
    cycle('B'); cycle('B'); pad('B', 4);
    expect(mocks.trigger).toHaveBeenLastCalledWith(7);
    expect(mocks.stop).toHaveBeenLastCalledWith(7);
  } finally {
    await act(async () => root.unmount());
    expect(mocks.modeProvider).toBeNull();
    expect(mocks.feedbackProvider).toBeNull();
    expect(mocks.storeUnsubscribe).toHaveBeenCalledOnce();
    mocks.refresh.mockClear();
    vi.advanceTimersByTime(100);
    expect(mocks.refresh).not.toHaveBeenCalled();
    vi.useRealTimers();
    vi.clearAllMocks();
  }
});

it('updates dedicated stem LEDs in all banks, including paused UI edits and reconnect snapshots', async () => {
  vi.useFakeTimers();
  (globalThis as any).IS_REACT_ACT_ENVIRONMENT = true;
  mocks.stemStatus = { available: true, mode: 'stems' };
  const stemState = () => Object.fromEntries(['vocals', 'music', 'bass', 'drums'].map(bus => [bus, { muted: false, gain: 1, solo: false }]));
  mocks.stemStates = { A: stemState(), B: stemState() };
  mocks.stemMute.mockImplementation((deck: string, bus: string, muted: boolean) => { mocks.stemStates[deck][bus].muted = muted; });
  const deck = () => ({ track: { id: 'song' }, position: 10, isPlaying: false,
    cueEnabled: false, originalBpm: 120, hotCues: [], loop: { enabled: false, start: 0, end: 0 } });
  mocks.state = { djDeckA: deck(), djDeckB: deck(), djSampler: [] };
  const root = createRoot(document.createElement('div'));
  function Harness() { useDJMidiActions(); return null; }
  const press = (deck: string, kind: string) => {
    for (const down of [true, false]) mocks.handler(`deck${deck}.${kind}`, down ? 1 : 0, down ? 'press' : 'release', { deviceId: 'controller' });
  };
  const leds = (status: number) => mocks.feedbackProvider().filter((message: number[]) => message[0] === status
    && [0x46, 0x48].includes(message[1])).map((message: number[]) => message[2]);
  try {
    await act(async () => root.render(<Harness />));
    expect(leds(0x90)).toEqual([0, 0]);
    press('A', 'stemAcapella');
    press('B', 'stemInstrumental');
    for (let mode = 0; mode < 4; mode++) {
      expect(leds(0x90)).toEqual([127, 0]);
      expect(leds(0x91)).toEqual([0, 127]);
      press('A', 'padMode'); press('B', 'padMode');
    }
    // Paused in Hot Cue mode: UI edits must still refresh engine-owned state.
    mocks.stemStates.A.music.muted = false;
    mocks.refresh.mockClear();
    vi.advanceTimersByTime(50);
    expect(mocks.refresh).toHaveBeenCalledOnce();
    expect(leds(0x90)).toEqual([0, 0]);
    expect(leds(0x91)).toEqual([0, 127]);
    mocks.listener(); // Reconnect asks the provider for the current snapshot.
    expect(leds(0x91)).toEqual([0, 127]);
    press('B', 'stemInstrumental');
    expect(leds(0x91)).toEqual([0, 0]);
    const previous = mocks.state;
    mocks.state = { ...previous, djDeckA: { ...previous.djDeckA, track: null } };
    mocks.stemStates.A.music.muted = true;
    mocks.storeListener(mocks.state, previous);
    expect(leds(0x90)).toEqual([0, 0]);
  } finally {
    await act(async () => root.unmount());
    mocks.stemStates = null;
    mocks.stemMute.mockReset();
    vi.useRealTimers();
    vi.clearAllMocks();
  }
});
