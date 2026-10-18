/** Tests and fixtures for stem Deck Source behavior. */

import { afterEach, describe, expect, it, vi } from 'vitest';
import type { DeckSource } from './deckSource';
import { StemDeckSource } from './stemDeckSource';

function fakeGain() {
  return { gain: { value: 0, cancelScheduledValues: vi.fn(), setTargetAtTime: vi.fn() }, connect: vi.fn(), disconnect: vi.fn() } as unknown as GainNode;
}

function fakeDeckSource(): DeckSource & { position: number; playing: boolean; seek: ReturnType<typeof vi.fn> } {
  const source = {
    outputNode: { connect: vi.fn() } as unknown as AudioNode,
    position: 0, playing: false,
    load: vi.fn(async () => '/api/audio/song'), cancelLoad: vi.fn(), unload: vi.fn(),
    play: vi.fn(async function (this: typeof source) { this.playing = true; }),
    pause: vi.fn(function (this: typeof source) { this.playing = false; }),
    seek: vi.fn(function (this: typeof source, position: number) { this.position = position; }),
    setTempo: vi.fn(), setKeyLock: vi.fn(), setLoop: vi.fn(),
    getPosition: vi.fn(function (this: typeof source) { return this.position; }), getDuration: vi.fn(() => 4),
    isPlaying: vi.fn(function (this: typeof source) { return this.playing; }), isLoaded: vi.fn(() => true),
    addEventListener: vi.fn(), removeEventListener: vi.fn(), dispose: vi.fn(),
  };
  return source;
}

function fakeWorklet() {
  const port = {
    onmessage: null as ((event: MessageEvent) => void) | null,
    postMessage: vi.fn(), close: vi.fn(),
  };
  const node = { port, connect: vi.fn(), disconnect: vi.fn() };
  return { node, port };
}

function response(status: boolean, start: number, count: number, sampleRate = 1000, channels = 1): Response {
  const headers = new Headers({
    'X-Start-Frame': String(start), 'X-Frame-Count': String(count), 'X-Sample-Rate': String(sampleRate),
    'X-Layout': 'dj4', 'X-Format': 'f32le', 'X-Channel-Order': 'vocals,drums,bass,music',
    'X-Channel-Count': String(channels * 4),
  });
  const bytes = new ArrayBuffer(count * channels * 4 * 4);
  return new Response(status ? bytes : null, { status: status ? 200 : 409, headers });
}

function fixture(frameRequestOk = true, statusAvailable = true,
  options: { sampleRate?: number; frames?: number; chunkFrames?: number } = {}) {
  const { sampleRate = 1000, frames = 4000, chunkFrames = 1024 } = options;
  let currentFrameRequestOk = frameRequestOk;
  const gains: GainNode[] = [];
  const context = { currentTime: 0, createGain: vi.fn(() => { const node = fakeGain(); gains.push(node); return node; }) } as unknown as AudioContext;
  const fallback = fakeDeckSource();
  const worklet = fakeWorklet();
  const fetcher = vi.fn(async (input: RequestInfo | URL) => {
    const url = String(input);
    if (url.endsWith('/song')) return new Response(JSON.stringify(statusAvailable
      ? { activeSetId: 'set-1', stemSets: [{ id: 'set-1', status: 'ready', stemLayout: 'four', sampleRate, channels: 1, frames, durationSeconds: frames / sampleRate }] }
      : { status: 'none', stemSets: [] }), { status: 200 });
    const query = new URL(url, 'http://local').searchParams;
    const start = Number(query.get('startFrame'));
    const count = Number(query.get('frameCount'));
    return response(currentFrameRequestOk, start, count, sampleRate);
  }) as unknown as typeof fetch;
  const source = new StemDeckSource(context, {
    fetch: fetcher,
    createFallback: () => fallback,
    createWorklet: async () => worklet.node,
    chunkFrames,
    targetBufferSeconds: 2,
  });
  return { source, fallback, worklet, fetcher, gains, context, setFrameRequestOk: (ok: boolean) => { currentFrameRequestOk = ok; } };
}

