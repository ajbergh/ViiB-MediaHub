// @vitest-environment jsdom

/** Correctness and 50k-track performance tests for smartMix, store selectors, and useHomeContent. */

import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('react-router', () => ({ useNavigate: () => vi.fn() }));

import { generateSmartMixes } from './smartMix';
import { affectsSmartMixes, catalogDeltas, patchCatalogSong } from './libraryIndex';
import { HomeContent, useHomeContent } from '../components/home/useHomeContent';
import { useAlbums, useArtists, useStore } from '../store';
import { HomeLayoutVariant, Song } from '../types';

const DAY_MS = 24 * 60 * 60 * 1000;

it('preserves derived groups on playback changes and updates only the affected artwork', async () => {
  (globalThis as any).IS_REACT_ACT_ENVIRONMENT = true;
  const previous = useStore.getState().songs;
  const initial = makeSyntheticLibrary(4);
  const host = document.createElement('div');
  const root = createRoot(host);
  let albums!: ReturnType<typeof useAlbums>;
  let artists!: ReturnType<typeof useArtists>;
  function Reader() {
    albums = useAlbums();
    artists = useArtists();
    return null;
  }

  try {
    useStore.setState({ songs: initial });
    await act(async () => root.render(React.createElement(Reader)));
    const originalAlbums = albums;
    const originalArtists = artists;

    const played = patchCatalogSong(initial, initial[0].id, { playCount: 99, lastPlayed: Date.now() });
    expect(catalogDeltas.get(played)?.changes.some(affectsSmartMixes)).toBe(true);
    await act(async () => useStore.setState({ songs: played }));
    expect(albums).toBe(originalAlbums);
    expect(artists).toBe(originalArtists);

    const artwork = patchCatalogSong(played, initial[0].id, { coverUrl: '/updated-cover' });
    expect(catalogDeltas.get(artwork)?.changes.some(affectsSmartMixes)).toBe(false);
    await act(async () => useStore.setState({ songs: artwork }));
    expect(albums.find(album => album.name === initial[0].album)?.coverUrl).toBe('/updated-cover');
    expect(artists.find(artist => artist.name === initial[0].albumArtist)?.imageUrl).toBe('/updated-cover');
    expect(albums.find(album => album.name === initial[1].album)).toBe(originalAlbums.find(album => album.name === initial[1].album));
    expect(artists.find(artist => artist.name === initial[2].artist)).toBe(originalArtists.find(artist => artist.name === initial[2].artist));
  } finally {
    await act(async () => root.unmount());
    useStore.setState({ songs: previous });
  }
});

function makeSyntheticLibrary(count: number, now = Date.now()): Song[] {
  const genresPool = [
    ['Indie Rock'],
    ['Acoustic', 'Folk'],
    ['Electronic', 'House'],
    ['Classical', 'Ambient'],
    ['Jazz'],
    ['Pop', 'Dance'],
    [],
  ];
  const songs: Song[] = new Array(count);
  for (let i = 0; i < count; i++) {
    const albumIdx = i % 2500;
    const artistIdx = i % 1500;
    const hasPlayed = i % 3 !== 0;
    const lastPlayedOffset = (i % 120) * DAY_MS;
    songs[i] = {
      id: `song-${i}`,
      title: `Track ${i}`,
      artist: i % 11 === 0 ? `Artist ${artistIdx} feat. Artist ${(artistIdx + 1) % 1500}` : `Artist ${artistIdx}`,
      album: `Album ${albumIdx}`,
      albumArtist: `Artist ${artistIdx}`,
      trackNumber: (i % 15) + 1,
      discNumber: 1,
      genre: genresPool[i % genresPool.length],
      year: 1985 + (i % 40),
      duration: 180 + (i % 120),
      url: `/api/audio/song-${i}`,
      coverUrl: albumIdx % 5 === 0 ? undefined : `/api/cover/${albumIdx}.jpg`,
      addedAt: now - ((count - i) * 1000),
      playCount: hasPlayed ? (i * 7) % 100 : 0,
      lastPlayed: hasPlayed ? now - lastPlayedOffset : 0,
    };
  }
  return songs;
}

