/** Tests recording-link and reference request contracts and refresh isolation. */

import { afterEach, expect, it, vi } from 'vitest';
import { spotifyRecordingId, spotifyReference } from './spotifyReference';
import { useStore } from '../store';
afterEach(() => { vi.unstubAllGlobals(); vi.restoreAllMocks(); });
it('accepts only Spotify track IDs and fixed-origin track URLs', () => {
 const id = '5r9W9MJLvHk83fcZSPQ8SE';
 expect(spotifyRecordingId(id)).toBe(id);
 expect(spotifyRecordingId('https://open.spotify.com/track/' + id + '?si=sample')).toBe(id);
 for (const value of ['https://evil.invalid/track/' + id, 'http://open.spotify.com/track/' + id, 'https://open.spotify.com/album/' + id, 'invalid'])
  expect(spotifyRecordingId(value)).toBeNull();
});
it('retains typed provider failure without silently fetching another endpoint', async () => {
 const fetcher = vi.fn().mockResolvedValue(new Response(JSON.stringify({ cache: null, failure: { code: 'not_found' } }), { status: 404 }));
 vi.stubGlobal('fetch', fetcher);
 const result = await spotifyReference.refresh('5r9W9MJLvHk83fcZSPQ8SE', 'audio_analysis');
 expect(result.failure?.code).toBe('not_found');
 expect(fetcher).toHaveBeenCalledTimes(1);
 expect(fetcher.mock.calls[0][0]).toContain('endpoint=audio_analysis');
});

it('retire current authentication on an actual provider rejection', async () => {
 const mark = vi.spyOn(useStore.getState(), 'markSpotifyAuthRequired').mockImplementation(() => {});
 vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(JSON.stringify({ cache: null, failure: { code: 'authentication_required' } }), { status: 401 })));
 await spotifyReference.refresh('5r9W9MJLvHk83fcZSPQ8SE', 'audio_features');
 expect(mark).toHaveBeenCalledTimes(1);
});

it('does not retire a replacement session on an old refresh response', async () => {
 const original = useStore.getState();
 const mark = vi.fn();
 let generation = 1;
 vi.spyOn(useStore, 'getState').mockImplementation(() => ({ ...original, spotifySessionGeneration: generation, markSpotifyAuthRequired: mark }));
 let resolve!: (response: Response) => void;
 vi.stubGlobal('fetch', vi.fn().mockImplementation(() => new Promise(done => { resolve = done; })));
 const pending = spotifyReference.refresh('5r9W9MJLvHk83fcZSPQ8SE', 'audio_features');
 generation = 2;
 resolve(new Response(JSON.stringify({ failure: { code: 'authentication_required' } }), { status: 401 }));
 await expect(pending).rejects.toThrow('Spotify session changed');
 expect(mark).not.toHaveBeenCalled();
});