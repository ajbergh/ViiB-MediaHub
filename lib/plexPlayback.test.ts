import { describe, expect, it } from 'vitest';
import type { Song } from '../types';
import { findPlexPlaybackFallback } from './plexPlayback';

const song = (id: string, source: Song['source']): Song => ({
  id,
  title: id,
  artist: 'Artist',
  album: 'Album',
  duration: 180,
  url: `/api/audio/${id}`,
  addedAt: 0,
  source,
});

describe('findPlexPlaybackFallback', () => {
  it('skips remaining Plex tracks and chooses the next local track', () => {
    const queue = [song('current', 'plex'), song('another-plex', 'plex'), song('local', 'local')];

    expect(findPlexPlaybackFallback(queue, 0)).toBe(2);
  });

  it('does not replace a Plex-only queue with an arbitrary unavailable track', () => {
    const queue = [song('current', 'plex'), song('another-plex', 'plex')];

    expect(findPlexPlaybackFallback(queue, 0)).toBe(-1);
  });
});
