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
  Object.assign(engine, { audioElementA: audio, scratchNodes: { A: { port } } });
  return { engine, audio, port };
}

describe('scratch deck handoff', () => {
  afterEach(() => { vi.restoreAllMocks(); vi.unstubAllGlobals(); });

  function animationClock() {
    let now = 0;
    let nextId = 0;
    const callbacks = new Map<number, FrameRequestCallback>();
    vi.spyOn(performance, 'now').mockImplementation(() => now);
    vi.stubGlobal('requestAnimationFrame', (callback: FrameRequestCallback) => {
      callbacks.set(++nextId, callback);
      return nextId;
    });
    vi.stubGlobal('cancelAnimationFrame', (id: number) => callbacks.delete(id));
    return (time: number) => {
      now = time;
      const pending = [...callbacks.values()];
      callbacks.clear();
      pending.forEach(callback => callback(now));
    };
  }

  it.each([-4, 4])('coasts with signed velocity %s and resumes only after settling', velocity => {
    const tick = animationClock();
    const { engine, audio } = deck(false);
    Object.assign(engine, { scratchReady: { A: true } });
    engine.startScratch('A');
    engine.endScratch('A', velocity);
    tick(50);
    expect(Math.sign(engine.getPosition('A') - 20)).toBe(Math.sign(velocity));
    expect(audio.play).not.toHaveBeenCalled();
    tick(2000);
    expect(engine.isScratching('A')).toBe(false);
    expect(audio.play).toHaveBeenCalledOnce();
  });

  it('re-grabbing cancels momentum, and a paused deck coasts to a stop', () => {
    const tick = animationClock();
    const { engine, audio } = deck(true);
    Object.assign(engine, { scratchReady: { A: true } });
    engine.startScratch('A');
    engine.endScratch('A', -3);
    tick(50);
    engine.startScratch('A');
    const held = engine.getPosition('A');
    tick(1000);
    expect(engine.getPosition('A')).toBe(held);
    engine.endScratch('A', 3);
    tick(3000);
    expect(engine.getPosition('A')).toBeGreaterThan(held);
    expect(audio.play).not.toHaveBeenCalled();
    expect(engine.isScratching('A')).toBe(false);
  });

  it('pause cancels an active coast without a delayed resume', () => {
    const tick = animationClock();
    const { engine, audio } = deck(false);
    Object.assign(engine, { scratchReady: { A: true } });
    engine.startScratch('A');
    engine.endScratch('A', 4);
    tick(50);
    engine.pause('A');
    tick(2000);
    expect(engine.isScratching('A')).toBe(false);
    expect(audio.play).not.toHaveBeenCalled();
  });
  it.each([true, false])('preserves paused=%s and avoids media seeks during dragging', paused => {
    const { engine, audio, port } = deck(paused);
    engine.startScratch('A');
    expect(audio.paused).toBe(true);
    engine.updateScratch('A', -0.5, -1);
    expect(audio.currentTime).toBe(20);
    expect(engine.getPosition('A')).toBe(19.5);
    expect(port.postMessage).toHaveBeenLastCalledWith({ type: 'move', position: 19.5 });
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
