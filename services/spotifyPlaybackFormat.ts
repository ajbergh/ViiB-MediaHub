/** Requests MP3 on macOS when the media engine cannot decode Spotify's Vorbis streams. */
export function needsSpotifyMP3(
  userAgent = globalThis.navigator?.userAgent ?? '',
): boolean {
  if (!/Macintosh|Mac OS X/i.test(userAgent) || typeof document === 'undefined') return false;
  const audio = document.createElement('audio');
  return !audio.canPlayType('audio/ogg; codecs="vorbis"') && Boolean(audio.canPlayType('audio/mpeg'));
}