describe('StemDeckSource', () => {
  afterEach(() => { vi.restoreAllMocks(); vi.unstubAllGlobals(); vi.useRealTimers(); });

  it.each([96000, 192000])('waits for a tempo-adjusted startup buffer at %s Hz with slow frame delivery', async sampleRate => {
    vi.useFakeTimers();
    const { source, fetcher } = fixture(true, true, { sampleRate, frames: sampleRate * 20, chunkFrames: 131072 });
    const original = vi.mocked(fetcher).getMockImplementation()!;
    vi.mocked(fetcher).mockImplementation(async (input, init) => {
      if (String(input).includes('/frames?')) await new Promise(resolve => setTimeout(resolve, 100));
      return original(input, init);
    });
    source.setTempo(1.5);
    let loaded = false;
    const loading = source.load({ id: 'song', title: 'Song' } as never).then(() => { loaded = true; });
    try {
      await vi.advanceTimersByTimeAsync(100);
      expect(loaded).toBe(false);
      await vi.advanceTimersByTimeAsync(1000);
      await loading;
      expect(source.getStemStatus()).toMatchObject({ mode: 'stems', underruns: 0 });
      expect(source.getStemStatus().bufferedSeconds / 1.5).toBeGreaterThanOrEqual(2);
      const requests = vi.mocked(fetcher).mock.calls.filter(([url]) => String(url).includes('/frames?'));
      expect(requests.length).toBe(Math.ceil(sampleRate * 3 / 131072));
      for (const [url] of requests) expect(Number(new URL(String(url), 'http://local').searchParams.get('frameCount')) * 32).toBeLessThanOrEqual(4 * 1024 * 1024);
    } finally { source.dispose(); }
  });

  it('ignores expected cancellation during rapid seeks and resumes the newest position', async () => {
    const { source, fetcher, worklet } = fixture();
    await source.load({ id: 'song', title: 'Song' } as never);
    await source.play();
    const original = vi.mocked(fetcher).getMockImplementation()!;
    let blocked = false;
    vi.mocked(fetcher).mockImplementationOnce((_input, init) => new Promise((_resolve, reject) => {
      blocked = true;
      init?.signal?.addEventListener('abort', () => reject(new DOMException('aborted', 'AbortError')));
    })).mockImplementation(original);
    try {
      source.seek(3.5);
      expect(blocked).toBe(true);
      source.seek(0.1);
      await vi.waitFor(() => expect(source.getStemStatus().bufferedSeconds).toBeGreaterThan(1.9));
      expect(source.getStemStatus().mode).toBe('stems');
      expect(source.getStemStatus().error).toBeUndefined();
      expect(source.getPosition()).toBeCloseTo(0.1);
      expect(worklet.port.postMessage).toHaveBeenLastCalledWith({ type: 'play', rate: 1 });
    } finally { source.dispose(); }
  });

  it('does not let a delayed stems transition override a newer Full request', async () => {
    const { source, fetcher } = fixture();
    await source.load({ id: 'song', title: 'Song' } as never);
    await source.setStemMode('full');
    let settle!: (value: Response) => void;
    vi.mocked(fetcher).mockImplementationOnce(() => new Promise(resolve => { settle = resolve; }));
    try {
      const pending = source.setStemMode('stems');
      await source.setStemMode('full');
      settle(response(false, 0, 1024));
      await pending;
      expect(source.getStemStatus().mode).toBe('full');
      expect(source.getStemStatus().error).toBeUndefined();
    } finally { source.dispose(); }
  });

  it('holds its position estimate during an underrun and restarts it on recovery', async () => {
    const { source, worklet, context } = fixture();
    try {
      await source.load({ id: 'song', title: 'Song' } as never);
      await source.play();
      worklet.port.onmessage?.({ data: { type: 'underrun', frame: 750 } } as MessageEvent);
      Object.assign(context, { currentTime: 10 });
      worklet.port.onmessage?.({ data: { type: 'position', frame: 1500 } } as MessageEvent);
      expect(source.getPosition()).toBe(0.75);
      worklet.port.onmessage?.({ data: { type: 'recovered', frame: 750 } } as MessageEvent);
      Object.assign(context, { currentTime: 10.25 });
      expect(source.getPosition()).toBe(1);
    } finally { source.dispose(); }
  });

  it('ignores a failed old-track seek after a new track has loaded', async () => {
    const { source, fetcher } = fixture();
    await source.load({ id: 'song', title: 'First' } as never);
    let settle!: (value: Response) => void;
    vi.mocked(fetcher).mockImplementationOnce(() => new Promise(resolve => { settle = resolve; }));
    try {
      source.seek(3.5);
      await source.load({ id: 'song', title: 'Next' } as never);
      settle(response(false, 3500, 500));
      await new Promise(resolve => setTimeout(resolve, 0));
      expect(source.getStemStatus()).toMatchObject({ mode: 'stems', available: true });
      expect(source.getStemStatus().error).toBeUndefined();
    } finally { source.dispose(); }
  });

  it('ignores stale loop failures and refills the loop after a seek flush', async () => {
    const { source, fetcher, worklet } = fixture();
    await source.load({ id: 'song', title: 'Song' } as never);
    const original = vi.mocked(fetcher).getMockImplementation()!;
    let settle!: (value: Response) => void;
    vi.mocked(fetcher).mockImplementationOnce(() => new Promise(resolve => { settle = resolve; })).mockImplementation(original);
    try {
      source.setLoop(0.5, 1.5, true);
      source.setLoop(0.6, 1.6, true);
      settle(response(false, 500, 1000));
      await vi.waitFor(() => expect(worklet.port.postMessage).toHaveBeenCalledWith(expect.objectContaining({ type: 'frames', startFrame: 600 }), expect.any(Array)));
      expect(source.getStemStatus()).toMatchObject({ mode: 'stems' });
      expect(source.getStemStatus().error).toBeUndefined();
      await source.play();
      worklet.port.postMessage.mockClear();
      source.seek(3);
      await vi.waitFor(() => expect(worklet.port.postMessage).toHaveBeenCalledWith({ type: 'play', rate: 1 }));
      const messages = worklet.port.postMessage.mock.calls.map(([message]) => message);
      expect(messages.findIndex(message => message.type === 'frames' && message.startFrame === 600)).toBeGreaterThan(0);
      expect(messages.at(-1)).toEqual({ type: 'play', rate: 1 });
    } finally { source.dispose(); }
  });

  it('calls the default global fetch with a Window receiver', async () => {
    // Native fetch throws "Illegal invocation" when called as a method of
    // another object; this stub enforces the same receiver rule.
    const nativeLike = vi.fn(function (this: unknown) {
      if (this !== undefined && this !== globalThis) throw new TypeError("Failed to execute 'fetch' on 'Window': Illegal invocation");
      return Promise.resolve(new Response(JSON.stringify({ status: 'none', stemSets: [] }), { status: 200 }));
    });
    vi.stubGlobal('fetch', nativeLike);
    const context = { currentTime: 0, createGain: vi.fn(fakeGain) } as unknown as AudioContext;
    const source = new StemDeckSource(context, { createFallback: fakeDeckSource, createWorklet: async () => fakeWorklet().node });
    await source.load({ id: 'song', title: 'Song', url: '/song' } as never);
    expect(nativeLike).toHaveBeenCalled();
    expect(source.getStemStatus().error).toBeUndefined();
  });

  it('packs all four buses behind one worklet clock and switches at the same position', async () => {
    const { source, fallback, worklet, fetcher } = fixture();
    await source.load({ id: 'song', title: 'Song', url: '/song' } as never);
    await vi.waitFor(() => expect(worklet.port.postMessage).toHaveBeenCalledWith(expect.objectContaining({ type: 'frames', startFrame: 0, channels: 1 }), expect.any(Array)));
    expect(fetcher).toHaveBeenCalledWith('/api/v2/stems/song', expect.objectContaining({ cache: 'no-store' }));
    expect(worklet.node.connect).toHaveBeenCalledTimes(4);
    expect(source.getStemStatus()).toMatchObject({ available: true, mode: 'stems' });

    await source.setStemMode('full');
    await source.play();
    fallback.position = 1.25;
    await source.setStemMode('stems');
    expect(source.getStemStatus().mode).toBe('stems');
    expect(source.getPosition()).toBeCloseTo(1.25);
    expect(fallback.pause).toHaveBeenCalled();
    expect(worklet.port.postMessage).toHaveBeenCalledWith(expect.objectContaining({ type: 'seek', frame: 1250 }));
    expect(source.getStemStatus()).toMatchObject({ supportsKeyLock: false, supportsScratch: true, supportsSampleAccurateLoop: true });

    source.setStemGain('vocals', 0.5);
    source.setStemMuted('drums', true);
    source.setStemSolo('bass', true);
    expect(source.getStemState()).toMatchObject({ vocals: { gain: 0.5 }, drums: { muted: true }, bass: { solo: true } });
    source.setLoop(0.5, 1.5, true);
    expect(worklet.port.postMessage).toHaveBeenCalledWith(expect.objectContaining({ type: 'loop', startFrame: 500, endFrame: 1500, enabled: true }));
    expect(source.getStemStatus().supportsSampleAccurateLoop).toBe(true);
    source.setLoop(0.25, 3.25, true);
    expect(source.getStemStatus().supportsSampleAccurateLoop).toBe(false);
    source.setLoop(0.5, 1.5, true);
    source.setTempo(1.2);
    expect(worklet.port.postMessage).toHaveBeenCalledWith({ type: 'rate', rate: 1.2 });
    source.dispose();
    expect(worklet.port.close).toHaveBeenCalledOnce();
  });

  it('applies independent gain, mute, and solo state to the four stem buses', async () => {
    const { source, gains } = fixture();
    await source.load({ id: 'song', title: 'Song', url: '/song' } as never);
    await vi.waitFor(() => expect(source.getStemStatus().bufferedSeconds).toBeGreaterThan(0));
    await source.setStemMode('stems');

    source.setStemGain('vocals', 0.4);
    source.setStemMuted('drums', true);
    source.setStemSolo('bass', true);
    expect(gains[2].gain.setTargetAtTime).toHaveBeenLastCalledWith(0, 0, 0.01);
    expect(gains[3].gain.setTargetAtTime).toHaveBeenLastCalledWith(0, 0, 0.01);
    expect(gains[4].gain.setTargetAtTime).toHaveBeenLastCalledWith(1, 0, 0.01);
    expect(gains[5].gain.setTargetAtTime).toHaveBeenLastCalledWith(0, 0, 0.01);

    source.setStemSolo('bass', false);
    expect(gains[2].gain.setTargetAtTime).toHaveBeenLastCalledWith(0.4, 0, 0.01);
    expect(gains[3].gain.setTargetAtTime).toHaveBeenLastCalledWith(0, 0, 0.01);
    expect(gains[4].gain.setTargetAtTime).toHaveBeenLastCalledWith(1, 0, 0.01);
    expect(gains[5].gain.setTargetAtTime).toHaveBeenLastCalledWith(1, 0, 0.01);
    source.dispose();
  });

  it('falls back safely when frame serving fails and reports the error', async () => {
    const { source, fallback } = fixture(false);
    await source.load({ id: 'song', title: 'Song', url: '/song' } as never);
    await vi.waitFor(() => expect(source.getStemStatus().mode).toBe('fallback'));
    expect(source.getStemStatus()).toMatchObject({ available: true, supportsScratch: true });
    expect(source.getStemStatus().error).toContain('Stem frame request failed');
    await source.setStemMode('stems');
    expect(source.getStemStatus().mode).toBe('fallback');
    expect(fallback.isLoaded()).toBe(true);
  });

  it('rejects malformed ready-package frames before selecting stems', async () => {
    const { source, fetcher, fallback } = fixture();
    const fetch = vi.mocked(fetcher);
    const original = fetch.getMockImplementation()!;
    fetch.mockImplementation(async (input, init) => {
      if (String(input).includes('/frames?')) return new Response(new ArrayBuffer(4), { status: 200 });
      return original(input, init);
    });
    try {
      await source.load({ id: 'song', title: 'Song', url: '/song' } as never);
      expect(source.getStemStatus()).toMatchObject({ mode: 'fallback', error: 'Stem frame response geometry did not match the active package' });
      await source.play();
      expect(fallback.play).toHaveBeenCalled();
    } finally { source.dispose(); }
  });

  it('rechecks each new song instead of inheriting a previous manual Full-mode choice', async () => {
    const { source, fetcher, gains, fallback } = fixture();
    try {
      const track = { id: 'song', title: 'Song', url: '/song' } as never;
      await source.load(track);
      expect(source.getStemStatus().mode).toBe('stems');
      await source.setStemMode('full');
      source.setStemMuted('vocals', true);
      await source.load(track);
      expect(source.getStemStatus().mode).toBe('stems');
      expect(source.getStemState().vocals.muted).toBe(false);
      vi.mocked(fetcher).mockResolvedValue(new Response(JSON.stringify({ status: 'none', stemSets: [] }), { status: 200 }));
      await source.load({ id: 'no-stems', title: 'No stems', url: '/no-stems' } as never);
      expect(source.getStemStatus()).toMatchObject({ available: false, mode: 'fallback' });
      expect(gains[1].gain.setTargetAtTime).toHaveBeenLastCalledWith(1, 0, 0.01);
      for (const gain of gains.slice(2)) expect(gain.gain.setTargetAtTime).toHaveBeenLastCalledWith(0, 0, 0.01);
      await source.play();
      expect(fallback.play).toHaveBeenCalled();
    } finally { source.dispose(); }
  });

  it('does not let a superseded frame failure switch the newly loaded song out of stems', async () => {
    const { source, fetcher } = fixture();
    const fetch = vi.mocked(fetcher);
    const original = fetch.getMockImplementation()!;
    let settle!: (response: Response) => void;
    let first = true;
    fetch.mockImplementation(async (input, init) => {
      if (first && String(input).includes('/frames?')) {
        first = false;
        return new Promise(resolve => { settle = resolve; });
      }
      return original(String(input).includes('/frames?') ? input : '/api/v2/stems/song', init);
    });
    try {
      const oldLoad = source.load({ id: 'old', title: 'Old', url: '/old' } as never);
      const cancelled = expect(oldLoad).rejects.toMatchObject({ name: 'AbortError' });
      await vi.waitFor(() => expect(settle).toBeTypeOf('function'));
      await source.load({ id: 'new', title: 'New', url: '/new' } as never);
      expect(source.getStemStatus().mode).toBe('stems');
      settle(response(false, 0, 1024));
      await cancelled;
      expect(source.getStemStatus()).toMatchObject({ mode: 'stems', available: true });
      expect(source.getStemStatus().error).toBeUndefined();
    } finally { source.dispose(); }
  });

  it('preserves the current stem position when frame loading falls back to the full-track source', async () => {
    const { source, worklet, fallback, setFrameRequestOk } = fixture();
    await source.load({ id: 'song', title: 'Song', url: '/song' } as never);
    await vi.waitFor(() => expect(worklet.port.postMessage).toHaveBeenCalledWith(expect.objectContaining({ type: 'frames' }), expect.any(Array)));
    await source.setStemMode('stems');
    await vi.waitFor(() => expect(source.getStemStatus().bufferedSeconds).toBeGreaterThan(1));
    await source.play();
    worklet.port.onmessage?.({ data: { type: 'position', frame: 2345 } } as MessageEvent);
    source.setStemGain('vocals', 0.7);
    setFrameRequestOk(false);
    worklet.port.onmessage?.({ data: { type: 'underrun', frame: 2345 } } as MessageEvent);
    await vi.waitFor(() => expect(source.getStemStatus().mode).toBe('fallback'));

    expect(source.getStemStatus()).toMatchObject({ mode: 'fallback', error: 'Stem frame request failed (409)' });
    expect(fallback.seek).toHaveBeenLastCalledWith(2.345);
    expect(source.getPosition()).toBe(2.345);
    source.dispose();
  });

  it('counts a worklet underrun and schedules another bounded prefetch', async () => {
    const { source, worklet, fetcher } = fixture();
    await source.load({ id: 'song', title: 'Song', url: '/song' } as never);
    await vi.waitFor(() => expect(worklet.port.postMessage).toHaveBeenCalledWith(expect.objectContaining({ type: 'frames' }), expect.any(Array)));
    const frameCalls = (fetcher as unknown as ReturnType<typeof vi.fn>).mock.calls.length;
    worklet.port.onmessage?.({ data: { type: 'position', frame: 2050 } } as MessageEvent);
    worklet.port.onmessage?.({ data: { type: 'underrun', frame: 2050 } } as MessageEvent);
    expect(source.getStemStatus().underruns).toBe(1);
    await vi.waitFor(() => expect((fetcher as unknown as ReturnType<typeof vi.fn>).mock.calls.length).toBeGreaterThan(frameCalls));
    source.dispose();
  });

  it('uses full-track fallback when the status endpoint has no ready set and disposes all sources', async () => {
    const f = fixture(true, false);
    await f.source.load({ id: 'song', title: 'Song', url: '/song' } as never);
    expect(f.source.getStemStatus()).toMatchObject({ available: false, mode: 'fallback' });
    expect(f.fallback.load).toHaveBeenCalledOnce();
    f.source.dispose();
    expect(f.fallback.dispose).toHaveBeenCalledOnce();
    expect(f.worklet.port.close).not.toHaveBeenCalled();
    expect(f.worklet.node.disconnect).not.toHaveBeenCalled();
  });

  it('cancels stale frame responses, then unloads and reloads with fresh stem state', async () => {
    let releaseFrameResponse: ((response: Response) => void) | undefined;
    let delayFrames = true;
    const base = fixture();
    const controlledFetch = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      if (delayFrames && String(input).includes('/frames?')) return await new Promise<Response>(resolve => { releaseFrameResponse = resolve; });
      return base.fetcher(input, init);
    }) as unknown as typeof fetch;
    const source = new StemDeckSource({
      currentTime: 0, createGain: () => fakeGain(),
    } as unknown as AudioContext, {
      fetch: controlledFetch,
      createFallback: () => base.fallback,
      createWorklet: async () => base.worklet.node,
      chunkFrames: 1024,
      targetBufferSeconds: 2,
    });

    const loading = source.load({ id: 'song', title: 'Song', url: '/song' } as never);
    const cancelled = expect(loading).rejects.toMatchObject({ name: 'AbortError' });
    await vi.waitFor(() => expect(releaseFrameResponse).toBeTypeOf('function'));
    source.cancelLoad();
    releaseFrameResponse?.(response(true, 0, 1024));
    await cancelled;
    await vi.waitFor(() => expect(source.getStemStatus().bufferedSeconds).toBe(0));
    expect(base.worklet.port.postMessage).not.toHaveBeenCalledWith(expect.objectContaining({ type: 'frames' }), expect.any(Array));

    source.unload();
    expect(source.getStemStatus()).toMatchObject({ available: false, mode: 'fallback' });
    delayFrames = false;
    await source.load({ id: 'song', title: 'Song', url: '/song' } as never);
    expect(source.getStemStatus().mode).toBe('stems');
    expect(source.getStemState()).toEqual({
      vocals: { gain: 1, muted: false, solo: false }, drums: { gain: 1, muted: false, solo: false },
      bass: { gain: 1, muted: false, solo: false }, music: { gain: 1, muted: false, solo: false },
    });
    source.dispose();
  });

  it('relays key lock, stretcher readiness and scratch messages through the stem worklet', async () => {
    const { source, worklet, fallback } = fixture();
    source.setKeyLock(true);
    await source.load({ id: 'song', title: 'Song', url: '/song' } as never);
    expect(worklet.port.postMessage).toHaveBeenCalledWith({ type: 'keyLock', enabled: true });
    await source.setStemMode('stems');
    expect(source.getStemStatus().supportsKeyLock).toBe(false);
    worklet.port.onmessage?.({ data: { type: 'stretch', ready: true } } as MessageEvent);
    expect(source.getStemStatus()).toMatchObject({ mode: 'stems', supportsKeyLock: true, supportsScratch: true });

    source.setKeyLock(false);
    expect(worklet.port.postMessage).toHaveBeenLastCalledWith({ type: 'keyLock', enabled: false });
    expect(fallback.setKeyLock).toHaveBeenLastCalledWith(false);
    expect(source.canScratch()).toBe(true);
    source.scratchPort.postMessage({ type: 'start', position: 1, token: 3 });
    expect(worklet.port.postMessage).toHaveBeenLastCalledWith({ type: 'scratch', command: { type: 'start', position: 1, token: 3 } });
    const events: unknown[] = [];
    source.onScratchEvent = event => events.push(event);
    worklet.port.onmessage?.({ data: { type: 'scratch', event: { type: 'held', token: 3, position: 0.9 } } } as MessageEvent);
    expect(events).toEqual([{ type: 'held', token: 3, position: 0.9 }]);
    source.dispose();
  });

  it('seeks inside retained stem frames without a flush and refetches outside them', async () => {
    const { source, worklet } = fixture();
    await source.load({ id: 'song', title: 'Song', url: '/song' } as never);
    await source.setStemMode('stems');
    await source.play();
    await vi.waitFor(() => expect(source.getStemStatus().bufferedSeconds).toBeGreaterThan(1.5));
    const post = worklet.port.postMessage;

    post.mockClear();
    source.seek(1);
    expect(post).toHaveBeenCalledWith(expect.objectContaining({ type: 'seek', frame: 1000 }));
    expect(post).not.toHaveBeenCalledWith(expect.objectContaining({ type: 'flush' }));

    // The worklet has dropped history before frame 900; 1.2 s is inside the safety margin.
    worklet.port.onmessage?.({ data: { type: 'evicted', frame: 900 } } as MessageEvent);
    post.mockClear();
    source.seek(1.2);
    expect(post).toHaveBeenCalledWith(expect.objectContaining({ type: 'flush' }));
    source.dispose();
  });
  it('tops up the stem read-ahead from position reports instead of waiting for an underrun', async () => {
    const { source, worklet, fetcher } = fixture();
    await source.load({ id: 'song', title: 'Song', url: '/song' } as never);
    await source.setStemMode('stems');
    await source.play();
    await vi.waitFor(() => expect(source.getStemStatus().bufferedSeconds).toBeGreaterThan(1.5));
    const calls = (fetcher as unknown as ReturnType<typeof vi.fn>).mock.calls.length;
    worklet.port.onmessage?.({ data: { type: 'position', frame: 1500 } } as MessageEvent);
    await vi.waitFor(() => expect((fetcher as unknown as ReturnType<typeof vi.fn>).mock.calls.length).toBeGreaterThan(calls));
    expect(source.getStemStatus().underruns).toBe(0);
    source.dispose();
  });
});
