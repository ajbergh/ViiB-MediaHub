import { afterEach, describe, expect, it, vi } from 'vitest';
import { api } from './api';

afterEach(() => vi.unstubAllGlobals());

describe('Mix Next recommendation filters', () => {
  it('serializes typed inclusive range and stem-availability filters', async () => {
    const fetchMock = vi.fn(async (_input: RequestInfo | URL, _init?: RequestInit) => new Response(JSON.stringify({ recommendations: [] }), { status: 200 }));
    vi.stubGlobal('fetch', fetchMock);
    await api.getTrackTransitionRecommendations('song / 1', 3, 'hold', {
      minBpm: 120.5, maxBpm: 132, minEnergyLevel: 4, maxEnergyLevel: 8, stemsAvailable: true,
    });
    const url = String(fetchMock.mock.calls[0][0]);
    const query = new URL(url, 'http://local').searchParams;
    expect(decodeURIComponent(url)).toContain('/analysis/song / 1/recommendations');
    expect(Object.fromEntries(query.entries())).toMatchObject({
      limit: '3', intent: 'hold', minBpm: '120.5', maxBpm: '132',
      minEnergyLevel: '4', maxEnergyLevel: '8', stemsAvailable: 'true',
    });
  });
});
