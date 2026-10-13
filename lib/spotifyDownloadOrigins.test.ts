import { describe, expect, it } from 'vitest';
import { savedLibraryDownloadOrigins, savedLibraryDownloadState } from './spotifyDownloadOrigins';

describe('saved library request origins', () => {
 it('keeps explicit navigation origin only for the same entity and session', () => {
  const state = savedLibraryDownloadState('saved_playlists', 'PPPPPPPPPPPPPPPPPPPPPP', 4);
  expect(savedLibraryDownloadOrigins(state, 'PPPPPPPPPPPPPPPPPPPPPP', 4)).toEqual([{ kind: 'library', id: 'saved_playlists', entityId: 'PPPPPPPPPPPPPPPPPPPPPP', position: -1 }]);
  expect(savedLibraryDownloadOrigins(state, 'another', 4)).toBeUndefined();
  expect(savedLibraryDownloadOrigins(state, 'PPPPPPPPPPPPPPPPPPPPPP', 5)).toBeUndefined();
  expect(savedLibraryDownloadOrigins(savedLibraryDownloadState('saved_albums', 'invalid', 4), 'invalid', 4)).toBeUndefined();
  expect(savedLibraryDownloadOrigins(null, 'PPPPPPPPPPPPPPPPPPPPPP', 4)).toBeUndefined();
  expect(savedLibraryDownloadOrigins({ downloadLibraryOrigin: { kind: 'account', entityId: 'PPPPPPPPPPPPPPPPPPPPPP', sessionGeneration: 4 } }, 'PPPPPPPPPPPPPPPPPPPPPP', 4)).toBeUndefined();
 });
});
