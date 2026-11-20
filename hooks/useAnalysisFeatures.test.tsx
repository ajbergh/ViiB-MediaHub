// @vitest-environment jsdom
import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { expect, it, vi } from 'vitest';
import { api, type TrackAnalysisFeature } from '../services/api';
import { useAnalysisFeatures } from './useAnalysisFeatures';

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
    vi.useRealTimers();
    vi.restoreAllMocks();
  }
});
