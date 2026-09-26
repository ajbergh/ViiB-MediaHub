import type { Song } from '../types';
import type { DeckSource } from './deckSource';
import { SingleTrackDeckSource } from './singleTrackDeckSource';
import { stretchModuleUrl } from './stemStretchModule';

export type StemBus = 'vocals' | 'drums' | 'bass' | 'music';
export type StemMode = 'full' | 'stems' | 'fallback';
export interface StemBusState { gain: number; muted: boolean; solo: boolean }
export type StemDeckState = Record<StemBus, StemBusState>;
export interface StemDeckStatus {
  mode: StemMode;
  available: boolean;
  bufferedSeconds: number;
  underruns: number;
  supportsKeyLock: boolean;
  supportsScratch: boolean;
  supportsSampleAccurateLoop: boolean;
  error?: string;
}

interface StemSetDescriptor {
  id: string;
  status: string;
  stemLayout?: 'four' | 'six';
  sampleRate: number;
  channels: number;
  frames: number;
  durationSeconds: number;
}
interface StemStatusResponse { activeSetId?: string; stemSets?: StemSetDescriptor[] }
type WorkletMessage = { type: string; [key: string]: unknown };
interface WorkletPort {
  onmessage: ((event: MessageEvent<WorkletMessage>) => void) | null;
  postMessage(message: unknown, transfer?: Transferable[]): void;
  close(): void;
}
interface StemWorkletNode {
  port: WorkletPort;
  connect(destination: AudioNode, output?: number, input?: number): unknown;
  disconnect(): void;
}
/** Vinyl-scratch protocol messages (see vinylScratch.worklet.js), relayed to the stem worklet. */
export interface ScratchPort { postMessage(message: Record<string, unknown>): void }
export type ScratchEvent = { type: string; token?: number; position?: number; time?: number; rate?: number };
export interface StemDeckSourceOptions {
  fetch?: typeof fetch;
  createFallback?: (context: AudioContext) => DeckSource;
  createWorklet?: (context: AudioContext) => Promise<StemWorkletNode>;
  chunkFrames?: number;
  targetBufferSeconds?: number;
}

const STEM_BUSES: StemBus[] = ['vocals', 'drums', 'bass', 'music'];
const EMPTY_STEM_STATE = (): StemDeckState => ({
  vocals: { gain: 1, muted: false, solo: false },
  drums: { gain: 1, muted: false, solo: false },
  bass: { gain: 1, muted: false, solo: false },
  music: { gain: 1, muted: false, solo: false },
});
const MAX_CHUNK_FRAMES = 32760; // Keep four stereo float32 buses below the endpoint's 4 MiB limit.
const DEFAULT_TARGET_BUFFER_SECONDS = 4;
// A seek this far inside the worklet's retained frames skips the flush and
// refetch; the margin covers evictions still in flight from the audio thread.
const RETAINED_SEEK_MARGIN_SECONDS = 0.5;

/**
 * One deck clock for all four DJ buses. One HTMLAudioElement is retained solely
 * as the full-track fallback; stem audio comes from one four-output worklet.
 */
export class StemDeckSource implements DeckSource {
  readonly outputNode: GainNode;
  readonly fallback: DeckSource;
  private readonly fallbackGain: GainNode;
  private readonly stemGains: Record<StemBus, GainNode>;
  private readonly context: AudioContext;
  private readonly fetcher: typeof fetch;
  private readonly createWorklet?: StemDeckSourceOptions['createWorklet'];
  private readonly chunkFrames: number;
  private readonly targetBufferSeconds: number;
  private worklet: StemWorkletNode | null = null;
  private descriptor: StemSetDescriptor | null = null;
  private song: Song | null = null;
  private mode: StemMode = 'fallback';
  private playing = false;
  private tempo = 1;
  private keyLockRequested = false;
  private generation = 0;
  private controllers = new Set<AbortController>();
  private prefetchPromise: Promise<void> | null = null;
  private nextPrefetchFrame = 0;
  private positionFrame = 0;
  private positionAnchorTime = 0;
  private bufferEndFrame = 0;
  private underruns = 0;
  private error: string | undefined;
  private stemState = EMPTY_STEM_STATE();
  private loop = { startFrame: 0, endFrame: 0, enabled: false };
  private loopFitsBuffer = true;
  private loopPrefetchGeneration = 0;
  private listeners = new Map<'ended' | 'loadedmetadata' | 'timeupdate', Set<() => void>>();
  private stretchReady = false;
  private retainedStartFrame = 0;
  /** Receives held/position/settled replies for scratches sent through {@link scratchPort}. */
  onScratchEvent: ((event: ScratchEvent) => void) | null = null;
  readonly scratchPort: ScratchPort = {
    postMessage: command => this.worklet?.port.postMessage({ type: 'scratch', command }),
  };

