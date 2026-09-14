import { chromium } from 'playwright';
import { createServer } from 'node:http';
import { readFileSync } from 'node:fs';
const source = readFileSync('lib/vinylScratch.worklet.js');
const server = createServer((req, res) => {
  res.setHeader('Content-Type', req.url === '/scratch.js' ? 'text/javascript' : 'text/html');
  res.end(req.url === '/scratch.js' ? source : '<html><body>Scratch audio verification</body></html>');
});
await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
let browser;
try {
  browser = await chromium.launch({ channel: 'msedge', headless: true, args: ['--autoplay-policy=no-user-gesture-required'] });
  const page = await browser.newPage();
  await page.goto(`http://127.0.0.1:${server.address().port}`);
  const result = await page.evaluate(async () => {
    const context = new AudioContext({ sampleRate: 48000 });
    await context.audioWorklet.addModule('/scratch.js');
    const node = new AudioWorkletNode(context, 'vinyl-scratch', { numberOfInputs: 0, outputChannelCount: [2] });
    const errors = [];
    node.onprocessorerror = e => errors.push(String(e));
    const analyser = context.createAnalyser();
    analyser.fftSize = 2048;
    const mute = context.createGain(); mute.gain.value = 0;
    node.connect(analyser).connect(mute).connect(context.destination);
    await context.resume();
    const samples = Float32Array.from({length: 960000}, (_, i) => Math.sin(i * 2 * Math.PI * 440 / 48000) * 0.5);
    node.port.postMessage({type: 'load', channels: [samples]});
    await new Promise(resolve => setTimeout(resolve, 100));
    node.port.postMessage({type: 'start', position: 5, token: 1});
    node.port.postMessage({type: 'coast', velocity: -4, targetRate: 1, token: 2});
    await new Promise(resolve => setTimeout(resolve, 50));
    // Deliberately prevent all UI callbacks for longer than the full coast.
    const blockedUntil = performance.now() + 2200;
    while (performance.now() < blockedUntil) {}
    const data = new Float32Array(2048);
    analyser.getFloatTimeDomainData(data);
    const peak = Math.max(...data.map(Math.abs));
    node.port.postMessage({type: 'stop'});
    await new Promise(resolve => setTimeout(resolve, 200));
    const end = new Float32Array(2048);
    analyser.getFloatTimeDomainData(end);
    const tail = Math.max(...end.map(Math.abs));
    await context.close();
    return {peak, tail, errors, finite: data.every(Number.isFinite)};
  });
  console.log(JSON.stringify(result));
  if (result.peak < 0.1 || result.tail > 0.001 || result.errors.length || !result.finite) process.exitCode = 1;
} finally {
  await browser?.close();
  server.close();
}
