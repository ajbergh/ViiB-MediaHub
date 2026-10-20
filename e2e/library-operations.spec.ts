import { expect, test } from '@playwright/test';

// Synthetic API/UI evidence only: no database activation, provider or private data.
test('recovery route reviews diagnostics, cancels destructive actions and stages only the selected backup', async ({ page }) => {
  const mutations: { path: string; body: unknown }[] = [];
  const errors: string[] = [];
  page.on('pageerror', error => errors.push(error.message));
  await page.route('**/*', async route => {
    const request = route.request();
    const url = new URL(request.url());
    if (url.origin !== 'http://127.0.0.1:4173') return route.abort();
    if (url.pathname === '/api/v2/operations/backups' && request.method() === 'GET') {
      return route.fulfill({ json: { backups: [
        { name: 'other-fixture.zip', size: 1024, createdAt: 1 },
        { name: 'selected-fixture.zip', size: 2048, createdAt: 2 },
      ] } });
    }
    if (url.pathname === '/api/v2/operations/diagnostics' && request.method() === 'GET') {
      return route.fulfill({ json: {
        checkedAt: 1, integrity: 'ok', songCount: 2, searchIndexCount: 2, revision: 1, retainedChanges: 0,
        missingMedia: [{ songId: 'missing-fixture', title: 'Missing fixture', filePath: '/synthetic/missing.mp3', reason: 'missing' }],
        unavailableMedia: [{ songId: 'offline-fixture', title: 'Offline fixture', filePath: '/synthetic/offline.mp3', reason: 'source offline' }],
        brokenPlaylistReferences: [], scannerFailures: [],
      } });
    }
    if (url.pathname === '/api/v2/operations/restore/preview' && request.method() === 'POST') {
      expect(request.postDataJSON()).toEqual({ name: 'selected-fixture.zip' });
      return route.fulfill({ json: { valid: true, restartRequired: true, manifest: {} } });
    }
    if (url.pathname.startsWith('/api/v2/operations/') && request.method() !== 'GET') {
      mutations.push({ path: url.pathname, body: request.postDataJSON() });
      if (url.pathname === '/api/v2/operations/restore/stage') {
        return route.fulfill({ status: 202, json: { staged: true, restartRequired: true, path: '/synthetic/restore-pending' } });
      }
    }
    if (url.pathname.startsWith('/api/')) {
      return route.fulfill({ status: 503, json: { error: 'Unavailable in synthetic recovery fixture' } });
    }
    return route.continue();
  });

  await page.goto('/library-operations');
  await expect(page).toHaveURL('/settings');
  await expect(page.getByRole('heading', { name: 'Diagnostics & Repair', exact: true })).toBeVisible();
  await page.getByRole('button', { name: 'Run diagnostics', exact: true }).click();
  await expect(page.getByRole('list', { name: 'Missing tracks to remove' })).toContainText('Missing fixture');
  await expect(page.getByRole('list', { name: 'Unavailable tracks retained' })).toContainText('Offline fixture');
  const remove = page.getByRole('button', { name: 'Remove missing files', exact: true });
  const prompts: string[] = [];
  page.once('dialog', async dialog => { prompts.push(dialog.message()); await dialog.dismiss(); });
  await remove.click();
  await expect(remove).toBeEnabled();
  expect(prompts).toHaveLength(1);
  expect(prompts[0]).toContain('likes and playlist entries');
  expect(prompts[0]).toContain('Create a backup');
  expect(mutations).toEqual([]);

  const selected = page.getByText('selected-fixture.zip', { exact: true }).locator('../..');
  await selected.getByRole('button', { name: 'Validate', exact: true }).click();
  await expect(page.getByRole('status').filter({ hasText: 'Backup selected-fixture.zip is valid' })).toBeVisible();
  const stage = selected.getByRole('button', { name: 'Stage', exact: true });
  page.once('dialog', async dialog => { prompts.push(dialog.message()); await dialog.dismiss(); });
  await stage.click();
  await expect(stage).toBeEnabled();
  expect(prompts).toHaveLength(2);
  expect(prompts[1]).toContain('catalog, playlists, likes, listening history and settings');
  expect(prompts[1]).toContain('Audio files are unaffected');
  expect(mutations).toEqual([]);

  page.once('dialog', async dialog => { await dialog.accept(); });
  await stage.click();
  await expect(page.getByRole('status').filter({ hasText: 'A restart is required' })).toContainText('Close ViiB before applying');
  expect(mutations).toEqual([{ path: '/api/v2/operations/restore/stage', body: { name: 'selected-fixture.zip' } }]);
  expect(errors).toEqual([]);
});
