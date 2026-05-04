// DJv2 Playwright audit — captures screenshots at multiple resolutions and collects layout metrics
import { chromium } from 'playwright';
import { mkdir, writeFile } from 'fs/promises';

await mkdir('output/playwright/djv2-audit', { recursive: true });

const viewports = [
  { name: '1920x1080', w: 1920, h: 1080 },
  { name: '1440x900',  w: 1440, h: 900  },
  { name: '1366x768',  w: 1366, h: 768  },
  { name: '1280x720',  w: 1280, h: 720  },
  { name: '1024x768',  w: 1024, h: 768  },
  { name: '768x1024',  w: 768,  h: 1024 },
];

const browser = await chromium.launch({ headless: true });
const metrics = [];

for (const vp of viewports) {
  const page = await browser.newPage();
  await page.setViewportSize({ width: vp.w, height: vp.h });
  await page.goto('http://localhost:5173/dj-v2', { waitUntil: 'domcontentloaded', timeout: 20000 });
  await page.waitForTimeout(1800);

  const data = await page.evaluate(() => {
    const w = window.innerWidth;
    const h = window.innerHeight;

    const interactive = Array.from(document.querySelectorAll('button, input[type=range], select'));
    const smallTargets = interactive.filter(el => {
      const r = el.getBoundingClientRect();
      return (r.width < 32 || r.height < 32) && r.width > 0;
    }).map(el => {
      const r = el.getBoundingClientRect();
      return {
        tag: el.tagName,
        text: (el.textContent || '').trim().slice(0, 40),
        cls: (el.className || '').toString().slice(0, 80),
        w: Math.round(r.width),
        h: Math.round(r.height),
        x: Math.round(r.x),
        y: Math.round(r.y),
      };
    });

    const overflow = document.body.scrollWidth > w;
    const bodyScrollWidth = document.body.scrollWidth;

    const allButtons = Array.from(document.querySelectorAll('button')).map(b => {
      const r = b.getBoundingClientRect();
      return {
        text: (b.textContent || '').trim().slice(0, 40),
        aria: b.getAttribute('aria-label') || '',
        cls: (b.className || '').toString().slice(0, 80),
        w: Math.round(r.width),
        h: Math.round(r.height),
        x: Math.round(r.x),
        y: Math.round(r.y),
        visible: r.width > 0 && r.height > 0,
      };
    }).filter(b => b.visible);

    const allSliders = Array.from(document.querySelectorAll('input[type=range]')).map(s => {
      const r = s.getBoundingClientRect();
      return {
        id: s.id,
        cls: (s.className || '').toString().slice(0, 80),
        w: Math.round(r.width),
        h: Math.round(r.height),
        x: Math.round(r.x),
        y: Math.round(r.y),
        min: s.min, max: s.max, value: s.value,
      };
    });

    const headings = Array.from(document.querySelectorAll('h1,h2,h3')).map(h => (h.textContent || '').trim().slice(0, 60));

    return { w, h, overflow, bodyScrollWidth, smallTargets, allButtons, allSliders, headings };
  });

  await page.screenshot({ path: 'output/playwright/djv2-audit/' + vp.name + '-viewport.png', fullPage: false });
  await page.screenshot({ path: 'output/playwright/djv2-audit/' + vp.name + '-full.png', fullPage: true });

  metrics.push({ viewport: vp.name, ...data });
  console.log('Captured ' + vp.name + ' — overflow:' + data.overflow + ' smallTargets:' + data.smallTargets.length + ' buttons:' + data.allButtons.length + ' sliders:' + data.allSliders.length);
  await page.close();
}

await browser.close();
await writeFile('output/playwright/djv2-audit/metrics.json', JSON.stringify(metrics, null, 2));
console.log('Done. Metrics written to output/playwright/djv2-audit/metrics.json');
