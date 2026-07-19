/**
 * API Client for ViiB MediaHub Backend
 * 
 * This module provides a typed interface for communicating with the Go backend.
 * All API calls go through this module to ensure consistent error handling and typing.
 * 
 * Architecture:
 * - Development: Vite dev server proxies /api requests to Go backend (port 8080)
 * - Production: Backend serves both the built frontend and API from same origin
 * - All responses are JSON with consistent error format
 * - TypeScript interfaces match backend Go structs
 * 
 * Features:
 * - Library management (songs, playlists, folders)
 * - File scanning and metadata extraction
 * - Spotify OAuth credential storage
 * - Spotify download queue management
 * - Real-time download progress via SSE (handled by DownloadManager component)
 * 
 * AI DJ Features:
 * - generateSmartPlaylist: Natural language playlist generation with multi-tier matching
 * - enrichGenresStream: SSE-based genre enrichment using the configured LLM provider
 * - enrichMoodStream: SSE-based mood/energy/tempo analysis using the configured LLM provider
 * 
 * Last.FM Features (added 2025-12-31):
 * - getLastFMSettings/saveLastFMSettings: API key configuration
 * - testLastFMConnection: Verify API credentials
 * - authenticateLastFM: Mobile auth for scrobbling
 * - getLastFMStatus: Connection status and enrichment stats
 * - triggerLastFMEnrichment: Batch enrich songs with Last.FM tags
 * - getLastFMTrackInfo/getLastFMSimilarTracks: Individual track lookups
 * 
 * Smart Playlist Options:
 * - blendMode: 'single' or 'mixed' for genre blending
 * - discoverMode: 'balanced', 'discover', 'favorites' for play history preferences
 * - avoidRecentlyHours: Exclude recently played songs
 * - onePerArtist: Limit to one song per artist for variety
 * - useTimeContext: Add time-of-day context to recommendations
 */

import { AudioSettings } from '../types';

const API_BASE = '/api';

/**
 * Standard error response from backend
 */
interface ApiError {
  error: string;
}

/**
 * Handles HTTP responses and errors consistently.
 * Throws on non-OK status codes with error message from backend.
 * 
 * @param response - Fetch API Response object
 * @returns Parsed JSON response
 * @throws Error with backend error message or HTTP status
 */
async function handleResponse<T>(response: Response): Promise<T> {
  if (!response.ok) {
    const error: ApiError = await response.json().catch(() => ({ error: 'Unknown error' }));
    throw new Error(error.error || `HTTP ${response.status}`);
  }
  return response.json();
}

// Song types matching backend
export interface ApiSong {
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
  duration: number;
  replayGainDb?: number;
  replayPeak?: number;
  replayGainDb?: number;
  replayPeak?: number;
  filePath: string; // This will be the API URL like /api/audio/{id}
  coverPath?: string; // This will be /api/cover/{id}
  addedAt: number;
  playCount?: number;
  lastPlayed?: number;
  skipCount?: number;
  fileHash?: string;
  mood?: string;
  energy?: string;
  tempo?: string;
  bpm?: number;
  instrumental?: boolean;
  moodAnalyzedAt?: number;
  lastfmListeners?: number;
  lastfmPlaycount?: number;
  lastfmTags?: string;
  lastfmUrl?: string;
  lastfmMbid?: string;
  lastfmEnrichedAt?: number;
  // User preferences
  liked?: boolean;
  likedAt?: number;
}

export interface DuplicateSong extends ApiSong {
  sourcePath?: string;
}

export interface DuplicateGroup {
  fileHash: string;
  songs: DuplicateSong[];
}

export interface M3UImportResult {
  playlist: ApiPlaylist;
  matched: number;
  unmatched: string[];
}

export interface ApiPlaylist {
  id: string;
  name: string;
  songIds: string[];
  coverPath?: string;
  createdAt: number;
}

export interface ScanFolder {
  id: string;
  path: string;
  addedAt: number;
  lastScan?: number;
  songCount: number;
}

/**
 * Progress update from genre enrichment SSE stream
 */
export interface EnrichmentProgress {
  status: 'started' | 'processing' | 'batch_complete' | 'complete' | 'error';
  message: string;
  totalSongs: number;
  processedSongs: number;
  currentBatch: number;
  totalBatches: number;
  error?: string;
}

export interface FolderEntry {
  name: string;
  path: string;
  isDir: boolean;
}

export interface BrowseResult {
  currentPath: string;
  entries: FolderEntry[];
}

export interface ScanStatus {
  scanning: boolean;
  progress: string;
}

export interface GenreStat {
  name: string;
  count: number;
  topArtists: string[];
  coverUrl?: string;
}

/**
 * Represents a Spotify download in the queue.
 * Matches the SpotifyDownload struct in backend/internal/db/db.go.
 * 
 * Download Lifecycle:
 * 1. queued - Added to queue, waiting to be processed
 * 2. downloading - Currently being downloaded from Spotify
 * 3. completed - Successfully downloaded to disk
 * 4. failed - Download failed (see errorMessage)
 * 
 * Progress updates are received via SSE (Server-Sent Events) from
 * the /api/spotify/downloads/events endpoint.
 */
export interface ApiSpotifyDownload {
  id: string;                    // UUID generated by backend
  spotifyId: string;             // Spotify track ID (e.g., "3n3Ppam7vgaVa1iaRUc9Lp")
  type: 'track' | 'album' | 'playlist'; // Download type
  title: string;                 // Track/album/playlist title
  artist: string;                // Primary artist
  album: string;                 // Album name
  status: 'queued' | 'downloading' | 'completed' | 'failed'; // Current status
  progress: number;              // 0-100 percentage
  errorMessage?: string;         // Error details if status is 'failed'
  filePath?: string;             // Full path to downloaded file (if completed)
  addedAt: number;               // Unix timestamp when queued
  startedAt?: number;            // Unix timestamp when download started
  completedAt?: number;          // Unix timestamp when completed
  artworkUrl?: string;           // Album/playlist artwork URL from Spotify
}

// API Functions

