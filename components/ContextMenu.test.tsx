// @vitest-environment jsdom
import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { afterEach, expect, it, vi } from 'vitest';
const mocks = vi.hoisted(() => ({ state: {} as any }));
vi.mock('../store', () => ({ useStore: (selector?: any) => selector ? selector(mocks.state) : mocks.state }));
vi.mock('react-router', () => ({ useNavigate: () => vi.fn() }));
vi.mock('./context-menus/AlbumMenu', () => ({ AlbumMenu: () => null }));
vi.mock('./context-menus/ArtistMenu', () => ({ ArtistMenu: () => null }));
vi.mock('./context-menus/PlaylistMenu', () => ({ PlaylistMenu: () => null }));
vi.mock('./context-menus/SmartMixMenu', () => ({ SmartMixMenu: () => null }));
vi.mock('./context-menus/QueueItemMenu', () => ({ QueueItemMenu: () => null }));
import { ContextMenu } from './ContextMenu';
import { ContextMenuType } from '../types';
afterEach(() => vi.restoreAllMocks());

it('opens the existing playlist submenu within the viewport and adds the selected song', async () => {
  (globalThis as any).IS_REACT_ACT_ENVIRONMENT = true;
  vi.spyOn(window, 'requestAnimationFrame').mockImplementation(callback => { queueMicrotask(() => callback(0)); return 1; });
  vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockImplementation(function () {
    const submenu = this.hasAttribute('data-viib-submenu');
    return { x: 780, y: 700, left: 780, right: 1004, top: 700, bottom: 950, width: 224, height: submenu ? 250 : 40, toJSON() {} };
  });
  mocks.state = {
    contextMenu: { isOpen: true, x: 780, y: 700, type: ContextMenuType.SONG, data: { id: 'song', title: 'Track', artist: 'Artist', album: 'Album' } },
    playlists: [{ id: 'existing', name: 'Existing Playlist', songIds: [] }],
    addToPlaylist: vi.fn(), createPlaylist: vi.fn().mockResolvedValue(undefined), showToast: vi.fn(), closeContextMenu: vi.fn(),
  };
  const container = document.createElement('div'); document.body.appendChild(container);
  const root = createRoot(container);
  try {
    await act(async () => root.render(<ContextMenu />));
    const menu = container.querySelector('[aria-label="Context menu"]')!;
    expect(menu.classList.contains('overflow-hidden')).toBe(false);
    const trigger = container.querySelector<HTMLButtonElement>('[aria-label="Add to Playlist"]')!;
    await act(async () => { trigger.focus(); trigger.click(); });
    const submenu = container.querySelector<HTMLElement>('[data-viib-submenu="playlists"]')!;
    expect(submenu).not.toBeNull();
    expect(Number.parseFloat(submenu.style.left)).toBeLessThan(0);
    expect(Number.parseFloat(submenu.style.top)).toBeLessThan(0);
    expect(document.activeElement?.getAttribute('aria-label')).toBe('New Playlist');
    await act(async () => document.activeElement!.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowLeft', bubbles: true })));
    expect(container.querySelector('[data-viib-submenu="playlists"]')).toBeNull();
    expect(document.activeElement).toBe(trigger);
    await act(async () => trigger.click());
    await act(async () => container.querySelector<HTMLButtonElement>('[aria-label="Existing Playlist"]')!.click());
    expect(mocks.state.addToPlaylist).toHaveBeenCalledWith('existing', 'song');
    expect(mocks.state.closeContextMenu).toHaveBeenCalled();
    await act(async () => trigger.click());
    vi.spyOn(window, 'prompt').mockReturnValue('New Mix');
    await act(async () => container.querySelector<HTMLButtonElement>('[aria-label="New Playlist"]')!.click());
    expect(mocks.state.createPlaylist).toHaveBeenCalledWith('New Mix', ['song']);
  } finally { await act(async () => root.unmount()); container.remove(); }
});
