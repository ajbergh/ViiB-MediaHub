import type { Song } from '../types';

type SourceAwareTrack = Pick<Song, 'id' | 'source' | 'sourceName' | 'path' | 'fileHash' | 'fileHandle'>;

/** Includes source revision hints so async edits cannot update a different loaded asset with the same catalog ID. */
export function djTrackSourceIdentity(track: SourceAwareTrack | null | undefined): string {
  if (!track) return '';
  return JSON.stringify([
    track.id,
    track.source ?? '',
    track.sourceName ?? '',
    track.path ?? '',
    track.fileHash ?? '',
    track.fileHandle?.name ?? '',
  ]);
}

/** Derive tap tempo from four recent taps, using the median of three intervals to reduce jitter. */
export function tapTempoBpm(timestamps: number[]): number | undefined {
  if (timestamps.length < 4) return undefined;
  const recent = timestamps.slice(-4);
  const intervals = recent.slice(1).map((time, index) => time - recent[index]);
  if (intervals.some(interval => !Number.isFinite(interval) || interval < 100 || interval > 2000)) return undefined;
  intervals.sort((a, b) => a - b);
  const medianInterval = intervals[1];
  const bpm = 60_000 / medianInterval;
  return Number.isFinite(bpm) && bpm > 0 && bpm <= 1000 ? bpm : undefined;
}

export function formatManualBpm(value: number): string {
  if (typeof value !== 'number' || !Number.isFinite(value) || value <= 0) return 'Unknown';
  return value.toFixed(2).replace(/\.0+$/, '').replace(/(\.\d*[1-9])0+$/, '$1');
}
