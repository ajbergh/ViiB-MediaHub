/** Tests and fixtures for library Slice behavior. */

import { afterEach, describe, expect, it, vi } from 'vitest';
import { createLibrarySlice } from './librarySlice';
import { backendService } from '../services/backendService';
import { libraryV2 } from '../services/libraryV2';
import { Song } from '../types';
import { libraryOperationsV2 } from '../services/libraryOperationsV2';
import { libraryService } from '../services/libraryService';
import api from '../services/api';

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
  state = { ...state, ...slice, backendAvailable: true, isScanning: true, isLibraryInitializing: false };
  return state as unknown as typeof slice;
}

it('publishes a new song snapshot when liking without mutating existing subscribers', async () => {
  const state = createTestLibraryState();
  const first = Object.freeze(songs(2)[0]);
  const second = songs(2)[1];
  const previous = [first, second];
  state.songs = previous;
  vi.spyOn(api, 'toggleLike').mockResolvedValue({ liked: true } as never);
  try {
    await state.toggleLikeSong(first.id);
    expect(state.likedSongIds.has(first.id)).toBe(true);
    expect(state.songs).not.toBe(previous);
    expect(state.songs[0]).not.toBe(first);
    expect(state.songs[0].liked).toBe(true);
    expect(first.liked).toBeUndefined();
    expect(state.songs[1]).toBe(second);
  } finally { vi.restoreAllMocks(); }
});

describe('library scan polling', () => {
  afterEach(() => {
    vi.useRealTimers();
    vi.restoreAllMocks();
  });

  it('uses one snapshot and catches up via deltas while scanning', async () => {
    vi.useFakeTimers();
    let statusReads = 0;
    vi.spyOn(backendService, 'getScanStatus').mockImplementation(async () => ({
      scanning: statusReads++ < 4,
      progress: 'Indexing local music',
    }));
    const snapshot = vi.spyOn(libraryV2, "getSnapshot").mockResolvedValue({ songs: songs(50), revision: 1 });
    const oldRead = vi.spyOn(backendService, "getAllSongs");
    const catchup = vi.spyOn(libraryV2, "getChanges");
    const batch = (from: number, end: number, revision: number) => ({
      revision, songs: songs(end).slice(from),
      changes: songs(end).slice(from).map(song => ({ songId: song.id, revision, operation: "upsert" as const, changedAt: 1 })),
    });
    catchup.mockResolvedValueOnce(batch(50, 100, 2)).mockResolvedValueOnce(batch(100, 200, 3));
    const state = createTestLibraryState();
    await state.pollScanStatus();
    await vi.advanceTimersByTimeAsync(0);
    expect(state.songs).toHaveLength(50);
    await vi.advanceTimersByTimeAsync(3100);
    expect(state.songs).toHaveLength(100);
    await vi.advanceTimersByTimeAsync(1000);
    expect(state.songs).toHaveLength(200);
    expect(state.isScanning).toBe(false);
    expect(snapshot).toHaveBeenCalledTimes(1);
    expect(oldRead).not.toHaveBeenCalled();
    expect(catchup.mock.calls.map(([revision]) => revision)).toEqual([1, 2]);
  });
  it('keeps scanning visible until the final catalog response arrives', async () => {
    vi.useFakeTimers();
    vi.spyOn(backendService, 'getScanStatus')
      .mockResolvedValueOnce({ scanning: true, progress: 'Scanning' })
      .mockResolvedValue({ scanning: false, progress: '' });
    vi.spyOn(libraryV2, 'getSnapshot').mockResolvedValue({ songs: [], revision: 1 });
    let finish!: (value: Awaited<ReturnType<typeof libraryV2.getChanges>>) => void;
    vi.spyOn(libraryV2, 'getChanges').mockImplementationOnce(() => new Promise(resolve => { finish = resolve; }));
    vi.spyOn(backendService, 'getFolders').mockResolvedValue([]);
    const state = createTestLibraryState();
    await state.pollScanStatus();
    await vi.advanceTimersByTimeAsync(1000);
    expect(state.songs).toHaveLength(0);
    expect(state.isScanning).toBe(true);
    finish({ revision: 2, songs: songs(1), changes: [{ revision: 2, songId: "song-0", operation: "upsert", changedAt: 1 }] });
    await vi.advanceTimersByTimeAsync(0);
    expect(state.songs).toHaveLength(1);
    expect(state.isScanning).toBe(false);
  });

});

