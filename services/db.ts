import { openDB, DBSchema, IDBPDatabase } from 'idb';
import { Song, Playlist } from '../types';

interface MediaHubDB extends DBSchema {
  songs: {
    key: string;
    value: Song;
    indexes: { 'by-album': string; 'by-artist': string };
  };
  playlists: {
    key: string;
    value: Playlist;
  };
}

let dbPromise: Promise<IDBPDatabase<MediaHubDB>> | null = null;

export const initDB = () => {
  if (!dbPromise) {
    dbPromise = openDB<MediaHubDB>('mediahub-db', 1, {
      upgrade(db) {
        // Songs Store
        const songStore = db.createObjectStore('songs', { keyPath: 'id' });
        songStore.createIndex('by-album', 'album');
        songStore.createIndex('by-artist', 'artist');

        // Playlists Store
        db.createObjectStore('playlists', { keyPath: 'id' });
      },
    });
  }
  return dbPromise;
};

export const getDB = () => {
    if (!dbPromise) return initDB();
    return dbPromise;
};

export const closeDB = async () => {
    if (dbPromise) {
        const db = await dbPromise;
        db.close();
        dbPromise = null;
    }
};