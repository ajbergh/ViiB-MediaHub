/** Provides typed recording-link and optional reference-cache APIs with refresh session checks. */

import { useStore } from '../store';
export type SpotifyReferenceEndpoint = 'audio_features' | 'audio_analysis';
export interface SpotifyRecordingLink {
  songId: string; sourceFingerprint: string;
  link: { externalId: string; sourceFingerprint: string; linkOrigin: string } | null;
}
export interface SpotifyReferenceFailure { code: string; checkedAt: string; retryAt: string }
export interface SpotifyReferenceObservation {
  source: string; sourceEndpoint: SpotifyReferenceEndpoint; trackId: string; retrievedAt: string;
  bpm: number | null; bpmConfidence: number | null; key: number | null; mode: number | null;
  keyConfidence: number | null; camelot: string | null;
}
export interface SpotifyReferenceCache {
  observation: SpotifyReferenceObservation; adapterRevision: string; expiresAt: string;
}
export interface SpotifyReferenceView {
  state: string; endpoint: SpotifyReferenceEndpoint; stale: boolean; ageSeconds: number | null;
  cache: SpotifyReferenceCache | null; lastFailure: SpotifyReferenceFailure | null;
}
async function read<T>(response: Response): Promise<T> {
  if (!response.ok) throw new Error('Spotify reference could not be loaded.');
  return response.json() as Promise<T>;
}
const recordingPath = (id: string) => '/api/v2/analysis/' + encodeURIComponent(id) + '/external/spotify';
const referencePath = (id: string, endpoint: SpotifyReferenceEndpoint) =>
  '/api/spotify/analysis/' + encodeURIComponent(id) + '?endpoint=' + endpoint;
export const spotifyReference = {
  link: async (id: string): Promise<SpotifyRecordingLink> => read(await fetch(recordingPath(id), { cache: 'no-store' })),
  confirm: async (songId: string, trackId: string, sourceFingerprint: string): Promise<void> => {
    await read(await fetch(recordingPath(songId), { method: 'PUT', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ trackId, sourceFingerprint, confirmed: true }) }));
  },
  unlink: async (id: string): Promise<void> => {
    const response = await fetch(recordingPath(id), { method: 'DELETE' });
    if (!response.ok) throw new Error('Recording link could not be removed.');
  },
  cache: async (id: string, endpoint: SpotifyReferenceEndpoint): Promise<SpotifyReferenceView> =>
    read(await fetch(referencePath(id, endpoint), { cache: 'no-store' })),
  refresh: async (id: string, endpoint: SpotifyReferenceEndpoint): Promise<{ cache: SpotifyReferenceCache | null; failure: SpotifyReferenceFailure | null }> => {
    const generation = useStore.getState().spotifySessionGeneration;
    const response = await fetch('/api/spotify/analysis/' + encodeURIComponent(id) + '/refresh?endpoint=' + endpoint, { method: 'POST' });
    const result = await response.json();
if (useStore.getState().spotifySessionGeneration !== generation) throw new Error('Spotify session changed.');
if (response.status === 401) useStore.getState().markSpotifyAuthRequired();
    if (!response.ok && !result.failure) throw new Error('Spotify reference is temporarily unavailable.');
    return result;
  },
};
export function spotifyRecordingId(value: string): string | null {
  const trimmed = value.trim();
  if (/^[A-Za-z0-9]{22}$/.test(trimmed)) return trimmed;
  try {
    const url = new URL(trimmed);
    const match = /^\/track\/([A-Za-z0-9]{22})\/?$/.exec(url.pathname);
    return url.protocol === 'https:' && url.hostname === 'open.spotify.com' && match ? match[1] : null;
  } catch { return null; }
}
