/**
 * Zustand slice for Spotify connection, account generation, profile, search,
 * playback preferences, and download count. Disconnect and authentication failure
 * retire Spotify playback and invalidate requests from the prior generation.
 * Legacy OAuth fields remain for compatibility. Cookies and active Web Player
 * bearer tokens stay in the backend; renderer persistence excludes secrets and tokens.
 *
 * @module spotifySlice
 */

import { StateCreator } from 'zustand';
import { AppState, SpotifySlice } from './types';

export const createSpotifySlice: StateCreator<AppState, [], [], SpotifySlice> = (set, get) => ({
  spotifyConnected: false,
  spotifySessionGeneration: 0,
  spotifyAuthRequired: false,
  setSpotifyConnected: (connected) => set(state => ({
    spotifyConnected: connected,
    spotifyAuthRequired: connected ? false : state.spotifyAuthRequired,
    spotifySessionGeneration: state.spotifySessionGeneration + (state.spotifyConnected !== connected ? 1 : 0),
    ...(!connected ? {spotifyUser: null, spotifySearchResults: null} : {})
  })),
  markSpotifyAuthRequired: () => {
    get().retireSpotifyPlayback();
    set(state => ({
      spotifyConnected: false, spotifyAuthRequired: true,
      spotifySessionGeneration: state.spotifySessionGeneration + 1,
      spotifyUser: null, spotifySearchResults: null,
      spotifyAccessToken: null, spotifyRefreshToken: null, spotifyTokenExpiry: 0
    }));
  },
  spotifyClientId: '',
  spotifyClientSecret: '',
  spotifyAccessToken: null,
  spotifyRefreshToken: null,
  spotifyTokenExpiry: 0,
  spotifyUser: null,
  
  // Search persistence state
  spotifySearchQuery: '',
  spotifySearchResults: null,
  spotifyActiveTab: 'search',
  
  setSpotifyCredentials: (id, _secret) => set({ spotifyClientId: id, spotifyClientSecret: '' }),
  setSpotifyTokens: (accessToken, refreshToken, expiry) => set({ 
      spotifyAccessToken: accessToken, 
      spotifyRefreshToken: refreshToken, 
      spotifyTokenExpiry: expiry 
  }),
  setSpotifyUser: (user) => set({ spotifyUser: user }),
  logoutSpotify: () => {
    get().retireSpotifyPlayback();
    set(state => ({
      spotifySessionGeneration: state.spotifySessionGeneration + 1,
      spotifyAuthRequired: false,
      spotifyConnected: false,
      spotifySearchResults: null,
      spotifyAccessToken: null, 
      spotifyRefreshToken: null, 
      spotifyTokenExpiry: 0, 
      spotifyUser: null 
    }));
  },
  
  // Search persistence actions
  setSpotifySearchQuery: (query) => set({ spotifySearchQuery: query }),
  setSpotifySearchResults: (results) => set({ spotifySearchResults: results }),
  setSpotifyActiveTab: (tab) => set({ spotifyActiveTab: tab }),
  
  downloadCount: 0,
  setDownloadCount: (count) => set({ downloadCount: count }),
  
  // Streaming settings - persisted via Zustand persist
  streamingEnabled: true, // Default: streaming enabled
  streamingQuality: 'high', // Default: high quality (320kbps)
  preferLocalPlayback: true, // Default: prefer downloaded files over streaming
  setStreamingEnabled: (enabled) => set({ streamingEnabled: enabled }),
  setStreamingQuality: (quality) => set({ streamingQuality: quality }),
  setPreferLocalPlayback: (prefer) => set({ preferLocalPlayback: prefer })
});
