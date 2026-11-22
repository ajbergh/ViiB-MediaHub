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
        description: 'A sample RSS feed', imageUrl: '', language: 'en',
        episodeCount: 1,
      }] : [] } });
    }
    if (u.pathname === '/api/v2/podcasts/feeds/preview' && req.method() === 'POST') {
      expect(req.postDataJSON()).toEqual({ url: feedURL });
      return route.fulfill({ json: { feed: {
        url: feedURL, title: 'Independent Podcast', author: 'Publisher',
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
  await expect(page.getByRole('button', { name: 'Load more episodes' })).toHaveCount(0);
  expect(requestedOffsets).toEqual([0, 1]);
});
