import { afterEach, describe, expect, it, vi } from 'vitest';
import { VinylScratchTransport } from './vinylScratch.worklet.js';
import { vinylMomentum } from './vinylMomentum';

vi.mock('../store', () => ({ useStore: { getState: () => state } }));
import { DJAudioEngine } from './djAudio';

const state = {
  djDeckA: { tempo: 1 }, djDeckB: { tempo: 1 },
  djMixer: { slipModeA: false, slipModeB: false },
  setDeckPosition: vi.fn(), setDeckPlaying: vi.fn(),
};

function render(transport: VinylScratchTransport, frames = 4800) {
  const output = [new Float32Array(frames), new Float32Array(frames)];
  transport.render(output);
  return output;
}

describe('vinyl sample transport', () => {
  it('reads the recording in both directions and settles silently at the held position', () => {
    const transport = new VinylScratchTransport(48000);
    const ramp = Float32Array.from({ length: 48000 }, (_, i) => i / 48000);
    transport.command({ type: 'load', channels: [ramp] });
    transport.command({ type: 'start', position: 0.5 });
    expect(render(transport)[0].every(value => value === 0)).toBe(true);
    transport.command({ type: 'move', position: 0.7 });
    const forward = render(transport, 960);
    expect(forward[0][900]).toBeGreaterThan(forward[0][400]);
    transport.command({ type: 'move', position: 0.3 });
    const backward = render(transport, 960);
    expect(backward[0][900]).toBeLessThan(backward[0][400]);
    expect(backward[1]).toEqual(backward[0]);
    render(transport, 24000);
    expect(transport.position / 48000).toBeCloseTo(0.3, 4);
    expect(Math.max(...render(transport)[0].map(Math.abs))).toBeLessThan(0.0001);
  });

  it('stays finite at track edges and fades out on release', () => {
    const transport = new VinylScratchTransport(48000);
    transport.command({ type: 'load', channels: [new Float32Array(48000).fill(0.5)] });
    transport.command({ type: 'start', position: 0.5 });
    transport.command({ type: 'move', position: -10 });
    expect(render(transport)[0].every(Number.isFinite)).toBe(true);
    transport.command({ type: 'move', position: 10 });
    render(transport);
    transport.command({ type: 'stop' });
    const stopped = render(transport);
    expect(Math.abs(stopped[0].at(-1)!)).toBeLessThan(0.0001);
    expect(transport.position).toBeGreaterThanOrEqual(0);
    expect(transport.position).toBeLessThan(48000);
  });
});

function deck(paused: boolean) {
  const engine = new DJAudioEngine();
  const audio = { src: 'track.mp3', currentTime: 20, duration: 100, playbackRate: 1, paused,
    pause: vi.fn(function () { audio.paused = true; }),
    play: vi.fn(async function () { audio.paused = false; }),
  };
  const port = { postMessage: vi.fn() };
  Object.assign(engine, { audioElementA: audio, scratchNodes: { A: { port } }, scratchReady: { A: true } });
  return { engine, audio, port };
}