describe('library persistence failures', () => {
 afterEach(() => vi.restoreAllMocks());
 it('propagates metadata rejection without committing or replacing displayed metadata', async () => {
  const state = createTestLibraryState() as any;
  const song = songs(1)[0]; Object.assign(state, { songs: [song], queue:[song], currentSong:song, songInfoModalSong:song });
  vi.spyOn(libraryOperationsV2,'updateSongMetadata').mockRejectedValue(new Error('write failed'));
  await expect(state.updateSongMetadata(song.id,{title:'Changed'})).rejects.toThrow('write failed');
  expect(state.songs[0].title).toBe(song.title); expect(state.songInfoModalSong).toBe(song);
  await expect(state.updateSongMetadata('unknown',{title:'Changed'})).rejects.toThrow('Unknown song');
 });
 it('awaits browser metadata writes and refreshes the displayed song after success', async () => {
  const state=createTestLibraryState() as any; const song=songs(1)[0];
  Object.assign(state,{backendAvailable:false,songs:[song],queue:[song],currentSong:song,songInfoModalSong:song});
  let finish!:()=>void; vi.spyOn(libraryService,'saveSongs').mockImplementation(()=>new Promise(resolve=>{finish=resolve}));
  const pending=state.updateSongMetadata(song.id,{title:'Changed'});
  expect(state.songs[0].title).toBe(song.title); finish(); await pending;
  expect(state.songs[0].title).toBe('Changed'); expect(state.songInfoModalSong.title).toBe('Changed');
 });
 it('publishes only selected album fields after one successful batch', async () => {
  const state = createTestLibraryState() as any;
  const [first, second, unrelated] = songs(3);
  Object.assign(state, { songs: [first, second, unrelated], queue: [first, unrelated], currentSong: second, songInfoModalSong: first });
  const persist = vi.spyOn(libraryOperationsV2, 'updateAlbumMetadata');
  persist.mockRejectedValueOnce(new Error('write failed'));
  const ids = [first.id, second.id];
  await expect(state.updateAlbumMetadata(ids, { album: 'New album' })).rejects.toThrow('write failed');
  expect(state.songs).toEqual([first, second, unrelated]);
  persist.mockResolvedValueOnce([first, second].map(song => ({ ...song, album: 'New album', title: 'Do not overwrite' })));
  await expect(state.updateAlbumMetadata(ids, { album: 'New album' })).resolves.toEqual({ album: 'New album', artist: first.artist });
  expect(persist).toHaveBeenLastCalledWith(ids, { album: 'New album' });
  expect(state.songs).toEqual([{ ...first, album: 'New album' }, { ...second, album: 'New album' }, unrelated]);
  expect(state.currentSong.album).toBe('New album');
  expect(state.songInfoModalSong.album).toBe('New album');
  expect(state.queue[1]).toBe(unrelated);
 });
 it('never creates a local phantom playlist after server creation fails', async () => {
  const state=createTestLibraryState() as any;
  vi.spyOn(backendService,'createPlaylist').mockRejectedValue(new Error('server unavailable'));
  const local=vi.spyOn(libraryService,'savePlaylist');
  await expect(state.createPlaylist('New')).rejects.toThrow('server unavailable');
  expect(state.playlists).toEqual([]);expect(local).not.toHaveBeenCalled();
  state.smartMixes=[{id:'mix',name:'Mix',songIds:[]}];
  await expect(state.saveSmartMixAsPlaylist('mix')).rejects.toThrow('server unavailable');
  expect(local).not.toHaveBeenCalled();
 });
});


describe('playlist content editing', () => {
  afterEach(() => vi.restoreAllMocks());
  it.each([true, false])('persists ordered occurrences and artwork before committing (backend=%s)', async backendAvailable => {
    const state = createTestLibraryState() as any;
    const original = { id: 'playlist', name: 'Mix', songIds: ['a', 'a', 'b'], coverUrl: '/cover.jpg', createdAt: 1 };
    Object.assign(state, { backendAvailable, playlists: [original] });
    let finish!: () => void;
    const write = backendAvailable ? vi.spyOn(backendService, 'updatePlaylist') : vi.spyOn(libraryService, 'savePlaylist');
    write.mockImplementation(() => new Promise(resolve => { finish = resolve; }));
    const ids = ['b', 'a', 'a'];
    const pending = state.updatePlaylistContents('playlist', ids, original.songIds);
    expect(state.playlists[0]).toBe(original);
    expect(write).toHaveBeenCalledWith({ ...original, songIds: ids });
    finish(); await pending;
    expect(state.playlists[0].songIds).toEqual(ids);
    ids.push('c');
    expect(state.playlists[0].songIds).toEqual(['b', 'a', 'a']);
    expect(state.playlists[0].coverUrl).toBe('/cover.jpg');
  });
  it.each([true, false])('retains saved contents after a write failure (backend=%s)', async backendAvailable => {
    const state = createTestLibraryState() as any;
    const original = { id: 'playlist', name: 'Mix', songIds: ['a', 'a', 'b'], createdAt: 1 };
    Object.assign(state, { backendAvailable, playlists: [original] });
    const write = backendAvailable ? vi.spyOn(backendService, 'updatePlaylist') : vi.spyOn(libraryService, 'savePlaylist');
    write.mockRejectedValue(new Error('Save failed'));
    await expect(state.updatePlaylistContents('playlist', ['b'])).rejects.toThrow('Save failed');
    expect(state.playlists[0]).toBe(original);
  });
  it('rejects stale drafts and missing playlists without writing', async () => {
    const state = createTestLibraryState() as any;
    state.playlists = [{ id: 'playlist', songIds: ['a', 'new'] }];
    const write = vi.spyOn(backendService, 'updatePlaylist');
    await expect(state.updatePlaylistContents('playlist', ['b'], ['a'])).rejects.toThrow('changed while you were editing');
    await expect(state.updatePlaylistContents('missing', ['b'])).rejects.toThrow('no longer exists');
    expect(write).not.toHaveBeenCalled();
  });
});
