/** Exercises the same parser used by the browser worker with real tagged audio bytes. */
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { describe, expect, it, vi } from 'vitest';
import { parseBrowserSongMetadata } from './browserMetadata';

describe('browser audio metadata fixtures', () => {
  it.each(['mp3', 'ogg', 'flac', 'm4a'])('extracts tags, duration and embedded artwork from %s', async extension => {
    const bytes = readFileSync(fileURLToPath(new URL(`../testdata/browser-metadata/tagged.${extension}`, import.meta.url)));
    const file = new File([new Uint8Array(bytes)], `tagged.${extension}`);
    const result = await parseBrowserSongMetadata(file, '');
    expect(result.title).toBe('Fixture Title');
    expect(result.artist).toBe('Fixture Artist');
    expect(result.album).toBe('Fixture Album');
    expect(result.albumArtist).toBe('Fixture Album Artist');
    expect(result.trackNumber).toBe(3);
    expect(result.discNumber).toBe(1);
    expect(result.year).toBe(2024);
    expect(result.genre).toContain('Electronic');
    expect(result.duration).toBeGreaterThan(0.9);
    expect(result.duration).toBeLessThan(1.2);
    expect(result.coverData?.type).toBe('image/png');
    const artwork = new Uint8Array(await result.coverData.arrayBuffer());
    expect([...artwork.slice(0, 8)]).toEqual([137, 80, 78, 71, 13, 10, 26, 10]);
  });

  it('uses honest filename and folder fallbacks for malformed audio', async () => {
    const warning = vi.spyOn(console, 'warn').mockImplementation(() => {});
    try {
      const result = await parseBrowserSongMetadata(new File(['broken'], '03 - Track.flac'), 'Music/Artist/Album/03 - Track.flac');
      expect(result).toMatchObject({ title: 'Track', artist: 'Artist', album: 'Album', trackNumber: 3, duration: 0 });
      expect(result.coverData).toBeUndefined();
    } finally {
      warning.mockRestore();
    }
  });
});