export const api = {
  _smartPlaylistAbort: null as AbortController | null,

  // Songs
  async getSongs(): Promise<ApiSong[]> {
    const response = await fetch(`${API_BASE}/songs`);
    return handleResponse(response);
  },

  async getGenres(): Promise<GenreStat[]> {
    const response = await fetch(`${API_BASE}/genres`);
    const data = await handleResponse<GenreStat[] | null>(response);
    // Ensure we always return an array, and topArtists is always defined
    return (data || []).map(g => ({
      ...g,
      topArtists: g.topArtists || []
    }));
  },

  async normalizeGenres(): Promise<{ normalized: number; errors: number }> {
    const response = await fetch(`${API_BASE}/genres/normalize`, { method: 'POST' });
    return handleResponse(response);
  },

  async clearSongs(): Promise<void> {
    const response = await fetch(`${API_BASE}/songs`, { method: 'DELETE' });
    await handleResponse(response);
  },

  async recordPlay(songId: string): Promise<void> {
    const response = await fetch(`${API_BASE}/songs/${songId}/play`, { method: 'POST' });
    await handleResponse(response);
  },

  /**
   * Record a listening event for AI DJ preference learning.
   * Called when a song ends or is skipped to track user behavior.
   * @param songId - The ID of the song
   * @param playDuration - How many seconds the song was played
   * @param songDuration - Total duration of the song
   * @param context - Playback context: 'ai_dj', 'album', 'playlist', 'queue', 'search'
   * @returns The auto-detected event type
   */
  async recordListenEvent(songId: string, playDuration: number, songDuration: number, context: string): Promise<{ status: string; eventType: string }> {
    const response = await fetch(`${API_BASE}/songs/${songId}/listen-event`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ playDuration, songDuration, context }),
    });
    return handleResponse(response);
  },

  /**
   * Update the duration of a song.
   * Used to fix incorrect durations from metadata extraction with the actual audio duration.
   * @param songId - The ID of the song to update
   * @param duration - The correct duration in seconds
   */
  async updateSongDuration(songId: string, duration: number): Promise<void> {
    const response = await fetch(`${API_BASE}/songs/${songId}/duration`, {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ duration }),
    });
    await handleResponse(response);
  },

  // Likes
  /**
   * Toggle the liked status of a song.
   * @param songId - The ID of the song to like/unlike
   * @returns The new liked state and timestamp
   */
  async toggleLike(songId: string): Promise<{ id: string; liked: boolean; likedAt: number }> {
    const response = await fetch(`${API_BASE}/songs/${songId}/like`, { method: 'POST' });
    return handleResponse(response);
  },

  /**
   * Get all liked song IDs.
   * @returns Array of song IDs that are liked
   */
  async getLikedSongIds(): Promise<string[]> {
    const response = await fetch(`${API_BASE}/songs/liked`);
    const data = await handleResponse<{ ids: string[] }>(response);
    return data.ids;
  },

  /**
   * Bulk like or unlike multiple songs at once.
   * Useful for liking/unliking all songs in an album.
   * @param songIds - Array of song IDs to update
   * @param liked - True to like, false to unlike
   * @returns Number of songs updated
   */
  async bulkLikeSongs(songIds: string[], liked: boolean): Promise<{ updated: number }> {
    const response = await fetch(`${API_BASE}/songs/like/bulk`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ songIds, liked }),
    });
    return handleResponse(response);
  },

  // Album Likes
  /**
   * Toggle the liked status of an album.
   * @param albumKey - The album key in "AlbumName::ArtistName" format
   * @returns The new liked state and timestamp
   */
  async toggleAlbumLike(albumKey: string): Promise<{ albumKey: string; liked: boolean; likedAt: number }> {
    const response = await fetch(`${API_BASE}/albums/${encodeURIComponent(albumKey)}/like`, { method: 'POST' });
    return handleResponse(response);
  },

  /**
   * Get all liked album keys.
   * @returns Array of album keys that are liked
   */
  async getLikedAlbumKeys(): Promise<string[]> {
    const response = await fetch(`${API_BASE}/albums/liked`);
    const data = await handleResponse<{ albumKeys: string[] }>(response);
    return data.albumKeys;
  },

  /**
   * Get all liked albums with full metadata.
   * @returns Array of album metadata objects
   */
  async getLikedAlbums(): Promise<ApiAlbumMetadata[]> {
    const response = await fetch(`${API_BASE}/albums/liked/full`);
    return handleResponse(response);
  },

  // Playlists
  async getPlaylists(): Promise<ApiPlaylist[]> {
    const response = await fetch(`${API_BASE}/playlists`);
    return handleResponse(response);
  },

  async createPlaylist(name: string, songIds: string[] = []): Promise<ApiPlaylist> {
    const response = await fetch(`${API_BASE}/playlists`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name, songIds }),
    });
    return handleResponse(response);
  },

  async updatePlaylist(id: string, data: Partial<ApiPlaylist>): Promise<ApiPlaylist> {
    const response = await fetch(`${API_BASE}/playlists/${id}`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(data),
    });
    return handleResponse(response);
  },

  async deletePlaylist(id: string): Promise<void> {
    const response = await fetch(`${API_BASE}/playlists/${id}`, { method: 'DELETE' });
    await handleResponse(response);
  },

  // Folders
  async getFolders(): Promise<ScanFolder[]> {
    const response = await fetch(`${API_BASE}/folders`);
    return handleResponse(response);
  },

  async addFolder(path: string): Promise<ScanFolder> {
    const response = await fetch(`${API_BASE}/folders`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ path }),
    });
    return handleResponse(response);
  },

  async removeFolder(id: string): Promise<void> {
    const response = await fetch(`${API_BASE}/folders/${id}`, { method: 'DELETE' });
    await handleResponse(response);
  },

  // Library integrity
  async getDuplicateGroups(): Promise<DuplicateGroup[]> {
    const response = await fetch(`${API_BASE}/library/duplicates`);
    return handleResponse(response);
  },

  async getIgnoredSongs(): Promise<DuplicateSong[]> {
    const response = await fetch(`${API_BASE}/library/duplicates/ignored`);
    return handleResponse(response);
  },

  async setDuplicateIgnored(songId: string, ignored: boolean): Promise<void> {
    const response = await fetch(`${API_BASE}/library/duplicates/ignore`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ songId, ignored }),
    });
    await handleResponse(response);
  },

  async importPlaylistM3U(name: string, content: string): Promise<M3UImportResult> {
    const response = await fetch(`${API_BASE}/playlists/import/m3u`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name, content }),
    });
    return handleResponse(response);
  },

  async exportPlaylistM3U(id: string): Promise<Blob> {
    const response = await fetch(`${API_BASE}/playlists/${encodeURIComponent(id)}/export.m3u`);
    if (!response.ok) {
      const error = await response.json().catch(() => ({ error: `HTTP ${response.status}` }));
      throw new Error(error.error || `HTTP ${response.status}`);
    }
    return response.blob();
  },

  // Scanning
  async startScan(): Promise<void> {
    const response = await fetch(`${API_BASE}/scan`, { method: 'POST' });
    await handleResponse(response);
  },

  async startQuickScan(): Promise<void> {
    const response = await fetch(`${API_BASE}/scan/quick`, { method: 'POST' });
    await handleResponse(response);
  },

  async getScanStatus(): Promise<ScanStatus> {
    const response = await fetch(`${API_BASE}/scan/status`);
    return handleResponse(response);
  },

  // Browse folders
  async browseFolder(path?: string): Promise<BrowseResult> {
    const response = await fetch(`${API_BASE}/browse`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ path: path || '' }),
    });
    return handleResponse(response);
  },

  // Health check
  async healthCheck(): Promise<{ status: string; version: string }> {
    const response = await fetch(`${API_BASE}/health`);
    return handleResponse(response);
  },

  // Helper to get audio URL
  getAudioUrl(songId: string): string {
    return `${API_BASE}/audio/${songId}`;
  },

  // Helper to get cover URL
  getCoverUrl(songId: string): string {
    return `${API_BASE}/cover/${songId}`;
  },

  // Helper to get Spotify stream URL
  // Used for direct streaming of Spotify tracks without downloading
  // Quality parameter: 'high' (320kbps), 'medium' (160kbps), 'low' (96kbps)
  getSpotifyStreamUrl(spotifyId: string, quality?: 'high' | 'medium' | 'low'): string {
    const qualityParam = quality ? `?quality=${quality}` : '';
    return `${API_BASE}/spotify/stream/${spotifyId}${qualityParam}`;
  },

  // Spotify Credentials

  /**
   * Saves Spotify OAuth credentials to backend database.
   * Credentials are stored in the spotify_credentials setting as JSON.
   * The access token is used for both Web API calls and librespot downloads.
   * 
   * @param creds - OAuth credentials from Spotify authorization flow
   * @returns Promise resolving to success response
   */
  async saveSpotifyCredentials(creds: { clientId: string; clientSecret: string; accessToken: string; refreshToken: string; expiry: number; codeVerifier?: string; oauthState?: string; redirectUri?: string }) {
    const response = await fetch(`${API_BASE}/spotify/credentials`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(creds),
    });
    return handleResponse(response);
  },

  /**
   * Retrieves stored Spotify OAuth credentials from backend.
   * Used for checking if user is authenticated and for token refresh.
   * 
   * @returns Promise resolving to credentials or empty object if not set
   */
  async getSpotifyCredentials() {
    const response = await fetch(`${API_BASE}/spotify/credentials`);
    return handleResponse<{ clientId: string; clientSecret: string; accessToken: string; refreshToken: string; expiry: number; codeVerifier?: string; oauthState?: string; redirectUri?: string }>(response);
  },

  /**
   * Searches Spotify playlists using web scraping fallback.
   * The Spotify Web API doesn't return "First Party" playlists (Made For You, 
   * Discover Weekly, etc.) in search results. This endpoint uses web scraping
   * to find all playlists including first-party ones.
   * 
   * @param query - Search query string
   * @returns Promise resolving to playlist search results in Spotify API format
   */
  async searchPlaylistsFallback(query: string): Promise<{ playlists: { items: any[]; total: number } }> {
    const response = await fetch(`${API_BASE}/spotify/search/playlists?q=${encodeURIComponent(query)}`);
    return handleResponse<{ playlists: { items: any[]; total: number } }>(response);
  },

  /**
   * Gets Spotify playlist details using web scraping fallback.
   * This is used for first-party playlists that return 404/403 from the Web API.
   * 
   * @param playlistId - Spotify playlist ID
   * @returns Promise resolving to playlist details in Spotify API format
   */
  async getPlaylistByScraping(playlistId: string): Promise<any> {
    const response = await fetch(`${API_BASE}/spotify/playlists/${playlistId}/scrape`);
    return handleResponse<any>(response);
  },

  // Spotify Downloads

  /**
   * Queues a single track for download from Spotify.
   * Requires Spotify Premium account and valid OAuth authentication.
   * Track is downloaded as OGG Vorbis format using librespot-go.
   * 
   * Backend Process:
   * 1. Adds download to spotify_downloads table with status 'queued'
   * 2. Background worker picks up download
   * 3. Authenticates with Spotify using OAuth token
   * 4. Streams and saves to {downloadDir}/{artist}/{track}.ogg
   * 5. Updates status to 'completed' or 'failed'
   * 
   * @param spotifyId - Spotify track ID
   * @param title - Track title
   * @param artist - Primary artist name
   * @param album - Album name
   * @param duration - Track duration in seconds
   * @returns Promise with download ID and confirmation message
   */
  async downloadTrack(spotifyId: string, title: string, artist: string, album: string, duration: number) {
    const response = await fetch(`${API_BASE}/spotify/download/track`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ spotifyId, title, artist, album, duration }),
    });
    return handleResponse<{ id: string; message: string }>(response);
  },

  /**
   * Queues an entire album for download from Spotify.
   * Backend fetches all tracks from Spotify Web API and queues each individually.
   * 
   * @param spotifyId - Spotify album ID
   * @param title - Album title
   * @param artist - Primary artist name
   * @returns Promise with confirmation message
   */
  async downloadAlbum(spotifyId: string, title: string, artist: string) {
    const response = await fetch(`${API_BASE}/spotify/download/album`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ spotifyId, title, artist }),
    });
    return handleResponse<{ id: string; message: string }>(response);
  },

  /**
   * Queues an entire playlist for download from Spotify.
   * Backend fetches all tracks from Spotify Web API and queues each individually.
   * 
   * @param spotifyId - Spotify playlist ID
   * @param name - Playlist name
   * @param owner - Playlist owner name
   * @returns Promise with confirmation message
   */
  async downloadPlaylist(spotifyId: string, name: string, owner: string) {
    const response = await fetch(`${API_BASE}/spotify/download/playlist`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ spotifyId, name, owner }),
    });
    return handleResponse<{ id: string; message: string }>(response);
  },

  /**
   * Downloads content directly from a Spotify URL or URI.
   * Supports tracks, albums, and playlists.
   * 
   * Accepted formats:
   * - https://open.spotify.com/track/4iV5W9uYEdYUVa79Axb7Rh
   * - https://open.spotify.com/album/1DFixLWuPkv3KT3TnV35m3
   * - https://open.spotify.com/playlist/37i9dQZF1DXcBWIGoYBM5M
   * - spotify:track:4iV5W9uYEdYUVa79Axb7Rh
   * - spotify:album:1DFixLWuPkv3KT3TnV35m3
   * - spotify:playlist:37i9dQZF1DXcBWIGoYBM5M
   * 
   * @param url - Spotify URL or URI
   * @returns Promise with download result including type, title, and count (for albums/playlists)
   */
  async downloadFromURL(url: string) {
    const response = await fetch(`${API_BASE}/spotify/download/url`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ url }),
    });
    return handleResponse<{
      status: string;
      type: string;
      title: string;
      artist?: string;
      owner?: string;
      album?: string;
      message: string;
      count?: number;
    }>(response);
  },

  /**
   * Retrieves all downloads from the queue.
   * Includes queued, downloading, completed, and failed downloads.
   * Use this for initial state load; real-time updates come via SSE.
   * 
   * @returns Promise with array of all downloads
   */
  async getDownloads() {
    const response = await fetch(`${API_BASE}/spotify/downloads`);
    return handleResponse<ApiSpotifyDownload[]>(response);
  },

  /**
   * Gets the current status of a specific download.
   * 
   * @param id - Download ID (UUID)
   * @returns Promise with download details
   */
  async getDownloadStatus(id: string) {
    const response = await fetch(`${API_BASE}/spotify/downloads/${id}`);
    return handleResponse<ApiSpotifyDownload>(response);
  },

  /**
   * Deletes a download from the queue.
   * Cannot delete downloads that are currently in progress.
   * Does NOT delete the downloaded file from disk (if completed).
   * 
   * @param id - Download ID (UUID)
   * @returns Promise resolving when deleted
   */
  async deleteDownload(id: string) {
    const response = await fetch(`${API_BASE}/spotify/downloads/${id}`, { method: 'DELETE' });
    await handleResponse(response);
  },

  /**
   * Retries a failed download by resetting it to queued status.
   * 
   * @param id - Download ID (UUID)
   * @returns Promise resolving when reset
   */
  async retryDownload(id: string) {
    const response = await fetch(`${API_BASE}/spotify/downloads/${id}/retry`, { method: 'POST' });
    await handleResponse(response);
  },

  /**
   * Force restarts a stuck/stalled download.
   * Cancels any active download and requeues it.
   * 
   * @param id - Download ID (UUID)
   * @returns Promise resolving when reset
   */
  async forceRestartDownload(id: string) {
    const response = await fetch(`${API_BASE}/spotify/downloads/${id}/force-restart`, { method: 'POST' });
    await handleResponse(response);
  },

  /**
   * Gets the current Spotify authentication status.
   * Returns whether re-authentication is required (e.g., token expired/revoked).
   * 
   * @returns Promise with authRequired status and optional message
   */
  async getSpotifyAuthStatus(): Promise<{ authRequired: boolean; message: string }> {
    const response = await fetch(`${API_BASE}/spotify/auth/status`);
    return handleResponse<{ authRequired: boolean; message: string }>(response);
  },

  /**
   * Clears the auth required flag after successful re-authentication.
   * Called after the user completes Spotify OAuth flow.
   * 
   * @returns Promise resolving when cleared
   */
  async refreshSpotifyAuth() {
    const response = await fetch(`${API_BASE}/spotify/auth/refresh`, { method: 'POST' });
    await handleResponse(response);
  },

  /**
   * Clears all completed downloads from the queue.
   * Does NOT delete the downloaded files from disk.
   * 
   * @returns Promise with count of cleared downloads
   */
  async clearCompletedDownloads(): Promise<{ count: number }> {
    const response = await fetch(`${API_BASE}/spotify/downloads/completed`, { method: 'DELETE' });
    return handleResponse<{ status: string; count: number }>(response);
  },

  // Settings

  /**
   * Gets a setting value from the backend.
   * 
   * @param key - Setting key
   * @returns Promise with setting value (empty string if not set)
   */
  async getSetting(key: string): Promise<string> {
    const response = await fetch(`${API_BASE}/settings/${key}`);
    const data = await handleResponse<{ key: string; value: string }>(response);
    return data.value;
  },

  /**
   * Sets a setting value in the backend.
   * 
   * @param key - Setting key
   * @param value - Setting value
   * @returns Promise resolving when saved
   */
  async setSetting(key: string, value: string) {
    const response = await fetch(`${API_BASE}/settings/${key}`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ value }),
    });
    await handleResponse(response);
  },

  /**
   * Gets audio settings from the backend database.
   * Audio settings are stored as JSON in the 'audio_settings' key.
   * 
   * @returns Promise with audio settings object or null if not set
   */
  async getAudioSettings(): Promise<AudioSettings | null> {
    try {
      const value = await this.getSetting('audio_settings');
      if (!value) return null;
      return JSON.parse(value) as AudioSettings;
    } catch (e) {
      console.warn('Failed to load audio settings from backend:', e);
      return null;
    }
  },

  /**
   * Saves audio settings to the backend database.
   * 
   * @param settings - Audio settings object to save
   * @returns Promise resolving when saved
   */
  async saveAudioSettings(settings: AudioSettings): Promise<void> {
    await this.setSetting('audio_settings', JSON.stringify(settings));
  },

  // Album Metadata Cache

  /**
   * Gets all cached album metadata from the backend.
   * Used for fast startup to avoid re-fetching from Spotify.
   * 
   * @returns Promise with all cached album metadata
   */
  async getAllAlbumMetadata(): Promise<ApiAlbumMetadata[]> {
    const response = await fetch(`${API_BASE}/albums/metadata`);
    return handleResponse<ApiAlbumMetadata[]>(response);
  },

  /**
   * Gets cached metadata for a specific album.
   * 
   * @param albumKey - Album key in "albumName::artistName" format
   * @returns Promise with album metadata or null if not found
   */
  async getAlbumMetadata(albumKey: string): Promise<ApiAlbumMetadata | null> {
    try {
      const response = await fetch(`${API_BASE}/albums/metadata/${encodeURIComponent(albumKey)}`);
      if (response.status === 404) return null;
      return handleResponse<ApiAlbumMetadata>(response);
    } catch {
      return null;
    }
  },

  /**
   * Saves album metadata to the backend cache.
   * Also marks whether Spotify was checked and if data was found.
   * 
   * @param metadata - Album metadata to save
   */
  async saveAlbumMetadata(metadata: ApiAlbumMetadata): Promise<void> {
    const response = await fetch(`${API_BASE}/albums/metadata`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(metadata),
    });
    await handleResponse(response);
  },

  /**
   * Downloads album artwork to the album's folder as cover.jpg.
   * Uses the first song in the album to determine the folder path.
   * 
   * @param albumKey - Album key in "albumName::artistName" format
   * @param imageUrl - URL of the image to download
   * @returns Promise with the local cover path
   */
  async downloadAlbumCover(albumKey: string, imageUrl: string): Promise<{ coverPath: string }> {
    const response = await fetch(`${API_BASE}/albums/metadata/download-cover`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ albumKey, imageUrl }),
    });
    return handleResponse<{ status: string; coverPath: string }>(response);
  },

  /**
   * Resets the Spotify check status for an album to force re-fetch.
   * Call this when user wants to manually refresh metadata from Spotify.
   * 
   * @param albumKey - Album key in "albumName::artistName" format
   */
  async resetAlbumMetadata(albumKey: string): Promise<void> {
    const response = await fetch(`${API_BASE}/albums/metadata/${encodeURIComponent(albumKey)}`, {
      method: 'DELETE',
    });
    await handleResponse(response);
  },

  /**
   * Gets metadata for multiple albums in a single request.
   * More efficient than calling getAlbumMetadata for each album.
   * 
   * @param albumKeys - Array of album keys in "albumName::artistName" format
   * @returns Promise with array of album metadata
   */
  async batchGetAlbumMetadata(albumKeys: string[]): Promise<ApiAlbumMetadata[]> {
    if (albumKeys.length === 0) return [];
    const response = await fetch(`${API_BASE}/albums/metadata/batch`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ albumKeys }),
    });
    return handleResponse<ApiAlbumMetadata[]>(response);
  },

  /**
   * Gets albums that haven't been checked on Spotify yet.
   * Used for background metadata enrichment.
   * 
   * @returns Promise with albums needing Spotify check
   */
  async getUncheckedAlbumMetadata(): Promise<ApiAlbumMetadata[]> {
    const response = await fetch(`${API_BASE}/albums/metadata/unchecked`);
    return handleResponse<ApiAlbumMetadata[]>(response);
  },

  /**
   * Gets albums that were checked but not found, and are past expiration.
   * These should be re-checked as Spotify catalog changes over time.
   * 
   * @returns Promise with expired album metadata needing re-check
   */
  async getExpiredAlbumMetadata(): Promise<ApiAlbumMetadata[]> {
    const response = await fetch(`${API_BASE}/albums/metadata/expired`);
    return handleResponse<ApiAlbumMetadata[]>(response);
  },

  // Artist metadata endpoints

  /**
   * Gets all cached artist metadata from the backend.
   * 
   * @returns Promise with all cached artist metadata
   */
  async getAllArtistMetadata(): Promise<ApiArtistMetadata[]> {
    const response = await fetch(`${API_BASE}/artists/metadata`);
    return handleResponse<ApiArtistMetadata[]>(response);
  },

  /**
   * Gets cached metadata for a specific artist.
   * 
   * @param artistName - Artist name
   * @returns Promise with artist metadata or null if not found
   */
  async getArtistMetadata(artistName: string): Promise<ApiArtistMetadata | null> {
    try {
      const response = await fetch(`${API_BASE}/artists/metadata/${encodeURIComponent(artistName)}`);
      if (response.status === 404) return null;
      return handleResponse<ApiArtistMetadata>(response);
    } catch {
      return null;
    }
  },

  /**
   * Saves artist metadata to the backend cache.
   * 
   * @param metadata - Artist metadata to save
   */
  async saveArtistMetadata(metadata: ApiArtistMetadata): Promise<void> {
    const response = await fetch(`${API_BASE}/artists/metadata`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(metadata),
    });
    await handleResponse(response);
  },

  /**
   * Downloads artist image to local cache.
   * 
   * @param artistName - Artist name
   * @param imageUrl - URL of the image to download
   * @returns Promise with the local image path
   */
  async downloadArtistImage(artistName: string, imageUrl: string): Promise<{ imagePath: string }> {
    const response = await fetch(`${API_BASE}/artists/metadata/download-image`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ artistName, imageUrl }),
    });
    return handleResponse<{ status: string; imagePath: string }>(response);
  },

  /**
   * Resets the Spotify check status for an artist to force re-fetch.
   * 
   * @param artistName - Artist name
   */
  async resetArtistMetadata(artistName: string): Promise<void> {
    const response = await fetch(`${API_BASE}/artists/metadata/${encodeURIComponent(artistName)}`, {
      method: 'DELETE',
    });
    await handleResponse(response);
  },

  /**
   * Gets artists that haven't been checked on Spotify yet.
   * 
   * @returns Promise with artists needing Spotify check
   */
  async getUncheckedArtistMetadata(): Promise<ApiArtistMetadata[]> {
    const response = await fetch(`${API_BASE}/artists/metadata/unchecked`);
    return handleResponse<ApiArtistMetadata[]>(response);
  },

  // ==================== LLM Settings API ====================

  /**
   * Gets current LLM settings and available providers/models.
   * 
   * @returns Current LLM configuration with provider/model options
   */
  async getLLMSettings(): Promise<LLMSettingsResponse> {
    const response = await fetch(`${API_BASE}/llm/settings`);
    return handleResponse<LLMSettingsResponse>(response);
  },

  /**
   * Updates LLM settings.
   * 
   * @param settings - New LLM configuration
   * @returns Success status
   */
  async updateLLMSettings(settings: LLMSettingsRequest): Promise<{ status: string }> {
    const response = await fetch(`${API_BASE}/llm/settings`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(settings),
    });
    return handleResponse(response);
  },

  /**
   * Gets available LLM providers and models.
   * 
   * @returns List of providers and models
   */
  async getLLMProviders(): Promise<{ providers: LLMProviderInfo[]; models: Record<string, LLMModelInfo[]> }> {
    const response = await fetch(`${API_BASE}/llm/providers`);
    return handleResponse(response);
  },

  /**
   * Tests the connection to the currently configured LLM provider.
   * 
   * @returns Test result with success status and message
   */
  async testLLMConnection(): Promise<LLMTestResponse> {
    const response = await fetch(`${API_BASE}/llm/test`, { method: 'POST' });
    return handleResponse<LLMTestResponse>(response);
  },

  /**
   * Triggers the Gemini genre enrichment process.
   * @param apiKey - Optional Gemini API key. If not provided, backend will use stored key.
   * @param force - If true, re-checks all songs. If false, only checks songs with missing genres.
   * @param offset - Pagination offset for processing large libraries in batches.
   * @returns Result of the enrichment process.
   */
  async enrichGenres(apiKey?: string, force: boolean = false, offset: number = 0): Promise<{ status: string; message: string; count: number }> {
    const response = await fetch(`${API_BASE}/library/enrich-genres`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({ apiKey, force, offset }),
    });
    return handleResponse(response);
  },

  /**
   * Streams genre enrichment progress via SSE.
   * @param force - If true, re-checks all songs.
   * @param onProgress - Callback for progress updates.
   * @returns EventSource instance for cleanup.
   */
  enrichGenresStream(
    force: boolean = false,
    onProgress: (progress: EnrichmentProgress) => void
  ): EventSource {
    const url = `${API_BASE}/library/enrich-genres/stream?force=${force}`;
    const eventSource = new EventSource(url);

    eventSource.onmessage = (event) => {
      try {
        const progress = JSON.parse(event.data) as EnrichmentProgress;
        onProgress(progress);
        
        // Auto-close on completion or error
        if (progress.status === 'complete' || progress.status === 'error') {
          eventSource.close();
        }
      } catch (e) {
        console.error('Failed to parse enrichment progress:', e);
      }
    };

    eventSource.onerror = () => {
      onProgress({
        status: 'error',
        message: 'Connection lost',
        error: 'SSE connection failed',
        totalSongs: 0,
        processedSongs: 0,
        currentBatch: 0,
        totalBatches: 0,
      });
      eventSource.close();
    };

    return eventSource;
  },

  /**
   * Streams unified metadata enrichment progress via SSE.
   * 
   * This is the RECOMMENDED enrichment endpoint - it enriches ALL metadata in a single
   * efficient API call per batch using TOON (Token-Oriented Object Notation) format.
   * 
   * Enriches:
   * - genres: Detailed genre classifications
   * - mood: happy, sad, energetic, calm, melancholic, uplifting, aggressive, romantic, chill, intense, dreamy, nostalgic
   * - energy: high, medium, low
   * - tempo: fast, medium, slow
   * - bpm: Estimated beats per minute (integer)
   * - instrumental: Whether the song has vocals
   * - originalYear: Original release year (for remasters)
   * 
   * Benefits:
   * - 3x more token efficient than JSON-based methods
   * - 200 songs per batch (vs 50 for JSON)
   * - Single API call gets all data (vs 3 separate calls)
   * 
   * @param force - If true, re-enriches songs that already have metadata
   * @param onProgress - Callback for progress updates with batch status
   * @returns EventSource instance (auto-closes on completion/error)
   */
  enrichAllMetadataStream(
    force: boolean = false,
    onProgress: (progress: EnrichmentProgress) => void
  ): EventSource {
    const url = `${API_BASE}/library/enrich-all/stream?force=${force}`;
    const eventSource = new EventSource(url);

    eventSource.onmessage = (event) => {
      try {
        const progress = JSON.parse(event.data) as EnrichmentProgress;
        onProgress(progress);
        
        // Auto-close on completion or error
        if (progress.status === 'complete' || progress.status === 'error') {
          eventSource.close();
        }
      } catch (e) {
        console.error('Failed to parse enrichment progress:', e);
      }
    };

    eventSource.onerror = () => {
      onProgress({
        status: 'error',
        message: 'Connection lost',
        error: 'SSE connection failed',
        totalSongs: 0,
        processedSongs: 0,
        currentBatch: 0,
        totalBatches: 0,
      });
      eventSource.close();
    };

    return eventSource;
  },

  /**
   * Streams mood/energy/tempo/BPM analysis progress via SSE.
   * 
   * Uses Gemini AI to analyze songs based on metadata (artist, title, album, genre).
   * This approach leverages Gemini's knowledge of music styles rather than audio analysis.
   * Results are stored in the songs table for use by the AI DJ.
   * 
   * Analysis values:
   * - mood: happy, sad, energetic, calm, melancholic, uplifting, aggressive, romantic, chill, intense, dreamy, nostalgic
   * - energy: high, medium, low
   * - tempo: fast, medium, slow
   * - bpm: Estimated beats per minute (integer)
   * 
   * @param onProgress - Callback for progress updates with batch status
   * @returns EventSource instance (auto-closes on completion/error)
   */
  enrichMoodStream(
    onProgress: (progress: EnrichmentProgress) => void
  ): EventSource {
    const url = `${API_BASE}/library/enrich-mood/stream`;
    const eventSource = new EventSource(url);

    eventSource.onmessage = (event) => {
      try {
        const progress = JSON.parse(event.data) as EnrichmentProgress;
        onProgress(progress);
        
        // Auto-close on completion or error
        if (progress.status === 'complete' || progress.status === 'error') {
          eventSource.close();
        }
      } catch (e) {
        console.error('Failed to parse mood analysis progress:', e);
      }
    };

    eventSource.onerror = () => {
      onProgress({
        status: 'error',
        message: 'Connection lost',
        error: 'SSE connection failed',
        totalSongs: 0,
        processedSongs: 0,
        currentBatch: 0,
        totalBatches: 0,
      });
      eventSource.close();
    };

    return eventSource;
  },

  /**
   * Backfills missing song year values from album_metadata.release_date.
   * This enables the AI DJ to correctly filter by decade (e.g., "90s hip hop").
   * @returns The count of songs updated.
   */
  async backfillSongYears(): Promise<{ updated: number; message: string }> {
    const response = await fetch(`${API_BASE}/library/backfill-years`, {
      method: 'POST',
    });
    return handleResponse(response);
  },

  /**
   * Detects potential remaster songs using heuristic pattern matching.
   * Scans album/title for patterns like "Remastered", "Deluxe Edition", "Anniversary"
   * and flags songs with year_uncertain for later AI analysis.
   * @returns The count of songs processed and flagged.
   */
  async detectRemasters(): Promise<{ processed: number; flagged: number; message: string }> {
    const response = await fetch(`${API_BASE}/library/detect-remasters`, {
      method: 'POST',
    });
    return handleResponse(response);
  },

  /**
   * Stream original year enrichment using SSE.
   * Uses Gemini AI to determine the original release year of songs that may have remaster dates.
   * @param onProgress Callback for progress updates
   * @param onComplete Callback when enrichment completes
   * @param onError Callback for errors
   * @returns AbortController to cancel the stream
   */
  enrichOriginalYearsStream(
    onProgress: (progress: EnrichmentProgress) => void,
    onComplete: (progress: EnrichmentProgress) => void,
    onError: (error: string) => void
  ): AbortController {
    const controller = new AbortController();
    
    fetch(`${API_BASE}/library/enrich-years/stream`, {
      signal: controller.signal,
    })
      .then(response => {
        if (!response.ok) {
          throw new Error(`HTTP ${response.status}`);
        }
        const reader = response.body?.getReader();
        const decoder = new TextDecoder();
        
        const readStream = async () => {
          if (!reader) return;
          
          let buffer = '';
          while (true) {
            const { done, value } = await reader.read();
            if (done) break;
            
            buffer += decoder.decode(value, { stream: true });
            const lines = buffer.split('\n');
            buffer = lines.pop() || '';
            
            for (const line of lines) {
              if (line.startsWith('data: ')) {
                try {
                  const data = JSON.parse(line.slice(6)) as EnrichmentProgress;
                  if (data.status === 'complete') {
                    onComplete(data);
                  } else if (data.status === 'error') {
                    onError(data.error || data.message);
                  } else {
                    onProgress(data);
                  }
                } catch (e) {
                  console.error('Failed to parse SSE data:', e);
                }
              }
            }
          }
        };
        
        readStream().catch(err => {
          if (err.name !== 'AbortError') {
            onError(err.message);
          }
        });
      })
      .catch(err => {
        if (err.name !== 'AbortError') {
          onError(err.message);
        }
      });
    
    return controller;
  },

  /**
   * Generate a smart playlist using the AI DJ feature.
   * 
   * The backend uses a three-tier matching system:
   * 1. Artist-based matching: For "more like [artist]" prompts
   * 2. Local genre matching: Direct match against indexed genres
   * 3. Gemini AI fallback: For complex prompts requiring AI interpretation
   * 
   * Always uses multi-genre blending to create cross-genre playlists based on user input.
   * 
   * @param prompt - Natural language description of desired playlist
   * @param options.blendMode - 'mixed' for multi-genre blending (always used)
   * @param options.targetSongs - Number of songs to return (default: 50, max: 100)
   * @param options.discoverMode - 'balanced', 'discover' (underplayed), or 'favorites'
   * @param options.avoidRecentlyHours - Exclude songs played within N hours (0 = off)
   * @param options.onePerArtist - Limit to one song per artist for variety
   * @param options.useTimeContext - Add time-of-day context to AI recommendations
   * @param options.mode - 'playlist' (default) or 'dj' for DJ mode
   * @param options.persona - DJ persona key (FlowMaster, CrowdPleaser, etc.)
   * @param options.targetDurationMinutes - DJ set duration in minutes
   * @param options.flowStrictness - BPM continuity strictness 0-100
   * @param options.talkMode - Enable DJ narration cues
   * @returns Playlist filter and matching songs, plus DJ payload when mode='dj'
   */
  async generateSmartPlaylist(
    prompt: string, 
    options?: { 
      blendMode?: 'single' | 'mixed';
      targetSongs?: number;
      discoverMode?: 'balanced' | 'discover' | 'favorites';
      avoidRecentlyHours?: number;
      onePerArtist?: boolean;
      useTimeContext?: boolean;
      // DJ Mode options
      mode?: SmartPlaylistMode;
      persona?: string;
      targetDurationMinutes?: number;
      flowStrictness?: number;
      talkMode?: boolean;
    }
  ): Promise<SmartPlaylistResponse> {
    // Cancel any previous in-flight smart playlist request
    if (this._smartPlaylistAbort) {
      this._smartPlaylistAbort.abort();
    }
    this._smartPlaylistAbort = new AbortController();

    const response = await fetch(`${API_BASE}/smart-playlist`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      signal: this._smartPlaylistAbort.signal,
      body: JSON.stringify({ 
        prompt,
        blendMode: options?.blendMode || 'mixed',
        targetSongs: options?.targetSongs || 50,
        discoverMode: options?.discoverMode || 'balanced',
        avoidRecentlyHours: options?.avoidRecentlyHours || 0,
        onePerArtist: options?.onePerArtist || false,
        useTimeContext: options?.useTimeContext || false,
        // DJ Mode fields
        mode: options?.mode || 'playlist',
        persona: options?.persona || 'FlowMaster',
        targetDurationMinutes: options?.targetDurationMinutes || 45,
        flowStrictness: options?.flowStrictness || 60,
        talkMode: options?.talkMode || false,
      }),
    });
    return handleResponse(response);
  },

  // DJ Mode
  /**
   * Get all available DJ personas.
   * @returns Array of persona definitions with keys, names, descriptions
   */
  async getDJPersonas(): Promise<DJPersonaDefinition[]> {
    const response = await fetch(`${API_BASE}/dj/personas`);
    return handleResponse(response);
  },

  /**
   * Get waveform data for a track.
   * If not cached, the backend will generate it from the audio file.
   * 
   * @param trackId - Song ID
   * @returns Waveform response with peaks array
   */
  async getDJWaveform(trackId: string): Promise<DJWaveformResponse> {
    const response = await fetch(`${API_BASE}/dj/waveform/${trackId}`);
    return handleResponse<DJWaveformResponse>(response);
  },

  /**
   * Get hot cues for a track.
   * 
   * @param trackId - Song ID
   * @returns Hot cues response with array of cue points
   */
  async getDJHotCues(trackId: string): Promise<DJHotCuesResponse> {
    const response = await fetch(`${API_BASE}/dj/hotcues/${trackId}`);
    return handleResponse<DJHotCuesResponse>(response);
  },

  /**
   * Save hot cues for a track.
   * 
   * @param trackId - Song ID
   * @param hotCues - Array of hot cue points
   * @returns Success status
   */
  async saveDJHotCues(trackId: string, hotCues: DJHotCue[]): Promise<{ success: boolean }> {
    const response = await fetch(`${API_BASE}/dj/hotcues/${trackId}`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ hotCues }),
    });
    return handleResponse(response);
  },

  // ==================== Last.FM API ====================

  /**
   * Gets current Last.FM settings.
   * API key is masked for security if configured.
   * 
   * @returns Current Last.FM configuration
   */
  async getLastFMSettings(): Promise<LastFMSettings> {
    const response = await fetch(`${API_BASE}/lastfm/settings`);
    return handleResponse<LastFMSettings>(response);
  },

  /**
   * Saves Last.FM settings.
   * Sensitive fields (API key, shared secret) are encrypted at rest.
   * 
   * @param settings - Last.FM configuration to save
   * @returns Success status
   */
  async saveLastFMSettings(settings: LastFMSettingsRequest): Promise<{ success: boolean }> {
    const response = await fetch(`${API_BASE}/lastfm/settings`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(settings),
    });
    return handleResponse(response);
  },

  /**
   * Tests the Last.FM API connection.
   * Attempts to fetch a known artist to verify credentials.
   * 
   * @returns Test result with success status and message
   */
  async testLastFMConnection(): Promise<{ success: boolean; message: string }> {
    const response = await fetch(`${API_BASE}/lastfm/test`, { method: 'POST' });
    return handleResponse(response);
  },

  /**
   * Authenticates with Last.FM for scrobbling support.
   * Uses mobile authentication flow - session key is stored encrypted.
   * 
   * @param username - Last.FM username
   * @param password - Last.FM password (not stored, only used for session)
   * @returns Authentication result with username
   */
  async authenticateLastFM(username: string, password: string): Promise<{ success: boolean; username: string }> {
    const response = await fetch(`${API_BASE}/lastfm/authenticate`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ username, password }),
    });
    return handleResponse(response);
  },

  /**
   * Gets current Last.FM integration status.
   * Includes connection status, scrobbling capability, and enrichment stats.
   * 
   * @returns Last.FM status with stats
   */
  async getLastFMStatus(): Promise<LastFMStatus> {
    const response = await fetch(`${API_BASE}/lastfm/status`);
    return handleResponse<LastFMStatus>(response);
  },

  /**
   * Triggers Last.FM song enrichment.
   * Fetches track info and tags from Last.FM for songs without enrichment data.
   * 
   * @param options - Enrichment options
   * @returns Enrichment start result with queued count
   */
  async triggerLastFMEnrichment(options?: {
    limit?: number;
    force?: boolean;
    fetchSimilar?: boolean;
  }): Promise<{ message: string; queued: number; inFlight: boolean }> {
    const response = await fetch(`${API_BASE}/lastfm/enrich/songs`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(options || {}),
    });
    return handleResponse(response);
  },

  /**
   * Triggers Last.FM artist enrichment.
   * Fetches artist info and similar artists from Last.FM.
   * 
   * @param options - Enrichment options
   * @returns Enrichment start result with queued count
   */
  async triggerLastFMArtistEnrichment(options?: {
    limit?: number;
    fetchSimilar?: boolean;
  }): Promise<{ message: string; queued: number; inFlight: boolean }> {
    const response = await fetch(`${API_BASE}/lastfm/enrich/artists`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(options || {}),
    });
    return handleResponse(response);
  },

  /**
   * Gets track info from Last.FM.
   * Fetches metadata, tags, and popularity data for a specific track.
   * 
   * @param artist - Artist name
   * @param track - Track title
   * @returns Track info from Last.FM
   */
  async getLastFMTrackInfo(artist: string, track: string): Promise<LastFMTrackInfo> {
    const params = new URLSearchParams({ artist, track });
    const response = await fetch(`${API_BASE}/lastfm/track?${params}`);
    return handleResponse<LastFMTrackInfo>(response);
  },

  /**
   * Gets similar tracks from Last.FM.
   * Returns tracks similar to the given track with match scores.
   * 
   * @param artist - Artist name
   * @param track - Track title
   * @param limit - Maximum number of results (default 20)
   * @returns Array of similar tracks
   */
  async getLastFMSimilarTracks(artist: string, track: string, limit?: number): Promise<LastFMSimilarTrack[]> {
    const params = new URLSearchParams({ artist, track });
    if (limit) params.append('limit', limit.toString());
    const response = await fetch(`${API_BASE}/lastfm/similar?${params}`);
    return handleResponse<LastFMSimilarTrack[]>(response);
  },
};

