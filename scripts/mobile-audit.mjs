// Quick mobile UI audit script
import { chromium } from 'playwright';
import { mkdirSync } from 'fs';

const BASE_URL = 'http://localhost:5173';
const OUT = 'output/playwright/mobile-fresh';
mkdirSync(OUT, { recursive: true });

const browser = await chromium.launch({ headless: true });
const page = await browser.newPage();

// ========== Mobile 390x844 ==========
await page.setViewportSize({ width: 390, height: 844 });
await page.goto(BASE_URL + '/songs', { waitUntil: 'domcontentloaded', timeout: 20000 });
await page.waitForTimeout(1500);

const check = await page.evaluate(() => {
  const w = window.innerWidth;
  const mq = window.matchMedia('(min-width: 768px)').matches;
  // Check if sidebar is in flow layout (desktop mode) or fixed (mobile drawer mode)
  const sidebar = document.querySelector('[class*="w-64"]') || document.querySelector('[class*="w-16"]');
  const sidebarFixed = sidebar ? window.getComputedStyle(sidebar).position === 'fixed' : null;
  const sidebarTranslate = sidebar ? window.getComputedStyle(sidebar).transform : null;
  const mobileBar = document.querySelector('header');
  const mobileBarDisplay = mobileBar ? window.getComputedStyle(mobileBar).display : null;
  return { w, mq, sidebarFixed, sidebarTranslate, mobileBarDisplay };
});

console.log('Mobile check:', JSON.stringify(check, null, 2));
await page.screenshot({ path: `${OUT}/mobile-390-songs.png`, fullPage: false });

// Navigate home
await page.goto(BASE_URL + '/', { waitUntil: 'domcontentloaded', timeout: 20000 });
await page.waitForTimeout(800);
await page.screenshot({ path: `${OUT}/mobile-390-home.png`, fullPage: false });

await browser.close();
console.log('Done. Check output/playwright/mobile-fresh/');
