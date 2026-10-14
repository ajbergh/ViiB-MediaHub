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
  setSpotifyConnected: (connected) => set(state => {
    const nextGeneration = state.spotifySessionGeneration + (state.spotifyConnected !== connected ? 1 : 0);
    return {
      spotifyConnected: connected,
      spotifyAuthRequired: connected ? false : state.spotifyAuthRequired,
      spotifySessionGeneration: nextGeneration,
      ...(!connected ? {
        spotifyUser: null,
        spotifySearchResults: null,
        spotifyLastSearchedQuery: '',
        spotifyRecentlyPlayed: null,
        spotifySavedAlbums: null,
        spotifySavedPlaylists: null,
        spotifyCacheGeneration: nextGeneration,
      } : {})
    };
  }),
  markSpotifyAuthRequired: () => {
    get().retireSpotifyPlayback();
    set(state => {
      const nextGeneration = state.spotifySessionGeneration + 1;
      return {
        spotifyConnected: false, spotifyAuthRequired: true,
        spotifySessionGeneration: nextGeneration,
        spotifyUser: null, spotifySearchResults: null,
        spotifyLastSearchedQuery: '',
        spotifyRecentlyPlayed: null,
        spotifySavedAlbums: null,
        spotifySavedPlaylists: null,
        spotifyCacheGeneration: nextGeneration,
        spotifyAccessToken: null, spotifyRefreshToken: null, spotifyTokenExpiry: 0
      };
    });
  },
  spotifyClientId: '',
  spotifyClientSecret: '',
  spotifyAccessToken: null,
  spotifyRefreshToken: null,
  spotifyTokenExpiry: 0,
  spotifyUser: null,
  
  // Search & library persistence state
  spotifySearchQuery: '',
  spotifyLastSearchedQuery: '',
  spotifySearchResults: null,
  spotifyActiveTab: 'search',
  spotifyRecentlyPlayed: null,
  spotifySavedAlbums: null,
  spotifySavedPlaylists: null,
  spotifyCacheGeneration: 0,
  
  setSpotifyCredentials: (id, _secret) => set({ spotifyClientId: id, spotifyClientSecret: '' }),
  setSpotifyTokens: (accessToken, refreshToken, expiry) => set({ 
      spotifyAccessToken: accessToken, 
      spotifyRefreshToken: refreshToken, 
      spotifyTokenExpiry: expiry 
  }),
  setSpotifyUser: (user) => set(state => ({
    spotifyUser: user,
    spotifyCacheGeneration: state.spotifySessionGeneration,
  })),
  logoutSpotify: () => {
    get().retireSpotifyPlayback();
    set(state => {
      const nextGeneration = state.spotifySessionGeneration + 1;
      return {
        spotifySessionGeneration: nextGeneration,
        spotifyAuthRequired: false,
        spotifyConnected: false,
        spotifySearchResults: null,
        spotifyLastSearchedQuery: '',
        spotifyRecentlyPlayed: null,
        spotifySavedAlbums: null,
        spotifySavedPlaylists: null,
        spotifyCacheGeneration: nextGeneration,
        spotifyAccessToken: null, 
        spotifyRefreshToken: null, 
        spotifyTokenExpiry: 0, 
        spotifyUser: null 
      };
    });
  },
  
  // Search & library persistence actions
  setSpotifySearchQuery: (query) => set({ spotifySearchQuery: query }),
  setSpotifyLastSearchedQuery: (query) => set(state => ({
    spotifyLastSearchedQuery: query,
    spotifyCacheGeneration: state.spotifySessionGeneration,
  })),
  setSpotifySearchResults: (results) => set(state => ({
    spotifySearchResults: results,
    spotifyCacheGeneration: state.spotifySessionGeneration,
  })),
  setSpotifyActiveTab: (tab) => set({ spotifyActiveTab: tab }),
  setSpotifyRecentlyPlayed: (data) => set(state => ({
    spotifyRecentlyPlayed: data,
    spotifyCacheGeneration: state.spotifySessionGeneration,
  })),
  setSpotifySavedAlbums: (data) => set(state => ({
    spotifySavedAlbums: data,
    spotifyCacheGeneration: state.spotifySessionGeneration,
  })),
  setSpotifySavedPlaylists: (data) => set(state => ({
    spotifySavedPlaylists: data,
    spotifyCacheGeneration: state.spotifySessionGeneration,
  })),
  
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
