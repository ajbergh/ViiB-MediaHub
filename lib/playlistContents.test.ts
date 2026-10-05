/** Tests and fixtures for playlist Contents behavior. */

import { expect, it } from 'vitest';
import { resolvePlaylistSongs } from './playlistContents';
import { Song } from '../types';
it('resolves playlist order and repeated occurrences rather than library order', () => {
  const songs = [{ id: 'a' }, { id: 'b' }] as Song[];
  expect(resolvePlaylistSongs(['b', 'missing', 'a', 'a'], new Map(songs.map(song => [song.id, song]))).map(song => song.id)).toEqual(['b', 'a', 'a']);
});
