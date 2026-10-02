// @vitest-environment jsdom
import { afterEach, expect, it } from 'vitest';
import { DEFAULT_SONG_COLUMNS, loadSongColumns, SONG_COLUMNS_STORAGE_KEY, songColumnValue } from './songColumns';
import { Song } from '../types';
afterEach(() => localStorage.clear());
it('loads defaults on malformed preferences and validates stored column names', () => {
  localStorage.setItem(SONG_COLUMNS_STORAGE_KEY, '{broken'); expect(loadSongColumns()).toEqual(DEFAULT_SONG_COLUMNS);
  localStorage.setItem(SONG_COLUMNS_STORAGE_KEY, '{}'); expect(loadSongColumns()).toEqual(DEFAULT_SONG_COLUMNS);
  localStorage.setItem(SONG_COLUMNS_STORAGE_KEY, '["key","bpm","key","unknown"]'); expect(loadSongColumns()).toEqual(['bpm', 'key']);
  localStorage.setItem(SONG_COLUMNS_STORAGE_KEY, '[]'); expect(loadSongColumns()).toEqual([]);
});
it('retains missing values and formats useful catalog fields', () => {
  const song = { genre: ['Rock','Alternative'], year: 2000, originalYear: 1995, source: 'plex', sourceName: 'Home server' } as Song;
  expect(songColumnValue('genre', song)).toBe('Rock, Alternative'); expect(songColumnValue('year', song)).toBe('1995'); expect(songColumnValue('source', song)).toBe('Home server');
  expect(songColumnValue('key', song)).toBe('—'); expect(songColumnValue('lastPlayed', song)).toBe('—'); expect(songColumnValue('plays', song)).toBe('0');
});
