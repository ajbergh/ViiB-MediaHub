// @vitest-environment jsdom
import React, { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

const mocks = vi.hoisted(() => ({
  browseFolder: vi.fn(),
  addScanFolder: vi.fn(),
  loadScanFolders: vi.fn(),
  getStemLibraryLocations: vi.fn(),
  setStemLibraryLocations: vi.fn(),
  scanStemLibraries: vi.fn(),
  getJob: vi.fn(),
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
    getStemLibraryLocations: mocks.getStemLibraryLocations,
    setStemLibraryLocations: mocks.setStemLibraryLocations,
    scanStemLibraries: mocks.scanStemLibraries,
  },
}));

vi.mock('../services/jobsV2', () => ({ jobsV2: { get: mocks.getJob } }));

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
    mocks.getStemLibraryLocations.mockReset().mockResolvedValue([]);
    mocks.setStemLibraryLocations.mockReset().mockImplementation(async (paths: string[]) => paths.map((path, index) => ({ id: `stem-${index}`, path, enabled: true, createdAt: 1 })));
    mocks.scanStemLibraries.mockReset().mockResolvedValue({ jobId: 'stem-scan-job', status: 'accepted' });
    mocks.getJob.mockReset().mockResolvedValue({ id: 'stem-scan-job', type: 'stem_library_scan', status: 'queued', progressCurrent: 0, progressTotal: 0, message: 'Queued Stem Library scan', attempts: 1, createdAt: 1, updatedAt: 1 });
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
    expect(buttonNamed('Add This Folder').disabled).toBe(true);

    await act(async () => {
      resolveInitialBrowse({ currentPath: 'Drives', entries: [{ name: 'C:', path: 'C:\\', isDir: true }] });
      await Promise.resolve();
    });
    expect(buttonNamed('Add This Folder').disabled).toBe(true);
    expect(container.querySelector('button[aria-label="C: drive"]')).not.toBeNull();

    await click(buttonNamed('C: drive'));
    expect(container.textContent).toContain('Music');
    expect(mocks.browseFolder).toHaveBeenLastCalledWith('C:\\');
    expect(buttonNamed('Add This Folder').disabled).toBe(false);

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

    mocks.browseFolder.mockRejectedValueOnce(new Error('browse unavailable'));
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => undefined);
    await click(buttonNamed('Add Folder'));
    expect(buttonNamed('Add This Folder').disabled).toBe(true);
    expect(container.querySelector('[role="dialog"]')?.textContent).not.toContain('C:\\Music');
    consoleError.mockRestore();
    expect(mocks.addScanFolder).toHaveBeenCalledTimes(1);
  });

  it('stores Stem Library roots separately and starts an independent scan job', async () => {
    await act(async () => root.render(React.createElement(Settings)));
    await click(buttonNamed('Add Stem Library'));
    await act(async () => {
      resolveInitialBrowse({ currentPath: 'Drives', entries: [{ name: 'C:', path: 'C:\\', isDir: true }] });
      await Promise.resolve();
    });
    await click(buttonNamed('C: drive'));
    await click(buttonNamed('Use This Folder'));

    expect(mocks.setStemLibraryLocations).toHaveBeenCalledWith(['C:\\']);
    expect(mocks.addScanFolder).not.toHaveBeenCalled();
    expect(container.textContent).toContain('Artist/Album subfolders');
    expect(container.textContent).toContain('run Full Rescan once');

    await click(buttonNamed('Scan Stem Libraries'));
    expect(mocks.scanStemLibraries).toHaveBeenCalledTimes(1);
    expect(mocks.getJob).toHaveBeenCalledWith('stem-scan-job');
    expect(mocks.addScanFolder).not.toHaveBeenCalled();
  });
});
