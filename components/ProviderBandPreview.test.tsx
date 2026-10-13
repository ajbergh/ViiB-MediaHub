// @vitest-environment jsdom
import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { expect, it, vi } from 'vitest';
const mocks = vi.hoisted(() => ({ load: vi.fn(), refresh: vi.fn(), connected: true }));
vi.mock('../services/providerThreeBand', async original => ({ ...await original<typeof import('../services/providerThreeBand')>(), loadProviderThreeBand: mocks.load, refreshProviderThreeBand: mocks.refresh }));
vi.mock('../store', () => ({ useStore: (select: any) => select({ spotifyConnected: mocks.connected }) }));
import { ProviderBandPreview } from './ProviderBandPreview';
it('labels mismatch and hides another recording or session response', async () => {
  (globalThis as any).IS_REACT_ACT_ENVIRONMENT = true;
  mocks.load.mockResolvedValue({ sourceFingerprint: 'fp', recordingId: 'recording', lows: [-10, 10], mids: [0, 5], highs: [0, 1], durationSeconds: 30, alignment: 'duration_mismatch', retrievedAt: '2026-10-01T00:00:00Z', stale: true, unverified: true, readOnly: true, provenance: 'spotify_private_cache', windowMilliseconds: 20 });
  const host = document.createElement('div'); const root = createRoot(host);
  try {
    await act(async () => root.render(<ProviderBandPreview songId="song" fingerprint="fp" recordingId="recording" session={0} />));
    expect(host.querySelectorAll('svg')).toHaveLength(3);
    expect(host.textContent).toContain('Duration differs'); expect(host.textContent).toContain('Stale');
    expect(host.textContent).toContain('Account confirmation pending');
    expect(host.textContent).toContain('Available for viewing only');
    expect(mocks.refresh).not.toHaveBeenCalled();
    mocks.refresh.mockResolvedValue({ state: 'unavailable' });
    await act(async () => host.querySelector('button')!.click());
    expect(mocks.refresh).toHaveBeenCalledWith('recording');
    expect(host.querySelectorAll('svg')).toHaveLength(3);
    expect(host.textContent).toContain('Cached observations remain available');
    await act(async () => root.render(<ProviderBandPreview songId="song" fingerprint="fp" recordingId="replacement" session={1} />));
    expect(host.querySelectorAll('svg')).toHaveLength(0);
  } finally { await act(async () => root.unmount()); }
});

it('discovers durable bands without scalar recording props and does not enable unknown refresh',async()=>{
 mocks.connected=false;
 mocks.load.mockResolvedValue({sourceFingerprint:'fp',recordingId:'imported',lows:[1],mids:[2],highs:[3],durationSeconds:12,alignment:'local_duration_unavailable',retrievedAt:'2026-10-01T00:00:00Z',stale:false,provenance:'spotify_durable_import',windowMilliseconds:20});
 const host=document.createElement('div'),root=createRoot(host);
 try {
  await act(async()=>root.render(<ProviderBandPreview songId="song" fingerprint="fp" session={0} />));
  expect(host.querySelectorAll('svg')).toHaveLength(3);
  expect(host.textContent).toContain('Retained with this downloaded file');
  expect(host.querySelector('button')?.disabled).toBe(true);
  await act(async()=>root.render(<ProviderBandPreview songId="song" fingerprint="changed" session={0} />));
  expect(host.querySelectorAll('svg')).toHaveLength(0);
 }finally {await act(async()=>root.unmount());mocks.connected=true;}
});
