import type { Song } from '../types';

/**
 * Returns the next local track after an unavailable Plex item. Once PMS has
 * failed during playback, trying other Plex entries from the same generated
 * queue only creates a chain of player errors. A local track is the safe
 * source-independent fallback; -1 means the user intentionally queued Plex
 * only tracks and playback should stop with a clear message.
 */
export function findPlexPlaybackFallback(queue: Song[], currentIndex: number): number {
  for (let index = currentIndex + 1; index < queue.length; index += 1) {
    if (queue[index]?.source !== 'plex') {
      return index;
    }
  }
  return -1;
}
