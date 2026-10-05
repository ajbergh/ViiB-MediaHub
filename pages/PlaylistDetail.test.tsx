// @vitest-environment jsdom

/** Tests and fixtures for Playlist Detail behavior. */

import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { expect, it, vi } from 'vitest';
const mocks = vi.hoisted(() => ({ state: {} as any }));
vi.mock('../store', () => ({ useStore: (selector: any) => selector(mocks.state), useAlbumCovers: () => ({}) }));
vi.mock('react-router', () => ({ useParams: () => ({ playlistId: 'playlist' }), Link: ({ children }: any) => <a>{children}</a> }));
import { PlaylistDetail } from './PlaylistDetail';

it('edits individual occurrences, retains unavailable tracks, cancels, and retries failed saves', async () => {
  (globalThis as any).IS_REACT_ACT_ENVIRONMENT = true;
  const original = ['a', 'a', 'missing', 'b'];
  mocks.state = {
    playlists: [{ id: 'playlist', name: 'Mix', songIds: original, createdAt: 1 }],
    songs: ['a', 'b'].map(id => ({ id, title: id === 'a' ? 'Alpha' : 'Beta', artist: 'Artist', album: 'Album', duration: 1 })),
    playSong: vi.fn(), updatePlaylistContents: vi.fn().mockRejectedValueOnce(new Error('Save failed')).mockResolvedValue(undefined),
  };
  const container = document.createElement('div'); const root = createRoot(container);
  const render = () => act(async () => root.render(<PlaylistDetail />));
  const click = (label: string) => act(async () => {
    const button = [...container.querySelectorAll('button')].find(item => item.getAttribute('aria-label') === label || item.textContent === label)!;
    button.click();
  });
  const titles = () => [...container.querySelectorAll('ol li p.font-medium')].map(item => item.textContent);
  try {
    await render();
    expect(titles()).toEqual(['Alpha', 'Alpha', 'Unavailable track', 'Beta']);
    await click('Remove track 2: Alpha');
    expect(titles()).toEqual(['Alpha', 'Unavailable track', 'Beta']);
    await click('Cancel');
    expect(titles()).toEqual(['Alpha', 'Alpha', 'Unavailable track', 'Beta']);
    expect(mocks.state.updatePlaylistContents).not.toHaveBeenCalled();
    await click('Remove track 2: Alpha'); await click('Move track 3 up');
    await click('Add Alpha to playlist');
    expect(titles()).toEqual(['Alpha', 'Beta', 'Unavailable track', 'Alpha']);
    await click('Play playlist');
    expect(mocks.state.playSong.mock.calls[0][1].map((song: any) => song.id)).toEqual(['a', 'b', 'a']);
    await click('Save changes');
    expect(container.querySelector('[role=alert]')?.textContent).toBe('Save failed');
    expect(titles()).toEqual(['Alpha', 'Beta', 'Unavailable track', 'Alpha']);
    expect(mocks.state.updatePlaylistContents).toHaveBeenLastCalledWith('playlist', ['a', 'b', 'missing', 'a'], original);
    await click('Save changes');
    expect(container.querySelector('[role=status]')?.textContent).toBe('Playlist saved');
  } finally { await act(async () => root.unmount()); }
});
