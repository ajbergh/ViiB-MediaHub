import { afterEach, expect, it, vi } from 'vitest';
import { api } from './api';

afterEach(() => vi.unstubAllGlobals());
it('carries explicit download request origins and omits them for ordinary requests', async () => {
 const fetchMock = vi.fn(async (_input: RequestInfo | URL, _init?: RequestInit) => new Response('{}', { status: 200 }));
 vi.stubGlobal('fetch', fetchMock);
 const origins = [{ kind: 'library' as const, id: 'saved_playlists', entityId: 'PPPPPPPPPPPPPPPPPPPPPP', position: -1 }];
 await api.downloadPlaylist('playlist', 'Playlist', 'Owner', origins);
 await api.downloadAlbum('album', 'Album', 'Artist', [{ ...origins[0], id: 'saved_albums' }]);
 await api.downloadTrack('track', 'Track', 'Artist', 'Album', 42);
 expect(JSON.parse(String(fetchMock.mock.calls[0][1]?.body)).origins).toEqual(origins);
 expect(JSON.parse(String(fetchMock.mock.calls[1][1]?.body)).origins[0].id).toBe('saved_albums');
 expect(JSON.parse(String(fetchMock.mock.calls[2][1]?.body))).not.toHaveProperty('origins');
});
