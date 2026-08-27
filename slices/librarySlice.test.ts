import { afterEach, describe, expect, it, vi } from 'vitest';
import { createLibrarySlice } from './librarySlice';
import { backendService } from '../services/backendService';
import { libraryIndex } from '../lib/libraryIndex';
import { Song } from '../types';

vi.mock('../store', () => ({
  useStore: { getState: () => ({}) },
}));

const songs = (count: number): Song[] => Array.from({ length: count }, (_, index) => ({
  id: `song-${index}`,
  title: `Song ${index}`,
  artist: 'Artist',
  album: 'Album',
  duration: 120,
  url: `/api/audio/song-${index}`,
  addedAt: index,
}));

function createTestLibraryState() {
  let state: Record<string, unknown> = {};
  const set = (update: unknown) => {
    const patch = typeof update === 'function'
      ? (update as (current: Record<string, unknown>) => Record<string, unknown>)(state)
      : update;
    Object.assign(state, patch);
  };
  const get = () => state;
  const slice = createLibrarySlice(set as never, get as never, {} as never);
  state = { ...state, ...slice, backendAvailable: true, isScanning: true };
  return state as unknown as typeof slice;
}

describe('library scan polling', () => {
  afterEach(() => {
    vi.useRealTimers();
    vi.restoreAllMocks();
    libraryIndex.initialize([]);
  });

  it('refreshes visible songs every few seconds while a scan is active', async () => {
    vi.useFakeTimers();
    let statusReads = 0;
    vi.spyOn(backendService, 'getScanStatus').mockImplementation(async () => ({
      scanning: statusReads++ < 4,
      progress: 'Indexing local music',
    }));
    vi.spyOn(backendService, 'getAllSongs')
      .mockResolvedValueOnce(songs(50))
      .mockResolvedValueOnce(songs(100))
      .mockResolvedValueOnce(songs(200));
    vi.spyOn(backendService, 'getFolders').mockResolvedValue([]);

    const state = createTestLibraryState();
    await state.pollScanStatus();
    await vi.advanceTimersByTimeAsync(0);
    expect(state.songs).toHaveLength(50);

    await vi.advanceTimersByTimeAsync(3100);
    expect(state.songs).toHaveLength(100);

    await vi.advanceTimersByTimeAsync(1000);
    expect(state.songs).toHaveLength(200);
    expect(state.isScanning).toBe(false);
    expect(backendService.getAllSongs).toHaveBeenCalledTimes(3);
  });
});
