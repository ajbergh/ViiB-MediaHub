/** Browser audio metadata parsing shared with the bundled worker and fixture tests. */

import { parseBlob } from 'music-metadata';

const ARTWORK_FILENAMES = [
    'cover.jpg', 'cover.jpeg', 'cover.png',
    'folder.jpg', 'folder.jpeg', 'folder.png',
    'album.jpg', 'album.jpeg', 'album.png'
];

/**
 * Returns transferable metadata and artwork; media URLs belong to the main thread.
 */
export async function parseBrowserSongMetadata(file: File, path: string, relevantImages: File[] = []) {
    const fileName = file.name.replace(/\.[^/.]+$/, ""); // remove extension

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
             const validImage = relevantImages.find(img => /\.(jpg|jpeg|png)$/i.test(img.name));
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
