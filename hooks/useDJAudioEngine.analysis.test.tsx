// @vitest-environment jsdom
/** Tests DJ deck load recovery and rejection of stale audio and analysis results. */

import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { Song } from '../types';
const mocks = vi.hoisted(() => ({
 state: {} as any, generation: 0, loadedId: null as string | null,
 feature: vi.fn(), grid: vi.fn(), waveform: vi.fn(), hotCues: vi.fn(), clientWaveform: vi.fn(),
 patch: vi.fn(), status: vi.fn(), audioLoad: vi.fn(),
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
 getDJWaveform: mocks.waveform, getDJHotCues: mocks.hotCues,
} }));
vi.mock('../lib/clientWaveform', () => ({ generateClientWaveform: mocks.clientWaveform }));
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
 mocks.waveform.mockReset().mockResolvedValue({peaks:[0]});
 mocks.hotCues.mockReset().mockResolvedValue({hotCues:[]});
 mocks.clientWaveform.mockReset().mockResolvedValue([0]);
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
  it('marks deck analysisStatus as unsupported and skips beatgrid fetch when feature.status is unsupported without overrides',async()=>{
   mocks.feature.mockResolvedValueOnce({songId:'song',status:'unsupported',hasManualBeatGrid:false,bpmSource:'unknown',keySource:'unknown',syncAllowed:false,sourceFingerprint:'fp'});
   await mounted(async audio=>{await audio.loadTrack('A',song);});
   expect(mocks.grid).not.toHaveBeenCalled();
   expect(mocks.status).toHaveBeenLastCalledWith('A','unsupported');
  });
  it('loads a saved manual grid on an unsupported format',async()=>{
   mocks.feature.mockResolvedValueOnce({songId:'song',status:'unsupported',hasManualBeatGrid:true,bpmSource:'unknown',keySource:'unknown',syncAllowed:false,sourceFingerprint:'fp'});
   mocks.grid.mockResolvedValueOnce({songId:'song',resolution:'available',source:'manual',provenance:'manual',beats:[0,0.5,1],downbeatIndices:[0],locked:true,sourceFingerprint:'fp'});
   await mounted(async audio=>{await audio.loadTrack('A',song);});
   expect(mocks.grid).toHaveBeenCalledWith('song',expect.any(AbortSignal));
   expect(mocks.patch).toHaveBeenCalledWith('A',expect.objectContaining({beatGrid:[0,0.5,1],beatGridSource:'manual',beatGridLocked:true}));
   expect(mocks.status).toHaveBeenLastCalledWith('A','available');
  });
  it('marks deck analysisStatus as available when feature.status is unsupported but a manual or Spotify BPM exists',async()=>{
   mocks.feature.mockResolvedValueOnce({songId:'song',status:'unsupported',hasManualBeatGrid:false,bpm:126,bpmSource:'manual',keySource:'unknown',syncAllowed:true,sourceFingerprint:'fp'});
   await mounted(async audio=>{await audio.loadTrack('A',song);});
   expect(mocks.grid).not.toHaveBeenCalled();
   expect(mocks.status).toHaveBeenLastCalledWith('A','available');
  });
  it('aborts superseded deck requests and passes signals to waveform, client fallback, analysis, grid, and hot cues',async()=>{
   const signals: Record<string, AbortSignal|undefined>={};
   const pendingClient=deferred<number[]>();
   const pendingHotCues=deferred<{hotCues:[]}>();
   mocks.waveform.mockImplementationOnce(async (_id:string,s?:AbortSignal)=>{signals.waveform=s;throw new Error('no server waveform');});
   mocks.clientWaveform.mockImplementationOnce(async (_u:string,_p:unknown,s?:AbortSignal)=>{signals.client=s;return pendingClient.promise;});
   mocks.feature.mockImplementationOnce(async (_id:string,s?:AbortSignal)=>{signals.feature=s;return feature;});
   mocks.grid.mockImplementationOnce(async (_id:string,s?:AbortSignal)=>{signals.grid=s;return null;});
   mocks.hotCues.mockImplementationOnce(async (_id:string,s?:AbortSignal)=>{signals.hotCues=s;return pendingHotCues.promise;});
   const deckBSignals: AbortSignal[]=[];
   mocks.feature.mockImplementationOnce(async (_id:string,s?:AbortSignal)=>{if(s)deckBSignals.push(s);return feature;});
   await mounted(async audio=>{
    const first=audio.loadTrack('A',song);
    await vi.advanceTimersByTimeAsync(0);
    expect(signals.waveform).toBeInstanceOf(AbortSignal);
    expect(signals.client).toBe(signals.waveform);
    expect(signals.feature).toBe(signals.waveform);
    expect(signals.grid).toBe(signals.waveform);
    expect(signals.hotCues).toBe(signals.waveform);
    expect(signals.waveform!.aborted).toBe(false);
    await audio.loadTrack('B',{...song,id:'song-b'});
    expect(signals.waveform!.aborted).toBe(false);
    expect(deckBSignals[0]?.aborted).toBe(false);
    await audio.loadTrack('A',{...song,id:'song-2'});
    expect(signals.waveform!.aborted).toBe(true);
    expect(deckBSignals[0]?.aborted).toBe(false);
    await first;
   });
   expect(mocks.status).not.toHaveBeenCalledWith('A','error');
  });
  it('aborts in-flight deck requests on unmount and ignores AbortError cleanly',async()=>{
   let captured: AbortSignal|undefined;
   const pendingFeature=deferred<typeof feature>();
   const pendingHotCues=deferred<{hotCues:[]}>();
   mocks.feature.mockImplementationOnce(async (_id:string,s?:AbortSignal)=>{captured=s;return pendingFeature.promise;});
   mocks.hotCues.mockImplementationOnce(async ()=>pendingHotCues.promise);
   const root=createRoot(document.createElement('div'));
   let audio!:ReturnType<typeof useAudio>;
   function Harness(){audio=useAudio();return null;}
   await act(async()=>root.render(<Harness/>));
   const load=audio.loadTrack('A',song);
   await vi.advanceTimersByTimeAsync(0);
   expect(captured?.aborted).toBe(false);
   await act(async()=>root.unmount());
   expect(captured?.aborted).toBe(true);
   pendingFeature.reject(new DOMException('aborted','AbortError'));
   pendingHotCues.reject(new DOMException('aborted','AbortError'));
   await load;
   expect(mocks.status).not.toHaveBeenCalledWith('A','error');
   expect(mocks.state.loadHotCues).not.toHaveBeenCalled();
  });
 });
}
