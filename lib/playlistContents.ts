/** Resolves playlist song identities against the current library. */

import { Song } from '../types';

const indexes = new WeakMap<readonly Song[], ReadonlyMap<string, Song>>();

/** Share the lookup between consumers of the same immutable library snapshot. */
export function indexPlaylistSongs(songs: readonly Song[]): ReadonlyMap<string, Song> {
  const cached = indexes.get(songs);
  if (cached) return cached;
  const index = new Map(songs.map(song => [song.id, song]));
  indexes.set(songs, index);
  return index;
}

/** Resolve occurrences in playlist order, retaining repeated tracks. */
export function resolvePlaylistSongs(songIds: string[], songsById: ReadonlyMap<string, Song>): Song[] {
  return songIds.map(id => songsById.get(id)).filter((song): song is Song => Boolean(song));
}
