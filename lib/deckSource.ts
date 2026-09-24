import type { Song } from '../types';

/** Transport and graph boundary for one DJ deck. Mixer processing stays in DJAudioEngine. */
export interface DeckSource {
  readonly outputNode: AudioNode;
  load(track: Song): Promise<string>;
  cancelLoad(): void;
  unload(): void;
  play(): Promise<void>;
  pause(): void;
  seek(seconds: number): void;
  setTempo(rate: number): void;
  setKeyLock(enabled: boolean): void;
  /** Loop wrapping is scheduled by DJAudioEngine; this records no native element loop. */
  setLoop(start: number, end: number, enabled: boolean): void;
  getPosition(): number;
  getDuration(): number;
  isPlaying(): boolean;
  isLoaded(): boolean;
  addEventListener(type: 'ended' | 'loadedmetadata' | 'timeupdate', listener: () => void): void;
  removeEventListener(type: 'ended' | 'loadedmetadata' | 'timeupdate', listener: () => void): void;
  dispose(): void;
}
