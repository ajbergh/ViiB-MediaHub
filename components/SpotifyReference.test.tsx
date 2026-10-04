// @vitest-environment jsdom
import React, { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';

const mocks = vi.hoisted(() => ({
 state: { spotifyConnected: true, spotifySessionGeneration: 0 },
 link: vi.fn(), cache: vi.fn(), confirm: vi.fn(), refresh: vi.fn(), unlink: vi.fn(),
}));
vi.mock('../store', () => ({ useStore: (select: (value: typeof mocks.state) => unknown) => select(mocks.state) }));
vi.mock('../services/spotifyReference', async importOriginal => ({
 ...await importOriginal<typeof import('../services/spotifyReference')>(),
 spotifyReference: { link: mocks.link, cache: mocks.cache, confirm: mocks.confirm, refresh: mocks.refresh, unlink: mocks.unlink },
}));
import { SpotifyReference } from './SpotifyReference';

const linked = { songId: 'local-song', sourceFingerprint: 'source', link: { externalId: '5r9W9MJLvHk83fcZSPQ8SE' } };
const cached = {
 state: 'available', stale: true, lastFailure: null,
 cache: { observation: { bpm: 108.022, camelot: null, bpmConfidence: null, sourceEndpoint: 'audio_features', retrievedAt: '2026-10-03T12:00:00Z' } },
};
let root: Root;
let host: HTMLDivElement;
beforeEach(() => {
 vi.clearAllMocks();
 mocks.state.spotifyConnected = true; mocks.state.spotifySessionGeneration = 0;
 mocks.link.mockResolvedValue(linked); mocks.cache.mockResolvedValue(cached);
 host = document.createElement('div'); root = createRoot(host);
 (globalThis as any).IS_REACT_ACT_ENVIRONMENT = true;
});
afterEach(async () => { await act(async () => root.unmount()); });

it('reads cache without fetching provider data and preserves unknown/stale values', async () => {
 await act(async () => root.render(<SpotifyReference songId="local-song" sourceIdentity="source" />));
 expect(mocks.refresh).not.toHaveBeenCalled();
 expect(host.textContent).toContain('BPM 108.022');
 expect(host.textContent).toContain('Key unknown');
 expect(host.textContent).toContain('BPM confidence unknown');
 expect(host.textContent).toContain('Stale');
 mocks.refresh.mockResolvedValue({ failure: { code: 'not_found' } });
 mocks.cache.mockResolvedValue({ ...cached, lastFailure: { code: 'not_found' } });
 const refresh = [...host.querySelectorAll('button')].find(button => button.textContent === 'Refresh reference')!;
 await act(async () => refresh.click());
 expect(mocks.refresh).toHaveBeenCalledWith(linked.link.externalId, 'audio_features');
 expect(host.textContent).toContain('BPM 108.022');
 expect(host.textContent).toContain('Spotify has no reference');
});

it('discards late cached reference when the local source changes', async () => {
 let resolve!: (value: typeof cached) => void;
 mocks.cache.mockImplementationOnce(() => new Promise(done => { resolve = done; }));
 await act(async () => root.render(<SpotifyReference songId="local-song" sourceIdentity="old-source" />));
 mocks.link.mockResolvedValue({ ...linked, link: null });
 await act(async () => root.render(<SpotifyReference songId="local-song" sourceIdentity="new-source" />));
 await act(async () => resolve(cached));
 expect(host.textContent).not.toContain('BPM 108.022');
 expect(host.textContent).toContain('same recording and version');
 expect(mocks.refresh).not.toHaveBeenCalled();
});

it('requires explicit refresh for detailed analysis and disables it while offline', async () => {
 await act(async () => root.render(<SpotifyReference songId="local-song" sourceIdentity="source" />));
 const select = host.querySelector('select')!;
 await act(async () => { select.value = 'audio_analysis'; select.dispatchEvent(new Event('change', { bubbles: true })); });
 expect(mocks.refresh).not.toHaveBeenCalled();
 expect(mocks.cache).toHaveBeenLastCalledWith(linked.link.externalId, 'audio_analysis');
 mocks.state.spotifyConnected = false; mocks.state.spotifySessionGeneration++;
 await act(async () => root.render(<SpotifyReference songId="local-song" sourceIdentity="source" />));
 const refresh = [...host.querySelectorAll('button')].find(button => button.textContent === 'Refresh reference')!;
 expect(refresh.disabled).toBe(true);
});
