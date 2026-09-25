// @vitest-environment jsdom
import React, { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

const mocks = vi.hoisted(() => ({
  browseFolder: vi.fn(),
  addScanFolder: vi.fn(),
  loadScanFolders: vi.fn(),
}));

vi.mock('../store', () => ({
  useStore: () => ({
    backendAvailable: true,
    scanFolders: [],
    isScanning: false,
    scanProgress: '',
    addScanFolder: mocks.addScanFolder,
    loadScanFolders: mocks.loadScanFolders,
  }),
}));

vi.mock('../services/api', () => ({
  api: {
    browseFolder: mocks.browseFolder,
    getLLMSettings: vi.fn().mockResolvedValue({ provider: 'ollama', providers: [], models: {} }),
    getSemanticSettings: vi.fn().mockResolvedValue({ provider: 'disabled', model: '', dimensions: 0 }),
    getSemanticStatus: vi.fn().mockResolvedValue({}),
    getLastFMSettings: vi.fn().mockResolvedValue({ apiKey: '', enabled: false, username: '' }),
    getLastFMStatus: vi.fn().mockResolvedValue({ connected: false, canScrobble: false }),
    getSetting: vi.fn().mockResolvedValue(''),
  },
}));

vi.mock('react-router', () => ({ useLocation: () => ({ state: null }) }));
vi.mock('../components/PlexMusicSourceSettings', () => ({ PlexMusicSourceSettings: () => null }));
vi.mock('./LibraryOperations', () => ({ LibraryMonitoringPanel: () => null, LibraryOperationsPanel: () => null }));

import { Settings } from './Settings';

describe('Settings Add Folder browser', () => {
  let container: HTMLDivElement;
  let root: Root;
  let resolveInitialBrowse: (result: { currentPath: string; entries: { name: string; path: string; isDir: boolean }[] }) => void;

  beforeEach(() => {
    (globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
    mocks.browseFolder.mockReset();
    mocks.addScanFolder.mockReset().mockResolvedValue(undefined);
    mocks.loadScanFolders.mockReset();
    mocks.browseFolder.mockImplementation(async (path?: string) => {
      if (path === 'C:\\') {
        return { currentPath: 'C:\\', entries: [{ name: 'Music', path: 'C:\\Music', isDir: true }] };
      }
      if (path === 'C:\\Music') {
        return { currentPath: 'C:\\Music', entries: [] };
      }
      return { currentPath: 'Drives', entries: [{ name: 'C:', path: 'C:\\', isDir: true }] };
    });
    mocks.browseFolder.mockImplementationOnce(() => new Promise(resolve => {
      resolveInitialBrowse = resolve;
    }));
    Object.defineProperty(navigator, 'platform', { configurable: true, value: 'Win32' });
    container = document.createElement('div');
    document.body.appendChild(container);
    root = createRoot(container);
  });

  afterEach(async () => {
    await act(async () => root.unmount());
    container.remove();
  });

  async function click(button: HTMLButtonElement): Promise<void> {
    await act(async () => {
      button.click();
      await Promise.resolve();
      await Promise.resolve();
    });
  }

  function buttonNamed(name: string): HTMLButtonElement {
    const button = Array.from(container.querySelectorAll('button')).find(candidate =>
      candidate.getAttribute('aria-label') === name || candidate.textContent?.trim() === name,
    );
    if (!button) throw new Error(`Button not found: ${name}`);
    return button;
  }

  it('opens, navigates, adds the current folder, and supports close and cancel', async () => {
    await act(async () => root.render(React.createElement(Settings)));

    await click(buttonNamed('Add Folder'));
    expect(container.querySelector('[role="dialog"]')).not.toBeNull();
    expect(container.textContent).toContain('Select Music Folder');
    expect(container.querySelector('[role="status"]')?.textContent).toContain('Loading folders');
    expect(mocks.browseFolder).toHaveBeenCalledWith('drives');

    await act(async () => {
      resolveInitialBrowse({ currentPath: 'Drives', entries: [{ name: 'C:', path: 'C:\\', isDir: true }] });
      await Promise.resolve();
    });
    expect(container.querySelector('button[aria-label="C: drive"]')).not.toBeNull();

    await click(buttonNamed('C: drive'));
    expect(container.textContent).toContain('Music');
    expect(mocks.browseFolder).toHaveBeenLastCalledWith('C:\\');

    await click(buttonNamed('Music'));
    expect(container.textContent).toContain('C:\\Music');
    expect(mocks.browseFolder).toHaveBeenLastCalledWith('C:\\Music');

    await click(buttonNamed('Add This Folder'));
    expect(mocks.addScanFolder).toHaveBeenCalledWith('C:\\Music');
    expect(container.querySelector('[role="dialog"]')).toBeNull();

    await click(buttonNamed('Add Folder'));
    await click(buttonNamed('Close folder browser'));
    expect(container.querySelector('[role="dialog"]')).toBeNull();
    expect(mocks.addScanFolder).toHaveBeenCalledTimes(1);

    await click(buttonNamed('Add Folder'));
    await click(buttonNamed('Cancel'));
    expect(container.querySelector('[role="dialog"]')).toBeNull();
    expect(mocks.addScanFolder).toHaveBeenCalledTimes(1);
  });
});
