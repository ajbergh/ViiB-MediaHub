import { afterEach, expect, it, vi } from 'vitest';
import { api } from './api';

afterEach(() => vi.unstubAllGlobals());

it.each([
  ['playlist', 60, 60],
  ['dj', 90, 90],
  ['playlist', undefined, undefined],
  ['dj', undefined, 45],
] as const)('serializes duration for %s mode (%s minutes)', async (mode, selected, expected) => {
  const fetchMock = vi.fn(async (_input: RequestInfo | URL, _init?: RequestInit) =>
    new Response(JSON.stringify({ filter: {}, songs: [] }), { status: 200 }));
  vi.stubGlobal('fetch', fetchMock);
  await api.generateSmartPlaylist('jazz', { mode, targetDurationMinutes: selected });
  const body = JSON.parse(fetchMock.mock.calls[0][1]!.body as string);
  expect(body.targetDurationMinutes).toBe(expected);
  if (expected === undefined) expect(body).not.toHaveProperty('targetDurationMinutes');
});
