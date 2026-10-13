import { afterEach, expect, it, vi } from 'vitest';
import { api } from './api';

afterEach(() => vi.unstubAllGlobals());
it('writes and resets a manual field with its loaded source and accepts no content', async () => {
  const fetchMock = vi.fn(async (_input: RequestInfo | URL, _init?: RequestInit) => new Response(null, { status: 204 }));
  vi.stubGlobal('fetch', fetchMock);
  await api.updateTrackMetadataField('song', 'time_signature', 3, 'fp');
  await api.resetTrackMetadataField('song', 'time_signature', 'fp');
  expect(fetchMock.mock.calls[0][1]?.body).toBe('{"value":3}');
  for (const call of fetchMock.mock.calls) expect(new Headers(call[1]?.headers).get('If-Match')).toBe('"fp"');
});
it('preserves source precondition failures', async () => {
  vi.stubGlobal('fetch', vi.fn(async () => new Response('{"error":"source changed"}', { status: 412 })));
  await expect(api.updateTrackMetadataField('song', 'local_energy_level', 7, 'old')).rejects.toMatchObject({ status: 412 });
});
