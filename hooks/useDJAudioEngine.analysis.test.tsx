// @vitest-environment jsdom
import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { Song } from '../types';
const mocks = vi.hoisted(() => ({
 state: {} as any, generation: 0, loadedId: null as string | null,
 feature: vi.fn(), grid: vi.fn(), patch: vi.fn(), status: vi.fn(), audioLoad: vi.fn(),
 engine: {} as any,
}));
vi.mock('../store', () => {
 const useStore = (select: any) => select(mocks.state);
 Object.assign(useStore, { getState: () => mocks.state, subscribe: () => () => {} });
 return { useStore };
});
vi.mock('../lib/djAudio', () => ({ getDJAudioEngine: () => mocks.engine, disposeDJAudioEngine: vi.fn() }));
vi.mock('../services/api', () => ({ api: {
 getTrackAnalysisFeature: mocks.feature, getTrackBeatGrid: mocks.grid,
 getDJWaveform: vi.fn(async () => ({peaks:[0]})), getDJHotCues: vi.fn(async () => ({hotCues:[]})),
} }));
vi.mock('../lib/clientWaveform', () => ({ generateClientWaveform: vi.fn() }));
vi.mock('../services/loggerService', () => ({ createLogger: () => ({debug:vi.fn(), info:vi.fn(), warn:vi.fn(), logError:vi.fn()}) }));
import { useDJAudioEngine, useDJAudioEngineActions } from './useDJAudioEngine';
const song = {id:'song',title:'Track',duration:60} as Song;
const feature = {songId:'song',bpm:128,sourceFingerprint:'fp'};
function deferred<T>() { let resolve!: (value:T)=>void; let reject!: (error:unknown)=>void;
 const promise=new Promise<T>((yes,no)=>{resolve=yes;reject=no;});return {promise,resolve,reject}; }
