// @vitest-environment jsdom
import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { expect, it, vi } from 'vitest';
const mocks = vi.hoisted(() => ({ save: vi.fn(), reset: vi.fn() }));
vi.mock('../services/api', () => ({ api: { updateTrackMetadataField: mocks.save, resetTrackMetadataField: mocks.reset } }));
import { ManualAudioMetadata } from './ManualAudioMetadata';
import type { TrackAnalysisFeature } from '../services/trackAnalysisContracts';
const data = { sourceFingerprint: 'fp', effectiveFields: [{ key: 'time_signature', selected: { value: 3, source: 'manual' } }] } as TrackAnalysisFeature;
it('saves and resets source-qualified values and reloads authoritative data', async () => {
  (globalThis as any).IS_REACT_ACT_ENVIRONMENT = true;
  mocks.save.mockResolvedValue(undefined); mocks.reset.mockResolvedValue(undefined);
  const host = document.createElement('div'); const root = createRoot(host); const reload = vi.fn();
  const events = vi.fn(); window.addEventListener('library_updated', events);
  try {
    await act(async () => root.render(<ManualAudioMetadata songId="a" data={data} onReload={reload} />));
    expect(host.textContent).toContain('3 beats per bar · manual');
    await act(async () => host.querySelector('form')!.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true })));
    expect(mocks.save).toHaveBeenCalledWith('a', 'time_signature', 3, 'fp');
    await act(async () => (host.querySelector('button[type="button"]') as HTMLButtonElement).click());
    expect(mocks.reset).toHaveBeenCalledWith('a', 'time_signature', 'fp');
    expect(reload).not.toHaveBeenCalled();
    expect(events).toHaveBeenCalledTimes(2);
    expect((events.mock.calls[0][0] as CustomEvent).detail).toEqual({ source: 'manual_audio_metadata', songId: 'a', sourceFingerprint: 'fp', field: 'time_signature' });
  } finally { window.removeEventListener('library_updated', events); await act(async () => root.unmount()); }
});
it('ignores a late save after the source component has been replaced', async () => {
  (globalThis as any).IS_REACT_ACT_ENVIRONMENT = true;
  let resolve!: () => void; mocks.save.mockImplementationOnce(() => new Promise<void>(done => { resolve = done; }));
  const host = document.createElement('div'); const root = createRoot(host); const reload = vi.fn();
  const events = vi.fn(); window.addEventListener('library_updated', events);
  try {
    await act(async () => root.render(<ManualAudioMetadata key="a:fp" songId="a" data={data} onReload={reload} />));
    await act(async () => host.querySelector('form')!.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true })));
    await act(async () => root.render(<ManualAudioMetadata key="b:next" songId="b" data={{ ...data, sourceFingerprint: 'next' }} onReload={reload} />));
    await act(async () => resolve());
    expect(reload).not.toHaveBeenCalled();
    expect(events).not.toHaveBeenCalled();
  } finally { window.removeEventListener('library_updated', events); await act(async () => root.unmount()); }
});
it('disables editing without a current file identity', async () => {
  (globalThis as any).IS_REACT_ACT_ENVIRONMENT = true;
  const host = document.createElement('div'); const root = createRoot(host);
  try {
    await act(async () => root.render(<ManualAudioMetadata songId="a" data={{} as TrackAnalysisFeature} onReload={() => {}} />));
    expect(host.querySelector('fieldset')?.disabled).toBe(true);
  } finally { await act(async () => root.unmount()); }
});

it('keeps a rejected write visible without applying an optimistic value', async () => {
  (globalThis as any).IS_REACT_ACT_ENVIRONMENT = true;
  mocks.save.mockRejectedValueOnce(new Error('412'));
  const host = document.createElement('div'); const root = createRoot(host); const reload = vi.fn();
  const events = vi.fn(); window.addEventListener('library_updated', events);
  try {
    await act(async () => root.render(<ManualAudioMetadata songId="a" data={data} onReload={reload} />));
    await act(async () => host.querySelector('form')!.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true })));
    expect(host.querySelector('[role="alert"]')?.textContent).toContain('Reload audio metadata');
    expect(reload).not.toHaveBeenCalled();
    expect(events).not.toHaveBeenCalled();
    expect(host.querySelector('fieldset')?.disabled).toBe(false);
  } finally { window.removeEventListener('library_updated', events); await act(async () => root.unmount()); }
});
