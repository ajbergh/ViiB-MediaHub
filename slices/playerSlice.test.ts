/** Tests playback, queue navigation, source resolution, and Spotify session isolation. */

import { resetEventStreamURLCacheForTests } from '../services/eventStreamURL';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { createPlayerSlice } from './playerSlice';
import { createSpotifySlice } from './spotifySlice';
import { libraryService } from '../services/libraryService';
import { managedObjectUrls } from '../lib/playbackLifecycle';
import type { Song } from '../types';
vi.mock('../store',()=>({useStore:{getState:()=>({})}}));
const song=(id:string):Song=>({id,title:id,artist:'Artist',album:'Album',duration:120,url:'/audio/'+id,addedAt:1});
function setup() {
 let state:any={};const set=(update:any)=>Object.assign(state,typeof update==='function'?update(state):update);
 state={...createPlayerSlice(set as never,(()=>state) as never,{} as never),...createSpotifySlice(set as never,(()=>state) as never,{} as never),showToast:vi.fn(),recordStreamEvent:vi.fn()};return state;
}
afterEach(()=>{vi.useRealTimers();vi.restoreAllMocks();managedObjectUrls.releaseAll()});
describe('playback ownership',()=>{
 it('preserves duplicate occurrence through next, previous and direct selection',async()=>{
  const state=setup(),a=song('a'),b=song('b'),queue=[a,b,a];
  await state.playSong(a,queue);state.nextSong();await Promise.resolve();expect(state.currentSongIndex).toBe(1);
  state.nextSong();await Promise.resolve();expect(state.currentSongIndex).toBe(2);
  state.prevSong();await Promise.resolve();expect(state.currentSongIndex).toBe(1);
  state.playQueueItem(2);await Promise.resolve();expect(state.currentSongIndex).toBe(2);
  const pending=state.retryStream(); await pending;expect(state.currentSongIndex).toBe(2);
 });
 it('cancels a delayed retry after a newer selection or pause',async()=>{
  vi.useFakeTimers();const state=setup();await state.playSong(song('a'));const pending=state.retryStream();
  await state.playSong(song('b'));await vi.advanceTimersByTimeAsync(1000);await pending;expect(state.currentSong.id).toBe('b');
  const pausedRetry=state.retryStream();state.togglePlay();await vi.advanceTimersByTimeAsync(2000);await pausedRetry;expect(state.isPlaying).toBe(false);
 });
 it('regenerates persisted and revoked blobs and does not revoke a reused active URL',async()=>{
  vi.useFakeTimers();let counter=0;vi.spyOn(URL,'createObjectURL').mockImplementation(()=>`blob:owned-${++counter}`);
  const revoke=vi.spyOn(URL,'revokeObjectURL').mockImplementation(()=>{});
  vi.spyOn(libraryService,'verifyPermission').mockResolvedValue(true);
  const getFile=vi.fn().mockResolvedValue(new Blob(['audio']));const a={...song('a'),url:'blob:dead',fileHandle:{getFile} as never};const state=setup();
  await state.playSong(a);const live=state.currentSong;expect(live.url).toBe('blob:owned-1');
  await state.playSong(song('b'));await state.playSong(live);await vi.advanceTimersByTimeAsync(1100);expect(revoke).not.toHaveBeenCalledWith(live.url);
  managedObjectUrls.release(live.url);await state.playSong(live);expect(state.currentSong.url).toBe('blob:owned-2');
 });
});

