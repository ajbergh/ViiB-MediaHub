import { describe, expect, it } from 'vitest';
import { isAuthoritativePlexArtwork, isPlexSourcePath, resolveAlbumArtwork } from './artwork';

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
