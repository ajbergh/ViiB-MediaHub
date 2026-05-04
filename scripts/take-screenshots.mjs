/**
 * ViiB MediaHub - Screenshot capture script
 * 
 * Takes screenshots of all UI panels and saves them to /assets/screenshots/
 * 
 * Usage:
 *   node scripts/take-screenshots.mjs
 * 
 * Requires the dev server to be running on http://localhost:3000/
 */

import { chromium } from '@playwright/test';
import { mkdir } from 'fs/promises';
import { fileURLToPath } from 'url';
import path from 'path';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const PROJECT_ROOT = path.resolve(__dirname, '..');
const SCREENSHOTS_DIR = path.join(PROJECT_ROOT, 'assets', 'screenshots');
const BASE_URL = 'http://localhost:3000';
const VIEWPORT = { width: 1440, height: 900 };

// Pages to screenshot
const PAGES = [
  { name: 'home', path: '/', label: 'Home' },
  { name: 'songs', path: '/songs', label: 'Songs' },
  { name: 'albums', path: '/albums', label: 'Albums' },
  { name: 'artists', path: '/artists', label: 'Artists' },
  { name: 'genres', path: '/genres', label: 'Genres' },
  { name: 'playlists', path: '/playlists', label: 'Playlists' },
  { name: 'liked-songs', path: '/liked', label: 'Liked Songs' },
  { name: 'liked-albums', path: '/liked-albums', label: 'Liked Albums' },
  { name: 'smart-playlists', path: '/smart-playlists', label: 'Smart Playlists / AI DJ' },
  { name: 'search', path: '/search', label: 'Search' },
  { name: 'spotify', path: '/spotify', label: 'Spotify' },
  { name: 'dj-mode', path: '/dj', label: 'DJ Mode' },
  { name: 'downloads', path: '/downloads', label: 'Downloads' },
  { name: 'stats', path: '/stats', label: 'Stats' },
  { name: 'settings', path: '/settings', label: 'Settings' },
];

async function waitForContentLoaded(page) {
  // Wait for the main content area to be stable (no skeletons, no loading spinners)
  try {
    await page.waitForLoadState('networkidle', { timeout: 8000 });
  } catch {
    // networkidle can time out on SSE connections — that's fine
  }
  // Small extra settle time for animations
  await page.waitForTimeout(800);
}

async function main() {
  await mkdir(SCREENSHOTS_DIR, { recursive: true });
  console.log(`📁 Output directory: ${SCREENSHOTS_DIR}`);

  const browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({ viewport: VIEWPORT });
  const page = await context.newPage();

  // Suppress console errors from the page
  page.on('console', msg => {
    if (msg.type() === 'error') return;
  });

  for (const { name, path: pagePath, label } of PAGES) {
    const url = `${BASE_URL}${pagePath}`;
    const outFile = path.join(SCREENSHOTS_DIR, `${name}.png`);
    console.log(`📸 ${label} — ${url}`);
    try {
      await page.goto(url, { waitUntil: 'domcontentloaded', timeout: 15000 });
      await waitForContentLoaded(page);
      await page.screenshot({ path: outFile, fullPage: false });
      console.log(`   ✓ Saved: assets/screenshots/${name}.png`);
    } catch (err) {
      console.error(`   ✗ Failed: ${err.message}`);
    }
  }

  // ── Special panel screenshots ──────────────────────────────────────────────

  // Queue panel open (navigate to songs, play a song to populate state, open queue)
  try {
    console.log('📸 Player — Queue panel open');
    await page.goto(`${BASE_URL}/songs`, { waitUntil: 'domcontentloaded', timeout: 15000 });
    await waitForContentLoaded(page);
    // Press Q to open queue
    await page.keyboard.press('q');
    await page.waitForTimeout(600);
    await page.screenshot({ path: path.join(SCREENSHOTS_DIR, 'player-queue.png'), fullPage: false });
    console.log('   ✓ Saved: assets/screenshots/player-queue.png');
  } catch (err) {
    console.error(`   ✗ Failed (queue panel): ${err.message}`);
  }

  // EQ panel open
  try {
    console.log('📸 Player — Equalizer panel open');
    await page.goto(`${BASE_URL}/songs`, { waitUntil: 'domcontentloaded', timeout: 15000 });
    await waitForContentLoaded(page);
    // Close queue if open, then open EQ
    await page.keyboard.press('q');
    await page.waitForTimeout(300);
    await page.keyboard.press('e');
    await page.waitForTimeout(600);
    await page.screenshot({ path: path.join(SCREENSHOTS_DIR, 'player-equalizer.png'), fullPage: false });
    console.log('   ✓ Saved: assets/screenshots/player-equalizer.png');
  } catch (err) {
    console.error(`   ✗ Failed (EQ panel): ${err.message}`);
  }

  await browser.close();
  console.log('\n✅ Screenshot capture complete!');
  console.log(`   ${PAGES.length + 2} screenshots saved to: assets/screenshots/`);
}

main().catch(err => {
  console.error('Fatal error:', err);
  process.exit(1);
});
