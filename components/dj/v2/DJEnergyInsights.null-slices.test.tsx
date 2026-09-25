// @vitest-environment jsdom
import React, { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

const mocks = vi.hoisted(() => ({
  getEnergy: vi.fn(),
  getRecommendations: vi.fn(),
  state: {
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
});