describe('generateSmartMixes', () => {
  it('applies exact mix rules, thresholds, and size limits', () => {
    const now = Date.now();
    const songs: Song[] = [
      // Heavy rotation candidates (played within 90 days)
      ...Array.from({ length: 60 }, (_, i) => ({
        id: `heavy-${i}`,
        title: `Heavy ${i}`,
        artist: 'Heavy Artist',
        album: 'Heavy Album',
        duration: 200,
        url: `/audio/heavy-${i}`,
        addedAt: now - 1000 - i,
        playCount: i + 1,
        lastPlayed: now - 5 * DAY_MS,
      })),
      // Rediscover candidates (playCount > 2, lastPlayed < now - 30d, outside 90d heavy window)
      ...Array.from({ length: 10 }, (_, i) => ({
        id: `old-${i}`,
        title: `Old ${i}`,
        artist: 'Old Artist',
        album: 'Old Album',
        duration: 200,
        url: `/audio/old-${i}`,
        addedAt: now - 50000 - i,
        playCount: i + 3,
        lastPlayed: now - 100 * DAY_MS,
      })),
      // 90s alt with genre and without genre, plus 90s pop excluded
      {
        id: '90s-rock',
        title: '90s Rock',
        artist: 'Alt Band',
        album: '90s Album',
        genre: ['Alternative Rock'],
        year: 1994,
        duration: 210,
        url: '/audio/90s-rock',
        addedAt: now - 200,
      },
      {
        id: '90s-nogenre',
        title: '90s No Genre',
        artist: 'Alt Band',
        album: '90s Album',
        year: 1997,
        duration: 210,
        url: '/audio/90s-nogenre',
        addedAt: now - 201,
      },
      {
        id: '90s-pop',
        title: '90s Pop',
        artist: 'Pop Star',
        album: 'Pop Album',
        genre: ['Bubblegum Pop'],
        year: 1995,
        duration: 210,
        url: '/audio/90s-pop',
        addedAt: now - 202,
      },
      // 6 workout + 6 focus tracks (> 5 threshold)
      ...Array.from({ length: 6 }, (_, i) => ({
        id: `workout-${i}`,
        title: `Workout ${i}`,
        artist: 'DJ',
        album: 'Workout',
        genre: ['Electronic'],
        duration: 180,
        url: `/audio/workout-${i}`,
        addedAt: now - 300 - i,
      })),
      ...Array.from({ length: 6 }, (_, i) => ({
        id: `focus-${i}`,
        title: `Focus ${i}`,
        artist: 'Composer',
        album: 'Focus',
        genre: ['Ambient', 'Piano'],
        duration: 180,
        url: `/audio/focus-${i}`,
        addedAt: now - 400 - i,
      })),
    ];

    const mixes = generateSmartMixes(songs);
    expect(mixes.map(m => m.id)).toEqual([
      'heavy-rotation',
      'rediscover',
      'fresh-finds',
      'chill-acoustic',
      '90s-alt',
      'workout-energy',
      'focus-mode',
    ]);

    const heavy = mixes.find(m => m.id === 'heavy-rotation')!;
    expect(heavy.songIds).toHaveLength(50);
    expect(heavy.songIds[0]).toBe('heavy-59');
    expect(heavy.songIds[49]).toBe('heavy-10');

    const rediscover = mixes.find(m => m.id === 'rediscover')!;
    expect(rediscover.songIds).toHaveLength(10);
    expect(rediscover.songIds[0]).toBe('old-9');

    const fresh = mixes.find(m => m.id === 'fresh-finds')!;
    expect(fresh.songIds).toHaveLength(50);
    expect(fresh.songIds[0]).toBe('90s-rock');

    const nineties = mixes.find(m => m.id === '90s-alt')!;
    expect(nineties.songIds).toContain('90s-rock');
    expect(nineties.songIds).toContain('90s-nogenre');
    expect(nineties.songIds).not.toContain('90s-pop');

    expect(mixes.find(m => m.id === 'chill-acoustic')!.songIds).toHaveLength(6);
    expect(mixes.find(m => m.id === 'workout-energy')!.songIds).toHaveLength(8);
    expect(mixes.find(m => m.id === 'focus-mode')!.songIds).toHaveLength(6);
  });

  it('omits workout and focus mixes when matches do not exceed 5', () => {
    const mixes = generateSmartMixes([
      {
        id: '1',
        title: 'Track 1',
        artist: 'A',
        album: 'Al',
        genre: ['Electronic', 'Ambient'],
        duration: 120,
        url: '/1',
        addedAt: 100,
      },
    ]);
    expect(mixes.map(m => m.id)).toEqual([
      'heavy-rotation',
      'rediscover',
      'fresh-finds',
      'chill-acoustic',
      '90s-alt',
    ]);
  });
});

