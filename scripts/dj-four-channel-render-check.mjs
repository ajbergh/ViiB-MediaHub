/** Generate a Playwright CLI eval probe using the production output router. */
import { readFileSync, mkdirSync, writeFileSync } from 'node:fs';
import { transformSync } from 'esbuild';

const compiled = transformSync(readFileSync('lib/djOutputRouting.ts', 'utf8'), {
  loader: 'ts', format: 'iife', globalName: 'DJRouting', target: 'es2022',
}).code;
const probe = `async () => {
  ${compiled}
  const results = [];
  for (const rate of [44100, 48000]) {
    for (const signals of [[0.125, 0.25, 0.5, 0.75], [0, 0, 0.125, 0.25], [0.25, 0.5, 0, 0]]) {
      const context = new OfflineAudioContext(4, 4096, rate);
      const master = context.createGain();
      const headphones = context.createGain();
      const route = DJRouting.connectFourChannelOutput(context, master, headphones);
      for (const [bus, offset] of [[master, 0], [headphones, 2]]) {
        const buffer = context.createBuffer(2, 4096, rate);
        buffer.getChannelData(0).fill(signals[offset]);
        buffer.getChannelData(1).fill(signals[offset + 1]);
        const source = context.createBufferSource();
        source.buffer = buffer;
        source.connect(bus);
        source.start();
      }
      const rendered = await context.startRendering();
      const actual = Array.from({ length: 4 }, (_, channel) => rendered.getChannelData(channel)[1024]);
      if (actual.some((value, index) => Math.abs(value - signals[index]) > 0.000001)) {
        throw new Error(JSON.stringify({ rate, expected: signals, actual }));
      }
      route.disconnect();
      results.push({ rate, expected: signals, actual });
    }
  }
  return { passed: results.length, results };
}`;
mkdirSync('output/playwright', { recursive: true });
writeFileSync('output/playwright/dj-render-probe.js', probe);
writeFileSync('output/playwright/dj-render-check.js', `async page => { return await page.evaluate(${probe}); }`);
console.log('Generated output/playwright/dj-render-check.js for playwright-cli run-code --filename (offline rendering; no physical audio).');
