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
 * Only the client ID and non-sensitive preferences are persisted to localStorage.
 * Access/refresh tokens are held in-memory only (not persisted) to prevent
 * XSS token theft — re-authentication occurs on app restart.
 * Credentials are also synced with the Go backend for download functionality.
 * 
 * @module spotifySlice
 */

import { StateCreator } from 'zustand';
import { AppState, SpotifySlice } from './types';

export const createSpotifySlice: StateCreator<AppState, [], [], SpotifySlice> = (set) => ({
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
  logoutSpotify: () => set({ 
      spotifyAccessToken: null, 
      spotifyRefreshToken: null, 
      spotifyTokenExpiry: 0, 
      spotifyUser: null 
  }),
  
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