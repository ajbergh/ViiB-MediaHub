/** Derives Home shelves, spotlight items, shortcuts, and statistics from application state. */

import React, { useMemo } from 'react';
import { useNavigate } from 'react-router';
import { useAlbums, useArtists, useStore } from '../../store';
import { splitArtistNames } from '../../lib/artistNames';
import { Album, Artist, ContextMenuType, SmartMix, Song } from '../../types';

export type SpotlightKind = 'album' | 'artist' | 'smartMix';

export interface HomeSpotlightItem {
  kind: SpotlightKind;
  title: string;
  subtitle: string;
  description: string;
  imageUrl?: string;
  fallbackSeed: string;
  coverColors?: string[];
  onClick: () => void;
  onPlay?: () => void;
}

export interface HomeQuickTileItem {
  id: string;
  title: string;
  subtitle: string;
  imageUrl?: string;
  fallbackSeed: string;
  onClick: () => void;
  onPlay?: () => void;
  onContextMenu?: (e: React.MouseEvent) => void;
}

export interface HomeContent {
  songs: Song[];
  albums: Album[];
  artists: Artist[];
  smartMixes: SmartMix[];
  showSmartMixes: boolean;
  recentlyPlayedTracks: Song[];
  recentlyPlayedAlbums: Album[];
  recentlyAddedAlbums: Album[];
  topAlbumsByPlays: Album[];
  topArtistsByLibrary: Artist[];
  topArtistsByPlays: Artist[];
  coverWallAlbums: Album[];
  quickTiles: HomeQuickTileItem[];
  spotlightItem: HomeSpotlightItem | null;
  formatRelativeTime: (timestamp: number) => string;
  playTrack: (song: Song) => void;
  playAlbum: (album: Album) => void;
  playArtist: (artist: Artist) => void;
  playMix: (mix: SmartMix) => void;
  navigateToAlbum: (album: Album) => void;
  navigateToArtist: (artist: Artist) => void;
  navigateToSmartMix: (mix: SmartMix) => void;
  getArtistImage: (artist: Artist) => string | undefined;
  openAlbumMenu: (e: React.MouseEvent, album: Album) => void;
  openArtistMenu: (e: React.MouseEvent, artist: Artist) => void;
  openSongMenu: (e: React.MouseEvent, song: Song) => void;
  openSmartMixMenu: (e: React.MouseEvent, mix: SmartMix) => void;
}

const getAlbumKey = (album: Album) => `${album.name}::${album.artist}`;

const sortSongsForAlbum = (songs: Song[]) =>
  [...songs].sort((a, b) => {
    const discDiff = (a.discNumber || 0) - (b.discNumber || 0);
    if (discDiff !== 0) return discDiff;
    return (a.trackNumber || 0) - (b.trackNumber || 0);
  });

interface HeapEntry<T> {
  item: T;
  index: number;
}

function selectTopK<T>(
  items: readonly T[],
  k: number,
  compare: (a: T, b: T) => number,
  include?: (item: T) => boolean,
): T[] {
  if (k <= 0 || items.length === 0) return [];

  // Max-eviction heap: root is the worst element among the current top K
  // (larger according to `compare`, breaking ties by larger original index).
  const heap: HeapEntry<T>[] = [];
  const isWorse = (a: HeapEntry<T>, b: HeapEntry<T>) => {
    const cmp = compare(a.item, b.item);
    return cmp > 0 || (cmp === 0 && a.index > b.index);
  };

  for (let i = 0; i < items.length; i++) {
    const item = items[i];
    if (include && !include(item)) continue;

    if (heap.length < k) {
      heap.push({ item, index: i });
      let idx = heap.length - 1;
      while (idx > 0) {
        const parent = (idx - 1) >> 1;
        if (isWorse(heap[idx], heap[parent])) {
          const tmp = heap[idx];
          heap[idx] = heap[parent];
          heap[parent] = tmp;
          idx = parent;
        } else {
          break;
        }
      }
    } else if (compare(item, heap[0].item) < 0) {
      heap[0] = { item, index: i };
      let idx = 0;
      const len = heap.length;
      while (true) {
        const left = (idx << 1) + 1;
        const right = left + 1;
        let worst = idx;
        if (left < len && isWorse(heap[left], heap[worst])) worst = left;
        if (right < len && isWorse(heap[right], heap[worst])) worst = right;
        if (worst === idx) break;
        const tmp = heap[idx];
        heap[idx] = heap[worst];
        heap[worst] = tmp;
        idx = worst;
      }
    }
  }

  heap.sort((a, b) => compare(a.item, b.item) || a.index - b.index);
  return heap.map((entry) => entry.item);
}