  constructor(private readonly audioContext: AudioContext, options: StemDeckSourceOptions = {}) {
    this.context = audioContext;
    // Bound: calling the native fetch as this.fetcher(...) would run it with a
    // non-Window receiver and throw "Illegal invocation".
    this.fetcher = options.fetch ?? fetch.bind(globalThis);
    this.createWorklet = options.createWorklet;
    this.chunkFrames = Math.max(1024, Math.min(MAX_CHUNK_FRAMES, Math.floor(options.chunkFrames ?? MAX_CHUNK_FRAMES)));
    this.targetBufferSeconds = Math.max(2, Math.min(4, options.targetBufferSeconds ?? DEFAULT_TARGET_BUFFER_SECONDS));
    this.outputNode = audioContext.createGain();
    this.fallbackGain = audioContext.createGain();
    this.stemGains = {
      vocals: audioContext.createGain(), drums: audioContext.createGain(),
      bass: audioContext.createGain(), music: audioContext.createGain(),
    };
    this.fallback = options.createFallback?.(audioContext) ?? SingleTrackDeckSource.create(audioContext);
    this.fallback.outputNode.connect(this.fallbackGain);
    this.fallbackGain.connect(this.outputNode);
    for (const bus of STEM_BUSES) this.stemGains[bus].connect(this.outputNode);
    this.fallbackGain.gain.value = 1;
    for (const bus of STEM_BUSES) this.stemGains[bus].gain.value = 0;
  }

  async load(track: Song): Promise<string> {
    this.cancelLoad();
    const generation = ++this.generation;
    this.song = track;
    this.mode = 'fallback';
    this.error = undefined;
    this.playing = false;
    this.underruns = 0;
    this.fallback.unload();
    const fallbackUrl = await this.fallback.load(track);
    if (generation !== this.generation) throw abortError('Stem source load superseded');
    this.positionFrame = 0;
    this.positionAnchorTime = this.context.currentTime;
    this.descriptor = null;
    this.nextPrefetchFrame = 0;
    this.bufferEndFrame = 0;
    this.resetStemState();
    this.loop = { startFrame: 0, endFrame: 0, enabled: false };
    this.loopFitsBuffer = true;

    try {
      const statusController = new AbortController();
      this.controllers.add(statusController);
      const response = await this.fetcher(`/api/v2/stems/${encodeURIComponent(track.id)}`, { cache: 'no-store', signal: statusController.signal });
      this.controllers.delete(statusController);
      if (!response.ok) throw new Error(`Stem status request failed (${response.status})`);
      const status = await response.json() as StemStatusResponse;
      if (generation !== this.generation) throw abortError('Stem source load superseded');
      const descriptor = status.stemSets?.find(set => set.id === status.activeSetId && set.status === 'ready');
      if (descriptor && (descriptor.stemLayout === 'four' || descriptor.stemLayout === 'six')
        && descriptor.sampleRate > 0 && descriptor.frames > 0 && (descriptor.channels === 1 || descriptor.channels === 2)) {
        this.descriptor = descriptor;
        await this.ensureWorklet();
        if (generation !== this.generation) throw abortError('Stem source load superseded');
        this.worklet?.port.postMessage({ type: 'configure', sampleRate: descriptor.sampleRate, channels: descriptor.channels, frames: descriptor.frames, generation });
        this.nextPrefetchFrame = 0;
        this.positionFrame = 0;
        this.bufferEndFrame = 0;
        this.retainedStartFrame = 0;
        this.mode = 'full';
        this.startPrefetch(generation);
        return fallbackUrl;
      }
    } catch (cause) {
      this.stopFrameRequests();
      if ((cause as Error).name === 'AbortError') throw cause;
      this.error = (cause as Error).message || 'Stem package unavailable';
      this.descriptor = null;
    }
    this.mode = 'fallback';
    return fallbackUrl;
  }

