/**
 * ViiB MediaHub - Core Type Definitions
 * 
 * Defines TypeScript interfaces and types used throughout the application:
 * 
 * Core Entities:
 * - Song: Audio file with metadata, URLs, and usage metrics
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

export interface Song {
  id: string;
  title: string;
  artist: string;
  album: string;
  albumArtist?: string;
  trackNumber?: number;
  discNumber?: number;
  genre?: string[];
  year?: number;
  duration: number; // in seconds
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

// --- Audio Enhancement Types ---

/**
 * Audio Visualizer Mode
 * 
 * Defines 21 available visualization modes for the Now Playing view.
 * Each mode renders audio-reactive graphics using Canvas 2D API.
 * 
 * Classic Modes (Original 6):
 * - OFF: No visualization
 * - WAVE: Smooth glowing waveform with quadratic curve interpolation
 * - SPECTRUM: Circular frequency bars radiating from center (sun-burst effect)
 * - AURORA: Ambient flowing gradients reacting to bass/mid/treble bands
 * - CIRCULAR: Enhanced circular with rotating bars, pulsing rings, inner waveform
 * - PARTICLES: Dynamic particle system with gravity effects and audio-reactive spawning
 * - NEBULA: Cosmic atmosphere with swirling nebula clouds, stars, and lens flares
 * 
 * Next-Gen Modes (15 New):
 * - FLAME_SPECTRUM: Stylized flame tongues rising with frequency-based height and color intensity
 * - STARDUST_HALO: Pulsing particle halo with stardust bursts on bass hits
 * - AURORA_RIBBON: Translucent ribbon with waveform modulation and frequency-based colors
 * - ELECTRIC_ARC: TRON-style geometric light beams with crackling effects on treble
 * - GRASS_OSCILLOSCOPE: Organic swaying grass blades with amplitude height and stereo sway
 * - CRYSTAL_SHARDS: Prismatic diamond shards bursting outward with refraction effects
 * - WATERCOLOR_BLOOM: Painterly circular blooms with multi-layer depth
 * - ICE_FRACTURE: Cracking ice radiating from center with branching fractures
 * - FIREFLY_FIELD: Drifting fireflies with warm glow and gentle flicker (seasonal)
 * - VINYL_SPIN: Rotating vinyl grooves with tempo-based rotation and treble glints
 * - BEAT_ORBS: Volumetric orbs expanding on bass hits with soft gradients
 * - TUNNEL_WAVEFORM: 3D tunnel of pulsating rings with perspective depth
 * - GLASS_SHARDS: Reflective rotating glass fragments with prismatic colors
 * - WIND_FIELD: Flowing particle wind effect with bass intensity and treble sparkles
 * 
 * Audio Mapping:
 * - Bass (0-30 Hz): Triggers bursts, expansions, intensity
 * - Mid (30-150 Hz): Controls thickness, height, density
 * - Treble (150-300 Hz): Sparkles, glints, shimmer effects
 * 
 * Performance:
 * - All modes target 60 FPS rendering
 * - Particle systems capped at 40-300 particles
 * - Canvas 2D for broad compatibility
 * - Automatic cleanup on mode switch
 * 
 * @see AlbumArtVisualizer - Component that renders these visualizations
 * @see audioEngine - Web Audio API wrapper providing frequency data
 */
export type VisualizerMode = 
  | 'OFF' 
  | 'WAVE' 
  | 'SPECTRUM' 
  | 'AURORA' 
  | 'CIRCULAR' 
  | 'PARTICLES' 
  | 'NEBULA'
  | 'FLAME_SPECTRUM'
  | 'STARDUST_HALO'
  | 'AURORA_RIBBON'
  | 'ELECTRIC_ARC'
  | 'GRASS_OSCILLOSCOPE'
  | 'CRYSTAL_SHARDS'
  | 'WATERCOLOR_BLOOM'
  | 'ICE_FRACTURE'
  | 'FIREFLY_FIELD'
  | 'VINYL_SPIN'
  | 'BEAT_ORBS'
  | 'TUNNEL_WAVEFORM'
  | 'GLASS_SHARDS'
  | 'WIND_FIELD';

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
  eqEnabled: boolean;
  eqBands: number[]; // 10 bands, -12 to +12 dB
  activePresetId: string;
}