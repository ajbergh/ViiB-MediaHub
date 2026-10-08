// @vitest-environment jsdom
import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { beforeEach, expect,it,vi } from 'vitest';
const mocks=vi.hoisted(()=>({ create:vi.fn(), getPlaylists:vi.fn(), generation:0 }));
vi.mock('../../../services/api',()=>({ api:{ getPlaylists:mocks.getPlaylists } }));
vi.mock('../../../store',()=>{
 const useStore=(select:any)=>select({createPlaylist:mocks.create,spotifySessionGeneration:mocks.generation});
 Object.assign(useStore,{getState:()=>({spotifySessionGeneration:mocks.generation})});
 return {useStore};
});
import { DJSaveResultsPlaylist } from './DJSaveResultsPlaylist';
beforeEach(()=>{mocks.create.mockReset();mocks.getPlaylists.mockReset();});
it('captures order and retains the draft through failed saves',async()=>{
 (globalThis as any).IS_REACT_ACT_ENVIRONMENT=true;
 mocks.generation=0;
 const host=document.createElement('div'),root=createRoot(host);
 mocks.create.mockRejectedValueOnce(new Error('offline')).mockResolvedValueOnce({id:'playlist'});
 mocks.getPlaylists.mockResolvedValue([{id:'playlist',name:'Set',songIds:['b','a'],createdAt:1}]);
 try {
  await act(async()=>root.render(<DJSaveResultsPlaylist songIds={['b','a']} />));
  await act(async()=>host.querySelector('button')!.click());
  await act(async()=>root.render(<DJSaveResultsPlaylist songIds={['changed']} />));
  const input=host.querySelector('input')!;
  await act(async()=>{Object.getOwnPropertyDescriptor(HTMLInputElement.prototype,'value')!.set!.call(input,' Set '); input.dispatchEvent(new Event('input',{bubbles:true}));});
  await act(async()=>host.querySelector('form')!.dispatchEvent(new Event('submit',{bubbles:true,cancelable:true})));
  expect(mocks.create).toHaveBeenLastCalledWith('Set',['b','a']);
  expect(host.querySelector('[role="alert"]')?.textContent).toContain('could not be saved or verified');
  expect(input.value).toBe(' Set ');
  await act(async()=>host.querySelector('form')!.dispatchEvent(new Event('submit',{bubbles:true,cancelable:true})));
  expect(host.textContent).toContain('Saved 2 tracks');
 } finally {await act(async()=>root.unmount());}
});

it('does not report success or clear the draft when the saved playlist order cannot be verified',async()=>{
 (globalThis as any).IS_REACT_ACT_ENVIRONMENT=true;
 mocks.generation=0;
 mocks.create.mockResolvedValue({id:'playlist'});
 mocks.getPlaylists.mockResolvedValue([{id:'playlist',name:'Set',songIds:['a','b'],createdAt:1}]);
 const host=document.createElement('div'),root=createRoot(host);
 try {
  await act(async()=>root.render(<DJSaveResultsPlaylist songIds={['b','a']} />));
  await act(async()=>host.querySelector('button')!.click());
  const input=host.querySelector('input')!;
  await act(async()=>{Object.getOwnPropertyDescriptor(HTMLInputElement.prototype,'value')!.set!.call(input,'Set');input.dispatchEvent(new Event('input',{bubbles:true}));});
  await act(async()=>host.querySelector('form')!.dispatchEvent(new Event('submit',{bubbles:true,cancelable:true})));
  expect(mocks.create).toHaveBeenCalledWith('Set',['b','a']);
  expect(mocks.getPlaylists).toHaveBeenCalledTimes(1);
  expect(host.querySelector('[role="alert"]')?.textContent).toContain('could not be verified');
  expect(host.querySelector('form')).not.toBeNull();
  expect(host.querySelector('input')?.value).toBe('Set');
  expect(host.textContent).not.toContain('Saved 2 tracks');
 } finally {await act(async()=>root.unmount());}
});

it('keeps the draft when server readback does not contain the created playlist',async()=>{
 (globalThis as any).IS_REACT_ACT_ENVIRONMENT=true;
 mocks.generation=0;
 mocks.create.mockResolvedValue({id:'missing-from-list'});
 mocks.getPlaylists.mockResolvedValue([]);
 const host=document.createElement('div'),root=createRoot(host);
 try {
  await act(async()=>root.render(<DJSaveResultsPlaylist songIds={['reference','candidate']} />));
  await act(async()=>host.querySelector('button')!.click());
  const input=host.querySelector('input')!;
  await act(async()=>{Object.getOwnPropertyDescriptor(HTMLInputElement.prototype,'value')!.set!.call(input,'Persistent');input.dispatchEvent(new Event('input',{bubbles:true}));});
  await act(async()=>host.querySelector('form')!.dispatchEvent(new Event('submit',{bubbles:true,cancelable:true})));
  expect(host.querySelector('[role="alert"]')?.textContent).toContain('could not be verified');
  expect(host.querySelector('form')).not.toBeNull();
  expect(host.querySelector('input')?.value).toBe('Persistent');
  expect(host.textContent).not.toContain('Saved 2 tracks');
 } finally {await act(async()=>root.unmount());}
});

