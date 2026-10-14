// @vitest-environment jsdom
/** Checks collection actions, duplicate occurrences, and retiring-session responses. */
import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { MemoryRouter, Route, Routes } from 'react-router';
import { afterEach, expect, it, vi } from 'vitest';
import { useStore } from '../store';

const mocks = vi.hoisted(() => ({ album: vi.fn(), playlist: vi.fn(), backend: vi.fn() }));
vi.mock('../services/spotifyAlbum', () => ({ fetchSpotifyAlbum: mocks.album }));
vi.mock('../services/spotifyPlaylist', () => ({ fetchSpotifyPlaylist: mocks.playlist }));
vi.mock('../services/spotifyBackend', () => ({ backendSpotifyFetch: mocks.backend, assertSpotifySession: vi.fn() }));
import { SpotifyAlbumDetail } from './SpotifyAlbumDetail';
import { SpotifyPlaylistDetail } from './SpotifyPlaylistDetail';

const initial = useStore.getState();
afterEach(() => { useStore.setState(initial, true); vi.restoreAllMocks(); vi.clearAllMocks(); });

it.each(['album', 'playlist'] as const)('%s actions preserve collection order and duplicate row positions', async kind => {
  (globalThis as any).IS_REACT_ACT_ENVIRONMENT = true;
  const play = vi.fn(), queue = vi.fn(), context = vi.fn();
  useStore.setState({ spotifyConnected: true, spotifySessionGeneration: 1, playSong: play, addToQueue: queue, openContextMenu: context, showToast: vi.fn(), addLog: vi.fn() });
  const track = { id: 'duplicate', name: 'Duplicate Track', duration_ms: 120000, artists: [{ id: 'artist', name: 'Artist' }], album: { name: 'Collection', images: [] } };
  const collection = { id: 'collection', name: 'Collection', images: [], artists: track.artists, release_date: '2024', owner: { display_name: 'Owner' }, external_urls: { spotify: '#' }, tracks: { total: 2, items: kind === 'album' ? [track, track] : [{ track }, { track }] } };
  mocks.album.mockResolvedValue(collection);
  mocks.backend.mockResolvedValue({ ok: true, status: 200 });
  mocks.playlist.mockResolvedValue(collection);
  const host = document.createElement('div');
  const root = createRoot(host);
  const Component = kind === 'album' ? SpotifyAlbumDetail : SpotifyPlaylistDetail;
  try {
    await act(async () => root.render(<MemoryRouter initialEntries={['/collection/id']}><Routes><Route path="/collection/:id" element={<Component/>}/></Routes></MemoryRouter>));
    const click = (label: string) => act(async () => host.querySelector<HTMLButtonElement>(`[aria-label="${label}"]`)!.click());
    await click(`Play ${kind}`);
    expect(play.mock.lastCall?.[1].map((song: any) => song.spotifyId)).toEqual(['duplicate', 'duplicate']);
    await click('Add to queue');
    expect(queue.mock.lastCall?.[0]).toHaveLength(2);
    await click(`Shuffle ${kind}`);
    expect(play.mock.lastCall?.[1]).toHaveLength(2);
    const more = host.querySelectorAll<HTMLButtonElement>('[aria-label="More"]');
    expect(more).toHaveLength(2);
    const calls = play.mock.calls.length;
    await act(async () => more[1].click());
    expect(context).toHaveBeenCalled();
    expect(play).toHaveBeenCalledTimes(calls);
    const row = more[1].closest('div.group')!;
    await act(async () => row.dispatchEvent(new MouseEvent('click', { bubbles: true })));
    expect(play.mock.lastCall?.[3]).toBe(1);
    expect(play.mock.lastCall?.[1]).toHaveLength(2);
  } finally { await act(async () => root.unmount()); }
});

it.each(['album', 'playlist'] as const)('%s ignores detail responses from a retired generation', async kind => {
  (globalThis as any).IS_REACT_ACT_ENVIRONMENT = true;
  useStore.setState({ spotifyConnected: true, spotifySessionGeneration: 1, addLog: vi.fn() });
  let finish!: (value: any) => void;
  const late = new Promise(resolve => { finish = resolve; });
  mocks.album.mockReturnValueOnce(late).mockRejectedValue(new Error('Session retired'));
  mocks.backend.mockResolvedValue({ ok: true, status: 200 });
  mocks.playlist.mockReturnValueOnce(late).mockRejectedValue(new Error('Session retired'));
  const host = document.createElement('div'); const root = createRoot(host);
  const Component = kind === 'album' ? SpotifyAlbumDetail : SpotifyPlaylistDetail;
  try {
    await act(async () => root.render(<MemoryRouter initialEntries={['/collection/id']}><Routes><Route path="/collection/:id" element={<Component/>}/></Routes></MemoryRouter>));
    await act(async () => useStore.setState({ spotifySessionGeneration: 2, spotifyConnected: false }));
    await act(async () => finish({ name: 'Retired Collection', tracks: { items: [] } }));
    expect(host.textContent).not.toContain('Retired Collection');
    expect(host.querySelector(`[aria-label="Play ${kind}"]`)).toBeNull();
  } finally { await act(async () => root.unmount()); }
});
