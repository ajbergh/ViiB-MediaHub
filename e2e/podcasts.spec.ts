import { expect, test } from '@playwright/test';

const feedURL = 'https://publisher.example/feed.xml';
const feedId = 'podcast-fixture-1';

test('Podcasts follows public RSS and browses episodes without playback', async ({ page }) => {
  let subscribed = false;
  let unfollowed = false;
  let refreshed = 0;
  const mutations: string[] = [];
  const errors: string[] = [];
  page.on('pageerror', error => errors.push(error.message));

  await page.route('**/*', async route => {
    const req = route.request();
    const u = new URL(req.url());
    if (u.origin !== 'http://127.0.0.1:4173') return route.abort();
    if (u.pathname === '/api/settings/podcasts_preview_enabled') {
      return route.fulfill({ json: { key: 'podcasts_preview_enabled', value: 'true' } });
    }
    if (u.pathname === '/api/v2/podcasts/feeds' && req.method() === 'GET') {
      return route.fulfill({ json: { feeds: subscribed ? [{
        id: feedId, url: feedURL, title: 'Independent Podcast', author: 'Publisher',
        description: 'A sample RSS feed', imageUrl: 'https://images.publisher.example/show.png', language: 'en',
        episodeCount: 1,
      }] : [] } });
    }
    if (u.pathname === '/api/v2/podcasts/feeds/preview' && req.method() === 'POST') {
      expect(req.postDataJSON()).toEqual({ url: feedURL });
      return route.fulfill({ json: { feed: {
        // The preview's final publisher URL differs from the user-entered one.
        // Subscribe must retain the original input for alias reconciliation.
        url: 'https://cdn.example.org/canonical.xml', title: 'Independent Podcast', author: 'Publisher',
        description: 'A sample RSS feed', episodes: [{ guid: 'ep-1', title: 'First episode' }],
      }, episodeCount: 1 } });
    }
    if (u.pathname === '/api/v2/podcasts/feeds' && req.method() === 'POST') {
      mutations.push('follow');
      expect(req.postDataJSON()).toEqual({ url: feedURL });
      subscribed = true;
      return route.fulfill({ status: 201, json: {
        id: feedId, title: 'Independent Podcast', episodeCount: 1,
      } });
    }
    if (u.pathname === '/api/v2/podcasts/feeds/' + feedId && req.method() === 'GET') {
      return route.fulfill({ json: { feed: {
        id: feedId, url: feedURL, title: 'Independent Podcast', author: 'Publisher',
        description: 'A sample RSS feed', imageUrl: 'https://images.publisher.example/show.png',
        language: 'en', episodeCount: 1,
      } } });
    }
    if (u.pathname === '/api/v2/podcasts/feeds/' + feedId + '/artwork') {
      return route.fulfill({ status: 200, contentType: 'image/png',
        body: Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+/6u0AAAAASUVORK5CYII=', 'base64') });
    }
    if (u.pathname === '/api/v2/podcasts/episodes/ep-1/progress') {
      if (req.method() === 'GET') {
        return route.fulfill({ json: { episodeId: 'ep-1', positionMs: 30000,
          listenedMs: 30000, isPlayed: false, revision: 0, completedAt: 0, lastPlayedAt: 0 } });
      }
      expect(req.method()).toBe('PUT');
      expect(req.postDataJSON()).toMatchObject({ positionMs: 30000,
        listenedMs: 30000, isPlayed: true, expectedRevision: 0 });
      return route.fulfill({ json: { episodeId: 'ep-1', positionMs: 30000,
        listenedMs: 30000, isPlayed: true, revision: 1, completedAt: 100, lastPlayedAt: 100 } });
    }
    if (u.pathname === '/api/v2/podcasts/feeds/' + feedId + '/refresh') {
      if (req.method() === 'POST') {
        refreshed++;
        return route.fulfill({ status: 202, json: { status: 'queued' } });
      }
      return route.fulfill({ json: {
        lastCheckedAt: 0, lastSuccessAt: 0, nextRefreshAt: 0,
        failures: 0, refreshing: false,
      } });
    }
    if (u.pathname === '/api/v2/podcasts/feeds/' + feedId + '/episodes') {
      return route.fulfill({ json: { episodes: [{
        id: 'ep-1', feedId, guid: 'ep-1', title: 'First episode',
        description: 'Publisher episode description', enclosureUrl: '',
        enclosureType: 'audio/mpeg', publishedAt: 1795377600000, durationMs: 1800000,
      }], nextOffset: null } });
    }
    if (u.pathname === '/api/v2/podcasts/feeds/' + feedId && req.method() === 'DELETE') {
      mutations.push('unfollow');
      subscribed = false;
      unfollowed = true;
      return route.fulfill({ status: 204, body: '' });
    }
    if (u.pathname.startsWith('/api/')) return route.fulfill({
      status: 503, json: { error: 'Unavailable in synthetic podcast fixture' },
    });
    return route.continue();
  });

  await page.goto('/podcasts');
  await expect(page.getByRole('heading', { name: 'Podcasts', exact: true })).toBeVisible();
  await expect(page.getByText('No podcasts followed yet')).toBeVisible();
  await page.getByRole('textbox', { name: 'Podcast RSS URL' }).fill(feedURL);
  await page.getByRole('button', { name: 'Preview feed' }).click();
  await expect(page.getByText('1 episodes in the feed')).toBeVisible();
  await page.getByRole('button', { name: 'Follow podcast' }).click();
  await expect(page).toHaveURL('/podcasts/show/' + feedId);
  await expect(page.getByRole('heading', { name: 'First episode' })).toBeVisible();
  await expect(page.getByRole('img', { name: 'Independent Podcast cover art' })).toHaveAttribute(
    'src', '/api/v2/podcasts/feeds/' + feedId + '/artwork'
  );
  await page.getByRole('button', { name: 'Mark First episode played' }).click();
  await expect(page.getByRole('button', { name: 'Mark First episode unplayed' })).toBeVisible();
  await page.getByRole('combobox', { name: 'Filter episodes by played status' }).selectOption('unplayed');
  await expect(page.getByRole('heading', { name: 'First episode' })).toHaveCount(0);
  await page.getByRole('combobox', { name: 'Filter episodes by played status' }).selectOption('played');
  await expect(page.getByRole('heading', { name: 'First episode' })).toBeVisible();

  await page.getByRole('button', { name: 'Refresh feed' }).click();
  await expect(page.getByText('Refresh queued. Episodes will update when the publisher responds.')).toBeVisible();
  expect(refreshed).toBe(1);
  await expect(page.getByText('Streaming and downloads are coming in a later phase.')).toBeVisible();
  await page.getByRole('button', { name: 'Unfollow', exact: true }).click();
  await page.getByRole('button', { name: 'Confirm', exact: true }).click();
  await expect(page).toHaveURL('/podcasts');
  await expect(page.getByText('No podcasts followed yet')).toBeVisible();
  expect(unfollowed).toBe(true);
  expect(mutations).toEqual(['follow', 'unfollow']);
  expect(errors).toEqual([]);
});

test('Podcasts stays visible with automatic refresh disabled by default', async ({ page }) => {
  let podcastMutations = 0;
  await page.route('**/*', async route => {
    const url = new URL(route.request().url());
    if (url.origin !== 'http://127.0.0.1:4173') return route.abort();
    if (url.pathname === '/api/settings/podcasts_preview_enabled') {
      return route.fulfill({ json: { key: 'podcasts_preview_enabled', value: '' } });
    }
    if (url.pathname.startsWith('/api/v2/podcasts/') && route.request().method() !== 'GET') podcastMutations++;
    if (url.pathname === '/api/v2/podcasts/feeds') return route.fulfill({ json: { feeds: [] } });
    if (url.pathname.startsWith('/api/')) {
      return route.fulfill({ status: 503, json: { error: 'Fixture backend unavailable' } });
    }
    return route.continue();
  });
  await page.goto('/podcasts');
  await expect(page.getByRole('heading', { name: 'Podcasts', exact: true })).toBeVisible();
  await expect(page.getByRole('link', { name: 'Podcasts', exact: true })).toBeVisible();
  await expect(page.getByText('No podcasts followed yet')).toBeVisible();
  expect(podcastMutations).toBe(0);
});


test('show detail paginates episodes without duplicates', async ({ page }) => {
  const requestedOffsets: number[] = [];
  await page.route('**/*', async route => {
    const url = new URL(route.request().url());
    if (url.origin !== 'http://127.0.0.1:4173') return route.abort();
    if (url.pathname === '/api/settings/podcasts_preview_enabled') {
      return route.fulfill({ json: { key: 'podcasts_preview_enabled', value: '' } });
    }
    if (url.pathname === '/api/v2/podcasts/feeds') return route.fulfill({ json: { feeds: [{
      id: feedId, url: feedURL, title: 'Paged Podcast', author: 'Publisher',
      description: '', imageUrl: '', language: 'en', episodeCount: 3,
    }] } });
    if (url.pathname === '/api/v2/podcasts/feeds/' + feedId) return route.fulfill({
      json: { feed: {
        id: feedId, url: feedURL, title: 'Paged Podcast', author: 'Publisher',
        description: '', imageUrl: '', language: 'en', episodeCount: 3,
      } },
    });
    if (url.pathname === '/api/v2/podcasts/feeds/' + feedId + '/episodes') {
      const offset = Number(url.searchParams.get('offset') || '0');
      requestedOffsets.push(offset);
      if (offset === 0) return route.fulfill({ json: { episodes: [{
        id: 'ep-1', feedId, guid: 'one', title: 'Episode One', description: '',
        enclosureUrl: '', enclosureType: 'audio/mpeg', publishedAt: 0, durationMs: 0,
      }], nextOffset: 1 } });
      return route.fulfill({ json: { episodes: [{
        id: 'ep-2', feedId, guid: 'two', title: 'Episode Two', description: '',
        enclosureUrl: '', enclosureType: 'audio/mpeg', publishedAt: 0, durationMs: 0,
      }], nextOffset: null } });
    }
    if (url.pathname === '/api/v2/podcasts/feeds/' + feedId + '/refresh')
      return route.fulfill({ json: {
        lastCheckedAt: 0, lastSuccessAt: 0, nextRefreshAt: 0,
        failures: 0, refreshing: false,
      } });
    if (url.pathname.startsWith('/api/'))
      return route.fulfill({ status: 503, json: { error: 'Unrelated fixture API' } });
    return route.continue();
  });
  await page.goto('/podcasts/show/' + feedId);
  await expect(page.getByRole('heading', { name: 'Episode One' })).toBeVisible();
  await expect(page.getByRole('button', { name: 'Load more episodes' })).toBeVisible();
  await page.getByRole('button', { name: 'Load more episodes' }).click();
  await expect(page.getByRole('heading', { name: 'Episode Two' })).toBeVisible();
  await page.getByRole('searchbox', { name: 'Filter loaded episodes' }).fill('Two');
  await expect(page.getByRole('heading', { name: 'Episode Two' })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Episode One' })).toHaveCount(0);
  await page.getByRole('searchbox', { name: 'Filter loaded episodes' }).clear();
  await expect(page.getByRole('button', { name: 'Load more episodes' })).toHaveCount(0);
  expect(requestedOffsets).toEqual([0, 1]);
});


test('following subscriptions export as a standard OPML download', async ({ page }) => {
  const xml = '<?xml version="1.0" encoding="UTF-8"?>\n' +
    '<opml version="2.0"><head><title>ViiB MediaHub Podcasts</title></head>' +
    '<body><outline type="rss" text="Example &amp; Friends" title="Example &amp; Friends" xmlUrl="https://publisher.example/rss.xml"></outline></body></opml>';
  let exportCalls = 0;
  await page.route('**/*', async route => {
    const url = new URL(route.request().url());
    if (url.origin !== 'http://127.0.0.1:4173') return route.abort();
    if (url.pathname === '/api/v2/podcasts/feeds' && route.request().method() === 'GET') {
      return route.fulfill({ json: { feeds: [{
        id: feedId, url: feedURL, title: 'Example & Friends', author: 'Publisher',
        description: '', imageUrl: '', language: 'en', episodeCount: 1,
      }] } });
    }
    if (url.pathname === '/api/v2/podcasts/export/opml') {
      exportCalls++;
      expect(route.request().method()).toBe('GET');
      return route.fulfill({ status: 200, contentType: 'application/xml; charset=utf-8',
        headers: { 'content-disposition': 'attachment; filename="viib-podcasts.opml"' },
        body: xml });
    }
    if (url.pathname.startsWith('/api/')) return route.fulfill({ status: 503,
      json: { error: { code: 'fixture_unavailable', message: 'Synthetic fixture only' } } });
    return route.continue();
  });
  await page.goto('/podcasts');
  const button = page.getByRole('button', { name: 'Export followed podcasts as OPML' });
  await expect(button).toBeEnabled();
  const downloadPromise = page.waitForEvent('download');
  await button.click();
  const download = await downloadPromise;
  expect(download.suggestedFilename()).toBe('viib-podcasts.opml');
  expect(exportCalls).toBe(1);
  const stream = await download.createReadStream();
  const chunks: Buffer[] = [];
  for await (const chunk of stream) chunks.push(Buffer.isBuffer(chunk) ? chunk : Buffer.from(chunk));
  expect(Buffer.concat(chunks).toString('utf8')).toBe(xml);
});


test('OPML import previews safely and reports per-feed failures with cancellation control', async ({ page }) => {
  const first = 'https://podcast.example.org/show-one.xml';
  const second = 'https://podcast.example.org/show-two.xml';
  let imports = 0;
  await page.route('**/*', async route => {
    const request = route.request();
    const url = new URL(request.url());
    if (url.origin !== 'http://127.0.0.1:4173') return route.abort();
    if (url.pathname === '/api/v2/podcasts/feeds' && request.method() === 'GET') {
      return route.fulfill({ json: { feeds: [] } });
    }
    if (url.pathname === '/api/v2/podcasts/import/opml/preview') {
      expect(request.method()).toBe('POST');
      expect(request.headers()['content-type']).toBe('application/xml');
      expect(request.postData()).toContain('<opml');
      return route.fulfill({ json: {
        feeds: [
          { title: 'Podcast One', url: first, alreadyFollowed: false },
          { title: 'Podcast Two', url: second, alreadyFollowed: false },
        ],
        invalidCount: 1, duplicateCount: 1,
      } });
    }
    if (url.pathname === '/api/v2/podcasts/feeds' && request.method() === 'POST') {
      imports++;
      const payload = request.postDataJSON() as { url: string };
      if (payload.url === first) return route.fulfill({
        status: 201, json: { id: 'one', title: 'Podcast One', episodeCount: 1 },
      });
      expect(payload.url).toBe(second);
      return route.fulfill({ status: 502, json: {
        error: { code: 'feed_unavailable', message: 'Publisher feed unavailable' },
      } });
    }
    if (url.pathname.startsWith('/api/')) return route.fulfill({
      status: 503, json: { error: { code: 'fixture_only', message: 'Synthetic API' } },
    });
    return route.continue();
  });
  await page.goto('/podcasts');
  await page.getByLabel('Choose OPML file').setInputFiles({
    name: 'subscriptions.opml', mimeType: 'application/xml',
    buffer: Buffer.from('<opml version="2.0"><body></body></opml>'),
  });
  await expect(page.getByText('2 supported feeds, 0 already followed')).toBeVisible();
  await expect(page.getByText('1 unsupported or private feeds skipped; 1 duplicates skipped.')).toBeVisible();
  await page.getByRole('button', { name: 'Import 2 feeds' }).click();
  await expect(page.getByText('Followed 1 podcast; 1 failed.')).toBeVisible();
  await expect(page.getByText('Podcast Two: feed_unavailable')).toBeVisible();
  expect(imports).toBe(2);
  await expect(page.getByRole('button', { name: 'Import 1 feed' })).toBeEnabled();
});


test('Continue Listening only displays saved-progress entries and opens followed show', async ({ page }) => {
  await page.route('**/*', async route => {
    const url = new URL(route.request().url());
    if (url.origin !== 'http://127.0.0.1:4173') return route.abort();
    if (url.pathname === '/api/v2/podcasts/episodes/continue') {
      expect(url.searchParams.get('limit')).toBe('12');
      return route.fulfill({ json: { episodes: [{
        id: 'episode-42', feedId: feedId, feedTitle: 'Following a podcast',
        title: 'An unfinished episode', positionMs: 185000, listenedMs: 120000,
        durationMs: 1800000, isPlayed: false, progressRevision: 1,
      }] } });
    }
    if (url.pathname === '/api/v2/podcasts/feeds') return route.fulfill({ json: { feeds: [{
      id: feedId, url: feedURL, title: 'Following a podcast', author: 'Publisher',
      description: '', imageUrl: '', language: 'en', episodeCount: 1,
    }] } });
    if (url.pathname.startsWith('/api/')) return route.fulfill({ status: 503,
      json: { error: { code: 'synthetic', message: 'No backend in test' } } });
    return route.continue();
  });
  await page.goto('/podcasts');
  await expect(page.getByRole('heading', { name: 'Continue Listening' })).toBeVisible();
  const section = page.getByRole('region', { name: 'Continue listening' });
  await expect(section.getByText('An unfinished episode')).toBeVisible();
  await expect(section.getByText('Saved at 3 min · View episode')).toBeVisible();
  await expect(section.getByRole('link')).toHaveAttribute('href', '/podcasts/show/' + feedId);
});