/**
 * Matched genre from AI DJ with scoring information.
 */
export interface MatchedGenre {
  name: string;
  score: number;
  songCount: number;
  proportion: number; // 0.0-1.0
}

/**
 * Smart playlist filter returned by AI DJ.
 */
export interface SmartPlaylistFilter {
  genres: string[];
  artists?: string[];
  minYear?: number;
  maxYear?: number;
  description?: string;
  mood?: string;
  energy?: string;
  tempo?: string;
  occasion?: string;
  instrumental?: boolean;
  fromCache?: boolean;
  localMatch?: boolean;
  blendMode?: 'single' | 'mixed';
  matchedGenres?: MatchedGenre[];
}

// ==================== DJ Mode Types ====================

/**
 * Mode for smart playlist generation.
 */
export type SmartPlaylistMode = 'playlist' | 'dj';

/**
 * Persona keys for DJ mode.
 * Each persona applies different scoring weights.
 */
export type DJPersona = 
  | 'FlowMaster'    // Default: strong continuity, balanced novelty
  | 'CrowdPleaser'  // Favors high completion, favorites, familiar songs
  | 'DeepCutDJ'     // Heavy underplayed boost, novelty, low favorites bias
  | 'Explorer'      // Controlled novelty, medium continuity
  | 'Curator'       // Strict genre purity, one-per-artist
  | 'NightDrive';   // Smoother tempos, medium energy, nostalgic