  private async ensureWorklet(): Promise<void> {
    if (this.worklet) return;
    const node = this.createWorklet
      ? await this.createWorklet(this.context)
      : await this.createBrowserWorklet();
    this.worklet = node;
    node.connect(this.stemGains.vocals, 0, 0);
    node.connect(this.stemGains.drums, 1, 0);
    node.connect(this.stemGains.bass, 2, 0);
    node.connect(this.stemGains.music, 3, 0);
    node.port.onmessage = event => this.onWorkletMessage(event.data);
    node.port.postMessage({ type: 'keyLock', enabled: this.keyLockRequested });
  }

  private async createBrowserWorklet(): Promise<StemWorkletNode> {
    if (!this.context.audioWorklet || typeof AudioWorkletNode === 'undefined') throw new Error('AudioWorklet is unavailable');
    // Key lock only; the transport reports the stretcher as unavailable without it.
    try { await this.context.audioWorklet.addModule(await stretchModuleUrl()); }
    catch (error) { console.warn('Stem key lock unavailable', error); }
    await this.context.audioWorklet.addModule(new URL('./stemTransport.worklet.js', import.meta.url));
    return new AudioWorkletNode(this.context, 'viib-stem-transport', {
      numberOfInputs: 0, numberOfOutputs: 4, outputChannelCount: [2, 2, 2, 2],
    }) as unknown as StemWorkletNode;
  }

  private onWorkletMessage(message: WorkletMessage): void {
    if (message.type === 'position' && typeof message.frame === 'number') {
      this.positionFrame = message.frame;
      this.positionAnchorTime = this.context.currentTime;
      // Keep the read-ahead topped up; waiting for an underrun leaves a gap.
      if (this.mode === 'stems') this.startPrefetch(this.generation);
      this.emit('timeupdate');
    } else if (message.type === 'underrun') {
      this.underruns++;
      this.startPrefetch(this.generation, true);
    } else if (message.type === 'ended') {
      this.playing = false;
      this.emit('ended');
    } else if (message.type === 'stretch') {
      this.stretchReady = message.ready === true;
    } else if (message.type === 'evicted' && typeof message.frame === 'number') {
      this.retainedStartFrame = Math.max(this.retainedStartFrame, message.frame);
    } else if (message.type === 'scratch' && message.event) {
      this.onScratchEvent?.(message.event as ScratchEvent);
    }
  }

  /** Stem-mode scratching runs in the transport worklet on the buffered stem frames. */
  canScratch(): boolean {
    return this.mode === 'stems' && !!this.descriptor && !!this.worklet;
  }

  private startPrefetch(generation: number, urgent = false): void {
    if (this.prefetchPromise || !this.descriptor || !this.worklet || generation !== this.generation) return;
    this.prefetchPromise = this.prefetchLoop(generation, urgent).finally(() => { this.prefetchPromise = null; });
  }

