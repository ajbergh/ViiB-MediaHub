/** Implements the deck-source contract for a single audio track. */

import type { Song } from '../types';
import type { DeckSource, ScratchEvent } from './deckSource';
import { BufferedDeckTransport } from './bufferedDeckTransport';

export class SingleTrackDeckSource implements DeckSource {
  readonly outputNode: GainNode;
  private readonly mediaSource: MediaElementAudioSourceNode;
  private loadGeneration = 0;
  private cancelPendingLoad: (() => void) | null = null;
  private loopBuffer: AudioBuffer | null = null;
  private loop = { start: 0, end: 0, enabled: false };
  private loopPlayback: { node: AudioBufferSourceNode; position: number; time: number; rate: number } | null = null;
  private buffered: BufferedDeckTransport | null = null;
  private bufferPreparation: AbortController | null = null;
  private tempo: number;
  private keyLock: boolean;
  private readonly transportListeners = {
    ended: new Set<() => void>(),
    timeupdate: new Set<() => void>(),
  };

  constructor(
    private readonly audio: HTMLAudioElement,
    private readonly context: AudioContext,
  ) {
    this.outputNode = context.createGain();
    this.mediaSource = context.createMediaElementSource(audio);
    this.mediaSource.connect(this.outputNode);
    audio.crossOrigin = 'anonymous';
    this.tempo = audio.playbackRate;
    this.keyLock = audio.preservesPitch;
  }

  static create(context: AudioContext): SingleTrackDeckSource {
    return new SingleTrackDeckSource(new Audio(), context);
  }

  async load(track: Song): Promise<string> {
    this.clearBufferedTransport();
    this.stopBufferedLoop();
    this.loopBuffer = null;
    this.loop = { start: 0, end: 0, enabled: false };
    this.cancelLoad();
    const generation = ++this.loadGeneration;
    const url = track.url || `/api/audio/${track.id}`;
    this.audio.pause();
    this.audio.currentTime = 0;
    this.audio.src = url;

    await new Promise<void>((resolve, reject) => {
      let settled = false;
      const timeoutId = setTimeout(() => finish(new Error(`Timeout loading track: ${track.title}`)), 30000);
      const cleanup = () => {
        clearTimeout(timeoutId);
        this.audio.removeEventListener('canplay', onCanPlay);
        this.audio.removeEventListener('error', onError);
        if (this.cancelPendingLoad === cancel) this.cancelPendingLoad = null;
      };
      const finish = (error?: Error) => {
        if (settled) return;
        settled = true;
        cleanup();
        if (error) reject(error); else resolve();
      };
      const cancel = () => {
        const error = new Error(`Track load superseded: ${track.title}`);
        error.name = 'AbortError';
        finish(error);
      };
      const onCanPlay = () => {
        if (generation !== this.loadGeneration) return cancel();
        finish();
      };
      const onError = () => {
        if (generation !== this.loadGeneration) return cancel();
        const error = this.audio.error?.message || 'Unknown error';
        finish(new Error(`Failed to load track: ${track.title} - ${error}`));
      };
      this.cancelPendingLoad = cancel;
      this.audio.addEventListener('canplay', onCanPlay);
      this.audio.addEventListener('error', onError);
      if (this.audio.readyState >= 3) onCanPlay();
      else this.audio.load();
    });
    return url;
  }

  cancelLoad(): void {
    this.cancelPendingLoad?.();
    this.cancelPendingLoad = null;
    ++this.loadGeneration;
  }

  unload(): void {
    this.cancelLoad();
    this.clearBufferedTransport();
    this.stopBufferedLoop();
    this.loopBuffer = null;
    this.loop = { start: 0, end: 0, enabled: false };
    this.audio.pause();
    this.audio.src = '';
    this.audio.currentTime = 0;
  }

  async play(): Promise<void> {
    if (this.buffered) { this.buffered.play(); return; }
    if (this.loopPlayback) return;
    if (this.supportsSampleAccurateLoop()) this.startBufferedLoop(this.getPosition());
    else {
      const generation = this.loadGeneration;
      await this.audio.play().catch(error => {
        if (generation !== this.loadGeneration || !this.buffered?.playing) throw error;
      });
    }
  }
  pause(): void {
    if (this.buffered) { this.buffered.pause(); return; }
    if (this.loopPlayback) {
      const position = this.getPosition();
      this.stopBufferedLoop();
      this.audio.currentTime = position;
    }
    this.audio.pause();
  }

  seek(seconds: number): void {
    const duration = this.getDuration();
    if (!duration) return;
    const position = Math.max(0, Math.min(seconds, duration));
    if (this.buffered) this.buffered.seek(position);
    else if (this.loopPlayback) this.startBufferedLoop(position);
    else this.audio.currentTime = position;
  }

  setTempo(rate: number): void {
    if (!Number.isFinite(rate)) return;
    const nextRate = Math.max(0.5, Math.min(1.5, rate));
    this.tempo = nextRate;
    if (this.buffered) { this.buffered.setTempo(nextRate); return; }
    if (this.audio.playbackRate === nextRate) return;
    const position = this.getPosition();
    this.audio.playbackRate = nextRate;
    if (this.loopPlayback && this.supportsSampleAccurateLoop()) {
      const time = this.context.currentTime;
      this.loopPlayback.node.playbackRate.setValueAtTime(nextRate, time);
      Object.assign(this.loopPlayback, { position, time, rate: nextRate });
    } else {
      this.refreshLoopPlayback(position);
    }
  }

  setKeyLock(enabled: boolean): void {
    this.keyLock = enabled;
    if (this.buffered) { this.buffered.setKeyLock(enabled); return; }
    if (this.audio.preservesPitch === enabled) return;
    this.audio.preservesPitch = enabled;
    this.refreshLoopPlayback(this.getPosition());
  }

