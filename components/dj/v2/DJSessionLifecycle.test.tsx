// @vitest-environment jsdom

import React, { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';

const mocks = vi.hoisted(() => ({
  createPlaylist: vi.fn(),
  sessionGeneration: 0,
  setDeckAnalysisStatus: vi.fn(),
  energyFeatures: vi.fn(),
  recommendations: vi.fn(),
  state: {
    songs: [{ id: 'reference', title: 'Reference', artist: 'Fixture', album: 'Album', duration: 10, filePath: '/reference.wav', path: '/reference.wav', addedAt: 1 }],
    playlists: [],
    spotifySessionGeneration: 0,
    djDeckA: { hotCues: [], analysisStatus: 'available', track: { id: 'reference' }, duration: 10, position: 0, isPlaying: false, beatGrid: null, downbeatIndices: null, beatGridLocked: false, beatGridSource: 'unknown' },
    djDeckB: { hotCues: [], analysisStatus: 'not_analyzed', track: null, duration: 0, position: 0, isPlaying: false, beatGrid: null, downbeatIndices: null, beatGridLocked: false, beatGridSource: 'unknown' },
    djMixer: { crossfader: 0, masterCueEnabled: false, autoGainA: false, autoGainB: false },
  },
}));

vi.mock('../../../store', () => {
  const useStore = (selector: (state: typeof mocks.state & { createPlaylist: typeof mocks.createPlaylist; spotifySessionGeneration: number; setDeckAnalysisStatus: typeof mocks.setDeckAnalysisStatus; setHotCue: ReturnType<typeof vi.fn>; retireSpotifyPlayback: ReturnType<typeof vi.fn>; djMixer: typeof mocks.state.djMixer }) => unknown) =>
    selector({ ...mocks.state, createPlaylist: mocks.createPlaylist, spotifySessionGeneration: mocks.sessionGeneration, setDeckAnalysisStatus: mocks.setDeckAnalysisStatus, setHotCue: vi.fn(), retireSpotifyPlayback: vi.fn(), djMixer: mocks.state.djMixer });
  Object.assign(useStore, { getState: () => ({ ...mocks.state, spotifySessionGeneration: mocks.sessionGeneration, djMixer: mocks.state.djMixer, setHotCue: vi.fn(), setDeckAnalysisStatus: mocks.setDeckAnalysisStatus, retireSpotifyPlayback: vi.fn() }) });
  return { useStore };
});

vi.mock('../../../hooks/useDJAudioEngine', () => ({ useDJAudioEngineActions: () => ({ loadTrack: vi.fn() }) }));
vi.mock('../../../lib/djAudio', () => ({ getDJAudioEngine: () => ({ initialized: false, getHeadphoneOutputDeviceId: () => '', getMainOutputDeviceId: () => '', getDeckLoadedTrackId: () => null, isLoaded: () => false, isScratching: () => false, isPlaying: () => false, isDeckLoading: () => false, getCueEnabled: () => false, getMasterCueEnabled: () => false }) }));
vi.mock('../../../lib/testMixPreviewGuard', () => ({ hasSeparateHeadphoneRoute: () => false, isPreviewDeckOffAir: () => false, isPristineEmptyPreviewDeck: () => false, isRestorableOccupiedPreviewDeck: () => false, stillOwnsDeckSnapshot: () => false, stillOwnsPreviewRoute: () => false }));
vi.mock('../../../lib/mixNextAcceptanceGuard', () => ({ canAcceptMixNextCandidate: () => false, stillOwnsMixNextAcceptance: () => false }));
vi.mock('../../../services/api', async importOriginal => {
  const actual = await importOriginal<typeof import('../../../services/api')>();
  return { ...actual, api: { ...actual.api, getTrackEnergyFeatures: mocks.energyFeatures, getTrackTransitionRecommendations: mocks.recommendations } };
});

import { DJSaveResultsPlaylist } from './DJSaveResultsPlaylist';
import { DJEnergyInsights } from './DJEnergyInsights';

let root: Root;
let host: HTMLDivElement;
const waitForRecommendation = async () => {
  for (let i = 0; i < 20 && mocks.recommendations.mock.calls.length === 0; i++) await new Promise(resolve => setTimeout(resolve, 0));
};

beforeEach(() => {
  (globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
  mocks.sessionGeneration = 1;
  mocks.state.spotifySessionGeneration = 1;
  mocks.state.djDeckA.track = mocks.state.songs[0];
  mocks.state.djDeckA.analysisStatus = 'available';
  mocks.state.djDeckB.track = null;
  mocks.state.djDeckB.analysisStatus = 'not_analyzed';
  mocks.setDeckAnalysisStatus.mockReset();
  mocks.energyFeatures.mockReset().mockResolvedValue({ songId: 'reference', integratedLufs: -14, truePeakDbfs: -1, energy: [], sections: [], cueSuggestions: [], algorithmVersion: 'energy-structure-v1' });
  mocks.recommendations.mockReset().mockResolvedValue({
    songId: 'reference', intent: 'hold', algorithmVersion: 'fixture', filters: {}, candidatesBeforeFilters: 1, candidatesAfterFilters: 1,
    recommendations: [{ songId: 'candidate', title: 'Candidate', artist: 'Fixture', score: .8, intent: 'hold', vector: { outgoingTailEnergy: .5, incomingHeadEnergy: .5, energyDelta: 0, loudnessDeltaLu: 0, outgoingMixOutConfidence: 0, incomingMixInConfidence: 0 }, components: [], filterEvidence: {} }],
  });
  mocks.createPlaylist.mockReset().mockResolvedValue({ id: 'created' });
  host = document.createElement('div');
  document.body.appendChild(host);
  root = createRoot(host);
});

afterEach(async () => {
  await act(async () => root.unmount());
  host.remove();
});

async function openDraft() {
  await act(async () => root.render(<DJSaveResultsPlaylist songIds={['reference', 'candidate']} />));
  await act(async () => host.querySelector('button')!.click());
  const input = host.querySelector('input')!;
  await act(async () => {
    Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!.call(input, 'Old account draft');
    input.dispatchEvent(new Event('input', { bubbles: true }));
  });
  expect(host.querySelector('form')).not.toBeNull();
}

it('clears the saved draft synchronously when disconnect advances session generation', async () => {
  await openDraft();

  // Mirrors logoutSpotify's authoritative state transition, without an
  // intervening render/effect flush between changing the store and the assertion.
  mocks.sessionGeneration++;
  mocks.state.spotifySessionGeneration = mocks.sessionGeneration;
  expect(host.querySelector('form')).not.toBeNull(); // Existing DOM before subscription rerender.

  await act(async () => root.render(<DJSaveResultsPlaylist songIds={['reference', 'next-account-song']} />));
  expect(host.querySelector('form')).toBeNull();
  expect(host.textContent).not.toContain('Old account draft');
  expect(host.textContent).toContain('Save 2 results as playlist');
});

it('does not submit old captured IDs after a session switch', async () => {
  await openDraft();
  mocks.sessionGeneration++;
  mocks.state.spotifySessionGeneration = mocks.sessionGeneration;
  await act(async () => root.render(<DJSaveResultsPlaylist songIds={['reference', 'next-account-song']} />));

  expect(host.querySelector('form')).toBeNull();
  expect(mocks.createPlaylist).not.toHaveBeenCalled();
});

it('does not show a prior account save completion after the store switches accounts', async () => {
  let finish!: (result: unknown) => void;
  mocks.createPlaylist.mockImplementationOnce(() => new Promise(resolve => { finish = resolve; }));
  await openDraft();
  await act(async () => host.querySelector('form')!.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true })));
  expect(mocks.createPlaylist).toHaveBeenCalledWith('Old account draft', ['reference', 'candidate']);

  mocks.sessionGeneration++;
  mocks.state.spotifySessionGeneration = mocks.sessionGeneration;
  await act(async () => root.render(<DJSaveResultsPlaylist songIds={['reference', 'next-account-song']} />));
  await act(async () => finish({ id: 'old-account-result' }));
  expect(host.textContent).not.toContain('Old account draft');
  expect(host.textContent).not.toContain('Saved 2 tracks');
  expect(host.querySelector('form')).toBeNull();
});

