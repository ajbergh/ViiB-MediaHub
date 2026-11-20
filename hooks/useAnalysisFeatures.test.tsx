// @vitest-environment jsdom
import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { afterEach, expect, it, vi } from 'vitest';
import { api, type TrackAnalysisFeature } from '../services/api';
import { useStore } from '../store';
import { compareLibraryText, LibraryIndex, sortByTextKey } from '../lib/libraryIndex';
import type { Song } from '../types';
import { useAnalysisFeatures } from './useAnalysisFeatures';

afterEach(() => {
  vi.useRealTimers();
  vi.restoreAllMocks();
});

it('shares an initial analysis request and coalesces repeated changes by track', async () => {
  vi.useFakeTimers();
  (globalThis as any).IS_REACT_ACT_ENVIRONMENT = true;
  let bpm = 120;
  let energyLevel = 1;
  const fetch = vi.spyOn(api, 'getTrackAnalysisFeatures').mockImplementation(async ids =>
    (ids ?? ['first', 'second']).map(songId => ({ songId, bpm, energyLevel }) as TrackAnalysisFeature));
  const rootElement = document.createElement('div');
  const root = createRoot(rootElement);
  let bpmRevision = 0;
  let energyRevision = 0;
  function Reader() {
    const { rows } = useAnalysisFeatures();
    bpmRevision = rows.revisionFor(['bpm']);
    energyRevision = rows.revisionFor(['energyLevel']);
    return <span>{rows.get('first')?.bpm},{rows.get('second')?.bpm};</span>;
  }
  try {
    await act(async () => root.render(<><Reader /><Reader /></>));
    await act(async () => vi.advanceTimersByTimeAsync(100));
    expect(fetch).toHaveBeenCalledTimes(1);
    expect(rootElement.textContent).toBe('120,120;120,120;');
    bpm = 130;
    for (let i = 0; i < 2; i++) {
      window.dispatchEvent(new CustomEvent('library_updated', { detail: { source: 'library_delta', songIds: ['first'] } }));
    }
    await act(async () => vi.advanceTimersByTimeAsync(100));
    expect(fetch).toHaveBeenCalledTimes(2);
    expect(fetch.mock.calls[1][0]).toEqual(['first']);
    expect(rootElement.textContent).toBe('130,120;130,120;');
    const orderedBpm = bpmRevision;
    const orderedEnergy = energyRevision;
    energyLevel = 2;
    window.dispatchEvent(new CustomEvent('library_updated', { detail: { songId: 'first' } }));
    await act(async () => vi.advanceTimersByTimeAsync(100));
    expect(bpmRevision).toBe(orderedBpm);
    expect(energyRevision).toBeGreaterThan(orderedEnergy);
  } finally {
    await act(async () => root.unmount());
  }
});

