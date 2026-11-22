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
import { catalogDeltas, compareLibraryText } from './lib/libraryIndex';
import { create } from 'zustand';
import { createJSONStorage, persist } from 'zustand/middleware';
import { shallow } from 'zustand/shallow';
import { deduplicatingPersistStorage } from './lib/deduplicatingPersistStorage';
import { Album, Artist, MilkdropSettings } from './types';
import { AppState } from './slices/types';
import { createPlayerSlice } from './slices/playerSlice';
import { createLibrarySlice } from './slices/librarySlice';
import { createSpotifySlice } from './slices/spotifySlice';
import { createUISlice } from './slices/uiSlice';
import { createAIDJSlice } from './slices/aiDjSlice';
import { createDJMixerSlice } from './slices/djMixerSlice';
import { buildAlbumCoverIndex, isAuthoritativePlexArtwork, isPlexSourcePath } from './lib/artwork';

let lastMilkdropSettings: MilkdropSettings | undefined;
let milkdropPreferences: MilkdropSettings | undefined;

function persistedMilkdropSettings(settings: MilkdropSettings) {
  // Keep the persisted reference stable across playback ticks and preset changes.
  if (settings !== lastMilkdropSettings || !milkdropPreferences) {
    lastMilkdropSettings = settings;
    const preferences = { ...settings, currentPreset: null };
    if (!milkdropPreferences || !shallow(milkdropPreferences, preferences)) {
      milkdropPreferences = preferences;
    }
  }
  return milkdropPreferences;
}

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
      storage: deduplicatingPersistStorage(createJSONStorage<Partial<AppState>>(() => localStorage)),
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
          visualizerScenes: state.visualizerScenes,
          activeVisualizerSceneId: state.activeVisualizerSceneId,
          artworkOverlayEnabled: state.artworkOverlayEnabled,
          milkdropSettings: persistedMilkdropSettings(state.milkdropSettings),
          partyStageLayout: state.partyStageLayout,
          partyTrackInfoPinned: state.partyTrackInfoPinned,
          partySceneRotationEnabled: state.partySceneRotationEnabled,
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
      merge: (persistedState: unknown, currentState: AppState): AppState => {
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
          },
          milkdropSettings: { ...currentState.milkdropSettings, ...persisted?.milkdropSettings, currentPreset: null },
          // Supply new defaults to old preference objects while preserving an
          // explicit Canvas/WebGL choice saved by a returning user.
          djMixer: { ...currentState.djMixer, ...(persisted?.djMixer || {}) },
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

function groupSongs(songs: AppState['songs'], keys: (song: AppState['songs'][number]) => string[]) {
  const groups = new Map<string, AppState['songs']>();
  for (const song of songs) {
    for (const key of keys(song)) {
      const members = groups.get(key);
      if (members) members.push(song);
      else groups.set(key, [song]);
    }
  }
  return groups;
}

function updateStableGroups(
  songs: AppState['songs'],
  previous: AppState['songs'] | null,
  groups: Map<string, AppState['songs']>,
  keys: (song: AppState['songs'][number]) => string[],
  changed: (before: AppState['songs'][number], after: AppState['songs'][number]) => boolean,
) {
  const delta = catalogDeltas.get(songs);
  if (!delta || delta.previous.deref() !== previous || delta.orderChanged) return null;
  const replacements = new Map<string, AppState['songs'][number]>();
  const affected = new Set<string>();
  for (const { before, after } of delta.changes) {
    if (!before || !after) return null;
    if (!changed(before, after)) continue;
    const oldKeys = keys(before);
    const newKeys = keys(after);
    if (oldKeys.length !== newKeys.length || oldKeys.some((key, index) => key !== newKeys[index])) return null;
    replacements.set(after.id, after);
    for (const key of newKeys) affected.add(key);
  }
  for (const key of affected) {
    const members = groups.get(key)!;
    groups.set(key, members.map(song => replacements.get(song.id) ?? song));
  }
  return affected;
}

const albumArtist = (song: AppState['songs'][number]) => song.albumArtist || song.artist || 'Unknown Artist';
const albumGroupKeys = (song: AppState['songs'][number]) => [`${song.album}::${albumArtist(song)}`];
let cachedAlbumsSongsRef: AppState['songs'] | null = null;
let cachedAlbumsResult: DerivedAlbum[] = [];
let albumGroups = new Map<string, AppState['songs']>();
let albumSummaries = new Map<string, DerivedAlbum>();

