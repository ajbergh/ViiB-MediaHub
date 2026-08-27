/**
 * ViiB MediaHub - Library State Slice
 * 
 * Zustand slice managing the music library, playlists, and metadata.
 * 
 * State:
 * - songs: Complete song library
 * - playlists: User-created playlists
 * - smartMixes: Auto-generated playlists
 * - artistMetadata/albumMetadata: Spotify-enriched metadata cache
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

import { StateCreator } from 'zustand';
import { AppState, LibrarySlice, ScanFolder } from './types';
import { generateSmartMixes } from '../lib/smartMix';
import { SpotifyService } from '../services/spotifyService';
import { libraryService } from '../services/libraryService';
import { backendService } from '../services/backendService';
import { libraryOperationsV2 } from '../services/libraryOperationsV2';
import api, { ApiAlbumMetadata, ApiArtistMetadata } from '../services/api';
import { Playlist, Song, AlbumMetadata, ArtistMetadata } from '../types';
import { libraryIndex } from '../lib/libraryIndex';

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

let isPollingActive = false;
const SCAN_LIBRARY_REFRESH_INTERVAL_MS = 3000;

export const createLibrarySlice: StateCreator<AppState, [], [], LibrarySlice> = (set, get) => ({
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

  initLibrary: async () => {
      // First, check if backend is available
      const backendAvailable = await backendService.init();
      set({ backendAvailable });

      if (backendAvailable) {
          // Show scanning UI immediately - startup scan will begin shortly
          set({ isScanning: true, scanProgress: 'Updating media library' });
          
          // Load from Go backend
          try {
              const [songs, playlists, scanFolders, cachedAlbumMetadata, cachedArtistMetadata, likedIds, likedAlbumKeysList] = await Promise.all([
                  backendService.getAllSongs(),
                  backendService.getAllPlaylists(),
                  backendService.getFolders(),
                  api.getAllAlbumMetadata().catch(() => [] as ApiAlbumMetadata[]),
                  api.getAllArtistMetadata().catch(() => [] as ApiArtistMetadata[]),
                  api.getLikedSongIds().catch(() => [] as string[]),
                  api.getLikedAlbumKeys().catch(() => [] as string[])
              ]);
              
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
              
              const indexedSongs = libraryIndex.initialize(songs);
              const mixes = generateSmartMixes(indexedSongs);
              const likedSongIds = new Set(likedIds);
              const likedAlbumKeys = new Set(likedAlbumKeysList);
              set({ songs: indexedSongs, playlists, smartMixes: mixes, scanFolders, albumMetadata, artistMetadata, likedSongIds, likedAlbumKeys });
              console.log(`✅ Loaded ${songs.length} songs, ${Object.keys(albumMetadata).length} cached album metadata, ${Object.keys(artistMetadata).length} cached artist metadata, ${likedIds.length} liked songs, ${likedAlbumKeysList.length} liked albums from backend`);
              
              // Note: isScanning is already set to true at the start of initLibrary
              // The SSE connection will handle scan_complete events to reset it
              // Start polling to ensure we catch scan completion even if SSE events are missed
              get().pollScanStatus();
          } catch (e) {
              console.error("Failed to initialize library from backend", e);
              // Reset scanning state on error
              set({ isScanning: false, scanProgress: '' });
          } finally {
              set({ isLibraryInitializing: false });
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
  },

  refreshLibrary: async () => {
      const { backendAvailable } = get();
      
      if (backendAvailable) {
          try {
              console.log('🔄 Refreshing library from backend...');
              const [songs, playlists, scanFolders, cachedArtistMetadata] = await Promise.all([
                  backendService.getAllSongs(),
                  backendService.getAllPlaylists(),
                  backendService.getFolders(),
                  api.getAllArtistMetadata()
              ]);
              
              const indexedSongs = libraryIndex.initialize(songs);
              const mixes = generateSmartMixes(indexedSongs);
              set({ songs: indexedSongs, playlists, smartMixes: mixes, scanFolders, artistMetadata: mapCachedArtistMetadata(cachedArtistMetadata) });
              console.log(`✅ Library refreshed: ${songs.length} songs`);
          } catch (e) {
              console.error("Failed to refresh library from backend", e);
          }
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
            if (a.album !== b.album) return a.album.localeCompare(b.album);
            if (a.discNumber !== b.discNumber) return (a.discNumber || 0) - (b.discNumber || 0);
            if (a.trackNumber !== b.trackNumber) return (a.trackNumber || 0) - (b.trackNumber || 0);
            return a.title.localeCompare(b.title);
          })
        : state.songs;
      
      const shouldInitQueue = state.queue.length === 0;
      const mixes = generateSmartMixes(updatedSongs);

      return { 
          songs: updatedSongs,
          queue: shouldInitQueue ? updatedSongs : state.queue,
          smartMixes: mixes
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
                console.error('Failed to create playlist on backend, falling back to local:', e);
            }
        }

        const newPlaylist: Playlist = {
            id: Math.random().toString(36).substr(2, 9),
            name,
            songIds,
            createdAt: Date.now()
        };
    
        libraryService.savePlaylist(newPlaylist).catch(console.error);
    
        set((state) => ({
            playlists: [...state.playlists, newPlaylist]
        }));
        return newPlaylist;
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

  refreshSmartMixes: () => set((state) => ({
      smartMixes: generateSmartMixes(state.songs)
  })),

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
            console.error('Failed to create playlist on backend, falling back to local:', e);
        }
      }

      const newPlaylist: Playlist = {
          id: Math.random().toString(36).substr(2, 9),
          name: mix.name,
          songIds: [...mix.songIds],
          createdAt: Date.now()
      };

      libraryService.savePlaylist(newPlaylist).catch(console.error);
      set((state) => ({ playlists: [...state.playlists, newPlaylist] }));
      return newPlaylist;
  },

    /**
     * recordPlay - Increment local play count and optionally persist to backend.
     * - When backend is available: call backend API to increment DB play_count and last_played
     * - When backend is not available: update IndexedDB via libraryService
     */
    recordPlay: (songId) => {
      const { backendAvailable } = get();
      set((state) => {
          const updatedSongs = state.songs.map(s => {
              if (s.id === songId) {
                  const updated = {
                      ...s,
                      playCount: (s.playCount || 0) + 1,
                      lastPlayed: Date.now()
                  };
                  // Persist the play count
                  if (backendAvailable) {
                      backendService.recordPlay(songId).catch(e => console.error('Failed to record play on backend:', e));
                  } else {
                      libraryService.saveSongs([updated]);
                  }
                  return updated;
              }
              return s;
          });
          return { songs: updatedSongs };
      });
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
          songs: state.songs.map(s => 
              s.id === songId ? { ...s, duration } : s
          )
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
      if (!existing) return;

      const updatedSong = { ...existing, ...patch };
      set((state) => ({
          songs: state.songs.map(s => (s.id === songId ? updatedSong : s)),
          currentSong: state.currentSong?.id === songId ? { ...state.currentSong, ...patch } : state.currentSong,
          queue: state.queue.map(s => (s.id === songId ? { ...s, ...patch } : s)),
      }));

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

          try {
              await libraryOperationsV2.updateSongMetadata(songId, backendPatch);
          } catch (err) {
              console.error('Failed to persist song metadata on backend:', err);
          }
      } else {
          libraryService.saveSongs([updatedSong]);
      }
  },

  fetchArtistMetadata: async (artistName) => {
      const state = get();
      if (state.artistMetadata[artistName] || state.fetchingArtists.has(artistName)) return;

      // Check if we've already checked Spotify and found nothing (cached "not found")
      try {
          const cached = await api.getArtistMetadata(artistName);
          if (cached?.plexImageUrl) {
              set((s) => ({
                  artistMetadata: {
                      ...s.artistMetadata,
                      [artistName]: {
                          spotifyId: cached.spotifyId,
                          name: cached.artistName,
                          imageUrl: cached.plexImageUrl,
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
          const data = await SpotifyService.searchArtist(artistName);

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
      
      // Skip if already have metadata or currently fetching
      if (state.albumMetadata[key] || state.fetchingAlbums.has(key)) return;

      // Check if we've already checked Spotify and found nothing (cached "not found")
      try {
          const cached = await api.getAlbumMetadata(key);
          if (cached?.spotifyChecked && !cached.spotifyFound) {
              // Already checked, Spotify had nothing - don't re-query
              console.log(`📦 Album "${albumName}" already checked - Spotify had no results`);
              return;
          }
      } catch {
          // Ignore cache lookup errors
      }

      set((s) => {
          const newSet = new Set(s.fetchingAlbums);
          newSet.add(key);
          return { fetchingAlbums: newSet };
      });

      try {
          const data = await SpotifyService.searchAlbum(albumName, artistName);

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

          // Save to backend cache (fire and forget)
          api.saveAlbumMetadata(cacheEntry).catch(e => {
              console.warn('Failed to cache album metadata:', e);
          });

          // If we got artwork URL and backend is available, download cover.jpg to album folder
          if (data?.coverUrl && state.backendAvailable) {
              api.downloadAlbumCover(key, data.coverUrl).catch(e => {
                  console.warn('Failed to download album cover:', e);
              });
          }

          set((s) => {
              const newFetching = new Set(s.fetchingAlbums);
              newFetching.delete(key);
              if (data) {
                  return {
                      albumMetadata: { ...s.albumMetadata, [key]: data },
                      fetchingAlbums: newFetching
                  };
              }
              return { fetchingAlbums: newFetching };
          });
      } catch (error) {
          console.error(`Failed to fetch album metadata for "${albumName}":`, error);
          set((s) => {
              const newFetching = new Set(s.fetchingAlbums);
              newFetching.delete(key);
              return { fetchingAlbums: newFetching };
          });
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
      let lastLibraryRefreshAt = 0;

      const refreshVisibleSongs = async (force = false) => {
          const now = Date.now();
          if (!force && now - lastLibraryRefreshAt < SCAN_LIBRARY_REFRESH_INTERVAL_MS) return;
          lastLibraryRefreshAt = now;

          try {
              const songs = libraryIndex.initialize(await backendService.getAllSongs());
              set({ songs, smartMixes: generateSmartMixes(songs) });
              console.log(`🔄 Refreshed ${songs.length} visible songs during scan`);
          } catch (error) {
              // A transient catalog read must not stop scan-status polling.
              console.warn('Failed to refresh songs during scan', error);
          }
      };
      
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
                  await refreshVisibleSongs();
                  // Continue polling
                  setTimeout(poll, 1000);
              } else {
	              // If we haven't observed the scan start yet, keep polling briefly.
	              // This is especially important on startup when the backend may delay
	              // the quick scan by a couple seconds.
	              if (!everSawScanning && Date.now() - pollStartedAt < 8000) {
	                  await refreshVisibleSongs();
	                  setTimeout(poll, 500);
	                  return;
	              }

                  // Scan complete - reload library
                  console.log('✅ Scan complete detected via polling, resetting UI...');
                  isPollingActive = false;
                  set({ isScanning: false, scanProgress: '' });
                  
                  // Refresh songs from backend
                  const [loadedSongs, folders] = await Promise.all([
                      backendService.getAllSongs(),
                      backendService.getFolders()
                  ]);
                  const songs = libraryIndex.initialize(loadedSongs);
                  const mixes = generateSmartMixes(songs);
                  set({ songs, smartMixes: mixes, scanFolders: folders });
                  console.log('✅ Library refreshed after scan completion');
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
          
          // Update local song object as well
          set((state) => ({
              likedSongIds: newLikedIds,
              songs: state.songs.map(s => 
                  s.id === songId 
                      ? { ...s, liked: result.liked, likedAt: result.liked ? Date.now() : undefined }
                      : s
              )
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
});
