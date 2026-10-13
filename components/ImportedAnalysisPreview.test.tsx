// @vitest-environment jsdom
import React,{act} from 'react';
import {createRoot} from 'react-dom/client';
import {expect,it,vi} from 'vitest';
const mocks=vi.hoisted(()=>({load:vi.fn()}));
vi.mock('../services/importedAnalysis',()=>({loadImportedAnalysis:mocks.load}));
import {ImportedAnalysisPreview} from './ImportedAnalysisPreview';
it('loads on demand, pages, preserves zero and discards prior-source results',async()=>{
 (globalThis as any).IS_REACT_ACT_ENVIRONMENT=true;
 const page={songId:'song',sourceFingerprint:'fp',recordingId:'recording',kind:'beats',provenance:'spotify_private_cache',unverified:true,retrievedAt:'2026-10-01T00:00:00Z',stale:true,offset:0,limit:25,totalItems:26,items:Array.from({length:25},(_,i)=>({start:i,duration:1,confidence:0}))};
 mocks.load.mockResolvedValue(page);
 const host=document.createElement('div'),root=createRoot(host);
 try {
  await act(async()=>root.render(<ImportedAnalysisPreview songId="song" fingerprint="fp" />));
  expect(mocks.load).not.toHaveBeenCalled();
  await act(async()=>{const details=host.querySelector('details')!;details.open=true;details.dispatchEvent(new Event('toggle'));});
  expect(mocks.load).toHaveBeenCalledWith('song','beats',0);
  expect(host.querySelectorAll('tbody tr')).toHaveLength(25);
  expect(host.textContent).toContain('0%');expect(host.textContent).toContain('Stale observation');
  expect(host.textContent).toContain('Cached Spotify observation for the current recording.');
  expect(host.textContent).toContain('Account confirmation pending; viewing only.');
  mocks.load.mockResolvedValue({...page,kind:'segments',totalItems:1,items:[{start:0,duration:1,confidence:0,loudness_start:-20,pitches:Array(12).fill(0),timbre:Array(12).fill(-1)}]});
  await act(async()=>{const select=host.querySelector('select')!;select.value='segments';select.dispatchEvent(new Event('change',{bubbles:true}));});
  expect(host.textContent).toContain('Pitch classes C through B: 0.00');
  expect(host.textContent).toContain('Timbre coefficients 1-12: -1.00');
  expect(host.textContent).toContain('-20.00');
  const nested=host.querySelector('td details')! as HTMLDetailsElement;
  await act(async()=>{nested.open=true;nested.dispatchEvent(new Event('toggle',{bubbles:true}));});
  expect(host.querySelectorAll('tbody tr')).toHaveLength(1);
  mocks.load.mockResolvedValue(page);
  await act(async()=>{const select=host.querySelector('select')!;select.value='beats';select.dispatchEvent(new Event('change',{bubbles:true}));});
  let resolve!:(value:unknown)=>void;mocks.load.mockImplementationOnce(()=>new Promise(done=>{resolve=done;}));
  await act(async()=>Array.from(host.querySelectorAll('button')).find(button=>button.textContent==='Next')!.click());
  expect(mocks.load).toHaveBeenLastCalledWith('song','beats',25);
  expect(host.querySelectorAll('tbody tr')).toHaveLength(0);
  mocks.load.mockResolvedValue(null);
  await act(async()=>root.render(<ImportedAnalysisPreview songId="replacement" fingerprint="new" />));
  await act(async()=>resolve({...page,offset:25,items:[{start:999,duration:1,confidence:1}]}));
  expect(host.textContent).not.toContain('999');
  expect(host.textContent).toContain('not retained for the current file');
 }finally{await act(async()=>root.unmount());}
});
