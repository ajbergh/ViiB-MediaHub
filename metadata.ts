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
 * The worker code is inlined as a Blob to avoid module bundling issues.
 * 
 * Note: When backend is available, metadata is extracted server-side
 * using taglib-go for better performance and format support.
 * 
 * @module metadata
 */

import { Song } from './types';

// Inline Worker Code
// We use a Blob to create the worker to ensure it works in all environments without 
// worrying about file serving paths for .ts files or bundler configurations.
const WORKER_CODE = `
import { Buffer } from 'https://esm.sh/buffer@5.7.1';
import { parseBlob } from 'https://esm.sh/music-metadata@10.0.0?bundle';

// Polyfill Buffer for the worker environment as music-metadata might rely on it
globalThis.Buffer = Buffer;

const ARTWORK_FILENAMES = [
    'cover.jpg', 'cover.jpeg', 'cover.png',
    'folder.jpg', 'folder.jpeg', 'folder.png',
    'album.jpg', 'album.jpeg', 'album.png'
];

/**
 * Parsers logic duplicated here to run inside the worker scope.
 */
async function parseSongFile(file, path, relevantImages = []) {
    const fileName = file.name.replace(/\\.[^/.]+$/, ""); // remove extension
    
    let title = fileName;
    let artist = "Unknown Artist";
    let album = "Unknown Album";
    let albumArtist;
    let trackNumber;
    let discNumber;
    let genre;
    let year;
    let duration = 0;
    let coverData; // Return Blob, NOT URL

    const applyPathFallbacks = () => {
        if (path) {
            const parts = path.split('/');
            // Expected parts: RootFolder/Artist/Album/Song.ext
            if (parts.length >= 3) {
                 const potentialAlbum = parts[parts.length - 2];
                 if (!album || album === "Unknown Album") album = potentialAlbum;

                 if (parts.length >= 4) {
                     const potentialArtist = parts[parts.length - 3];
                     if (!artist || artist === "Unknown Artist") artist = potentialArtist;
                 }
            }
        }

        // Try regex on filename: "01 - Song Title", "01. Song Title"
        const trackMatch = fileName.match(/^(\\d+)\\s*[-.]?\\s*(.+)$/);
        if (trackMatch) {
            const num = parseInt(trackMatch[1], 10);
            const cleanTitle = trackMatch[2];
            
            if (!trackNumber) trackNumber = num;
            if (title === fileName) title = cleanTitle;
        } else {
             // Try "Artist - Title"
             const artistTitleMatch = fileName.match(/^(.+?)\\s*-\\s*(.+)$/);
             if (artistTitleMatch) {
                 if (artist === "Unknown Artist") artist = artistTitleMatch[1];
                 if (title === fileName) title = artistTitleMatch[2];
             }
        }
    };

    try {
        // Parse metadata
        const metadata = await parseBlob(file, { duration: true, skipCovers: false });
        const { common, format } = metadata;
        
        if (common.title && common.title.trim().length > 0) title = common.title;
        if (common.artist && common.artist.trim().length > 0) artist = common.artist;
        if (common.album && common.album.trim().length > 0) album = common.album;
        if (common.albumartist && common.albumartist.trim().length > 0) albumArtist = common.albumartist;
        
        if (common.track.no) trackNumber = common.track.no;
        if (common.disk.no) discNumber = common.disk.no;
        if (common.genre) genre = common.genre;
        if (common.year) year = common.year;
        if (format.duration) duration = format.duration;

        if (common.picture && common.picture.length > 0) {
            const pic = common.picture[0];
            coverData = new Blob([pic.data], { type: pic.format });
        }
    } catch (error) {
        console.warn('Failed to parse metadata for ' + file.name + '. Using fallback.');
        applyPathFallbacks();
    }

    // Apply fallbacks if metadata was empty or generic
    if (artist === "Unknown Artist" || album === "Unknown Album" || title === fileName) {
        applyPathFallbacks();
    }

    // Folder Image Fallback (Worker can't create URL, so we find the file and pass it back as coverData)
    if (!coverData && relevantImages.length > 0) {
        for (const nameToCheck of ARTWORK_FILENAMES) {
            const match = relevantImages.find(
                img => img.name.toLowerCase() === nameToCheck
            );
            if (match) {
                coverData = match; // It's already a File (Blob)
                break;
            }
        }
        
        // Fallback to any valid image in the folder
        if (!coverData) {
             const validImage = relevantImages.find(img => /\\.(jpg|jpeg|png)$/i.test(img.name));
             if (validImage) {
                 coverData = validImage;
             }
        }
    }

    return {
        title,
        artist,
        album,
        albumArtist,
        trackNumber,
        discNumber,
        genre,
        year,
        duration,
        coverData,
        // We do NOT return URLs here. URLs created in Worker are not valid in Main Thread.
    };
}

self.onmessage = async (e) => {
    const { id, file, path, relevantImages } = e.data;
    try {
        const metadata = await parseSongFile(file, path, relevantImages || []);
        self.postMessage({ id, metadata });
    } catch (error) {
        console.error('Worker Processing Error', error);
        self.postMessage({ id, error: error.message || 'Unknown worker error' });
    }
};
`;

// Singleton worker instance
let worker: Worker | null = null;
const pendingRequests = new Map<string, { resolve: (s: Song) => void, reject: (e: any) => void }>();

function getWorker() {
    if (!worker) {
        try {
            const blob = new Blob([WORKER_CODE], { type: 'application/javascript' });
            const workerUrl = URL.createObjectURL(blob);
            
            worker = new Worker(workerUrl, { type: 'module' });

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
                        url: URL.createObjectURL(file), // Valid Main Thread URL
                        coverData: metadata.coverData, // Blob
                        coverUrl: metadata.coverData ? URL.createObjectURL(metadata.coverData) : undefined,
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