/**
 * ViiB MediaHub - AI DJ State Slice
 * 
 * Zustand slice managing the AI DJ feature state.
 * 
 * State:
 * - prompt: Last search query entered by user
 * - generatedSongs: Songs from last generation
 * - filter: Filter/metadata from last generation
 * - isLoading: Generation in progress
 * - discoverMode: 'balanced', 'discover', or 'favorites'
 * - avoidRecentlyHours: Time window for avoiding recently played songs (0 = off)
 * - onePerArtist: Whether to limit to one song per artist
 * - useTimeContext: Whether to use time-of-day context in recommendations
 * 
 * This slice is persisted to localStorage, allowing users to navigate away
 * and return to their previous AI DJ session with search results intact.
 * 
 * @module aiDjSlice
 */

import { StateCreator } from 'zustand';
import { Song } from '../types';
import { SmartPlaylistFilter } from '../services/api';
import { AppState } from './types';

export interface AIDJSlice {
  // Search and results
  aiDjPrompt: string;
  aiDjGeneratedSongs: Song[];
  aiDjFilter: SmartPlaylistFilter | null;
  aiDjIsLoading: boolean;
  
  // User preferences
  aiDjDiscoverMode: 'balanced' | 'discover' | 'favorites';
  aiDjAvoidRecentlyHours: number;
  aiDjOnePerArtist: boolean;
  aiDjUseTimeContext: boolean;
  
  // Actions
  setAIDJPrompt: (prompt: string) => void;
  setAIDJGeneratedSongs: (songs: Song[]) => void;
  setAIDJFilter: (filter: SmartPlaylistFilter | null) => void;
  setAIDJIsLoading: (isLoading: boolean) => void;
  setAIDJDiscoverMode: (mode: 'balanced' | 'discover' | 'favorites') => void;
  setAIDJAvoidRecentlyHours: (hours: number) => void;
  setAIDJOnePerArtist: (onePerArtist: boolean) => void;
  setAIDJUseTimeContext: (useTimeContext: boolean) => void;
  
  // Bulk update for generation result
  setAIDJResult: (prompt: string, songs: Song[], filter: SmartPlaylistFilter | null) => void;
  
  // Clear all AI DJ state
  clearAIDJ: () => void;
}

export const createAIDJSlice: StateCreator<AppState, [], [], AIDJSlice> = (set) => ({
  // Initial state
  aiDjPrompt: '',
  aiDjGeneratedSongs: [],
  aiDjFilter: null,
  aiDjIsLoading: false,
  aiDjDiscoverMode: 'balanced',
  aiDjAvoidRecentlyHours: 0,
  aiDjOnePerArtist: false,
  aiDjUseTimeContext: false,
  
  // Actions
  setAIDJPrompt: (prompt) => set({ aiDjPrompt: prompt }),
  
  setAIDJGeneratedSongs: (songs) => set({ aiDjGeneratedSongs: songs }),
  
  setAIDJFilter: (filter) => set({ aiDjFilter: filter }),
  
  setAIDJIsLoading: (isLoading) => set({ aiDjIsLoading: isLoading }),
  
  setAIDJDiscoverMode: (mode) => set({ aiDjDiscoverMode: mode }),
  
  setAIDJAvoidRecentlyHours: (hours) => set({ aiDjAvoidRecentlyHours: hours }),
  
  setAIDJOnePerArtist: (onePerArtist) => set({ aiDjOnePerArtist: onePerArtist }),
  
  setAIDJUseTimeContext: (useTimeContext) => set({ aiDjUseTimeContext: useTimeContext }),
  
  // Bulk update for generation result
  setAIDJResult: (prompt, songs, filter) => set({
    aiDjPrompt: prompt,
    aiDjGeneratedSongs: songs,
    aiDjFilter: filter,
  }),
  
  // Clear all AI DJ state
  clearAIDJ: () => set({
    aiDjPrompt: '',
    aiDjGeneratedSongs: [],
    aiDjFilter: null,
    aiDjIsLoading: false,
    aiDjDiscoverMode: 'balanced',
    aiDjAvoidRecentlyHours: 0,
    aiDjOnePerArtist: false,
    aiDjUseTimeContext: false,
  }),
});
