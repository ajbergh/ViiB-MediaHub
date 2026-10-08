import { afterEach, expect, it, vi } from 'vitest';
import { api } from './api';
afterEach(() => vi.unstubAllGlobals());
it('sends the loaded source precondition on grid save and reset', async () => {
 const fetchMock = vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => init?.method === 'DELETE' ? new Response(null,{status:204}) : new Response(JSON.stringify({songId:'song',beats:[0,.5],downbeatIndices:[],locked:true,algorithmVersion:'v1',sourceFingerprint:'source-v1',resolution:'available'})));
 vi.stubGlobal('fetch',fetchMock);
 await api.updateTrackBeatGrid('song', {beats:[0,.5],downbeatIndices:[],locked:true}, 'source-v1');
 await api.resetTrackBeatGrid('song','source-v1');
 for (const call of fetchMock.mock.calls) expect(new Headers(call[1]?.headers).get('If-Match')).toBe('"source-v1"');
});
