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
        coverUrl: song.coverUrl,
        addedAt: song.addedAt || 0
      });
    }
    const album = albumsMap.get(key)!;
    album.songCount++;
    if (!album.coverUrl && song.coverUrl) {
        album.coverUrl = song.coverUrl;
    }
    // Track the most recent addedAt for this album
    if (song.addedAt && song.addedAt > (album.addedAt || 0)) {
      album.addedAt = song.addedAt;
    }
  });
  
  // Sort by most recently added first
  return Array.from(albumsMap.values()).sort((a, b) => (b.addedAt || 0) - (a.addedAt || 0));
};

/**
 * Splits an artist string into individual artists.
 * Handles common separators like ", ", " & ", " feat. ", " ft. ", etc.
 */
const splitArtistNames = (artistString: string): string[] => {
  if (!artistString) return [];
  
  // Common separators used in artist fields
  const separators = [
    ' feat. ', ' feat ', ' ft. ', ' ft ', 
    ' featuring ', ' & ', ' x ', ' and ', 
    ', ', ' / ', ' vs. ', ' vs '
  ];
  
  let artists = [artistString];
  
  for (const sep of separators) {
    const newArtists: string[] = [];
    for (const artist of artists) {
      // Case-insensitive split
      const parts = artist.split(new RegExp(sep.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'), 'i'));
      newArtists.push(...parts);
    }
    artists = newArtists;
  }
  
  // Clean up and filter empty/whitespace-only entries
  return artists.map(a => a.trim()).filter(a => a.length > 0);
};

export const useArtists = () => {
  const songs = useStore((state) => state.songs);
  const artistMap = new Map<string, Artist & { albums: Set<string> }>();

  songs.forEach(song => {
    // Split artist field into individual artists
    const artistNames = splitArtistNames(song.artist);
    
    for (const artistName of artistNames) {
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
    }
  });

  // Sort by song count (most popular first)
  return Array.from(artistMap.values())
    .map(a => ({
      ...a,
      albumCount: a.albums.size
    }))
    .sort((a, b) => b.songCount - a.songCount);
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
