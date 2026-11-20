/** Maintains the renderer library index and applies catalog updates. */

import { Song } from '../types';
import { LibraryChange } from '../services/libraryV2';

export const compareLibraryText = new Intl.Collator().compare;

/**
 * Computes each text key once and reuses the collator for comparisons.
 */
export function sortByTextKey<T>(
  items: readonly T[],
  getText: (item: T) => string,
  direction: 'asc' | 'desc' = 'asc',
): T[] {
  const len = items.length;
  if (len <= 1) return [...items];
  const keys = new Array<string>(len);
  const indices = new Array<number>(len);
  for (let i = 0; i < len; i++) {
    keys[i] = getText(items[i]);
    indices[i] = i;
  }
  const dir = direction === 'asc' ? 1 : -1;
  indices.sort((a, b) => dir * compareLibraryText(keys[a], keys[b]));
  const sorted = new Array<T>(len);
  for (let i = 0; i < len; i++) {
    sorted[i] = items[indices[i]];
  }
  return sorted;
}

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

function sameSong(a: Song, b: Song): boolean {
  const keys = Object.keys(b) as (keyof Song)[];
  return Object.keys(a).length === keys.length && keys.every(key => {
    const before = a[key];
    const after = b[key];
    return Object.prototype.hasOwnProperty.call(a, key) && (Object.is(before, after) ||
      (Array.isArray(before) && Array.isArray(after) && before.length === after.length &&
        before.every((item, index) => Object.is(item, after[index]))));
  });
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
  private snapshot: Song[] = [];

  initialize(songs: Song[]): Song[] {
    this.songsById = new Map(songs.map(song => [song.id, song]));
    this.sortKeys = new Map([...this.songsById.values()].map(song => [song.id, songSortKey(song)]));
    this.sortIds();
    this.snapshot = this.readSongs();
    return this.toArray();
  }

  apply(changes: LibraryChange[], upserts: Song[]): Song[] {
    const upsertById = new Map(upserts.map(song => [song.id, song]));
    let requiresSort = false;
    let updated = false;
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
      const previous = this.songsById.get(song.id);
      if (previous && sameSong(previous, song)) continue;
      updated = true;
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
    if (updated || deleted.size > 0) this.snapshot = this.readSongs();

    return this.toArray();
  }

  private sortIds(): void {
    const entries = [...this.sortKeys.entries()];
    entries.sort((a, b) => compareLibraryText(a[1], b[1]));
    this.orderedIds = entries.map(([id]) => id);
  }

  toArray(): Song[] {
    return this.snapshot;
  }

  private readSongs(): Song[] {
    return this.orderedIds.map(id => this.songsById.get(id)!);
  }

  size(): number {
    return this.songsById.size;
  }
}
