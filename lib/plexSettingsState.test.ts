import { describe, expect, it } from 'vitest';
import { initialPlexSettingsState, plexSettingsReducer } from './plexSettingsState';

const source = {
  id: 'plexsrc_a', machineIdentifier: 'machine-a', baseUrl: 'http://192.168.1.2:32400', name: 'Studio Plex',
  libraryId: '2', libraryTitle: 'Music', connectedAt: 1, lastSyncStatus: 'complete', available: true, active: true,
};

describe('plexSettingsReducer', () => {
  it('loads configured source and library without secret state', () => {
    const state = plexSettingsReducer(initialPlexSettingsState, { type: 'loaded', source, authenticated: true });
    expect(state.source?.name).toBe('Studio Plex');
    expect(state.selectedLibraryId).toBe('2');
    expect(state.authenticated).toBe(true);
    expect(JSON.stringify(state)).not.toContain('token');
  });

  it('records discovery results and manual address transitions', () => {
    let state = plexSettingsReducer(initialPlexSettingsState, { type: 'manual_url', url: 'plex.local' });
    state = plexSettingsReducer(state, { type: 'discovered', servers: [{
      name: 'LAN Plex', host: '192.168.1.3', port: 32400, scheme: 'http', url: 'http://192.168.1.3:32400',
      machineIdentifier: 'machine-b', claimed: true, authRequired: true,
    }] });
    expect(state.manualUrl).toBe('plex.local');
    expect(state.discovered).toHaveLength(1);
    expect(state.busy).toBeNull();
  });

  it('keeps an offline source configured and exposes its source state', () => {
    const offline = { ...source, available: false, lastSyncStatus: 'error', lastSyncError: 'offline' };
    const state = plexSettingsReducer(initialPlexSettingsState, { type: 'loaded', source: offline, authenticated: true });
    expect(state.source?.available).toBe(false);
    expect(state.source?.lastSyncStatus).toBe('error');
    expect(state.source?.libraryTitle).toBe('Music');
  });

  it('clears configuration on disconnect reset', () => {
    const configured = plexSettingsReducer(initialPlexSettingsState, { type: 'loaded', source, authenticated: true });
    const reset = plexSettingsReducer(configured, { type: 'reset' });
    expect(reset.source).toBeNull();
    expect(reset.authenticated).toBe(false);
    expect(reset.libraries).toEqual([]);
  });
});