describe('Spotify playback retirement', () => {
 const stream = (id: string): Song => ({...song(id), spotifyId: id, isStreaming: true});
 for (const action of ['logoutSpotify', 'markSpotifyAuthRequired']) {
  it(action + ' stops streaming, removes remote queue entries and cancels retries', async () => {
   vi.useFakeTimers();
   const state = setup(), remote = stream('remote'), local = song('local');
   state.currentSong = remote;
   state.currentSongIndex = 1;
   state.queue = [local, remote, {...stream('pending'), url: ''}];
   state.isPlaying = true;
   state.isBuffering = true;
   state.preloadedTrackId = 'pending';
   const generation = state.playbackGeneration;
   const retry = state.retryStream();
   state[action]();
   expect(state.currentSong).toBeNull();
   expect(state.currentSongIndex).toBe(-1);
   expect(state.isPlaying).toBe(false);
   expect(state.isBuffering).toBe(false);
   expect(state.queue).toEqual([local]);
   expect(state.preloadedTrackId).toBeNull();
   expect(state.playbackGeneration).toBeGreaterThan(generation);
   await vi.runAllTimersAsync();
   await retry;
   expect(state.currentSong).toBeNull();
   expect(state.isPlaying).toBe(false);
  });
 }
 it('keeps downloaded Spotify and Plex playback and duplicate queue positions', () => {
  const state = setup(), downloaded = {...song('local'), spotifyId: 'saved'};
  const plex = {...song('plex'), source: 'plex' as const};
  state.currentSong = downloaded;
  state.queue = [stream('remote'), downloaded, plex, downloaded];
  state.currentSongIndex = 3;
  state.isPlaying = true;
  state.logoutSpotify();
  expect(state.currentSong).toBe(downloaded);
  expect(state.isPlaying).toBe(true);
  expect(state.queue).toEqual([downloaded, plex, downloaded]);
  expect(state.currentSongIndex).toBe(2);
 });
 it('releases a hidden Spotify preload on logout without committing stale state', async () => {
  class PreloadAudio extends EventTarget {
   src = '';
   preload = '';
   removeAttribute = vi.fn(() => {this.src = '';});
   load = vi.fn();
  }
  const audio = new PreloadAudio();
  vi.stubGlobal('Audio', vi.fn(function () {return audio;}));
  try {
   const state = setup();
   state.queue = [song('local'), stream('remote')];
   state.currentSong = state.queue[0];
   state.currentSongIndex = 0;
   state.isPlaying = true;
   const pending = state.preloadNextTrack();
   await vi.waitFor(() => expect(audio.load).toHaveBeenCalled());
   state.logoutSpotify();
   await pending;
   expect(audio.src).toBe('');
   expect(audio.removeAttribute).toHaveBeenCalledWith('src');
   expect(state.preloadedTrackId).toBeNull();
   expect(state.currentSong.id).toBe('local');
   expect(state.isPlaying).toBe(true);
  } finally {
   vi.unstubAllGlobals();
  }
 });
 it('prevents an in-flight Spotify selection from committing after disconnect', async () => {
  const state = setup();
  let resolve!: (value: null) => void;
  vi.spyOn(libraryService, 'getSongBySpotifyId').mockImplementation(() =>
   new Promise(resolveLookup => {resolve = resolveLookup as typeof resolve;}));
  const pending = state.playSong({...stream('remote'), url: ''});
  state.logoutSpotify();
  resolve(null);
  await pending;
  expect(state.currentSong).toBeNull();
  expect(state.queue).toEqual([]);
  expect(state.isPlaying).toBe(false);
 });
});


describe('native Spotify audio transport', () => {
 afterEach(() => { resetEventStreamURLCacheForTests(); vi.unstubAllGlobals(); });
 it('selects the direct localhost audio URL instead of the buffered Wails asset path', async () => {
  vi.stubGlobal('window', {go:{main:{App:{GetServerURL:vi.fn().mockResolvedValue('http://127.0.0.1:34115')}}}});
  const state=setup(); state.preferLocalPlayback=false;
  await state.playSong({...song('remote'),spotifyId:'remote',isStreaming:true});
  expect(state.currentSong.url).toBe('http://127.0.0.1:34115/api/spotify/stream/remote?quality=high');
 });
 it('cannot activate a native URL that resolves after account retirement', async () => {
  let release!:(value:string)=>void;
  vi.stubGlobal('window', {go:{main:{App:{GetServerURL:vi.fn().mockReturnValue(new Promise<string>(resolve=>{release=resolve;}))}}}});
  const state=setup(); state.preferLocalPlayback=false;
  const pending=state.playSong({...song('remote'),spotifyId:'remote',isStreaming:true});
  await vi.waitFor(()=>expect(release).toBeDefined());
  state.logoutSpotify(); release('http://127.0.0.1:34115'); await pending;
  expect(state.currentSong).toBeNull();
  expect(state.isPlaying).toBe(false);
 });
});
