import { describe, expect, it } from 'vitest';
import { LibraryIndex } from './libraryIndex';
import { Song } from '../types';

const song = (id: string, title: string, album = 'Album'): Song => ({ id, title, artist: 'Artist', album, duration: 120, url: `/api/audio/${id}`, addedAt: 1 });

describe('LibraryIndex', () => {
  it('applies upserts and deletes by ID', () => {
    const index = new LibraryIndex();
    index.initialize([song('1', 'One'), song('2', 'Two')]);
    const result = index.apply([
      { revision: 1, songId: '1', operation: 'upsert', changedAt: 1 },
      { revision: 2, songId: '2', operation: 'delete', changedAt: 2 },
      { revision: 3, songId: '3', operation: 'upsert', changedAt: 3 },
    ], [song('1', 'One updated'), song('3', 'Three')]);
    expect(result.map(item => item.id)).toEqual(['1', '3']);
    expect(result.find(item => item.id === '1')?.title).toBe('One updated');
    expect(index.size()).toBe(2);
  });

  it('keeps deterministic album and track ordering', () => {
    const index = new LibraryIndex();
    const second = { ...song('2', 'Second', 'B'), trackNumber: 2 };
    const first = { ...song('1', 'First', 'A'), trackNumber: 1 };
    expect(index.initialize([second, first]).map(item => item.id)).toEqual(['1', '2']);
  });
});
