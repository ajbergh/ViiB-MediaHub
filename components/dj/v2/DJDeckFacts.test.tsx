// @vitest-environment jsdom

import React, { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

const mocks = vi.hoisted(() => ({
  state: {
    djDeckA: { effectiveBpm: null as number | null, originalBpm: 130.2 as number | null, key: null as string | null, analysisStatus: 'available' },
    djDeckB: { effectiveBpm: null as number | null, originalBpm: null as number | null, key: null as string | null, analysisStatus: 'not_analyzed' },
  },
}));

vi.mock('../../../store', () => ({
  useStore: (selector: (state: typeof mocks.state) => unknown) => selector(mocks.state),
}));

import { DeckFacts } from './DJDeckFacts';

describe('DeckFacts', () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    (globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
    mocks.state.djDeckA = { effectiveBpm: null, originalBpm: 130.2, key: null, analysisStatus: 'available' };
    container = document.createElement('div');
    document.body.appendChild(container);
    root = createRoot(container);
  });

  afterEach(async () => {
    await act(async () => root.unmount());
    container.remove();
  });

  it('shows stored original BPM when the effective deck BPM is missing', async () => {
    await act(async () => root.render(<DeckFacts deck="A" />));
    expect(container.textContent).toContain('130.2 BPM');
  });

  it('prefers effective BPM when both deck values are available', async () => {
    mocks.state.djDeckA.effectiveBpm = 129.5;
    await act(async () => root.render(<DeckFacts deck="A" />));
    expect(container.textContent).toContain('129.5 BPM');
    expect(container.textContent).not.toContain('130.2 BPM');
  });

  it('shows Unsupported format when deck analysisStatus is unsupported and no BPM override exists', async () => {
    mocks.state.djDeckA = { effectiveBpm: null, originalBpm: null, key: null, analysisStatus: 'unsupported' };
    await act(async () => root.render(<DeckFacts deck="A" />));
    expect(container.textContent).toContain('Unsupported format');
  });
});
