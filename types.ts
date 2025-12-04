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

export type VisualizerMode = 'OFF' | 'WAVE' | 'SPECTRUM' | 'AURORA';

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