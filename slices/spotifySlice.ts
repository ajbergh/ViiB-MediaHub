/**
 * ViiB MediaHub - Spotify State Slice
 * 
 * Zustand slice managing Spotify integration state.
 * 
 * State:
 * - spotifyClientId/Secret: OAuth application credentials
 * - spotifyAccessToken/RefreshToken: User authentication tokens
 * - spotifyTokenExpiry: Token expiration timestamp
 * - spotifyUser: Authenticated user profile
 * 
 * Only the client ID and non-sensitive preferences are persisted by the
 * renderer's localStorage store. Access/refresh tokens are never written to
 * renderer localStorage; the Go backend may persist the encrypted session and
 * the app restores valid credentials into renderer memory at startup.
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