it('fetches viewport-scoped scalarsOnce, caches scrolled IDs, upgrades to full detail, and invalidates per ID', async () => {
  vi.useFakeTimers();
  (globalThis as any).IS_REACT_ACT_ENVIRONMENT = true;
  useStore.setState(s => ({ spotifySessionGeneration: s.spotifySessionGeneration + 1 }));

  const fetch = vi.spyOn(api, 'getTrackAnalysisFeatures').mockImplementation(async (ids, opts) =>
    (ids ?? []).map(songId => ({ songId, bpm: opts?.scalarsOnly ? 124 : 125 }) as TrackAnalysisFeature));

  const rootElement = document.createElement('div');
  const root = createRoot(rootElement);
  function ViewportReader({ ids, scalarsOnly = true }: { ids: string[]; scalarsOnly?: boolean }) {
    const { rows } = useAnalysisFeatures({ scalarsOnly, songIds: ids });
    return <span>{ids.map(id => rows.get(id)?.bpm ?? '-').join(',')}</span>;
  }

  try {
    await act(async () => root.render(<ViewportReader ids={['a', 'b']} />));
    await act(async () => vi.advanceTimersByTimeAsync(100));
    expect(fetch).toHaveBeenCalledTimes(1);
    expect(fetch).toHaveBeenLastCalledWith(['a', 'b'], expect.objectContaining({ summary: true, scalarsOnly: true }));
    expect(rootElement.textContent).toBe('124,124');

    // Scroll forward to ['b', 'c']: only 'c' should be fetched.
    await act(async () => root.render(<ViewportReader ids={['b', 'c']} />));
    await act(async () => vi.advanceTimersByTimeAsync(100));
    expect(fetch).toHaveBeenCalledTimes(2);
    expect(fetch).toHaveBeenLastCalledWith(['c'], expect.objectContaining({ summary: true, scalarsOnly: true }));
    expect(rootElement.textContent).toBe('124,124');

    // Scroll back to ['a', 'b']: already cached, so no fetch.
    await act(async () => root.render(<ViewportReader ids={['a', 'b']} />));
    await act(async () => vi.advanceTimersByTimeAsync(100));
    expect(fetch).toHaveBeenCalledTimes(2);

    // Upgrade 'a' to full detail (scalarsOnly: false): fetches 'a' without scalarsOnly.
    await act(async () => root.render(<ViewportReader ids={['a']} scalarsOnly={false} />));
    await act(async () => vi.advanceTimersByTimeAsync(100));
    expect(fetch).toHaveBeenCalledTimes(3);
    expect(fetch.mock.calls[2][0]).toEqual(['a']);
    expect(fetch.mock.calls[2][1]?.scalarsOnly).toBeUndefined();
    expect(rootElement.textContent).toBe('125');

    // Invalidate only 'a' via library_updated: re-fetches only 'a'.
    await act(async () => {
      window.dispatchEvent(new CustomEvent('library_updated', { detail: { source: 'library_delta', songIds: ['a'] } }));
    });
    await act(async () => vi.advanceTimersByTimeAsync(100));
    expect(fetch).toHaveBeenCalledTimes(4);
    expect(fetch.mock.calls[3][0]).toEqual(['a']);
  } finally {
    await act(async () => root.unmount());
  }
});

it('loads all pages and removes rows omitted from a full refresh', async () => {
  vi.useFakeTimers();
  (globalThis as any).IS_REACT_ACT_ENVIRONMENT = true;
  useStore.setState(s => ({ spotifySessionGeneration: s.spotifySessionGeneration + 1 }));

  const pageFetch = vi.spyOn(api, 'getTrackAnalysisFeaturesPage')
    .mockResolvedValueOnce({
      items: [{ songId: 'p1', bpm: 120 } as TrackAnalysisFeature],
      nextCursor: 'cursor-1',
      hasMore: true,
    })
    .mockResolvedValueOnce({
      items: [{ songId: 'p2', bpm: 128 } as TrackAnalysisFeature],
      hasMore: false,
    })
    .mockResolvedValueOnce({
      items: [{ songId: 'p2', bpm: 130 } as TrackAnalysisFeature],
      hasMore: false,
    });

  const rootElement = document.createElement('div');
  const root = createRoot(rootElement);
  function PagedReader() {
    const { rows, status } = useAnalysisFeatures({ fullLoad: true, paged: true });
    return <span>{status}:{rows.get('p1')?.bpm ?? '-'}:{rows.get('p2')?.bpm ?? '-'}</span>;
  }

  try {
    await act(async () => root.render(<PagedReader />));
    await act(async () => vi.advanceTimersByTimeAsync(100));
    expect(pageFetch).toHaveBeenCalledTimes(2);
    expect(pageFetch).toHaveBeenNthCalledWith(1, undefined, 500, expect.objectContaining({ summary: true }));
    expect(pageFetch).toHaveBeenNthCalledWith(2, 'cursor-1', 500, expect.objectContaining({ summary: true }));
    expect(rootElement.textContent).toBe('loaded:120:128');
    window.dispatchEvent(new CustomEvent('library_updated', { detail: { source: 'full_refresh' } }));
    await act(async () => vi.advanceTimersByTimeAsync(100));
    expect(rootElement.textContent).toBe('loaded:-:130');
  } finally {
    await act(async () => root.unmount());
  }
});

