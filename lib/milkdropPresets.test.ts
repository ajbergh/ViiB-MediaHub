// @vitest-environment jsdom
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { createRequire } from 'node:module';

const require = createRequire(import.meta.url);
class PresetWorker {
  static instances: PresetWorker[] = [];
  onmessage: ((event: { data: Record<string, object> }) => void) | undefined;
  onerror: (() => void) | undefined;
  terminate = vi.fn();
  constructor() { PresetWorker.instances.push(this); }
}

beforeEach(() => {
  vi.resetModules();
  vi.useFakeTimers();
  PresetWorker.instances = [];
  vi.stubGlobal('Worker', PresetWorker);
});
afterEach(() => { vi.useRealTimers(); vi.unstubAllGlobals(); });

it('keeps startup at 100 presets and shares one worker and expanded cache across layers', async () => {
  const loader = await import('./milkdropPresets');
  const base = await loader.loadMilkdropPresets();
  expect(Object.keys(base)).toHaveLength(100);
  expect(PresetWorker.instances).toHaveLength(0);
  const first = loader.loadAdditionalMilkdropPresets();
  const second = loader.loadAdditionalMilkdropPresets();
  expect(first).toBe(second);
  await Promise.resolve();
  const packs = ['MD1', 'Extra2', 'Extra'].map(name =>
    require(`butterchurn-presets/lib/butterchurnPresets${name}.min.js`).getPresets());
  const extras = Object.assign({}, ...packs);
  // The worker transport must support all preset data without functions.
  expect(() => structuredClone(extras)).not.toThrow();
  PresetWorker.instances[0].onmessage?.({ data: extras });
  const expanded = await first;
  expect(Object.keys(expanded)).toHaveLength(395);
  for (const name of Object.keys(base)) expect(expanded[name]).toBe(base[name]);
  expect(await loader.loadMilkdropPresets()).toBe(expanded);
  expect(await second).toBe(expanded);
  expect(PresetWorker.instances).toHaveLength(1);
  expect(PresetWorker.instances[0].terminate).toHaveBeenCalledOnce();
  expect(vi.getTimerCount()).toBe(0);
});

it('keeps the original pack usable when the worker fails', async () => {
  const loader = await import('./milkdropPresets');
  const base = await loader.loadMilkdropPresets();
  const result = loader.loadAdditionalMilkdropPresets();
  const failure = expect(result).rejects.toThrow('Could not load');
  await Promise.resolve();
  PresetWorker.instances[0].onerror?.();
  await failure;
  expect(await loader.loadMilkdropPresets()).toBe(base);
  expect(PresetWorker.instances[0].terminate).toHaveBeenCalledOnce();
});

it('defers expansion and cancels pending work on cleanup', async () => {
  const { scheduleMilkdropExpansion } = await import('./milkdropPresets');
  const callback = vi.fn();
  const cancel = scheduleMilkdropExpansion(callback);
  expect(callback).not.toHaveBeenCalled();
  cancel();
  vi.runAllTimers();
  expect(callback).not.toHaveBeenCalled();
  scheduleMilkdropExpansion(callback);
  vi.runAllTimers();
  expect(callback).toHaveBeenCalledOnce();
});
