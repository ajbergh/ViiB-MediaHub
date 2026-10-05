// @vitest-environment jsdom

/** Tests and fixtures for Song Info Dialog behavior. */

import React,{act} from 'react';
import {createRoot} from 'react-dom/client';
import {expect,it,vi} from 'vitest';
const mocks=vi.hoisted(()=>({state:{} as any}));
vi.mock('../store',()=>({useStore:(selector?:any)=>selector?selector(mocks.state):mocks.state,useAlbumCovers:()=>({})}));
vi.mock('react-router',()=>({useNavigate:()=>vi.fn()}));
vi.mock('../hooks/useFocusTrap',()=>({useFocusTrap:()=>({current:null})}));
vi.mock('./LikeButton',()=>({LikeButton:()=>null}));
import {SongInfoDialog} from './SongInfoDialog';
it('closing an unfinished edit and opening another song discards the old draft',async()=>{
 (globalThis as any).IS_REACT_ACT_ENVIRONMENT=true;
 const a={id:'a',title:'Title A',artist:'Artist',album:'Album',url:'/a',duration:120,addedAt:1};
 mocks.state={songInfoModalSong:a,closeSongInfoModal:vi.fn(),playSong:vi.fn(),addToQueue:vi.fn(),showToast:vi.fn(),updateSongMetadata:vi.fn()};
 const container=document.createElement('div');document.body.appendChild(container);const root=createRoot(container);
 const render=()=>act(async()=>root.render(<SongInfoDialog/>));
 try {
  await render();const edit=[...container.querySelectorAll('button')].find(button=>button.textContent?.includes('Edit Tags'))!;
  await act(async()=>edit.click());expect(container.querySelector('input')?.value).toBe('Title A');
  mocks.state.songInfoModalSong=null;await render();mocks.state.songInfoModalSong={...a,id:'b',title:'Title B'};await render();
  expect(container.querySelector('input')).toBeNull();expect(container.textContent).toContain('Title B');
  await act(async()=>[...container.querySelectorAll('button')].find(button=>button.textContent?.includes('Edit Tags'))!.click());
  expect(container.querySelector('input')?.value).toBe('Title B');expect(mocks.state.updateSongMetadata).not.toHaveBeenCalled();
 } finally {await act(async()=>root.unmount());container.remove()}
});
