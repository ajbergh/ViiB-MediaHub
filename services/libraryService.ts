
import { getDB, closeDB } from './db';
import { deleteDB } from 'idb';
import { Song, Playlist } from '../types';

export const libraryService = {
  async getAllSongs(): Promise<Song[]> {
    const db = await getDB();
    return db.getAll('songs');
  },

  async saveSongs(songs: Song[]): Promise<void> {
    const db = await getDB();
    const tx = db.transaction('songs', 'readwrite');
    await Promise.all(songs.map(song => tx.store.put(song)));
    await tx.done;
  },

  async clearSongs(): Promise<void> {
    const db = await getDB();
    await db.clear('songs');
  },

  async getAllPlaylists(): Promise<Playlist[]> {
    const db = await getDB();
    return db.getAll('playlists');
  },

  async savePlaylist(playlist: Playlist): Promise<void> {
    const db = await getDB();
    await db.put('playlists', playlist);
  },

  async deletePlaylist(id: string): Promise<void> {
    const db = await getDB();
    await db.delete('playlists', id);
  },

  async resetDB(): Promise<void> {
      try {
        // Attempt 1: Try to clear stores gracefully while connection is open
        const db = await getDB();
        const tx = db.transaction(['songs', 'playlists'], 'readwrite');
        await Promise.all([
          tx.objectStore('songs').clear(),
          tx.objectStore('playlists').clear()
        ]);
        await tx.done;
      } catch (e) {
        console.warn("Error clearing stores, attempting full DB deletion fallback:", e);
        try {
            // Attempt 2: Nuclear option - Close and Delete
            await closeDB();
            await deleteDB('mediahub-db', {
                blocked() {
                    console.warn("Delete blocked by open connection");
                }
            });
        } catch (err) {
            console.error("Critical failure resetting DB:", err);
            throw err;
        }
      }
  },

  // Helper to ensure we can read the file handle
  async verifyPermission(fileHandle: FileSystemFileHandle, readWrite = false): Promise<boolean> {
    if (!fileHandle) return false;
    
    // Cast to any because FileSystemHandlePermissionDescriptor and methods
    // might not be in the current TS lib version
    const options: any = {
        mode: readWrite ? 'readwrite' : 'read'
    };

    const handle = fileHandle as any;

    if (handle.queryPermission) {
        if ((await handle.queryPermission(options)) === 'granted') {
            return true;
        }
    }

    if (handle.requestPermission) {
        if ((await handle.requestPermission(options)) === 'granted') {
            return true;
        }
    }

    return false;
  }
};
