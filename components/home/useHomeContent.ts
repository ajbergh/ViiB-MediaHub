import React, { useMemo } from 'react';
import { useNavigate } from 'react-router-dom';
import { useAlbums, useArtists, useStore } from '../../store';
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

export const useHomeContent = (): HomeContent => {
  const navigate = useNavigate();
  const {
    songs,
    smartMixes,
    playSong,
    openContextMenu,
    showSmartMixes,
    artistMetadata,
  } = useStore();
  const albums = useAlbums();
  const artists = useArtists();

  const albumByName = useMemo(() => {
    const map = new Map<string, Album>();
    albums.forEach((album) => {
      if (!map.has(album.name)) {
        map.set(album.name, album);
      }
    });
    return map;
  }, [albums]);

  const recentlyPlayedTracks = useMemo(() => {
    return [...songs]
      .filter((song) => song.lastPlayed && song.lastPlayed > 0)
      .sort((a, b) => (b.lastPlayed || 0) - (a.lastPlayed || 0))
      .slice(0, 20);
  }, [songs]);

  const recentlyPlayedAlbums = useMemo(() => {
    const seen = new Set<string>();
    const result: Album[] = [];

    recentlyPlayedTracks.forEach((song) => {
      const album = albumByName.get(song.album);
      if (!album) return;

      const key = getAlbumKey(album);
      if (seen.has(key)) return;

      seen.add(key);
      result.push(album);
    });

    return result.slice(0, 10);
  }, [albumByName, recentlyPlayedTracks]);

  const recentlyAddedAlbums = useMemo(() => albums.slice(0, 12), [albums]);

  const topAlbumsByPlays = useMemo(() => {
    const playsByAlbum = new Map<string, number>();

    songs.forEach((song) => {
      playsByAlbum.set(song.album, (playsByAlbum.get(song.album) || 0) + (song.playCount || 0));
    });

    const ranked = albums
      .map((album) => ({ album, plays: playsByAlbum.get(album.name) || 0 }))
      .filter((entry) => entry.plays > 0)
      .sort((a, b) => b.plays - a.plays)
      .map((entry) => entry.album);

    return (ranked.length > 0 ? ranked : albums).slice(0, 12);
  }, [albums, songs]);

  const topArtistsByLibrary = useMemo(() => artists.slice(0, 12), [artists]);

  const topArtistsByPlays = useMemo(() => {
    const ranked = artists
      .map((artist) => {
        const artistName = artist.name.toLowerCase();
        const plays = songs.reduce((total, song) => {
          return song.artist.toLowerCase().includes(artistName) ? total + (song.playCount || 0) : total;
        }, 0);
        return { artist, plays };
      })
      .filter((entry) => entry.plays > 0)
      .sort((a, b) => b.plays - a.plays)
      .map((entry) => entry.artist);

    return (ranked.length > 0 ? ranked : artists).slice(0, 12);
  }, [artists, songs]);

  const coverWallAlbums = useMemo(() => {
    return [...albums]
      .sort((a, b) => Number(Boolean(b.coverUrl)) - Number(Boolean(a.coverUrl)))
      .slice(0, 16);
  }, [albums]);

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
    const album = recentlyPlayedAlbums[0] || recentlyAddedAlbums[0] || topAlbumsByPlays[0];
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

    const artist = topArtistsByLibrary[0];
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
  }, [artistMetadata, recentlyAddedAlbums, recentlyPlayedAlbums, showSmartMixes, smartMixes, topAlbumsByPlays, topArtistsByLibrary]);

  const quickTiles = useMemo<HomeQuickTileItem[]>(() => {
    const trackTiles = recentlyPlayedTracks.slice(0, 6).map((song) => ({
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
  }, [recentlyAddedAlbums, recentlyPlayedTracks, songs]);

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
