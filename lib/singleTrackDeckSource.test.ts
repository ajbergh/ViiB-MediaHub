import { describe, expect, it, vi } from 'vitest';
import { SingleTrackDeckSource } from './singleTrackDeckSource';

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
  const outputNode = { connect: vi.fn() } as unknown as MediaElementAudioSourceNode;
  const context = { createMediaElementSource: vi.fn(() => outputNode) } as unknown as AudioContext;
  return { audio, context, source: new SingleTrackDeckSource(audio as unknown as HTMLAudioElement, context) };
}

describe('SingleTrackDeckSource', () => {
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
