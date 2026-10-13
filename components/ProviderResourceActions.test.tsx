// @vitest-environment jsdom
import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { expect, it, vi } from 'vitest';
const mocks = vi.hoisted(() => ({ link: vi.fn(), refresh: vi.fn() }));
vi.mock('../services/spotifyReference', () => ({ spotifyReference: mocks }));
vi.mock('../store', () => ({ useStore: (select: any) => select({ spotifyConnected: true }) }));
import { ProviderResourceActions } from './ProviderResourceActions';
it('dispatches independent explicit resources and preserves failure state', async () => {
  (globalThis as any).IS_REACT_ACT_ENVIRONMENT = true;
  mocks.link.mockResolvedValue({ sourceFingerprint: 'fp', link: { externalId: 'recording', sourceFingerprint: 'fp' } });
  mocks.refresh.mockResolvedValueOnce({ failure: { code: 'not_found' } }).mockResolvedValueOnce({ failure: null });
  const reload = vi.fn(); const host = document.createElement('div'); const root = createRoot(host);
  try {
    await act(async () => root.render(<ProviderResourceActions songId="song" fingerprint="fp" session={0} onReload={reload} />));
    expect(mocks.refresh).not.toHaveBeenCalled();
    await act(async () => (host.querySelectorAll('button')[0] as HTMLButtonElement).click());
    expect(mocks.refresh).toHaveBeenLastCalledWith('recording', 'audio_features'); expect(reload).not.toHaveBeenCalled();
    expect(host.textContent).toContain('Last-good observations are preserved');
    await act(async () => (host.querySelectorAll('button')[1] as HTMLButtonElement).click());
    expect(mocks.refresh).toHaveBeenLastCalledWith('recording', 'audio_analysis'); expect(reload).toHaveBeenCalledOnce();
  } finally { await act(async () => root.unmount()); }
});