beforeEach(() => {
 (globalThis as any).IS_REACT_ACT_ENVIRONMENT=true;
 vi.useFakeTimers(); mocks.generation=0; mocks.loadedId=null;
 mocks.feature.mockReset().mockResolvedValue(feature); mocks.grid.mockReset().mockResolvedValue(null);
 mocks.patch.mockReset();mocks.status.mockReset();mocks.audioLoad.mockReset();
 const deck=()=>({track:null,duration:0,volume:1,eq:{low:0,mid:0,high:0},tempo:0,cueEnabled:false});
 mocks.state={spotifySessionGeneration:0,djDeckA:deck(),djDeckB:deck(),djMixer:{beatFX:{target:'A'}},
  setDeckDuration:vi.fn(),setDeckAnalysis:mocks.patch,setDeckAnalysisStatus:mocks.status,setDeckWaveform:vi.fn(),loadHotCues:vi.fn(),
  loadTrackToDeck:vi.fn((deck:string,track:Song)=>{mocks.state[deck==='A'?'djDeckA':'djDeckB'].track=track;}),
 };
 const engine={initialized:true,getDeckLoadGeneration:()=>mocks.generation,getDeckLoadedTrackId:()=>mocks.loadedId,
  unloadDeck:()=>{mocks.generation++;mocks.loadedId=null;},
  isLoaded:()=>mocks.loadedId!==null,loadTrack:async (_deck:string,track:Song)=>{
   const generation=++mocks.generation;mocks.loadedId=null;await mocks.audioLoad();
   if(generation===mocks.generation)mocks.loadedId=track.id;
  }};
 mocks.engine=new Proxy(engine,{get:(target,key)=>key in target ? (target as any)[key] : vi.fn()});
});
afterEach(()=>vi.useRealTimers());
for (const [name,useAudio] of [['full',useDJAudioEngine],['actions',useDJAudioEngineActions]] as const) {
 describe(name,()=>{
  async function mounted(run:(audio:ReturnType<typeof useAudio>)=>Promise<void>) {
   const root=createRoot(document.createElement('div'));let audio!:ReturnType<typeof useAudio>;
   function Harness(){audio=useAudio();return null;}
   try {await act(async()=>root.render(<Harness/>));await run(audio);}
   finally{await act(async()=>root.unmount());}
  }
  it.each([new TypeError('Failed to fetch'),Object.assign(new Error('temporary'),{status:503})])('recovers a transient read once',async(error)=>{
   mocks.feature.mockRejectedValueOnce(error);
   await mounted(async audio=>{const load=audio.loadTrack('A',song);await vi.advanceTimersByTimeAsync(250);await load;});
   expect(mocks.feature).toHaveBeenCalledTimes(2);expect(mocks.patch).toHaveBeenCalledWith('A',expect.objectContaining({bpm:128}));
   expect(mocks.status).toHaveBeenLastCalledWith('A','available');
  });
  it.each([404,401,412,429])('does not retry HTTP %i',async status=>{
   mocks.feature.mockRejectedValueOnce(Object.assign(new Error('HTTP'),{status}));
   await mounted(async audio=>{await audio.loadTrack('A',song);});
   expect(mocks.feature).toHaveBeenCalledTimes(1);expect(mocks.grid).not.toHaveBeenCalled();
   expect(mocks.status).toHaveBeenLastCalledWith('A',status===404?'not_analyzed':'error');
  });
  it.each([new DOMException('cancelled','AbortError'),new SyntaxError('invalid JSON')])('does not retry abort or malformed responses',async error=>{
   mocks.feature.mockRejectedValueOnce(error);await mounted(async audio=>{await audio.loadTrack('A',song);});
   expect(mocks.feature).toHaveBeenCalledTimes(1);expect(mocks.patch).not.toHaveBeenCalled();
  });
  it('stops after the second transient failure',async()=>{
   mocks.feature.mockRejectedValue(Object.assign(new Error('temporary'),{status:500}));
   await mounted(async audio=>{const load=audio.loadTrack('A',song);await vi.advanceTimersByTimeAsync(1000);await load;});
   expect(mocks.feature).toHaveBeenCalledTimes(2);expect(mocks.status).toHaveBeenLastCalledWith('A','error');
  });
  it.each(['session','reload'] as const)('cancels retry after %s changes',async change=>{
   mocks.feature.mockRejectedValueOnce(new TypeError('Failed to fetch'));
   await mounted(async audio=>{const load=audio.loadTrack('A',song);await vi.advanceTimersByTimeAsync(0);
    if(change==='session')mocks.state.spotifySessionGeneration++;else mocks.generation++;
    await vi.advanceTimersByTimeAsync(250);await load;});
   expect(mocks.feature).toHaveBeenCalledTimes(1);expect(mocks.patch).not.toHaveBeenCalled();expect(mocks.status).not.toHaveBeenCalled();
  });
  it('rejects a late response after the same song is loaded again',async()=>{
   const first=deferred<typeof feature>();mocks.feature.mockReturnValueOnce(first.promise);
   await mounted(async audio=>{const old=audio.loadTrack('A',song);await vi.advanceTimersByTimeAsync(0);
    await audio.loadTrack('A',song);first.resolve({...feature,bpm:90});await old;});
   expect(mocks.patch).toHaveBeenCalledTimes(1);expect(mocks.patch).toHaveBeenCalledWith('A',expect.objectContaining({bpm:128}));
   expect(mocks.grid).toHaveBeenCalledTimes(1);
  });
  it('rejects late grid results after session change',async()=>{
   const grid=deferred<null>();mocks.grid.mockReturnValueOnce(grid.promise);
   await mounted(async audio=>{const load=audio.loadTrack('A',song);await vi.advanceTimersByTimeAsync(0);
    mocks.state.spotifySessionGeneration++;grid.resolve(null);await load;});
   expect(mocks.patch).not.toHaveBeenCalled();expect(mocks.status).not.toHaveBeenCalled();
  });
  it('does not commit an audio load superseded by a same-song reload',async()=>{
   const first=deferred<void>();mocks.audioLoad.mockReturnValueOnce(first.promise);
   await mounted(async audio=>{const old=audio.loadTrack('A',song);await audio.loadTrack('A',song);first.resolve();await old;});
   expect(mocks.state.loadTrackToDeck).toHaveBeenCalledTimes(1);expect(mocks.feature).toHaveBeenCalledTimes(1);
  });
  it('discards its uncommitted audio after the session changes during loading',async()=>{
   const pending=deferred<void>();mocks.audioLoad.mockReturnValueOnce(pending.promise);
   await mounted(async audio=>{const load=audio.loadTrack('A',song);mocks.state.spotifySessionGeneration++;pending.resolve();await load;});
   expect(mocks.state.loadTrackToDeck).not.toHaveBeenCalled();expect(mocks.feature).not.toHaveBeenCalled();expect(mocks.loadedId).toBeNull();
  });
  it('does not load audio when the caller guard has expired',async()=>{
   await mounted(async audio=>{await audio.loadTrack('A',song,{expectedLoadGeneration:0,shouldCommit:()=>false});});
   expect(mocks.audioLoad).not.toHaveBeenCalled();expect(mocks.state.loadTrackToDeck).not.toHaveBeenCalled();
  });
  it('discards uncommitted audio when a caller guard expires during loading',async()=>{
   const pending=deferred<void>();mocks.audioLoad.mockReturnValueOnce(pending.promise);let owned=true;
   await mounted(async audio=>{const load=audio.loadTrack('A',song,{expectedLoadGeneration:0,shouldCommit:()=>owned});owned=false;pending.resolve();await load;});
   expect(mocks.state.loadTrackToDeck).not.toHaveBeenCalled();expect(mocks.feature).not.toHaveBeenCalled();expect(mocks.loadedId).toBeNull();
  });
  it('cleans up a failed guarded audio load without starting metadata reads',async()=>{
   mocks.audioLoad.mockRejectedValueOnce(new Error('audio failed'));
   await mounted(async audio=>{await expect(audio.loadTrack('A',song,{expectedLoadGeneration:0,shouldCommit:()=>true})).rejects.toThrow('audio failed');});
   expect(mocks.state.loadTrackToDeck).not.toHaveBeenCalled();expect(mocks.feature).not.toHaveBeenCalled();expect(mocks.generation).toBe(2);
  });
  it('uses a precommit empty-deck guard without rejecting committed metadata',async()=>{
   await mounted(async audio=>{await audio.loadTrack('A',song,{expectedLoadGeneration:0,shouldCommit:()=>mocks.state.djDeckA.track===null});});
   expect(mocks.patch).toHaveBeenCalledTimes(1);expect(mocks.status).toHaveBeenLastCalledWith('A','available');
  });
 });
}
