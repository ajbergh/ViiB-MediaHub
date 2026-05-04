/**
 * ViiB MediaHub - Background Metadata Enrichment Hook
 *
 * Silently enriches albums with Spotify metadata in the background,
 * running only when both the backend and a Spotify session are available.
 *
 * Features:
 * - Processes one album every 5 seconds (Spotify API rate-limit friendly)
 * - Also re-checks albums whose cached data has expired (> 30 days old, not found)
 * - Backs off to 30 s on error and retries after 60 s when no albums remain
 * - No-ops when backend is unavailable or Spotify is not authenticated
 *
 * Requirements:
 * - Backend must be reachable (`backendAvailable === true`)
 * - Spotify OAuth token must be present (`spotifyAccessToken` is set)
 *
 * @module hooks/useBackgroundEnrichment
 */

import { useEffect, useRef } from 'react';
import { useStore } from '../store';
import api from '../services/api';

/**
 * @internal Hook implementation — see module-level JSDoc above.
 */
export function useBackgroundEnrichment() {
    const { fetchAlbumMetadata, backendAvailable, spotifyAccessToken } = useStore();
    const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
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
}
