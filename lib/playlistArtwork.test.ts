/** Tests and fixtures for playlist Artwork behavior. */

import { expect, it } from 'vitest';
import { Song } from '../types';
import { getPlaylistArtwork } from './playlistArtwork';
const song = (id: string, coverUrl?: string, album = 'Album'): Song => ({
  id, title: id, artist: 'Artist', album, coverUrl, duration: 1, url: '/song', addedAt: 1,
});
it('uses up to four distinct available covers in playlist order, ignoring missing tracks and art', () => {
  const tracks = [song('a', '/a'), song('b', '/a'), song('c'), song('d', '/d'), song('e', '/e'), song('f', '/f'), song('g', '/g')];
  const index = new Map(tracks.map(track => [track.id, track]));
  expect(getPlaylistArtwork(['missing', 'c', 'b', 'a', 'd', 'e', 'f', 'g'], index)).toEqual(['/a', '/d', '/e', '/f']);
  expect(getPlaylistArtwork([], index)).toEqual([]);
});
it('uses exact album fallback artwork and respects absent Plex art', () => {
  const local = song('local');
  const plex = { ...song('plex'), path: 'plex://server/track' };
  const index = new Map([['local', local], ['plex', plex]]);
  expect(getPlaylistArtwork(['local'], index, { 'Album::Artist': '/exact', Album: '/other' })).toEqual(['/exact']);
  expect(getPlaylistArtwork(['plex'], index, { 'Album::Artist': '/other', Album: '/other' })).toEqual([]);
});