it('renders a candidate draft and drops it when session generation changes', async () => {
  await act(async () => root.render(<DJEnergyInsights trackID="reference" deck="A" />));
  await act(async () => { await waitForRecommendation(); });
  expect(mocks.recommendations).toHaveBeenCalled();
  expect(mocks.energyFeatures).toHaveBeenCalledWith('reference');
  expect(host.textContent).toContain('0 advisory cues');
  expect(host.textContent).toContain('Recommended next: Candidate');
  const build = Array.from(host.querySelectorAll('summary')).find(node => node.textContent?.includes('Build a playlist'))!;
  await act(async () => build.click());
  const save = Array.from(host.querySelectorAll('button')).find(button => button.textContent?.includes('Save 2 results'))!;
  await act(async () => save.click());
  const input = host.querySelector('input')!;
  await act(async () => {
    Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!.call(input, 'Account one draft');
    input.dispatchEvent(new Event('input', { bubbles: true }));
  });
  expect(host.querySelector('form')).not.toBeNull();

  mocks.sessionGeneration++;
  mocks.state.spotifySessionGeneration = mocks.sessionGeneration;
  mocks.recommendations.mockResolvedValueOnce({ songId: 'reference', recommendations: [] });
  await act(async () => root.render(<DJEnergyInsights trackID="reference" deck="A" />));
  await act(async () => {
    await new Promise(resolve => setTimeout(resolve, 0));
  });
  await act(async () => { await Promise.resolve(); await Promise.resolve(); });
  expect(host.querySelector('form')).toBeNull();
  expect(host.textContent).not.toContain('Account one draft');
  expect(host.textContent).not.toContain('Recommended next: Candidate');
});