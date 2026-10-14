/**
 * ViiB MediaHub - Global State Store
 * 
 * Zustand-based state management combining multiple slices:
 * - PlayerSlice: Playback state, queue, audio settings, EQ
 * - LibrarySlice: Songs, playlists, smart mixes, metadata cache
 * - SpotifySlice: Backend session state, account generation, profile, and legacy OAuth fields
 * - UISlice: Context menus, dialogs, download count, panel states
 * 
 * Persistence:
 * - Audio settings and UI preferences persisted to localStorage
 * - Spotify client ID and non-sensitive preferences persisted to localStorage; secrets and tokens are excluded from renderer persistence; connection status is restored from the backend
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
import { splitArtistNames } from './lib/artistNames';
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
import { buildAlbumCoverIndex, isAuthoritativePlexArtwork, isPlexSourcePath } from './lib/artwork';

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
      // v2: removes legacy renderer secrets and persists Spotify playback preferences.
      // v3: DJv2 workstation defaults to the performance layout. 'fx' was the old
      //     default, so returning users are moved to 'perf' once.
      // v4: persists desktop close behavior.
      version: 5,
      migrate: (persistedState: any, version: number) => {
        const migrated = { ...(persistedState || {}) };
        delete migrated.spotifyClientSecret;
        delete migrated.spotifyUser;
        delete migrated.spotifyAccessToken;
        delete migrated.spotifyRefreshToken;
        delete migrated.spotifyTokenExpiry;
        delete migrated.spotifyConnected;
        if (version < 3 && migrated.djMixer?.djLayoutMode === 'fx') {
          migrated.djMixer = { ...migrated.djMixer, djLayoutMode: 'perf' };
        }
        return migrated;
      },
      // We do NOT persist 'songs' here anymore because they are in IndexedDB
      partialize: (state) => ({ 
          audioSettings: state.audioSettings,
          showSmartMixes: state.showSmartMixes,
          homeLayoutVariant: state.homeLayoutVariant,
          hasCompletedSetup: state.hasCompletedSetup,
          isSkinnyMode: state.isSkinnyMode,
          isSkinnyAlwaysOnTop: state.isSkinnyAlwaysOnTop,
          windowCloseAction: state.windowCloseAction,
          spotifyClientId: state.spotifyClientId,
          // NOTE: spotifyAccessToken, spotifyRefreshToken, and spotifyTokenExpiry
          // are intentionally NOT persisted to renderer localStorage. The backend
          // owns encrypted session persistence and restores valid tokens at startup.

          streamingEnabled: state.streamingEnabled,
          streamingQuality: state.streamingQuality,
          preferLocalPlayback: state.preferLocalPlayback,
          // AI DJ state
          aiDjPrompt: state.aiDjPrompt,
          aiDjGeneratedSongs: state.aiDjGeneratedSongs,
          aiDjFilter: state.aiDjFilter,
          aiDjValidation: state.aiDjValidation,
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

type DerivedAlbum = Album & { plexBacked?: boolean };

let cachedAlbumsSongsRef: AppState['songs'] | null = null;
let cachedAlbumsResult: DerivedAlbum[] = [];

const computeAlbums = (songs: AppState['songs']): DerivedAlbum[] => {
  if (songs === cachedAlbumsSongsRef) {
    return cachedAlbumsResult;
  }

  const albumsMap = new Map<string, DerivedAlbum>();

  for (let i = 0; i < songs.length; i++) {
    const song = songs[i];
    const artist = song.albumArtist || song.artist || 'Unknown Artist';
    const key = `${song.album}::${artist}`;
    const plexBacked = isPlexSourcePath(song.path);
    const existing = albumsMap.get(key);
    if (existing) {
      existing.songCount += 1;
      existing.addedAt = Math.max(existing.addedAt || 0, song.addedAt || 0);
      if (plexBacked) {
        // Once a logical album has a Plex source, Plex's artwork decision is
        // authoritative. The first Plex row deliberately clears a local cover
        // if PMS has no artwork; later Plex rows may supply that PMS cover.
        if (!existing.plexBacked) {
          existing.coverUrl = song.coverUrl;
        } else if (song.coverUrl && (!existing.coverUrl || isAuthoritativePlexArtwork(song.coverUrl))) {
          existing.coverUrl = song.coverUrl;
        }
        existing.plexBacked = true;
      } else if (!existing.plexBacked && !existing.coverUrl && song.coverUrl) {
        existing.coverUrl = song.coverUrl;
      }
      continue;
    }
    albumsMap.set(key, {
      name: song.album || 'Unknown Album',
      artist,
      songCount: 1,
      coverUrl: song.coverUrl,
      addedAt: song.addedAt || 0,
      plexBacked,
    });
  }

  cachedAlbumsSongsRef = songs;
  cachedAlbumsResult = Array.from(albumsMap.values()).sort(
    (a, b) => (b.addedAt || 0) - (a.addedAt || 0) || a.name.localeCompare(b.name) || a.artist.localeCompare(b.artist),
  );
  return cachedAlbumsResult;
};

export const useAlbums = () => {
  const songs = useStore((state) => state.songs);
  return useMemo(() => computeAlbums(songs), [songs]);
};

let cachedArtistsSongsRef: AppState['songs'] | null = null;
let cachedArtistsResult: Artist[] = [];

const computeArtists = (songs: AppState['songs']): Artist[] => {
  if (songs === cachedArtistsSongsRef) {
    return cachedArtistsResult;
  }

  const artistMap = new Map<string, Artist & { albums: Set<string> }>();

  for (let i = 0; i < songs.length; i++) {
    const song = songs[i];
    const artistNames = splitArtistNames(song.artist);

    for (const artistName of artistNames) {
      let artist = artistMap.get(artistName);
      if (!artist) {
        artist = {
          name: artistName,
          songCount: 0,
          albumCount: 0,
          albums: new Set(),
          imageUrl: song.coverUrl,
        };
        artistMap.set(artistName, artist);
      }
      artist.songCount++;
      artist.albums.add(song.album);
      if (!artist.imageUrl && song.coverUrl) {
        artist.imageUrl = song.coverUrl;
      }
    }
  }

  cachedArtistsSongsRef = songs;
  cachedArtistsResult = Array.from(artistMap.values())
    .map((a) => ({
      ...a,
      albumCount: a.albums.size,
    }))
    .sort((a, b) => b.songCount - a.songCount);
  return cachedArtistsResult;
};

export const useArtists = () => {
  const songs = useStore((state) => state.songs);
  return useMemo(() => computeArtists(songs), [songs]);
};

let cachedCoversAlbumsRef: DerivedAlbum[] | null = null;
let cachedCoversResult: Record<string, string> = {};

export const useAlbumCovers = () => {
  const albums = useAlbums();
  return useMemo(() => {
    if (albums === cachedCoversAlbumsRef) {
      return cachedCoversResult;
    }
    cachedCoversAlbumsRef = albums;
    cachedCoversResult = buildAlbumCoverIndex(albums);
    return cachedCoversResult;
  }, [albums]);
};

