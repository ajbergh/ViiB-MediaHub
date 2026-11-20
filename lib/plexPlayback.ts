/** Finds playback fallback candidates for unavailable Plex tracks. */

import type { Song } from '../types';

/** Advances in queue order, skipping Plex entries during a confirmed outage. */
export function findPlexPlaybackFallback(queue: Song[], currentIndex: number, plexAvailable: boolean): number {
  for (let index = currentIndex + 1; index < queue.length; index += 1) {
    if (plexAvailable || queue[index].source !== 'plex') {
      return index;
    }
  }
  return -1;
}
