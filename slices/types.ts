/**
 * ViiB MediaHub - Slice Type Definitions
 * 
 * TypeScript interfaces defining the shape of each Zustand slice.
 * These interfaces ensure type safety across state management.
 * 
 * Slices:
 * - PlayerSlice: Playback, queue, and audio settings
 * - LibrarySlice: Songs, playlists, metadata, scanning
 * - SpotifySlice: OAuth tokens and user profile
 * - UISlice: Context menus, dialogs, logs, panel states
 * 
 * AppState combines all slices into the complete store type.
 * 
 * @module slices/types
 */

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

// Scan folder type (from backend)
export interface ScanFolder {
  id: string;
  path: string;
  addedAt: number;
  lastScan?: number;
  songCount: number;
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
  backendAvailable: boolean;
  scanFolders: ScanFolder[];
  
  initLibrary: () => Promise<void>;
  refreshLibrary: () => Promise<void>;
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
  clearAlbumMetadata: (albumKey: string) => void;
  
  setScanning: (isScanning: boolean) => void;
  setScanProgress: (progress: string) => void;
  
  // Backend folder management
  loadScanFolders: () => Promise<void>;
  addScanFolder: (path: string) => Promise<void>;
  removeScanFolder: (id: string) => Promise<void>;
  startBackendScan: () => Promise<void>;
  pollScanStatus: () => Promise<void>;
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
  
  // Download state
  downloadCount: number;
  setDownloadCount: (count: number) => void;
}

export interface ConfirmDialogConfig {
  title: string;
  message: string;
  confirmLabel?: string;
  cancelLabel?: string;
  variant?: 'danger' | 'warning' | 'default';
  onConfirm: () => void;
}

export interface UISlice {
  isQueueOpen: boolean;
  isNowPlayingOpen: boolean;
  showSmartMixes: boolean;
  hasCompletedSetup: boolean;
  contextMenu: {
    isOpen: boolean;
    x: number;
    y: number;
    type: ContextMenuType | null;
    data: any;
  };
  confirmDialog: ConfirmDialogConfig | null;
  logs: LogEntry[];

  setQueueOpen: (isOpen: boolean) => void;
  setNowPlayingOpen: (isOpen: boolean) => void;
  setShowSmartMixes: (show: boolean) => void;
  setHasCompletedSetup: (completed: boolean) => void;
  
  openContextMenu: (e: React.MouseEvent, type: ContextMenuType, data: any) => void;
  closeContextMenu: () => void;
  
  showConfirmDialog: (config: ConfirmDialogConfig) => void;
  closeConfirmDialog: () => void;
  
  addLog: (level: LogEntry['level'], message: string, details?: any) => void;
  clearLogs: () => void;
}

export type AppState = PlayerSlice & LibrarySlice & SpotifySlice & UISlice;