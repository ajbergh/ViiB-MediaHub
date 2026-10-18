// @vitest-environment jsdom
import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { expect, it, vi } from 'vitest';
import { DJMidiMapping } from './DJMidiMapping';

it.each([
  ['DDJ-400', 'Pioneer DDJ-400 · Experimental · hardware untested · input only'],
  ['CMD MICRO', 'Behringer CMD Micro · Experimental · hardware untested · input only'],
  ['Generic Keyboard', 'No preset · use MIDI Learn'],
])('shows the connected preset status for %s', async (name, status) => {
  (globalThis as any).IS_REACT_ACT_ENVIRONMENT = true;
  localStorage.clear();
  const midi = getDJMidiService();
  const input = { id: 'experimental', name, state: 'connected', onmidimessage: null } as unknown as MIDIInput;
  vi.stubGlobal('navigator', { requestMIDIAccess: vi.fn().mockResolvedValue({
    inputs: new Map([[input.id, input]]), outputs: new Map(), sysexEnabled: true,
  }) });
  await midi.init();
  const host = document.createElement('div'); const root = createRoot(host);
  try {
    await act(async () => root.render(<DJMidiMapping onClose={() => {}} />));
    expect(host.querySelector('[data-testid="controller-profile-experimental"]')?.textContent).toContain(status);
  } finally { await act(async () => root.unmount()); midi.destroy(); localStorage.clear(); vi.unstubAllGlobals(); }
});
import { getDJMidiService } from '../../../lib/djMidi';

it('learns the same physical pad as independent actions in two banks and restores the automatic preset', async () => {
  (globalThis as any).IS_REACT_ACT_ENVIRONMENT = true;
  localStorage.clear();
  const midi = getDJMidiService();
  const input = { id: 'mixtrack', name: 'Numark MixTrack Go', manufacturer: 'Numark', state: 'connected', onmidimessage: null } as unknown as MIDIInput;
  vi.stubGlobal('navigator', { requestMIDIAccess: vi.fn().mockResolvedValue({
    inputs: new Map([[input.id, input]]), outputs: new Map(), sysexEnabled: true,
  }) });
  await midi.init();
  const container = document.createElement('div');
  const root = createRoot(container);
  const click = async (label: string) => {
    const button = Array.from(container.querySelectorAll('button')).find(button => button.textContent === label)!;
    await act(async () => button.click());
  };
  const selectBank = async (value: string) => {
    const select = container.querySelector<HTMLSelectElement>('[aria-label="Mapping bank"]')!;
    await act(async () => { select.value = value; select.dispatchEvent(new Event('change', { bubbles: true })); });
  };
  const learn = async (label: string) => {
    const row = Array.from(container.querySelectorAll('tr')).find(row => row.textContent?.includes(label))!;
    await act(async () => Array.from(row.querySelectorAll('button')).find(button => button.textContent === 'Learn')!.click());
    await act(async () => input.onmidimessage?.({ data: new Uint8Array([0x94, 0x14, 127]) } as MIDIMessageEvent));
  };
  try {
    await act(async () => root.render(<DJMidiMapping onClose={() => {}} />));
    expect(midi.getMappings().some(mapping => mapping.builtIn)).toBe(true);
    await selectBank('A0'); await click('Hot Cues'); await learn('Deck A Hot Cue 2');
    await selectBank('A2'); await click('Sampler'); await learn('Sampler Pad 3');
    const custom = midi.getMappings().filter(mapping => !mapping.builtIn);
    expect(custom).toHaveLength(2);
    expect(custom.map(mapping => [mapping.action, mapping.deck, mapping.padMode, mapping.note])).toEqual([
      ['deckA.hotCue2', 'A', 0, 0x14], ['sampler.pad3', 'A', 2, 0x14],
    ]);
    expect(JSON.parse(localStorage.getItem('viib-dj-midi-mappings')!)).toHaveLength(2);
    await selectBank('A0'); await click('Hot Cues');
    expect(container.textContent).toContain('Ch5 Note20');
    await click('Restore preset');
    expect(midi.getMappings().every(mapping => mapping.builtIn)).toBe(true);
  } finally {
    await act(async () => root.unmount());
    midi.destroy(); localStorage.clear(); vi.unstubAllGlobals();
  }
});