function summarizeAlbum(songs: AppState['songs']): DerivedAlbum {
  const first = songs[0];
  const album: DerivedAlbum = {
    name: first.album || 'Unknown Album', artist: albumArtist(first), songCount: songs.length,
    coverUrl: first.coverUrl, addedAt: first.addedAt || 0, plexBacked: isPlexSourcePath(first.path),
  };
  for (let i = 1; i < songs.length; i++) {
    const song = songs[i];
    album.addedAt = Math.max(album.addedAt || 0, song.addedAt || 0);
    if (isPlexSourcePath(song.path)) {
      if (!album.plexBacked || (song.coverUrl && (!album.coverUrl || isAuthoritativePlexArtwork(song.coverUrl)))) {
        album.coverUrl = song.coverUrl;
      }
      album.plexBacked = true;
    } else if (!album.plexBacked && !album.coverUrl && song.coverUrl) {
      album.coverUrl = song.coverUrl;
    }
  }
  return album;
}

const computeAlbums = (songs: AppState['songs']): DerivedAlbum[] => {
  if (songs === cachedAlbumsSongsRef) {
    return cachedAlbumsResult;
  }

  const affected = updateStableGroups(songs, cachedAlbumsSongsRef, albumGroups, albumGroupKeys, (a, b) =>
    a.album !== b.album || albumArtist(a) !== albumArtist(b) || a.coverUrl !== b.coverUrl ||
    a.addedAt !== b.addedAt || a.path !== b.path);
  cachedAlbumsSongsRef = songs;
  if (affected?.size === 0) return cachedAlbumsResult;
  if (!affected) {
    albumGroups = groupSongs(songs, albumGroupKeys);
    albumSummaries = new Map();
  }
  for (const key of affected ?? albumGroups.keys()) albumSummaries.set(key, summarizeAlbum(albumGroups.get(key)!));
  cachedAlbumsResult = Array.from(albumSummaries.values()).sort(
    (a, b) =>
      (b.addedAt || 0) - (a.addedAt || 0) ||
      compareLibraryText(a.name, b.name) ||
      compareLibraryText(a.artist, b.artist),
  );
  return cachedAlbumsResult;
};

export const useAlbums = () => useStore(state => computeAlbums(state.songs));

let cachedArtistsSongsRef: AppState['songs'] | null = null;
let cachedArtistsResult: Artist[] = [];
let artistGroups = new Map<string, AppState['songs']>();
let artistSummaries = new Map<string, Artist>();
const artistGroupKeys = (song: AppState['songs'][number]) => splitArtistNames(song.artist);

function summarizeArtist(name: string, members: AppState['songs']): Artist {
  const firstAlbum = members[0].album;
  let albums: Set<string> | undefined;
  let imageUrl = members[0].coverUrl;
  for (const song of members) {
    if (song.album !== firstAlbum) (albums ??= new Set([firstAlbum])).add(song.album);
    if (!imageUrl && song.coverUrl) imageUrl = song.coverUrl;
  }
  return { name, songCount: members.length, albumCount: albums?.size ?? 1, imageUrl };
}

const computeArtists = (songs: AppState['songs']): Artist[] => {
  if (songs === cachedArtistsSongsRef) {
    return cachedArtistsResult;
  }

  const affected = updateStableGroups(songs, cachedArtistsSongsRef, artistGroups, artistGroupKeys,
    (a, b) => a.artist !== b.artist || a.album !== b.album || a.coverUrl !== b.coverUrl);
  cachedArtistsSongsRef = songs;
  if (affected?.size === 0) return cachedArtistsResult;
  if (!affected) {
    const splitCache = new Map<string, string[]>();
    artistGroups = groupSongs(songs, song => {
      let names = splitCache.get(song.artist);
      if (!names) {
        names = artistGroupKeys(song);
        splitCache.set(song.artist, names);
      }
      return names;
    });
    artistSummaries = new Map();
  }
  for (const key of affected ?? artistGroups.keys()) artistSummaries.set(key, summarizeArtist(key, artistGroups.get(key)!));
  cachedArtistsResult = Array.from(artistSummaries.values()).sort((a, b) => b.songCount - a.songCount);
  return cachedArtistsResult;
};

const emptyArtists: Artist[] = [];
export const useArtists = (enabled = true) => useStore(state => enabled ? computeArtists(state.songs) : emptyArtists);

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

