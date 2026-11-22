// @vitest-environment jsdom
/** Tests DJ library navigation, analysis freshness, and viewport loading. */

import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
const mocks = vi.hoisted(() => ({ load: vi.fn(), loadTrack: vi.fn(), scroll: vi.fn(), state: { spotifySessionGeneration: 0, songs: [{ id: 'a', title: 'Track', artist: 'Artist', album: 'Album', duration: 60, genre: [], addedAt: 1 }], playlists: [], djDeckA: { track: null, key: null, isPlaying: false }, djDeckB: { track: null, key: null, isPlaying: false } } }));
vi.mock('../../../store', () => { const useStore = (select: any) => select(mocks.state); Object.assign(useStore, { getState: () => mocks.state }); return { useStore }; });
vi.mock('../../../services/api', () => ({ api: { getTrackAnalysisFeatures: mocks.load } }));
vi.mock('../../../hooks/useDJAudioEngine', () => ({ useDJAudioEngineActions: () => ({ loadTrack: mocks.loadTrack }) }));
vi.mock('react-virtuoso', () => ({ TableVirtuoso: React.forwardRef(function Table({ data, itemContent, fixedHeaderContent, context, components }: any, ref) {
 React.useImperativeHandle(ref, () => ({ scrollIntoView: mocks.scroll }), []);
 const Row = components.TableRow;
 return <table><thead>{fixedHeaderContent?.()}</thead><tbody>{data.map((item: any, index: number) => <Row key={item.id} item={item} context={context}>{itemContent(index, item)}</Row>)}</tbody></table>;
}) }));
import { DJLibraryBrowserV2 } from './DJLibraryBrowserV2';
import { browseDJMidiLibrary, loadDJMidiLibrary } from '../../../lib/djMidiLibrary';

beforeEach(() => { vi.useFakeTimers(); mocks.state.spotifySessionGeneration++; });
afterEach(() => vi.useRealTimers());
const flushAnalysis = () => act(async () => vi.advanceTimersByTimeAsync(100));

it('highlights and scrolls the dial selection, loading that same track to either deck', async () => {
 (globalThis as any).IS_REACT_ACT_ENVIRONMENT = true;
 localStorage.clear();
 const originalSongs = mocks.state.songs;
 mocks.state.songs = ['Alpha', 'Bravo', 'Charlie'].map((title, index) => ({ ...originalSongs[0], id: String(index), title }));
 mocks.state.djDeckA.isPlaying = true; // Hardware load matches the UI even on a playing deck.
 mocks.load.mockReset().mockResolvedValue([]);
 mocks.loadTrack.mockReset().mockResolvedValue(undefined);
 mocks.scroll.mockClear();
 const host = document.createElement('div'); const root = createRoot(host);
 const selected = () => host.querySelector('tr[aria-selected="true"]');
 try {
  await act(async () => root.render(<DJLibraryBrowserV2 />));
  await flushAnalysis();
  await act(async () => { browseDJMidiLibrary(1); browseDJMidiLibrary(1); });
  expect(selected()?.textContent).toContain('Bravo');
  expect(selected()?.className).toContain('outline-[var(--dj-info)]');
  expect(mocks.scroll).toHaveBeenLastCalledWith({ index: 1, align: 'center', behavior: 'auto' });
  await act(async () => { loadDJMidiLibrary('A'); loadDJMidiLibrary('B'); });
  expect(mocks.loadTrack).toHaveBeenNthCalledWith(1, 'A', mocks.state.songs[1]);
  expect(mocks.loadTrack).toHaveBeenNthCalledWith(2, 'B', mocks.state.songs[1]);
  const alpha = Array.from(host.querySelectorAll('tr')).find(row => row.textContent?.includes('Alpha'))!;
  await act(async () => alpha.click());
  await act(async () => loadDJMidiLibrary('A'));
  expect(mocks.loadTrack).toHaveBeenLastCalledWith('A', mocks.state.songs[0]);
  await act(async () => browseDJMidiLibrary(-100));
  expect(selected()?.textContent).toContain('Alpha');
  await act(async () => browseDJMidiLibrary(100));
  expect(selected()?.textContent).toContain('Charlie');
  const search = host.querySelector<HTMLInputElement>('input[type="text"]')!;
  await act(async () => {
   Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!.call(search, 'Bravo');
   search.dispatchEvent(new Event('input', { bubbles: true }));
  });
  await act(async () => loadDJMidiLibrary('A'));
  expect(mocks.loadTrack).toHaveBeenLastCalledWith('A', mocks.state.songs[1]);
  expect(selected()?.textContent).toContain('Bravo');
 } finally {
  await act(async () => root.unmount());
  mocks.state.songs = originalSongs;
  mocks.state.djDeckA.isPlaying = false;
 }
});
it('refreshes mounted library energy and ignores responses captured before a manual change', async () => {
 (globalThis as any).IS_REACT_ACT_ENVIRONMENT = true;
 localStorage.clear();
 mocks.load.mockReset();
 let settle!: (value: unknown) => void;
 mocks.load.mockImplementationOnce(() => new Promise(done => { settle = done; })).mockResolvedValue([{ songId: 'a', energyLevel: 9, energyLevelSource: 'manual' }]);
 const host = document.createElement('div'); const root = createRoot(host);
 try {
  await act(async () => root.render(<DJLibraryBrowserV2 />));
  await flushAnalysis();
  await act(async () => {
   window.dispatchEvent(new CustomEvent('library_updated', { detail: { source: 'manual_audio_metadata', songId: 'a', sourceFingerprint: 'fp', field: 'local_energy_level' } }));
   settle([{ songId: 'a', energyLevel: 3 }]);
  });
  await flushAnalysis();
  expect(mocks.load).toHaveBeenCalledTimes(2);
  expect(host.querySelector('[title="Energy Level 9/10 · Manual override"]')).not.toBeNull();
  expect(host.querySelector('[title^="Energy Level 3/10"]')).toBeNull();
 } finally { await act(async () => root.unmount()); }
});

