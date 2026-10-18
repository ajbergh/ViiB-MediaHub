// @vitest-environment jsdom
import React, { act } from 'react';
import { createRoot, Root } from 'react-dom/client';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import type { MilkdropSettings } from '../../types';

const mocks = vi.hoisted(() => ({
  loadPreset: vi.fn(), publish: vi.fn(), change: vi.fn(), cancel: vi.fn(),
  base: { original: {} }, extra: {},
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
  vi.unstubAllGlobals();
});
async function mount(currentPreset = 'original') {
  await act(async () => {
    root.render(<MilkdropVisualizer settings={{ ...settings, currentPreset }} isActive
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
