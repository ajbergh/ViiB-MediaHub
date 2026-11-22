// @vitest-environment jsdom

/** Tests and fixtures for Search behavior. */

import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { expect, it, vi } from 'vitest';
const mocks = vi.hoisted(() => ({
  state: {} as any,
  searchResult: {
    query: 'smash', tracks: [], albums: [], playlists: [],
    artists: [{ name: 'The Smashing Pumpkins', songCount: 436, albumCount: 14 }],
  } as any,
}));
vi.mock('../store', () => ({ useStore: (selector: any) => selector(mocks.state) }));
vi.mock('react-router', () => ({ useNavigate: () => vi.fn(), useLocation: () => ({ state: { query: 'smash' } }) }));
vi.mock('../services/libraryV2', () => ({ libraryV2: { search: async () => mocks.searchResult } }));
import { Search } from './Search';

it('uses artist portraits, responds to metadata updates, and falls back safely for absent or broken artwork', async () => {
  (globalThis as any).IS_REACT_ACT_ENVIRONMENT = true;
  mocks.searchResult = {
    query: 'smash', tracks: [], albums: [], playlists: [],
    artists: [{ name: 'The Smashing Pumpkins', songCount: 436, albumCount: 14 }],
  };
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

it('resolves plex album artwork, falls back on error, supports roving tab focus, and clears query', async () => {
  (globalThis as any).IS_REACT_ACT_ENVIRONMENT = true;
  mocks.searchResult = {
    query: 'smash', tracks: [], playlists: [], artists: [],
    albums: [{ name: 'Siamese Dream', artist: 'Smashing Pumpkins', songCount: 13, coverPath: 'plex://art/plex_abc123?v=deadbeef' }],
  };
  mocks.state = {
    songs: [], playlists: [], artistMetadata: {}, albumMetadata: {},
    backendAvailable: true, localSearchQuery: '', localSearchTab: 'all',
    setLocalSearchQuery: vi.fn(), setLocalSearchTab: vi.fn(), playSong: vi.fn(), addToQueue: vi.fn(), showToast: vi.fn(),
  };
  const container = document.createElement('div');
  document.body.appendChild(container);
  const root = createRoot(container);
  try {
    await act(async () => root.render(<Search />));
    const albumImg = container.querySelector('img');
    expect(albumImg?.getAttribute('src')).toBe('/api/cover/plex_abc123?v=deadbeef&size=256');
    await act(async () => albumImg!.dispatchEvent(new Event('error')));
    expect(container.querySelector('img')).toBeNull();

    const tabs = [...container.querySelectorAll<HTMLButtonElement>('[role="tab"]')];
    expect(tabs[0].tabIndex).toBe(0);
    expect(tabs[1].tabIndex).toBe(-1);
    await act(async () => {
      tabs[0].focus();
      tabs[0].dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowRight', bubbles: true }));
    });
    expect(mocks.state.setLocalSearchTab).toHaveBeenCalledWith('tracks');
    expect(document.activeElement).toBe(tabs[1]);

    await act(async () => {
      tabs[1].dispatchEvent(new KeyboardEvent('keydown', { key: 'End', bubbles: true }));
    });
    expect(mocks.state.setLocalSearchTab).toHaveBeenCalledWith('playlists');
    expect(document.activeElement).toBe(tabs[4]);

    const clearBtn = container.querySelector<HTMLButtonElement>('button[aria-label="Clear search"]');
    expect(clearBtn).not.toBeNull();
    await act(async () => clearBtn!.click());
    expect(mocks.state.setLocalSearchQuery).toHaveBeenCalledWith('');
  } finally {
    await act(async () => root.unmount());
    container.remove();
  }
});