/**
 * Persona definition with weights and description.
 */
export interface DJPersonaDefinition {
  key: DJPersona;
  name: string;
  description: string;
}

// ==================== DJ Mixer Waveform Types ====================

/**
 * Waveform data response from backend.
 * Used for visual waveform display in DJ mode.
 */
export interface DJWaveformResponse {
  trackId: string;
  duration: number;      // Track duration in seconds
  sampleRate: number;    // Source audio sample rate
  resolution: number;    // Samples per peak point
  peaks: number[];       // Normalized peak values (0-1)
}

/**
 * Hot cue point for DJ mode.
 * Allows instant jump to marked positions.
 */
export interface DJHotCue {
  slot: number;          // 1-8
  position: number;      // Position in seconds
  label?: string;        // Optional user label
  color: string;         // Hex color (e.g., "#ef4444")
}

/**
 * Hot cues response from backend.
 */
export interface DJHotCuesResponse {
  trackId: string;
  hotCues: DJHotCue[];
}

// ==================== DJ Set Planning Types ====================

/**
 * A single phase in a DJ set plan.
 */
export interface DJPhase {
  name: string;         // "Warm-up", "Build", "Peak", "Cooldown", "Afterhours"
  targetEnergy: string; // "low", "medium", "high"
  targetTempo: string;  // "slow", "medium", "fast"
  targetMoods: string[];
  targetCount: number;
  minBPM: number;
  maxBPM: number;
  notes: string;
}