  private async prefetchLoop(generation: number, urgent: boolean): Promise<void> {
    const descriptor = this.descriptor;
    const worklet = this.worklet;
    if (!descriptor || !worklet) return;
    const targetFrames = Math.ceil(this.targetBufferSeconds * descriptor.sampleRate);
    while (generation === this.generation && this.nextPrefetchFrame < descriptor.frames) {
      const buffered = this.nextPrefetchFrame - this.currentFrame();
      if (!urgent && buffered >= targetFrames) break;
      urgent = false;
      const startFrame = this.nextPrefetchFrame;
      const count = Math.min(this.chunkFrames, descriptor.frames - startFrame, Math.max(1, targetFrames - buffered));
      const controller = new AbortController();
      this.controllers.add(controller);
      try {
        const query = new URLSearchParams({ startFrame: String(startFrame), frameCount: String(count), layout: 'dj4', format: 'f32le' });
        const response = await this.fetcher(`/api/v2/stems/${encodeURIComponent(this.song!.id)}/${encodeURIComponent(descriptor.id)}/frames?${query}`, { signal: controller.signal, cache: 'no-store' });
        if (!response.ok) throw new Error(`Stem frame request failed (${response.status})`);
        const payload = await response.arrayBuffer();
        if (generation !== this.generation || controller.signal.aborted) return;
        this.validateFrameResponse(response, startFrame, count, descriptor, payload.byteLength);
        this.nextPrefetchFrame += count;
        this.bufferEndFrame = this.nextPrefetchFrame;
        worklet.port.postMessage({ type: 'frames', generation, startFrame, frameCount: count, channels: descriptor.channels, data: payload }, [payload]);
      } catch (cause) {
        if ((cause as Error).name !== 'AbortError' && generation === this.generation) this.fallbackFromError(cause as Error);
        return;
      } finally {
        this.controllers.delete(controller);
      }
      if (this.mode === 'stems' && this.playing && this.currentFrame() >= this.bufferEndFrame) {
        // The worklet has advanced into missing data; promptly refill before normal prefetch resumes.
        urgent = true;
      }
    }
  }

  private validateFrameResponse(response: Response, start: number, count: number, descriptor: StemSetDescriptor, bytes: number): void {
    const headers = response.headers;
    const expectedBytes = count * 4 * descriptor.channels * Float32Array.BYTES_PER_ELEMENT;
    if (headers.get('X-Start-Frame') !== String(start) || headers.get('X-Frame-Count') !== String(count)
      || headers.get('X-Sample-Rate') !== String(descriptor.sampleRate) || headers.get('X-Layout') !== 'dj4'
      || headers.get('X-Format') !== 'f32le' || headers.get('X-Channel-Order') !== 'vocals,drums,bass,music'
      || headers.get('X-Channel-Count') !== String(4 * descriptor.channels) || bytes !== expectedBytes) {
      throw new Error('Stem frame response geometry did not match the active package');
    }
  }

  private fallbackFromError(error: Error): void {
    this.error = error.message;
    const position = this.getPosition();
    this.mode = 'fallback';
    this.stopFrameRequests();
    this.fallback.seek(position);
    if (this.playing) void this.fallback.play().catch(() => {});
    this.ramp(this.fallbackGain, 1);
    for (const bus of STEM_BUSES) this.ramp(this.stemGains[bus], 0);
    this.worklet?.port.postMessage({ type: 'pause' });
  }

  private currentFrame(): number {
    const descriptor = this.descriptor;
    if (!descriptor) return 0;
    let frame = this.positionFrame;
    if (this.playing && this.mode === 'stems') frame += Math.max(0, this.context.currentTime - this.positionAnchorTime) * descriptor.sampleRate * this.tempo;
    if (this.loop.enabled && this.loop.endFrame > this.loop.startFrame && frame >= this.loop.endFrame) {
      frame = this.loop.startFrame + ((frame - this.loop.startFrame) % (this.loop.endFrame - this.loop.startFrame));
    }
    return Math.max(0, Math.min(descriptor.frames, Math.floor(frame)));
  }

  async setStemMode(mode: 'full' | 'stems'): Promise<void> {
    if (mode === 'full' || !this.descriptor || !this.worklet) {
      const position = this.getPosition();
      this.fallback.seek(position);
      this.mode = this.descriptor ? 'full' : 'fallback';
      if (this.playing) await this.fallback.play();
      this.worklet?.port.postMessage({ type: 'pause' });
      this.ramp(this.fallbackGain, 1);
      for (const bus of STEM_BUSES) this.ramp(this.stemGains[bus], 0);
      return;
    }
    const generation = this.generation;
    const frame = this.mode === 'stems' ? this.currentFrame() : Math.round(this.fallback.getPosition() * this.descriptor.sampleRate);
    try { await this.prefetchFrom(frame, generation); }
    catch (cause) {
      this.fallbackFromError(cause as Error);
      return;
    }
    if (this.loop.enabled) await this.prefetchLoopFrames(generation, this.loop.startFrame, this.loop.endFrame);
    if (generation !== this.generation || this.mode === 'fallback' || !this.worklet || !this.descriptor) return;
    this.positionFrame = frame;
    this.positionAnchorTime = this.context.currentTime;
    this.worklet.port.postMessage({ type: 'seek', frame, generation });
    this.worklet.port.postMessage({ type: this.playing ? 'play' : 'pause', rate: this.tempo });
    this.mode = 'stems';
    if (this.playing) this.fallback.pause();
    this.ramp(this.fallbackGain, 0);
    this.applyStemGains();
  }

