// @vitest-environment jsdom
/** Tests reconnecting the Plex settings panel to an in-progress catalog sync. */

import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { afterEach, expect, it, vi } from 'vitest';
import { plexService, type PlexSource } from '../services/plex';
import { PlexMusicSourceSettings } from './PlexMusicSourceSettings';

const store = vi.hoisted(() => ({
  refreshLibrary: vi.fn(async () => {}),
  addLog: vi.fn(),
}));

vi.mock('../store', () => ({
  useStore: (select: (state: typeof store) => unknown) => select(store),
}));

afterEach(() => {
  vi.restoreAllMocks();
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

it('reattaches to a sync already running and refreshes the catalog when it completes', async () => {
  vi.useFakeTimers();
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  const active: PlexSource = {
    id: 'source', machineIdentifier: 'machine', baseUrl: 'http://plex.local:32400', name: 'Plex',
    libraryId: '2', libraryTitle: 'Music', connectedAt: 1, lastSyncStatus: 'running',
    syncTracks: 3, syncTotal: 10, available: true, active: true,
  };
  vi.spyOn(plexService, 'getConfig').mockResolvedValue({ source: active, authenticated: true });
  vi.spyOn(plexService, 'getLibraries').mockResolvedValue([{ id: '2', title: 'Music', type: 'artist' }]);
  vi.spyOn(plexService, 'getSyncStatus')
    .mockRejectedValueOnce(new Error('temporary status request failure'))
    .mockResolvedValueOnce({ ...active, lastSyncStatus: 'complete', lastSyncAt: 100 });
  const container = document.createElement('div');
  const root = createRoot(container);
  try {
    await act(async () => root.render(<PlexMusicSourceSettings />));
    expect(container.textContent).toContain('Read 3 of 10 Plex tracks');
    await act(async () => vi.advanceTimersByTimeAsync(1500));
    expect(container.textContent).toContain('Read 3 of 10 Plex tracks');
    await act(async () => vi.advanceTimersByTimeAsync(1500));
    expect(container.textContent).not.toContain('Read 3 of 10 Plex tracks');
    expect(container.textContent).toContain('Last successful sync');
    expect(store.refreshLibrary).toHaveBeenCalledOnce();
  } finally {
    await act(async () => root.unmount());
  }
});
