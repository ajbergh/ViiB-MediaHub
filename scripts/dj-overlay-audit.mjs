import assert from 'node:assert/strict';
import { mkdir, writeFile } from 'node:fs/promises';
import { chromium } from '@playwright/test';

// Run against Vite + backend: DJ_AUDIT_URL defaults to http://localhost:3000/dj.
// A fresh browser context uses synthetic catalog/audio; it never writes library data.
const output = 'output/playwright/dj-overlay';
await mkdir(output, { recursive: true });
const browser = await chromium.launch({ headless: true });
const page = await browser.newPage({ viewport: { width: 1920, height: 1080 } });
const results = [];
const tone = Buffer.alloc(44 + 44100 * 12 * 2);
tone.write('RIFF'); tone.writeUInt32LE(tone.length - 8, 4); tone.write('WAVEfmt ', 8);
tone.writeUInt32LE(16, 16); tone.writeUInt16LE(1, 20); tone.writeUInt16LE(1, 22);
tone.writeUInt32LE(44100, 24); tone.writeUInt32LE(88200, 28);
tone.writeUInt16LE(2, 32); tone.writeUInt16LE(16, 34); tone.write('data', 36);
tone.writeUInt32LE(tone.length - 44, 40);
for (let i = 0; i < (tone.length - 44) / 2; i++) tone.writeInt16LE(Math.round(Math.sin(i * 2 * Math.PI * 220 / 44100) * 500), 44 + i * 2);
await page.route('**/api/audio/dj-audit-*', route => route.fulfill({ contentType: 'audio/wav', body: tone }));
await page.route('**/api/dj/waveform/dj-audit-*', route => route.fulfill({ status: 404, body: '{}' }));
const fixtureSongs = Array.from({ length: 12000 }, (_, i) => ({ id: `dj-audit-${i}`, title: `Audit Track ${String(i).padStart(5, '0')}`, artist: 'DJ Audit', album: 'Overlay regression', duration: 12, filePath: `/api/audio/dj-audit-${i}`, coverPath: '', genre: ['Test'], bpm: 120 }));
await page.route('**/api/songs', route => route.fulfill({ json: fixtureSongs }));
await page.route('**/api/playlists', route => route.fulfill({ json: [{ id: 'audit', name: 'Audit playlist', songIds: fixtureSongs.slice(0, 1000).map(s => s.id) }] }));
const drawer = page.getByRole('region', { name: 'DJ library' });
const search = page.getByRole('textbox', { name: 'Search DJ library' });
const geometry = () => page.locator('[data-dj-workspace], [data-dj-workspace] > div').evaluateAll(elements =>
  elements.map(el => el.getBoundingClientRect().toJSON()));