  private async prefetchFrom(frame: number, generation: number): Promise<void> {
    if (!this.descriptor || !this.worklet) return;
    this.stopFrameRequests();
    this.worklet.port.postMessage({ type: 'flush', generation });
    this.nextPrefetchFrame = Math.min(this.descriptor.frames, frame);
    this.retainedStartFrame = this.nextPrefetchFrame;
    this.bufferEndFrame = this.nextPrefetchFrame;
    const wantedEnd = Math.min(this.descriptor.frames, frame + Math.max(this.chunkFrames, Math.floor(this.descriptor.sampleRate * 0.35)));
    while (generation === this.generation && this.nextPrefetchFrame < wantedEnd) {
      const before = this.nextPrefetchFrame;
      await this.fetchNextChunk(generation);
      if (this.nextPrefetchFrame <= before) break;
    }
  }

  private async fetchNextChunk(generation: number): Promise<void> {
    const descriptor = this.descriptor;
    const worklet = this.worklet;
    if (!descriptor || !worklet || !this.song) return;
    const startFrame = this.nextPrefetchFrame;
    const count = Math.min(this.chunkFrames, descriptor.frames - startFrame);
    const controller = new AbortController();
    this.controllers.add(controller);
    try {
      const query = new URLSearchParams({ startFrame: String(startFrame), frameCount: String(count), layout: 'dj4', format: 'f32le' });
      const response = await this.fetcher(`/api/v2/stems/${encodeURIComponent(this.song.id)}/${encodeURIComponent(descriptor.id)}/frames?${query}`, { signal: controller.signal, cache: 'no-store' });
      if (!response.ok) throw new Error(`Stem frame request failed (${response.status})`);
      const payload = await response.arrayBuffer();
      if (generation !== this.generation || controller.signal.aborted) return;
      this.validateFrameResponse(response, startFrame, count, descriptor, payload.byteLength);
      this.nextPrefetchFrame += count;
      this.bufferEndFrame = this.nextPrefetchFrame;
      worklet.port.postMessage({ type: 'frames', generation, startFrame, frameCount: count, channels: descriptor.channels, data: payload }, [payload]);
    } finally { this.controllers.delete(controller); }
  }

  private async prefetchLoopFrames(generation: number, startFrame: number, endFrame: number): Promise<void> {
    const descriptor = this.descriptor;
    const worklet = this.worklet;
    if (!descriptor || !worklet || generation !== this.generation) return;
    const loopGeneration = ++this.loopPrefetchGeneration;
    for (let start = startFrame; start < endFrame; start += this.chunkFrames) {
      if (generation !== this.generation || loopGeneration !== this.loopPrefetchGeneration) return;
      const count = Math.min(this.chunkFrames, endFrame - start);
      const controller = new AbortController();
      this.controllers.add(controller);
      try {
        const query = new URLSearchParams({ startFrame: String(start), frameCount: String(count), layout: 'dj4', format: 'f32le' });
        const response = await this.fetcher(`/api/v2/stems/${encodeURIComponent(this.song!.id)}/${encodeURIComponent(descriptor.id)}/frames?${query}`, { signal: controller.signal, cache: 'no-store' });
        if (!response.ok) throw new Error(`Stem loop frame request failed (${response.status})`);
        const payload = await response.arrayBuffer();
        if (generation !== this.generation || controller.signal.aborted || loopGeneration !== this.loopPrefetchGeneration) return;
        this.validateFrameResponse(response, start, count, descriptor, payload.byteLength);
        worklet.port.postMessage({ type: 'frames', generation, startFrame: start, frameCount: count, channels: descriptor.channels, data: payload }, [payload]);
      } catch (cause) {
        if ((cause as Error).name !== 'AbortError' && generation === this.generation) this.fallbackFromError(cause as Error);
        return;
      } finally { this.controllers.delete(controller); }
    }
  }