/**
 * Complete DJ set plan generated by LLM.
 */
export interface DJSetPlan {
  intentSummary: string;
  targetDurationMin: number;
  persona: string;
  flowStrictness: number;
  phases: DJPhase[];
  seedGenres: string[];
  seedArtists: string[];
  createdAtUnix: number;
  fromCache: boolean;
}

/**
 * Result of song selection for a specific phase.
 */
export interface DJPhaseResult {
  name: string;
  songIds: string[];
  avgBpm: number;
  minBpm: number;
  maxBpm: number;
  notes: string;
  songCount: number;
}

/**
 * Optional DJ narration cues for talk mode.
 */
export interface DJNarration {
  intro: string;
  phaseIntros: string[];
  outro: string;
}

/**
 * DJ mode response payload.
 */
export interface DJModeResponse {
  plan: DJSetPlan;
  phases: DJPhaseResult[];
  narration?: DJNarration;
}

/**
 * Response from the smart playlist API.
 */
export interface SmartPlaylistResponse {
  filter: SmartPlaylistFilter;
  songs: ApiSong[];
  dj?: DJModeResponse; // Present when mode === 'dj'
}

/**
 * Cached album metadata from the backend.
 * Used to persist Spotify lookup results and prevent redundant API calls.
 */
export interface ApiAlbumMetadata {
  albumKey: string;           // "{album}::{artist}" format
  albumName: string;
  artistName: string;
  spotifyId?: string;
  coverUrl?: string;          // Spotify artwork URL
  localCoverPath?: string;    // Local path to downloaded cover.jpg
  description?: string;
  genre?: string;
  releaseDate?: string;
  spotifyUrl?: string;
  copyright?: string;
  spotifyChecked: boolean;    // True if we've queried Spotify (even if not found)
  spotifyFound: boolean;      // True if Spotify returned results
  fetchedAt?: number;
  updatedAt?: number;
  liked?: boolean;            // True if user has liked this album
  likedAt?: number;           // Timestamp when album was liked
}

