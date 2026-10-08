// @vitest-environment jsdom
import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { beforeEach, expect, it, vi } from 'vitest';
const mocks = vi.hoisted(() => ({reset:vi.fn(),update:vi.fn(),patch:vi.fn(),deck:{track:{id:'song'},beatGrid:null,downbeatIndices:null,beatGridLocked:true,beatGridSourceFingerprint:'source-v1',originalBpm:120} as any}));
vi.mock('../../../services/api', () => ({api:{resetTrackBeatGrid:mocks.reset,updateTrackBeatGrid:mocks.update}}));
vi.mock('../../../store', () => {const state=()=>({djDeckA:mocks.deck,setDeckAnalysis:mocks.patch});const useStore=(select:any)=>select(state());Object.assign(useStore,{getState:state});return {useStore};});
import { DJBeatGridEdit } from './DJBeatGridEdit';
beforeEach(()=>{(globalThis as any).IS_REACT_ACT_ENVIRONMENT=true;mocks.reset.mockReset();mocks.update.mockReset();mocks.patch.mockReset();mocks.deck={track:{id:'song'},beatGrid:null,downbeatIndices:null,beatGridLocked:true,beatGridSourceFingerprint:'source-v1',originalBpm:120};});
it('offers source-bound reset for a locked unresolved grid',async()=>{
 const host=document.createElement('div'),root=createRoot(host);mocks.reset.mockResolvedValue(undefined);
 try{await act(async()=>root.render(<DJBeatGridEdit deck='A'/>));expect(host.textContent).toContain('locked but unavailable');await act(async()=>host.querySelector('button')!.click());expect(mocks.reset).toHaveBeenCalledWith('song','source-v1');expect(mocks.patch).toHaveBeenCalledWith('A',expect.objectContaining({beatGrid:null,beatGridLocked:false}));}finally{await act(async()=>root.unmount());}
});
it('ignores a late reset after the loaded source changes',async()=>{
 let complete!:()=>void;mocks.reset.mockImplementation(()=>new Promise<void>(resolve=>{complete=resolve}));
 const host=document.createElement('div'),root=createRoot(host);
 try{await act(async()=>root.render(<DJBeatGridEdit deck='A'/>));await act(async()=>host.querySelector('button')!.click());mocks.deck={...mocks.deck,beatGridSourceFingerprint:'replacement'};await act(async()=>complete());expect(mocks.patch).not.toHaveBeenCalled();}finally{await act(async()=>root.unmount());}
});
