import { expect, test } from '@playwright/test';

// A fresh browser context exercises real IndexedDB fallback. Every API request
// fails locally; no backend, private library, provider account or credentials.
test.beforeEach(async ({ page }) => {
  await page.route('**/*', async route => {
    const url = new URL(route.request().url());
    if (url.origin !== 'http://127.0.0.1:4173') return route.abort();
    if (url.pathname.startsWith('/api/')) {
      return route.fulfill({ status: 503, json: { error: 'Backend unavailable in browser-only fixture' } });
    }
    return route.continue();
  });
});

test('empty library offers a working route to source settings', async ({ page }) => {
  const errors: string[] = [];
  page.on('pageerror', error => errors.push(error.message));
  await page.goto('/songs');
  await expect(page.getByRole('heading', { name: 'Your library is empty', exact: true })).toBeVisible();
  await page.getByRole('button', { name: 'Add Music', exact: true }).click();
  await expect(page).toHaveURL('/settings');
  await expect(page.getByRole('heading', { name: 'Settings', exact: true })).toBeVisible();
  expect(errors).toEqual([]);
});

test('legacy DJ route opens the workstation and survives reload offline', async ({ page }) => {
  const errors: string[] = [];
  page.on('pageerror', error => errors.push(error.message));
  await page.goto('/dj-v2');
  await expect(page).toHaveURL('/dj');
  await expect(page.locator('[data-dj-workspace]')).toBeVisible();
  await expect(page.locator('[data-dj-canvas]')).toBeVisible();
  await page.reload();
  await expect(page.locator('[data-dj-workspace]')).toBeVisible();
  expect(errors).toEqual([]);
});
