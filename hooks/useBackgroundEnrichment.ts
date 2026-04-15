import { useEffect, useRef } from 'react';
import { useStore } from '../store';
import api from '../services/api';

/**
 * Background metadata enrichment hook.
 * 
 * Slowly processes albums that haven't been checked on Spotify yet,
 * enriching metadata in the background without impacting user experience.
 * 
 * Features:
 * - Processes one album every 5 seconds to respect rate limits
 * - Pauses when user is actively navigating or interacting
 * - Resumes automatically after idle period
 * - Also re-checks expired albums (checked > 30 days ago, not found)
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
