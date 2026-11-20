// @vitest-environment jsdom
import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { expect, it, vi } from 'vitest';
import { api, type TrackAnalysisFeature } from '../services/api';
import { useAnalysisFeatures } from './useAnalysisFeatures';

it('shares an initial analysis request and coalesces repeated changes by track', async () => {
  vi.useFakeTimers();
  (globalThis as any).IS_REACT_ACT_ENVIRONMENT = true;
  const fetch = vi.spyOn(api, 'getTrackAnalysisFeatures').mockImplementation(async ids =>
    (ids ?? ['first', 'second']).map(songId => ({ songId, bpm: ids ? 130 : 120 }) as TrackAnalysisFeature));
  const rootElement = document.createElement('div');
  const root = createRoot(rootElement);
  function Reader() {
    const { rows } = useAnalysisFeatures();
    return <span>{rows.first?.bpm},{rows.second?.bpm};</span>;
  }
  try {
    await act(async () => root.render(<><Reader /><Reader /></>));
    await act(async () => vi.advanceTimersByTimeAsync(100));
    expect(fetch).toHaveBeenCalledTimes(1);
    expect(rootElement.textContent).toBe('120,120;120,120;');
    for (let i = 0; i < 2; i++) {
      window.dispatchEvent(new CustomEvent('library_updated', { detail: { source: 'library_delta', songIds: ['first'] } }));
    }
    await act(async () => vi.advanceTimersByTimeAsync(100));
    expect(fetch).toHaveBeenCalledTimes(2);
    expect(fetch.mock.calls[1][0]).toEqual(['first']);
    expect(rootElement.textContent).toBe('130,120;130,120;');
  } finally {
    await act(async () => root.unmount());
    vi.useRealTimers();
    vi.restoreAllMocks();
  }
});
