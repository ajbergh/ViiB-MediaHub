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

  it('requests a bounded Continue Listening list', async () => {
    const records = [{ id: 'ep1', feedId: 'feed1', feedTitle: 'A show', positionMs: 32000 }];
    const fetchMock = vi.fn(async (_url: string, _options?: RequestInit) => ({
      ok: true, status: 200, json: async () => ({ episodes: records }),
    }));
    vi.stubGlobal('fetch', fetchMock);
    const result = await podcastsAPI.listContinueListening(12);
    expect(result.episodes).toEqual(records);
    expect(fetchMock.mock.calls[0][0]).toBe('/api/v2/podcasts/episodes/continue?limit=12');
  });

  it('builds only same-origin episode media URLs by stable episode ID', () => {
    expect(podcastsAPI.mediaURL('episode/a')).toBe('/api/v2/podcasts/episodes/episode%2Fa/media');
    expect(podcastsAPI.mediaURL('https://evil.example/?token=abc')).toBe(
      '/api/v2/podcasts/episodes/https%3A%2F%2Fevil.example%2F%3Ftoken%3Dabc/media',
    );
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

  it('queues, lists and cancels offline work without handling publisher URLs', async () => {
    const row = { episodeId: 'episode/1', id: 'episode/1', status: 'queued', bytesReceived: 0, expectedBytes: 0 };
    const fetchMock = vi.fn(async () => ({ ok: true, status: 202, json: async () => ({ download: row }) }));
    vi.stubGlobal('fetch', fetchMock);
    await expect(podcastsAPI.queueDownload('episode/1')).resolves.toEqual({ download: row });
    expect(fetchMock).toHaveBeenCalledWith('/api/v2/podcasts/episodes/episode%2F1/download', expect.objectContaining({
      method: 'POST',
    }));
    const cancelMock = vi.fn(async () => ({ ok: true, status: 200, json: async () => ({ download: { ...row, status: 'canceled' } }) }));
    vi.stubGlobal('fetch', cancelMock);
    await expect(podcastsAPI.cancelDownload('episode/1')).resolves.toMatchObject({ download: { status: 'canceled' } });
    expect(cancelMock).toHaveBeenCalledWith('/api/v2/podcasts/episodes/episode%2F1/download', expect.objectContaining({ method: 'DELETE' }));
    const listMock = vi.fn(async (_url: string) => ({ ok: true, status: 200, json: async () => ({ downloads: [row] }) }));
    vi.stubGlobal('fetch', listMock);
    expect((await podcastsAPI.listDownloads()).downloads).toEqual([row]);
    expect(listMock.mock.calls[0][0]).toBe('/api/v2/podcasts/downloads');
  });

  it('removes a completed offline file via 204 without exposing paths', async () => {
    const mock = vi.fn(async (_url: string, _options?: RequestInit) => ({ ok: true, status: 204 }));
    vi.stubGlobal('fetch', mock);
    await expect(podcastsAPI.removeDownload('ep-1')).resolves.toBeUndefined();
    expect(mock).toHaveBeenCalledWith('/api/v2/podcasts/episodes/ep-1/download',
      expect.objectContaining({ method: 'DELETE' }));
  });

  it('reads and updates global and per-show podcast playback settings', async () => {
    const global = { defaultSpeed: 1.25, skipBackSeconds: 10, skipForwardSeconds: 45 };
    const show = { defaultSpeed: 1.5, skipIntroSeconds: 20, skipOutroSeconds: 30 };
    const fetchMock = vi.fn()
      .mockResolvedValueOnce({ ok: true, status: 200, json: async () => ({ playback: global }) })
      .mockResolvedValueOnce({ ok: true, status: 200, json: async () => ({ playback: global }) })
      .mockResolvedValueOnce({ ok: true, status: 200, json: async () => ({ settings: show }) })
      .mockResolvedValueOnce({ ok: true, status: 200, json: async () => ({ settings: show }) });
    vi.stubGlobal('fetch', fetchMock);
    expect(await podcastsAPI.getGlobalSettings()).toEqual({ playback: global });
    expect(await podcastsAPI.setGlobalSettings(global)).toEqual({ playback: global });
    expect(await podcastsAPI.getShowPlaybackSettings('show/1')).toEqual({ settings: show });
    expect(await podcastsAPI.setShowPlaybackSettings('show/1', show)).toEqual({ settings: show });
    expect(fetchMock.mock.calls[0][0]).toBe('/api/v2/podcasts/settings');
    expect(fetchMock.mock.calls[1][1]).toMatchObject({ method: 'PUT', body: JSON.stringify(global) });
    expect(fetchMock.mock.calls[2][0]).toBe('/api/v2/podcasts/feeds/show%2F1/playback-settings');
    expect(fetchMock.mock.calls[3][1]).toMatchObject({ method: 'PUT', body: JSON.stringify(show) });
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

  it('calls directory discover, triage views, chapters/transcripts/bookmarks, queue and retention endpoints', async () => {
    const fetchMock = vi.fn(async (_url: string, _options?: RequestInit) => ({ ok: true, status: 200, json: async () => ({}) }));
    vi.stubGlobal('fetch', fetchMock);
    await podcastsAPI.getDiscoverStatus();
    await podcastsAPI.searchDirectory('tech', 10);
    await podcastsAPI.listEpisodesByView('starred', { q: 'ai', offset: 0, limit: 25 });
    await podcastsAPI.patchEpisodeState('ep/1', { starred: true, archived: false });
    await podcastsAPI.getEpisodeChapters('ep/1');
    await podcastsAPI.getEpisodeTranscript('ep/1');
    await podcastsAPI.createBookmark('ep/1', 15000, 'Key point');
    await podcastsAPI.addToQueue('ep/1', 'next');
    await podcastsAPI.setRetentionPolicy('feed/1', { keepLatestCount: 3 });

    expect(fetchMock.mock.calls[0][0]).toBe('/api/v2/podcasts/discover/status');
    expect(fetchMock.mock.calls[1][0]).toBe('/api/v2/podcasts/search?q=tech&max=10');
    expect(fetchMock.mock.calls[2][0]).toBe('/api/v2/podcasts/episodes?view=starred&offset=0&limit=25&q=ai');
    expect(fetchMock.mock.calls[3][0]).toBe('/api/v2/podcasts/episodes/ep%2F1/state');
    expect(fetchMock.mock.calls[3][1]?.method).toBe('PATCH');
    expect(fetchMock.mock.calls[4][0]).toBe('/api/v2/podcasts/episodes/ep%2F1/chapters');
    expect(fetchMock.mock.calls[5][0]).toBe('/api/v2/podcasts/episodes/ep%2F1/transcript');
    expect(fetchMock.mock.calls[6][0]).toBe('/api/v2/podcasts/episodes/ep%2F1/bookmarks');
    expect(fetchMock.mock.calls[7][0]).toBe('/api/v2/podcasts/queue');
    expect(fetchMock.mock.calls[8][0]).toBe('/api/v2/podcasts/feeds/feed%2F1/retention');
    expect(fetchMock.mock.calls[8][1]?.method).toBe('PUT');
  });
});
