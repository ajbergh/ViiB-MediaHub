import { Song } from '../types';
import { isPlexSourcePath } from './artwork';

/** Select distinct covers in playlist order without scanning the library per card. */
export function getPlaylistArtwork(
  songIds: string[],
  songsById: ReadonlyMap<string, Song>,
  albumCovers: Record<string, string> = {},
): string[] {
  const covers = new Set<string>();
  for (const id of songIds) {
    const song = songsById.get(id);
    if (!song) continue;
    const albumKey = `${song.album}::${song.albumArtist || song.artist}`;
    const cover = song.coverUrl || (isPlexSourcePath(song.path)
      ? undefined
      : albumCovers[albumKey] ?? albumCovers[song.album]);
    if (cover) covers.add(cover);
    if (covers.size === 4) break;
  }
  return [...covers];
}
