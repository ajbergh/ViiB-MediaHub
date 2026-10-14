/** Tests and fixtures for metadata behavior. */

import { afterEach, expect, it, vi } from 'vitest';
import { managedObjectUrls } from './lib/playbackLifecycle';
import { parseSong, resetMetadataWorkerForTests } from './metadata';
import { isBrowserImportSupportedFile } from './services/mediaCapabilities';

afterEach(() => {
  resetMetadataWorkerForTests();
  managedObjectUrls.releaseAll();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

it('registers actual browser-parser URLs and releases the worker bootstrap URL', async () => {
  let index = 0;
  vi.spyOn(URL, 'createObjectURL').mockImplementation(() => `blob:parser-${++index}`);
  const revoke = vi.spyOn(URL, 'revokeObjectURL').mockImplementation(() => {});
  class WorkerFixture {
    onmessage: any;
    onerror: any;
    postMessage(payload: any) {
      queueMicrotask(() =>
        this.onmessage({
          data: {
            id: payload.id,
            metadata: {
              title: 'Song',
              artist: 'Artist',
              album: 'Album',
              duration: 1,
              coverData: new Blob(['art']),
            },
          },
        }),
      );
    }
  }
  vi.stubGlobal('Worker', WorkerFixture);
  const parsed = await parseSong(new File(['audio'], 'song.mp3'), new Map());
  expect(managedObjectUrls.owns(parsed.url)).toBe(true);
  expect(managedObjectUrls.owns(parsed.coverUrl)).toBe(true);
  expect(revoke).toHaveBeenCalledWith('blob:parser-1');
  managedObjectUrls.release(parsed.url);
  expect(revoke).toHaveBeenCalledWith(parsed.url);
});

it('admits and parses MP3, OGG, FLAC, and M4A files with metadata, artwork, and duration', async () => {
  let index = 0;
  vi.spyOn(URL, 'createObjectURL').mockImplementation(() => `blob:format-${++index}`);
  vi.spyOn(URL, 'revokeObjectURL').mockImplementation(() => {});

  class MultiFormatWorkerFixture {
    onmessage: any;
    onerror: any;
    postMessage(payload: any) {
      const file: File = payload.file;
      const ext = file.name.split('.').pop()?.toLowerCase();
      queueMicrotask(() =>
        this.onmessage({
          data: {
            id: payload.id,
            metadata: {
              title: `Title (${ext})`,
              artist: `Artist (${ext})`,
              album: `Album (${ext})`,
              albumArtist: `AlbumArtist (${ext})`,
              trackNumber: 3,
              discNumber: 1,
              genre: ['Electronic'],
              year: 2024,
              duration: 215.5,
              coverData: new Blob([`cover-${ext}`], { type: 'image/jpeg' }),
            },
          },
        }),
      );
    }
  }
  vi.stubGlobal('Worker', MultiFormatWorkerFixture);

  for (const filename of ['01 - Track.mp3', '02 - Track.ogg', '03 - Track.flac', '04 - Track.m4a']) {
    expect(isBrowserImportSupportedFile(filename)).toBe(true);
    const ext = filename.split('.').pop()?.toLowerCase();
    const file = new File(['fixture-bytes'], filename);
    const song = await parseSong(file, new Map());
    expect(song.title).toBe(`Title (${ext})`);
    expect(song.artist).toBe(`Artist (${ext})`);
    expect(song.album).toBe(`Album (${ext})`);
    expect(song.albumArtist).toBe(`AlbumArtist (${ext})`);
    expect(song.trackNumber).toBe(3);
    expect(song.discNumber).toBe(1);
    expect(song.duration).toBe(215.5);
    expect(song.coverUrl).toBeDefined();
    expect(managedObjectUrls.owns(song.url)).toBe(true);
    expect(managedObjectUrls.owns(song.coverUrl)).toBe(true);
  }

  expect(isBrowserImportSupportedFile('unsupported.wma')).toBe(false);
  expect(isBrowserImportSupportedFile('unsupported.wav')).toBe(false);
});