const deckState = () => page.evaluate(async () => {
  const useStore = window.__djAuditStore;
  const s = useStore.getState();
  return [s.djDeckA, s.djDeckB].map(d => ({ id: d.track?.id, playing: d.isPlaying, tempo: d.tempo, cue: d.cuePoint, volume: d.volume }));
});
const settle = () => page.evaluate(async () => { await Promise.all(document.getAnimations().map(a => a.finished.catch(() => {}))); });
try {
  await page.goto(process.env.DJ_AUDIT_URL || 'http://localhost:3000/dj');
  await page.locator('[data-dj-workspace]').waitFor();
  await page.evaluate(async () => {
    // Match Vite's actual module URL, including an HMR timestamp if present.
    // Importing a bare /store.ts after HMR can create a second Zustand instance.
    const source = await (await fetch('/pages/DJModeV2.tsx')).text();
    const storeUrl = source.match(/import \{ useStore \} from "([^"]+)"/)[1];
    window.__djAuditStore = (await import(storeUrl)).useStore;
  });
  await page.waitForFunction(() => window.__djAuditStore.getState().songs.length === 12000);
  for (const [width, height] of [[1920,1080], [2560,1440], [1840,960], [1600,900], [1440,900]]) {
    await page.setViewportSize({ width, height });
    await settle();
    const before = await geometry();
    const state = await deckState();
    await page.screenshot({ path: `${output}/${width}x${height}-closed.png` });
    await page.getByRole('button', { name: 'Library /', exact: true }).click();
    await search.waitFor(); await settle();
    assert.equal(await search.evaluate(el => el === document.activeElement), true);
    assert.deepEqual(await geometry(), before, 'opening moves workspace');
    assert.deepEqual(await deckState(), state, 'opening changes decks');
    assert.equal(await page.getByRole('button', { name: 'Library /', exact: true }).getAttribute('aria-expanded'), 'true');
    await page.screenshot({ path: `${output}/${width}x${height}-open.png` });
    const rows = await drawer.locator('tbody tr').count();
    assert.ok(rows < 80, `virtualization rendered ${rows} rows`);
    assert.equal(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth), false);
    await search.press('Escape');
    assert.equal(await drawer.isVisible(), false);
    assert.deepEqual(await geometry(), before, 'closing moves workspace');
    results.push({ width, height, geometry: before, renderedRows: rows, displacement: 0 });
  }
  await page.keyboard.press('/'); await search.waitFor();
  await search.fill('Audit Track 00001');
  await page.getByRole('button', { name: 'Load Audit Track 00001 to Deck A', exact: true }).click();
  await page.waitForFunction(() => window.__djAuditStore.getState().djDeckA.track?.id === 'dj-audit-1');
  await search.fill('Audit Track 00002');
  await page.getByRole('button', { name: 'Load Audit Track 00002 to Deck B', exact: true }).click();
  await page.waitForFunction(() => window.__djAuditStore.getState().djDeckB.track?.id === 'dj-audit-2');
  await search.fill('');
  await page.getByRole('columnheader', { name: /Title/ }).click();
  await page.getByRole('button', { name: /^Audit playlist/ }).click();
  await page.getByText('1000 tracks', { exact: true }).waitFor();
  await page.getByRole('button', { name: 'All Tracks', exact: true }).click();
  await page.getByRole('button', { name: 'Columns', exact: true }).click();
  await page.getByRole('checkbox', { name: 'Album', exact: true }).uncheck();
  assert.equal(await page.getByRole('columnheader', { name: /^Album/ }).count(), 0);
  await search.press('Escape');
  await search.fill('Audit Track 00003');
  const row = drawer.locator('tbody tr').filter({ has: page.getByRole('button', { name: 'Load Audit Track 00003 to Deck A', exact: true }) });
  await row.dragTo(page.locator('.dj-deck').first().locator('.dj-deck-info'));
  await page.waitForFunction(() => window.__djAuditStore.getState().djDeckA.track?.id === 'dj-audit-3');
  await page.locator('.dj-deck-info').first().getByText('Audit Track 00003', { exact: true }).waitFor();
  const loaded = await deckState();
  assert.equal(loaded[0].id, 'dj-audit-3');
  await search.fill('wpqozxc 123');
  assert.deepEqual(await deckState(), loaded, 'typing triggers DJ shortcuts');
  await search.press('Escape'); await page.keyboard.press('/');
  assert.equal(await search.inputValue(), 'wpqozxc 123', 'closing loses library search');
  await search.fill('');
  await page.getByRole('button', { name: 'Columns', exact: true }).click();
  await search.press('Escape');
  assert.equal(await drawer.isVisible(), true, 'menu Escape also closes drawer');
  await search.press('Escape');
  assert.equal(await drawer.isVisible(), false);
  await page.keyboard.press('?');
  assert.equal(await page.getByRole('dialog', { name: 'Keyboard shortcuts' }).isVisible(), true);
  await page.keyboard.press('Escape');
  await page.keyboard.press('w'); await page.keyboard.press('p');
  await page.waitForFunction(() => { const s = window.__djAuditStore.getState(); return s.djDeckA.isPlaying && s.djDeckB.isPlaying; });
  const playing = await deckState();
  const positions = await page.evaluate(async () => { const s = window.__djAuditStore.getState(); return [s.djDeckA.position, s.djDeckB.position]; });
  for (let i = 0; i < 5; i++) {
    await page.keyboard.press('/'); await search.waitFor();
    await search.fill(i % 2 ? 'Audit Track 01' : 'Audit Track');
    await drawer.locator('[data-virtuoso-scroller]').evaluate(el => { el.scrollTop = 12000; });
    await search.press('Escape');
  }
  assert.deepEqual(await deckState(), playing, 'browsing changes playing decks');
  await page.waitForFunction((prev) => { const s = window.__djAuditStore.getState(); return s.djDeckA.position > prev[0] && s.djDeckB.position > prev[1]; }, positions);
  await page.keyboard.press('w'); await page.keyboard.press('p');
  await page.getByRole('button', { name: 'BROWSE', exact: true }).click();
  assert.equal(await drawer.isVisible(), true);
  await search.press('Escape');
  await page.getByRole('button', { name: 'Enter fullscreen (F11)', exact: true }).click();
  await page.waitForFunction(() => !!document.fullscreenElement);
  const fullscreenGeometry = await geometry();
  await page.keyboard.press('/'); await search.waitFor(); await settle();
  assert.deepEqual(await geometry(), fullscreenGeometry);
  await search.press('Escape');
  await page.getByRole('button', { name: 'Exit fullscreen', exact: true }).click();
  await page.waitForFunction(() => !document.fullscreenElement);
  await page.setViewportSize({ width: 1439, height: 900 });
  await page.getByRole('heading', { name: /needs a wider screen/ }).waitFor();
  console.log('PASS: 5 desktop geometries, 12,000-track virtualization, focus/Escape/modal priority, typing, sort/playlist/columns, A/B loading + drag, playback continuity, fullscreen, width gate');
  await writeFile(`${output}/results.json`, JSON.stringify(results, null, 2));
} catch (error) {
  await page.screenshot({ path: `${output}/failure.png` });
  console.error(await page.locator('#dj-library-drawer').innerText());
  console.error(await page.evaluate(async () => { const s = window.__djAuditStore.getState(); return { count: s.songs.length, first: s.songs[0]?.title }; }));
  throw error;
} finally { await browser.close(); }
