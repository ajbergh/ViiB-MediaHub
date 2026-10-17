import { CachedWaveformRenderer } from './cachedWaveformRenderer';
import type { MainWaveformFrame } from './canvasWaveformRenderer';
import type { WaveformWorkerRequest, WaveformWorkerResponse } from './waveformWorkerProtocol';

interface DrawRequest { frame: MainWaveformFrame; width: number; height: number; ratio: number }
export type WaveformWorkerFactory = () => Worker;

/** One outstanding bitmap plus one latest frame: worker latency never builds
 * an unbounded queue. Immutable arrays cross the boundary only when changed. */
export class CanvasWaveformDriver {
  private worker?: Worker;
  private ready = false;
  private busy = false;
  private latest?: DrawRequest;
  private staticKey?: Omit<MainWaveformFrame, 'position'>;
  private generation = 0;
  private timeout?: ReturnType<typeof setTimeout>;
  private disposed = false;
  private fallback = new CachedWaveformRenderer();
  constructor(private canvas: HTMLCanvasElement, private ctx: CanvasRenderingContext2D,
    createWorker: WaveformWorkerFactory | null = typeof Worker !== 'undefined' && typeof OffscreenCanvas !== 'undefined'
      ? () => new Worker(new URL('./waveform.worker.ts', import.meta.url), { type: 'module' }) : null) {
    if (!createWorker) { canvas.dataset.djRenderer = 'canvas'; return; }
    try {
      this.worker = createWorker();
      this.worker.onmessage = (event: MessageEvent<WaveformWorkerResponse>) => {
        const message = event.data;
        if (this.disposed) { if (message.type === 'frame') message.bitmap.close(); return; }
        if (message.type === 'ready') { this.ready = true; this.cancelTimeout(); this.canvas.dataset.djRenderer = 'canvas-worker'; this.safeFlush(); }
        else if (message.type === 'error') this.fail();
        else {
          this.busy = false; this.cancelTimeout();
          const pending = this.latest?.frame;
          const current = !pending || Object.keys(this.staticKey ?? {}).every(key => pending[key as keyof MainWaveformFrame] === this.staticKey![key as keyof typeof this.staticKey]);
          if (message.generation === this.generation && this.latestPresented && current) {
            this.ctx.setTransform(1, 0, 0, 1, 0, 0);
            this.ctx.clearRect(0, 0, this.canvas.width, this.canvas.height);
            this.ctx.drawImage(message.bitmap, 0, 0, this.canvas.width, this.canvas.height);
          }
          message.bitmap.close();
          this.safeFlush();
        }
      };
      this.worker.onerror = () => this.fail();
      this.timeout = setTimeout(() => this.fail(), 3000);
    } catch { this.fail(); }
  }
  private cancelTimeout(): void { if (this.timeout) clearTimeout(this.timeout); this.timeout = undefined; }
  private fail(): void {
    this.cancelTimeout(); this.worker?.terminate(); this.worker = undefined; this.busy = false;
    this.canvas.dataset.djRenderer = 'canvas';
    const latest = this.latest ?? this.latestPresented;
    if (latest && !this.disposed) this.drawFallback(latest);
  }
  draw(frame: MainWaveformFrame, width: number, height: number, ratio: number): void {
    if (this.disposed) return;
    this.latest = { frame, width, height, ratio };
    if (!this.worker) this.drawFallback(this.latest);
    else this.safeFlush();
  }
  private drawFallback({ frame, width, height, ratio }: DrawRequest): void {
    this.ctx.setTransform(ratio, 0, 0, ratio, 0, 0);
    this.fallback.draw(this.ctx, width, height, frame, ratio);
  }
  private safeFlush(): void { try { this.flush(); } catch { this.fail(); } }
  private flush(): void {
    if (!this.worker || !this.ready || this.busy || !this.latest) return;
    const { frame, width, height, ratio } = this.latest;
    this.latest = undefined;
    const { position, ...staticFrame } = frame;
    const patch = Object.fromEntries(Object.entries(staticFrame).filter(([key, value]) => !this.staticKey || value !== this.staticKey[key as keyof typeof staticFrame]));
    if (Object.keys(patch).length) {
      this.generation++;
      this.staticKey = staticFrame;
      this.worker.postMessage({ type: 'state', generation: this.generation, frame: patch } satisfies WaveformWorkerRequest);
    }
    // Keep the most recent dimensions for presenting the resulting bitmap.
    this.latestPresented = { frame, width, height, ratio };
    this.busy = true;
    this.worker.postMessage({ type: 'draw', generation: this.generation, position, width, height, ratio } satisfies WaveformWorkerRequest);
    this.timeout = setTimeout(() => this.fail(), 3000);
  }
  private latestPresented?: DrawRequest;
  dispose(): void {
    this.disposed = true; this.cancelTimeout(); this.worker?.terminate(); this.worker = undefined;
    this.latest = undefined; this.latestPresented = undefined; this.fallback.clear();
  }
}
