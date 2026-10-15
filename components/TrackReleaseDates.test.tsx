// @vitest-environment jsdom
import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { expect, it, vi } from 'vitest';
const mocks = vi.hoisted(() => ({ load: vi.fn() }));
vi.mock('../services/api', () => ({ api: { getTrackReleaseMetadata: mocks.load } }));
import { TrackReleaseDates } from './TrackReleaseDates';

it('preserves date precision and ignores results for a previous track', async () => {
  (globalThis as any).IS_REACT_ACT_ENVIRONMENT = true;
  let finishOld!: (value: unknown) => void;
  mocks.load.mockReturnValueOnce(new Promise(resolve => { finishOld = resolve; }));
  mocks.load.mockResolvedValueOnce({ originalDate: '1989', originalSource: 'musicbrainz', editionDate: '2024-02', editionSource: 'spotify' });
  const host = document.createElement('div');
  const root = createRoot(host);
  try {
    await act(async () => root.render(<TrackReleaseDates songID="old" />));
    await act(async () => root.render(<TrackReleaseDates songID="new" />));
    expect(host.textContent).toContain('Original release: 1989 · MusicBrainz');
    expect(host.textContent).toContain('Edition release: 2024-02 · Spotify');
    expect(host.textContent).not.toContain('1989-01-01');
    await act(async () => finishOld({ originalDate: '1999', originalSource: 'ai_inferred' }));
    expect(host.textContent).not.toContain('1999');
  } finally { await act(async () => root.unmount()); }
});
