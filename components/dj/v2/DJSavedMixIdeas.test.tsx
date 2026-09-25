// @vitest-environment jsdom
import React, { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { DJ_MIX_IDEAS_STORAGE_KEY, type NewDJMixIdea } from '../../../lib/djMixIdeas';
import { DJSavedMixIdeas } from './DJSavedMixIdeas';

function makeIdea(score: number): NewDJMixIdea {
  return {
    source: { trackId: 'source', title: 'Reference', artist: 'Artist A' },
    candidate: { trackId: 'candidate', title: 'Next track', artist: 'Artist B' },
    intent: 'lift', algorithmVersion: 'transition-v2', score,
    vector: {
      outgoingTailEnergy: 0.6, incomingHeadEnergy: 0.8, energyDelta: score, loudnessDeltaLu: 1,
      outgoingMixOutConfidence: 0.8, incomingMixInConfidence: 0.7,
    },
    components: [{ name: 'energy', score: 0.8, weight: 0.4, rationale: `Score ${score}` }],
    filters: { minEnergyLevel: 5 }, filterEvidence: { bpm: 124, energyLevel: 6 },
  };
}

describe('DJSavedMixIdeas', () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    (globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
    window.localStorage.clear();
    container = document.createElement('div');
    document.body.appendChild(container);
    root = createRoot(container);
  });

  afterEach(async () => {
    await act(async () => root.unmount());
    window.localStorage.clear();
    container.remove();
  });

  it('saves the recommendation evidence snapshot and removes it without deck actions', async () => {
    await act(async () => root.render(<DJSavedMixIdeas currentIdea={makeIdea(0.84)} />));
    await act(async () => container.querySelector<HTMLButtonElement>('button')!.click());
    const raw = window.localStorage.getItem(DJ_MIX_IDEAS_STORAGE_KEY)!;
    const captured = JSON.parse(raw).ideas[0];
    expect(captured.score).toBe(0.84);
    expect(captured.vector.energyDelta).toBe(0.84);
    expect(captured.candidate.trackId).toBe('candidate');
    expect(container.textContent).toContain('1 saved on this device');

    await act(async () => root.render(<DJSavedMixIdeas currentIdea={makeIdea(0.25)} />));
    const updateButton = Array.from(container.querySelectorAll('button')).find(button => button.textContent === 'Update saved idea')!;
    expect(updateButton.disabled).toBe(false);
    await act(async () => updateButton.click());
    const updated = JSON.parse(window.localStorage.getItem(DJ_MIX_IDEAS_STORAGE_KEY)!).ideas[0];
    expect(updated.score).toBe(0.25);
    expect(updated.vector.energyDelta).toBe(0.25);
    expect(updated.components[0].rationale).toBe('Score 0.25');
    await act(async () => container.querySelector<HTMLButtonElement>('[aria-label^="Remove"]')!.click());
    expect(JSON.parse(window.localStorage.getItem(DJ_MIX_IDEAS_STORAGE_KEY)!).ideas).toEqual([]);
  });
});
