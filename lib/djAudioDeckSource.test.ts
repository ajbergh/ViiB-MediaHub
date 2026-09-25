import { afterEach, describe, expect, it, vi } from 'vitest';

const state = {
  djDeckA: { tempo: 1, loop: { start: 2, end: 4, enabled: true, pendingIn: null }, beatGrid: [0, 0.5, 1], beatGridOffset: 0, beatGridSource: 'manual', beatGridLocked: true, effectiveBpm: 120, originalBpm: 120 },
  djDeckB: { tempo: 1, loop: { start: 0, end: 0, enabled: false, pendingIn: null }, beatGrid: [0, 0.5, 1], beatGridOffset: 0, beatGridSource: 'manual', beatGridLocked: true, effectiveBpm: 120, originalBpm: 120 },
  djMixer: { quantize: false, slipModeA: false, slipModeB: false },
  setDeckPosition: vi.fn(), setDeckPlaying: vi.fn(), setDeckDuration: vi.fn(),
  setLoop: vi.fn(), setPendingLoopIn: vi.fn(), toggleLoop: vi.fn(), clearLoop: vi.fn(),
};
vi.mock('../store', () => ({ useStore: { getState: () => state } }));

import { DJAudioEngine } from './djAudio';

function fakeSource(position = 3.9) {
  const state = { position, duration: 20, playing: true, loaded: true, tempo: 1, keyLock: false };
  const source = {
    outputNode: {} as AudioNode, load: vi.fn(async () => 'track.mp3'), cancelLoad: vi.fn(), unload: vi.fn(),
    play: vi.fn(async () => { state.playing = true; }), pause: vi.fn(() => { state.playing = false; }),
    seek: vi.fn((position: number) => { state.position = Math.max(0, Math.min(position, state.duration)); }),
    setTempo: vi.fn((tempo: number) => { state.tempo = Math.max(0.5, Math.min(1.5, tempo)); }),
    setKeyLock: vi.fn((enabled: boolean) => { state.keyLock = enabled; }), setLoop: vi.fn(),
    getPosition: vi.fn(() => state.position), getDuration: vi.fn(() => state.duration),
    isPlaying: vi.fn(() => state.playing), isLoaded: vi.fn(() => state.loaded),
    addEventListener: vi.fn(), removeEventListener: vi.fn(), dispose: vi.fn(),
  };
  return { source, state };
}

describe('DJAudioEngine DeckSource integration', () => {
  afterEach(() => { vi.restoreAllMocks(); });

  it('wraps an active loop using source position and seek', () => {
    const engine = new DJAudioEngine();
    const { source } = fakeSource(4.5);
    Object.assign(engine, { deckSourceA: source });
    (engine as unknown as { wrapActiveLoops: () => void }).wrapActiveLoops();
    expect(source.seek).toHaveBeenCalledWith(2.5);
  });

  it('routes cue through the existing headphone gain node', () => {
    const engine = new DJAudioEngine();
    const setTargetAtTime = vi.fn();
    Object.assign(engine, {
      audioContext: { currentTime: 7 },
      headphoneCueGainA: { gain: { setTargetAtTime } },
    });
    engine.setCueEnabled('A', true);
    expect(setTargetAtTime).toHaveBeenCalledWith(1, 7, 0.01);
  });

  it('uses source-neutral seek for nudge and phase sync', () => {
    const engine = new DJAudioEngine();
    const { source, state: sourceState } = fakeSource(0.3);
    Object.assign(engine, { deckSourceB: source });
    engine.nudgePosition('B', 100);
    expect(sourceState.position).toBeCloseTo(0.4);

    sourceState.position = 0.2;
    engine.syncBeatPhase('B', [0, 0.5, 1], [0, 0.5, 1], 0.3);
    expect(source.seek).toHaveBeenLastCalledWith(0.3);
  });

  it('exposes playback and loaded state from the source', async () => {
    const engine = new DJAudioEngine();
    const { source } = fakeSource();
    Object.assign(engine, { deckSourceA: source });
    expect(engine.isLoaded('A')).toBe(true);
    expect(engine.isPlaying('A')).toBe(true);
    await engine.play('A');
    expect(source.play).toHaveBeenCalledOnce();
    engine.pause('A');
    expect(source.pause).toHaveBeenCalledOnce();
  });

  it('increments the load generation when a deck is unloaded', () => {
    const engine = new DJAudioEngine();
    const { source } = fakeSource();
    Object.assign(engine, { deckSourceB: source });
    expect(engine.getDeckLoadGeneration('B')).toBe(0);
    engine.unloadDeck('B');
    expect(engine.getDeckLoadGeneration('B')).toBe(1);
    expect(source.unload).toHaveBeenCalledOnce();
  });

  it('tracks the loaded source identity separately from deck store state', async () => {
    const engine = new DJAudioEngine();
    const { source } = fakeSource();
    Object.assign(engine, { deckSourceA: source });
    expect(engine.getDeckLoadedTrackId('A')).toBeNull();
    await engine.loadTrack('A', { id: 'source-one', title: 'Source', url: '/source' } as never);
    expect(engine.getDeckLoadedTrackId('A')).toBe('source-one');
    engine.unloadDeck('A');
    expect(engine.getDeckLoadedTrackId('A')).toBeNull();
  });

  it('advances the stem-control ownership epoch for user mixer edits', async () => {
    const engine = new DJAudioEngine();
    expect(engine.getStemControlGeneration('B')).toBe(0);
    engine.setStemGain('B', 'vocals', .5);
    expect(engine.getStemControlGeneration('B')).toBe(1);
    engine.setStemMuted('B', 'drums', true);
    engine.setStemSolo('B', 'bass', true);
    await engine.setStemMode('B', 'stems');
    expect(engine.getStemControlGeneration('B')).toBe(4);
  });
});
