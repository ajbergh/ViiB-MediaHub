// @vitest-environment jsdom
import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { expect, it, vi } from 'vitest';

const state = vi.hoisted(() => ({
  djDeckA: { beatGridSource: 'spotify', bpmConfidence: null, beatGrid: [0, 0.5, 1], beatGridLocked: false },
  djDeckB: { beatGridSource: 'unknown', bpmConfidence: null, beatGrid: [], beatGridLocked: false },
}));
vi.mock('../../../store', () => ({ useStore: (select: any) => select(state) }));

import { DJBeatGridStatus } from './DJBeatGridStatus';

it('labels provider timing as Spotify and still requires manual review before sync', async () => {
  (globalThis as any).IS_REACT_ACT_ENVIRONMENT = true;
  const host = document.createElement('div');
  const root = createRoot(host);
  try {
    await act(async () => root.render(<DJBeatGridStatus deck="A" />));
    expect(host.textContent).toContain('Spotify grid · Review');
    expect(host.textContent).not.toContain('Verified grid');
  } finally {
    await act(async () => root.unmount());
  }
});
