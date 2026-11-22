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

  it('reports a stale backend when it receives a non-JSON HTTP 404', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => ({
      ok: false, status: 404, json: async () => { throw new SyntaxError('HTML 404'); },
    })));
    await expect(podcastsAPI.previewFeed('https://example.org/rss')).rejects.toMatchObject({
      status: 404, code: 'podcast_api_unavailable',
      message: expect.stringContaining('Rebuild and restart the Go backend'),
    });
  });

  it('diagnoses proxy failures without reflecting arbitrary upstream content', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => ({
      ok: false, status: 502, json: async () => ({ error: 'secret=http://example.org' }),
    })));
    await expect(podcastsAPI.subscribe('https://example.org/rss')).rejects.toMatchObject({
      status: 502, code: 'podcast_api_unavailable',
      message: expect.stringContaining('development proxy'),
    });
  });

  it('queues manual refresh without assuming completion', async () => {
    const mock = vi.fn()
      .mockResolvedValueOnce({ ok: true, status: 202, json: async () => ({ status: 'queued' }) })
      .mockResolvedValueOnce({ ok: true, status: 200, json: async () => ({
        lastCheckedAt: 100, lastSuccessAt: 100, nextRefreshAt: 200,
        failures: 0, refreshing: false,
      }) });
    vi.stubGlobal('fetch', mock);
    expect(await podcastsAPI.refresh('a-feed')).toEqual({ status: 'queued' });
    expect(await podcastsAPI.refreshStatus('a-feed')).toMatchObject({ lastSuccessAt: 100 });
    expect(mock.mock.calls[0][1].method).toBe('POST');
  });

  it('previews OPML via backend XML parser before requesting any feeds', async () => {
    const preview = { feeds: [{ title: 'Public', url: 'https://example.org/feed.xml', alreadyFollowed: false }], invalidCount: 0, duplicateCount: 0 };
    const fetchMock = vi.fn(async (_url: string, _options?: RequestInit) => ({
      ok: true, status: 200, json: async () => preview,
    }));
    vi.stubGlobal('fetch', fetchMock);
    const xml = '<opml version="2.0"><body/></opml>';
    expect(await podcastsAPI.previewOPML(xml)).toEqual(preview);
    expect(fetchMock.mock.calls[0][0]).toBe('/api/v2/podcasts/import/opml/preview');
    expect(fetchMock.mock.calls[0][1]).toMatchObject({
      method: 'POST', body: xml, headers: { Accept: 'application/json', 'Content-Type': 'application/xml' },
    });
  });

  it('exports OPML from the same-origin backend without JSON parsing', async () => {
    const blob = new Blob(['<opml version="2.0"></opml>'], { type: 'application/xml' });
    const fetchMock = vi.fn(async () => ({ ok: true, status: 200, blob: async () => blob }));
    vi.stubGlobal('fetch', fetchMock);
    expect(await podcastsAPI.exportOPML()).toBe(blob);
    expect(fetchMock).toHaveBeenCalledWith('/api/v2/podcasts/export/opml', expect.objectContaining({
      headers: expect.objectContaining({ Accept: 'application/xml' }),
    }));
  });

  it('looks up a single followed show by ID', async () => {
    const record = { id: 'id/one', title: 'Following', url: 'https://example.org/rss' };
    const fetchMock = vi.fn(async (_url: string, _options?: RequestInit) => ({
      ok: true, status: 200, json: async () => ({ feed: record }),
    }));
    vi.stubGlobal('fetch', fetchMock);
    expect(await podcastsAPI.getFeed('id/one')).toEqual({ feed: record });
    expect(fetchMock.mock.calls[0][0]).toBe('/api/v2/podcasts/feeds/id%2Fone');
  });

  it('loads and saves episode progress with an optimistic revision', async () => {
    const response = { episodeId: 'e/1', positionMs: 4000, listenedMs: 4000, isPlayed: true, revision: 2 };
    const fetchMock = vi.fn(async (_url: string, _options?: RequestInit) => ({
      ok: true, status: 200, json: async () => response,
    }));
    vi.stubGlobal('fetch', fetchMock);
    expect(await podcastsAPI.getProgress('e/1')).toEqual(response);
    const result = await podcastsAPI.setProgress('e/1', {
      positionMs: 4000, listenedMs: 4000, isPlayed: true, expectedRevision: 1,
    });
    expect(result.revision).toBe(2);
    expect(fetchMock.mock.calls[0][0]).toBe('/api/v2/podcasts/episodes/e%2F1/progress');
    expect(fetchMock.mock.calls[1][1]?.method).toBe('PUT');
    expect(JSON.parse(String(fetchMock.mock.calls[1][1]?.body))).toMatchObject({ expectedRevision: 1, isPlayed: true });
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
