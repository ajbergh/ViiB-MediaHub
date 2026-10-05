// @vitest-environment jsdom

/** Tests and fixtures for use Home Content behavior. */

import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { expect, it, vi } from 'vitest';
const mocks = vi.hoisted(() => ({ state: {} as any, artists: [] as any[] }));
vi.mock('../../store', () => ({
  useStore: (selector: any) => selector(mocks.state),
  useAlbums: () => [],
  useArtists: () => mocks.artists,
}));
vi.mock('react-router', () => ({ useNavigate: () => vi.fn() }));
import { useHomeContent, HomeContent } from './useHomeContent';

it('ranks exact artist credits and collaborations with a bounded number of song reads', async () => {
  (globalThis as any).IS_REACT_ACT_ENVIRONMENT = true;
  let artistReads = 0;
  const songs = Array.from({ length: 2000 }, (_, i) => ({
    id: String(i), album: 'Album', playCount: i % 2 ? 10 : 50,
    get artist() { artistReads++; return i % 2 ? 'Alpha feat. Beta' : 'Alphabet'; },
  }));
  mocks.artists = ['Alpha', 'Beta', 'Alphabet', ...Array.from({ length: 100 }, (_, i) => `Other ${i}`)]
    .map(name => ({ name, songCount: 1, albumCount: 1 }));
  mocks.state = { songs, smartMixes: [], artistMetadata: {}, showSmartMixes: false, playSong: vi.fn(), openContextMenu: vi.fn() };
  let content: HomeContent | undefined;
  const Probe = () => { content = useHomeContent(); return null; };
  const root = createRoot(document.createElement('div'));
  try {
    await act(async () => root.render(<Probe />));
    expect(content!.topArtistsByPlays.map(artist => artist.name)).toEqual(['Alphabet', 'Alpha', 'Beta']);
    expect(artistReads).toBeLessThanOrEqual(songs.length * 2);
  } finally { await act(async () => root.unmount()); }
});
