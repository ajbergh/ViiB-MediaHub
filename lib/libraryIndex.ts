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

function compareSongs(a: Song, b: Song): number {
  return songSortKey(a).localeCompare(songSortKey(b));
}

/**
 * Canonical renderer-side song index. Zustand continues to expose an array for
 * compatibility, while synchronization updates are applied by ID and only
 * resort when membership or ordering fields change.
 */
export class LibraryIndex {
  private songsById = new Map<string, Song>();
  private orderedIds: string[] = [];

  initialize(songs: Song[]): Song[] {
    this.songsById = new Map(songs.map(song => [song.id, song]));
    this.orderedIds = [...this.songsById.values()].sort(compareSongs).map(song => song.id);
    return this.toArray();
  }

  apply(changes: LibraryChange[], upserts: Song[]): Song[] {
    if (this.orderedIds.length === 0 && this.songsById.size === 0) {
      this.initialize([]);
    }

    const upsertById = new Map(upserts.map(song => [song.id, song]));
    let requiresSort = false;
    const deleted = new Set<string>();

    for (const change of changes) {
      if (change.operation === 'delete') {
        if (this.songsById.delete(change.songId)) {
          deleted.add(change.songId);
        }
        continue;
      }

      const song = upsertById.get(change.songId);
      if (!song) continue;
      const previous = this.songsById.get(song.id);
      if (!previous || songSortKey(previous) !== songSortKey(song)) {
        requiresSort = true;
      }
      this.songsById.set(song.id, song);
    }

    if (deleted.size > 0) {
      this.orderedIds = this.orderedIds.filter(id => !deleted.has(id));
    }

    if (requiresSort) {
      this.orderedIds = [...this.songsById.values()].sort(compareSongs).map(song => song.id);
    } else {
      // Append non-order-changing new IDs defensively; normally new membership
      // always requiresSort because there is no previous value.
      for (const id of this.songsById.keys()) {
        if (!this.orderedIds.includes(id)) this.orderedIds.push(id);
      }
    }
    return this.toArray();
  }

  toArray(): Song[] {
    return this.orderedIds.map(id => this.songsById.get(id)).filter((song): song is Song => Boolean(song));
  }

  size(): number {
    return this.songsById.size;
  }
}

export const libraryIndex = new LibraryIndex();
