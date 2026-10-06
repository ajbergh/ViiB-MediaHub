// @vitest-environment jsdom
import React,{act} from 'react';
import {createRoot} from 'react-dom/client';
import {expect,it,vi} from 'vitest';
vi.mock('./DJSaveResultsPlaylist',()=>({DJSaveResultsPlaylist:({songIds}:{songIds:string[]})=><div data-order={songIds.join(',')} />}));
import {DJMixNextPlaylist} from './DJMixNextPlaylist';
it('preserves rank with reference first and explains transition score semantics',async()=>{
 (globalThis as any).IS_REACT_ACT_ENVIRONMENT=true;
 const host=document.createElement('div'),root=createRoot(host);
 try {
  await act(async()=>root.render(<DJMixNextPlaylist referenceId="source" referenceTitle="Reference" candidates={[{songId:'b',title:'B',artist:'Artist',score:0.8},{songId:'source',title:'Reference',artist:'Artist',score:1},{songId:'a',title:'A',artist:'Artist',score:0.6}]} />));
  expect(host.querySelector('[data-order]')?.getAttribute('data-order')).toBe('source,b,a');
  expect(host.textContent).toContain('Transition suitability 80%');
  expect(host.textContent).toContain('Listen and adjust consecutive transitions');
  const includeB=host.querySelector('input[aria-label="Include B in playlist"]') as HTMLInputElement;
  await act(async()=>includeB.click());
  expect(host.querySelector('[data-order]')?.getAttribute('data-order')).toBe('source,a');
  await act(async()=>includeB.click());
  expect(host.querySelector('[data-order]')?.getAttribute('data-order')).toBe('source,b,a');
 }finally{await act(async()=>root.unmount());}
});
