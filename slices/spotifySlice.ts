import { StateCreator } from 'zustand';
import { AppState, SpotifySlice } from './types';

export const createSpotifySlice: StateCreator<AppState, [], [], SpotifySlice> = (set) => ({
  spotifyClientId: '',
  spotifyClientSecret: '',
  spotifyAccessToken: null,
  spotifyRefreshToken: null,
  spotifyTokenExpiry: 0,
  spotifyUser: null,
  
  setSpotifyCredentials: (id, secret) => set({ spotifyClientId: id, spotifyClientSecret: secret }),
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
  
  downloadCount: 0,
  setDownloadCount: (count) => set({ downloadCount: count })
});