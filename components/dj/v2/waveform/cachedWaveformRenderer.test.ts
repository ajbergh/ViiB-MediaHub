import { expect, it, vi } from 'vitest';
import { CachedWaveformRenderer, type RasterFactory } from './cachedWaveformRenderer';
import type { MainWaveformFrame } from './canvasWaveformRenderer';

function context() {
  const methods = new Map<string, ReturnType<typeof vi.fn>>();
  const ctx = new Proxy({}, { get: (_, key) => { if (!methods.has(String(key))) methods.set(String(key), vi.fn()); return methods.get(String(key)); }, set: () => true });
  return { ctx: ctx as CanvasRenderingContext2D, methods };
}
it.each(['horizontal', 'vertical'] as const)('reuses %s tiles for position/cue updates and invalidates on zoom/source changes', orientation => {
  const { ctx, methods } = context();
  const create = vi.fn((width, height) => ({ width, height, getContext: () => context().ctx })) as unknown as RasterFactory;
  const renderer = new CachedWaveformRenderer(create);
  const frame: MainWaveformFrame = { deck: 'A', peaks: Array(10000).fill(0.8), position: 10, duration: 100,
    visibleSeconds: 4, colorMode: 'single', hasTrack: true, beatGrid: [9, 10, 11], beatGridOffset: 0,
    hotCues: [], cuePoint: 0, loop: { start: 0, end: 0, enabled: false }, orientation };
  const width = orientation === 'horizontal' ? 1024 : 64;
  const height = orientation === 'vertical' ? 1024 : 64;
  renderer.draw(ctx, width, height, frame);
  const builds = renderer.stats.builds;
  renderer.draw(ctx, width, height, { ...frame, position: 10.01, hotCues: [{ slot: 1, position: 10, color: 'blue' }] });
  expect(renderer.stats.builds).toBe(builds);
  expect(renderer.stats.hits).toBeGreaterThan(0);
  expect(methods.get('drawImage')!.mock.calls.length).toBeGreaterThan(3);
  expect(methods.get('moveTo')!.mock.calls).toContainEqual(orientation === 'horizontal' ? [512, 0] : [0, 512]);
  renderer.draw(ctx, width, height, { ...frame, visibleSeconds: 8 });
  expect(renderer.stats.builds).toBeGreaterThan(builds);
  const zoomBuilds = renderer.stats.builds;
  renderer.draw(ctx, width, height, { ...frame, peaks: Array(10000).fill(0.2) });
  expect(renderer.stats.builds).toBeGreaterThan(zoomBuilds);
});
