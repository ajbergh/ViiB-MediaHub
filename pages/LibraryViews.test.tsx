// @vitest-environment jsdom

/** Tests and fixtures for Library Views behavior. */

import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { afterEach, expect, it, vi } from 'vitest';
const mocks = vi.hoisted(() => ({ navigate: vi.fn(), context: vi.fn(), play: vi.fn(), genres: vi.fn(), export: vi.fn() }));
vi.mock('../store', () => {
  const songs = [{ id: 'a', title: 'Track', artist: 'Artist', album: 'Album', genre: ['Rock'], coverUrl: '/cover.jpg' }];
  const state = { songs, playlists: [{ id: 'pl', name: 'Mix', songIds: ['a'], createdAt: 1 }], albumMetadata: {}, artistMetadata: {}, openContextMenu: mocks.context, playSong: mocks.play, fetchAlbumMetadata: vi.fn() };
  return {
    useStore: (selector?: any) => selector ? selector(state) : state,
    useAlbums: () => [{ name: 'Album', artist: 'Artist', songCount: 1, coverUrl: '/cover.jpg' }],
    useArtists: () => [{ name: 'Artist', songCount: 1, albumCount: 1, imageUrl: '/artist.jpg' }],
    useAlbumCovers: () => ({}),
  };
});
vi.mock('react-router', () => ({ useNavigate: () => mocks.navigate }));
vi.mock('../services/api', () => ({ api: { getGenres: mocks.genres, exportPlaylistM3U: mocks.export } }));
vi.mock('react-virtuoso', () => ({ VirtuosoGrid: ({ data, itemContent, components }: any) => <components.List>{data.map((item: any, index: number) => <components.Item key={index}>{itemContent(index, item)}</components.Item>)}</components.List> }));
import { Albums } from './Albums';
import { Artists } from './Artists';
import { Genres } from './Genres';
import { Playlists } from './Playlists';

afterEach(() => { localStorage.clear(); vi.clearAllMocks(); });

it.each([
  ['albums', Albums, 'Album', '/album/Album/Artist'],
  ['artists', Artists, 'Artist', '/artist/Artist'],
  ['genres', Genres, 'Rock', '/genres/Rock'],
  ['playlists', Playlists, 'Mix', '/playlist/pl'],
] as const)('%s switches to persisted rows at the left endpoint and back to tiles', async (key, Component, name, route) => {
  (globalThis as any).IS_REACT_ACT_ENVIRONMENT = true;
  mocks.genres.mockResolvedValue([{ name: 'Rock', count: 1, topArtists: ['Artist'], coverUrl: '/cover.jpg' }]);
  const container = document.createElement('div');
  document.body.append(container);
  let root = createRoot(container);
  const render = () => act(async () => { root.render(<Component />); });
  const slider = () => container.querySelector<HTMLInputElement>('input[type=range]')!;
  const change = async (value: string) => act(async () => {
    Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!.call(slider(), value);
    slider().dispatchEvent(new Event('input', { bubbles: true }));
    slider().dispatchEvent(new Event('change', { bubbles: true }));
  });
  try {
    await render();
    expect(slider().getAttribute('aria-valuetext')).not.toBe('List view');
    await change('2');
    expect(slider().getAttribute('aria-valuetext')).toBe('List view');
    expect(localStorage.getItem(`${key}-card-cols`)).toBe('8');
    const open = container.querySelector<HTMLButtonElement>(`[aria-label="Open ${name}"]`)!;
    expect(open).not.toBeNull();
    expect(open.querySelector('.w-10')).not.toBeNull();
    await act(async () => open.click());
    expect(mocks.navigate).toHaveBeenCalledWith(route);
    if (key !== 'genres') {
      await act(async () => open.dispatchEvent(new MouseEvent('contextmenu', { bubbles: true })));
      expect(mocks.context).toHaveBeenCalled();
    } else {
      mocks.navigate.mockClear();
      await act(async () => container.querySelector<HTMLButtonElement>('[aria-label="Play Rock"]')!.click());
      await act(async () => container.querySelector<HTMLButtonElement>('[aria-label="Shuffle Rock"]')!.click());
      expect(mocks.play).toHaveBeenCalledTimes(2);
      expect(mocks.navigate).not.toHaveBeenCalled();
    }
    if (key === 'playlists') {
      mocks.export.mockResolvedValue(new Blob(['#EXTM3U']));
      const createURL = vi.fn(() => 'blob:playlist');
      const revokeURL = vi.fn();
      Object.defineProperty(URL, 'createObjectURL', { configurable: true, value: createURL });
      Object.defineProperty(URL, 'revokeObjectURL', { configurable: true, value: revokeURL });
      const download = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {});
      mocks.navigate.mockClear();
      try {
        await act(async () => container.querySelector<HTMLButtonElement>('[aria-label="Export Mix as M3U"]')!.click());
        expect(mocks.export).toHaveBeenCalledWith('pl');
        expect(download).toHaveBeenCalledOnce();
        expect(revokeURL).toHaveBeenCalledWith('blob:playlist');
        expect(mocks.navigate).not.toHaveBeenCalled();
      } finally { download.mockRestore(); }
    }
    await act(async () => root.unmount());
    root = createRoot(container); await render();
    expect(slider().getAttribute('aria-valuetext')).toBe('List view');
    await change('3');
    expect(slider().getAttribute('aria-valuetext')).toBe('7 columns');
    expect(container.querySelector(`[aria-label="Open ${name}"]`)).toBeNull();
  } finally { await act(async () => root.unmount()); container.remove(); }
});
