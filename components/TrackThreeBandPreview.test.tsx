// @vitest-environment jsdom
import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { expect, it, vi } from 'vitest';
const mocks = vi.hoisted(() => ({ provider: vi.fn(), local: vi.fn(), refresh: vi.fn(), report: vi.fn(), connected: true }));
vi.mock('../services/providerThreeBand', async original => ({ ...await original<typeof import('../services/providerThreeBand')>(), loadProviderThreeBand: mocks.provider, refreshProviderThreeBand: mocks.refresh }));
vi.mock('../services/localThreeBand', async original => ({ ...await original<typeof import('../services/localThreeBand')>(), loadLocalThreeBand: mocks.local }));
vi.mock('../services/audioReadDiagnostics', () => ({ reportAudioReadFailure: mocks.report }));
vi.mock('../store', () => ({ useStore: (select: any) => select({ spotifyConnected: mocks.connected }) }));
import { TrackThreeBandPreview } from './TrackThreeBandPreview';

it('selects provider bands as the only preview when current Spotify data exists', async () => {
  (globalThis as any).IS_REACT_ACT_ENVIRONMENT = true;
  mocks.provider.mockResolvedValue({ songId: 'song', sourceFingerprint: 'fp', recordingId: 'recording', representation: 'spotify_three_band', lows: [1, -2], mids: [3, 4], highs: [5, 6], durationSeconds: 30, alignment: 'duration_compatible', retrievedAt: '2026-10-01T00:00:00Z', stale: false, windowMilliseconds: 20, totalSamples: 2, displayStride: 1, displayAggregation: 'signed_absolute_max_v1' });
  const host = document.createElement('div'); const root = createRoot(host);
  try {
    await act(async () => root.render(<TrackThreeBandPreview songId="song" fingerprint="fp" recordingId="recording" session={0} />));
    expect(host.querySelectorAll('svg')).toHaveLength(3);
    expect(host.getAttribute('aria-label')).toBe(null);
    expect(host.textContent).toContain('Spotify three-band waveform');
    expect(mocks.local).not.toHaveBeenCalled();
  } finally { await act(async () => root.unmount()); }
});

it('uses local bands only after provider data is absent', async () => {
  (globalThis as any).IS_REACT_ACT_ENVIRONMENT = true;
  mocks.provider.mockResolvedValue(null);
  mocks.local.mockResolvedValue({ songId: 'song', sourceFingerprint: 'fp', representation: 'local_three_band_estimate', algorithmVersion: 'onepole-band-peaks-v1', overview: { frames: 960, sampleRate: 48000, resolution: 960, low: [.5], mid: [.2], high: [.1] } });
  const host = document.createElement('div'); const root = createRoot(host);
  try {
    await act(async () => root.render(<TrackThreeBandPreview songId="song" fingerprint="fp" session={0} />));
    expect(host.querySelectorAll('svg')).toHaveLength(3);
    expect(host.textContent).toContain('Local three-band estimate');
    expect(mocks.local).toHaveBeenCalledWith('song', 'fp', expect.any(Function));
  } finally { await act(async () => root.unmount()); }
});
