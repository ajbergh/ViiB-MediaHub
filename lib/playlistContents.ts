/** Resolves playlist song identities against the current library. */

import { Song } from '../types';

/** Resolve occurrences in playlist order, retaining repeated tracks. */
export function resolvePlaylistSongs(songIds: string[], songsById: ReadonlyMap<string, Song>): Song[] {
  return songIds.map(id => songsById.get(id)).filter((song): song is Song => Boolean(song));
}
