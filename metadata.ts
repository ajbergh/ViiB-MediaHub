/**
 * ViiB MediaHub - Browser-Side Metadata Extraction
 * 
 * Provides audio metadata parsing using music-metadata in a Web Worker.
 * Used for browser-only mode when Go backend is unavailable.
 * 
 * Features:
 * - Web Worker for non-blocking parsing
 * - Supports common audio formats via music-metadata
 * - Album artwork extraction from embedded tags
 * - Fallback artwork search in same folder
 * - Automatic track info parsing from filename
 * 
 * The worker and pinned parser are bundled locally for offline browser import.
 * 
 * Note: When backend is available, metadata is extracted server-side
 * using taglib-go for better performance and format support.
 * 
 * @module metadata
 */

import { Song } from './types';
import { managedObjectUrls } from './lib/playbackLifecycle';

let worker: Worker | null = null;
const pendingRequests = new Map<string, { resolve: (s: Song) => void, reject: (e: any) => void }>();

export function resetMetadataWorkerForTests(): void {
    worker?.terminate?.();
    worker = null;
    pendingRequests.clear();
}

function getWorker() {
    if (!worker) {
        try {
            worker = new Worker(new URL('./workers/browserMetadata.worker.ts', import.meta.url), { type: 'module' });

            worker.onmessage = (e) => {
                const { id, metadata, error } = e.data;
                const req = pendingRequests.get(id);
                if (req) {
                    if (error) {
                        req.reject(error);
                    } else {
                        // Resolve with the metadata
                        // We construct the full Song object in the Main Thread to ensure URLs are valid
                        req.resolve(metadata);
                    }
                    pendingRequests.delete(id);
                }
            };

            worker.onerror = (e) => {
                console.error("Worker Global Error", e.message, e);
                const error = new Error(e.message || 'Metadata worker failed to load');
                for (const request of pendingRequests.values()) request.reject(error);
                pendingRequests.clear();
                worker?.terminate();
                worker = null;
            };
        } catch (error) {
            console.error("Failed to initialize worker:", error);
            throw error;
        }
    }
    return worker;
}

const generateId = () => Math.random().toString(36).substr(2, 9);

/**
 * Parses a song using a Web Worker to avoid blocking the main thread.
 */
export function parseSong(
    file: File, 
    folderImages: Map<string, File[]>
): Promise<Song> {
    const id = generateId();
    
    // Extract path information
    const path = file.webkitRelativePath || '';
    const parts = path.split('/');
    parts.pop(); // Remove filename
    const folderPath = parts.join('/');
    
    // Get only the images relevant for this specific song/folder
    const relevantImages = folderImages.get(folderPath) || [];

    return new Promise((resolve, reject) => {
        // Set a timeout to prevent hanging forever on corrupt files
        const timeoutId = setTimeout(() => {
            if (pendingRequests.has(id)) {
                pendingRequests.delete(id);
                reject(new Error(`Timeout parsing file: ${file.name}`));
            }
        }, 15000); // 15 seconds timeout per file

        try {
            const w = getWorker();
            if (!w) {
                clearTimeout(timeoutId);
                reject(new Error("Worker could not be initialized"));
                return;
            }
            
            pendingRequests.set(id, { 
                resolve: (metadata: any) => {
                    clearTimeout(timeoutId);
                    
                    // Construct the final Song object on the Main Thread
                    // generating URLs here ensures they are valid for the DOM
                    const song: Song = {
                        id: generateId(), // New ID for DB
                        title: metadata.title,
                        artist: metadata.artist,
                        album: metadata.album,
                        albumArtist: metadata.albumArtist,
                        trackNumber: metadata.trackNumber,
                        discNumber: metadata.discNumber,
                        genre: metadata.genre,
                        year: metadata.year,
                        duration: metadata.duration,
                        url: managedObjectUrls.create(file), // Owned Main Thread URL
                        coverData: metadata.coverData, // Blob
                        coverUrl: metadata.coverData ? managedObjectUrls.create(metadata.coverData) : undefined,
                        addedAt: Date.now(),
                        path: path
                    };

                    resolve(song);
                }, 
                reject: (err) => {
                    clearTimeout(timeoutId);
                    reject(err);
                } 
            });
            
            w.postMessage({ 
                id, 
                file, 
                path,
                relevantImages 
            });
        } catch (e) {
            clearTimeout(timeoutId);
            reject(e);
        }
    });
}