it('keeps usable rows during a failed refresh and clears them on account change', async () => {
 (globalThis as any).IS_REACT_ACT_ENVIRONMENT = true;
 mocks.load.mockReset();
 let reject!: (error: Error) => void;
 mocks.load.mockResolvedValueOnce([{songId:'a',energyLevel:9,energyLevelSource:'manual'}]).mockImplementationOnce(()=>new Promise((_resolve,fail)=>{reject=fail;})).mockImplementation(()=>new Promise(()=>{}));
 const host=document.createElement('div');const root=createRoot(host);
 try {
  await act(async()=>root.render(<DJLibraryBrowserV2/>));
  await flushAnalysis();
  const manual=()=>host.querySelector('[title="Energy Level 9/10 · Manual override"]');
  expect(manual()).not.toBeNull();
  await act(async()=>window.dispatchEvent(new CustomEvent('library_updated',{detail:{source:'manual_audio_metadata',songId:'a',sourceFingerprint:'fp',field:'local_energy_level'}})));
  expect(manual()).not.toBeNull();
  await flushAnalysis();
  await act(async()=>reject(new Error('temporary')));
  expect(manual()).not.toBeNull();
  mocks.state.spotifySessionGeneration++;
  await act(async()=>root.render(<DJLibraryBrowserV2/>));
  await flushAnalysis();
  expect(manual()).toBeNull();
 }finally{await act(async()=>root.unmount());}
});


it('retries failed initial analysis without reloading the library', async () => {
 (globalThis as any).IS_REACT_ACT_ENVIRONMENT = true;
 mocks.load.mockReset();
 mocks.load.mockRejectedValueOnce(new Error('temporary')).mockResolvedValueOnce([{songId:'a',energyLevel:9,energyLevelSource:'manual'}]);
 const host=document.createElement('div');const root=createRoot(host);
 try {
  await act(async()=>root.render(<DJLibraryBrowserV2/>));
  await flushAnalysis();
  const retry=Array.from(host.querySelectorAll('button')).find(button=>button.textContent==='Retry analysis');
  expect(retry).toBeDefined();
  expect(host.textContent).toContain('Track');
  await act(async()=>retry!.click());
  await flushAnalysis();
  expect(mocks.load).toHaveBeenCalledTimes(2);
  expect(host.querySelector('[title="Energy Level 9/10 · Manual override"]')).not.toBeNull();
  expect(host.textContent).not.toContain('Analysis could not be refreshed');
 }finally{await act(async()=>root.unmount());}
});