/**
 * Cached artist metadata from the backend.
 */
export interface ApiArtistMetadata {
  artistName: string;
  spotifyId?: string;
  imageUrl?: string;          // Spotify image URL
  localImagePath?: string;    // Local path to cached image
  spotifyUrl?: string;
  spotifyChecked: boolean;
  spotifyFound: boolean;
  fetchedAt?: number;
  updatedAt?: number;
}

// ==================== LLM Settings Types ====================

/**
 * Information about an available LLM provider.
 */
export interface LLMProviderInfo {
  id: string;           // e.g., "ollama", "gemini", "openai"
  name: string;         // e.g., "Ollama (Local)"
  requiresKey: boolean; // Whether an API key is required
  defaultModel: string; // Default model for this provider
  description?: string;  // Brief description of the provider
  freeformModel: boolean; // Whether user can type any model name (true for Ollama)
}

/**
 * Information about an available LLM model.
 */
export interface LLMModelInfo {
  id: string;       // e.g., "llama3.2:8b", "gpt-4o"
  name: string;     // e.g., "Llama 3.2 8B"
  description?: string; // Brief description
}

/**
 * LLM settings response from backend.
 */
export interface LLMSettingsResponse {
  provider: string;
  model: string;
  apiKey?: string;  // Masked for security (e.g., "****abc1")
  baseURL?: string;
  providers: LLMProviderInfo[];
  models: Record<string, LLMModelInfo[]>;
}

