/** Tests Spotify stream format selection using the platform and Vorbis capability. */

import { afterEach, describe, expect, it, vi } from 'vitest';
import { api } from './api';

afterEach(() => vi.unstubAllGlobals());

describe('Spotify playback format', () => {
  it.each([
    ['Macintosh', '', '/api/spotify/stream/track?quality=high&format=mp3'],
    ['Macintosh', 'probably', '/api/spotify/stream/track?quality=high'],
    ['Windows NT', '', '/api/spotify/stream/track?quality=high'],
  ])('selects the playback URL for %s with Vorbis support %j', (userAgent, vorbis, expected) => {
    vi.stubGlobal('navigator', { userAgent });
    vi.stubGlobal('document', {
      createElement: () => ({ canPlayType: (mime: string) => mime.includes('vorbis') ? vorbis : 'probably' }),
    });
    expect(api.getSpotifyStreamUrl('track', 'high')).toBe(expected);
  });
});
