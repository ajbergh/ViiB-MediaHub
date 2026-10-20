// @vitest-environment jsdom
import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { expect, it, vi } from 'vitest';
import { browseDJMidiLibrary, loadDJMidiLibrary, toggleDJMidiLibrary } from '../../../lib/djMidiLibrary';

vi.mock('./DJLibraryBrowserV2', () => ({ DJLibraryBrowserV2: () => <input type="text" aria-label="Library search" /> }));
import { DJLibraryDrawer } from './DJLibraryDrawer';

it('opens and closes on successive Browse presses without toggling during dial or load actions', async () => {
  (globalThis as any).IS_REACT_ACT_ENVIRONMENT = true;
  const host = document.createElement('div');
  document.body.appendChild(host);
  const root = createRoot(host);
  const drawer = () => host.querySelector<HTMLElement>('#dj-library-drawer')!;
  try {
    await act(async () => root.render(<DJLibraryDrawer />));
    expect(drawer().hidden).toBe(true);
    await act(async () => toggleDJMidiLibrary());
    expect(drawer().hidden).toBe(false);
    await act(async () => toggleDJMidiLibrary());
    expect(drawer().hidden).toBe(true);
    await act(async () => browseDJMidiLibrary(1));
    expect(drawer().hidden).toBe(false);
    await act(async () => browseDJMidiLibrary(1));
    expect(drawer().hidden).toBe(false);
    await act(async () => loadDJMidiLibrary('A'));
    expect(drawer().hidden).toBe(false);
    await act(async () => toggleDJMidiLibrary());
    expect(drawer().hidden).toBe(true);
  } finally {
    await act(async () => root.unmount());
    host.remove();
  }
});