/**
 * LLM settings request for updating configuration.
 */
export interface LLMSettingsRequest {
  provider: string;
  model: string;
  apiKey?: string;
  baseURL?: string;
}

/**
 * LLM connection test response.
 */
export interface LLMTestResponse {
  success: boolean;
  message: string;
}

// ==================== Last.FM Types ====================

/**
 * Enrichment source type - controls which system is used for metadata enrichment.
 * - "ai": Use configured LLM provider (Gemini, OpenAI, etc.)
 * - "lastfm": Use Last.FM community metadata
 * - "hybrid": Use Last.FM first, fall back to AI for missing data
 */
export type EnrichmentSource = 'ai' | 'lastfm' | 'hybrid';

/**
 * Last.FM settings configuration.
 */
export interface LastFMSettings {
  apiKey: string;       // Masked for security (e.g., "abcd...wxyz")
  sharedSecret?: string;
  sessionKey?: string;
  username?: string;
  enabled: boolean;
  enrichmentSource?: EnrichmentSource;
}

/**
 * Last.FM settings request for saving configuration.
 */
export interface LastFMSettingsRequest {
  apiKey?: string;
  sharedSecret?: string;
  username?: string;
  password?: string;    // Only used for authentication, not stored
  enabled?: boolean;
  enrichmentSource?: EnrichmentSource;
}

/**
 * Last.FM integration status.
 */
export interface LastFMStatus {
  configured: boolean;
  connected: boolean;
  canScrobble: boolean;
  username?: string;
  stats?: {
    totalSongs?: number;
    enrichedSongs?: number;
    songsWithTags?: number;
    songsWithSimilar?: number;
  };
  lastError?: string;
  lastSyncTime?: number;
}

/**
 * Last.FM tag with usage count.
 */
export interface LastFMTag {
  name: string;
  count?: number;
  url?: string;
}

/**
 * Last.FM track info.
 */
export interface LastFMTrackInfo {
  name: string;
  artist: string;
  album?: string;
  duration: number;
  listeners: number;
  playcount: number;
  mbid?: string;
  url: string;
  topTags: LastFMTag[];
  wiki?: string;
  corrected: boolean;
  fetchedAt: string;
}

/**
 * Similar track from Last.FM.
 */
export interface LastFMSimilarTrack {
  name: string;
  artist: string;
  match: number;    // 0-1, higher = more similar
  mbid?: string;
  url: string;
}

export default api;
