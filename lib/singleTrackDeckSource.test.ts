/** Tests and fixtures for single Track Deck Source behavior. */

import { afterEach, describe, expect, it, vi } from 'vitest';
import { SingleTrackDeckSource } from './singleTrackDeckSource';
import { resetEventStreamURLCacheForTests } from '../services/eventStreamURL';

class FakeAudio {
  crossOrigin = '';
  src = '';
  currentTime = 0;
  duration = 12;
  playbackRate = 1;
  preservesPitch = false;
  paused = true;
  readyState = 0;
  error: MediaError | null = null;
  listeners = new Map<string, Set<() => void>>();
  load = vi.fn();
  play = vi.fn(async () => { this.paused = false; });
  pause = vi.fn(() => { this.paused = true; });
  addEventListener(type: string, listener: EventListenerOrEventListenerObject): void {
    const callback = typeof listener === 'function' ? listener as () => void : () => listener.handleEvent(new Event(type));
    const callbacks = this.listeners.get(type) ?? new Set();
    callbacks.add(callback);
    this.listeners.set(type, callbacks);
  }
  removeEventListener(type: string, listener: EventListenerOrEventListenerObject): void {
    if (typeof listener === 'function') this.listeners.get(type)?.delete(listener as () => void);
  }
  emit(type: string): void { this.listeners.get(type)?.forEach(listener => listener()); }
}

function sourceFixture() {
  const audio = new FakeAudio();
  const mediaSource = { connect: vi.fn(), disconnect: vi.fn() };
  const nodes: Array<ReturnType<typeof makeBufferSource>> = [];
  function makeBufferSource() {
    return {
      connect: vi.fn(), disconnect: vi.fn(), start: vi.fn(), stop: vi.fn(),
      playbackRate: { value: 1, setValueAtTime: vi.fn() }, loop: false, loopStart: 0, loopEnd: 0,
    };
  }
  const context = {
    currentTime: 0,
    createGain: vi.fn(() => ({})),
    createMediaElementSource: vi.fn(() => mediaSource),
    createBufferSource: vi.fn(() => {
      const node = makeBufferSource();
      nodes.push(node);
      return node;
    }),
  };
  return { audio, context, nodes, source: new SingleTrackDeckSource(audio as unknown as HTMLAudioElement, context as unknown as AudioContext) };
}

describe('SingleTrackDeckSource', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    resetEventStreamURLCacheForTests();
  });

  it('wraps a quarter beat on the audio clock and preserves key lock when changing tempo', async () => {
    const { audio, context, nodes, source } = sourceFixture();
    audio.readyState = 3;
    await source.load({ id: 'loop', title: 'Loop' } as never);
    source.seek(2);
    source.setLoop(2, 2.125, true);
    source.setLoopBuffer({ duration: 12 } as AudioBuffer);
    source.setKeyLock(true);
    await source.play();
    expect(source.supportsSampleAccurateLoop()).toBe(true);
    expect(nodes[0]).toMatchObject({ loop: true, loopStart: 2, loopEnd: 2.125 });
    expect(nodes[0].start).toHaveBeenCalledWith(0, 2);
    expect(audio.play).not.toHaveBeenCalled();

    context.currentTime = 0.4;
    expect(source.getPosition()).toBeCloseTo(2.025);
    source.setTempo(1.2);
    expect(source.supportsSampleAccurateLoop()).toBe(false);
    expect(nodes[0].stop).toHaveBeenCalledOnce();
    expect(audio.currentTime).toBeCloseTo(2.025);
    expect(audio.play).toHaveBeenCalledOnce();
    expect(audio.preservesPitch).toBe(true);

    source.setKeyLock(false);
    expect(source.supportsSampleAccurateLoop()).toBe(true);
    expect(nodes[1].playbackRate.value).toBe(1.2);
    source.setLoop(0, 0, false);
    expect(nodes[1].stop).toHaveBeenCalledOnce();
    expect(audio.play).toHaveBeenCalledTimes(2);
  });

  it('loads the supplied or fallback URL and resolves when canplay fires', async () => {
    const { audio, source } = sourceFixture();
    const supplied = source.load({ id: 'one', title: 'One', url: '/custom.mp3' } as never);
    expect(audio.src).toBe('/custom.mp3');
    expect(audio.pause).toHaveBeenCalledOnce();
    expect(audio.load).toHaveBeenCalledOnce();
    audio.emit('canplay');
    await expect(supplied).resolves.toBe('/custom.mp3');

    audio.readyState = 3;
    await expect(source.load({ id: 'two', title: 'Two' } as never)).resolves.toBe('/api/audio/two');
    expect(audio.src).toBe('/api/audio/two');
  });

  it('rejects media errors and aborts a superseded load', async () => {
    const { audio, source } = sourceFixture();
    const first = source.load({ id: 'one', title: 'One' } as never);
    const second = source.load({ id: 'two', title: 'Two' } as never);
    await expect(first).rejects.toMatchObject({ name: 'AbortError' });
    audio.emit('canplay');
    await expect(second).resolves.toBe('/api/audio/two');

    audio.readyState = 0;
    const failed = source.load({ id: 'bad', title: 'Bad' } as never);
    audio.error = { message: 'decode failed' } as MediaError;
    audio.emit('error');
    await expect(failed).rejects.toThrow('Failed to load track: Bad - decode failed');
  });

  it('uses the native HTTP endpoint for Plex on macOS', async () => {
    vi.stubGlobal('navigator', { userAgent: 'Macintosh; Mac OS X' });
    vi.stubGlobal('window', {
      location: { protocol: 'wails:', hostname: 'wails' },
      go: { main: { App: { GetServerURL: vi.fn().mockResolvedValue('http://127.0.0.1:34115') } } },
    });
    const { audio, source } = sourceFixture();
    audio.readyState = 3;
    const track = { id: 'plex_song', title: 'Plex', source: 'plex', url: '/api/audio/plex_song' } as const;

    await expect(source.load(track as never)).resolves.toBe('http://127.0.0.1:34115/api/audio/plex_song');
    expect(audio.src).toBe('http://127.0.0.1:34115/api/audio/plex_song');
    await expect(source.load({ ...track, source: undefined } as never))
      .resolves.toBe('http://127.0.0.1:34115/api/audio/plex_song');
    await expect(source.load({ ...track, id: 'local', source: 'local', url: '/api/audio/local' } as never))
      .resolves.toBe('/api/audio/local');
  });

  it('provides playback, seek, tempo, key lock, state, event, and disposal behavior', async () => {
    const { audio, source } = sourceFixture();
    audio.src = 'track.mp3';
    audio.readyState = 2;
    await source.play();
    expect(source.isPlaying()).toBe(true);
    source.seek(-4);
    expect(audio.currentTime).toBe(0);
    source.seek(99);
    expect(audio.currentTime).toBe(12);
    source.setTempo(2);
    expect(audio.playbackRate).toBe(1.5);
    source.setTempo(0.1);
    expect(audio.playbackRate).toBe(0.5);
    source.setKeyLock(true);
    expect(audio.preservesPitch).toBe(true);
    expect(source.getPosition()).toBe(12);
    expect(source.getDuration()).toBe(12);
    expect(source.isLoaded()).toBe(true);

    const ended = vi.fn();
    source.addEventListener('ended', ended);
    audio.emit('ended');
    expect(ended).toHaveBeenCalledOnce();
    source.dispose();
    expect(audio.paused).toBe(true);
    expect(audio.src).toBe('');
  });
});
