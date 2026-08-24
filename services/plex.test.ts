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
});
