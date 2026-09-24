import type { Song } from '../types';
import type { DeckSource } from './deckSource';

/** Existing HTMLAudioElement transport, kept behind the DeckSource boundary. */
export class SingleTrackDeckSource implements DeckSource {
  readonly outputNode: MediaElementAudioSourceNode;
  private loadGeneration = 0;
  private cancelPendingLoad: (() => void) | null = null;

  constructor(
    private readonly audio: HTMLAudioElement,
    context: AudioContext,
  ) {
    this.outputNode = context.createMediaElementSource(audio);
    audio.crossOrigin = 'anonymous';
  }

  static create(context: AudioContext): SingleTrackDeckSource {
    return new SingleTrackDeckSource(new Audio(), context);
  }

  async load(track: Song): Promise<string> {
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
    this.audio.pause();
    this.audio.src = '';
    this.audio.currentTime = 0;
  }

  play(): Promise<void> { return this.audio.play(); }
  pause(): void { this.audio.pause(); }

  seek(seconds: number): void {
    const duration = this.getDuration();
    if (duration) this.audio.currentTime = Math.max(0, Math.min(seconds, duration));
  }

  setTempo(rate: number): void {
    this.audio.playbackRate = Math.max(0.5, Math.min(1.5, rate));
  }

  setKeyLock(enabled: boolean): void { this.audio.preservesPitch = enabled; }

  // Loops remain engine-scheduled seeks, preserving the existing worker/timeupdate cadence.
  setLoop(_start: number, _end: number, _enabled: boolean): void {}

  getPosition(): number { return this.audio.currentTime; }
  getDuration(): number { return this.audio.duration || 0; }
  isPlaying(): boolean { return !this.audio.paused; }
  isLoaded(): boolean { return !!this.audio.src && this.audio.readyState >= 2; }

  addEventListener(type: 'ended' | 'loadedmetadata' | 'timeupdate', listener: () => void): void {
    this.audio.addEventListener(type, listener);
  }

  removeEventListener(type: 'ended' | 'loadedmetadata' | 'timeupdate', listener: () => void): void {
    this.audio.removeEventListener(type, listener);
  }

  dispose(): void { this.unload(); }
}
