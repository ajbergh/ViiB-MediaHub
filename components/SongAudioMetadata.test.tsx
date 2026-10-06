// @vitest-environment jsdom
import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { expect, it, vi } from 'vitest';
const mocks = vi.hoisted(() => ({ load: vi.fn(), session: 0 }));
vi.mock('../services/api', () => ({ api: { getTrackAnalysisFeature: mocks.load } }));
vi.mock('../store', () => ({ useStore: (select: any) => select({ spotifySessionGeneration: mocks.session }) }));
import { SongAudioMetadata } from './SongAudioMetadata';

it('keeps zero provider scores, stale provenance and local metrics separate', async () => {
  (globalThis as any).IS_REACT_ACT_ENVIRONMENT = true;
  const field = { key: 'spotify_energy_score', metric: 'spotify_energy', units: 'unit_interval', value: 0, endpoint: 'audio_features', retrievedAt: '2026-10-01T00:00:00Z', expiresAt: '2026-10-02T00:00:00Z', stale: true };
  mocks.load.mockResolvedValue({ songId: 'a', bpmSource: 'unknown', keySource: 'unknown', energyLevel: 7, providerScalars: { unverified: true, readOnly: true, recordingId: 'recording', selected: [field], fields: [field], attempts: [] } });
  const host = document.createElement('div'); const root = createRoot(host);
  try {
    await act(async () => root.render(<SongAudioMetadata songId="a" />));
    expect(host.textContent).toContain('0 unit interval');
    expect(host.textContent).toContain('Account confirmation pending');
    expect(host.textContent).toContain('Available for viewing only');
    expect(host.textContent).toContain('Stale');
    expect(host.textContent).toContain('Local DJ energy 7 / 10');
    expect(host.textContent).toContain('Local integrated loudness Unknown LUFS');
    expect(host.textContent).toContain('Audio features');
    expect(host.textContent).toContain('Treat these fields as unavailable locally');
    expect(host.textContent).toContain('not measured meter');
    expect(host.textContent).toContain('Missing Spotify fields do not imply zero');
  } finally { await act(async () => root.unmount()); }
});

it('discards late results when the song changes', async () => {
  let resolve!: (value: unknown) => void;
  mocks.load.mockImplementationOnce(() => new Promise(done => { resolve = done; }));
  mocks.load.mockResolvedValue({ songId: 'b', bpm: 120, bpmSource: 'measured', keySource: 'unknown' });
  const host = document.createElement('div'); const root = createRoot(host);
  try {
    await act(async () => root.render(<SongAudioMetadata songId="a" />));
    await act(async () => root.render(<SongAudioMetadata songId="b" />));
    await act(async () => resolve({ songId: 'a', bpm: 199, bpmSource: 'spotify', keySource: 'unknown' }));
    expect(host.textContent).toContain('BPM 120');
    expect(host.textContent).not.toContain('199');
  } finally { await act(async () => root.unmount()); }
});
