/** Maintains the renderer library index and applies catalog updates. */

import { Song } from '../types';
import { LibraryChange } from '../services/libraryV2';

interface CatalogSongChange {
  before?: Song;
  after?: Song;
}

interface CatalogDelta {
  previous: WeakRef<Song[]>;
  changes: CatalogSongChange[];
  orderChanged: boolean;
}

export const catalogDeltas = new WeakMap<Song[], CatalogDelta>();

export function patchCatalogSong(songs: Song[], id: string, patch: Partial<Song>): Song[] {
  const index = songs.findIndex(song => song.id === id);
  if (index < 0) return songs;
  const before = songs[index];
  const after = { ...before, ...patch };
  if (sameSong(before, after)) return songs;
  const updated = songs.slice();
  updated[index] = after;
  catalogDeltas.set(updated, { previous: new WeakRef(songs), changes: [{ before, after }], orderChanged: false });
  return updated;
}

export function patchCatalogSongs(songs: Song[], patches: ReadonlyMap<string, Partial<Song>>): Song[] {
  const changes: CatalogSongChange[] = [];
  const updated = songs.map(before => {
    const patch = patches.get(before.id);
    if (!patch) return before;
    const after = { ...before, ...patch };
    if (sameSong(before, after)) return before;
    changes.push({ before, after });
    return after;
  });
  if (changes.length === 0) return songs;
  catalogDeltas.set(updated, { previous: new WeakRef(songs), changes, orderChanged: false });
  return updated;
}

// An optimistic patch and a server delta can share the same base snapshot.
// Express the server result relative to the array currently shown by the store.
export function rebaseCatalogDelta(songs: Song[], current: Song[]) {
  const delta = catalogDeltas.get(songs);
  const local = catalogDeltas.get(current);
  const base = delta?.previous.deref();
  if (!delta || !local || !base || local.orderChanged || local.previous.deref() !== base) return;

  const byId = new Map(delta.changes.map(change => [change.after?.id ?? change.before?.id, change]));
  for (const change of local.changes) {
    const id = change.after?.id ?? change.before?.id;
    const incoming = byId.get(id);
    byId.set(id, { before: change.after, after: incoming ? incoming.after : change.before });
  }
  catalogDeltas.set(songs, { ...delta, previous: new WeakRef(current), changes: Array.from(byId.values()) });
}

export function affectsSmartMixes({ before, after }: CatalogSongChange): boolean {
  if (!before || !after) return true;
  const oldGenres = before.genre ?? [];
  const newGenres = after.genre ?? [];
  return before.playCount !== after.playCount || before.lastPlayed !== after.lastPlayed ||
    before.addedAt !== after.addedAt || before.year !== after.year ||
    oldGenres.length !== newGenres.length || oldGenres.some((genre, index) => genre !== newGenres[index]);
}

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
    const priorSongs = new Map<string, Song | undefined>();

    for (const change of changes) {
      if (!priorSongs.has(change.songId)) priorSongs.set(change.songId, this.songsById.get(change.songId));
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
    if (updated || deleted.size > 0) {
      const previous = this.snapshot;
      this.snapshot = this.readSongs();
      catalogDeltas.set(this.snapshot, {
        previous: new WeakRef(previous),
        orderChanged: requiresSort || deleted.size > 0,
        changes: Array.from(priorSongs, ([id, before]) => ({ before, after: this.songsById.get(id) }))
          .filter(({ before, after }) => before !== after),
      });
    }

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
