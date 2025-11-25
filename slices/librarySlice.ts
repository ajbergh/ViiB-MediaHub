import { StateCreator } from 'zustand';
import { AppState, LibrarySlice, ScanFolder } from './types';
import { generateSmartMixes } from '../lib/smartMix';
import { SpotifyService } from '../services/spotifyService';
import { libraryService } from '../services/libraryService';
import { backendService } from '../services/backendService';
import { Playlist, Song } from '../types';

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
  scanFolders: [],

  initLibrary: async () => {
      // First, check if backend is available
      const backendAvailable = await backendService.init();
      set({ backendAvailable });

      if (backendAvailable) {
          // Load from Go backend
          try {
              const [songs, playlists, scanFolders] = await Promise.all([
                  backendService.getAllSongs(),
                  backendService.getAllPlaylists(),
                  backendService.getFolders()
              ]);
              
              const mixes = generateSmartMixes(songs);
              set({ songs, playlists, smartMixes: mixes, scanFolders });
              console.log(`✅ Loaded ${songs.length} songs from backend`);
          } catch (e) {
              console.error("Failed to initialize library from backend", e);
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
      const updatedSongs = [...state.songs, ...newSongs].sort((a, b) => {
          if (a.album !== b.album) return a.album.localeCompare(b.album);
          if (a.discNumber !== b.discNumber) return (a.discNumber || 0) - (b.discNumber || 0);
          if (a.trackNumber !== b.trackNumber) return (a.trackNumber || 0) - (b.trackNumber || 0);
          return a.title.localeCompare(b.title);
      });
      
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

  createPlaylist: (name) => {
    const newPlaylist: Playlist = {
      id: Math.random().toString(36).substr(2, 9),
      name,
      songIds: [],
      createdAt: Date.now()
    };
    
    libraryService.savePlaylist(newPlaylist);
    
    set((state) => ({
      playlists: [...state.playlists, newPlaylist]
    }));
  },

  addToPlaylist: (playlistId, songId) => {
      set((state) => {
          const updated = state.playlists.map(p => 
            p.id === playlistId ? { ...p, songIds: [...p.songIds, songId] } : p
          );
          // Sync with DB
          const playlist = updated.find(p => p.id === playlistId);
          if (playlist) libraryService.savePlaylist(playlist);

          return { playlists: updated };
      });
  },

  deletePlaylist: (playlistId) => {
      libraryService.deletePlaylist(playlistId);
      set((state) => ({
        playlists: state.playlists.filter(p => p.id !== playlistId)
      }));
  },

  refreshSmartMixes: () => set((state) => ({
      smartMixes: generateSmartMixes(state.songs)
  })),

  saveSmartMixAsPlaylist: (mixId) => {
      set((state) => {
        const mix = state.smartMixes.find(m => m.id === mixId);
        if (!mix) return state;
        
        const newPlaylist: Playlist = {
            id: Math.random().toString(36).substr(2, 9),
            name: mix.name,
            songIds: [...mix.songIds],
            createdAt: Date.now()
        };
        
        libraryService.savePlaylist(newPlaylist);
        
        return { playlists: [...state.playlists, newPlaylist] };
      });
  },

  recordPlay: (songId) => set((state) => {
      const updatedSongs = state.songs.map(s => {
          if (s.id === songId) {
              const updated = {
                  ...s,
                  playCount: (s.playCount || 0) + 1,
                  lastPlayed: Date.now()
              };
              // Persist the play count
              libraryService.saveSongs([updated]);
              return updated;
          }
          return s;
      });
      return { songs: updatedSongs };
  }),

  fetchArtistMetadata: async (artistName) => {
      const state = get();
      if (state.artistMetadata[artistName] || state.fetchingArtists.has(artistName)) return;

      set((s) => {
          const newSet = new Set(s.fetchingArtists);
          newSet.add(artistName);
          return { fetchingArtists: newSet };
      });

      const data = await SpotifyService.searchArtist(artistName);
      
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
  },

  fetchAlbumMetadata: async (albumName, artistName) => {
      const key = `${albumName}::${artistName}`;
      const state = get();
      if (state.albumMetadata[key] || state.fetchingAlbums.has(key)) return;

      set((s) => {
          const newSet = new Set(s.fetchingAlbums);
          newSet.add(key);
          return { fetchingAlbums: newSet };
      });

      const data = await SpotifyService.searchAlbum(albumName, artistName);

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
  },

  setScanning: (isScanning) => set({ isScanning }),
  setScanProgress: (scanProgress) => set({ scanProgress }),

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
          set({ isScanning: true, scanProgress: 'Starting scan...' });
          await backendService.startScan();
          
          // Start polling for status
          get().pollScanStatus();
      } catch (e: any) {
          set({ isScanning: false, scanProgress: '' });
          alert(e.message || "Failed to start scan");
      }
  },

  pollScanStatus: async () => {
      const { backendAvailable } = get();
      if (!backendAvailable) return;
      
      const poll = async () => {
          try {
              const status = await backendService.getScanStatus();
              set({ scanProgress: status.progress });
              
              if (status.scanning) {
                  // Continue polling
                  setTimeout(poll, 1000);
              } else {
                  // Scan complete - reload library
                  set({ isScanning: false, scanProgress: '' });
                  
                  // Refresh songs from backend
                  const [songs, folders] = await Promise.all([
                      backendService.getAllSongs(),
                      backendService.getFolders()
                  ]);
                  
                  const mixes = generateSmartMixes(songs);
                  set({ songs, smartMixes: mixes, scanFolders: folders });
              }
          } catch (e) {
              console.error("Error polling scan status", e);
              set({ isScanning: false, scanProgress: '' });
          }
      };
      
      poll();
  },
});