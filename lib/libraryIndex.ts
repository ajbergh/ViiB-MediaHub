/** Maintains the renderer library index and applies catalog updates. */

import { Song } from '../types';
import { LibraryChange } from '../services/libraryV2';

function songSortKey(song: Song): string {
  return [
    song.album || '',
    song.albumArtist || song.artist || '',
    String(song.discNumber || 0).padStart(4, '0'),
    String(song.trackNumber || 0).padStart(5, '0'),
    song.title || '',
    song.id,
  ].join('\u0000').toLocaleLowerCase();
}

/**
 * Canonical renderer-side song index. Zustand continues to expose an array for
 * compatibility, while synchronization updates are applied by ID and only
 * resort when membership or ordering fields change.
 */
export class LibraryIndex {
  private songsById = new Map<string, Song>();
  private sortKeys = new Map<string, string>();
  private orderedIds: string[] = [];

  initialize(songs: Song[]): Song[] {
    this.songsById = new Map(songs.map(song => [song.id, song]));
    this.sortKeys = new Map([...this.songsById.values()].map(song => [song.id, songSortKey(song)]));
    this.sortIds();
    return this.toArray();
  }

  apply(changes: LibraryChange[], upserts: Song[]): Song[] {
    const upsertById = new Map(upserts.map(song => [song.id, song]));
    let requiresSort = false;
    const deleted = new Set<string>();

    for (const change of changes) {
      const song = change.operation === 'delete' ? undefined : upsertById.get(change.songId);
      if (!song) {
        if (this.songsById.delete(change.songId)) {
          this.sortKeys.delete(change.songId);
          deleted.add(change.songId);
        }
        continue;
      }
      const sortKey = songSortKey(song);
      if (this.sortKeys.get(song.id) !== sortKey) {
        this.sortKeys.set(song.id, sortKey);
        requiresSort = true;
      }
      this.songsById.set(song.id, song);
    }

    if (requiresSort) {
      this.sortIds();
    } else if (deleted.size > 0) {
      this.orderedIds = this.orderedIds.filter(id => !deleted.has(id));
    }

    return this.toArray();
  }

  private sortIds(): void {
    this.orderedIds = [...this.sortKeys.keys()].sort((a, b) => this.sortKeys.get(a)!.localeCompare(this.sortKeys.get(b)!));
  }

  toArray(): Song[] {
    return this.orderedIds.map(id => this.songsById.get(id)).filter((song): song is Song => Boolean(song));
  }

  size(): number {
    return this.songsById.size;
  }
}
