// DJv2 accessibility + visual-baseline audit (docs/DJV2_UI_REFACTOR_ENGINEERING_PLAN.md §12, Phase 6).
//
// Runs against Vite + backend: DJ_AUDIT_URL defaults to http://localhost:3000/dj.
// For each supported geometry it captures a visual baseline and measures every
// visible control in authored-canvas pixels (the workstation scales as one unit,
// so physical sizes shrink uniformly at small viewports).
//
// Fails on: controls without an accessible name, toggle-looking controls without
// aria-pressed/aria-expanded, primary transport under 44px, page overflow.
// Reports (does not fail): controls under the 32px secondary-target guideline.
import assert from 'node:assert/strict';
import { mkdir, writeFile } from 'node:fs/promises';
import { chromium } from '@playwright/test';

const output = 'output/playwright/djv2-audit';
await mkdir(output, { recursive: true });
const url = process.env.DJ_AUDIT_URL || 'http://localhost:3000/dj';
const geometries = [[1470, 825], [1920, 1080], [2560, 1440], [3840, 2160]];

const browser = await chromium.launch({ headless: true });
const page = await browser.newPage({ viewport: { width: 1920, height: 1080 } });
const report = [];
try {
  await page.goto(url, { waitUntil: 'domcontentloaded' });
  await page.locator('[data-dj-workspace]').waitFor({ timeout: 30000 });

  for (const [width, height] of geometries) {
    await page.setViewportSize({ width, height });
    await page.waitForFunction(({ w, h }) => {
      const canvas = document.querySelector('[data-dj-canvas]');
      const viewport = canvas?.parentElement;
      if (!canvas || !viewport) return false;
      const box = viewport.getBoundingClientRect();
      return Math.abs(canvas.getBoundingClientRect().width / w - Math.min(box.width / w, box.height / h)) < 0.001;
    }, { w: 1856, h: 1090 });
    await page.waitForTimeout(400);

    const data = await page.evaluate(() => {
      const canvas = document.querySelector('[data-dj-canvas]');
      const scale = canvas.getBoundingClientRect().width / canvas.clientWidth;
      const describe = el => (el.getAttribute('aria-label') || el.textContent || '').trim().replace(/\s+/g, ' ').slice(0, 40);
      const controls = [...canvas.querySelectorAll('button, select, summary, input, [role="slider"], [role="tab"]')]
        .filter(el => el.offsetParent !== null && !el.closest('[hidden]') && el.checkVisibility({ visibilityProperty: true, contentVisibilityAuto: true }));
      const measured = controls.map(el => {
        const box = el.getBoundingClientRect();
        return { el, name: describe(el), w: box.width / scale, h: box.height / scale };
      });
      const unnamed = measured.filter(({ el }) => {
        const labelled = el.getAttribute('aria-label') || el.getAttribute('aria-labelledby') || el.getAttribute('title')
          || (el.id && document.querySelector(`label[for="${el.id}"]`)) || el.closest('label');
        return !labelled && !(el.textContent || '').trim();
      }).map(({ el }) => el.outerHTML.slice(0, 120));
      // Buttons that belong to a stateful group (a segmented group where any
      // member exposes state, the top-bar modes, deck-accent toggles) must
      // expose their own state. Momentary actions (zoom, nudge, sync) are exempt.
      const hasState = el => el.hasAttribute('aria-pressed') || el.hasAttribute('aria-expanded') || el.hasAttribute('aria-selected');
      const statefulGroup = el => el.parentElement?.matches('.dj-segmented') && [...el.parentElement.children].some(hasState);
      const statelessToggles = measured.filter(({ el }) => el.tagName === 'BUTTON' && !hasState(el)
        && (el.matches('.dj-topbar-mode') || statefulGroup(el) || (el.matches('[data-deck-accent]') && !el.matches('.dj-transport-sync, [aria-label^="Nudge"]'))))
        .map(({ name }) => name);
      const transport = measured.filter(({ el }) => el.closest('.dj-transport')).map(({ name, h }) => ({ name, h: +h.toFixed(1) }));
      const small = measured.filter(({ w, h }) => w < 31.5 || h < 31.5)
        .map(({ el, name, w, h }) => ({ name, region: el.closest('[data-dj-deck],[data-dj-mixer],.dj-topbar,.dj-upper,.dj-library-affordance')?.className.split(' ')[0] ?? 'other', w: +w.toFixed(0), h: +h.toFixed(0) }));
      return {
        scale: +scale.toFixed(3),
        controls: measured.length,
        unnamed,
        statelessToggles,
        transport,
        small,
        pageOverflow: document.documentElement.scrollWidth > innerWidth || document.documentElement.scrollHeight > innerHeight,
      };
    });

    await page.screenshot({ path: `${output}/${width}x${height}.png` });
    const label = `${width}×${height}`;
    assert.deepEqual(data.unnamed, [], `${label}: controls without an accessible name`);
    assert.deepEqual(data.statelessToggles, [], `${label}: toggle controls without aria-pressed/expanded/selected`);
    assert.ok(data.transport.length >= 6 && data.transport.every(t => t.h >= 44 - 0.5), `${label}: primary transport under 44px: ${JSON.stringify(data.transport)}`);
    assert.equal(data.pageOverflow, false, `${label}: page overflows`);
    report.push({ viewport: label, ...data });
    const byRegion = data.small.reduce((acc, item) => ({ ...acc, [item.region]: (acc[item.region] || 0) + 1 }), {});
    console.log(`${label} scale ${data.scale}: ${data.controls} controls; under 32px (report only): ${JSON.stringify(byRegion)}`);
  }
  await writeFile(`${output}/metrics.json`, JSON.stringify(report, null, 2));
  console.log(`PASS: accessible names, toggle state, 44px transport and no page overflow at ${geometries.length} geometries; baselines in ${output}/`);
} catch (error) {
  await page.screenshot({ path: `${output}/failure.png` });
  throw error;
} finally {
  await browser.close();
}