  private stopFrameRequests(): void {
    for (const controller of this.controllers) controller.abort();
    this.controllers.clear();
  }

  private ramp(gain: GainNode, value: number): void {
    const now = this.context.currentTime;
    gain.gain.cancelScheduledValues(now);
    gain.gain.setTargetAtTime(value, now, 0.01);
  }

  private resetStemState(): void {
    this.stemState = EMPTY_STEM_STATE();
    for (const bus of STEM_BUSES) this.applyBusGain(bus);
  }

  private applyStemGains(): void { for (const bus of STEM_BUSES) this.applyBusGain(bus); }
  private applyBusGain(bus: StemBus): void {
    const hasSolo = STEM_BUSES.some(name => this.stemState[name].solo);
    const state = this.stemState[bus];
    const audible = !state.muted && (!hasSolo || state.solo) ? state.gain : 0;
    this.ramp(this.stemGains[bus], this.mode === 'stems' ? audible : 0);
  }

  setStemGain(bus: StemBus, gain: number): void {
    this.stemState[bus].gain = Number.isFinite(gain) ? Math.max(0, Math.min(2, gain)) : 1;
    this.applyBusGain(bus);
  }
  setStemMuted(bus: StemBus, muted: boolean): void { this.stemState[bus].muted = muted; this.applyBusGain(bus); }
  setStemSolo(bus: StemBus, solo: boolean): void { this.stemState[bus].solo = solo; this.applyStemGains(); }
  getStemState(): StemDeckState {
    return {
      vocals: { ...this.stemState.vocals }, drums: { ...this.stemState.drums },
      bass: { ...this.stemState.bass }, music: { ...this.stemState.music },
    };
  }
  getStemStatus(): StemDeckStatus {
    const supportsStems = this.mode === 'stems' && !!this.descriptor;
    return {
      mode: this.mode, available: !!this.descriptor,
      bufferedSeconds: this.descriptor ? Math.max(0, (this.bufferEndFrame - this.currentFrame()) / this.descriptor.sampleRate) : 0,
      underruns: this.underruns, supportsKeyLock: !supportsStems || this.stretchReady, supportsScratch: true,
      supportsSampleAccurateLoop: supportsStems && this.loopFitsBuffer, ...(this.error ? { error: this.error } : {}),
    };
  }

