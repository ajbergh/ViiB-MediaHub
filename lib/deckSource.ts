/** Defines the common audio-source contract used by DJ decks. */

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
  /** Sources with sample-accurate loop support wrap internally. */
  setLoop(start: number, end: number, enabled: boolean): void;
  supportsSampleAccurateLoop?(): boolean;
  getPosition(): number;
  getDuration(): number;
  isPlaying(): boolean;
  isLoaded(): boolean;
  addEventListener(type: 'ended' | 'loadedmetadata' | 'timeupdate', listener: () => void): void;
  removeEventListener(type: 'ended' | 'loadedmetadata' | 'timeupdate', listener: () => void): void;
  dispose(): void;
}

/** Messages shared by the vinyl and deck transport worklets. */
export interface ScratchPort { postMessage(message: Record<string, unknown>): void }
export type ScratchEvent = { type: string; token?: number; position?: number; time?: number; rate?: number };
