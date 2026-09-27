import assert from 'node:assert/strict';
import { mkdir, writeFile } from 'node:fs/promises';
import { chromium } from '@playwright/test';

// Run against Vite + backend: DJ_AUDIT_URL defaults to http://localhost:3000/dj.
// A fresh browser context uses synthetic catalog/audio; it never writes library data.
const output = 'output/playwright/dj-overlay';
await mkdir(output, { recursive: true });
const disableWebGL = process.env.DJ_AUDIT_DISABLE_WEBGL === '1';
const browser = await chromium.launch({
  headless: true,
  // Exercise the Canvas fallback in addition to the normal WebGL path.
  args: disableWebGL ? ['--disable-webgl'] : [],
});
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
const geometry = () => page.locator('[data-dj-workspace], [data-dj-workspace] > *').evaluateAll(elements =>
  elements.map(el => el.getBoundingClientRect().toJSON()));
const layoutMetrics = () => page.evaluate(() => {
  const rect = element => element.getBoundingClientRect();
  const canvas = document.querySelector('[data-dj-canvas]');
  const viewport = canvas.parentElement;
  const workspace = document.querySelector('[data-dj-workspace]');
  const deckA = workspace.querySelector(':scope > [data-dj-deck="A"]');
  const mixer = workspace.querySelector(':scope > [data-dj-mixer]');
  const deckB = workspace.querySelector(':scope > [data-dj-deck="B"]');
  const canvasRect = rect(canvas);
  const scale = canvasRect.width / canvas.clientWidth;
  const normalize = element => {
    const box = rect(element);
    return { x: (box.x - canvasRect.x) / scale, y: (box.y - canvasRect.y) / scale, width: box.width / scale, height: box.height / scale };
  };

  return {
    scale,
    canvas: rect(canvas).toJSON(),
    viewport: rect(viewport).toJSON(),
    workspace: {
      clientWidth: workspace.clientWidth,
      clientHeight: workspace.clientHeight,
      scrollWidth: workspace.scrollWidth,
      scrollHeight: workspace.scrollHeight,
    },
    normalized: {
      workspace: normalize(workspace),
      deckA: normalize(deckA),
      mixer: normalize(mixer),
      deckB: normalize(deckB),
    },
  };
});
// Plan §15: 50/50 waveform split, mixer centerline, A/B symmetry, toolbar
// fit and overlap detection, all in normalized authored-canvas pixels.
const structure = () => page.evaluate(() => {
  const canvas = document.querySelector('[data-dj-canvas]');
  const canvasRect = canvas.getBoundingClientRect();
  const scale = canvasRect.width / canvas.clientWidth;
  const n = element => {
    if (!element) return null;
    const box = element.getBoundingClientRect();
    return { x: (box.x - canvasRect.x) / scale, y: (box.y - canvasRect.y) / scale, width: box.width / scale, height: box.height / scale };
  };
  const q = selector => document.querySelector(selector);
  const deckParts = deck => {
    const root = q(`[data-dj-deck="${deck}"]`);
    return Object.fromEntries(['.dj-deck-header', '.dj-deck-toolbar', '.dj-fx-rack', '.dj-deck-performance', '.dj-deck-footer']
      .map(selector => [selector, n(root?.querySelector(selector))]));
  };
  const intersects = (a, b) => a && b && a.x < b.x + b.width - 0.5 && b.x < a.x + a.width - 0.5 && a.y < b.y + b.height - 0.5 && b.y < a.y + a.height - 0.5;
  const overlaps = [];
  const checkSiblings = (label, elements) => {
    const boxes = [...elements].filter(el => el && el.offsetParent !== null).map(el => [el, n(el)]);
    for (let i = 0; i < boxes.length; i++) for (let j = i + 1; j < boxes.length; j++) {
      if (intersects(boxes[i][1], boxes[j][1])) overlaps.push(`${label}: ${boxes[i][0].textContent.trim().slice(0, 16)} × ${boxes[j][0].textContent.trim().slice(0, 16)}`);
    }
  };
  for (const deck of ['A', 'B']) {
    const root = q(`[data-dj-deck="${deck}"]`);
    checkSiblings(`Deck ${deck} toolbar`, root.querySelectorAll('.dj-deck-toolbar button, .dj-deck-toolbar select, .dj-deck-toolbar summary'));
    checkSiblings(`Deck ${deck} header`, root.querySelectorAll('.dj-deck-info > *'));
    checkSiblings(`Deck ${deck} footer`, [root.querySelector('.dj-hotcues'), root.querySelector('.dj-transport')]);
  }
  for (const deck of ['A', 'B']) {
    const root = q(`[data-dj-deck="${deck}"]`);
    checkSiblings(`Deck ${deck} tempo column`, root.querySelectorAll('.dj-deck-tempo [role="group"], .dj-deck-tempo .dj-fader > *'));
  }
  // Interactive controls must stay inside their layout region.
  const escapes = [];
  for (const region of document.querySelectorAll('.dj-deck-header, .dj-deck-toolbar, .dj-deck-tempo, .dj-deck-eq, .dj-deck-footer, .dj-mixer-head, .dj-mixer-channels, .dj-mixer-section, .dj-waveform-lane-header')) {
    const box = n(region);
    for (const control of region.querySelectorAll('button, select, summary, [role="slider"]')) {
      if (control.offsetParent === null || control.closest('.dj-popover, .dj-deck-inspector')) continue;
      const c = n(control);
      if (c.x < box.x - 1 || c.y < box.y - 1 || c.x + c.width > box.x + box.width + 1 || c.y + c.height > box.y + box.height + 1) {
        escapes.push(`${region.className.split(' ')[0]}: ${control.getAttribute('aria-label') || control.textContent.trim().slice(0, 20)}`);
      }
    }
  }
  const mixer = q('[data-dj-mixer]');
  checkSiblings('Mixer sections', mixer.querySelectorAll(':scope > *'));
  for (const deck of ['A', 'B']) {
    const header = q(`[data-dj-waveform-deck="${deck}"] .dj-waveform-lane-header`);
    if (header) checkSiblings(`Waveform lane ${deck} toolbar`, header.children);
  }
  const hiddenOverflow = [...document.querySelectorAll('.dj-deck-toolbar, .dj-deck-info, .dj-fx-rack, .dj-topbar, .dj-waveform-lane-header, .dj-deck-footer, .dj-deck-tempo, .dj-mixer-head, .dj-mixer-channels, .dj-mixer-section')]
    .filter(el => el.scrollWidth > el.clientWidth + 1 || el.scrollHeight > el.clientHeight + 1).map(el => `${el.className} (${el.scrollWidth}×${el.scrollHeight} in ${el.clientWidth}×${el.clientHeight})`);
  return {
    split: n(q('[data-dj-waveform-split]')),
    laneA: n(q('[data-dj-waveform-deck="A"]')),
    laneB: n(q('[data-dj-waveform-deck="B"]')),
    deckA: n(q('[data-dj-deck="A"]')),
    deckB: n(q('[data-dj-deck="B"]')),
    mixer: n(mixer),
    partsA: deckParts('A'),
    partsB: deckParts('B'),
    overlaps,
    escapes,
    hiddenOverflow,
    webglCanvases: document.querySelectorAll('canvas[data-dj-renderer="webgl"]').length,
  };
});
const near = (a, b, tolerance = 1) => Math.abs(a - b) <= tolerance;
const assertStructure = (s, label) => {
  if (s.split) {
    // Lanes are framed panels; the split is exact when both frames are equal,
    // mirror their outer insets, and the gutter between them is centred on 50%.
    const center = s.split.x + s.split.width / 2;
    assert.ok(near(s.laneA.width, s.laneB.width), `${label}: waveform lanes differ in width (${s.laneA.width} vs ${s.laneB.width})`);
    assert.ok(near(s.laneA.x - s.split.x, s.split.x + s.split.width - (s.laneB.x + s.laneB.width)), `${label}: waveform lane outer insets differ`);
    assert.ok(s.laneA.x + s.laneA.width <= center + 0.5 && s.laneB.x >= center - 0.5, `${label}: a waveform lane crosses the 50% line`);
    assert.ok(near((s.laneA.x + s.laneA.width + s.laneB.x) / 2, center), `${label}: waveform gutter is not centred on 50%`);
    assert.ok(near(s.laneA.height, s.laneB.height), `${label}: waveform lanes differ in height`);
    assert.ok(near(s.mixer.x + s.mixer.width / 2, s.split.x + s.split.width / 2), `${label}: mixer centerline misses the waveform divider`);
    assert.ok(s.webglCanvases <= 2, `${label}: ${s.webglCanvases} WebGL waveform canvases (max 2)`);
  }
  assert.ok(near(s.deckA.width, s.deckB.width) && near(s.deckA.height, s.deckB.height), `${label}: decks are not symmetric`);
  assert.ok(s.mixer.width < s.deckA.width, `${label}: mixer is not narrower than a deck`);
  for (const part of Object.keys(s.partsA)) {
    assert.ok(s.partsA[part] && s.partsB[part], `${label}: missing ${part}`);
    assert.ok(near(s.partsA[part].height, s.partsB[part].height) && near(s.partsA[part].y, s.partsB[part].y), `${label}: ${part} differs between decks`);
  }
  assert.deepEqual(s.overlaps, [], `${label}: overlapping controls`);
  assert.deepEqual(s.escapes, [], `${label}: controls outside their region`);
  assert.deepEqual(s.hiddenOverflow, [], `${label}: fixed-size rows overflow their box`);
};
const deckState = () => page.evaluate(async () => {
  const useStore = window.__djAuditStore;
  const s = useStore.getState();
  return [s.djDeckA, s.djDeckB].map(d => ({ id: d.track?.id, playing: d.isPlaying, tempo: d.tempo, cue: d.cuePoint, volume: d.volume }));
});
const settle = () => page.evaluate(async () => { await Promise.all(document.getAnimations().map(a => a.finished.catch(() => {}))); });
// Viewport changes rescale the canvas via ResizeObserver; wait for that before measuring.
const waitForCanvasScale = async () => {
  await page.waitForFunction(({ canvasWidth, canvasHeight }) => {
    const canvas = document.querySelector('[data-dj-canvas]');
    const viewport = canvas?.parentElement;
    if (!canvas || !viewport) return false;
    const canvasRect = canvas.getBoundingClientRect();
    const viewportRect = viewport.getBoundingClientRect();
    const expectedScale = Math.min(viewportRect.width / canvasWidth, viewportRect.height / canvasHeight);
    return Math.abs(canvasRect.width / canvasWidth - expectedScale) < 0.001;
  }, { canvasWidth: 1856, canvasHeight: 1090 });
  await settle();
};
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
  let referenceLayout;
  for (const [width, height] of [[1470,825], [1920,1080], [2560,1440], [3840,2160]]) {
    await page.setViewportSize({ width, height });
    await waitForCanvasScale();
    const before = await geometry();
    const layout = await layoutMetrics();
    assert.ok(layout.workspace.scrollWidth <= layout.workspace.clientWidth + 1, `${width}×${height} workspace has horizontal overflow`);
    assert.ok(layout.workspace.scrollHeight <= layout.workspace.clientHeight + 1, `${width}×${height} workspace has vertical overflow: ${JSON.stringify(layout)}`);
    assert.ok(layout.canvas.right <= layout.viewport.right + 1, `${width}×${height} canvas is cut off on the right`);
    assert.ok(layout.canvas.bottom <= layout.viewport.bottom + 1, `${width}×${height} canvas is cut off at the bottom`);
    if (referenceLayout) {
      for (const key of Object.keys(referenceLayout)) {
        for (const dimension of ['x', 'y', 'width', 'height']) {
          assert.ok(Math.abs(layout.normalized[key][dimension] - referenceLayout[key][dimension]) < 1,
            `${width}×${height} changes normalized ${key}.${dimension}`);
        }
      }
    } else {
      referenceLayout = layout.normalized;
    }
    assertStructure(await structure(), `${width}×${height} empty`);
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
    results.push({ width, height, geometry: before, layout, renderedRows: rows, displacement: 0 });
  }
  await page.keyboard.press('/'); await search.waitFor();
  await search.fill('Audit Track 00001');
  await page.getByRole('button', { name: 'Load Audit Track 00001 to Deck A', exact: true }).click();
  await page.waitForFunction(() => window.__djAuditStore.getState().djDeckA.track?.id === 'dj-audit-1');
  await search.fill('Audit Track 00002');
  await page.getByRole('button', { name: 'Load Audit Track 00002 to Deck B', exact: true }).click();
  await page.waitForFunction(() => window.__djAuditStore.getState().djDeckB.track?.id === 'dj-audit-2');
  await search.press('Escape');
  for (const [width, height] of [[1470,825], [1920,1080], [2560,1440], [3840,2160]]) {
    await page.setViewportSize({ width, height });
    await waitForCanvasScale();
    // Deck inspector: opens anchored inside the deck, stays in bounds, never
    // moves the workspace, and closes on Escape with focus returned.
    const beforeInspector = await geometry();
    const inspectorTrigger = page.getByRole('button', { name: /^Deck A analysis and editing/ });
    await inspectorTrigger.click();
    const inspector = page.locator('[data-dj-deck-inspector="A"]');
    await inspector.waitFor();
    await inspector.getByRole('tab', { name: 'Grid' }).click();
    const inspectorBox = await inspector.boundingBox();
    const deckBox = await page.locator('[data-dj-deck="A"]').boundingBox();
    assert.ok(inspectorBox.y >= deckBox.y && inspectorBox.y + inspectorBox.height <= deckBox.y + deckBox.height + 1, `${width}: inspector leaves the deck`);
    assert.deepEqual(await geometry(), beforeInspector, 'opening the inspector moves the workspace');
    await page.keyboard.press('Escape');
    assert.equal(await inspector.isVisible(), false, 'Escape does not close the inspector');
    assert.equal(await inspectorTrigger.evaluate(el => el === document.activeElement), true, 'inspector does not return focus');
    assertStructure(await structure(), `${width}×${height} loaded`);
    // Every mixer tool must fit its panel without clipping (the panel is
    // budgeted for the tallest tool, Beat FX).
    for (const tool of ['FX Pad', 'Beat FX', 'Sampler']) {
      await page.getByRole('tab', { name: tool, exact: true }).click();
      const fit = await page.locator('.dj-mixer-bottom-body').evaluate(body => {
        const box = body.getBoundingClientRect();
        const content = [...body.querySelectorAll('*')].map(el => el.getBoundingClientRect()).filter(r => r.width > 0 && r.height > 0);
        return {
          scroll: body.scrollHeight <= body.clientHeight + 1 && body.scrollWidth <= body.clientWidth + 1,
          bottom: Math.max(...content.map(r => r.bottom)) <= box.bottom + 1,
          right: Math.max(...content.map(r => r.right)) <= box.right + 1,
        };
      });
      assert.deepEqual(fit, { scroll: true, bottom: true, right: true }, `${width}×${height}: mixer ${tool} tool does not fit its panel`);
    }
    await page.screenshot({ path: `${output}/${width}x${height}-loaded.png` });

    const overflow = await page.locator('.dj-deck-info').evaluateAll(elements => elements.map(el => ({ width: el.clientWidth, scroll: el.scrollWidth })));
    assert.ok(overflow.every(el => el.scroll <= el.width + 1), 'Loaded deck metadata overflows');
    // Mode changes never move deck/mixer geometry; SCOPE swaps only the waveform band.
    const referenceGeometry = await geometry();
    for (const mode of ['SCOPE', 'SCOPE', 'FX', 'DJ']) {
      await page.getByRole('button', { name: mode, exact: true }).click();
      const bounds = await layoutMetrics();
      assert.ok(bounds.workspace.scrollHeight <= bounds.workspace.clientHeight + 1, `${mode} workspace overflows at ${width}: ${JSON.stringify(bounds.workspace)}`);
      assert.deepEqual(await geometry(), referenceGeometry, `${mode} changes deck/mixer geometry at ${width}`);
    }
    assert.ok(await page.locator('[data-dj-waveform-split]').isVisible(), 'waveform hidden in the DJ layout');

  }
  // Renderer: WebGL lanes use one context per deck (two total); without WebGL
  // each lane falls back to Canvas 2D with identical geometry.
  await page.evaluate(() => { const s = window.__djAuditStore.getState(); if (!s.djMixer.useWebGLWaveform) s.toggleWebGLWaveform(); });
  await page.waitForTimeout(600);
  const rendererStructure = await structure();
  assert.equal(rendererStructure.webglCanvases, disableWebGL ? 0 : 2, `expected ${disableWebGL ? 'Canvas fallback' : 'two WebGL lanes'}`);
  assertStructure(rendererStructure, `${disableWebGL ? 'Canvas fallback' : 'WebGL'} renderer`);
  await page.screenshot({ path: `${output}/renderer-${disableWebGL ? 'fallback' : 'webgl'}.png` });
  await page.evaluate(() => { const s = window.__djAuditStore.getState(); if (s.djMixer.useWebGLWaveform) s.toggleWebGLWaveform(); });
  await page.keyboard.press('/'); await search.waitFor();
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
  console.log(`PASS${disableWebGL ? ' (Canvas fallback)' : ''}: 4 proportional desktop geometries, 12,000-track virtualization, focus/Escape/modal priority, typing, sort/playlist/columns, A/B loading + drag, playback continuity, fullscreen, width gate`);
  await writeFile(`${output}/results.json`, JSON.stringify(results, null, 2));
} catch (error) {
  await page.screenshot({ path: `${output}/failure.png` });
  console.error(await page.locator('#dj-library-drawer').innerText());
  console.error(await page.evaluate(async () => { const s = window.__djAuditStore.getState(); return { count: s.songs.length, first: s.songs[0]?.title }; }));
  throw error;
} finally { await browser.close(); }
