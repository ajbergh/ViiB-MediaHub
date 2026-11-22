import { afterEach, describe, expect, it, vi } from 'vitest';
import { podcastsAPI, PodcastAPIError } from './podcasts';

afterEach(() => vi.unstubAllGlobals());

describe('podcastsAPI', () => {
  it('subscribes with JSON body and returns stable metadata', async () => {
    const fetchMock = vi.fn(async () => ({
      ok: true, status: 201, json: async () => ({ id: 'feed-id', title: 'Example', episodeCount: 2 }),
    }));
    vi.stubGlobal('fetch', fetchMock);
    const result = await podcastsAPI.subscribe('https://example.org/rss');
    expect(result).toEqual({ id: 'feed-id', title: 'Example', episodeCount: 2 });
    expect(fetchMock).toHaveBeenCalledWith('/api/v2/podcasts/feeds', expect.objectContaining({
      method: 'POST', body: '{"url":"https://example.org/rss"}',
    }));
  });

  it('preserves structured error without reflecting user URLs', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => ({
      ok: false, status: 400,
      json: async () => ({ error: { code: 'invalid_feed_url', message: 'Provide a public HTTPS podcast RSS feed URL' } }),
    })));
    await expect(podcastsAPI.previewFeed('file:///etc/passwd')).rejects.toEqual(
      new PodcastAPIError(400, 'invalid_feed_url', 'Provide a public HTTPS podcast RSS feed URL'),
    );
  });

  it('handles an empty list and 204 unsubscribe', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce({ ok: true, status: 200, json: async () => ({ feeds: [] }) })
      .mockResolvedValueOnce({ ok: true, status: 204 });
    vi.stubGlobal('fetch', fetchMock);
    expect(await podcastsAPI.listSubscriptions()).toEqual({ feeds: [] });
    expect(await podcastsAPI.unsubscribe('id/path')).toBeUndefined();
    expect(fetchMock.mock.calls[1][0]).toBe('/api/v2/podcasts/feeds/id%2Fpath');
  });
});
