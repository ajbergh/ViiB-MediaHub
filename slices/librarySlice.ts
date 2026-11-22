/**
 * ViiB MediaHub - Library State Slice
 * 
 * Zustand slice managing the music library, playlists, and metadata.
 * 
 * State:
 * - songs: Complete song library
 * - playlists: User-created playlists
 * - smartMixes: Auto-generated playlists
 * - artistMetadata/albumMetadata: external metadata cache used for catalog presentation
 * - scanFolders: Configured music directories
 * - isScanning: Library scan status
 * - likedSongIds: Set of liked song IDs
 * - likedAlbumKeys: Set of liked album keys ("Album::Artist" format)
 * 
 * Features:
 * - Library initialization from backend or IndexedDB
 * - Spotify metadata fetching with rate limiting
 * - Smart mix generation based on listening patterns
 * - Folder management and backend scanning
 * - Playlist CRUD operations
 * - Song/Album like/unlike with backend persistence
 * 
 * @module librarySlice
 */

import { assertSpotifySession } from '../services/spotifyBackend';
import { StateCreator } from 'zustand';
import { AppState, LibrarySlice, ScanFolder } from './types';
import { generateSmartMixes } from '../lib/smartMix';
import { SpotifyService } from '../services/spotifyService';
import { libraryService } from '../services/libraryService';
import { backendService } from '../services/backendService';
import { libraryOperationsV2 } from '../services/libraryOperationsV2';
import api, { ApiAlbumMetadata, ApiArtistMetadata } from '../services/api';
import { Playlist, Song, AlbumMetadata, ArtistMetadata } from '../types';
import { affectsSmartMixes, catalogDeltas, compareLibraryText, LibraryIndex, patchCatalogSong, patchCatalogSongs, rebaseCatalogDelta } from '../lib/libraryIndex';
import { libraryV2, LibraryResnapshotRequired } from '../services/libraryV2';

function mapCachedArtistMetadata(entries: ApiArtistMetadata[]): Record<string, ArtistMetadata> {
    const artistMetadata: Record<string, ArtistMetadata> = {};
    for (const cached of entries) {
        if (!cached.plexImageUrl && !(cached.spotifyFound && (cached.localImagePath || cached.imageUrl))) continue;

        // Plex artist portraits are authoritative for Plex-backed artists.
        // Local/Spotify artwork remains available if Plex has no portrait.
        let imageUrl = cached.plexImageUrl || cached.imageUrl || '';
        if (!cached.plexImageUrl && cached.localImagePath) {
            imageUrl = `/api/cover/${encodeURIComponent(cached.localImagePath)}`;
        }
        artistMetadata[cached.artistName] = {
            spotifyId: cached.spotifyId,
            name: cached.artistName,
            imageUrl,
            url: cached.spotifyUrl || '',
            fetchedAt: cached.fetchedAt || Date.now()
        };
    }
    return artistMetadata;
}

const SCAN_LIBRARY_REFRESH_INTERVAL_MS = 3000;
const IDLE_MIX_DEBOUNCE_MS = 400;
const SCAN_MIX_DEBOUNCE_MS = 1500;

