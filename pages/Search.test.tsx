// @vitest-environment jsdom
import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { expect, it, vi } from 'vitest';
const mocks = vi.hoisted(() => ({ state: {} as any }));
vi.mock('../store', () => ({ useStore: (selector: any) => selector(mocks.state) }));
vi.mock('react-router', () => ({ useNavigate: () => vi.fn(), useLocation: () => ({ state: { query: 'smash' } }) }));
vi.mock('../services/libraryV2', () => ({ libraryV2: { search: async () => ({
  query: 'smash', tracks: [], albums: [], playlists: [],
  artists: [{ name: 'The Smashing Pumpkins', songCount: 436, albumCount: 14 }],
}) } }));
import { Search } from './Search';

it('uses artist portraits, responds to metadata updates, and falls back safely for absent or broken artwork', async () => {
  (globalThis as any).IS_REACT_ACT_ENVIRONMENT = true;
  mocks.state = {
    songs: [{ artist: 'The Smashing Pumpkins', albumArtist: 'The Smashing Pumpkins', coverUrl: '/album.jpg' }],
    playlists: [], artistMetadata: { 'the smashing pumpkins': { imageUrl: '/portrait.jpg' } },
    backendAvailable: true, localSearchQuery: '', localSearchTab: 'artists',
    setLocalSearchQuery: vi.fn(), setLocalSearchTab: vi.fn(), playSong: vi.fn(), addToQueue: vi.fn(), showToast: vi.fn(),
  };
  const container = document.createElement('div');
  const root = createRoot(container);
  const render = () => act(async () => root.render(<Search />));
  try {
    await render();
    expect(container.querySelector('img')?.getAttribute('src')).toBe('/portrait.jpg');
    await act(async () => container.querySelector('img')!.dispatchEvent(new Event('error')));
    expect(container.querySelector('img')).toBeNull();
    mocks.state.artistMetadata = { 'The Smashing Pumpkins': { imageUrl: '/new-portrait.jpg' } };
    await render();
    expect(container.querySelector('img')?.getAttribute('src')).toBe('/new-portrait.jpg');
    mocks.state.artistMetadata = {};
    await render();
    expect(container.querySelector('img')?.getAttribute('src')).toBe('/album.jpg');
    mocks.state.songs = [];
    await render();
    expect(container.querySelector('img')).toBeNull();
    expect(container.textContent).toContain('The Smashing Pumpkins');
  } finally { await act(async () => root.unmount()); }
});
