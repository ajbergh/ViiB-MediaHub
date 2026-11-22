// @vitest-environment jsdom
import React, { act } from 'react';
import { createRoot, Root } from 'react-dom/client';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import type { MilkdropSettings } from '../../types';

const mocks = vi.hoisted(() => ({
  loadPreset: vi.fn(), publish: vi.fn(), change: vi.fn(), cancel: vi.fn(),
  base: { original: {} } as Record<string, object>, extra: {}, alternate: {},
  idle: undefined as (() => void) | undefined,
  resolve: undefined as ((presets: Record<string, object>) => void) | undefined,
}));
vi.mock('../../lib/audio', () => ({ audioEngine: {
  getAudioContext: () => ({}), getMasterGainNode: () => ({}),
} }));
vi.mock('../../lib/milkdropSupport', () => ({
  isMilkdropSupported: () => true, MILKDROP_QUALITY_MAP: { medium: { width: 512, height: 512 } },
}));
vi.mock('butterchurn', () => ({ default: { createVisualizer: () => ({
  loadPreset: mocks.loadPreset, connectAudio: vi.fn(), render: vi.fn(),
  setRendererSize: vi.fn(), loseGLContext: vi.fn(),
}) } }));
vi.mock('../../lib/milkdropPresets', () => ({
  loadMilkdropPresets: async () => mocks.base,
  loadAdditionalMilkdropPresets: () => new Promise(resolve => { mocks.resolve = resolve; }),
  scheduleMilkdropExpansion: (callback: () => void) => { mocks.idle = callback; return mocks.cancel; },
}));
import { MilkdropVisualizer } from './MilkdropVisualizer';

let root: Root;
let container: HTMLDivElement;
const settings: MilkdropSettings = {
  enabled: true, currentPreset: 'original', presetCycleEnabled: false,
  presetCycleInterval: 30, blendDuration: 2.7, quality: 'medium', favoritePresets: [],
};
beforeEach(() => {
  vi.clearAllMocks();
  mocks.base = { original: {} };
  mocks.idle = undefined;
  mocks.resolve = undefined;
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  vi.stubGlobal('ResizeObserver', class { observe() {} disconnect() {} });
  vi.stubGlobal('requestAnimationFrame', vi.fn(() => 1));
  vi.stubGlobal('cancelAnimationFrame', vi.fn());
  container = document.createElement('div');
  root = createRoot(container);
});
afterEach(async () => {
  await act(async () => root.unmount());
  vi.useRealTimers();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});
async function mount(currentPreset: string | null = 'original', overrides: Partial<MilkdropSettings> = {}, managesPresets = true) {
  await act(async () => {
    root.render(<MilkdropVisualizer settings={{ ...settings, currentPreset, ...overrides }} isActive managesPresets={managesPresets}
      onPresetChange={mocks.change} onPresetsLoaded={mocks.publish} />);
  });
}
async function expand() {
  await act(async () => {
    mocks.idle?.();
    mocks.resolve?.({ ...mocks.base, extra: mocks.extra });
  });
}

it('renders the base preset before expansion and expands without switching the active preset', async () => {
  await mount();
  expect(container.querySelector('canvas')?.style.display).toBe('block');
  expect(mocks.resolve).toBeUndefined();
  expect(mocks.publish).toHaveBeenLastCalledWith(['original']);
  const loads = mocks.loadPreset.mock.calls.length;
  await expand();
  expect(mocks.publish).toHaveBeenLastCalledWith(['original', 'extra']);
  expect(mocks.loadPreset).toHaveBeenCalledTimes(loads);
});

it('preserves a saved extra selection and restores it after expansion', async () => {
  await mount('extra');
  expect(mocks.change).not.toHaveBeenCalled();
  await expand();
  expect(mocks.loadPreset).toHaveBeenLastCalledWith(mocks.extra, 2.7);
});

it('does not publish results after the visualizer unmounts', async () => {
  await mount();
  await act(async () => { mocks.idle?.(); });
  await act(async () => root.unmount());
  mocks.publish.mockClear();
  await act(async () => { mocks.resolve?.({ ...mocks.base, extra: mocks.extra }); });
  expect(mocks.publish).not.toHaveBeenCalled();
  expect(mocks.cancel).toHaveBeenCalled();
});

it('starts with a random preset even with auto-cycle disabled', async () => {
  vi.useFakeTimers();
  vi.spyOn(Math, 'random').mockReturnValue(0.9);
  mocks.base.alternate = mocks.alternate;
  await mount(null);
  expect(mocks.loadPreset).toHaveBeenCalledExactlyOnceWith(mocks.alternate, 0);
  expect(mocks.change).toHaveBeenCalledExactlyOnceWith('alternate');

  await act(async () => { vi.advanceTimersByTime(60_000); });
  expect(mocks.change).toHaveBeenCalledTimes(1);
  await mount('original');
  expect(mocks.loadPreset).toHaveBeenLastCalledWith(mocks.base.original, 2.7);
});

it('follows the configured auto-cycle interval and stops scheduling when disabled', async () => {
  mocks.base.alternate = mocks.alternate;
  vi.useFakeTimers();
  await mount('original', { presetCycleEnabled: true, presetCycleInterval: 15 });
  await act(async () => { vi.advanceTimersByTime(14_999); });
  expect(mocks.change).not.toHaveBeenCalled();
  await act(async () => { vi.advanceTimersByTime(1); });
  expect(mocks.change).toHaveBeenCalledExactlyOnceWith('alternate');

  // The shared state reflects the change without loading the same preset twice.
  await mount('alternate', { presetCycleEnabled: true, presetCycleInterval: 15 });
  expect(mocks.loadPreset).toHaveBeenCalledTimes(2);
  await mount('alternate', { presetCycleEnabled: false });
  await act(async () => { vi.advanceTimersByTime(30_000); });
  expect(mocks.change).toHaveBeenCalledTimes(1);
});

it('a secondary layer follows preset changes without starting its own cycle', async () => {
  mocks.base.alternate = mocks.alternate;
  vi.useFakeTimers();
  await mount('original', { presetCycleEnabled: true }, false);
  await act(async () => { vi.advanceTimersByTime(60_000); });
  expect(mocks.change).not.toHaveBeenCalled();
  await mount('alternate', { presetCycleEnabled: true }, false);
  expect(mocks.loadPreset).toHaveBeenLastCalledWith(mocks.alternate, 2.7);
});
