// API client for communicating with Go backend
// In development, Vite proxies /api to the backend
// In production, both are served from the same origin

const API_BASE = '/api';

interface ApiError {
  error: string;
}

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
  duration: number;
  filePath: string; // This will be the API URL like /api/audio/{id}
  coverPath?: string; // This will be /api/cover/{id}
  addedAt: number;
  playCount?: number;
  lastPlayed?: number;
  skipCount?: number;
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

// API Functions

export const api = {
  // Songs
  async getSongs(): Promise<ApiSong[]> {
    const response = await fetch(`${API_BASE}/songs`);
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

  // Scanning
  async startScan(): Promise<void> {
    const response = await fetch(`${API_BASE}/scan`, { method: 'POST' });
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
};

export default api;