describe('scratch deck handoff', () => {
  afterEach(() => { vi.restoreAllMocks(); vi.unstubAllGlobals(); });

  function message(engine: DJAudioEngine, payload: object) {
    (engine as unknown as { handleScratchMessage: (deck: string, value: object) => void }).handleScratchMessage('A', payload);
  }

  it('leaves playback untouched until the processor acknowledges readiness', () => {
    const { engine, audio } = deck(false);
    Object.assign(engine, { scratchReady: {} });
    expect(engine.startScratch('A')).toBe(false);
    expect(audio.pause).not.toHaveBeenCalled();
    expect(engine.isScratching('A')).toBe(false);
    message(engine, { type: 'ready' });
    expect(engine.startScratch('A')).toBe(true);
    expect(audio.pause).toHaveBeenCalledOnce();
  });

  it.each([-4, 4])('delegates signed coast %s without scheduling animation frames', velocity => {
    const raf = vi.fn(() => { throw new Error('Audio must not depend on RAF'); });
    vi.stubGlobal('requestAnimationFrame', raf);
    const { engine, audio, port } = deck(false);
    engine.startScratch('A');
    engine.endScratch('A', velocity);
    const coast = port.postMessage.mock.calls.at(-1)![0];
    expect(coast).toMatchObject({ type: 'coast', velocity, targetRate: 1 });
    message(engine, { type: 'position', token: coast.token, position: 19, time: 1, rate: -1 });
    expect(engine.getPosition('A')).toBe(19);
    expect(audio.play).not.toHaveBeenCalled();
    message(engine, { type: 'settled', token: coast.token, position: 20, time: 2, rate: 1 });
    expect(audio.play).toHaveBeenCalledOnce();
    expect(raf).not.toHaveBeenCalled();
  });

  it('ignores stale completion after re-grabbing or pausing', () => {
    const { engine, audio, port } = deck(false);
    engine.startScratch('A');
    engine.endScratch('A', -3);
    const token = port.postMessage.mock.calls.at(-1)![0].token;
    expect(engine.startScratch('A')).toBe(true);
    expect(port.postMessage.mock.calls.at(-1)![0].type).toBe('hold');
    message(engine, { type: 'settled', token, position: 30, time: 1, rate: 1 });
    expect(audio.play).not.toHaveBeenCalled();
    expect(engine.getPosition('A')).toBe(20);
    engine.endScratch('A', 3);
    const nextToken = port.postMessage.mock.calls.at(-1)![0].token;
    engine.pause('A');
    message(engine, { type: 'settled', token: nextToken, position: 30, time: 2, rate: 1 });
    expect(audio.play).not.toHaveBeenCalled();
    expect(engine.isScratching('A')).toBe(false);
  });

  it('keeps a paused deck stopped after audio-thread coast completion', () => {
    const { engine, audio, port } = deck(true);
    engine.startScratch('A');
    engine.endScratch('A', 3);
    const coast = port.postMessage.mock.calls.at(-1)![0];
    expect(coast.targetRate).toBe(0);
    message(engine, { type: 'settled', token: coast.token, position: 21, rate: 0, time: 1 });
    expect(audio.currentTime).toBe(21);
    expect(audio.play).not.toHaveBeenCalled();
  });

  it.each([true, false])('preserves paused=%s and avoids media seeks during dragging', paused => {
    const { engine, audio, port } = deck(paused);
    engine.startScratch('A');
    expect(audio.paused).toBe(true);
    engine.updateScratch('A', -0.5, -1);
    expect(audio.currentTime).toBe(20);
    expect(engine.getPosition('A')).toBe(19.5);
    expect(port.postMessage).toHaveBeenLastCalledWith({ type: 'move', delta: -0.5 });
    engine.endScratch('A');
    expect(audio.currentTime).toBe(19.5);
    expect(audio.play).toHaveBeenCalledTimes(paused ? 0 : 1);
    expect(audio.paused).toBe(paused);
  });

  it('honors play/pause while held and cancellation without resuming', async () => {
    const { engine, audio } = deck(false);
    engine.startScratch('A');
    expect(await engine.togglePlay('A')).toBe(false);
    engine.endScratch('A');
    expect(audio.play).not.toHaveBeenCalled();
    engine.startScratch('A');
    await engine.play('A');
    expect(audio.paused).toBe(true);
    engine.endScratch('A', 0, false);
    expect(audio.play).not.toHaveBeenCalled();
  });

  it('slip mode follows elapsed transport time only for a playing deck', () => {
    const now = vi.spyOn(performance, 'now').mockReturnValue(1000);
    state.djMixer.slipModeA = true;
    for (const paused of [true, false]) {
      now.mockReturnValue(1000);
      const { engine, audio } = deck(paused);
      engine.startScratch('A');
      engine.updateScratch('A', -2, -1);
      now.mockReturnValue(2000);
      engine.endScratch('A');
      expect(audio.currentTime).toBe(paused ? 18 : 21);
    }
    state.djMixer.slipModeA = false;
    now.mockRestore();
  });
});

describe('vinyl momentum curve', () => {
  it('keeps backward travel before the motor brings it forward', () => {
    expect(vinylMomentum(-4, 1, 0.1).distance).toBeLessThan(0);
    const nearEnd = vinylMomentum(-4, 1, 1.19);
    const end = vinylMomentum(-4, 1, 1.21);
    expect(end.distance).toBeGreaterThan(nearEnd.distance);
    expect(end.done).toBe(true);
  });
  it('settles a paused platter at a fixed distance', () => {
    expect(vinylMomentum(4, 0, 2).distance).toBe(vinylMomentum(4, 0, 10).distance);
    expect(vinylMomentum(-4, 0, 2).distance).toBe(-vinylMomentum(4, 0, 2).distance);
  });
});


describe('audio-thread momentum', () => {
  it.each([-4, 4])('renders a complete coast at %s without any main-thread updates', velocity => {
    const transport = new VinylScratchTransport(48000);
    const samples = Float32Array.from({ length: 480000 }, (_, i) => Math.sin(i / 20) * 0.5);
    transport.command({ type: 'load', channels: [samples] });
    transport.command({ type: 'start', position: 4, token: 1 });
    transport.command({ type: 'coast', velocity, targetRate: 1, token: 2 });
    const initial = transport.position;
    render(transport, 2400);
    expect(Math.sign(transport.position - initial)).toBe(Math.sign(velocity));
    // Simulate a blocked UI: no move messages or timers for three seconds.
    const output = render(transport, 144000)[0];
    expect(transport.coast?.done).toBe(true);
    expect(transport.rate).toBeCloseTo(1);
    expect(output.every(Number.isFinite)).toBe(true);
    expect(Math.max(...output.slice(-4800).map(Math.abs))).toBeGreaterThan(0.4);
  });

  it('stops a paused coast and lets a re-grab brake the moving record', () => {
    const transport = new VinylScratchTransport(48000);
    transport.command({ type: 'load', channels: [new Float32Array(480000).fill(0.5)] });
    transport.command({ type: 'start', position: 4, token: 1 });
    transport.command({ type: 'coast', velocity: -4, targetRate: 0, token: 2 });
    render(transport, 144000);
    expect(Math.abs(transport.rate)).toBeLessThan(0.001);
    expect(Math.max(...render(transport)[0].map(Math.abs))).toBeLessThan(0.001);
    transport.command({ type: 'coast', velocity: 4, targetRate: 1, token: 3 });
    render(transport, 2400);
    transport.command({ type: 'hold', token: 4 });
    const held = transport.position;
    render(transport, 24000);
    expect(transport.position).toBeCloseTo(held);
    expect(transport.coast).toBeNull();
  });
});