describe('useHomeContent rankings & layout scoping', () => {
  const initialState = useStore.getState();

  beforeEach(() => {
    (globalThis as any).IS_REACT_ACT_ENVIRONMENT = true;
  });

  afterEach(() => {
    useStore.setState(initialState);
  });

  async function renderHookWithStore(songs: Song[], variant: HomeLayoutVariant): Promise<HomeContent> {
    useStore.setState({
      songs,
      smartMixes: [],
      artistMetadata: {},
      showSmartMixes: true,
      homeLayoutVariant: variant,
    });
    let captured!: HomeContent;
    const Probe = () => {
      captured = useHomeContent();
      return null;
    };
    const container = document.createElement('div');
    const root = createRoot(container);
    try {
      await act(async () => {
        root.render(React.createElement(Probe));
      });
    } finally {
      await act(async () => {
        root.unmount();
      });
    }
    return captured;
  }

  it('matches full-sort rankings for bounded top-K lists and coverWallAlbums', async () => {
    const now = Date.now();
    const songs: Song[] = [];
    // 30 albums with varying plays and covers; ties included
    for (let i = 0; i < 30; i++) {
      songs.push({
        id: `s-${i}`,
        title: `Song ${i}`,
        artist: `Artist ${i}`,
        album: `Album ${i}`,
        duration: 180,
        url: `/audio/${i}`,
        coverUrl: i % 3 === 0 ? `/cover/${i}.jpg` : undefined,
        addedAt: now - i * 1000,
        playCount: (i % 7) * 10 + (i < 5 ? 100 : 0),
        lastPlayed: i % 2 === 0 ? now - (30 - i) * 1000 : 0,
      });
    }

    const expectedRecentIds = [...songs]
      .filter(s => s.lastPlayed && s.lastPlayed > 0)
      .sort((a, b) => (b.lastPlayed || 0) - (a.lastPlayed || 0))
      .slice(0, 20)
      .map(s => s.id);

    const shelves = await renderHookWithStore(songs, 'shelves');
    expect(shelves.recentlyPlayedTracks.map(s => s.id)).toEqual(expectedRecentIds);
    expect(shelves.quickTiles.map(t => t.id)).toEqual(expectedRecentIds.slice(0, 6));
    expect(shelves.topAlbumsByPlays).toEqual([]);
    expect(shelves.topArtistsByPlays).toEqual([]);
    expect(shelves.coverWallAlbums).toEqual([]);
    expect(shelves.spotlightItem).not.toBeNull();

    const dashboard = await renderHookWithStore(songs, 'dashboard');
    const expectedTopAlbums = [...dashboard.albums]
      .sort((a, b) => {
        const playsA = songs.filter(s => s.album === a.name).reduce((sum, s) => sum + (s.playCount || 0), 0);
        const playsB = songs.filter(s => s.album === b.name).reduce((sum, s) => sum + (s.playCount || 0), 0);
        return playsB - playsA;
      })
      .slice(0, 12)
      .map(a => a.name);
    expect(dashboard.topAlbumsByPlays.map(a => a.name)).toEqual(expectedTopAlbums);
    expect(dashboard.topArtistsByPlays).toHaveLength(12);
    expect(dashboard.recentlyPlayedTracks).toEqual([]);
    expect(dashboard.topArtistsByLibrary).toEqual([]);
    expect(dashboard.coverWallAlbums).toEqual([]);
    expect(dashboard.spotlightItem).toBeNull();
    expect(dashboard.quickTiles.map(t => t.id)).toEqual(expectedRecentIds.slice(0, 6));

    const coverWall = await renderHookWithStore(songs, 'coverWall');
    const expectedCoverWall = [...coverWall.albums]
      .sort((a, b) => Number(Boolean(b.coverUrl)) - Number(Boolean(a.coverUrl)))
      .slice(0, 16)
      .map(a => a.name);
    expect(coverWall.coverWallAlbums.map(a => a.name)).toEqual(expectedCoverWall);
    expect(coverWall.spotlightItem).toBeNull(); // >= 4 coverWall albums
    expect(coverWall.recentlyPlayedTracks).toEqual([]);
    expect(coverWall.topAlbumsByPlays).toEqual([]);
    expect(coverWall.topArtistsByPlays).toEqual([]);
    expect(coverWall.quickTiles.map(t => t.id)).toEqual(expectedRecentIds.slice(0, 6));

    const smallCoverWall = await renderHookWithStore(songs.slice(0, 3), 'coverWall');
    expect(smallCoverWall.coverWallAlbums).toHaveLength(3);
    expect(smallCoverWall.spotlightItem).not.toBeNull();

    // Fallback when no songs have plays
    const unplayedSongs = songs.slice(0, 15).map(s => ({ ...s, playCount: 0, lastPlayed: 0 }));
    const unplayedDashboard = await renderHookWithStore(unplayedSongs, 'dashboard');
    expect(unplayedDashboard.topAlbumsByPlays).toEqual(unplayedDashboard.albums.slice(0, 12));
    expect(unplayedDashboard.topArtistsByPlays).toEqual(unplayedDashboard.artists.slice(0, 12));
  });

  it('benchmarks 50,000 synthetic tracks for generateSmartMixes, bounded vs full-sort, and useHomeContent', async () => {
    const songs = makeSyntheticLibrary(50_000);

    const mixStart = performance.now();
    const mixes = generateSmartMixes(songs);
    const mixMs = performance.now() - mixStart;
    expect(mixes.length).toBe(7);

    // Compare full-sort recent tracks vs bounded top-20 via shelves
    const fullSortStart = performance.now();
    const fullRecentIds = [...songs]
      .filter(s => s.lastPlayed && s.lastPlayed > 0)
      .sort((a, b) => (b.lastPlayed || 0) - (a.lastPlayed || 0))
      .slice(0, 20)
      .map(s => s.id);
    const fullSortMs = performance.now() - fullSortStart;

    const timings: Record<HomeLayoutVariant, number> = {
      shelves: 0,
      coverWall: 0,
      dashboard: 0,
    };

    let shelvesIds: string[] = [];
    for (const variant of ['shelves', 'coverWall', 'dashboard'] as const) {
      const batch = [...songs];
      const start = performance.now();
      const content = await renderHookWithStore(batch, variant);
      timings[variant] = performance.now() - start;
      expect(content.songs).toHaveLength(50_000);
      if (variant === 'shelves') {
        shelvesIds = content.recentlyPlayedTracks.map(s => s.id);
      }
    }
    expect(shelvesIds).toEqual(fullRecentIds);

    console.info(
      `[50k benchmark] generateSmartMixes=${mixMs.toFixed(1)}ms | fullSortRecent=${fullSortMs.toFixed(1)}ms | shelves=${timings.shelves.toFixed(1)}ms | coverWall=${timings.coverWall.toFixed(1)}ms | dashboard=${timings.dashboard.toFixed(1)}ms`,
    );
  });
});

