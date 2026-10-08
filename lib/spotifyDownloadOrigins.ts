import type { SpotifyDownloadOrigin } from '../services/api';

export function savedLibraryDownloadState(kind: 'saved_albums' | 'saved_playlists', entityId: string, sessionGeneration: number) {
  return { downloadLibraryOrigin: { kind, entityId, sessionGeneration } };
}

// Navigation conveys the user's request context, not current provider membership.
export function savedLibraryDownloadOrigins(state: unknown, entityId: string | undefined, sessionGeneration: number): SpotifyDownloadOrigin[] | undefined {
  const origin = (state as ReturnType<typeof savedLibraryDownloadState> | null)?.downloadLibraryOrigin;
  if (!origin || origin.entityId !== entityId || origin.sessionGeneration !== sessionGeneration ||
      (!entityId || !/^[A-Za-z0-9]{22}$/.test(entityId)) ||
      (origin.kind !== 'saved_albums' && origin.kind !== 'saved_playlists')) return undefined;
  return [{ kind: 'library', id: origin.kind, entityId, position: -1 }];
}
