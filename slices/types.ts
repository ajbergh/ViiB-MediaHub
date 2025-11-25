import React from 'react';
import { Song, Playlist, SmartMix, ArtistMetadata, AlbumMetadata, SpotifyProfile, LogEntry, AudioSettings, VisualizerMode, ContextMenuType } from '../types';

export interface PlayerSlice {
  isPlaying: boolean;
  currentSong: Song | null;
  currentSongIndex: number;
  queue: Song[];
  volume: number;
  audioSettings: AudioSettings;
  isEqOpen: boolean;
  
  playSong: (song: Song, context?: Song[]) => Promise<void>;
  togglePlay: () => void;
  nextSong: () => void;
  prevSong: () => void;
  setVolume: (vol: number) => void;
  addToQueue: (items: Song | Song[]) => void;
  playNext: (items: Song | Song[]) => void;
  removeFromQueue: (index: number) => void;
  clearQueue: () => void;
  reorderQueue: (fromIndex: number, toIndex: number) => void;
  playQueueItem: (index: number) => void;
  
  setVisualizerMode: (mode: VisualizerMode) => void;
  setEqEnabled: (enabled: boolean) => void;
  setEqBand: (index: number, gain: number) => void;
  setEqPreset: (presetId: string) => void;
  setCrossfade: (seconds: number) => void;
  setGapless: (enabled: boolean) => void;
  setNormalization: (enabled: boolean) => void;
  toggleEqPanel: () => void;
}

export interface LibrarySlice {
  songs: Song[];
  playlists: Playlist[];
  smartMixes: SmartMix[];
  artistMetadata: Record<string, ArtistMetadata>;
  albumMetadata: Record<string, AlbumMetadata>;
  fetchingArtists: Set<string>;
  fetchingAlbums: Set<string>;
  isScanning: boolean;
  scanProgress: string;
  
  initLibrary: () => Promise<void>;
  addSongs: (newSongs: Song[]) => void;
  resetLibrary: () => Promise<void>;
  createPlaylist: (name: string) => void;
  addToPlaylist: (playlistId: string, songId: string) => void;
  deletePlaylist: (playlistId: string) => void;
  
  refreshSmartMixes: () => void;
  saveSmartMixAsPlaylist: (mixId: string) => void;
  recordPlay: (songId: string) => void;
  
  fetchArtistMetadata: (artistName: string) => Promise<void>;
  fetchAlbumMetadata: (albumName: string, artistName: string) => Promise<void>;
  
  setScanning: (isScanning: boolean) => void;
  setScanProgress: (progress: string) => void;
}

export interface SpotifySlice {
  spotifyClientId: string;
  spotifyClientSecret: string;
  spotifyAccessToken: string | null;
  spotifyRefreshToken: string | null;
  spotifyTokenExpiry: number;
  spotifyUser: SpotifyProfile | null;
  
  setSpotifyCredentials: (clientId: string, clientSecret: string) => void;
  setSpotifyTokens: (accessToken: string, refreshToken: string, expiry: number) => void;
  setSpotifyUser: (user: SpotifyProfile | null) => void;
  logoutSpotify: () => void;
}

export interface UISlice {
  isQueueOpen: boolean;
  isNowPlayingOpen: boolean;
  showSmartMixes: boolean;
  contextMenu: {
    isOpen: boolean;
    x: number;
    y: number;
    type: ContextMenuType | null;
    data: any;
  };
  logs: LogEntry[];

  setQueueOpen: (isOpen: boolean) => void;
  setNowPlayingOpen: (isOpen: boolean) => void;
  setShowSmartMixes: (show: boolean) => void;
  
  openContextMenu: (e: React.MouseEvent, type: ContextMenuType, data: any) => void;
  closeContextMenu: () => void;
  
  addLog: (level: LogEntry['level'], message: string, details?: any) => void;
  clearLogs: () => void;
}

export type AppState = PlayerSlice & LibrarySlice & SpotifySlice & UISlice;