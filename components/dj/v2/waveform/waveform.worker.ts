import { CachedWaveformRenderer } from './cachedWaveformRenderer';
import type { WaveformWorkerRequest, WaveformWorkerResponse } from './waveformWorkerProtocol';
import type { MainWaveformFrame } from './canvasWaveformRenderer';

// Own the offscreen surface here rather than irreversibly transferring the DOM
// canvas. If worker startup/drawing fails, that DOM canvas still supports 2D.
const canvas = new OffscreenCanvas(1, 1);
const ctx = canvas.getContext('2d');
const renderer = new CachedWaveformRenderer();
let state: Omit<MainWaveformFrame, 'position'> | undefined;
let generation = 0;
const send = (message: WaveformWorkerResponse, transfer: Transferable[] = []) => postMessage(message, { transfer });
self.onmessage = (event: MessageEvent<WaveformWorkerRequest>) => {
  try {
    const message = event.data;
    if (message.type === 'state') {
      state = { ...state, ...message.frame } as Omit<MainWaveformFrame, 'position'>;
      generation = message.generation; return;
    }
    if (!ctx || !state || message.generation !== generation) return;
    const { width, height, ratio } = message;
    const pixelWidth = Math.max(1, Math.round(width * ratio));
    const pixelHeight = Math.max(1, Math.round(height * ratio));
    if (canvas.width !== pixelWidth || canvas.height !== pixelHeight) { canvas.width = pixelWidth; canvas.height = pixelHeight; }
    ctx.setTransform(ratio, 0, 0, ratio, 0, 0);
    renderer.draw(ctx, width, height, { ...state, position: message.position }, ratio);
    const bitmap = canvas.transferToImageBitmap();
    send({ type: 'frame', generation, bitmap }, [bitmap]);
  } catch (error) { send({ type: 'error', message: String(error) }); }
};
if (ctx) send({ type: 'ready' });
else send({ type: 'error', message: 'Offscreen 2D context unavailable' });
