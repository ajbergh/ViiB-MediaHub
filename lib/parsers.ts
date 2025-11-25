import { parseBlob } from 'https://esm.sh/music-metadata@10.0.0?bundle';
import { Song } from '../types';
import { generateId } from '../utils';

const ARTWORK_FILENAMES = [
    'cover.jpg', 'cover.jpeg', 'cover.png',
    'folder.jpg', 'folder.jpeg', 'folder.png',
    'album.jpg', 'album.jpeg', 'album.png'
];

/**
 * Shared logic to parse a song file.
 * Used by the Web Worker to prevent blocking the main thread.
 */
export async function parseSongFile(
    file: File, 
    path: string, 
    relevantImages: File[] = []
): Promise<Song> {
    const fileName = file.name.replace(/\.[^/.]+$/, ""); // remove extension
    
    // Default values
    let title = fileName;
    let artist = "Unknown Artist";
    let album = "Unknown Album";
    let albumArtist: string | undefined;
    let trackNumber: number | undefined;
    let discNumber: number | undefined;
    let genre: string[] | undefined;
    let year: number | undefined;
    let duration = 0;
    let coverUrl: string | undefined;

    // Helper: Parse info from file path and name
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

        // Try regex on filename: "01 - Song Title", "01. Song Title", "01 Song Title"
        const trackMatch = fileName.match(/^(\d+)\s*[-.]?\s*(.+)$/);
        if (trackMatch) {
            const num = parseInt(trackMatch[1], 10);
            const cleanTitle = trackMatch[2];
            
            if (!trackNumber) trackNumber = num;
            if (title === fileName) title = cleanTitle;
        } else {
             // Try "Artist - Title"
             const artistTitleMatch = fileName.match(/^(.+?)\s*-\s*(.+)$/);
             if (artistTitleMatch) {
                 if (artist === "Unknown Artist") artist = artistTitleMatch[1];
                 if (title === fileName) title = artistTitleMatch[2];
             }
        }
    };

    try {
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
            const blob = new Blob([pic.data], { type: pic.format });
            coverUrl = URL.createObjectURL(blob);
        }
    } catch (error) {
        console.warn(`Failed to parse metadata for ${file.name}. Using fallback.`, error);
        applyPathFallbacks();
    }

    if (artist === "Unknown Artist" || album === "Unknown Album" || title === fileName) {
        applyPathFallbacks();
    }

    // --- Folder Image Fallback ---
    if (!coverUrl && relevantImages.length > 0) {
        // Look for priority artwork
        for (const nameToCheck of ARTWORK_FILENAMES) {
            const match = relevantImages.find(
                img => img.name.toLowerCase() === nameToCheck
            );
            if (match) {
                coverUrl = URL.createObjectURL(match);
                break;
            }
        }
        
        // Fallback to any image
        if (!coverUrl) {
             const validImage = relevantImages.find(img => /\.(jpg|jpeg|png)$/i.test(img.name));
             if (validImage) {
                 coverUrl = URL.createObjectURL(validImage);
             }
        }
    }

    return {
        id: generateId(),
        title,
        artist,
        album,
        albumArtist,
        trackNumber,
        discNumber,
        genre,
        year,
        duration,
        url: URL.createObjectURL(file),
        coverUrl,
        addedAt: Date.now(),
        path: path
    };
}