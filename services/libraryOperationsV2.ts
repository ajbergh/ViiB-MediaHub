/**
 * Client for local database maintenance. Backup, diagnostics, repair, and
 * restore preview/staging use longer timeouts; activation remains offline.
 */
import { requestJSON } from './httpClient';

const BASE = '/api/v2/operations';

export interface BackupInfo { name: string; size: number; createdAt: number }
export interface WatcherStatus { running: boolean; intervalMs: number; lastCheckAt?: number; lastChanges: number; lastError?: string; checks: number }
export interface MissingMedia { songId: string; title: string; filePath: string; reason: string }
export interface BrokenPlaylistReference { playlistId: string; playlistName: string; songId: string }
export interface ScannerFailure { filePath: string; failureKind: string; message: string; attempts: number; firstSeenAt: number; lastSeenAt: number; retryAfter: number }
export interface LibraryDiagnostics {
  checkedAt: number;
  integrity: string;
  songCount: number;
  searchIndexCount: number;
  revision: number;
  retainedChanges: number;
  missingMedia: MissingMedia[];
  brokenPlaylistReferences: BrokenPlaylistReference[];
  scannerFailures: ScannerFailure[];
}

export const libraryOperationsV2 = {
  diagnostics(signal?: AbortSignal): Promise<LibraryDiagnostics> {
    return requestJSON(`${BASE}/diagnostics`, { signal, timeoutMs: 120_000 });
  },
  repair(removeMissing: boolean, signal?: AbortSignal): Promise<Record<string, number>> {
    return requestJSON(`${BASE}/repair`, { method: 'POST', signal, retry: false, timeoutMs: 120_000, headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ removeMissing }) });
  },
  listBackups(signal?: AbortSignal): Promise<{ backups: BackupInfo[] }> {
    return requestJSON(`${BASE}/backups`, { signal });
  },
  createBackup(name = '', signal?: AbortSignal): Promise<BackupInfo> {
    return requestJSON(`${BASE}/backups`, { method: 'POST', signal, retry: false, timeoutMs: 120_000, headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ name }) });
  },
  previewRestore(name: string, signal?: AbortSignal): Promise<{ valid: boolean; restartRequired: boolean; manifest: Record<string, unknown> }> {
    return requestJSON(`${BASE}/restore/preview`, { method: 'POST', signal, retry: false, timeoutMs: 120_000, headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ name }) });
  },
  stageRestore(name: string, signal?: AbortSignal): Promise<{ staged: boolean; restartRequired: boolean; path: string }> {
    return requestJSON(`${BASE}/restore/stage`, { method: 'POST', signal, retry: false, timeoutMs: 120_000, headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ name }) });
  },
  watcherStatus(signal?: AbortSignal): Promise<WatcherStatus> {
    return requestJSON(`${BASE}/watcher`, { signal });
  },
  startWatcher(intervalMs: number, signal?: AbortSignal): Promise<WatcherStatus> {
    return requestJSON(`${BASE}/watcher/start`, { method: 'POST', signal, retry: false, headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ intervalMs }) });
  },
  stopWatcher(signal?: AbortSignal): Promise<WatcherStatus> {
    return requestJSON(`${BASE}/watcher/stop`, { method: 'POST', signal, retry: false });
  },
  updateSongMetadata(id: string, patch: Record<string, unknown>, signal?: AbortSignal): Promise<unknown> {
    return requestJSON(`${BASE}/songs/${encodeURIComponent(id)}/metadata`, {
      method: 'PATCH',
      signal,
      retry: false,
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(patch),
    });
  },
};
