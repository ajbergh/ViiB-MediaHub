// @vitest-environment jsdom
import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { expect, it, vi } from 'vitest';

const mocks = vi.hoisted(() => ({ state: {} as any, save: vi.fn() }));
vi.mock('../../../store', () => ({ useStore: Object.assign((selector: any) => selector(mocks.state), { getState: () => mocks.state }) }));
vi.mock('../../../services/api', () => ({ api: {
  getAnalysisCues: vi.fn().mockResolvedValue({ generatorVersion: 'fixture', sourceFingerprint: 'fixture', generatedCandidates: [] }),
  saveDJHotCues: mocks.save,
} }));
vi.mock('../../../hooks/useDJAudioEngine', () => ({ useDJAudioEngineActions: () => ({ seek: vi.fn() }) }));
import { DJAnalysisCueEditor } from './DJAnalysisCueEditor';

it('snaps only reviewed timing and rechecks provenance when an edit is clicked', async () => {
  (globalThis as any).IS_REACT_ACT_ENVIRONMENT = true;
  const deck = { track: { id: 'one' }, position: 0.3, beatGrid: [0, 0.5, 1], beatGridSource: 'spotify', beatGridLocked: true,
    hotCues: [{ slot: 1, position: 0.3, label: 'Cue', color: '', origin: 'manual' }] };
  mocks.save.mockResolvedValue(undefined);
  mocks.state = { djDeckA: deck, loadHotCues: vi.fn((_deck: string, cues: any[]) => { deck.hotCues = cues; }) };
  const container = document.createElement('div');
  const root = createRoot(container);
  const render = () => act(async () => root.render(<DJAnalysisCueEditor trackID="one" deck="A" embedded />));
  const select = () => container.querySelector<HTMLSelectElement>('[aria-label="Cue quantize mode"]')!;
  const button = (text: string) => [...container.querySelectorAll('button')].find(item => item.textContent === text)!;
  try {
    await render();
    expect(select().disabled).toBe(true);
    expect(button('Snap').disabled).toBe(true);
    deck.beatGridSource = 'manual';
    await render();
    await act(async () => { select().value = 'beat'; select().dispatchEvent(new Event('change', { bubbles: true })); });
    expect(button('Snap').disabled).toBe(false);
    // A retired/unreviewed grid must not be used even before React rerenders.
    deck.beatGridSource = 'inferred-from-meter';
    await act(async () => button('Snap').click());
    expect(mocks.state.loadHotCues).not.toHaveBeenCalled();
    await act(async () => button('Move').click());
    expect(deck.hotCues[0].position).toBe(0.3);
    deck.beatGridSource = 'manual';
    await render();
    await act(async () => button('Snap').click());
    expect(deck.hotCues[0].position).toBe(0.5);
    deck.beatGridLocked = false;
    await render();
    expect(select().disabled).toBe(true);
    expect(button('Snap').disabled).toBe(true);
  } finally { await act(async () => root.unmount()); }
});
