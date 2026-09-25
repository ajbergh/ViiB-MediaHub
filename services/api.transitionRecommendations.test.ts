import { afterEach, describe, expect, it, vi } from 'vitest';
import { api } from './api';

afterEach(() => vi.unstubAllGlobals());

describe('Mix Next recommendation filters', () => {
  it('serializes typed inclusive range, library, and stem-availability filters', async () => {
    const fetchMock = vi.fn(async (_input: RequestInfo | URL, _init?: RequestInit) => new Response(JSON.stringify({ recommendations: [] }), { status: 200 }));
    vi.stubGlobal('fetch', fetchMock);
    await api.getTrackTransitionRecommendations('song / 1', 3, 'hold', {
      minBpm: 120.5, maxBpm: 132, minEnergyLevel: 4, maxEnergyLevel: 8, stemsAvailable: true,
      camelotCompatible: true,
      playlistId: 'playlist / 1', genre: 'Rock',
      notRecentlyPlayedHours: 24,
    });
    const url = String(fetchMock.mock.calls[0][0]);
    const query = new URL(url, 'http://local').searchParams;
    expect(decodeURIComponent(url)).toContain('/analysis/song / 1/recommendations');
    expect(Object.fromEntries(query.entries())).toMatchObject({
      limit: '3', intent: 'hold', minBpm: '120.5', maxBpm: '132',
      minEnergyLevel: '4', maxEnergyLevel: '8', stemsAvailable: 'true',
      camelotCompatible: 'true',
      playlistId: 'playlist / 1', genre: 'Rock',
      notRecentlyPlayedHours: '24',
    });
  });

  it('omits empty library filters', async () => {
    const fetchMock = vi.fn(async (_input: RequestInfo | URL, _init?: RequestInit) => new Response(JSON.stringify({ recommendations: [] }), { status: 200 }));
    vi.stubGlobal('fetch', fetchMock);
    await api.getTrackTransitionRecommendations('song', 3, 'hold', { playlistId: '', genre: '' });
    const query = new URL(String(fetchMock.mock.calls[0][0]), 'http://local').searchParams;
    expect(query.has('playlistId')).toBe(false);
    expect(query.has('genre')).toBe(false);
  });

  it('omits recency filtering unless the user selects an interval', async () => {
    const fetchMock = vi.fn(async (_input: RequestInfo | URL, _init?: RequestInit) => new Response(JSON.stringify({ recommendations: [] }), { status: 200 }));
    vi.stubGlobal('fetch', fetchMock);
    await api.getTrackTransitionRecommendations('song');
    const query = new URL(String(fetchMock.mock.calls[0][0]), 'http://local').searchParams;
    expect(query.has('notRecentlyPlayedHours')).toBe(false);
  });
});