it('shows only current effective meter with its provenance and no beat-grid side effect', async () => {
 (globalThis as any).IS_REACT_ACT_ENVIRONMENT = true;
 localStorage.clear();
 localStorage.setItem('viib.dj.library.columnVisibility', JSON.stringify({ meter: true }));

 mocks.state.songs = [
  { id: 'provider', title: 'Provider meter', artist: 'Artist', album: 'Album', duration: 60, genre: [], addedAt: 1 },
  { id: 'manual', title: 'Manual meter', artist: 'Artist', album: 'Album', duration: 60, genre: [], addedAt: 2 },
  { id: 'stale', title: 'Stale meter', artist: 'Artist', album: 'Album', duration: 60, genre: [], addedAt: 3 },
 ];
 mocks.load.mockReset().mockResolvedValue([
  { songId: 'provider', effectiveFields: [{ key: 'time_signature', state: 'available', selected: { value: 4, source: 'spotify_private' } }] },
  { songId: 'manual', effectiveFields: [{ key: 'time_signature', state: 'available', selected: { value: 3, source: 'manual' } }] },
  { songId: 'stale', effectiveFields: [{ key: 'time_signature', state: 'unknown', lastGood: { value: 7, source: 'spotify_private' } }] },
 ]);
 const host=document.createElement('div');const root=createRoot(host);
 try {
  await act(async()=>root.render(<DJLibraryBrowserV2/>));
  await flushAnalysis();
  const providerRow = host.querySelector('[aria-label="Meter: 4 beats per bar"]')?.closest('tr');
  const manualRow = host.querySelector('[aria-label="Meter: 3 beats per bar"]')?.closest('tr');
  const staleRow = Array.from(host.querySelectorAll('tr')).find(row => row.textContent?.includes('Stale meter'));
  expect(providerRow?.querySelector('[title="Meter / time signature · Spotify; does not set beat-grid timing"]')).not.toBeNull();
  expect(manualRow?.querySelector('[title="Meter / time signature · Manual override; does not set beat-grid timing"]')).not.toBeNull();
  expect(staleRow?.querySelector('[aria-label="Meter unavailable"]')).not.toBeNull();
  expect(staleRow?.textContent).not.toContain('7');
 } finally {
  await act(async()=>root.unmount());
  localStorage.clear();
  mocks.state.songs = [{ id: 'a', title: 'Track', artist: 'Artist', album: 'Album', duration: 60, genre: [], addedAt: 1 }];
 }
});

it('fetches only the initial viewport window on open and loads full analysis when sorting by BPM', async () => {
 (globalThis as any).IS_REACT_ACT_ENVIRONMENT = true;
 localStorage.clear();
 const originalSongs = mocks.state.songs;
 mocks.state.songs = Array.from({ length: 150 }, (_, index) => ({
  id: `track-${String(index).padStart(3, '0')}`,
  title: `Track ${String(index).padStart(3, '0')}`,
  artist: 'Artist',
  album: 'Album',
  duration: 60,
  genre: [],
  addedAt: index,
 }));
 mocks.load.mockReset().mockResolvedValue([]);
 const host = document.createElement('div');
 const root = createRoot(host);
 try {
  await act(async () => root.render(<DJLibraryBrowserV2 />));
  await flushAnalysis();
  expect(mocks.load).toHaveBeenCalledTimes(1);
  expect(mocks.load.mock.calls[0][0]).toHaveLength(100);

  const bpmHeader = Array.from(host.querySelectorAll('th')).find(th => th.textContent?.includes('BPM'))!;
  await act(async () => bpmHeader.click());
  await flushAnalysis();
  expect(mocks.load).toHaveBeenCalledTimes(2);
  expect(mocks.load.mock.calls[1][0]).toBeUndefined();
 } finally {
  await act(async () => root.unmount());
  mocks.state.songs = originalSongs;
 }
});
