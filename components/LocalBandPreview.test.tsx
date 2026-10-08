// @vitest-environment jsdom
import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { expect, it, vi } from 'vitest';
const mocks = vi.hoisted(() => ({ load: vi.fn() }));
vi.mock('../services/localThreeBand', async original => ({ ...await original<typeof import('../services/localThreeBand')>(), loadLocalThreeBand: mocks.load }));
import { LocalBandPreview } from './LocalBandPreview';
it('renders three distinct envelopes and excludes another source', async () => {
  (globalThis as any).IS_REACT_ACT_ENVIRONMENT = true;
  mocks.load.mockResolvedValue({ sourceFingerprint: 'source', overview: { frames: 960, sampleRate: 48000, resolution: 960, low: [.5], mid: [.2], high: [.1] } });
  const host = document.createElement('div'); const root = createRoot(host);
  try {
    await act(async () => root.render(<LocalBandPreview songId="song" fingerprint="source" />));
    expect(host.querySelectorAll('svg')).toHaveLength(3); expect(mocks.load).toHaveBeenLastCalledWith('song', 'source');
    expect(host.textContent).toContain('0.02 seconds');
    expect(host.textContent).toContain('Common peak display scale');
    await act(async () => root.render(<LocalBandPreview songId="song" fingerprint="replacement" />));
    expect(host.querySelectorAll('svg')).toHaveLength(0);
    expect(host.textContent).toContain('not prepared');
  } finally { await act(async () => root.unmount()); }
});

it('recovers a transient load failure without reloading the application', async () => {
 (globalThis as any).IS_REACT_ACT_ENVIRONMENT = true;
 mocks.load.mockRejectedValueOnce(new Error('temporary')).mockResolvedValue({sourceFingerprint:'fp',overview:{frames:960,sampleRate:48000,resolution:960,low:[.5],mid:[.2],high:[.1]}});
 const host=document.createElement('div');const root=createRoot(host);
 try {
  await act(async()=>root.render(<LocalBandPreview songId="song" fingerprint="fp"/>));
  expect(host.textContent).toContain('could not be loaded');
  await act(async()=>(host.querySelector('button') as HTMLButtonElement).click());
  expect(host.querySelectorAll('svg')).toHaveLength(3);
  expect(host.textContent).not.toContain('could not be loaded');
 }finally{await act(async()=>root.unmount());}
});