  setLoop(start: number, end: number, enabled: boolean): void {
    if (start === this.loop.start && end === this.loop.end && enabled === this.loop.enabled) return;
    const position = this.getPosition();
    this.loop = { start, end, enabled };
    if (this.buffered) this.buffered.setLoop(this.loop);
    else this.refreshLoopPlayback(position);
  }

  setLoopBuffer(buffer: AudioBuffer | null): void {
    if (!buffer) this.recoverMediaTransport();
    if (buffer === this.loopBuffer) return;
    const position = this.getPosition();
    this.loopBuffer = buffer;
    this.refreshLoopPlayback(position);
  }

  get scratchPort() { return this.buffered?.scratchPort ?? null; }

  async prepareBufferedTransport(
    buffer: AudioBuffer, emitScratch: (event: ScratchEvent) => void, onError: () => void,
  ): Promise<boolean> {
    this.bufferPreparation?.abort();
    const pending = new AbortController();
    this.bufferPreparation = pending;
    let transport: BufferedDeckTransport | undefined;
    try {
      transport = await BufferedDeckTransport.prepare(
        this.context, this.outputNode, buffer, pending.signal,
        type => this.transportListeners[type].forEach(listener => listener()),
        emitScratch,
        () => { this.recoverMediaTransport(); onError(); },
      );
      if (pending.signal.aborted) { transport.dispose(); return false; }
      const position = this.getPosition();
      const wasPlaying = this.isPlaying();
      transport.setTempo(this.tempo);
      transport.setKeyLock(this.keyLock);
      transport.setLoop(this.loop);
      transport.seek(position);
      this.stopBufferedLoop();
      if (wasPlaying) transport.play();
      this.buffered = transport;
      this.audio.pause();
      return true;
    } catch (error) {
      transport?.dispose();
      if (!pending.signal.aborted) console.warn('Buffered DJ playback unavailable', error);
      return false;
    } finally {
      if (this.bufferPreparation === pending) this.bufferPreparation = null;
    }
  }

  private clearBufferedTransport(): void {
    this.bufferPreparation?.abort();
    this.bufferPreparation = null;
    this.buffered?.dispose();
    this.buffered = null;
  }

  private recoverMediaTransport(): void {
    if (!this.buffered) { this.clearBufferedTransport(); return; }
    const position = this.getPosition();
    const wasPlaying = this.isPlaying();
    this.clearBufferedTransport();
    this.resumeMediaTransport(position, wasPlaying);
  }

  private resumeMediaTransport(position: number, wasPlaying: boolean): void {
    this.audio.playbackRate = this.tempo;
    this.audio.preservesPitch = this.keyLock;
    this.audio.currentTime = position;
    if (wasPlaying) void this.play().catch(() => {});
  }

  supportsSampleAccurateLoop(): boolean {
    if (this.buffered) return this.buffered.supportsSampleAccurateLoop();
    return !!this.loopBuffer && this.loop.enabled && this.loop.start >= 0 && this.loop.end > this.loop.start &&
      this.loop.end <= this.loopBuffer.duration &&
      (!this.audio.preservesPitch || this.audio.playbackRate === 1);
  }

  private wrapBufferedPosition(position: number): number {
    const { start, end } = this.loop;
    return position >= end ? start + (position - start) % (end - start) : position;
  }

  private refreshLoopPlayback(position: number): void {
    if (!this.isPlaying()) return;
    if (this.supportsSampleAccurateLoop()) {
      this.startBufferedLoop(position);
    } else if (this.loopPlayback) {
      this.stopBufferedLoop();
      this.audio.currentTime = position;
      void this.audio.play().catch(() => {});
    }
  }

  private startBufferedLoop(position: number): void {
    this.stopBufferedLoop();
    const node = this.context.createBufferSource();
    const rate = this.audio.playbackRate;
    const time = this.context.currentTime;
    const offset = this.wrapBufferedPosition(position);
    node.buffer = this.loopBuffer;
    node.loop = true;
    node.loopStart = this.loop.start;
    node.loopEnd = this.loop.end;
    node.playbackRate.value = rate;
    node.connect(this.outputNode);
    this.audio.pause();
    node.start(time, offset);
    this.loopPlayback = { node, position: offset, time, rate };
  }

  private stopBufferedLoop(): void {
    this.loopPlayback?.node.stop();
    this.loopPlayback?.node.disconnect();
    this.loopPlayback = null;
  }

  getPosition(): number {
    if (this.buffered) return this.buffered.getPosition();
    const active = this.loopPlayback;
    return active
      ? this.wrapBufferedPosition(active.position + (this.context.currentTime - active.time) * active.rate)
      : this.audio.currentTime;
  }
  getDuration(): number { return this.buffered?.duration ?? (this.audio.duration || 0); }
  isPlaying(): boolean { return this.buffered?.playing ?? (!!this.loopPlayback || !this.audio.paused); }
  isLoaded(): boolean { return !!this.buffered || (!!this.audio.src && this.audio.readyState >= 2); }

  addEventListener(type: 'ended' | 'loadedmetadata' | 'timeupdate', listener: () => void): void {
    this.audio.addEventListener(type, listener);
    if (type !== 'loadedmetadata') this.transportListeners[type].add(listener);
  }

  removeEventListener(type: 'ended' | 'loadedmetadata' | 'timeupdate', listener: () => void): void {
    this.audio.removeEventListener(type, listener);
    if (type !== 'loadedmetadata') this.transportListeners[type].delete(listener);
  }

  dispose(): void {
    this.unload();
    this.mediaSource.disconnect();
  }
}
