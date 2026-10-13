// @vitest-environment jsdom

/** Tests and fixtures for DJEnergy Insights null-slices behavior. */

import React, { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

const mocks = vi.hoisted(() => ({
  getEnergy: vi.fn(),
  getRecommendations: vi.fn(),
  state: {
    spotifySessionGeneration: 0,
    djDeckA: { hotCues: [], analysisStatus: 'not_analyzed', track: null, duration: 0, position: 0, isPlaying: false },
    djDeckB: { hotCues: [], analysisStatus: 'not_analyzed', track: null, duration: 0, position: 0, isPlaying: false },
    djMixer: { crossfader: 0, masterCueEnabled: false, autoGainA: false, autoGainB: false },
    songs: [],
    playlists: [],
    setHotCue: vi.fn(),
  },
}));

vi.mock('../../../store', () => {
  const useStore = (selector: (state: typeof mocks.state) => unknown) => selector(mocks.state);
  Object.assign(useStore, { getState: () => mocks.state });
  return { useStore };
});

vi.mock('../../../hooks/useDJAudioEngine', () => ({
  useDJAudioEngineActions: () => ({ loadTrack: vi.fn() }),
}));

vi.mock('../../../lib/djAudio', () => ({
  getDJAudioEngine: () => ({
    initialized: false,
    getHeadphoneOutputDeviceId: () => '',
    getMainOutputDeviceId: () => '',
    getDeckLoadedTrackId: () => null,
    isLoaded: () => false,
    isScratching: () => false,
    isPlaying: () => false,
    isDeckLoading: () => false,
    getCueEnabled: () => false,
    getMasterCueEnabled: () => false,
  }),
}));

vi.mock('../../../lib/testMixPreviewGuard', () => ({
  hasSeparateHeadphoneRoute: () => false,
  isPreviewDeckOffAir: () => false,
  isPristineEmptyPreviewDeck: () => false,
  isRestorableOccupiedPreviewDeck: () => false,
  stillOwnsDeckSnapshot: () => false,
  stillOwnsPreviewRoute: () => false,
}));

vi.mock('../../../lib/mixNextAcceptanceGuard', () => ({
  canAcceptMixNextCandidate: () => false,
  stillOwnsMixNextAcceptance: () => false,
}));

vi.mock('../../../services/api', async importOriginal => {
  const actual = await importOriginal<typeof import('../../../services/api')>();
  return {
    ...actual,
    api: {
      ...actual.api,
      getTrackEnergyFeatures: mocks.getEnergy,
      getTrackTransitionRecommendations: mocks.getRecommendations,
    },
  };
});

import { DJEnergyInsights } from './DJEnergyInsights';

describe('DJEnergyInsights energy response normalization', () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    (globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
    mocks.state.spotifySessionGeneration = 0;
    mocks.state.djDeckA.analysisStatus = 'not_analyzed';
    mocks.getEnergy.mockReset().mockResolvedValue({
      songId: 'song', integratedLufs: -12, truePeakDbfs: -1, energy: null, sections: null,
      cueSuggestions: null, algorithmVersion: 'energy-structure-v1',
    });
    mocks.getRecommendations.mockReset().mockResolvedValue({ recommendations: [] });
    container = document.createElement('div');
    document.body.appendChild(container);
    root = createRoot(container);
  });

  afterEach(async () => {
    await act(async () => root.unmount());
    container.remove();
    vi.useRealTimers();
  });

  it('renders an analyzed track when energy, section, and cue arrays are missing', async () => {
    await act(async () => {
      root.render(<DJEnergyInsights trackID="song" deck="A" />);
      await new Promise(resolve => setTimeout(resolve, 0));
    });

    expect(container.textContent).toContain('Track not analysed yet.');

    await act(async () => {
      mocks.state.djDeckA.analysisStatus = 'available';
      root.render(<DJEnergyInsights trackID="song" deck="A" />);
      await new Promise(resolve => setTimeout(resolve, 0));
    });

    expect(mocks.getEnergy).toHaveBeenCalledWith('song');
    expect(container.textContent).toContain('0 advisory cues');
    expect(container.querySelector('[aria-label="Measured track energy"]')).not.toBeNull();
  });
  it('rejects an old session response before React effect cleanup and reloads for the new generation', async () => {
    let settle!: (value: unknown) => void;
    mocks.getRecommendations.mockImplementationOnce(() => new Promise(resolve => { settle = resolve; }));
    mocks.state.djDeckA.analysisStatus = 'available';
    await act(async () => { root.render(<DJEnergyInsights trackID="song" deck="A" />); });
    expect(mocks.getRecommendations).toHaveBeenCalledTimes(1);
    // Change the authoritative store before rendering the subscription update.
    mocks.state.spotifySessionGeneration = 1;
    await act(async () => { settle({ recommendations: [{ title: 'Old account candidate' }] }); });
    expect(container.textContent).not.toContain('Old account candidate');
    await act(async () => { root.render(<DJEnergyInsights trackID="song" deck="A" />); });
    expect(mocks.getRecommendations).toHaveBeenCalledTimes(2);
    expect(container.textContent).toContain('0 advisory cues');
  });

  it('refreshes recommendations for candidate Energy Level changes and discards old responses', async () => {
    let settle!: (value: unknown) => void;
    mocks.state.djDeckA.analysisStatus = 'available';
    mocks.getRecommendations.mockImplementationOnce(() => new Promise(resolve => { settle = resolve; }));
    await act(async () => root.render(<DJEnergyInsights trackID="song" deck="A" />));
    const change = (field: string) => window.dispatchEvent(new CustomEvent('library_updated', { detail: { source: 'manual_audio_metadata', songId: 'candidate', sourceFingerprint: 'fp', field } }));
    await act(async () => change('time_signature'));
    expect(mocks.getRecommendations).toHaveBeenCalledTimes(1);
    await act(async () => { change('local_energy_level'); settle({ recommendations: [{ title: 'Pre-edit candidate' }] }); });
    expect(mocks.getRecommendations).toHaveBeenCalledTimes(2);
    expect(mocks.getEnergy).toHaveBeenCalledTimes(1);
    expect(container.textContent).not.toContain('Pre-edit candidate');
  });

  it('re-arms score expiry timers beyond the browser timeout maximum', async () => {
    const maxDelay = 2_147_483_647;
    const start = new Date('2026-10-06T12:00:00.000Z');
    const expiry = new Date(start.getTime() + maxDelay * 2 + 1000).toISOString();
    vi.useFakeTimers();
    vi.setSystemTime(start);
    mocks.state.djDeckB.analysisStatus = 'available';
    mocks.getRecommendations
      .mockResolvedValueOnce({ songId: 'song', recommendations: [] })
      .mockResolvedValueOnce({
        songId: 'song', intent: 'hold', algorithmVersion: 'fixture', filters: {},
        candidatesBeforeFilters: 1, candidatesAfterFilters: 1,
        recommendations: [{
          songId: 'candidate', title: 'Long-lived candidate', artist: 'Fixture', score: 0.8,
          filterEvidence: { spotifyScoreMetric: 'energy', spotifyScore: {
            value: 0.5, stale: false, expiresAt: expiry, retrievedAt: start.toISOString(), endpoint: 'audio_features',
          } },
        }],
      })
      .mockResolvedValue({ songId: 'song', recommendations: [] });

    await act(async () => {
      root.render(<DJEnergyInsights trackID="song" />);
      await Promise.resolve();
    });
    const metric = container.querySelector('[aria-label="Mix Next Spotify score"]') as HTMLSelectElement;
    await act(async () => {
      Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, 'value')!.set!.call(metric, 'energy');
      metric.dispatchEvent(new Event('change', { bubbles: true }));
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(mocks.getRecommendations).toHaveBeenCalledTimes(2);
    expect(container.textContent).toContain('Recommended next: Long-lived candidate');

    await act(async () => { await vi.advanceTimersByTimeAsync(maxDelay); });
    expect(container.textContent).toContain('Recommended next: Long-lived candidate');
    expect(container.textContent).not.toContain('Spotify score evidence expired');
    expect(mocks.getRecommendations).toHaveBeenCalledTimes(2);

    await act(async () => { await vi.advanceTimersByTimeAsync(maxDelay); });
    expect(container.textContent).toContain('Recommended next: Long-lived candidate');
    expect(mocks.getRecommendations).toHaveBeenCalledTimes(2);

    await act(async () => { await vi.advanceTimersByTimeAsync(1001); });
    expect(container.textContent).toContain('Spotify score evidence expired. Updating candidates');
    expect(mocks.getRecommendations).toHaveBeenCalledTimes(3);
  });

});
