// @vitest-environment jsdom
import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { expect, it, vi } from 'vitest';
const mocks = vi.hoisted(() => ({ load: vi.fn(), state: { spotifySessionGeneration: 0, songs: [{ id: 'a', title: 'Track', artist: 'Artist', album: 'Album', duration: 60, genre: [], addedAt: 1 }], playlists: [], djDeckA: { track: null, key: null, isPlaying: false }, djDeckB: { track: null, key: null, isPlaying: false } } }));
vi.mock('../../../store', () => { const useStore = (select: any) => select(mocks.state); Object.assign(useStore, { getState: () => mocks.state }); return { useStore }; });
vi.mock('../../../services/api', () => ({ api: { getTrackAnalysisFeatures: mocks.load } }));
vi.mock('../../../hooks/useDJAudioEngine', () => ({ useDJAudioEngineActions: () => ({ loadTrack: vi.fn() }) }));
vi.mock('react-virtuoso', () => ({ TableVirtuoso: ({ data, itemContent }: any) => <table><tbody>{data.map((item: any, index: number) => <tr key={item.id}>{itemContent(index, item)}</tr>)}</tbody></table> }));
import { DJLibraryBrowserV2 } from './DJLibraryBrowserV2';
it('refreshes mounted library energy and ignores responses captured before a manual change', async () => {
 (globalThis as any).IS_REACT_ACT_ENVIRONMENT = true;
 localStorage.clear();
 let settle!: (value: unknown) => void;
 mocks.load.mockImplementationOnce(() => new Promise(done => { settle = done; })).mockResolvedValue([{ songId: 'a', energyLevel: 9, energyLevelSource: 'manual' }]);
 const host = document.createElement('div'); const root = createRoot(host);
 try {
  await act(async () => root.render(<DJLibraryBrowserV2 />));
  await act(async () => {
   window.dispatchEvent(new CustomEvent('library_updated', { detail: { source: 'manual_audio_metadata', songId: 'a', sourceFingerprint: 'fp', field: 'local_energy_level' } }));
   settle([{ songId: 'a', energyLevel: 3 }]);
  });
  expect(mocks.load).toHaveBeenCalledTimes(2);
  expect(host.querySelector('[title="Energy Level 9/10 · Manual override"]')).not.toBeNull();
  expect(host.querySelector('[title^="Energy Level 3/10"]')).toBeNull();
 } finally { await act(async () => root.unmount()); }
});

it('keeps usable rows during a failed refresh and clears them on account change', async () => {
 (globalThis as any).IS_REACT_ACT_ENVIRONMENT = true;
 mocks.load.mockReset(); mocks.state.spotifySessionGeneration=0;
 let reject!: (error: Error) => void;
 mocks.load.mockResolvedValueOnce([{songId:'a',energyLevel:9,energyLevelSource:'manual'}]).mockImplementationOnce(()=>new Promise((_resolve,fail)=>{reject=fail;})).mockImplementation(()=>new Promise(()=>{}));
 const host=document.createElement('div');const root=createRoot(host);
 try {
  await act(async()=>root.render(<DJLibraryBrowserV2/>));
  const manual=()=>host.querySelector('[title="Energy Level 9/10 · Manual override"]');
  expect(manual()).not.toBeNull();
  await act(async()=>window.dispatchEvent(new CustomEvent('library_updated',{detail:{source:'manual_audio_metadata',songId:'a',sourceFingerprint:'fp',field:'local_energy_level'}})));
  expect(manual()).not.toBeNull();
  await act(async()=>reject(new Error('temporary')));
  expect(manual()).not.toBeNull();
  mocks.state.spotifySessionGeneration=1;
  await act(async()=>root.render(<DJLibraryBrowserV2/>));
  expect(manual()).toBeNull();
 }finally{await act(async()=>root.unmount());mocks.state.spotifySessionGeneration=0;}
});


it('retries failed initial analysis without reloading the library', async () => {
 (globalThis as any).IS_REACT_ACT_ENVIRONMENT = true;
 mocks.load.mockReset(); mocks.state.spotifySessionGeneration=0;
 mocks.load.mockRejectedValueOnce(new Error('temporary')).mockResolvedValueOnce([{songId:'a',energyLevel:9,energyLevelSource:'manual'}]);
 const host=document.createElement('div');const root=createRoot(host);
 try {
  await act(async()=>root.render(<DJLibraryBrowserV2/>));
  const retry=Array.from(host.querySelectorAll('button')).find(button=>button.textContent==='Retry analysis');
  expect(retry).toBeDefined();
  expect(host.textContent).toContain('Track');
  await act(async()=>retry!.click());
  expect(mocks.load).toHaveBeenCalledTimes(2);
  expect(host.querySelector('[title="Energy Level 9/10 · Manual override"]')).not.toBeNull();
  expect(host.textContent).not.toContain('Analysis could not be refreshed');
 }finally{await act(async()=>root.unmount());}
});
