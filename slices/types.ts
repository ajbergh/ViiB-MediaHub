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
 * - UISlice: Context menus, dialogs, logs, panel states, toast notifications
 * - AIDJSlice: AI DJ search state and preferences (persisted)
 * 
 * Key Types:
 * - ToastConfig: Toast notification structure (type, message, action, duration)
 * - ConfirmDialogConfig: Confirmation dialog options
 * - ScanFolder: Music folder for library scanning
 * 
 * AppState combines all slices into the complete store type.
 * 
 * @module slices/types
 */

import React from 'react';
import { Song, Playlist, SmartMix, ArtistMetadata, AlbumMetadata, SpotifyProfile, LogEntry, AudioSettings, VisualizerMode, ContextMenuType } from '../types';
import { SmartPlaylistFilter, DJPersona, DJSetPlan, DJPhaseResult, DJNarration } from '../services/api';

export interface PlayerSlice {
  isPlaying: boolean;
  currentSong: Song | null;
  currentSongIndex: number;
  queue: Song[];
  volume: number;
  audioSettings: AudioSettings;
  isEqOpen: boolean;
  
  // Buffering state for streaming
  isBuffering: boolean;
  bufferProgress: number; // 0-100 percentage
  preloadedTrackId: string | null; // ID of preloaded next track
  
  // Streaming error state
  streamError: StreamingError | null;
  retryCount: number;
  
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
  loadAudioSettings: () => Promise<void>; // Load audio settings from backend on startup
  
  // Buffering actions
  setBuffering: (isBuffering: boolean) => void;
  setBufferProgress: (progress: number) => void;
  preloadNextTrack: () => Promise<void>;
  setPreloadedTrackId: (id: string | null) => void;
  
  // Error recovery actions
  setStreamError: (error: StreamingError | null) => void;
  retryStream: () => Promise<void>;
  clearStreamError: () => void;
  
  // Streaming analytics
  streamingStats: StreamingStats;
  recordStreamEvent: (event: StreamingEvent) => void;
  resetStreamingStats: () => void;
}

// Streaming error types
export type StreamingErrorType = 'network' | 'auth' | 'unavailable' | 'unknown';

export interface StreamingError {
  type: StreamingErrorType;
  message: string;
  canRetry: boolean;
  timestamp: number;
}

// Streaming analytics types
export interface StreamingStats {
  totalStreams: number;
  successfulStreams: number;
  failedStreams: number;
  totalBufferingTime: number; // in seconds
  bufferingEvents: number;
  averageBufferingDuration: number; // in seconds
  errorsByType: Record<StreamingErrorType, number>;
  lastStreamedTrack: string | null;
  sessionStartTime: number;
}

export type StreamingEventType = 'start' | 'complete' | 'error' | 'buffer_start' | 'buffer_end' | 'retry' | 'skip';

/**
 * Playback context for AI DJ preference learning.
 * Tracks where the user initiated playback from.
 */
export type PlaybackContext = 'ai_dj' | 'album' | 'playlist' | 'queue' | 'search' | 'artist' | 'liked' | 'smart_mix';

export interface StreamingEvent {
  type: StreamingEventType;
  trackId?: string;
  trackTitle?: string;
  errorType?: StreamingErrorType;
  duration?: number; // for buffer events, duration in ms
  timestamp: number;
}

// Scan folder type (from backend)
export interface ScanFolder {
  id: string;
  path: string;
  addedAt: number;
  lastScan?: number;
  songCount: number;
}

// Genre enrichment progress
export interface EnrichmentStatus {
  isEnriching: boolean;
  totalSongs: number;
  processedSongs: number;
  currentBatch: number;
  totalBatches: number;
  message: string;
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
  
  // Likes state
  likedSongIds: Set<string>;
  likedAlbumKeys: Set<string>;
  
  // Genre enrichment state
  enrichmentStatus: EnrichmentStatus;
  
  initLibrary: () => Promise<void>;
  refreshLibrary: () => Promise<void>;
  addSongs: (newSongs: Song[]) => void;
  resetLibrary: () => Promise<void>;
  createPlaylist: (name: string, songIds?: string[]) => Promise<Playlist | void>;
  addToPlaylist: (playlistId: string, songId: string) => void;
  deletePlaylist: (playlistId: string) => Promise<void>;
  
  refreshSmartMixes: () => void;
  saveSmartMixAsPlaylist: (mixId: string) => Promise<Playlist | void>;
  recordPlay: (songId: string) => void;
  /**
   * Record a listening event for AI DJ preference learning.
   * Called when a song ends or is skipped.
   * @param songId - The song ID
   * @param playDuration - Seconds played before event
   * @param songDuration - Total song duration
   * @param context - 'ai_dj' | 'album' | 'playlist' | 'queue' | 'search'
   */
  recordListenEvent: (songId: string, playDuration: number, songDuration: number, context: PlaybackContext) => void;
  updateSongDuration: (songId: string, duration: number) => void;
  
  fetchArtistMetadata: (artistName: string) => Promise<void>;
  fetchAlbumMetadata: (albumName: string, artistName: string) => Promise<void>;
  clearAlbumMetadata: (albumKey: string) => void;
  
