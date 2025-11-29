/**
 * ViiB MediaHub - IndexedDB Database Service
 * 
 * Provides IndexedDB initialization and connection management for
 * browser-only storage mode. Uses the 'idb' library for a modern
 * Promise-based IndexedDB wrapper.
 * 
 * Schema:
 * - songs: Stores Song objects with indexes on album and artist
 * - playlists: Stores Playlist objects
 * 
 * Database: 'mediahub-db' version 1
 * 
 * Note: Primary storage is SQLite via Go backend when available.
 * IndexedDB is used as fallback for browser-only mode.
 * 
 * @module db
 */

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