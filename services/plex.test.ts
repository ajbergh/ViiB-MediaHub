import { afterEach, describe, expect, it, vi } from 'vitest';
import { plexService } from './plex';

describe('plexService', () => {
  afterEach(() => vi.restoreAllMocks());

  it('defensively returns only music libraries', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify({ libraries: [
      { id: '1', title: 'Movies', type: 'movie' },
      { id: '2', title: 'Music', type: 'artist' },
      { id: '3', title: 'Audio Books', type: 'audio' },
      { id: '4', title: 'Music Videos', type: 'clip' },
    ] }), { status: 200, headers: { 'Content-Type': 'application/json' } }));

    const libraries = await plexService.getLibraries();
    expect(libraries.map(library => library.title)).toEqual(['Music', 'Audio Books']);
  });

  it('surfaces the structured v2 error message', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify({
      error: { code: 'plex_connection_failed', message: 'Plex server connection refused', retryable: true },
    }), { status: 502, headers: { 'Content-Type': 'application/json' } }));

    await expect(plexService.connect('192.168.1.20')).rejects.toThrow('Plex server connection refused');
  });

  it('lists and selects Plex account servers without exposing server tokens', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch')
      .mockResolvedValueOnce(new Response(JSON.stringify({ servers: [{
        name: 'Shared Music', url: 'https://shared.example:32400', machineIdentifier: 'shared-server', owned: false, local: false, relay: false,
      }] }), { status: 200, headers: { 'Content-Type': 'application/json' } }))
      .mockResolvedValueOnce(new Response(JSON.stringify({
        name: 'Shared Music', host: 'shared.example', port: 32400, scheme: 'https', url: 'https://shared.example:32400', machineIdentifier: 'shared-server', claimed: true, authRequired: true,
      }), { status: 200, headers: { 'Content-Type': 'application/json' } }));

    await expect(plexService.getAccountServers()).resolves.toMatchObject([{ machineIdentifier: 'shared-server', owned: false }]);
    await expect(plexService.connectAccountServer('shared-server')).resolves.toMatchObject({ machineIdentifier: 'shared-server' });
    expect(fetchMock).toHaveBeenNthCalledWith(1, '/api/v2/plex/servers');
    expect(fetchMock).toHaveBeenNthCalledWith(2, '/api/v2/plex/servers/select', expect.objectContaining({
      method: 'POST', body: JSON.stringify({ machineIdentifier: 'shared-server' }),
    }));
  });
});
