import { ApiSong } from './api';
import { apiSongToSong } from './backendService';
import { Playlist, Song } from '../types';

const API_V2_BASE = '/api/v2';
const REQUEST_TIMEOUT_MS = 15_000;
export interface LibrarySnapshotPage { revision: number; songs: ApiSong[]; nextCursor?: string; hasMore: boolean; }
export interface LibraryChange { revision: number; songId: string; operation: 'upsert' | 'delete'; changedAt: number; }
export interface LibraryChangePage { fromRevision: number; toRevision: number; changes: LibraryChange[]; songs: ApiSong[]; hasMore: boolean; }
export interface LibrarySearchAlbum { name: string; artist: string; songCount: number; coverPath?: string; }
export interface LibrarySearchArtist { name: string; songCount: number; albumCount: number; }
export interface LibrarySearchPlaylist { id: string; name: string; songCount: number; }
export interface LibrarySearchResult { query: string; tracks: ApiSong[]; albums: LibrarySearchAlbum[]; artists: LibrarySearchArtist[]; playlists: LibrarySearchPlaylist[]; }
export interface ClientLibrarySearchResult { query: string; tracks: Song[]; albums: LibrarySearchAlbum[]; artists: LibrarySearchArtist[]; playlists: LibrarySearchPlaylist[]; }
function createTimeoutSignal(parent?: AbortSignal, timeoutMs = REQUEST_TIMEOUT_MS): { signal: AbortSignal; cleanup: () => void } { const controller = new AbortController(); const timer = window.setTimeout(() => controller.abort(new DOMException('Request timed out', 'TimeoutError')), timeoutMs); const abortFromParent = () => controller.abort(parent?.reason); parent?.addEventListener('abort', abortFromParent, { once: true }); return { signal: controller.signal, cleanup: () => { window.clearTimeout(timer); parent?.removeEventListener('abort', abortFromParent); } }; }
async function getJSON<T>(url: string, parentSignal?: AbortSignal): Promise<T> { const { signal, cleanup } = createTimeoutSignal(parentSignal); try { const response = await fetch(url, { signal, headers: { Accept: 'application/json' } }); if (!response.ok) { const payload = await response.json().catch(() => ({ error: `HTTP ${response.status}` })); throw new Error(payload.error || `HTTP ${response.status}`); } return response.json() as Promise<T>; } finally { cleanup(); } }
export const libraryV2 = {
  async getSnapshotPage(cursor = '', limit = 500, signal?: AbortSignal): Promise<LibrarySnapshotPage> { const params = new URLSearchParams({ limit: String(limit) }); if (cursor) params.set('cursor', cursor); return getJSON(`${API_V2_BASE}/library/snapshot?${params.toString()}`, signal); },
  async getSnapshot(signal?: AbortSignal): Promise<{ revision: number; songs: Song[] }> { const songs: Song[] = []; let cursor = ''; let firstRevision: number | null = null; do { const page = await this.getSnapshotPage(cursor, 1000, signal); if (firstRevision === null) firstRevision = page.revision; songs.push(...page.songs.map(apiSongToSong)); cursor = page.hasMore ? page.nextCursor || '' : ''; } while (cursor); return { revision: firstRevision ?? 0, songs }; },
  async getChanges(since: number, signal?: AbortSignal): Promise<{ revision: number; changes: LibraryChange[]; songs: Song[] }> { const changes: LibraryChange[] = []; const songsById = new Map<string, Song>(); let cursorRevision = since; let hasMore = false; do { const page = await getJSON<LibraryChangePage>(`${API_V2_BASE}/library/changes?since=${cursorRevision}&limit=1000`, signal); changes.push(...page.changes); page.songs.map(apiSongToSong).forEach(song => songsById.set(song.id, song)); cursorRevision = page.toRevision; hasMore = page.hasMore; } while (hasMore); return { revision: cursorRevision, changes, songs: Array.from(songsById.values()) }; },
  async search(query: string, limit = 50, signal?: AbortSignal): Promise<ClientLibrarySearchResult> { const params = new URLSearchParams({ q: query, limit: String(limit) }); const result = await getJSON<LibrarySearchResult>(`${API_V2_BASE}/search?${params.toString()}`, signal); return { ...result, tracks: result.tracks.map(apiSongToSong) }; },
  eventURL(since: number): string { return `${API_V2_BASE}/library/events?since=${Math.max(0, since)}`; },
};
export function searchResultPlaylistToPlaylist(result: LibrarySearchPlaylist): Playlist { return { id: result.id, name: result.name, songIds: [], createdAt: 0 }; }
