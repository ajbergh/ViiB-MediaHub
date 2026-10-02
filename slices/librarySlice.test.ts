import { afterEach, describe, expect, it, vi } from 'vitest';
import { createLibrarySlice } from './librarySlice';
import { backendService } from '../services/backendService';
import { libraryIndex } from '../lib/libraryIndex';
import { Song } from '../types';
import { libraryOperationsV2 } from '../services/libraryOperationsV2';
import { libraryService } from '../services/libraryService';

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
  it('keeps scanning visible until the final catalog response arrives', async () => {
    vi.useFakeTimers();
    vi.spyOn(backendService, 'getScanStatus')
      .mockResolvedValueOnce({ scanning: true, progress: 'Scanning' })
      .mockResolvedValue({ scanning: false, progress: '' });
    let finish!: (value: Song[]) => void;
    vi.spyOn(backendService, 'getAllSongs')
      .mockResolvedValueOnce([])
      .mockImplementationOnce(() => new Promise(resolve => { finish = resolve; }));
    vi.spyOn(backendService, 'getFolders').mockResolvedValue([]);
    const state = createTestLibraryState();
    await state.pollScanStatus();
    await vi.advanceTimersByTimeAsync(1000);
    expect(state.songs).toHaveLength(0);
    expect(state.isScanning).toBe(true);
    finish(songs(1));
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