it('retries every unpublished analysis delta after a later chunk fails', async () => {
  vi.useFakeTimers();
  (globalThis as any).IS_REACT_ACT_ENVIRONMENT = true;
  useStore.setState(s => ({ spotifySessionGeneration: s.spotifySessionGeneration + 1 }));
  const ids = Array.from({ length: 101 }, (_, i) => `song-${i}`);
  const rows = (songIds: string[], bpm: number) => songIds.map(songId => ({ songId, bpm }) as TrackAnalysisFeature);
  const fetch = vi.spyOn(api, 'getTrackAnalysisFeatures')
    .mockResolvedValueOnce(rows(ids, 120))
    .mockResolvedValueOnce(rows(ids.slice(0, 100), 130))
    .mockRejectedValueOnce(new Error('Temporary fetch failure'))
    .mockImplementation(async songIds => rows(songIds ?? [], 130));
  const rootElement = document.createElement('div');
  const root = createRoot(rootElement);
  function Reader() {
    const { rows, status, retry } = useAnalysisFeatures();
    return <button onClick={retry}>{status}:{rows.get(ids[0])?.bpm}:{rows.get(ids[100])?.bpm}</button>;
  }

  try {
    await act(async () => root.render(<Reader />));
    await act(async () => vi.advanceTimersByTimeAsync(100));
    window.dispatchEvent(new CustomEvent('library_updated', { detail: { songIds: ids } }));
    await act(async () => vi.advanceTimersByTimeAsync(100));
    expect(rootElement.textContent).toBe('failed:120:120');
    await act(async () => rootElement.querySelector('button')!.click());
    await act(async () => vi.advanceTimersByTimeAsync(100));
    expect(fetch.mock.calls.slice(3).map(([songIds]) => songIds)).toEqual([ids.slice(0, 100), ids.slice(100)]);
    expect(rootElement.textContent).toBe('loaded:130:130');
  } finally {
    await act(async () => root.unmount());
  }
});

it('preserves localeCompare order for accents and case', () => {
  const names = ['Zulu', 'Écho', 'Ångström', 'echo', 'Beta'];
  expect(sortByTextKey(names, name => name)).toEqual([...names].sort((a, b) => a.localeCompare(b)));
  expect(sortByTextKey(names, name => name, 'desc')).toEqual([...names].sort((a, b) => b.localeCompare(a)));
});

it('benchmarks sorting 50,000 synthetic tracks: sortByTextKey and LibraryIndex vs localeCompare', () => {
  const count = 50_000;
  const tracks: Song[] = Array.from({ length: count }, (_, i) => ({
    id: `song-${i}`,
    title: `${i % 20 === 0 ? 'Écho' : 'Track'} ${((i * 2654435761) >>> 0).toString(36)}`,
    artist: `Artist ${i % 500}`,
    album: `Album ${i % 2000}`,
    duration: 180,
    url: `/api/audio/song-${i}`,
    addedAt: i,
  }));

  const tCollatorStart = performance.now();
  const collatorSorted = [...tracks].sort((a, b) => compareLibraryText(a.title, b.title));
  const collatorMs = performance.now() - tCollatorStart;

  const tPrecomputedStart = performance.now();
  const keySorted = sortByTextKey(tracks, s => s.title, 'asc');
  const precomputedMs = performance.now() - tPrecomputedStart;

  const index = new LibraryIndex();
  const tIndexStart = performance.now();
  const indexed = index.initialize(tracks);
  const indexMs = performance.now() - tIndexStart;

  // Measure localeCompare on 5,000 tracks (extrapolated reference) so test suite remains fast.
  const sample = tracks.slice(0, 5_000);
  const tLocaleStart = performance.now();
  [...sample].sort((a, b) => a.title.localeCompare(b.title));
  const localeSampleMs = performance.now() - tLocaleStart;

  expect(collatorSorted).toHaveLength(count);
  expect(keySorted).toHaveLength(count);
  expect(indexed).toHaveLength(count);
  for (let i = 1; i < keySorted.length; i++) {
    expect(keySorted[i]).toBe(collatorSorted[i]);
  }
  console.info(
    `[50k sort benchmark] sortByTextKey=${precomputedMs.toFixed(1)}ms, Intl.Collator=${collatorMs.toFixed(1)}ms, LibraryIndex=${indexMs.toFixed(1)}ms, localeCompare(5k)=${localeSampleMs.toFixed(1)}ms`,
  );
});
