import { create } from 'zustand';
import { persist } from 'zustand/middleware';
import { Album, Artist } from './types';
import { AppState } from './slices/types';
import { createPlayerSlice } from './slices/playerSlice';
import { createLibrarySlice } from './slices/librarySlice';
import { createSpotifySlice } from './slices/spotifySlice';
import { createUISlice } from './slices/uiSlice';

export const useStore = create<AppState>()(
  persist(
    (...a) => ({
      ...createPlayerSlice(...a),
      ...createLibrarySlice(...a),
      ...createSpotifySlice(...a),
      ...createUISlice(...a),
    }),
    {
      name: 'mediahub-storage',
      // We do NOT persist 'songs' here anymore because they are in IndexedDB
      partialize: (state) => ({ 
          audioSettings: state.audioSettings,
          showSmartMixes: state.showSmartMixes,
          spotifyClientId: state.spotifyClientId,
          spotifyClientSecret: state.spotifyClientSecret,
          spotifyAccessToken: state.spotifyAccessToken,
          spotifyRefreshToken: state.spotifyRefreshToken,
          spotifyTokenExpiry: state.spotifyTokenExpiry,
          spotifyUser: state.spotifyUser
      }),
    }
  )
);

// --- Selectors ---

export const useAlbums = () => {
  const songs = useStore((state) => state.songs);
  const albumsMap = new Map<string, Album>();
  
  songs.forEach(song => {
    const key = song.album;
    if (!albumsMap.has(key)) {
      albumsMap.set(key, {
        name: song.album,
        artist: song.albumArtist || song.artist,
        songCount: 0,
        coverUrl: song.coverUrl
      });
    }
    const album = albumsMap.get(key)!;
    album.songCount++;
    if (!album.coverUrl && song.coverUrl) {
        album.coverUrl = song.coverUrl;
    }
  });
  
  return Array.from(albumsMap.values());
};

export const useArtists = () => {
  const songs = useStore((state) => state.songs);
  const artistMap = new Map<string, Artist & { albums: Set<string> }>();

  songs.forEach(song => {
    const artistName = song.artist;
    if (!artistMap.has(artistName)) {
      artistMap.set(artistName, {
        name: artistName,
        songCount: 0,
        albumCount: 0,
        albums: new Set(),
        imageUrl: song.coverUrl
      });
    }
    const artist = artistMap.get(artistName)!;
    artist.songCount++;
    artist.albums.add(song.album);
    if (!artist.imageUrl && song.coverUrl) {
      artist.imageUrl = song.coverUrl;
    }
  });

  return Array.from(artistMap.values()).map(a => ({
    ...a,
    albumCount: a.albums.size
  }));
};

export const useAlbumCovers = () => {
  const songs = useStore((state) => state.songs);
  const covers = new Map<string, string>();
  songs.forEach(song => {
    if (song.coverUrl && !covers.has(song.album)) {
      covers.set(song.album, song.coverUrl);
    }
  });
  return Object.fromEntries(covers);
};
