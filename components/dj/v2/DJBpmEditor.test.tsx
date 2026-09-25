// @vitest-environment jsdom
import React, { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

const mocks = vi.hoisted(() => {
  const track = { id: 'song', title: 'Song', artist: 'Artist', album: 'Album', duration: 180, url: 'blob:loaded', source: 'local' as const, path: 'crate/song.wav', fileHash: 'hash-a' };
  const state = {
    djDeckA: { track, beatGrid: null, originalBpm: null },
    djDeckB: { track: null, beatGrid: null, originalBpm: null },
    setDeckAnalysis: vi.fn(),
  };
  return { state, getBpm: vi.fn(), updateBpm: vi.fn(), resetBpm: vi.fn() };
});

vi.mock('../../../store', () => {
  const useStore = (selector: (state: typeof mocks.state) => unknown) => selector(mocks.state);
  Object.assign(useStore, { getState: () => mocks.state });
  return { useStore };
});

vi.mock('../../../services/api', async importOriginal => {
  const actual = await importOriginal<typeof import('../../../services/api')>();
  return {
    ...actual,
    api: {
      ...actual.api,
      getTrackBPM: mocks.getBpm,
      updateTrackBPM: mocks.updateBpm,
      resetTrackBPM: mocks.resetBpm,
    },
  };
});

import { DJBpmEditor } from './DJBpmEditor';

const feature = (overrides: Record<string, unknown> = {}) => ({
  songId: 'song', status: 'complete', bpm: 120, bpmSource: 'manual', bpmConfidence: undefined,
  keySource: 'unknown', syncAllowed: true, sourceFingerprint: 'current-fingerprint', ...overrides,
});

describe('DJBpmEditor', () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    (globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
    mocks.state.djDeckA.track = { id: 'song', title: 'Song', artist: 'Artist', album: 'Album', duration: 180, url: 'blob:loaded', source: 'local', path: 'crate/song.wav', fileHash: 'hash-a' };
    mocks.state.djDeckA.beatGrid = null;
    mocks.state.djDeckA.originalBpm = null;
    mocks.state.setDeckAnalysis = vi.fn();
    mocks.getBpm.mockReset().mockResolvedValue(feature());
    mocks.updateBpm.mockReset().mockImplementation(async (_trackId, bpm, sourceFingerprint) => feature({ bpm, sourceFingerprint }));
    mocks.resetBpm.mockReset().mockResolvedValue(feature({ bpm: 128.25, bpmSource: 'measured', sourceFingerprint: 'current-fingerprint' }));
    container = document.createElement('div');
    document.body.appendChild(container);
    root = createRoot(container);
  });

  afterEach(async () => {
    await act(async () => root.unmount());
    container.remove();
    vi.restoreAllMocks();
  });

  it('offers scalar BPM correction on a loaded deck without a beat grid', async () => {
    await act(async () => {
      root.render(<DJBpmEditor track={mocks.state.djDeckA.track as never} deck="A" />);
      await Promise.resolve();
    });
    expect(mocks.getBpm).toHaveBeenCalledWith('song');
    expect(mocks.state.djDeckA.beatGrid).toBeNull();
    const details = container.querySelector('details');
    expect(details).not.toBeNull();
    details!.open = true;
    expect(container.textContent).toContain('do not move or re-align beat-grid timestamps');
    const input = container.querySelector<HTMLInputElement>('input[aria-label="Deck A manual BPM"]')!;
    expect(input.value).toBe('120');
    const doubleButton = container.querySelector<HTMLButtonElement>('button[aria-label="Double BPM"]')!;
    await act(async () => doubleButton.click());
    expect(input.value).toBe('240');
    const halveButton = container.querySelector<HTMLButtonElement>('button[aria-label="Halve BPM"]')!;
    await act(async () => halveButton.click());
    expect(input.value).toBe('120');

    const tapNow = vi.spyOn(performance, 'now');
    for (const time of [0, 500, 1000, 1500]) {
      tapNow.mockReturnValueOnce(time);
      await act(async () => container.querySelector<HTMLButtonElement>('button[aria-label="Tap BPM"]')!.click());
    }
    expect(input.value).toBe('120');
    const saveButton = [...container.querySelectorAll('button')].find(button => button.textContent === 'Save BPM')!;
    await act(async () => {
      saveButton.click();
      await Promise.resolve();
    });
    expect(mocks.updateBpm).toHaveBeenCalledWith('song', 120, 'current-fingerprint');
    expect(mocks.state.setDeckAnalysis).toHaveBeenCalledWith('A', { bpm: 120, bpmConfidence: null });
    expect(container.textContent).toContain('Existing beat-grid timestamps were not moved.');
  });

  it('does not apply an async response after the same song ID changes source', async () => {
    let completeUpdate: ((value: ReturnType<typeof feature>) => void) | undefined;
    mocks.updateBpm.mockImplementation(() => new Promise(resolve => { completeUpdate = resolve; }));
    const initialTrack = mocks.state.djDeckA.track;
    await act(async () => {
      root.render(<DJBpmEditor track={initialTrack as never} deck="A" />);
      await Promise.resolve();
    });
    const saveButton = [...container.querySelectorAll('button')].find(button => button.textContent === 'Save BPM')!;
    await act(async () => saveButton.click());
    mocks.state.djDeckA.track = { ...initialTrack, fileHash: 'hash-b' };
    await act(async () => {
      root.render(<DJBpmEditor track={mocks.state.djDeckA.track as never} deck="A" />);
      await Promise.resolve();
    });
    await act(async () => {
      completeUpdate?.(feature({ bpm: 121 }));
      await Promise.resolve();
    });
    expect(mocks.state.setDeckAnalysis).not.toHaveBeenCalled();
  });

  it('resets a manual BPM to the measured value', async () => {
    await act(async () => {
      root.render(<DJBpmEditor track={mocks.state.djDeckA.track as never} deck="A" />);
      await Promise.resolve();
    });
    const resetButton = [...container.querySelectorAll('button')].find(button => button.textContent === 'Reset to measured')!;
    await act(async () => {
      resetButton.click();
      await Promise.resolve();
    });
    expect(mocks.resetBpm).toHaveBeenCalledWith('song', 'current-fingerprint');
    expect(mocks.state.setDeckAnalysis).toHaveBeenCalledWith('A', { bpm: 128.25, bpmConfidence: null });
    expect(container.querySelector<HTMLInputElement>('input[aria-label="Deck A manual BPM"]')!.value).toBe('128.25');
  });
});