export const createLibrarySlice: StateCreator<AppState, [], [], LibrarySlice> = (set, get) => {
  const libraryIndex = new LibraryIndex();
  const albumRequests = new Map<string, number>();
  let isPollingActive = false;
  let revision: number | null = null;
  let lastLegacyRead = 0;
  let initializing: Promise<void> | undefined;
  let syncing: Promise<void> | undefined;
  let rerun = false;
  let recoveryRequested = false;
  let mixTimer: ReturnType<typeof setTimeout> | undefined;

  const scheduleMixes = () => {
    if (mixTimer !== undefined) return;
    const delay = get().isScanning ? SCAN_MIX_DEBOUNCE_MS : IDLE_MIX_DEBOUNCE_MS;
    mixTimer = setTimeout(() => {
      mixTimer = undefined;
      set({ smartMixes: generateSmartMixes(get().songs) });
    }, delay);
  };

  const loadCatalog = async (onFirstPage?: (songs: Song[]) => void) => {
    try {
      const snapshot = await libraryV2.getSnapshot(undefined, onFirstPage);
      revision = snapshot.revision;
      return libraryIndex.initialize(snapshot.songs);
    } catch (error) {
      console.warn("Library snapshot unavailable; using legacy catalog", error);
      const songs = await backendService.getAllSongs();
      revision = null;
      lastLegacyRead = Date.now();
      return libraryIndex.initialize(songs);
    }
  };

  const publishCatalog = (songs: Song[], songIds?: string[], analysisSongIds?: string[]) => {
    if (songs !== get().songs) {
      rebaseCatalogDelta(songs, get().songs);
      const delta = catalogDeltas.get(songs);
      const refreshMixes = !delta || delta.previous.deref() !== get().songs || delta.orderChanged || delta.changes.some(affectsSmartMixes);
      set({ songs });
      if (refreshMixes) scheduleMixes();
    }
    if (typeof window !== "undefined") {
      window.dispatchEvent(new CustomEvent("library_updated", {
        detail: songIds ? { source: "library_delta", revision, songIds, analysisSongIds } : { source: "full_refresh" },
      }));
    }
  };

  return {
  songs: [],
  playlists: [],
  smartMixes: [],
  artistMetadata: {},
  albumMetadata: {},
  fetchingArtists: new Set(),
  fetchingAlbums: new Set(),
  isScanning: false,
  scanProgress: '',
  backendAvailable: false,
  isLibraryInitializing: true,
  libraryPreviewSongs: undefined,
  scanFolders: [],
  likedSongIds: new Set(),
  likedAlbumKeys: new Set(),
  enrichmentStatus: {
    isEnriching: false,
    totalSongs: 0,
    processedSongs: 0,
    currentBatch: 0,
    totalBatches: 0,
    message: '',
  },

  initLibrary: () => {
    if (initializing) return initializing;
    initializing = (async () => {
      // First, check if backend is available
      const backendAvailable = await backendService.init();
      set({ backendAvailable });

      if (backendAvailable) {
          // Show scanning UI immediately - startup scan will begin shortly
          set({ isScanning: true, scanProgress: 'Updating media library' });
          
          // Load from Go backend
          try {
              const folders = backendService.getFolders().catch(() => get().scanFolders);
              const metadata = Promise.all([
                  backendService.getAllPlaylists().catch(() => get().playlists),
                  api.getAllAlbumMetadata().catch(() => [] as ApiAlbumMetadata[]),
                  api.getAllArtistMetadata().catch(() => [] as ApiArtistMetadata[]),
                  api.getLikedSongIds().catch(() => Array.from(get().likedSongIds)),
                  api.getLikedAlbumKeys().catch(() => Array.from(get().likedAlbumKeys)),
              ]);
              const [songs, scanFolders] = await Promise.all([loadCatalog(firstPage => {
                  if (get().songs.length === 0) set({ libraryPreviewSongs: firstPage });
              }), folders]);
              set({ songs, scanFolders, isLibraryInitializing: false, libraryPreviewSongs: undefined });
              scheduleMixes();
              void get().pollScanStatus();
              const [playlists, cachedAlbumMetadata, cachedArtistMetadata, likedIds, likedAlbumKeysList] = await metadata;

              // Convert cached album metadata to the format used by the store
              const albumMetadata: Record<string, AlbumMetadata> = {};
              for (const cached of cachedAlbumMetadata) {
                  if (cached.spotifyFound && cached.coverUrl) {
                      albumMetadata[cached.albumKey] = {
                          spotifyId: cached.spotifyId,
                          name: cached.albumName,
                          artist: cached.artistName,
                          coverUrl: cached.localCoverPath 
                              ? `/api/cover/${encodeURIComponent(cached.localCoverPath)}` 
                              : cached.coverUrl || '',
                          description: cached.description,
                          genre: cached.genre,
                          releaseDate: cached.releaseDate || '',
                          url: cached.spotifyUrl || '',
                          copyright: cached.copyright,
                          fetchedAt: cached.fetchedAt || Date.now()
                      };
                  }
              }

              // Convert cached artist metadata to the format used by the store
              console.log(`📦 Processing ${cachedArtistMetadata.length} cached artist metadata entries`);
              const artistMetadata = mapCachedArtistMetadata(cachedArtistMetadata);
              
              set({ playlists, albumMetadata, artistMetadata, likedSongIds: new Set(likedIds), likedAlbumKeys: new Set(likedAlbumKeysList) });
          } catch (e) {
              console.error("Failed to initialize library from backend", e);
              // Reset scanning state on error
              set({ isScanning: false, scanProgress: '' });
          } finally {
              set({ isLibraryInitializing: false, libraryPreviewSongs: undefined });
          }
      } else {
          // Fallback: Load from IndexedDB (browser-only mode)
          try {
              const [songs, playlists] = await Promise.all([
                  libraryService.getAllSongs(),
                  libraryService.getAllPlaylists()
              ]);

              // Restore Blob URLs from persisted Blob data
              songs.forEach(s => {
                  if (s.coverData && s.coverData instanceof Blob) {
                      s.coverUrl = URL.createObjectURL(s.coverData);
                  }
              });
              
              const mixes = generateSmartMixes(songs);
              set({ songs, playlists, smartMixes: mixes });
          } catch (e) {
              console.error("Failed to initialize library from IndexedDB", e);
          } finally {
              set({ isLibraryInitializing: false });
          }
      }
    })().finally(() => { initializing = undefined; });
    return initializing;
  },

  getLibraryRevision: () => revision,

  syncLibraryCatalog: (recover = false) => {
    if (!get().backendAvailable || get().isLibraryInitializing) return Promise.resolve();
    recoveryRequested ||= recover;
    if (syncing) {
      // A commit can arrive after the current request captured its revision.
      rerun = true;
      return syncing;
    }
    syncing = (async () => {
      do {
        rerun = false;
        const forceRecovery = recoveryRequested;
        recoveryRequested = false;
        try {
          if (revision === null) {
            if (forceRecovery || Date.now() - lastLegacyRead >= SCAN_LIBRARY_REFRESH_INTERVAL_MS) {
              lastLegacyRead = Date.now();
              publishCatalog(await loadCatalog());
            }
            continue;
          }
          const delta = await libraryV2.getChanges(revision);
          revision = delta.revision;
          if (delta.changes.length) {
            publishCatalog(
              libraryIndex.apply(delta.changes, delta.songs),
              [...new Set(delta.changes.map(change => change.songId))],
              [...new Set(delta.changes.filter(change => change.analysisChanged !== false).map(change => change.songId))],
            );
          }
        } catch (error) {
          if (error instanceof LibraryResnapshotRequired || forceRecovery || Date.now() - lastLegacyRead >= SCAN_LIBRARY_REFRESH_INTERVAL_MS) {
            lastLegacyRead = Date.now();
            try { publishCatalog(await loadCatalog()); }
            catch (recoveryError) { console.warn("Unable to recover library catalog", recoveryError); }
          }
        }
      } while (rerun && get().backendAvailable);
    })().finally(() => { syncing = undefined; });
    return syncing;
  },

  refreshLibrary: async () => {
    if (!get().backendAvailable) return;
    try {
      const [, playlists, scanFolders, artistMetadata] = await Promise.all([
        get().syncLibraryCatalog(true), backendService.getAllPlaylists(), backendService.getFolders(), api.getAllArtistMetadata(),
      ]);
      set({ playlists, scanFolders, artistMetadata: mapCachedArtistMetadata(artistMetadata) });
    } catch (error) {
      console.warn("Unable to refresh library metadata", error);
    }
  },

  addSongs: (newSongs) => {
    const { backendAvailable } = get();
    
    // Save to appropriate storage
    if (!backendAvailable) {
        libraryService.saveSongs(newSongs).catch(console.error);
    }
    // Note: When using backend, songs come from scanning, no need to save here

    set((state) => {
      // Use a Map for O(1) deduplication by song id
      const songMap = new Map(state.songs.map(s => [s.id, s]));
      let hasNew = false;
      for (const song of newSongs) {
        if (!songMap.has(song.id)) {
          songMap.set(song.id, song);
          hasNew = true;
        }
      }
      
      // Only re-sort if new songs were actually added
      const updatedSongs = hasNew
        ? Array.from(songMap.values()).sort((a, b) => {
            if (a.album !== b.album) return compareLibraryText(a.album, b.album);
            if (a.discNumber !== b.discNumber) return (a.discNumber || 0) - (b.discNumber || 0);
            if (a.trackNumber !== b.trackNumber) return (a.trackNumber || 0) - (b.trackNumber || 0);
            return compareLibraryText(a.title, b.title);
          })
        : state.songs;
      
      const shouldInitQueue = state.queue.length === 0;
      if (hasNew) scheduleMixes();

      return { 
          songs: updatedSongs,
          queue: shouldInitQueue ? updatedSongs : state.queue
      };
    })
  },

  resetLibrary: async () => {
      const { backendAvailable } = get();
      
      try {
          if (backendAvailable) {
              // Clear backend database
              await backendService.clearSongs();
          } else {
              // Clear IndexedDB
              await libraryService.resetDB();
          }
          
          // Clear Zustand Persist Storage
          localStorage.removeItem('mediahub-storage'); 
          
          // Reset Local State
          set({ 
              songs: [], 
              playlists: [], 
              smartMixes: [],
              queue: [],
              currentSong: null,
              isPlaying: false,
              scanFolders: []
          });
          
          // Force reload
          setTimeout(() => {
              window.location.reload();
          }, 100);

      } catch (e) {
          console.error("Failed to reset library", e);
          alert("Failed to reset library completely. Please check console.");
      }
  },

    /**
     * createPlaylist - Create a new playlist.
     * When backend is available, the playlist is created on the server and the
     * server-generated ID is used. When backend is not available, the playlist
     * is stored in IndexedDB and a local ID is generated.
     */
    createPlaylist: async (name, songIds = []) => {
        const { backendAvailable } = get();
        if (backendAvailable) {
            try {
                const created = await backendService.createPlaylist(name, songIds);
                set((state) => ({ playlists: [...state.playlists, created] }));
                return created;
            } catch (e) {
                console.error('Failed to create playlist on backend:', e);
                throw e;
            }
        }

        const newPlaylist: Playlist = {
            id: Math.random().toString(36).substr(2, 9),
            name,
            songIds,
            createdAt: Date.now()
        };
    
        await libraryService.savePlaylist(newPlaylist);
    
        set((state) => ({
            playlists: [...state.playlists, newPlaylist]
        }));
        return newPlaylist;
    },

  updatePlaylistContents: async (playlistId, songIds, expectedSongIds) => {
      const { backendAvailable, playlists } = get();
      const existing = playlists.find(playlist => playlist.id === playlistId);
      if (!existing) throw new Error('Playlist no longer exists.');
      if (expectedSongIds && (existing.songIds.length !== expectedSongIds.length ||
          existing.songIds.some((id, index) => id !== expectedSongIds[index]))) {
          throw new Error('This playlist changed while you were editing. Cancel to reload its latest contents.');
      }
      const updated = { ...existing, songIds: [...songIds] };
      if (backendAvailable) await backendService.updatePlaylist(updated);
      else await libraryService.savePlaylist(updated);
      set(state => ({ playlists: state.playlists.map(playlist => playlist.id === playlistId ? updated : playlist) }));
  },

  addToPlaylist: (playlistId, songId) => {
      const { backendAvailable } = get();
      set((state) => {
          const updated = state.playlists.map(p => 
            p.id === playlistId ? { ...p, songIds: [...p.songIds, songId] } : p
          );
          // Sync with DB
          const playlist = updated.find(p => p.id === playlistId);
          if (playlist) {
              if (backendAvailable) {
                  backendService.updatePlaylist(playlist).catch(e => console.error('Failed to update playlist on backend:', e));
              } else {
                  libraryService.savePlaylist(playlist).catch(console.error);
              }
          }

          return { playlists: updated };
      });
  },

  deletePlaylist: async (playlistId) => {
      const { backendAvailable } = get();
      try {
          if (backendAvailable) {
              await backendService.deletePlaylist(playlistId);
          } else {
              await libraryService.deletePlaylist(playlistId);
          }
          // Only update state after successful deletion
          set((state) => ({
            playlists: state.playlists.filter(p => p.id !== playlistId)
          }));
      } catch (e) {
          console.error('Failed to delete playlist:', e);
      }
  },

  refreshSmartMixes: () => {
    if (mixTimer !== undefined) clearTimeout(mixTimer);
    mixTimer = undefined;
    set({ smartMixes: generateSmartMixes(get().songs) });
  },

  saveSmartMixAsPlaylist: async (mixId) => {
      const { backendAvailable } = get();
      const mix = get().smartMixes.find(m => m.id === mixId);
      if (!mix) return;

      if (backendAvailable) {
        try {
            const created = await backendService.createPlaylist(mix.name, mix.songIds);
            set((state) => ({ playlists: [...state.playlists, created] }));
            return created;
        } catch (e) {
            console.error('Failed to create playlist on backend:', e);
                throw e;
        }
      }

      const newPlaylist: Playlist = {
          id: Math.random().toString(36).substr(2, 9),
          name: mix.name,
          songIds: [...mix.songIds],
          createdAt: Date.now()
      };

      await libraryService.savePlaylist(newPlaylist);
      set((state) => ({ playlists: [...state.playlists, newPlaylist] }));
      return newPlaylist;
  },

    /**
     * recordPlay - Increment local play count and optionally persist to backend.
     * - When backend is available: call backend API to increment DB play_count and last_played
     * - When backend is not available: update IndexedDB via libraryService
     */
    recordPlay: (songId) => {
      const { backendAvailable, songs } = get();
      const song = songs.find(item => item.id === songId);
      if (!song) return;
      const patch = { playCount: (song.playCount || 0) + 1, lastPlayed: Date.now() };
      set({ songs: patchCatalogSong(songs, songId, patch) });
      scheduleMixes();
      if (backendAvailable) {
          backendService.recordPlay(songId).catch(e => console.error('Failed to record play on backend:', e));
      } else {
          libraryService.saveSongs([{ ...song, ...patch }]);
      }
  },

  /**
   * recordListenEvent - Record a listening event for AI DJ preference learning.
   * Called when a song ends (play_complete) or is skipped.
   * The backend auto-detects event type based on playDuration vs songDuration.
   */
  recordListenEvent: (songId, playDuration, songDuration, context) => {
      const { backendAvailable } = get();
      if (backendAvailable) {
          backendService.recordListenEvent(songId, playDuration, songDuration, context);
      }
  },

  /**
   * updateSongDuration - Update a song's duration with the actual audio duration.
   * This fixes cases where metadata extraction reports incorrect duration.
   * Only updates if the difference is significant (> 5%).
   */
  updateSongDuration: (songId, duration) => {
      const { backendAvailable, songs } = get();
      const song = songs.find(s => s.id === songId);
      
      // Only update if duration is valid and significantly different (> 5% difference)
      if (!song || !duration || duration <= 0) return;
      const diff = Math.abs(song.duration - duration) / Math.max(song.duration, duration);
      if (diff < 0.05) return; // Skip if difference is less than 5%
      
      console.log(`🔧 Fixing duration for "${song.title}": ${song.duration.toFixed(1)}s → ${duration.toFixed(1)}s`);
      
      set((state) => ({
          songs: patchCatalogSong(state.songs, songId, { duration }),
      }));
      
      // Persist to backend
      if (backendAvailable) {
          api.updateSongDuration(songId, duration).catch(e => 
              console.error('Failed to update duration on backend:', e)
          );
      } else {
          const updatedSong = { ...song, duration };
          libraryService.saveSongs([updatedSong]);
      }
  },

  /**
   * updateSongMetadata - Update song metadata tags (title, artist, album, genre, year, etc.)
   */
  updateSongMetadata: async (songId, patch) => {
      const { backendAvailable, songs } = get();
      const existing = songs.find(s => s.id === songId);
      if (!existing) throw new Error(`Unknown song: ${songId}`);

      const updatedSong = { ...existing, ...patch };
      if (backendAvailable) {
          const backendPatch: Record<string, any> = {};
          if (patch.title !== undefined) backendPatch.title = patch.title;
          if (patch.artist !== undefined) backendPatch.artist = patch.artist;
          if (patch.album !== undefined) backendPatch.album = patch.album;
          if (patch.albumArtist !== undefined) backendPatch.albumArtist = patch.albumArtist;
          if (patch.trackNumber !== undefined) backendPatch.trackNumber = patch.trackNumber;
          if (patch.discNumber !== undefined) backendPatch.discNumber = patch.discNumber;
          if (patch.year !== undefined) backendPatch.year = patch.year;
          if (patch.genre !== undefined) backendPatch.genre = patch.genre;

          await libraryOperationsV2.updateSongMetadata(songId, backendPatch);
      } else {
          await libraryService.saveSongs([updatedSong]);
      }
      set((state) => ({
          songs: patchCatalogSong(state.songs, songId, patch),
          currentSong: state.currentSong?.id === songId ? { ...state.currentSong, ...patch } : state.currentSong,
          queue: state.queue.map(s => s.id === songId ? { ...s, ...patch } : s),
          songInfoModalSong: state.songInfoModalSong?.id === songId ? { ...state.songInfoModalSong, ...patch } : state.songInfoModalSong,
      }));
      if (affectsSmartMixes({ before: existing, after: updatedSong })) scheduleMixes();
  },

  updateAlbumMetadata: async (songIds, patch) => {
      const ids = [...new Set(songIds)];
      const { backendAvailable, songs } = get();
      const selected = new Set(ids);
      const tracks = new Map(songs.filter(song => selected.has(song.id)).map(song => [song.id, song]));
      if (!ids.length || tracks.size !== ids.length) {
          throw new Error('A track is no longer in the library. Reopen the album editor and try again.');
      }

      const patches = new Map<string, Partial<Song>>();
      if (backendAvailable) {
          const updated = await libraryOperationsV2.updateAlbumMetadata(ids, patch);
          for (const song of updated) {
              patches.set(song.id, {
                  ...(patch.album !== undefined && { album: song.album }),
                  ...(patch.albumArtist !== undefined && { albumArtist: song.albumArtist }),
                  ...(patch.artist !== undefined && { artist: song.artist }),
                  ...(patch.genre !== undefined && { genre: song.genre ?? [] }),
                  ...(patch.year !== undefined && { year: song.year }),
                  ...(patch.year !== undefined || patch.album !== undefined || patch.artist !== undefined
                      ? { originalYear: song.originalYear } : {}),
              });
          }
      } else {
          await libraryService.updateAlbumMetadata(ids, patch);
          const metadata = { ...patch, ...(patch.year !== undefined && { originalYear: patch.year }) };
          for (const id of ids) patches.set(id, metadata);
      }
      if (ids.some(id => !patches.has(id))) throw new Error('The album update returned an incomplete track list.');

      const patchTrack = (song: Song) => {
          const metadata = patches.get(song.id);
          return metadata ? { ...song, ...metadata } : song;
      };
      set(state => ({
          songs: patchCatalogSongs(state.songs, patches),
          queue: state.queue.map(patchTrack),
          currentSong: state.currentSong && patchTrack(state.currentSong),
          songInfoModalSong: state.songInfoModalSong && patchTrack(state.songInfoModalSong),
      }));
      if (patch.year !== undefined || patch.genre !== undefined) scheduleMixes();
      const first = patchTrack(tracks.get(ids[0])!);
      return { album: first.album, artist: first.albumArtist || first.artist };
  },

  fetchArtistMetadata: async (artistName) => {
      const state = get();
      const generation = state.spotifySessionGeneration;
      if (state.artistMetadata[artistName] || state.fetchingArtists.has(artistName)) return;

      // Check if we've already checked Spotify and found nothing (cached "not found")
      try {
          const cached = await api.getArtistMetadata(artistName);
          if (cached?.plexImageUrl) {
              const imageUrl = cached.plexImageUrl;
              set((s) => ({
                  artistMetadata: {
                      ...s.artistMetadata,
                      [artistName]: {
                          spotifyId: cached.spotifyId,
                          name: cached.artistName,
                          imageUrl,
                          url: cached.spotifyUrl || '',
                          fetchedAt: cached.fetchedAt || Date.now()
                      }
                  }
              }));
              return;
          }
          if (cached?.spotifyChecked && !cached.spotifyFound) {
              // Already checked, Spotify had nothing - don't re-query
              console.log(`📦 Artist "${artistName}" already checked - Spotify had no results`);
              return;
          }
          // If we have cached data with an image, use it
          if (cached?.spotifyFound && (cached.localImagePath || cached.imageUrl)) {
              const imageUrl = cached.localImagePath 
                  ? `/api/cover/${encodeURIComponent(cached.localImagePath)}` 
                  : cached.imageUrl || '';
              console.log(`📦 Using cached artist image for "${artistName}": ${imageUrl}`);
              set((s) => ({
                  artistMetadata: { 
                      ...s.artistMetadata, 
                      [artistName]: {
                          spotifyId: cached.spotifyId,
                          name: cached.artistName,
                          imageUrl,
                          url: cached.spotifyUrl || '',
                          fetchedAt: cached.fetchedAt || Date.now()
                      }
                  }
              }));
              return;
          }
      } catch (e) {
          console.warn(`Cache lookup error for artist "${artistName}":`, e);
      }

      set((s) => {
          const newSet = new Set(s.fetchingArtists);
          newSet.add(artistName);
          return { fetchingArtists: newSet };
      });

      try {
          console.log(`🔍 Searching Spotify for artist: "${artistName}"`);
          assertSpotifySession(generation);
          const data = await SpotifyService.searchArtist(artistName);
          assertSpotifySession(generation);

          // Save result to backend cache (whether found or not)
          const cacheEntry: import('../services/api').ApiArtistMetadata = {
              artistName,
              spotifyChecked: true,
              spotifyFound: !!data,
              fetchedAt: Date.now(),
          };

          if (data) {
              console.log(`✅ Found artist "${artistName}" on Spotify, imageUrl: ${data.imageUrl}`);
              cacheEntry.spotifyId = data.spotifyId;
              cacheEntry.imageUrl = data.imageUrl;
              cacheEntry.spotifyUrl = data.url;
          } else {
              console.log(`❌ No Spotify match found for artist: "${artistName}"`);
          }

          // Save to backend cache
          api.saveArtistMetadata(cacheEntry).catch(e => {
              console.error(`Failed to cache artist metadata for "${artistName}":`, e);
          });

          // If we got image URL and backend is available, download image locally
          if (data?.imageUrl && state.backendAvailable) {
              console.log(`📥 Downloading artist image for "${artistName}" from ${data.imageUrl}`);
              api.downloadArtistImage(artistName, data.imageUrl).then(result => {
                  if (get().spotifySessionGeneration !== generation) return;
                   console.log(`✅ Artist image saved for "${artistName}" at: ${result.imagePath}`);
                  // Update the store with local image path
                  set((s) => {
                      const existing = s.artistMetadata[artistName];
                      if (existing) {
                          return {
                              artistMetadata: {
                                  ...s.artistMetadata,
                                  [artistName]: {
                                      ...existing,
                                      imageUrl: `/api/cover/${encodeURIComponent(result.imagePath)}`
                                  }
                              }
                          };
                      }
                      return {};
                  });
              }).catch(e => {
                  console.error(`Failed to download artist image for "${artistName}":`, e);
              });
          }

          set((s) => {
              const newFetching = new Set(s.fetchingArtists);
              newFetching.delete(artistName);
              if (data) {
                  return { 
                      artistMetadata: { ...s.artistMetadata, [artistName]: data },
                      fetchingArtists: newFetching
                  };
              }
              return { fetchingArtists: newFetching };
          });
      } catch (error) {
          console.error(`Failed to fetch artist metadata for "${artistName}":`, error);
          set((s) => {
              const newFetching = new Set(s.fetchingArtists);
              newFetching.delete(artistName);
              return { fetchingArtists: newFetching };
          });
      }
  },

  fetchAlbumMetadata: async (albumName, artistName) => {
      const key = `${albumName}::${artistName}`;
      const state = get();
      const generation = state.spotifySessionGeneration;
      
      if (!state.spotifyConnected || state.albumMetadata[key] || albumRequests.get(key) === generation) return;
      albumRequests.set(key, generation);
      set(s => ({ fetchingAlbums: new Set(s.fetchingAlbums).add(key) }));

      try {
          const cached = await api.getAlbumMetadata(key).catch(() => null);
          assertSpotifySession(generation);
          const fetchedAt = cached?.fetchedAt ?? 0;
          const timestampMs = fetchedAt < 10_000_000_000 ? fetchedAt * 1000 : fetchedAt;
          if (cached?.spotifyChecked && !cached.spotifyFound &&
              Date.now() - timestampMs < 30 * 24 * 60 * 60 * 1000) return;

          const data = await SpotifyService.searchAlbum(albumName, artistName);
          assertSpotifySession(generation);

          // Save result to backend cache (whether found or not)
          const cacheEntry: ApiAlbumMetadata = {
              albumKey: key,
              albumName,
              artistName,
              spotifyChecked: true,
              spotifyFound: !!data,
              fetchedAt: Date.now(),
          };

          if (data) {
              cacheEntry.spotifyId = data.spotifyId;
              cacheEntry.coverUrl = data.coverUrl;
              cacheEntry.description = data.description;
              cacheEntry.genre = data.genre;
              cacheEntry.releaseDate = data.releaseDate;
              cacheEntry.spotifyUrl = data.url;
              cacheEntry.copyright = data.copyright;
          }

          await api.saveAlbumMetadata(cacheEntry).catch(error => {
              console.warn('Failed to cache album metadata:', error);
          });
          if (get().spotifySessionGeneration !== generation) return;

          if (data) {
              set(s => ({ albumMetadata: { ...s.albumMetadata, [key]: data } }));
              if (state.backendAvailable && data.coverUrl) {
                  api.downloadAlbumCover(key, data.coverUrl).catch(error => {
                      console.warn('Failed to download album cover:', error);
                  });
              }
          }
      } catch (error) {
          if (get().spotifySessionGeneration === generation) {
              console.error('Failed to fetch album metadata:', albumName, error);
          }
      } finally {
          if (albumRequests.get(key) === generation) {
              albumRequests.delete(key);
              set(s => {
                  const fetchingAlbums = new Set(s.fetchingAlbums);
                  fetchingAlbums.delete(key);
                  return { fetchingAlbums };
              });
          }
      }
  },

  clearAlbumMetadata: (albumKey) => {
      set((s) => {
          const newMetadata = { ...s.albumMetadata };
          delete newMetadata[albumKey];
          return { albumMetadata: newMetadata };
      });
  },

  setScanning: (isScanning) => set({ isScanning }),
  setScanProgress: (scanProgress) => set({ scanProgress }),
  setEnrichmentStatus: (status) => set((state) => ({
    enrichmentStatus: { ...state.enrichmentStatus, ...status }
  })),

  // Backend folder management
  loadScanFolders: async () => {
      const { backendAvailable } = get();
      if (!backendAvailable) return;
      
      try {
          const folders = await backendService.getFolders();
          set({ scanFolders: folders });
      } catch (e) {
          console.error("Failed to load scan folders", e);
      }
  },

  addScanFolder: async (path: string) => {
      const { backendAvailable } = get();
      if (!backendAvailable) {
          alert("Backend not available. Cannot add folders.");
          return;
      }
      
      try {
          const folder = await backendService.addFolder(path);
          if (folder) {
              set((state) => ({ scanFolders: [...state.scanFolders, folder] }));
          }
      } catch (e: any) {
          console.error("Failed to add folder", e);
          alert(e.message || "Failed to add folder");
      }
  },

  removeScanFolder: async (id: string) => {
      const { backendAvailable } = get();
      if (!backendAvailable) return;
      
      try {
          await backendService.removeFolder(id);
          set((state) => ({ 
              scanFolders: state.scanFolders.filter(f => f.id !== id) 
          }));
      } catch (e) {
          console.error("Failed to remove folder", e);
      }
  },

  startBackendScan: async () => {
      const { backendAvailable } = get();
      if (!backendAvailable) {
          alert("Backend not available. Use browser file import instead.");
          return;
      }
      
      try {
          set({ isScanning: true, scanProgress: 'Starting full rescan...' });
          await backendService.startScan();
          
          // Start polling for status
          get().pollScanStatus();
      } catch (e: any) {
          set({ isScanning: false, scanProgress: '' });
          alert(e.message || "Failed to start scan");
      }
  },

  startQuickScan: async () => {
      const { backendAvailable } = get();
      if (!backendAvailable) {
          alert("Backend not available. Use browser file import instead.");
          return;
      }
      
      try {
          set({ isScanning: true, scanProgress: 'Starting quick scan...' });
          await backendService.startQuickScan();
          
          // Start polling for status
          get().pollScanStatus();
      } catch (e: any) {
          set({ isScanning: false, scanProgress: '' });
          alert(e.message || "Failed to start quick scan");
      }
  },

  pollScanStatus: async () => {
      const { backendAvailable } = get();
      if (!backendAvailable) return;

      // Prevent multiple concurrent polling chains
      if (isPollingActive) {
          console.log('🔄 Poll already active, skipping duplicate');
          return;
      }
      isPollingActive = true;
      
      console.log('🔄 Starting scan status polling...');

	  // Avoid a startup race: the backend may report scanning=false for a brief
	  // window right after launch (before the startup scan flips its flag).
	  const pollStartedAt = Date.now();
	  let everSawScanning = false;
      let lastCatchup: number | undefined;

      const poll = async () => {
          try {
              const status = await backendService.getScanStatus();
              console.log('🔄 Poll result:', { scanning: status.scanning, progress: status.progress });

	          if (status.scanning) {
	              everSawScanning = true;
	          }

	          // Don't wipe the UI progress message with an empty backend status.
	          if (typeof status.progress === 'string' && status.progress.trim().length > 0) {
	              set({ scanProgress: status.progress });
	          }
              
              if (status.scanning) {
                  // SSE/revision events are the efficient fast path. This
                  // periodic read guarantees fresh installs still populate if
                  // either stream connects late or drops an early batch event.
                  if (lastCatchup === undefined || Date.now() - lastCatchup >= SCAN_LIBRARY_REFRESH_INTERVAL_MS) {
                    lastCatchup = Date.now();
                    await get().syncLibraryCatalog();
                  }
                  // Continue polling
                  setTimeout(poll, 1000);
              } else {
	              // If we haven't observed the scan start yet, keep polling briefly.
	              // This is especially important on startup when the backend may delay
	              // the quick scan by a couple seconds.
	              if (!everSawScanning && Date.now() - pollStartedAt < 8000) {
	                  setTimeout(poll, 500);
	                  return;
	              }

                  const [, scanFolders] = await Promise.all([
                    get().syncLibraryCatalog(true), backendService.getFolders().catch(() => get().scanFolders),
                  ]);
                  isPollingActive = false;
                  set({ isScanning: false, scanProgress: "", scanFolders });
              }
          } catch (e) {
              console.error("Error polling scan status", e);
              isPollingActive = false;
              set({ isScanning: false, scanProgress: '' });
          }
      };
      
      poll();
  },

  /**
   * Toggle the liked status of a song
   * Updates both backend and local state
   */
  toggleLikeSong: async (songId: string) => {
      const { backendAvailable } = get();
      if (!backendAvailable) {
          console.warn('Backend not available, cannot toggle like');
          return;
      }

      try {
          const result = await api.toggleLike(songId);
          const newLikedIds = new Set(get().likedSongIds);
          
          if (result.liked) {
              newLikedIds.add(songId);
          } else {
              newLikedIds.delete(songId);
          }
          
          // Publish immutable snapshots so selectors observing a song or the
          // library array see the update; retain all unaffected song identities.
          set((state) => ({
              likedSongIds: newLikedIds,
              songs: patchCatalogSong(state.songs, songId, { liked: result.liked, likedAt: result.liked ? Date.now() : undefined }),
          }));

          console.log(`${result.liked ? '❤️' : '💔'} Song ${songId} ${result.liked ? 'liked' : 'unliked'}`);
      } catch (e) {
          console.error('Failed to toggle like:', e);
      }
  },

  /**
   * Sync liked songs from backend on initialization
   * Populates the likedSongIds set for quick lookup
   */
  syncLikedSongs: async () => {
      const { backendAvailable } = get();
      if (!backendAvailable) return;

      try {
          const likedIds = await api.getLikedSongIds();
          set({ likedSongIds: new Set(likedIds) });
          console.log(`✅ Synced ${likedIds.length} liked songs from backend`);
      } catch (e) {
          console.error('Failed to sync liked songs:', e);
      }
  },

  /**
   * Check if a song is liked
   * Uses the Set for O(1) lookup
   */
  isLikedSong: (songId: string) => {
      return get().likedSongIds.has(songId);
  },

  /**
   * Toggle the liked status of an album
   * Updates both backend and local state
   */
  toggleLikeAlbum: async (albumKey: string) => {
      const { backendAvailable } = get();
      if (!backendAvailable) {
          console.warn('Backend not available, cannot toggle album like');
          return;
      }

      try {
          const result = await api.toggleAlbumLike(albumKey);
          const newLikedKeys = new Set(get().likedAlbumKeys);
          
          if (result.liked) {
              newLikedKeys.add(albumKey);
          } else {
              newLikedKeys.delete(albumKey);
          }
          
          set({ likedAlbumKeys: newLikedKeys });
          
          console.log(`${result.liked ? '❤️' : '💔'} Album "${albumKey}" ${result.liked ? 'liked' : 'unliked'}`);
      } catch (e) {
          console.error('Failed to toggle album like:', e);
      }
  },

  /**
   * Sync liked albums from backend on initialization
   * Populates the likedAlbumKeys set for quick lookup
   */
  syncLikedAlbums: async () => {
      const { backendAvailable } = get();
      if (!backendAvailable) return;

      try {
          const likedKeys = await api.getLikedAlbumKeys();
          set({ likedAlbumKeys: new Set(likedKeys) });
          console.log(`✅ Synced ${likedKeys.length} liked albums from backend`);
      } catch (e) {
          console.error('Failed to sync liked albums:', e);
      }
  },

  /**
   * Check if an album is liked
   * Uses the Set for O(1) lookup
   */
  isLikedAlbum: (albumKey: string) => {
      return get().likedAlbumKeys.has(albumKey);
  },
  };
};
