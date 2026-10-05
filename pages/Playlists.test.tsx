// @vitest-environment jsdom

/** Tests and fixtures for Playlists behavior. */

import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { expect, it, vi } from 'vitest';
const mocks = vi.hoisted(() => ({ state: {} as any, navigate: vi.fn() }));
vi.mock('../store', () => ({
  useStore: (selector?: any) => selector ? selector(mocks.state) : mocks.state,
  useAlbumCovers: () => ({}),
}));
vi.mock('react-router', () => ({ useNavigate: () => mocks.navigate }));
import { Playlists } from './Playlists';

it('creates thumbnails for new playlists and updates them with membership, custom covers, and failed art', async () => {
  (globalThis as any).IS_REACT_ACT_ENVIRONMENT = true;
  mocks.state = {
    playlists: [{ id: 'playlist', name: 'My Playlist', songIds: [], createdAt: 1 }],
    songs: ['a', 'b', 'c', 'd'].map(id => ({ id, title: id, artist: 'Artist', album: id, coverUrl: `/${id}.jpg` })),
    createPlaylist: vi.fn(), openContextMenu: vi.fn(), refreshLibrary: vi.fn(), showToast: vi.fn(),
  };
  const container = document.createElement('div');
  const root = createRoot(container);
  const render = () => act(async () => root.render(<Playlists />));
  const sources = () => [...container.querySelectorAll('img')].map(img => img.getAttribute('src'));
  const update = (patch: any) => { mocks.state.playlists = [{ ...mocks.state.playlists[0], ...patch }]; };
  try {
    await render();
    await act(async () => container.querySelector<HTMLButtonElement>('[aria-label="Open playlist My Playlist"]')!.click());
    expect(mocks.navigate).toHaveBeenCalledWith('/playlist/playlist');
    expect(sources()).toEqual([]);
    update({ songIds: ['a'] }); await render();
    expect(sources()).toEqual(['/a.jpg']);
    update({ songIds: ['a', 'b', 'c', 'd'] }); await render();
    expect(sources()).toEqual(['/a.jpg', '/b.jpg', '/c.jpg', '/d.jpg']);
    update({ songIds: ['b', 'a'] }); await render();
    expect(sources()).toEqual(['/b.jpg', '/a.jpg', '/b.jpg', '/a.jpg']);
    update({ coverUrl: '/custom.jpg' }); await render();
    expect(sources()).toEqual(['/custom.jpg']);
    await act(async () => container.querySelector('img')!.dispatchEvent(new Event('error')));
    expect(sources()).toEqual(['/b.jpg', '/a.jpg', '/b.jpg', '/a.jpg']);
    update({ coverUrl: undefined, songIds: ['a'] }); await render();
    await act(async () => container.querySelector('img')!.dispatchEvent(new Event('error')));
    expect(sources()).toEqual([]);
    expect(container.textContent).toContain('My Playlist');
  } finally { await act(async () => root.unmount()); }
});
