/**
 * ViiB MediaHub - Player State Slice
 * 
 * Zustand slice managing audio playback state and settings.
 * 
 * State:
 * - isPlaying: Current playback state
 * - currentSong: Currently loaded song object
 * - queue: Array of songs in playback queue
 * - volume: Master volume level (0-1)
 * - audioSettings: EQ, crossfade, visualizer configuration
 * 
 * Features:
 * - Song playback with automatic file handle resolution
 * - Queue management (add, remove, clear, shuffle)
 * - Auto-EQ based on song genre
 * - Navigation between songs (next/prev)
 * - Play count recording
 * 
 * @module playerSlice
 */

import { StateCreator } from 'zustand';
import { AppState, PlayerSlice } from './types';
import { EQ_PRESETS } from '../utils';
import { libraryService } from '../services/libraryService';

export const createPlayerSlice: StateCreator<AppState, [], [], PlayerSlice> = (set, get) => ({
  isPlaying: false,
  currentSong: null,
  currentSongIndex: -1,
  queue: [],
  volume: 0.8,
  audioSettings: {
    crossfadeDuration: 0,
    gapless: false,
    normalization: false,
    visualizerMode: 'SPECTRUM',
    visualizerEnabled: true,
    eqEnabled: false,
    eqBands: [0, 0, 0, 0, 0, 0, 0, 0, 0, 0],
    activePresetId: 'flat',
  },
  isEqOpen: false,

  playSong: async (song, context) => {
    // 1. Resolve URL if missing (e.g. after page reload)
    let playableSong = { ...song };
    
    // Check if we need to regenerate Blob URL from File Handle
    if ((!playableSong.url || playableSong.url.startsWith('blob:') === false) && playableSong.fileHandle) {
         try {
             // Verify permission or request it (browser might show prompt)
             const permitted = await libraryService.verifyPermission(playableSong.fileHandle);
             if (permitted) {
                 const file = await playableSong.fileHandle.getFile();
                 playableSong.url = URL.createObjectURL(file);
                 // We don't save this ephemeral URL to DB, just to state
             } else {
                 console.warn("Permission denied for file handle");
                 // Fallback or error handling
             }
         } catch (e) {
             console.error("Failed to resolve file handle for playback", e);
         }
    }

    const { songs } = get();
    const newQueue = context ? [...context] : [...songs];
    
    // Use ID to match, ensuring we map to the possibly updated playableSong object
    const index = newQueue.findIndex(s => s.id === song.id);
    const validIndex = index !== -1 ? index : 0;
    
    // If context was passed, we might need to update the object in queue if it was stale
    if (index !== -1) {
        newQueue[index] = playableSong;
    }
    const finalQueue = index !== -1 ? newQueue : [playableSong];

    // Simple Auto-EQ Logic based on Genre
    if (playableSong.genre && playableSong.genre.length > 0) {
        const genre = playableSong.genre[0].toLowerCase();
        let presetId = 'flat';
        if (genre.includes('rock') || genre.includes('metal')) presetId = 'rock';
        else if (genre.includes('pop')) presetId = 'pop';
        else if (genre.includes('jazz')) presetId = 'jazz';
        else if (genre.includes('classical')) presetId = 'classical';
        else if (genre.includes('electronic') || genre.includes('dance')) presetId = 'electronic';
        
        if (presetId !== 'flat') {
            const preset = EQ_PRESETS.find(p => p.id === presetId);
            if (preset) {
                set(state => ({
                    audioSettings: {
                        ...state.audioSettings,
                        activePresetId: presetId,
                        eqBands: [...preset.gains]
                    }
                }));
            }
        }
    }

    set({ 
        currentSong: playableSong, 
        currentSongIndex: validIndex,
        queue: finalQueue,
        isPlaying: true 
    });
  },

  togglePlay: () => set((state) => ({ isPlaying: !state.isPlaying })),

  nextSong: () => {
    const { queue, currentSongIndex, playSong } = get();
    if (queue.length === 0) return;
    
    if (currentSongIndex >= queue.length - 1) {
        set({ isPlaying: false });
        return;
    }
    
    const nextIndex = currentSongIndex + 1;
    // We call playSong to handle the file handle resolution for the next track
    playSong(queue[nextIndex], queue); 
  },

  prevSong: () => {
    const { queue, currentSongIndex, playSong } = get();
    if (queue.length === 0) return;
    
    const prevIndex = (currentSongIndex - 1 + queue.length) % queue.length;
    playSong(queue[prevIndex], queue);
  },

  setVolume: (volume) => set({ volume }),

  addToQueue: (items) => {
      const { queue, currentSong } = get();
      const itemsArr = Array.isArray(items) ? items : [items];
      const newQueue = [...queue, ...itemsArr];
      
      if (!currentSong && itemsArr.length > 0) {
          // Play first item using playSong to resolve handle
          get().playSong(itemsArr[0], newQueue);
      } else {
          set({ queue: newQueue });
      }
  },

  playNext: (items) => {
      const { queue, currentSongIndex } = get();
      const itemsArr = Array.isArray(items) ? items : [items];
      const newQueue = [...queue];
      newQueue.splice(currentSongIndex + 1, 0, ...itemsArr);
      set({ queue: newQueue });
  },

  removeFromQueue: (index) => {
      const { queue, currentSongIndex } = get();
      const newQueue = [...queue];
      newQueue.splice(index, 1);
      
      let newIndex = currentSongIndex;
      if (index < currentSongIndex) {
          newIndex = Math.max(0, currentSongIndex - 1);
      } 
      else if (index === currentSongIndex) {
          newIndex = index < newQueue.length ? index : Math.max(0, newQueue.length - 1);
      }

      set({
          queue: newQueue,
          currentSongIndex: newIndex,
          currentSong: newQueue.length > 0 ? newQueue[newIndex] : null,
          isPlaying: newQueue.length > 0 ? get().isPlaying : false
      });
  },

  clearQueue: () => {
      const { currentSong } = get();
      if (currentSong) {
          set({ queue: [currentSong], currentSongIndex: 0 });
      } else {
          set({ queue: [], currentSongIndex: -1, isPlaying: false });
      }
  },

  reorderQueue: (fromIndex, toIndex) => {
      const { queue, currentSongIndex } = get();
      const newQueue = [...queue];
      const [movedItem] = newQueue.splice(fromIndex, 1);
      newQueue.splice(toIndex, 0, movedItem);

      let newCurrentIndex = currentSongIndex;
      if (currentSongIndex === fromIndex) {
          newCurrentIndex = toIndex;
      } else if (currentSongIndex > fromIndex && currentSongIndex <= toIndex) {
          newCurrentIndex--;
      } else if (currentSongIndex < fromIndex && currentSongIndex >= toIndex) {
          newCurrentIndex++;
      }

      set({ queue: newQueue, currentSongIndex: newCurrentIndex });
  },

  playQueueItem: (index) => {
      const { queue, playSong } = get();
      if (index >= 0 && index < queue.length) {
          playSong(queue[index], queue);
      }
  },

  setVisualizerMode: (mode) => set((state) => ({ audioSettings: { ...state.audioSettings, visualizerMode: mode } })),
  setEqEnabled: (enabled) => set((state) => ({ audioSettings: { ...state.audioSettings, eqEnabled: enabled } })),
  setEqBand: (index, gain) => set((state) => {
      const newBands = [...state.audioSettings.eqBands];
      newBands[index] = gain;
      return { 
          audioSettings: { 
              ...state.audioSettings, 
              eqBands: newBands,
              activePresetId: 'custom'
          } 
      };
  }),
  setEqPreset: (presetId) => set((state) => {
      const preset = EQ_PRESETS.find(p => p.id === presetId);
      if (!preset) return state;
      return {
          audioSettings: {
              ...state.audioSettings,
              activePresetId: presetId,
              eqBands: [...preset.gains]
          }
      };
  }),
  setCrossfade: (val) => set((state) => ({ audioSettings: { ...state.audioSettings, crossfadeDuration: val } })),
  setGapless: (val) => set((state) => ({ audioSettings: { ...state.audioSettings, gapless: val } })),
  setNormalization: (val) => set((state) => ({ audioSettings: { ...state.audioSettings, normalization: val } })),
  toggleEqPanel: () => set((state) => ({ isEqOpen: !state.isEqOpen })),
});
