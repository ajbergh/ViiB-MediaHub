/**
 * ViiB MediaHub - Background Metadata Enrichment Hook
 *
 * Silently enriches albums and artists with Spotify metadata in the background.
 *
 * Features:
 * - Processes one album every 5 seconds (Spotify API rate-limit friendly)
 * - Fetches artist artwork without requiring the Artists page to be mounted
 * - Also re-checks albums whose cached data has expired (> 30 days old, not found)
 * - Backs off to 30 s on error and retries after 60 s when no albums remain
 * - Album enrichment no-ops without the backend; all enrichment no-ops without Spotify
 *
 * Requirements:
 * - Backend must be reachable for album enrichment (`backendAvailable === true`)
 * - Spotify OAuth token must be present (`spotifyAccessToken` is set)
 *
 * @module hooks/useBackgroundEnrichment
 */

import { useEffect, useRef } from 'react';
import { useArtists, useStore } from '../store';
import api from '../services/api';

/**
 * @internal Hook implementation — see module-level JSDoc above.
 */
export function useBackgroundEnrichment() {
    const fetchAlbumMetadata = useStore(state => state.fetchAlbumMetadata);
    const fetchArtistMetadata = useStore(state => state.fetchArtistMetadata);
    const backendAvailable = useStore(state => state.backendAvailable);
    const spotifyAccessToken = useStore(state => state.spotifyAccessToken);
    const artists = useArtists();
    const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
    const artistTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
    const processingRef = useRef(false);
    
    useEffect(() => {
        // Only run if backend is available and Spotify is connected
        if (!backendAvailable || !spotifyAccessToken) {
            return;
        }
        
        const processNextAlbum = async () => {
            if (processingRef.current) return;
            processingRef.current = true;
            
            try {
                // Get unchecked albums from backend
                const unchecked = await api.getUncheckedAlbumMetadata();
                const expired = await api.getExpiredAlbumMetadata();
                
                // Combine and pick the first one
                const albumsToProcess = [...unchecked, ...expired];
                
                if (albumsToProcess.length === 0) {
                    // No albums to process, check again in 60 seconds
                    timerRef.current = setTimeout(processNextAlbum, 60000);
                    return;
                }
                
                // Process one album
                const album = albumsToProcess[0];
                console.log(`🔄 Background enrichment: checking "${album.albumName}" by ${album.artistName}`);
                
                await fetchAlbumMetadata(album.albumName, album.artistName);
                
                // Wait 5 seconds before processing next album (rate limiting)
                timerRef.current = setTimeout(processNextAlbum, 5000);
            } catch (error) {
                console.warn('Background enrichment error:', error);
                // On error, wait longer before retrying
                timerRef.current = setTimeout(processNextAlbum, 30000);
            } finally {
                processingRef.current = false;
            }
        };
        
        // Start enrichment after 10 seconds of startup
        const startupTimer = setTimeout(() => {
            processNextAlbum();
        }, 10000);
        
        return () => {
            clearTimeout(startupTimer);
            if (timerRef.current) {
                clearTimeout(timerRef.current);
            }
        };
    }, [backendAvailable, spotifyAccessToken, fetchAlbumMetadata]);

    useEffect(() => {
        if (!spotifyAccessToken || artists.length === 0) {
            return;
        }

        // Build a stable queue for this library snapshot. Reading the metadata
        // directly from the store avoids restarting this worker every time an
        // artist image arrives.
        const artistsToProcess = artists.filter(
            artist => !useStore.getState().artistMetadata[artist.name],
        );
        if (artistsToProcess.length === 0) {
            return;
        }

        let currentIndex = 0;
        let cancelled = false;

        const processNextArtist = async () => {
            if (cancelled || currentIndex >= artistsToProcess.length) {
                return;
            }

            const artist = artistsToProcess[currentIndex++];
            console.log(`🔄 Background enrichment: checking artist "${artist.name}"`);
            await fetchArtistMetadata(artist.name);

            if (!cancelled && currentIndex < artistsToProcess.length) {
                artistTimerRef.current = setTimeout(processNextArtist, 500);
            }
        };

        // Preserve the former Artists-page startup delay while allowing this
        // work to continue regardless of the active route.
        artistTimerRef.current = setTimeout(processNextArtist, 2000);

        return () => {
            cancelled = true;
            if (artistTimerRef.current) {
                clearTimeout(artistTimerRef.current);
                artistTimerRef.current = null;
            }
        };
    }, [artists, fetchArtistMetadata, spotifyAccessToken]);
}
