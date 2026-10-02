import { afterEach, describe, expect, it, vi } from 'vitest';
import { createPlayerSlice } from './playerSlice';
import { libraryService } from '../services/libraryService';
import { managedObjectUrls } from '../lib/playbackLifecycle';
import type { Song } from '../types';
vi.mock('../store',()=>({useStore:{getState:()=>({})}}));
const song=(id:string):Song=>({id,title:id,artist:'Artist',album:'Album',duration:120,url:'/audio/'+id,addedAt:1});
function setup() {
 let state:any={};const set=(update:any)=>Object.assign(state,typeof update==='function'?update(state):update);
 state={...createPlayerSlice(set as never,(()=>state) as never,{} as never),showToast:vi.fn(),recordStreamEvent:vi.fn()};return state;
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
