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
 * DJ Mode State (new):
 * - djMode: Whether DJ mode is active (vs standard playlist mode)
 * - djPersona: Selected DJ persona
 * - djTargetDurationMinutes: Target set duration
 * - djFlowStrictness: BPM continuity strictness (0-100)
 * - djTalkMode: Whether to show DJ narration cues
 * - djPlan: Current DJ set plan (phases, intent)
 * - djPhases: Phase results with selected songs
 * - djNarration: Optional DJ talk lines
 * 
 * This slice is persisted to localStorage, allowing users to navigate away
 * and return to their previous AI DJ session with search results intact.
 * 
 * @module aiDjSlice
 */

import { StateCreator } from 'zustand';
import { Song } from '../types';
import { 
  SmartPlaylistFilter, 
  DJPersona, 
  DJSetPlan, 
  DJPhaseResult, 
  DJNarration 
} from '../services/api';
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
  
  // DJ Mode state
  aiDjMode: boolean; // true = DJ mode, false = playlist mode
  aiDjPersona: DJPersona;
  aiDjTargetDurationMinutes: number;
  aiDjFlowStrictness: number; // 0-100
  aiDjTalkMode: boolean;
  aiDjPlan: DJSetPlan | null;
  aiDjPhases: DJPhaseResult[];
  aiDjNarration: DJNarration | null;
  
  // Actions
  setAIDJPrompt: (prompt: string) => void;
  setAIDJGeneratedSongs: (songs: Song[]) => void;
  setAIDJFilter: (filter: SmartPlaylistFilter | null) => void;
  setAIDJIsLoading: (isLoading: boolean) => void;
  setAIDJDiscoverMode: (mode: 'balanced' | 'discover' | 'favorites') => void;
  setAIDJAvoidRecentlyHours: (hours: number) => void;
  setAIDJOnePerArtist: (onePerArtist: boolean) => void;
  setAIDJUseTimeContext: (useTimeContext: boolean) => void;
  
  // DJ Mode actions
  setAIDJMode: (djMode: boolean) => void;
  setAIDJPersona: (persona: DJPersona) => void;
  setAIDJTargetDurationMinutes: (minutes: number) => void;
  setAIDJFlowStrictness: (strictness: number) => void;
  setAIDJTalkMode: (talkMode: boolean) => void;
  setAIDJPlan: (plan: DJSetPlan | null) => void;
  setAIDJPhases: (phases: DJPhaseResult[]) => void;
  setAIDJNarration: (narration: DJNarration | null) => void;
  
  // Bulk update for generation result
  setAIDJResult: (prompt: string, songs: Song[], filter: SmartPlaylistFilter | null) => void;
  
  // Bulk update for DJ mode result
  setAIDJDJResult: (
    prompt: string, 
    songs: Song[], 
    filter: SmartPlaylistFilter | null,
    plan: DJSetPlan | null,
    phases: DJPhaseResult[],
    narration: DJNarration | null
  ) => void;
  
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
  
  // DJ Mode initial state
  aiDjMode: false,
  aiDjPersona: 'FlowMaster' as DJPersona,
  aiDjTargetDurationMinutes: 45,
  aiDjFlowStrictness: 60,
  aiDjTalkMode: false,
  aiDjPlan: null,
  aiDjPhases: [],
  aiDjNarration: null,
  
  // Actions
  setAIDJPrompt: (prompt) => set({ aiDjPrompt: prompt }),
  
  setAIDJGeneratedSongs: (songs) => set({ aiDjGeneratedSongs: songs }),
  
  setAIDJFilter: (filter) => set({ aiDjFilter: filter }),
  
  setAIDJIsLoading: (isLoading) => set({ aiDjIsLoading: isLoading }),
  
  setAIDJDiscoverMode: (mode) => set({ aiDjDiscoverMode: mode }),
  
  setAIDJAvoidRecentlyHours: (hours) => set({ aiDjAvoidRecentlyHours: hours }),
  
  setAIDJOnePerArtist: (onePerArtist) => set({ aiDjOnePerArtist: onePerArtist }),
  
  setAIDJUseTimeContext: (useTimeContext) => set({ aiDjUseTimeContext: useTimeContext }),
  
  // DJ Mode actions
  setAIDJMode: (djMode) => set({ aiDjMode: djMode }),
  
  setAIDJPersona: (persona) => set({ aiDjPersona: persona }),
  
  setAIDJTargetDurationMinutes: (minutes) => set({ aiDjTargetDurationMinutes: minutes }),
  
  setAIDJFlowStrictness: (strictness) => set({ aiDjFlowStrictness: strictness }),
  
  setAIDJTalkMode: (talkMode) => set({ aiDjTalkMode: talkMode }),
  
  setAIDJPlan: (plan) => set({ aiDjPlan: plan }),
  
  setAIDJPhases: (phases) => set({ aiDjPhases: phases }),
  
  setAIDJNarration: (narration) => set({ aiDjNarration: narration }),
  
  // Bulk update for generation result
  setAIDJResult: (prompt, songs, filter) => set({
    aiDjPrompt: prompt,
    aiDjGeneratedSongs: songs,
    aiDjFilter: filter,
  }),
  
  // Bulk update for DJ mode result
  setAIDJDJResult: (prompt, songs, filter, plan, phases, narration) => set({
    aiDjPrompt: prompt,
    aiDjGeneratedSongs: songs,
    aiDjFilter: filter,
    aiDjPlan: plan,
    aiDjPhases: phases,
    aiDjNarration: narration,
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
    aiDjMode: false,
    aiDjPersona: 'FlowMaster' as DJPersona,
    aiDjTargetDurationMinutes: 45,
    aiDjFlowStrictness: 60,
    aiDjTalkMode: false,
    aiDjPlan: null,
    aiDjPhases: [],
    aiDjNarration: null,
  }),
});
