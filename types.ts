/**
 * ViiB MediaHub - Core Type Definitions
 * 
 * Defines TypeScript interfaces and types used throughout the application:
 * 
 * Core Entities:
 * - Song: Audio file with metadata, URLs, usage metrics, and like status
 * - Album: Aggregated from songs, includes cover and addedAt
 * - Artist: Aggregated from songs with smart name splitting
 * - Playlist: User-created collections with song references
 * - SmartMix: Auto-generated playlists based on rules
 * 
 * Metadata:
 * - ArtistMetadata: Spotify-enriched artist info with images
 * - AlbumMetadata: Spotify-enriched album info with high-res covers
 * - SpotifyProfile: User's Spotify account information
 * 
 * Audio:
 * - AudioSettings: EQ, crossfade, visualizer, normalization config
 * - EqPreset: Named equalizer band configuration
 * - VisualizerMode: Visualization display options
 * 
 * UI:
 * - ViewState: Navigation view identifiers
 * - ContextMenuType: Right-click menu types
 * - LogEntry: Application log entry for debugging
 * 
 * @module types
 */

export type PlaybackContext = 'ai_dj' | 'album' | 'playlist' | 'queue' | 'search' | 'spotify' | 'artist' | 'liked' | 'smart_mix';

export interface Song {
  playbackContext?: PlaybackContext;
  id: string;
  title: string;
  artist: string;
  album: string;
  albumArtist?: string;
  trackNumber?: number;
  discNumber?: number;
  genre?: string[];
  year?: number;
  originalYear?: number; // Original release year (for remasters)
  yearUncertain?: boolean; // True if year may be remaster date
  yearAnalyzedAt?: number; // timestamp of year analysis
  duration: number; // in seconds
  replayGainDb?: number;
  replayPeak?: number;
  url: string; // Ephemeral Blob URL or streaming URL
  fileHandle?: FileSystemFileHandle; // Persisted handle
  coverUrl?: string;
  coverData?: Blob; // Persisted artwork data
  addedAt: number;
  path?: string; // relative path for folder association
  fileHash?: string; // placeholder for hash

  // Spotify streaming support
  spotifyId?: string; // Spotify track ID for direct streaming
  isStreaming?: boolean; // True if currently streaming from Spotify (not downloaded)

  // Usage Metrics
  playCount?: number;
  lastPlayed?: number; // timestamp
  skipCount?: number;

  // AI-analyzed mood/energy fields (from Gemini)
  mood?: string; // e.g., "happy", "sad", "energetic", "calm"
  energy?: string; // e.g., "high", "medium", "low"
  tempo?: string; // e.g., "fast", "medium", "slow"
  bpm?: number; // Beats per minute
  instrumental?: boolean; // true if song has no vocals
  moodAnalyzedAt?: number; // timestamp of mood analysis

  // Last.fm enrichment
  lastfmListeners?: number;
  lastfmPlaycount?: number;
  lastfmTags?: string;
  lastfmUrl?: string;
  lastfmMbid?: string;
  lastfmEnrichedAt?: number;

  // User preferences
  liked?: boolean; // true if user has liked this song
  likedAt?: number; // timestamp when song was liked
}

export interface Album {
  name: string;
  artist: string;
  songCount: number;
  coverUrl?: string;
  addedAt?: number; // Most recent addedAt from album's songs
}

export interface Artist {
  name: string;
  songCount: number;
  albumCount: number;
  imageUrl?: string;
}

export interface Playlist {
  id: string;
  name: string;
  songIds: string[];
  coverUrl?: string;
  createdAt: number;
}

export interface SmartMix {
  id: string;
  name: string;
  description: string;
  coverColors: string[];
  songIds: string[];
  rules: string;
  updatedAt: number;
}

export interface ArtistMetadata {
  spotifyId?: string;
  name: string;
  imageUrl: string; // High res
  url: string;
  fetchedAt: number;
}

export interface AlbumMetadata {
  spotifyId?: string;
  name: string;
  artist: string;
  coverUrl: string; // High res
  description?: string; // Spotify doesn't always provide desc, making optional
  genre?: string;
  releaseDate: string;
  url: string;
  copyright?: string;
  fetchedAt: number;
}

export interface SpotifyProfile {
  id: string;
  display_name: string;
  email: string;
  images: { url: string; height: number; width: number }[];
  product: string; // 'premium', 'free', 'open'
  country: string;
  followers: { href: string | null; total: number };
  external_urls: { spotify: string };
}

export interface LogEntry {
  id: string;
  timestamp: number;
  level: 'info' | 'warn' | 'error' | 'success';
  message: string;
  details?: any;
}

export enum ViewState {
  HOME = 'HOME',
  SONGS = 'SONGS',
  ALBUMS = 'ALBUMS',
  ARTISTS = 'ARTISTS',
  PLAYLISTS = 'PLAYLISTS',
  SMART_MIXES = 'SMART_MIXES',
  SPOTIFY = 'SPOTIFY',
  DOWNLOADS = 'DOWNLOADS',
  SEARCH = 'SEARCH',
  SETTINGS = 'SETTINGS',
}

