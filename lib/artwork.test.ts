import { describe, expect, it } from 'vitest';
import { buildAlbumCoverIndex, isAuthoritativePlexArtwork, isPlexSourcePath, resolveAlbumArtwork } from './artwork';

describe('album artwork precedence', () => {
  it('treats versioned ViiB Plex cover URLs as authoritative', () => {
    const plex = '/api/cover/plex_abc?v=deadbeef';
    expect(isAuthoritativePlexArtwork(plex)).toBe(true);
    expect(resolveAlbumArtwork(plex, 'https://i.scdn.co/spotify.jpg')).toBe(plex);
  });

  it('treats Plex artwork absence as authoritative for Plex-backed albums', () => {
    expect(isPlexSourcePath('plex://machine/2/123')).toBe(true);
    expect(resolveAlbumArtwork(undefined, 'https://i.scdn.co/spotify.jpg', true)).toBeUndefined();
  });

  it('preserves existing enrichment precedence for local catalog artwork', () => {
    expect(resolveAlbumArtwork('/api/cover/local_abc', 'https://i.scdn.co/spotify.jpg')).toBe('https://i.scdn.co/spotify.jpg');
  });

  it('falls back to catalog artwork when enrichment is absent', () => {
    expect(resolveAlbumArtwork('/api/cover/local_abc', undefined)).toBe('/api/cover/local_abc');
  });
});

describe('album cover index', () => {
  it('keeps exact album and artist keys while retaining a safe title alias', () => {
    expect(buildAlbumCoverIndex([
      { name: 'Signals', artist: 'Rush', coverUrl: '/api/cover/signals' },
    ])).toEqual({
      'Signals::Rush': '/api/cover/signals',
      Signals: '/api/cover/signals',
    });
  });

  it('retains Plex no-art as an authoritative sentinel when the title is unambiguous', () => {
    expect(buildAlbumCoverIndex([
      { name: 'Unknown Pleasures', artist: 'Joy Division', plexBacked: true },
    ])).toEqual({
      'Unknown Pleasures::Joy Division': '',
      'Unknown Pleasures': '',
    });
  });

  it('drops the unsafe title alias when a local and Plex album share a title', () => {
    const index = buildAlbumCoverIndex([
      { name: 'Greatest Hits', artist: 'Local Artist', coverUrl: '/api/cover/local' },
      { name: 'Greatest Hits', artist: 'Plex Artist', plexBacked: true },
    ]);

    expect(index['Greatest Hits::Local Artist']).toBe('/api/cover/local');
    expect(index['Greatest Hits::Plex Artist']).toBe('');
    expect('Greatest Hits' in index).toBe(false);
  });

  it('drops the title alias when same-title albums disagree on artwork', () => {
    const index = buildAlbumCoverIndex([
      { name: 'Live', artist: 'Artist One', coverUrl: '/api/cover/one' },
      { name: 'Live', artist: 'Artist Two', coverUrl: '/api/cover/two' },
    ]);

    expect(index['Live::Artist One']).toBe('/api/cover/one');
    expect(index['Live::Artist Two']).toBe('/api/cover/two');
    expect('Live' in index).toBe(false);
  });
});
