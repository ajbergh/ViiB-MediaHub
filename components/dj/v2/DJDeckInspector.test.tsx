// @vitest-environment jsdom
import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { expect, it, vi } from 'vitest';
const state = vi.hoisted(() => ({ djDeckA: { track: { id: 'a' }, beatGrid: [] }, djDeckB: { track: { id: 'b' }, beatGrid: [] } }));
vi.mock('../../../store', () => ({ useStore: (select: any) => select(state) }));
vi.mock('../../SongAudioMetadata', () => ({ SongAudioMetadata: ({ songId }: { songId: string }) => <div data-audio-song={songId}>Metadata {songId}</div> }));
vi.mock('./DJEnergyInsights', () => ({ DJEnergyInsights: () => null }));
vi.mock('./DJKeyVerificationKeyboard', () => ({ DJKeyVerificationKeyboard: () => null }));
vi.mock('./DJBpmEditor', () => ({ DJBpmEditor: () => null }));
vi.mock('./DJAnalysisCueEditor', () => ({ DJAnalysisCueEditor: () => null }));
vi.mock('./DJBeatGridEdit', () => ({ DJBeatGridEdit: () => null }));
vi.mock('./DJBeatGridStatus', () => ({ DJBeatGridStatus: () => null }));
import { DJDeckInspector } from './DJDeckInspector';
it('loads audio only for the visible selected deck and replaces track context', async () => {
 (globalThis as any).IS_REACT_ACT_ENVIRONMENT = true;
 const host = document.createElement('div'); const root = createRoot(host);
 const render = (open: boolean, deck: 'A' | 'B', tab: 'audio' | 'analysis' = 'audio') => root.render(<DJDeckInspector deck={deck} open={open} tab={tab} onTabChange={() => {}} onClose={() => {}} triggerRef={{ current: null }} />);
 try {
  await act(async () => render(false,'A'));
  expect(host.querySelector('[data-audio-song]')).toBeNull();
  await act(async () => render(true,'A'));
  expect(host.querySelector('[data-audio-song]')?.getAttribute('data-audio-song')).toBe('a');
  expect(host.textContent).toContain('does not establish beat alignment');
  await act(async () => render(true,'B'));
  expect(host.querySelector('[data-audio-song]')?.getAttribute('data-audio-song')).toBe('b');
  await act(async () => render(true,'B','analysis'));
  expect(host.querySelector('[data-audio-song]')).toBeNull();
 } finally { await act(async () => root.unmount()); }
});
