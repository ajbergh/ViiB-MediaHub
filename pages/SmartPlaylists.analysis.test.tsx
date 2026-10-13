// @vitest-environment jsdom
import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { expect, it, vi } from 'vitest';
const mocks = vi.hoisted(() => ({ state: {} as any, generate: vi.fn() }));
vi.mock('../store', () => ({ useStore: Object.assign((selector?: any) => selector ? selector(mocks.state) : mocks.state, { getState: () => mocks.state }) }));
vi.mock('react-router', () => ({ useNavigate: () => vi.fn() }));
vi.mock('../services/api', () => ({ api: { generateSmartPlaylist: mocks.generate, getSemanticStatus: vi.fn().mockResolvedValue({ state: 'unavailable' }) } }));
import { SmartPlaylists } from './SmartPlaylists';

it('shows resolved queue tempo transiently and discards a retired-session response', async () => {
  (globalThis as any).IS_REACT_ACT_ENVIRONMENT = true;
  mocks.state = {
    spotifySessionGeneration: 1, aiDjPrompt: 'party dance', aiDjGeneratedSongs: [], aiDjIsLoading: false,
    aiDjMode: true, aiDjPersona: 'FlowMaster', aiDjTargetDurationMinutes: 15, aiDjFlowStrictness: 60,
    aiDjDiscoverMode: 'balanced', aiDjAvoidRecentlyHours: 24, aiDjOnePerArtist: true, aiDjSource: 'local',
    aiDjPhases: [], showToast: vi.fn(), playSong: vi.fn(), createPlaylist: vi.fn(),
    setAIDJGeneratedSongs: vi.fn((songs: any[]) => { mocks.state.aiDjGeneratedSongs = songs; }),
    setAIDJFilter: vi.fn(), setAIDJValidation: vi.fn(), setAIDJIsLoading: vi.fn(),
    setAIDJDJResult: vi.fn((_prompt: string, songs: any[]) => { mocks.state.aiDjGeneratedSongs = songs; }),
  };
  const response = {
    filter: {}, dj: { plan: { phases: [] }, phases: [] },
    songs: [{ id: 'song', title: 'Track', artist: 'Artist', album: 'Album', filePath: '/api/audio/song', duration: 240, addedAt: 1, bpm: 199,
      analysis: { songId: 'song', status: 'complete', bpm: 135.25, bpmSource: 'spotify', keySource: 'unknown', syncAllowed: false, sourceFingerprint: 'fixture' } }],
  };
  mocks.generate.mockResolvedValueOnce(response);
  const container = document.createElement('div');
  const root = createRoot(container);
  const render = () => act(async () => root.render(<SmartPlaylists />));
  const generate = () => [...container.querySelectorAll('button')].find(button => button.textContent?.includes('Start Set'))!;
  try {
    await render();
    expect(generate()).toBeDefined();
    await act(async () => generate().click());
    expect(container.textContent).toContain('135.25 · Spotify');
    // Persisted/generated Song copies deliberately omit the transient analysis.
    expect(mocks.state.aiDjGeneratedSongs[0]).not.toHaveProperty('analysis');
    mocks.state.spotifySessionGeneration++;
    await render();
    expect(container.textContent).not.toContain('135.25 · Spotify');
    expect(container.textContent).toContain('Unknown');
    let release!: (value: typeof response) => void;
    mocks.generate.mockImplementationOnce(() => new Promise(resolve => { release = resolve; }));
    await act(async () => generate().click());
    mocks.state.spotifySessionGeneration++;
    await act(async () => release(response));
    expect(mocks.state.aiDjGeneratedSongs).toEqual([]);
    expect(mocks.state.setAIDJDJResult).toHaveBeenCalledTimes(1);
  } finally { await act(async () => root.unmount()); }
});

it.each([true, false])('keeps the newest same-session generation authoritative (older finishes first: %s)', async olderFirst => {
  (globalThis as any).IS_REACT_ACT_ENVIRONMENT = true;
  mocks.generate.mockReset();
  mocks.state = {
    spotifySessionGeneration: 1, aiDjPrompt: 'party dance', aiDjGeneratedSongs: [], aiDjIsLoading: false,
    aiDjMode: true, aiDjPersona: 'FlowMaster', aiDjTargetDurationMinutes: 15, aiDjFlowStrictness: 60,
    aiDjDiscoverMode: 'balanced', aiDjAvoidRecentlyHours: 24, aiDjOnePerArtist: true, aiDjSource: 'local',
    aiDjPhases: [], showToast: vi.fn(), playSong: vi.fn(), createPlaylist: vi.fn(),
    setAIDJGeneratedSongs: vi.fn((songs: any[]) => { mocks.state.aiDjGeneratedSongs = songs; }),
    setAIDJFilter: vi.fn(), setAIDJValidation: vi.fn(), setAIDJIsLoading: vi.fn(),
    setAIDJDJResult: vi.fn((_prompt: string, songs: any[]) => { mocks.state.aiDjGeneratedSongs = songs; }),
  };
  let resolveOld!: (value: any) => void;
  let resolveNew!: (value: any) => void;
  mocks.generate.mockImplementationOnce(() => new Promise(resolve => { resolveOld = resolve; }));
  mocks.generate.mockImplementationOnce(() => new Promise(resolve => { resolveNew = resolve; }));
  const response = (id: string) => ({ filter: {}, dj: { plan: { phases: [] }, phases: [] }, songs: [
    { id, title: id, artist: 'Artist', album: 'Album', filePath: `/api/audio/${id}`, duration: 240, addedAt: 1,
      analysis: { songId: id, status: 'complete', bpm: 121.75, bpmSource: 'manual', keySource: 'unknown' } },
  ] });
  const container = document.createElement('div');
  const root = createRoot(container);
  const generate = () => [...container.querySelectorAll('button')].find(button => button.textContent?.includes('Start Set'))!;
  try {
    await act(async () => root.render(<SmartPlaylists />));
    await act(async () => generate().click());
    await act(async () => generate().click());
    expect(mocks.generate).toHaveBeenCalledTimes(2);
    if (olderFirst) {
      await act(async () => resolveOld(response('old')));
      expect(mocks.state.setAIDJIsLoading).not.toHaveBeenCalledWith(false);
      expect(mocks.state.setAIDJDJResult).not.toHaveBeenCalled();
    }
    await act(async () => resolveNew(response('new')));
    if (!olderFirst) await act(async () => resolveOld(response('old')));
    expect(mocks.state.aiDjGeneratedSongs.map((song: any) => song.id)).toEqual(['new']);
    expect(mocks.state.setAIDJDJResult).toHaveBeenCalledTimes(1);
    expect(mocks.state.setAIDJIsLoading.mock.calls.filter(([loading]: [boolean]) => !loading)).toHaveLength(1);
    expect(mocks.state.showToast).toHaveBeenCalledTimes(1);
  } finally { await act(async () => root.unmount()); }
});
