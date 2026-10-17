// @vitest-environment jsdom
import { afterEach, expect, it, vi } from 'vitest';
import { CanvasWaveformDriver } from './canvasWaveformDriver';
import type { MainWaveformFrame } from './canvasWaveformRenderer';
import type { WaveformWorkerResponse } from './waveformWorkerProtocol';

function setup() {
  const methods = new Map<string, ReturnType<typeof vi.fn>>();
  const ctx = new Proxy({}, { get: (_, key) => { if (!methods.has(String(key))) methods.set(String(key), vi.fn()); return methods.get(String(key)); }, set: () => true }) as CanvasRenderingContext2D;
  const worker = { postMessage: vi.fn(), terminate: vi.fn(), onmessage: null as ((event: MessageEvent<WaveformWorkerResponse>) => void) | null, onerror: null as (() => void) | null };
  const canvas = document.createElement('canvas');
  const driver = new CanvasWaveformDriver(canvas, ctx, () => worker as unknown as Worker);
  const send = (data: WaveformWorkerResponse) => worker.onmessage!({ data } as MessageEvent<WaveformWorkerResponse>);
  const frame: MainWaveformFrame = { deck: 'A', sourceKey: 'one', peaks: null, position: 0, duration: 0,
    beatGrid: null, beatGridOffset: 0, cuePoint: 0, hotCues: [], loop: { enabled: false, start: 0, end: 0 }, visibleSeconds: 10, colorMode: 'single', hasTrack: false };
  return { driver, worker, send, frame, canvas, methods };
}
afterEach(() => vi.useRealTimers());

it('sends immutable source state once and coalesces position updates while a bitmap is outstanding', () => {
  const { driver, worker, send, frame, methods } = setup();
  try {
    send({ type: 'ready' });
    driver.draw(frame, 100, 40, 1);
    driver.draw({ ...frame, position: 1 }, 100, 40, 1);
    driver.draw({ ...frame, position: 2 }, 100, 40, 1);
    expect(worker.postMessage).toHaveBeenCalledTimes(2); // state + one draw
    const bitmap = { close: vi.fn() } as unknown as ImageBitmap;
    send({ type: 'frame', generation: 1, bitmap });
    expect(worker.postMessage).toHaveBeenCalledTimes(3);
    expect(worker.postMessage.mock.calls[2][0]).toMatchObject({ type: 'draw', position: 2 });
    expect(methods.get('drawImage')).toHaveBeenCalled();
    expect(bitmap.close).toHaveBeenCalled();
    driver.draw({ ...frame, hotCues: [{ slot: 1, position: 1, color: 'blue' }] }, 100, 40, 1);
    send({ type: 'frame', generation: 1, bitmap });
    const patch = worker.postMessage.mock.calls[3][0];
    expect(patch.type).toBe('state');
    expect(patch.frame.hotCues).toHaveLength(1);
    expect(patch.frame).not.toHaveProperty('peaks');
  } finally { driver.dispose(); }
  expect(worker.terminate).toHaveBeenCalledOnce();
});

it('discards a bitmap when newer source data is pending', () => {
  const { driver, send, frame, methods, worker } = setup();
  try {
    send({ type: 'ready' }); driver.draw(frame, 100, 40, 1);
    driver.draw({ ...frame, sourceKey: 'replacement' }, 100, 40, 1);
    const bitmap = { close: vi.fn() } as unknown as ImageBitmap;
    send({ type: 'frame', generation: 1, bitmap });
    expect(methods.get('drawImage')).toBeUndefined();
    expect(bitmap.close).toHaveBeenCalled();
    expect(worker.postMessage.mock.calls[2][0]).toMatchObject({ type: 'state', generation: 2, frame: { sourceKey: 'replacement' } });
  } finally { driver.dispose(); }
});

it('keeps the DOM canvas usable for fallback after worker timeout or error', () => {
  vi.useFakeTimers();
  const { driver, worker, frame, canvas, methods } = setup();
  try {
    driver.draw(frame, 100, 40, 1);
    vi.advanceTimersByTime(3000);
    expect(worker.terminate).toHaveBeenCalledOnce();
    expect(canvas.dataset.djRenderer).toBe('canvas');
    expect(methods.get('fillRect')).toHaveBeenCalled();
  } finally { driver.dispose(); }
});
