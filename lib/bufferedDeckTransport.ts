import { loadDeckStretchModule, loadDeckTransportModule } from './stemStretchModule';
import type { ScratchEvent, ScratchPort } from './deckSource';

interface Loop {
  start: number;
  end: number;
  enabled: boolean;
}

interface TransportEvent {
  type: string;
  ready?: boolean;
  frame: number;
  time: number;
  rate: number;
  revision: number;
  event?: ScratchEvent;
}

/** Full-track PCM transport for the macOS desktop webview. */
export class BufferedDeckTransport {
  readonly duration: number;
  playing = false;
  private seconds = 0;
  private time = 0;
  private rate = 1;
  private loop = { start: 0, end: 0, enabled: false };
  private revision = 0;
  readonly scratchPort: ScratchPort = {
    postMessage: command => this.node.port.postMessage({ type: 'scratch', command }),
  };

  private constructor(
    private readonly context: AudioContext,
    private readonly node: AudioWorkletNode,
    private readonly sampleRate: number,
    private readonly length: number,
  ) {
    this.duration = length / sampleRate;
  }

  static async prepare(
    context: AudioContext,
    output: AudioNode,
    buffer: AudioBuffer,
    signal: AbortSignal,
    emit: (type: 'ended' | 'timeupdate') => void,
    emitScratch: (event: ScratchEvent) => void,
    onError: () => void,
  ): Promise<BufferedDeckTransport> {
    if (!context.audioWorklet || typeof AudioWorkletNode === 'undefined') throw new Error('AudioWorklet is unavailable');
    await loadDeckStretchModule(context);
    await loadDeckTransportModule(context);
    signal.throwIfAborted();

    const { length, sampleRate } = buffer;
    const channels = Math.min(2, buffer.numberOfChannels);
    const left = buffer.getChannelData(0);
    let samples: Float32Array<ArrayBuffer>;
    if (channels === 2) {
      const right = buffer.getChannelData(1);
      samples = new Float32Array(length * channels);
      for (let i = 0; i < left.length; i++) {
        samples[i * 2] = left[i];
        samples[i * 2 + 1] = right[i];
      }
    } else samples = left.slice();

    const node = new AudioWorkletNode(context, 'viib-stem-transport', {
      numberOfInputs: 0, numberOfOutputs: 1, outputChannelCount: [2], processorOptions: { buses: 1 },
    });
    const transport = new BufferedDeckTransport(context, node, sampleRate, length);

    try {
      await new Promise<void>((resolve, reject) => {
        let timeout: ReturnType<typeof setTimeout> | undefined;
        const onStateChange = () => {
          clearTimeout(timeout);
          if (context.state === 'running') {
            timeout = setTimeout(() => fail(new Error('Timed out preparing buffered DJ playback')), 5000);
          }
        };
        const clear = () => {
          clearTimeout(timeout);
          signal.removeEventListener('abort', abort);
          context.removeEventListener('statechange', onStateChange);
        };
        const fail = (error: unknown) => { clear(); reject(error); };
        const abort = () => fail(signal.reason);
        signal.addEventListener('abort', abort, { once: true });
        context.addEventListener('statechange', onStateChange);
        onStateChange();
        node.onprocessorerror = () => fail(new Error('Buffered DJ audio processor failed to start'));
        node.port.onmessage = ({ data }: MessageEvent<TransportEvent>) => {
          if (data.type !== 'stretch') return;
          if (!data.ready) { fail(new Error('Buffered DJ key lock is unavailable')); return; }
          clear();
          node.onprocessorerror = onError;
          node.port.onmessage = ({ data: event }: MessageEvent<TransportEvent>) => {
            if (event.type === 'scratch' && event.event) emitScratch(event.event);
            else transport.onMessage(event, emit);
          };
          resolve();
        };
        try {
          node.port.postMessage({ type: 'configure', sampleRate, channels, frames: length });
          node.port.postMessage({ type: 'frames', startFrame: 0, frameCount: length, channels, data: samples.buffer }, [samples.buffer]);
          node.connect(output);
        } catch (error) { fail(error); }
      });
    } catch (error) {
      transport.dispose();
      throw error;
    }
    return transport;
  }

  getPosition(): number {
    const seconds = this.seconds + (this.playing ? Math.max(0, this.context.currentTime - this.time) * this.rate : 0);
    return this.wrap(seconds);
  }

  private wrap(seconds: number): number {
    const { start, end, enabled } = this.loop;
    return enabled && seconds >= end ? start + ((seconds - start) % (end - start)) : Math.min(seconds, this.duration);
  }

  setTempo(rate: number): void {
    if (rate === this.rate) return;
    this.seconds = this.getPosition();
    this.time = this.context.currentTime;
    this.rate = rate;
    this.node.port.postMessage({ type: 'rate', rate });
  }

  setKeyLock(enabled: boolean): void {
    this.node.port.postMessage({ type: 'keyLock', enabled });
  }

  setLoop(loop: Loop): void {
    const startFrame = Math.round(loop.start * this.sampleRate);
    const endFrame = Math.round(loop.end * this.sampleRate);
    const enabled = loop.enabled && startFrame >= 0 && endFrame > startFrame && endFrame <= this.length;
    this.seconds = this.getPosition();
    this.time = this.context.currentTime;
    this.loop = { start: startFrame / this.sampleRate, end: endFrame / this.sampleRate, enabled };
    this.node.port.postMessage({ type: 'loop', startFrame, endFrame, enabled });
  }

  seek(seconds: number): void {
    this.seconds = this.wrap(seconds);
    this.time = this.context.currentTime;
    this.node.port.postMessage({ type: 'seek', frame: this.seconds * this.sampleRate, revision: ++this.revision });
  }

  play(): void {
    if (this.playing) return;
    this.seek(this.seconds);
    this.playing = true;
    this.node.port.postMessage({ type: 'play', rate: this.rate });
  }

  pause(): void {
    this.seconds = this.getPosition();
    this.playing = false;
    this.node.port.postMessage({ type: 'pause' });
  }

  private onMessage(event: TransportEvent, emit: (type: 'ended' | 'timeupdate') => void): void {
    if (!this.playing || event.revision !== this.revision) return;
    if (event.type === 'ended') {
      this.playing = false;
      this.seconds = this.duration;
      emit('ended');
    } else if (event.type === 'position' && event.rate === this.rate && event.time >= this.time) {
      this.seconds = event.frame / this.sampleRate;
      this.time = event.time;
      emit('timeupdate');
    }
  }

  supportsSampleAccurateLoop(): boolean { return this.loop.enabled; }

  dispose(): void {
    this.node.onprocessorerror = null;
    this.node.port.onmessage = null;
    this.node.port.postMessage({ type: 'dispose' });
    this.node.port.close();
    this.node.disconnect();
  }
}