export enum ContextMenuType {
  SONG = 'SONG',
  ALBUM = 'ALBUM',
  ARTIST = 'ARTIST',
  PLAYLIST = 'PLAYLIST',
  SMART_MIX = 'SMART_MIX',
  QUEUE_ITEM = 'QUEUE_ITEM'
}

export type HomeLayoutVariant = 'shelves' | 'coverWall' | 'dashboard';

// --- Audio Enhancement Types ---

/**
 * Audio Visualizer Mode
 * 
 * Defines 12 available visualization modes for the Now Playing view.
 * Canvas 2D modes render audio-reactive graphics, MILKDROP uses WebGL via Butterchurn.
 * 
 * Basic Modes:
 * - OFF: No visualization
 * - WAVE: Smooth glowing waveform with quadratic curve interpolation
 * - SPECTRUM: Circular frequency bars radiating from center (sun-burst effect)
 * 
 * Custom Canvas 2D Modes:
 * - FLAME_SPECTRUM: Stylized flame tongues rising with frequency-based height and color intensity
 * - STARDUST_HALO: Pulsing particle halo with stardust bursts on bass hits
 * - AURORA_RIBBON: Translucent ribbon with waveform modulation and frequency-based colors
 * - ELECTRIC_ARC: TRON-style geometric light beams with crackling effects on treble
 * - GRASS_OSCILLOSCOPE: Organic swaying grass blades with amplitude height and stereo sway
 * - FIREFLY_FIELD: Drifting fireflies with warm glow and gentle flicker (seasonal)
 * - TUNNEL_WAVEFORM: 3D tunnel of pulsating rings with perspective depth
 * - WIND_FIELD: Flowing particle wind effect with bass intensity and treble sparkles
 * 
 * WebGL Mode:
 * - MILKDROP: Butterchurn-powered Winamp preset visualizations (GPU-accelerated)
 * 
 * Audio Mapping:
 * - Bass (0-30 Hz): Triggers bursts, expansions, intensity
 * - Mid (30-150 Hz): Controls thickness, height, density
 * - Treble (150-300 Hz): Sparkles, glints, shimmer effects
 * 
 * Performance:
 * - Canvas 2D modes target 60 FPS rendering
 * - Particle systems capped at 40-300 particles
 * - Automatic cleanup on mode switch
 * 
 * @see AlbumArtVisualizer - Component that renders Canvas 2D visualizations
 * @see MilkdropVisualizer - Component that renders WebGL Butterchurn visualizations
 * @see audioEngine - Web Audio API wrapper providing frequency data
 */
export type VisualizerMode = 
  | 'OFF' 
  | 'WAVE' 
  | 'SPECTRUM' 
  | 'FLAME_SPECTRUM'
  | 'STARDUST_HALO'
  | 'AURORA_RIBBON'
  | 'ELECTRIC_ARC'
  | 'GRASS_OSCILLOSCOPE'
  | 'FIREFLY_FIELD'
  | 'TUNNEL_WAVEFORM'
  | 'WIND_FIELD'
  | 'MILKDROP';

export interface EqPreset {
  id: string;
  name: string;
  gains: number[]; // 10 bands
}

export interface AudioSettings {
  crossfadeDuration: number; // seconds (0-12)
  gapless: boolean;
  normalization: boolean;
  visualizerMode: VisualizerMode;
  visualizerEnabled: boolean;
  /** Opacity of album artwork when visualizer is active (0-100) */
  visualizerArtworkOpacity: number;
  /** Enable fullscreen background visualizer behind entire Now Playing UI */
  visualizerFullscreenEnabled: boolean;
  /** Opacity of fullscreen background visualizer (0-100) */
  visualizerFullscreenOpacity: number;
  /** Background visualizer mode - can differ from album art mode */
  visualizerBackgroundMode: VisualizerMode;
  eqEnabled: boolean;
  eqBands: number[]; // 10 bands, -12 to +12 dB
  activePresetId: string;
  /** DJ Mode: Main/Live output audio device ID (empty = default) */
  mainOutputDevice: string;
  /** DJ Mode: Headphone/Cue output audio device ID (empty = default) */
  headphoneOutputDevice: string;
}

/**
 * Milkdrop Visualization Settings
 * 
 * Configuration for WebGL-based Milkdrop visualizations via Butterchurn.
 * Stored separately from AudioSettings to manage the additional preset state.
 * 
 * @see MilkdropVisualizer - Component that uses these settings
 */
export interface MilkdropSettings {
  /** Whether Milkdrop is the active visualizer */
  enabled: boolean;
  /** Currently selected preset key (null = random) */
  currentPreset: string | null;
  /** Auto-cycle through presets */
  presetCycleEnabled: boolean;
  /** Seconds between preset changes (15-120) */
  presetCycleInterval: number;
  /** Transition blend duration in seconds (0-5) */
  blendDuration: number;
  /** Rendering quality/resolution setting */
  quality: 'low' | 'medium' | 'high';
  /** User's favorite preset keys */
  favoritePresets: string[];
}

/**
 * Milkdrop Preset Info for UI display
 */
export interface MilkdropPresetInfo {
  /** Display name of the preset */
  name: string;
  /** Unique key for loading the preset */
  key: string;
  /** Whether user has favorited this preset */
  isFavorite: boolean;
  /** Optional category for organization */
  category?: string;
}