export const useHomeContent = (): HomeContent => {
  const navigate = useNavigate();
  const songs = useStore(state => state.songs);
  const smartMixes = useStore(state => state.smartMixes);
  const playSong = useStore(state => state.playSong);
  const openContextMenu = useStore(state => state.openContextMenu);
  const showSmartMixes = useStore(state => state.showSmartMixes);
  const artistMetadata = useStore(state => state.artistMetadata);
  const homeLayoutVariant = useStore(state => state.homeLayoutVariant);
  const albums = useAlbums();
  const artists = useArtists();

  const computeAll = homeLayoutVariant == null;
  const needsFullRecentTracks = computeAll || homeLayoutVariant === 'shelves';
  const needsTopAlbumsByPlays = computeAll || homeLayoutVariant === 'dashboard';
  const needsTopArtistsByLibrary = computeAll || homeLayoutVariant === 'shelves' || homeLayoutVariant === 'coverWall';
  const needsTopArtistsByPlays = computeAll || homeLayoutVariant === 'dashboard';
  const needsCoverWallAlbums = computeAll || homeLayoutVariant === 'coverWall';

  const recentTracksLimit = needsFullRecentTracks ? 20 : 6;

  const recentTracksTop = useMemo(
    () =>
      selectTopK(
        songs,
        recentTracksLimit,
        (a, b) => (b.lastPlayed || 0) - (a.lastPlayed || 0),
        (song) => Boolean(song.lastPlayed && song.lastPlayed > 0),
      ),
    [recentTracksLimit, songs],
  );

  const recentlyPlayedTracks = useMemo(
    () => (needsFullRecentTracks ? recentTracksTop : []),
    [needsFullRecentTracks, recentTracksTop],
  );

  const recentlyAddedAlbums = useMemo(() => albums.slice(0, 12), [albums]);

  const topAlbumsByPlays = useMemo(() => {
    if (!needsTopAlbumsByPlays) return [];

    const playsByAlbum = new Map<string, number>();
    for (let i = 0; i < songs.length; i++) {
      const song = songs[i];
      const plays = song.playCount || 0;
      if (plays > 0) {
        playsByAlbum.set(song.album, (playsByAlbum.get(song.album) || 0) + plays);
      }
    }

    if (playsByAlbum.size === 0) {
      return albums.slice(0, 12);
    }

    const ranked = selectTopK(
      albums,
      12,
      (a, b) => (playsByAlbum.get(b.name) || 0) - (playsByAlbum.get(a.name) || 0),
      (album) => (playsByAlbum.get(album.name) || 0) > 0,
    );

    return ranked.length > 0 ? ranked : albums.slice(0, 12);
  }, [albums, needsTopAlbumsByPlays, songs]);

  const topArtistsByLibrary = useMemo(
    () => (needsTopArtistsByLibrary ? artists.slice(0, 12) : []),
    [artists, needsTopArtistsByLibrary],
  );

  const topArtistsByPlays = useMemo(() => {
    if (!needsTopArtistsByPlays) return [];

    const playsByArtist = new Map<string, number>();
    const namesByCredit = new Map<string, Set<string>>();
    for (let i = 0; i < songs.length; i++) {
      const song = songs[i];
      const plays = song.playCount || 0;
      if (plays <= 0) continue;

      const credit = song.artist;
      let names = namesByCredit.get(credit);
      if (!names) {
        names = new Set(splitArtistNames(credit).map(name => name.toLowerCase()));
        namesByCredit.set(credit, names);
      }
      for (const name of names) {
        playsByArtist.set(name, (playsByArtist.get(name) || 0) + plays);
      }
    }

    if (playsByArtist.size === 0) {
      return artists.slice(0, 12);
    }

    const ranked = selectTopK(
      artists,
      12,
      (a, b) => (playsByArtist.get(b.name.toLowerCase()) || 0) - (playsByArtist.get(a.name.toLowerCase()) || 0),
      (artist) => (playsByArtist.get(artist.name.toLowerCase()) || 0) > 0,
    );

    return ranked.length > 0 ? ranked : artists.slice(0, 12);
  }, [artists, needsTopArtistsByPlays, songs]);

  const coverWallAlbums = useMemo(() => {
    if (!needsCoverWallAlbums) return [];

    const withCover: Album[] = [];
    const withoutCover: Album[] = [];
    for (let i = 0; i < albums.length; i++) {
      const album = albums[i];
      if (album.coverUrl) {
        withCover.push(album);
        if (withCover.length === 16) return withCover;
      } else if (withCover.length + withoutCover.length < 16) {
        withoutCover.push(album);
      }
    }

    const remaining = 16 - withCover.length;
    return remaining > 0 ? withCover.concat(withoutCover.slice(0, remaining)) : withCover;
  }, [albums, needsCoverWallAlbums]);

  const needsSpotlight =
    computeAll ||
    homeLayoutVariant === 'shelves' ||
    (homeLayoutVariant === 'coverWall' && coverWallAlbums.length < 4);

  const recentlyPlayedAlbums = useMemo(() => {
    if (!needsSpotlight || recentTracksTop.length === 0 || albums.length === 0) {
      return [];
    }

    const neededNames = new Set<string>();
    for (let i = 0; i < recentTracksTop.length; i++) {
      neededNames.add(recentTracksTop[i].album);
    }

    const matched = new Map<string, Album>();
    for (let i = 0; i < albums.length; i++) {
      const album = albums[i];
      if (neededNames.has(album.name) && !matched.has(album.name)) {
        matched.set(album.name, album);
        if (matched.size === neededNames.size) break;
      }
    }

    const seen = new Set<string>();
    const result: Album[] = [];
    for (let i = 0; i < recentTracksTop.length; i++) {
      const album = matched.get(recentTracksTop[i].album);
      if (!album) continue;

      const key = getAlbumKey(album);
      if (seen.has(key)) continue;

      seen.add(key);
      result.push(album);
      if (result.length === 10) break;
    }

    return result;
  }, [albums, needsSpotlight, recentTracksTop]);

  const formatRelativeTime = (timestamp: number): string => {
    const now = Date.now();
    const diff = now - timestamp;
    const minutes = Math.floor(diff / 60000);
    const hours = Math.floor(diff / 3600000);
    const days = Math.floor(diff / 86400000);

    if (minutes < 1) return 'Just now';
    if (minutes < 60) return `${minutes}m ago`;
    if (hours < 24) return `${hours}h ago`;
    if (days === 1) return 'Yesterday';
    if (days < 7) return `${days}d ago`;
    return new Date(timestamp).toLocaleDateString();
  };

  const navigateToAlbum = (album: Album) => {
    navigate(`/album/${encodeURIComponent(album.name)}/${encodeURIComponent(album.artist)}`);
  };

  const navigateToArtist = (artist: Artist) => {
    navigate(`/artist/${encodeURIComponent(artist.name)}`);
  };

  const navigateToSmartMix = (mix: SmartMix) => {
    navigate(`/smart-mix/${mix.id}`);
  };

  const getArtistImage = (artist: Artist) => artistMetadata[artist.name]?.imageUrl || artist.imageUrl;

  const playAlbum = (album: Album) => {
    const albumSongs = sortSongsForAlbum(songs.filter((song) => song.album === album.name));
    if (albumSongs.length > 0) {
      void playSong(albumSongs[0], albumSongs);
    }
  };

  const playTrack = (song: Song) => {
    void playSong(song);
  };

  const playArtist = (artist: Artist) => {
    const artistName = artist.name.toLowerCase();
    const artistSongs = songs.filter((song) => song.artist.toLowerCase().includes(artistName));
    if (artistSongs.length > 0) {
      void playSong(artistSongs[0], artistSongs);
    }
  };

  const playMix = (mix: SmartMix) => {
    const mixSongs = songs.filter((song) => mix.songIds.includes(song.id));
    if (mixSongs.length > 0) {
      void playSong(mixSongs[0], mixSongs);
    }
  };

  const openAlbumMenu = (e: React.MouseEvent, album: Album) => {
    openContextMenu(e, ContextMenuType.ALBUM, album);
  };

  const openArtistMenu = (e: React.MouseEvent, artist: Artist) => {
    openContextMenu(e, ContextMenuType.ARTIST, artist);
  };

  const openSongMenu = (e: React.MouseEvent, song: Song) => {
    openContextMenu(e, ContextMenuType.SONG, song);
  };

  const openSmartMixMenu = (e: React.MouseEvent, mix: SmartMix) => {
    openContextMenu(e, ContextMenuType.SMART_MIX, mix);
  };

  const spotlightItem = useMemo<HomeSpotlightItem | null>(() => {
    if (!needsSpotlight) return null;

    const album = recentlyPlayedAlbums[0] || recentlyAddedAlbums[0] || topAlbumsByPlays[0] || albums[0];
    if (album) {
      return {
        kind: 'album',
        title: album.name,
        subtitle: album.artist,
        description: `${album.songCount} ${album.songCount === 1 ? 'track' : 'tracks'} ready from your library.`,
        imageUrl: album.coverUrl,
        fallbackSeed: album.name,
        onClick: () => navigateToAlbum(album),
        onPlay: () => playAlbum(album),
      };
    }

    const artist = topArtistsByLibrary[0] || artists[0];
    if (artist) {
      return {
        kind: 'artist',
        title: artist.name,
        subtitle: 'Featured artist',
        description: `${artist.songCount} ${artist.songCount === 1 ? 'song' : 'songs'} across ${artist.albumCount} ${artist.albumCount === 1 ? 'album' : 'albums'}.`,
        imageUrl: artistMetadata[artist.name]?.imageUrl || artist.imageUrl,
        fallbackSeed: artist.name,
        onClick: () => navigateToArtist(artist),
        onPlay: () => playArtist(artist),
      };
    }

    const mix = showSmartMixes ? smartMixes[0] : undefined;
    if (mix) {
      return {
        kind: 'smartMix',
        title: mix.name,
        subtitle: 'Smart Mix',
        description: mix.description,
        fallbackSeed: mix.name,
        coverColors: mix.coverColors,
        onClick: () => navigateToSmartMix(mix),
        onPlay: () => playMix(mix),
      };
    }

    return null;
  }, [albums, artistMetadata, artists, needsSpotlight, recentlyAddedAlbums, recentlyPlayedAlbums, showSmartMixes, smartMixes, topAlbumsByPlays, topArtistsByLibrary]);

  const quickTiles = useMemo<HomeQuickTileItem[]>(() => {
    const trackTiles = recentTracksTop.slice(0, 6).map((song) => ({
      id: song.id,
      title: song.title,
      subtitle: `${song.artist} • ${formatRelativeTime(song.lastPlayed || Date.now())}`,
      imageUrl: song.coverUrl,
      fallbackSeed: song.album,
      onClick: () => {
        void playSong(song);
      },
      onPlay: () => {
        void playSong(song);
      },
      onContextMenu: (e: React.MouseEvent) => openSongMenu(e, song),
    }));

    if (trackTiles.length > 0) return trackTiles;

    return recentlyAddedAlbums.slice(0, 6).map((album) => ({
      id: getAlbumKey(album),
      title: album.name,
      subtitle: album.artist,
      imageUrl: album.coverUrl,
      fallbackSeed: album.name,
      onClick: () => navigateToAlbum(album),
      onPlay: () => playAlbum(album),
      onContextMenu: (e: React.MouseEvent) => openAlbumMenu(e, album),
    }));
  }, [recentTracksTop, recentlyAddedAlbums, songs]);

  return {
    songs,
    albums,
    artists,
    smartMixes,
    showSmartMixes,
    recentlyPlayedTracks,
    recentlyPlayedAlbums,
    recentlyAddedAlbums,
    topAlbumsByPlays,
    topArtistsByLibrary,
    topArtistsByPlays,
    coverWallAlbums,
    quickTiles,
    spotlightItem,
    formatRelativeTime,
    playTrack,
    playAlbum,
    playArtist,
    playMix,
    navigateToAlbum,
    navigateToArtist,
    navigateToSmartMix,
    getArtistImage,
    openAlbumMenu,
    openArtistMenu,
    openSongMenu,
    openSmartMixMenu,
  };
};
