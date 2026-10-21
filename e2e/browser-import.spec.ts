import { expect, test, type Page } from '@playwright/test';
import { fileURLToPath } from 'node:url';
import type { Song } from '../types';

// Read-only persistence assertions; import itself uses the production input,
// bundled worker/parser and store. No records or metadata are injected here.
async function persistedSongs(page: Page) {
  return page.evaluate(async () => {
    const database = await new Promise<IDBDatabase>((resolve, reject) => {
      const request = indexedDB.open('mediahub-db');
      request.onsuccess = () => resolve(request.result);
      request.onerror = () => reject(request.error);
    });
    try {
      const songs = await new Promise<Song[]>((resolve, reject) => {
        const request = database.transaction('songs', 'readonly').objectStore('songs').getAll();
        request.onsuccess = () => resolve(request.result);
        request.onerror = () => reject(request.error);
      });
      return Promise.all(songs.map(async song => ({
        id: song.id, title: song.title, artist: song.artist, album: song.album,
        albumArtist: song.albumArtist, trackNumber: song.trackNumber,
        discNumber: song.discNumber, year: song.year, genre: song.genre,
        duration: song.duration, path: song.path, url: song.url,
        coverUrl: song.coverUrl ?? null, hasFileHandle: Boolean(song.fileHandle),
        artworkType: song.coverData instanceof Blob ? song.coverData.type : null,
        artworkPrefix: song.coverData instanceof Blob
          ? [...new Uint8Array(await song.coverData.arrayBuffer()).slice(0, 8)] : [],
      })));
    } finally {
      database.close();
    }
  });
}

test('standard directory upload parses four real formats and retains metadata/artwork after reload', async ({ page, context }) => {
  const errors: string[] = [];
  const workers: string[] = [];
  const dialogs: string[] = [];
  // Exercise the supported upload fallback rather than the OS picker.
  await context.addInitScript(() => { Reflect.deleteProperty(window, 'showDirectoryPicker'); });
  page.on('pageerror', error => errors.push(error.message));
  page.on('worker', worker => workers.push(worker.url()));
  page.on('dialog', async dialog => { dialogs.push(dialog.message()); await dialog.accept(); });
  await context.route('**/*', async route => {
    const url = new URL(route.request().url());
    if (url.origin !== 'http://127.0.0.1:4173') return route.abort();
    if (url.pathname.startsWith('/api/')) {
      return route.fulfill({ status: 503, json: { error: 'Backend unavailable in browser import fixture' } });
    }
    return route.continue();
  });

  await page.goto('/songs');
  await expect(page.getByRole('heading', { name: 'Your library is empty', exact: true })).toBeVisible();
  await page.getByRole('button', { name: 'Add Music', exact: true }).click();
  await expect(page).toHaveURL('/settings');
  await expect(page.getByText('Browser Mode', { exact: true })).toBeVisible();

  // The production button opens its real directory-upload input. The native
  // FileSystemFileHandle branch remains separate from this fallback journey.
  const choosing = page.waitForEvent('filechooser');
  await page.getByRole('button', { name: 'Scan Local Directory', exact: true }).click();
  const chooser = await choosing;
  await chooser.setFiles(fileURLToPath(new URL('../testdata/browser-metadata', import.meta.url)));
  await expect.poll(() => dialogs, { timeout: 20_000 }).toEqual(['Import complete! Added 4 songs.']);
  await expect.poll(async () => (await persistedSongs(page)).length).toBe(4);
  const before = await persistedSongs(page);
  for (const song of before) {
    expect(song).toMatchObject({
      title: 'Fixture Title', artist: 'Fixture Artist', album: 'Fixture Album',
      albumArtist: 'Fixture Album Artist', trackNumber: 3, discNumber: 1,
      year: 2024, artworkType: 'image/png', artworkPrefix: [137, 80, 78, 71, 13, 10, 26, 10],
      url: '', coverUrl: null, hasFileHandle: false,
    });
    expect(song.genre).toContain('Electronic');
    expect(song.duration).toBeGreaterThan(0.9);
    expect(song.duration).toBeLessThan(1.2);
  }
  expect(before.map(song => song.path?.split('.').at(-1)).sort()).toEqual(['flac', 'm4a', 'mp3', 'ogg']);
  expect(workers.some(url => url.includes('browserMetadata.worker'))).toBe(true);

  await page.goto('/songs');
  const rows = page.locator('[data-song-id]');
  await expect(rows).toHaveCount(4);
  await expect(rows.first()).toContainText('Fixture Title');
  await expect(rows.first()).toContainText('Fixture Artist');
  await expect(rows.first()).toContainText('Fixture Album');
  await page.reload();
  await expect(rows).toHaveCount(4);
  await expect(rows.first()).toContainText('Fixture Title');
  await expect.poll(async () => rows.locator('img').evaluateAll(images =>
    images.length === 4 && images.every(image => image instanceof HTMLImageElement && image.complete && image.naturalWidth > 0 && image.src.startsWith('blob:')),
  )).toBe(true);
  expect(await persistedSongs(page)).toEqual(before);
  expect(errors).toEqual([]);
});
