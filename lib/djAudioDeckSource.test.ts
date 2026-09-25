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
import { StemDeckSource } from './stemDeckSource';
import type { DeckState } from '../slices/djMixerSlice';

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

  it('marks a pending load as unavailable and ignores it after ownership is unloaded', async () => {
    const engine = new DJAudioEngine();
    const { source } = fakeSource();
    let resolveLoad: ((url: string) => void) | undefined;
    source.load = vi.fn(() => new Promise<string>(resolve => { resolveLoad = resolve; }));
    Object.assign(engine, { deckSourceB: source });

    const pending = engine.loadTrack('B', { id: 'candidate', title: 'Candidate' } as never);
    expect(engine.isDeckLoading('B')).toBe(true);
    expect(engine.getDeckLoadedTrackId('B')).toBeNull();
    engine.unloadDeck('B');
    expect(engine.isDeckLoading('B')).toBe(false);
    resolveLoad?.('/candidate.mp3');
    await pending;
    expect(engine.getDeckLoadedTrackId('B')).toBeNull();
    expect(engine.isDeckLoading('B')).toBe(false);
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

  it('advances transport ownership for explicit seek, pause, play, and nudge commands', async () => {
    const engine = new DJAudioEngine();
    const { source } = fakeSource();
    Object.assign(engine, { deckSourceA: source });
    expect(engine.getDeckTransportControlGeneration('A')).toBe(0);
    engine.seek('A', 7);
    engine.pause('A');
    await engine.play('A');
    engine.nudgePosition('A', 20);
    expect(engine.getDeckTransportControlGeneration('A')).toBe(4);
  });

  it('does not write mixer controls after ownership changes during async stem-mode restoration', async () => {
    const engine = new DJAudioEngine();
    let mode: 'full' | 'stems' = 'full';
    let resolveMode: (() => void) | undefined;
    const source = Object.assign(Object.create(StemDeckSource.prototype), {
      isLoaded: () => true,
      getStemStatus: () => ({ mode, available: true }),
      setStemMode: () => new Promise<void>(resolve => { resolveMode = () => { mode = 'stems'; resolve(); }; }),
      setKeyLock: vi.fn(), setLoop: vi.fn(), setStemGain: vi.fn(), setStemMuted: vi.fn(), setStemSolo: vi.fn(),
    });
    Object.assign(engine, { deckSourceB: source });
    const controlWriters = [
      vi.spyOn(engine, 'setVolume'), vi.spyOn(engine, 'setEQ'), vi.spyOn(engine, 'setTempo'),
      vi.spyOn(engine, 'setFilterFX'), vi.spyOn(engine, 'setDelayFX'), vi.spyOn(engine, 'setFlangerFX'),
      vi.spyOn(engine, 'setReverbFX'), vi.spyOn(engine, 'setCueEnabled'),
    ];
    let stillOwned = true;
    const stemState = {
      vocals: { gain: 1, muted: false, solo: false }, drums: { gain: 1, muted: false, solo: false },
      bass: { gain: 1, muted: false, solo: false }, music: { gain: 1, muted: false, solo: false },
    };

    const restore = engine.restoreDeckMixState('B', {} as DeckState, false, 'stems', stemState, 0, () => stillOwned);
    for (const writer of controlWriters) expect(writer).not.toHaveBeenCalled();
    expect(source.setKeyLock).not.toHaveBeenCalled();
    expect(source.setLoop).not.toHaveBeenCalled();
    stillOwned = false;
    resolveMode?.();
    await expect(restore).rejects.toThrow('ownership changed');
    for (const writer of controlWriters) expect(writer).not.toHaveBeenCalled();
    expect(source.setKeyLock).not.toHaveBeenCalled();
    expect(source.setLoop).not.toHaveBeenCalled();
  });

  it('keeps the active source untouched during delayed or failed detached preparation', async () => {
    const engine = new DJAudioEngine();
    const active = fakeSource(5);
    let rejectLoad: ((reason?: unknown) => void) | undefined;
    const prepared = fakeSource(0);
    prepared.source.load = vi.fn(() => new Promise<string>((_resolve, reject) => { rejectLoad = reject; }));
    Object.assign(engine, {
      audioContext: {} as AudioContext, deckSourceA: active.source, gainNodeA: { connect: vi.fn() }, loadedTrackIdA: 'original',
      createPreparedSource: () => prepared.source,
    });
    const pending = engine.prepareTrack('A', { id: 'candidate', title: 'Candidate' } as never);
    expect(active.source.pause).not.toHaveBeenCalled();
    expect(active.source.unload).not.toHaveBeenCalled();
    expect(engine.getDeckLoadGeneration('A')).toBe(0);
    expect(engine.getDeckLoadedTrackId('A')).toBe('original');
    rejectLoad?.(new Error('failed detached preparation'));
    await expect(pending).rejects.toThrow('failed detached preparation');
    expect(prepared.source.dispose).toHaveBeenCalledOnce();
    expect(active.source.pause).not.toHaveBeenCalled();
    expect(engine.getDeckLoadGeneration('A')).toBe(0);
    expect(engine.getDeckLoadedTrackId('A')).toBe('original');
  });

  it('discards a prepared candidate when ownership changes before the synchronous commit', async () => {
    const engine = new DJAudioEngine();
    const active = fakeSource(5);
    const prepared = fakeSource(0);
    const gain = { connect: vi.fn() } as unknown as GainNode;
    Object.assign(prepared.source, { outputNode: { connect: vi.fn(), disconnect: vi.fn() } });
    Object.assign(active.source, { outputNode: { connect: vi.fn(), disconnect: vi.fn() } });
    Object.assign(engine, {
      audioContext: {} as AudioContext, deckSourceA: active.source, gainNodeA: gain, loadedTrackIdA: 'original',
      createPreparedSource: () => prepared.source,
    });
    const candidate = await engine.prepareTrack('A', { id: 'candidate', title: 'Candidate' } as never);
    expect(engine.commitPreparedTrack(candidate, () => false)).toBeNull();
    expect(candidate.state).toBe('discarded');
    expect(prepared.source.dispose).toHaveBeenCalledOnce();
    expect(active.source.pause).not.toHaveBeenCalled();
    expect(engine.getDeckLoadGeneration('A')).toBe(0);
    expect(engine.getDeckLoadedTrackId('A')).toBe('original');
    expect((engine as unknown as { deckSourceA: unknown }).deckSourceA).toBe(active.source);
  });

  it('swaps to a prepared source and restores the retained source at the captured position', async () => {
    const engine = new DJAudioEngine();
    const active = fakeSource(8);
    const prepared = fakeSource(0);
    const gain = {} as GainNode;
    Object.assign(prepared.source, { outputNode: { connect: vi.fn(), disconnect: vi.fn() } });
    Object.assign(active.source, { outputNode: { connect: vi.fn(), disconnect: vi.fn() } });
    Object.assign(engine, {
      audioContext: {} as AudioContext, deckSourceA: active.source, gainNodeA: gain, loadedTrackIdA: 'original',
      createPreparedSource: () => prepared.source,
    });
    (engine as unknown as { setupEventListeners: () => void }).setupEventListeners();
    const staleEnded = (active.source.addEventListener as ReturnType<typeof vi.fn>).mock.calls
      .find(([type]) => type === 'ended')?.[1] as (() => void) | undefined;
    const candidate = await engine.prepareTrack('A', { id: 'candidate', title: 'Candidate' } as never);
    const retained = engine.commitPreparedTrack(candidate, () => true, { position: 2.25, wasPlaying: true });
    expect(retained).not.toBeNull();
    expect(engine.getDeckLoadedTrackId('A')).toBe('candidate');
    expect(engine.getDeckLoadGeneration('A')).toBe(1);
    expect(active.source.pause).toHaveBeenCalledOnce();
    expect((engine as unknown as { deckSourceA: unknown }).deckSourceA).toBe(prepared.source);
    state.setDeckPlaying.mockClear();
    staleEnded?.();
    expect(state.setDeckPlaying).not.toHaveBeenCalled();

    let snapshotPublished = false;
    expect(engine.restoreRetainedDeckSource(retained!, () => true, () => { snapshotPublished = true; })).toBe(true);
    expect(snapshotPublished).toBe(true);
    expect(engine.getDeckLoadedTrackId('A')).toBe('original');
    expect(engine.getDeckLoadGeneration('A')).toBe(2);
    expect((engine as unknown as { deckSourceA: unknown }).deckSourceA).toBe(active.source);
    expect(active.source.seek).toHaveBeenCalledWith(2.25);
    expect(active.source.play).toHaveBeenCalledOnce();
    expect(prepared.source.dispose).toHaveBeenCalledOnce();
  });

  it('discards the retained original on late handoff without changing the active candidate', async () => {
    const engine = new DJAudioEngine();
    const active = fakeSource(8);
    const prepared = fakeSource(0);
    const gain = {} as GainNode;
    Object.assign(prepared.source, { outputNode: { connect: vi.fn(), disconnect: vi.fn() } });
    Object.assign(active.source, { outputNode: { connect: vi.fn(), disconnect: vi.fn() } });
    Object.assign(engine, {
      audioContext: {} as AudioContext, deckSourceA: active.source, gainNodeA: gain, loadedTrackIdA: 'original',
      createPreparedSource: () => prepared.source,
    });
    const candidate = await engine.prepareTrack('A', { id: 'candidate', title: 'Candidate' } as never);
    const retained = engine.commitPreparedTrack(candidate, () => true);
    expect(retained).not.toBeNull();
    expect(engine.restoreRetainedDeckSource(retained!, () => false)).toBe(false);
    expect(retained!.state).toBe('discarded');
    expect(active.source.dispose).toHaveBeenCalledOnce();
    expect(prepared.source.dispose).not.toHaveBeenCalled();
    expect(engine.getDeckLoadedTrackId('A')).toBe('candidate');
    expect(engine.getDeckLoadGeneration('A')).toBe(1);
    expect((engine as unknown as { deckSourceA: unknown }).deckSourceA).toBe(prepared.source);
  });

  it('plays detached reference and candidate copies through headphone-only gains', async () => {
    const engine = new DJAudioEngine();
    const activeA = fakeSource(22);
    const activeB = fakeSource(31);
    const previewReference = fakeSource();
    const previewCandidate = fakeSource();
    const cueBus = {} as GainNode;
    const gains: Array<{ gain: { value: number }; connect: ReturnType<typeof vi.fn>; disconnect: ReturnType<typeof vi.fn> }> = [];
    const audioContext = {
      createGain: () => {
        const gain = { gain: { value: 1 }, connect: vi.fn(), disconnect: vi.fn() };
        gains.push(gain);
        return gain;
      },
    } as unknown as AudioContext;
    for (const source of [activeA.source, activeB.source, previewReference.source, previewCandidate.source]) {
      Object.assign(source, { outputNode: { connect: vi.fn(), disconnect: vi.fn() } });
    }
    Object.assign(engine, {
      audioContext, headphoneCueMix: cueBus,
      deckSourceA: activeA.source, deckSourceB: activeB.source,
      loadedTrackIdA: 'reference', loadedTrackIdB: 'candidate',
      createSynchronizedPreviewSource: vi.fn().mockReturnValueOnce(previewReference.source).mockReturnValueOnce(previewCandidate.source),
    });

    const handles = await engine.startSynchronizedPreview(
      { id: 'reference' } as never, { id: 'candidate' } as never,
      { referencePosition: 8, candidatePosition: 16, referenceTempo: 1.02, candidateTempo: 1.08, stillOwned: () => true },
    );

    expect(previewReference.source.seek).toHaveBeenCalledWith(8);
    expect(previewCandidate.source.seek).toHaveBeenCalledWith(16);
    expect(previewReference.source.setTempo).toHaveBeenCalledWith(1.02);
    expect(previewCandidate.source.setTempo).toHaveBeenCalledWith(1.08);
    expect(previewReference.source.setKeyLock).toHaveBeenCalledWith(false);
    expect(previewCandidate.source.setKeyLock).toHaveBeenCalledWith(false);
    expect(previewReference.source.play).toHaveBeenCalledOnce();
    expect(previewCandidate.source.play).toHaveBeenCalledOnce();
    expect(gains.map(gain => gain.gain.value)).toEqual([0.5, 0.5]);
    expect((previewReference.source.outputNode as unknown as { connect: ReturnType<typeof vi.fn> }).connect).toHaveBeenCalledWith(gains[0]);
    expect(gains[0].connect).toHaveBeenCalledWith(cueBus);
    expect(engine.getDeckLoadedTrackId('A')).toBe('reference');
    expect(engine.getDeckLoadedTrackId('B')).toBe('candidate');
    expect(engine.getDeckLoadGeneration('A')).toBe(0);
    expect(engine.getDeckLoadGeneration('B')).toBe(0);
    expect(activeA.source.seek).not.toHaveBeenCalled();
    expect(activeB.source.seek).not.toHaveBeenCalled();
    for (const source of [activeA.source, activeB.source]) {
      expect(source.play).not.toHaveBeenCalled();
      expect(source.pause).not.toHaveBeenCalled();
      expect(source.setTempo).not.toHaveBeenCalled();
      expect(source.setKeyLock).not.toHaveBeenCalled();
    }

    handles.dispose();
    expect(gains[0].disconnect).toHaveBeenCalledWith(cueBus);
    expect(gains[1].disconnect).toHaveBeenCalledWith(cueBus);
    expect(previewReference.source.dispose).toHaveBeenCalledOnce();
    expect(previewCandidate.source.dispose).toHaveBeenCalledOnce();
  });

  it('disposes a partially connected headphone pair when connection setup fails', async () => {
    const engine = new DJAudioEngine();
    const reference = fakeSource();
    const candidate = fakeSource();
    const cueBus = {} as GainNode;
    const gains: Array<{ gain: { value: number }; connect: ReturnType<typeof vi.fn>; disconnect: ReturnType<typeof vi.fn> }> = [];
    const audioContext = { createGain: () => {
      const gain = { gain: { value: 1 }, connect: vi.fn(), disconnect: vi.fn() };
      gains.push(gain);
      return gain;
    } } as unknown as AudioContext;
    const referenceOutput = { connect: vi.fn(), disconnect: vi.fn() };
    const candidateOutput = { connect: vi.fn(() => { throw new Error('connect failed'); }), disconnect: vi.fn() };
    Object.assign(reference.source, { outputNode: referenceOutput });
    Object.assign(candidate.source, { outputNode: candidateOutput });
    Object.assign(engine, {
      audioContext, headphoneCueMix: cueBus,
      createSynchronizedPreviewSource: vi.fn().mockReturnValueOnce(reference.source).mockReturnValueOnce(candidate.source),
    });

    await expect(engine.startSynchronizedPreview(
      { id: 'reference' } as never, { id: 'candidate' } as never,
      { referencePosition: 0, candidatePosition: 0, referenceTempo: 1, candidateTempo: 1, stillOwned: () => true },
    )).rejects.toThrow('connect failed');

    expect(referenceOutput.disconnect).toHaveBeenCalledWith(gains[0]);
    expect(gains[0].disconnect).toHaveBeenCalledWith(cueBus);
    expect(reference.source.play).not.toHaveBeenCalled();
    expect(candidate.source.play).not.toHaveBeenCalled();
    expect(reference.source.dispose).toHaveBeenCalledOnce();
    expect(candidate.source.dispose).toHaveBeenCalledOnce();
  });

  it('disposes both headphone sources when ownership changes after preparation', async () => {
    const engine = new DJAudioEngine();
    const reference = fakeSource();
    const candidate = fakeSource();
    const cueBus = {} as GainNode;
    const gains: Array<{ gain: { value: number }; connect: ReturnType<typeof vi.fn>; disconnect: ReturnType<typeof vi.fn> }> = [];
    const audioContext = { createGain: () => {
      const gain = { gain: { value: 1 }, connect: vi.fn(), disconnect: vi.fn() };
      gains.push(gain);
      return gain;
    } } as unknown as AudioContext;
    const referenceOutput = { connect: vi.fn(), disconnect: vi.fn() };
    const candidateOutput = { connect: vi.fn(), disconnect: vi.fn() };
    Object.assign(reference.source, { outputNode: referenceOutput });
    Object.assign(candidate.source, { outputNode: candidateOutput });
    Object.assign(engine, {
      audioContext, headphoneCueMix: cueBus,
      createSynchronizedPreviewSource: vi.fn().mockReturnValueOnce(reference.source).mockReturnValueOnce(candidate.source),
    });
    let checks = 0;

    await expect(engine.startSynchronizedPreview(
      { id: 'reference' } as never, { id: 'candidate' } as never,
      { referencePosition: 0, candidatePosition: 0, referenceTempo: 1, candidateTempo: 1, stillOwned: () => ++checks < 3 },
    )).rejects.toThrow('ownership changed during synchronized preview');

    expect(checks).toBe(3);
    expect(referenceOutput.disconnect).toHaveBeenCalledWith(gains[0]);
    expect(candidateOutput.disconnect).toHaveBeenCalledWith(gains[1]);
    expect(gains[0].disconnect).toHaveBeenCalledWith(cueBus);
    expect(gains[1].disconnect).toHaveBeenCalledWith(cueBus);
    expect(reference.source.play).not.toHaveBeenCalled();
    expect(candidate.source.play).not.toHaveBeenCalled();
    expect(reference.source.dispose).toHaveBeenCalledOnce();
    expect(candidate.source.dispose).toHaveBeenCalledOnce();
  });

  it('aborts pending detached source loads immediately when Stop is pressed', async () => {
    const engine = new DJAudioEngine();
    const reference = fakeSource();
    const candidate = fakeSource();
    const cueBus = {} as GainNode;
    const audioContext = { createGain: () => ({ gain: { value: 1 }, connect: vi.fn(), disconnect: vi.fn() }) } as unknown as AudioContext;
    let rejectReference: ((error: Error) => void) | undefined;
    let rejectCandidate: ((error: Error) => void) | undefined;
    reference.source.load = vi.fn(() => new Promise<string>((_resolve, reject) => { rejectReference = reject; }));
    candidate.source.load = vi.fn(() => new Promise<string>((_resolve, reject) => { rejectCandidate = reject; }));
    reference.source.dispose = vi.fn(() => { reference.source.cancelLoad(); });
    candidate.source.dispose = vi.fn(() => { candidate.source.cancelLoad(); });
    reference.source.cancelLoad = vi.fn(() => { const error = new Error('aborted'); error.name = 'AbortError'; rejectReference?.(error); });
    candidate.source.cancelLoad = vi.fn(() => { const error = new Error('aborted'); error.name = 'AbortError'; rejectCandidate?.(error); });
    Object.assign(reference.source, { outputNode: { connect: vi.fn(), disconnect: vi.fn() } });
    Object.assign(candidate.source, { outputNode: { connect: vi.fn(), disconnect: vi.fn() } });
    Object.assign(engine, {
      audioContext, headphoneCueMix: cueBus,
      createSynchronizedPreviewSource: vi.fn().mockReturnValueOnce(reference.source).mockReturnValueOnce(candidate.source),
    });
    const controller = new AbortController();
    const pending = engine.startSynchronizedPreview(
      { id: 'reference' } as never, { id: 'candidate' } as never,
      { referencePosition: 0, candidatePosition: 0, referenceTempo: 1, candidateTempo: 1, stillOwned: () => true, signal: controller.signal },
    );

    controller.abort();
    await expect(pending).rejects.toMatchObject({ name: 'AbortError' });

    expect(reference.source.cancelLoad).toHaveBeenCalledOnce();
    expect(candidate.source.cancelLoad).toHaveBeenCalledOnce();
    expect(reference.source.dispose).toHaveBeenCalledOnce();
    expect(candidate.source.dispose).toHaveBeenCalledOnce();
    expect(reference.source.play).not.toHaveBeenCalled();
    expect(candidate.source.play).not.toHaveBeenCalled();
  });
});