  cancelLoad(): void {
    ++this.generation;
    this.stopFrameRequests();
    this.fallback.cancelLoad();
    this.worklet?.port.postMessage({ type: 'pause' });
  }
  unload(): void {
    this.cancelLoad();
    this.fallback.unload();
    this.descriptor = null;
    this.song = null;
    this.mode = 'fallback';
    this.playing = false;
    this.error = undefined;
    this.underruns = 0;
    this.positionFrame = 0;
    this.nextPrefetchFrame = 0;
    this.bufferEndFrame = 0;
    this.loop = { startFrame: 0, endFrame: 0, enabled: false };
    this.loopFitsBuffer = true;
    this.worklet?.port.postMessage({ type: 'flush', generation: this.generation });
    this.ramp(this.fallbackGain, 1);
    for (const bus of STEM_BUSES) this.ramp(this.stemGains[bus], 0);
  }
  async play(): Promise<void> {
    this.playing = true;
    if (this.mode === 'stems') {
      this.worklet?.port.postMessage({ type: 'play', rate: this.tempo });
      this.startPrefetch(this.generation);
    } else await this.fallback.play();
  }
  pause(): void {
    this.playing = false;
    if (this.mode === 'stems') this.worklet?.port.postMessage({ type: 'pause' });
    this.fallback.pause();
  }
  seek(seconds: number): void {
    const position = Math.max(0, Math.min(seconds, this.getDuration()));
    this.fallback.seek(position);
    if (this.descriptor && this.worklet) {
      const frame = Math.round(position * this.descriptor.sampleRate);
      this.positionFrame = frame;
      this.positionAnchorTime = this.context.currentTime;
      this.worklet.port.postMessage({ type: 'seek', frame, generation: this.generation });
      const retainedFrom = this.retainedStartFrame + RETAINED_SEEK_MARGIN_SECONDS * this.descriptor.sampleRate;
      if (this.mode === 'stems' && frame >= retainedFrom && frame < this.bufferEndFrame) {
        // Cues and jog releases usually land in frames the worklet still holds.
        this.startPrefetch(this.generation);
      } else if (this.mode === 'stems') {
        const generation = this.generation;
        void this.prefetchFrom(frame, generation).then(() => {
          if (this.playing && this.mode === 'stems' && generation === this.generation) this.worklet?.port.postMessage({ type: 'play', rate: this.tempo });
        }).catch(cause => this.fallbackFromError(cause as Error));
      }
    }
    this.emit('timeupdate');
  }
  setTempo(rate: number): void {
    this.tempo = Number.isFinite(rate) ? Math.max(0.5, Math.min(1.5, rate)) : 1;
    this.fallback.setTempo(this.tempo);
    this.worklet?.port.postMessage({ type: 'rate', rate: this.tempo });
  }
  setKeyLock(enabled: boolean): void {
    this.keyLockRequested = enabled;
    this.fallback.setKeyLock(enabled);
    this.worklet?.port.postMessage({ type: 'keyLock', enabled });
  }
  setLoop(start: number, end: number, enabled: boolean): void {
    const descriptor = this.descriptor;
    if (!descriptor) return;
    const startFrame = Math.max(0, Math.min(descriptor.frames, Math.round(start * descriptor.sampleRate)));
    const endFrame = Math.max(0, Math.min(descriptor.frames, Math.round(end * descriptor.sampleRate)));
    const maxLoopFrames = Math.floor(this.targetBufferSeconds * descriptor.sampleRate);
    this.loopFitsBuffer = !enabled || endFrame - startFrame <= maxLoopFrames;
    const next = { startFrame, endFrame, enabled: enabled && endFrame > startFrame && endFrame - startFrame <= maxLoopFrames };
    if (next.startFrame === this.loop.startFrame && next.endFrame === this.loop.endFrame && next.enabled === this.loop.enabled) return;
    this.loop = next;
    this.worklet?.port.postMessage({ type: 'loop', ...this.loop, generation: this.generation });
    ++this.loopPrefetchGeneration;
    if (next.enabled) void this.prefetchLoopFrames(this.generation, startFrame, endFrame);
  }
  getPosition(): number { return this.mode === 'stems' && this.descriptor ? this.currentFrame() / this.descriptor.sampleRate : this.fallback.getPosition(); }
  getDuration(): number { return this.descriptor?.durationSeconds || this.fallback.getDuration(); }
  isPlaying(): boolean { return this.mode === 'stems' ? this.playing : this.fallback.isPlaying(); }
  // In stem mode the fallback element is silent and may still be seeking.
  isLoaded(): boolean { return (this.mode === 'stems' && !!this.descriptor) || this.fallback.isLoaded(); }
  addEventListener(type: 'ended' | 'loadedmetadata' | 'timeupdate', listener: () => void): void {
    const set = this.listeners.get(type) ?? new Set();
    set.add(listener);
    this.listeners.set(type, set);
    this.fallback.addEventListener(type, listener);
  }
  removeEventListener(type: 'ended' | 'loadedmetadata' | 'timeupdate', listener: () => void): void {
    this.listeners.get(type)?.delete(listener);
    this.fallback.removeEventListener(type, listener);
  }
  private emit(type: 'ended' | 'loadedmetadata' | 'timeupdate'): void { this.listeners.get(type)?.forEach(listener => listener()); }
  dispose(): void {
    this.unload();
    for (const [type, listeners] of this.listeners) {
      for (const listener of listeners) this.fallback.removeEventListener(type, listener);
      listeners.clear();
    }
    this.fallback.dispose();
    if (this.worklet) {
      this.worklet.port.onmessage = null;
      this.worklet.port.close();
      this.worklet.disconnect();
      this.worklet = null;
    }
    this.outputNode.disconnect();
    this.fallbackGain.disconnect();
    for (const bus of STEM_BUSES) this.stemGains[bus].disconnect();
  }
}

function abortError(message: string): Error { const error = new Error(message); error.name = 'AbortError'; return error; }
