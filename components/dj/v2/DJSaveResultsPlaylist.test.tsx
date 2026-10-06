// @vitest-environment jsdom
import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { expect,it,vi } from 'vitest';
const mocks=vi.hoisted(()=>({ create:vi.fn(), generation:0 }));
vi.mock('../../../store',()=>{
 const useStore=(select:any)=>select({createPlaylist:mocks.create,spotifySessionGeneration:mocks.generation});
 Object.assign(useStore,{getState:()=>({spotifySessionGeneration:mocks.generation})});
 return {useStore};
});
import { DJSaveResultsPlaylist } from './DJSaveResultsPlaylist';
it('captures order and retains the draft through failed saves',async()=>{
 (globalThis as any).IS_REACT_ACT_ENVIRONMENT=true;
 mocks.generation=0;
 const host=document.createElement('div'),root=createRoot(host);
 mocks.create.mockRejectedValueOnce(new Error('offline')).mockResolvedValueOnce({id:'playlist'});
 try {
  await act(async()=>root.render(<DJSaveResultsPlaylist songIds={['b','a']} />));
  await act(async()=>host.querySelector('button')!.click());
  await act(async()=>root.render(<DJSaveResultsPlaylist songIds={['changed']} />));
  const input=host.querySelector('input')!;
  await act(async()=>{Object.getOwnPropertyDescriptor(HTMLInputElement.prototype,'value')!.set!.call(input,' Set '); input.dispatchEvent(new Event('input',{bubbles:true}));});
  await act(async()=>host.querySelector('form')!.dispatchEvent(new Event('submit',{bubbles:true,cancelable:true})));
  expect(mocks.create).toHaveBeenLastCalledWith('Set',['b','a']);
  expect(host.querySelector('[role="alert"]')).not.toBeNull();
  expect(input.value).toBe(' Set ');
  await act(async()=>host.querySelector('form')!.dispatchEvent(new Event('submit',{bubbles:true,cancelable:true})));
  expect(host.textContent).toContain('Saved 2 tracks');
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
