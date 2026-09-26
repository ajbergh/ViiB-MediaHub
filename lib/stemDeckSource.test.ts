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

function fixture(frameRequestOk = true, statusAvailable = true) {
  let currentFrameRequestOk = frameRequestOk;
  const gains: GainNode[] = [];
  const context = { currentTime: 0, createGain: vi.fn(() => { const node = fakeGain(); gains.push(node); return node; }) } as unknown as AudioContext;
  const fallback = fakeDeckSource();
  const worklet = fakeWorklet();
  const fetcher = vi.fn(async (input: RequestInfo | URL) => {
    const url = String(input);
    if (url.endsWith('/song')) return new Response(JSON.stringify(statusAvailable
      ? { activeSetId: 'set-1', stemSets: [{ id: 'set-1', status: 'ready', stemLayout: 'four', sampleRate: 1000, channels: 1, frames: 4000, durationSeconds: 4 }] }
      : { status: 'none', stemSets: [] }), { status: 200 });
    const query = new URL(url, 'http://local').searchParams;
    const start = Number(query.get('startFrame'));
    const count = Number(query.get('frameCount'));
    return response(currentFrameRequestOk, start, count);
  }) as unknown as typeof fetch;
  const source = new StemDeckSource(context, {
    fetch: fetcher,
    createFallback: () => fallback,
    createWorklet: async () => worklet.node,
    chunkFrames: 1024,
    targetBufferSeconds: 2,
  });
  return { source, fallback, worklet, fetcher, gains, setFrameRequestOk: (ok: boolean) => { currentFrameRequestOk = ok; } };
}

describe('StemDeckSource', () => {
  afterEach(() => { vi.restoreAllMocks(); vi.unstubAllGlobals(); });

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
    expect(source.getStemStatus()).toMatchObject({ available: true, mode: 'full' });

    await source.play();
    fallback.position = 1.25;
    await source.setStemMode('stems');
    expect(source.getStemStatus().mode).toBe('stems');
    expect(source.getPosition()).toBeCloseTo(1.25);
    expect(fallback.pause).toHaveBeenCalled();
    expect(worklet.port.postMessage).toHaveBeenCalledWith(expect.objectContaining({ type: 'seek', frame: 1250 }));
    expect(source.getStemStatus()).toMatchObject({ supportsKeyLock: false, supportsScratch: false, supportsSampleAccurateLoop: true });

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
    const base = fixture();
    const controlledFetch = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      if (String(input).includes('/frames?')) return await new Promise<Response>(resolve => { releaseFrameResponse = resolve; });
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

    await source.load({ id: 'song', title: 'Song', url: '/song' } as never);
    await vi.waitFor(() => expect(releaseFrameResponse).toBeTypeOf('function'));
    source.cancelLoad();
    releaseFrameResponse?.(response(true, 0, 1024));
    await vi.waitFor(() => expect(source.getStemStatus().bufferedSeconds).toBe(0));
    expect(base.worklet.port.postMessage).not.toHaveBeenCalledWith(expect.objectContaining({ type: 'frames' }), expect.any(Array));

    source.unload();
    expect(source.getStemStatus()).toMatchObject({ available: false, mode: 'fallback' });
    await source.load({ id: 'song', title: 'Song', url: '/song' } as never);
    expect(source.getStemState()).toEqual({
      vocals: { gain: 1, muted: false, solo: false }, drums: { gain: 1, muted: false, solo: false },
      bass: { gain: 1, muted: false, solo: false }, music: { gain: 1, muted: false, solo: false },
    });
    source.dispose();
  });
});