it('clears an open draft when the Spotify account generation changes',async()=>{
 (globalThis as any).IS_REACT_ACT_ENVIRONMENT=true;
 mocks.generation=0;
 const host=document.createElement('div'),root=createRoot(host);
 try {
  await act(async()=>root.render(<DJSaveResultsPlaylist songIds={['reference','candidate']} />));
  await act(async()=>host.querySelector('button')!.click());
  expect(host.querySelector('form')).not.toBeNull();
  mocks.generation=1;
    await act(async()=>root.render(<DJSaveResultsPlaylist songIds={['reference','new-candidate']} />));
  expect(host.querySelector('form')).toBeNull();
  expect(host.textContent).toContain('Save 2 results as playlist');
  expect(host.querySelector('[role="status"]')).toBeNull();
 } finally {await act(async()=>root.unmount());}
});

it('ignores an old-account save completion after generation changes',async()=>{
 (globalThis as any).IS_REACT_ACT_ENVIRONMENT=true;
 mocks.generation=0;
 let complete!: (value:unknown)=>void;
 mocks.create.mockReset().mockImplementationOnce(()=>new Promise(resolve=>{complete=resolve;}));
 const host=document.createElement('div'),root=createRoot(host);
 try {
  await act(async()=>root.render(<DJSaveResultsPlaylist songIds={['reference','candidate']} />));
  await act(async()=>host.querySelector('button')!.click());
  const input=host.querySelector('input')!;
  await act(async()=>{Object.getOwnPropertyDescriptor(HTMLInputElement.prototype,'value')!.set!.call(input,'Old account mix'); input.dispatchEvent(new Event('input',{bubbles:true}));});
  await act(async()=>host.querySelector('form')!.dispatchEvent(new Event('submit',{bubbles:true,cancelable:true})));
  expect(mocks.create).toHaveBeenCalledWith('Old account mix',['reference','candidate']);
  mocks.generation=1;
  await act(async()=>root.render(<DJSaveResultsPlaylist songIds={['reference','new-candidate']} />));
  await act(async()=>complete({id:'old-account-playlist'}));
  expect(host.textContent).not.toContain('Saved 2 tracks');
  expect(host.textContent).not.toContain('Old account mix');
  expect(host.querySelector('form')).toBeNull();
 } finally {await act(async()=>root.unmount());}
});

it.each(['missing-id', 'readback-error', 'name-mismatch'])('retains the draft on %s without false success', async failure => {
 (globalThis as any).IS_REACT_ACT_ENVIRONMENT=true;
 mocks.generation=0;
 mocks.create.mockResolvedValue(failure==='missing-id'?undefined:{id:'playlist'});
 if(failure==='readback-error') mocks.getPlaylists.mockRejectedValue(new Error('offline'));
 else mocks.getPlaylists.mockResolvedValue([{id:'playlist',name:'Wrong name',songIds:['reference','candidate']}]);
 const host=document.createElement('div'),root=createRoot(host);
 try {
  await act(async()=>root.render(<DJSaveResultsPlaylist songIds={['reference','candidate']} />));
  await act(async()=>host.querySelector('button')!.click());
  const input=host.querySelector('input')!;
  await act(async()=>{Object.getOwnPropertyDescriptor(HTMLInputElement.prototype,'value')!.set!.call(input,' Set ');input.dispatchEvent(new Event('input',{bubbles:true}));});
  await act(async()=>host.querySelector('form')!.dispatchEvent(new Event('submit',{bubbles:true,cancelable:true})));
  expect(host.querySelector('[role="alert"]')).not.toBeNull();
  expect(host.querySelector('input')?.value).toBe(' Set ');
  expect(host.querySelector('[role="status"]')).toBeNull();
  expect(mocks.getPlaylists).toHaveBeenCalledTimes(failure==='missing-id'?0:1);
 } finally {await act(async()=>root.unmount());}
});

it('ignores readback from an old generation without clearing a new draft',async()=>{
 (globalThis as any).IS_REACT_ACT_ENVIRONMENT=true;
 mocks.generation=0;
 mocks.create.mockResolvedValue({id:'playlist'});
 let complete!: (value:unknown)=>void;
 mocks.getPlaylists.mockImplementationOnce(()=>new Promise(resolve=>{complete=resolve;}));
 const host=document.createElement('div'),root=createRoot(host);
 try {
  await act(async()=>root.render(<DJSaveResultsPlaylist songIds={['reference','candidate']} />));
  await act(async()=>host.querySelector('button')!.click());
  const input=host.querySelector('input')!;
  await act(async()=>{Object.getOwnPropertyDescriptor(HTMLInputElement.prototype,'value')!.set!.call(input,'Old mix');input.dispatchEvent(new Event('input',{bubbles:true}));});
  await act(async()=>host.querySelector('form')!.dispatchEvent(new Event('submit',{bubbles:true,cancelable:true})));
  expect(mocks.getPlaylists).toHaveBeenCalledTimes(1);
  mocks.generation=1;
  await act(async()=>root.render(<DJSaveResultsPlaylist songIds={['reference','new']} />));
  await act(async()=>host.querySelector('button')!.click());
  await act(async()=>complete([{id:'playlist',name:'Old mix',songIds:['reference','candidate']}]));
  expect(host.querySelector('form')).not.toBeNull();
  expect(host.querySelector('[role="status"]')).toBeNull();
  expect(host.querySelector('[role="alert"]')).toBeNull();
 } finally {await act(async()=>root.unmount());}
});