describe('librarySlice scan mix debounce', () => {
  afterEach(() => {
    vi.useRealTimers();
  });

  it('coalesces scheduleMixes to 1500ms while scanning and 400ms when idle', async () => {
    vi.useFakeTimers();
    const { createLibrarySlice } = await import('../slices/librarySlice');
    let state: Record<string, any> = {};
    const set = (update: any) => {
      const patch = typeof update === 'function' ? update(state) : update;
      Object.assign(state, patch);
    };
    const get = () => state as any;
    const slice = createLibrarySlice(set as never, get as never, {} as never);
    state = { ...state, ...slice, backendAvailable: true, isScanning: true, queue: [{ id: 'existing' }] };

    state.addSongs([{
      id: 'scan-1',
      title: 'Scan 1',
      artist: 'Artist',
      album: 'Album',
      duration: 120,
      url: '/audio/scan-1',
      addedAt: 1,
    }]);

    await vi.advanceTimersByTimeAsync(500);
    expect(state.smartMixes).toEqual([]);
    await vi.advanceTimersByTimeAsync(1000);
    expect(state.smartMixes.length).toBeGreaterThan(0);

    // Idle uses 400ms debounce
    state.smartMixes = [];
    state.isScanning = false;
    state.addSongs([{
      id: 'idle-1',
      title: 'Idle 1',
      artist: 'Artist',
      album: 'Album',
      duration: 120,
      url: '/audio/idle-1',
      addedAt: 2,
    }]);
    await vi.advanceTimersByTimeAsync(399);
    expect(state.smartMixes).toEqual([]);
    await vi.advanceTimersByTimeAsync(1);
    expect(state.smartMixes.length).toBeGreaterThan(0);
  });
});
