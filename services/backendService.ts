/**
 * ViiB MediaHub - Backend Service
 * 
 * Unified interface for communicating with the Go backend API.
 * Provides abstraction layer that works in both backend and browser-only modes.
 * 
 * Modes:
 * - Backend Mode: Routes all operations through Go API endpoints
 * - Browser-Only Mode: Falls back to IndexedDB via libraryService
 * 
 * Features:
 * - Song and playlist CRUD operations
 * - Folder scanning and library management
 * - Automatic backend availability detection
 * - Type conversion between API and frontend formats
 * 
 * @module backendService
 */

// Backend service - communicates with Go backend API
// This provides a unified interface that works with both:
// - Browser-only mode (using IndexedDB directly)
// - Backend mode (using Go API endpoints)

import { api, ApiSong, ApiPlaylist, ScanFolder } from './api';
import { Song, Playlist } from '../types';

// Check if backend is available (re-checks every 30 seconds if initially unavailable)
let backendAvailable: boolean | null = null;
let backendCheckedAt = 0;
const BACKEND_CHECK_TTL_MS = 30_000;

export async function isBackendAvailable(): Promise<boolean> {
  // Return cached result if available and not stale (or if backend was found available)
  if (backendAvailable !== null) {
    // If backend is available, trust the cached result indefinitely
    if (backendAvailable) return true;
    // If backend was unavailable, re-check after TTL expires
    if (Date.now() - backendCheckedAt < BACKEND_CHECK_TTL_MS) return false;
  }
  
  try {
    await api.healthCheck();
    backendAvailable = true;
    backendCheckedAt = Date.now();
    console.log('✅ Go backend connected');
  } catch {
    backendAvailable = false;
    backendCheckedAt = Date.now();
    console.log('ℹ️ Running in browser-only mode');
  }
  
  return backendAvailable;
}

// Convert API song format to frontend Song format
export function apiSongToSong(apiSong: ApiSong): Song {
  return {
    id: apiSong.id,
    title: apiSong.title,
    artist: apiSong.artist,
    album: apiSong.album,
    albumArtist: apiSong.albumArtist,
    trackNumber: apiSong.trackNumber,
    discNumber: apiSong.discNumber,
    genre: apiSong.genre,
    year: apiSong.year,
    originalYear: apiSong.originalYear,
    yearUncertain: apiSong.yearUncertain,
    yearAnalyzedAt: apiSong.yearAnalyzedAt,
    duration: apiSong.duration,
    url: apiSong.filePath, // API URL like /api/audio/{id}
    coverUrl: apiSong.coverPath, // API URL like /api/cover/{id}
    addedAt: apiSong.addedAt,
    playCount: apiSong.playCount,
    lastPlayed: apiSong.lastPlayed,
    skipCount: apiSong.skipCount,
    fileHash: apiSong.fileHash,
    mood: apiSong.mood,
    energy: apiSong.energy,
    tempo: apiSong.tempo,
    bpm: apiSong.bpm,
    instrumental: apiSong.instrumental,
    moodAnalyzedAt: apiSong.moodAnalyzedAt,
    liked: apiSong.liked,
    likedAt: apiSong.likedAt,
  };
}

function apiPlaylistToPlaylist(apiPlaylist: ApiPlaylist): Playlist {
  return {
    id: apiPlaylist.id,
    name: apiPlaylist.name,
    songIds: apiPlaylist.songIds,
    coverUrl: apiPlaylist.coverPath,
    createdAt: apiPlaylist.createdAt,
  };
}

// Backend service with fallback to browser storage
export const backendService = {
  // Check backend availability on init
  async init(): Promise<boolean> {
    return isBackendAvailable();
  },

  // Songs
  async getAllSongs(): Promise<Song[]> {
    if (await isBackendAvailable()) {
      const apiSongs = await api.getSongs();
      return apiSongs.map(apiSongToSong);
    }
    // Fallback handled by libraryService
    return [];
  },

  async clearSongs(): Promise<void> {
    if (await isBackendAvailable()) {
      await api.clearSongs();
    }
  },

  async recordPlay(songId: string): Promise<void> {
    if (await isBackendAvailable()) {
      await api.recordPlay(songId);
    }
  },

  /**
   * Record a listening event for AI DJ preference learning.
   * Called when a song ends or is skipped to track user behavior.
   * @param songId - The ID of the song
   * @param playDuration - How many seconds the song was played
   * @param songDuration - Total duration of the song
   * @param context - Playback context: 'ai_dj', 'album', 'playlist', 'queue', 'search'
   */
  async recordListenEvent(
    songId: string,
    playDuration: number,
    songDuration: number,
    context: string
  ): Promise<void> {
    if (await isBackendAvailable()) {
      try {
        await api.recordListenEvent(songId, playDuration, songDuration, context);
      } catch (e) {
        console.error('Failed to record listen event:', e);
      }
    }
  },

  // Playlists
  async getAllPlaylists(): Promise<Playlist[]> {
    if (await isBackendAvailable()) {
      const apiPlaylists = await api.getPlaylists();
      return apiPlaylists.map(apiPlaylistToPlaylist);
    }
    return [];
  },

  async createPlaylist(name: string, songIds: string[] = []): Promise<Playlist> {
    if (await isBackendAvailable()) {
      const apiPlaylist = await api.createPlaylist(name, songIds);
      return apiPlaylistToPlaylist(apiPlaylist);
    }
    throw new Error('Backend not available');
  },

  async updatePlaylist(playlist: Playlist): Promise<void> {
    if (await isBackendAvailable()) {
      await api.updatePlaylist(playlist.id, {
        name: playlist.name,
        songIds: playlist.songIds,
        createdAt: playlist.createdAt,
      });
    }
  },

  async deletePlaylist(id: string): Promise<void> {
    if (await isBackendAvailable()) {
      await api.deletePlaylist(id);
    }
  },

  // Folder management (backend only)
  async getFolders(): Promise<ScanFolder[]> {
    if (await isBackendAvailable()) {
      return api.getFolders();
    }
    return [];
  },

  async addFolder(path: string): Promise<ScanFolder | null> {
    if (await isBackendAvailable()) {
      return api.addFolder(path);
    }
    return null;
  },

  async removeFolder(id: string): Promise<void> {
    if (await isBackendAvailable()) {
      await api.removeFolder(id);
    }
  },

  // Scanning
  async startScan(): Promise<void> {
    if (await isBackendAvailable()) {
      await api.startScan();
    }
  },

  async startQuickScan(): Promise<void> {
    if (await isBackendAvailable()) {
      await api.startQuickScan();
    }
  },

  async getScanStatus(): Promise<{ scanning: boolean; progress: string }> {
    if (await isBackendAvailable()) {
      return api.getScanStatus();
    }
    return { scanning: false, progress: '' };
  },

  // Browse folders (backend only)
  async browseFolder(path?: string) {
    if (await isBackendAvailable()) {
      return api.browseFolder(path);
    }
    return { currentPath: '', entries: [] };
  },

  // Get URLs
  getAudioUrl(songId: string): string {
    return api.getAudioUrl(songId);
  },

  getCoverUrl(songId: string): string {
    return api.getCoverUrl(songId);
  },
};

export default backendService;
