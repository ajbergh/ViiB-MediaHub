import { StateCreator } from 'zustand';
import { AppState, LibrarySlice } from './types';
import { generateSmartMixes } from '../lib/smartMix';
import { SpotifyService } from '../services/spotifyService';
import { libraryService } from '../services/libraryService';
import { Playlist } from '../types';

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

  initLibrary: async () => {
      // Load persistence from IndexedDB
      try {
          const [songs, playlists] = await Promise.all([
              libraryService.getAllSongs(),
              libraryService.getAllPlaylists()
          ]);

          // Restore Blob URLs from persisted Blob data
          // Previously saved URLs are invalid after reload, so we MUST regenerate them.
          songs.forEach(s => {
              if (s.coverData && s.coverData instanceof Blob) {
                  s.coverUrl = URL.createObjectURL(s.coverData);
              }
              // We do not regenerate s.url here (audio src) because keeping thousands of audio blob URLs 
              // in memory is heavy. We rely on JIT generation in playerSlice when playSong is called,
              // or the ephemeral URL if just scanned.
          });
          
          const mixes = generateSmartMixes(songs);
          set({ songs, playlists, smartMixes: mixes });
      } catch (e) {
          console.error("Failed to initialize library from DB", e);
      }
  },

  addSongs: (newSongs) => {
    // 1. Update DB
    libraryService.saveSongs(newSongs).catch(console.error);

    set((state) => {
      // 2. Update State
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
      try {
          // 1. Clear IndexedDB Data
          await libraryService.resetDB();
          
          // 2. Clear Zustand Persist Storage
          localStorage.removeItem('mediahub-storage'); 
          
          // 3. Reset Local State (UI feedback)
          set({ 
              songs: [], 
              playlists: [], 
              smartMixes: [],
              queue: [],
              currentSong: null,
              isPlaying: false
          });
          
          // 4. Force reload to clear all in-memory object URLs and ensure clean slate
          // Small timeout to ensure local storage write completes if any async logic is pending
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
});