/**
 * ViiB MediaHub - Global State Store
 * 
 * Zustand-based state management combining multiple slices:
 * - PlayerSlice: Playback state, queue, audio settings, EQ
 * - LibrarySlice: Songs, playlists, smart mixes, metadata cache
 * - SpotifySlice: OAuth tokens, user profile, Spotify integration
 * - UISlice: Context menus, dialogs, download count, panel states
 * 
 * Persistence:
 * - Audio settings and UI preferences persisted to localStorage
 * - Spotify client ID and non-sensitive preferences persisted to localStorage; secrets and tokens remain memory-only
 * - Song library backed by SQLite (via Go backend); IndexedDB used as fallback in browser-only mode
 * 
 * Selectors:
 * - useAlbums: Derives album list with song counts from songs
 * - useArtists: Derives artist list with intelligent name splitting
 * - useAlbumCovers: Maps album names to cover URLs
 * 
 * @module store
 */

import { useMemo } from 'react';
import { create } from 'zustand';
import { persist } from 'zustand/middleware';
import { Album, Artist } from './types';
import { AppState } from './slices/types';
import { createPlayerSlice } from './slices/playerSlice';
import { createLibrarySlice } from './slices/librarySlice';
import { createSpotifySlice } from './slices/spotifySlice';
import { createUISlice } from './slices/uiSlice';
import { createAIDJSlice } from './slices/aiDjSlice';
import { createDJMixerSlice } from './slices/djMixerSlice';

export const useStore = create<AppState>()(
  persist(
    (...a) => ({
      ...createPlayerSlice(...a),
      ...createLibrarySlice(...a),
      ...createSpotifySlice(...a),
      ...createUISlice(...a),
      ...createAIDJSlice(...a),
      ...createDJMixerSlice(...a),
    }),
    {
      name: 'mediahub-storage',
      version: 2, // Removes legacy renderer secrets and persists Spotify playback preferences
      migrate: (persistedState: any) => {
        const migrated = { ...(persistedState || {}) };
        delete migrated.spotifyClientSecret;
        return migrated;
      },
      // We do NOT persist 'songs' here anymore because they are in IndexedDB
      partialize: (state) => ({ 
          audioSettings: state.audioSettings,
          showSmartMixes: state.showSmartMixes,
          homeLayoutVariant: state.homeLayoutVariant,
          hasCompletedSetup: state.hasCompletedSetup,
          spotifyClientId: state.spotifyClientId,
          // NOTE: spotifyAccessToken, spotifyRefreshToken, and spotifyTokenExpiry
          // are intentionally NOT persisted to localStorage to avoid XSS token theft.
          // Tokens are held in-memory only; re-auth occurs on app restart.
          spotifyUser: state.spotifyUser,
          streamingEnabled: state.streamingEnabled,
          streamingQuality: state.streamingQuality,
          preferLocalPlayback: state.preferLocalPlayback,
          // AI DJ state
          aiDjPrompt: state.aiDjPrompt,
          aiDjGeneratedSongs: state.aiDjGeneratedSongs,
          aiDjFilter: state.aiDjFilter,
          aiDjDiscoverMode: state.aiDjDiscoverMode,
          aiDjAvoidRecentlyHours: state.aiDjAvoidRecentlyHours,
          aiDjOnePerArtist: state.aiDjOnePerArtist,
          aiDjUseTimeContext: state.aiDjUseTimeContext,
          // DJ Mixer settings (persist mixer preferences, not track state)
          djMixer: state.djMixer,
      }),
      // Deep merge audioSettings to preserve nested values properly
      merge: (persistedState: any, currentState: AppState) => {
        const persisted = persistedState as Partial<AppState>;
        console.log('🔄 Zustand: Rehydrating state from localStorage', { 
          persistedAudioSettings: persisted?.audioSettings,
          currentAudioSettings: currentState.audioSettings 
        });
        return {
          ...currentState,
          ...persisted,
          // Deep merge audioSettings to handle nested properties
          audioSettings: {
            ...currentState.audioSettings,
            ...(persisted?.audioSettings || {})
          }
        };
      },
      onRehydrateStorage: () => (state) => {
        if (state) {
          console.log('✅ Zustand: Store rehydrated successfully', { 
            audioSettings: state.audioSettings 
          });
        } else {
          console.log('⚠️ Zustand: No persisted state found');
        }
      }
    }
  )
);

// --- Selectors ---

// Plex artwork URLs carry a PMS-derived version query. Besides cache busting,
// this lets aggregation preserve PMS artwork if the same logical album is also
// present from a local source.
const isVersionedPlexArtwork = (coverUrl?: string) => Boolean(coverUrl && /^\/api\/cover\/[^?]+\?v=/.test(coverUrl));

export const useAlbums = () => {
  const songs = useStore((state) => state.songs);
  return useMemo(() => {
    const albumsMap = new Map<string, Album>();

    songs.forEach((song) => {
      const artist = song.albumArtist || song.artist || 'Unknown Artist';
      const key = `${song.album}::${artist}`;
      const existing = albumsMap.get(key);
      if (existing) {
        existing.songCount += 1;
        existing.addedAt = Math.max(existing.addedAt || 0, song.addedAt || 0);
        if (song.coverUrl && (!existing.coverUrl || isVersionedPlexArtwork(song.coverUrl))) {
          existing.coverUrl = song.coverUrl;
        }
        return;
      }
      albumsMap.set(key, {
        name: song.album || 'Unknown Album',
        artist,
        songCount: 1,
        coverUrl: song.coverUrl,
        addedAt: song.addedAt || 0,
      });
    });

    return Array.from(albumsMap.values()).sort(
      (a, b) => (b.addedAt || 0) - (a.addedAt || 0) || a.name.localeCompare(b.name) || a.artist.localeCompare(b.artist),
    );
  }, [songs]);
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
  return useMemo(() => {
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
  }, [songs]);
};

export const useAlbumCovers = () => {
  const songs = useStore((state) => state.songs);
  return useMemo(() => {
    const covers: Record<string, string> = {};
    songs.forEach((song) => {
      if (!song.coverUrl) return;
      const artist = song.albumArtist || song.artist || 'Unknown Artist';
      const composite = `${song.album}::${artist}`;
      if (!covers[composite] || isVersionedPlexArtwork(song.coverUrl)) covers[composite] = song.coverUrl;
      if (!covers[song.album] || isVersionedPlexArtwork(song.coverUrl)) covers[song.album] = song.coverUrl;
    });
    return covers;
  }, [songs]);
};