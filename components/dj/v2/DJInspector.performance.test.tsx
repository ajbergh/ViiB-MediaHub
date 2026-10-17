// @vitest-environment jsdom
import React, { act, Profiler } from 'react';
import { createRoot } from 'react-dom/client';
import { expect, it, vi } from 'vitest';
import { useStore } from '../../../store';
import { DJEnergyInsights } from './DJEnergyInsights';
import { DJBeatGridEdit } from './DJBeatGridEdit';

it('does not React-render inspector tools on transport ticks but responds to grid edits', async () => {
  (globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
  const initial = useStore.getState();
  const host = document.createElement('div');
  const root = createRoot(host);
  const committed = vi.fn();
  try {
    // A transition away from pristine position zero is relevant to preview guards.
    // Subsequent playback positions must not invalidate the analysis/editor UI.
    useStore.getState().setDeckPosition('A', 1);
    useStore.getState().setDeckPosition('B', 1);
    await act(async () => root.render(
      <Profiler id='inspector-tools' onRender={committed}>
        <DJEnergyInsights deck='A' />
        <DJBeatGridEdit deck='A' />
      </Profiler>
    ));
    committed.mockClear();
    await act(async () => {
      for (let i = 1; i <= 100; i++) {
        useStore.getState().setDeckPosition('A', 1 + i / 20);
        useStore.getState().setDeckPosition('B', 1 + i / 20);
      }
    });
    expect(committed).not.toHaveBeenCalled();
    await act(async () => useStore.getState().setDeckAnalysis('A', { beatGrid: [0, 0.5, 1] }));
    expect(committed).toHaveBeenCalled();
  } finally {
    await act(async () => root.unmount());
    useStore.setState(initial);
    host.remove();
  }
});