  setScanning: (isScanning: boolean) => void;
  setScanProgress: (progress: string) => void;
  setEnrichmentStatus: (status: Partial<EnrichmentStatus>) => void;
  
  // Likes management
  toggleLikeSong: (songId: string) => Promise<void>;
  syncLikedSongs: () => Promise<void>;
  isLikedSong: (songId: string) => boolean;
  
  // Album likes management
  toggleLikeAlbum: (albumKey: string) => Promise<void>;
  syncLikedAlbums: () => Promise<void>;
  isLikedAlbum: (albumKey: string) => boolean;
  
  // Backend folder management
  loadScanFolders: () => Promise<void>;
  addScanFolder: (path: string) => Promise<void>;
  removeScanFolder: (id: string) => Promise<void>;
  startBackendScan: () => Promise<void>;
  startQuickScan: () => Promise<void>;
  pollScanStatus: () => Promise<void>;
}

export interface SpotifySlice {
  spotifyClientId: string;
  spotifyClientSecret: string;
  spotifyAccessToken: string | null;
  spotifyRefreshToken: string | null;
  spotifyTokenExpiry: number;
  spotifyUser: SpotifyProfile | null;
  
  // Search persistence
  spotifySearchQuery: string;
  spotifySearchResults: any | null;
  spotifyActiveTab: 'search' | 'recent' | 'albums' | 'playlists';
  
  setSpotifyCredentials: (clientId: string, clientSecret: string) => void;
  setSpotifyTokens: (accessToken: string, refreshToken: string, expiry: number) => void;
  setSpotifyUser: (user: SpotifyProfile | null) => void;
  logoutSpotify: () => void;
  
  // Search persistence actions
  setSpotifySearchQuery: (query: string) => void;
  setSpotifySearchResults: (results: any | null) => void;
  setSpotifyActiveTab: (tab: 'search' | 'recent' | 'albums' | 'playlists') => void;
  
  // Download state
  downloadCount: number;
  setDownloadCount: (count: number) => void;
  
  // Streaming settings
  streamingEnabled: boolean;
  streamingQuality: 'high' | 'medium' | 'low';
  preferLocalPlayback: boolean; // When true, prefer downloaded files over streaming
  setStreamingEnabled: (enabled: boolean) => void;
  setStreamingQuality: (quality: 'high' | 'medium' | 'low') => void;
  setPreferLocalPlayback: (prefer: boolean) => void;
}

export interface ConfirmDialogConfig {
  title: string;
  message: string;
  confirmLabel?: string;
  cancelLabel?: string;
  variant?: 'danger' | 'warning' | 'default';
  onConfirm: () => void;
}

export interface ToastConfig {
  id: string;
  type: 'success' | 'error' | 'info' | 'warning';
  message: string;
  duration?: number;
  action?: {
    label: string;
    onClick: () => void;
  };
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
  toasts: ToastConfig[];
  
  // Local search persistence
  localSearchQuery: string;
  localSearchTab: 'all' | 'tracks' | 'albums' | 'artists' | 'playlists';

  setQueueOpen: (isOpen: boolean) => void;
  setNowPlayingOpen: (isOpen: boolean) => void;
  setShowSmartMixes: (show: boolean) => void;
  setHasCompletedSetup: (completed: boolean) => void;
  
  openContextMenu: (e: React.MouseEvent, type: ContextMenuType, data: any) => void;
  closeContextMenu: () => void;
  
  showConfirmDialog: (config: ConfirmDialogConfig) => void;
  closeConfirmDialog: () => void;
  
  showToast: (toast: Omit<ToastConfig, 'id'>) => void;
  dismissToast: (id: string) => void;
  
  addLog: (level: LogEntry['level'], message: string, details?: any) => void;
  clearLogs: () => void;
  
  // Local search persistence
  setLocalSearchQuery: (query: string) => void;
  setLocalSearchTab: (tab: 'all' | 'tracks' | 'albums' | 'artists' | 'playlists') => void;
}

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
  setAIDJResult: (prompt: string, songs: Song[], filter: SmartPlaylistFilter | null) => void;
  
  // DJ Mode actions
  setAIDJMode: (djMode: boolean) => void;
  setAIDJPersona: (persona: DJPersona) => void;
  setAIDJTargetDurationMinutes: (minutes: number) => void;
  setAIDJFlowStrictness: (strictness: number) => void;
  setAIDJTalkMode: (talkMode: boolean) => void;
  setAIDJPlan: (plan: DJSetPlan | null) => void;
  setAIDJPhases: (phases: DJPhaseResult[]) => void;
  setAIDJNarration: (narration: DJNarration | null) => void;
  
  // Bulk update for DJ mode result
  setAIDJDJResult: (
    prompt: string, 
    songs: Song[], 
    filter: SmartPlaylistFilter | null,
    plan: DJSetPlan | null,
    phases: DJPhaseResult[],
    narration: DJNarration | null
  ) => void;
  
  clearAIDJ: () => void;
}

export type AppState = PlayerSlice & LibrarySlice & SpotifySlice & UISlice & AIDJSlice;