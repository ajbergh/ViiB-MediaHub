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
 * - Backend Spotify session must be connected
 *
 * @module hooks/useBackgroundEnrichment
 */

import { useEffect, useRef } from 'react';
import { useArtists, useStore } from '../store';
import api from '../services/api';
import { spotifyCooldownRemaining } from '../services/spotifyBackend';
import type { Album } from '../types';

type VisibleAlbum = Album & { plexBacked?: boolean };
let visibleAlbums: readonly VisibleAlbum[] = [];
let onVisibleAlbumsChanged: (() => void) | undefined;

export function useVisibleAlbumEnrichment(albums: readonly VisibleAlbum[]) {
    useEffect(() => {
        visibleAlbums = albums;
        if (albums.length) onVisibleAlbumsChanged?.();
        return () => { visibleAlbums = []; };
    }, [albums]);
}

/**
 * @internal Hook implementation — see module-level JSDoc above.
 */
export function useBackgroundEnrichment() {
    const fetchAlbumMetadata = useStore(state => state.fetchAlbumMetadata);
    const fetchArtistMetadata = useStore(state => state.fetchArtistMetadata);
    const backendAvailable = useStore(state => state.backendAvailable);
    const generation = useStore(state => state.spotifySessionGeneration);
    const spotifyConnected = useStore(state => state.spotifyConnected);
    const artists = useArtists(backendAvailable && spotifyConnected);
    const artistTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
    const processingRef = useRef(false);
    
    useEffect(() => {
        // Only run if backend is available and Spotify is connected
        if (!backendAvailable || !spotifyConnected) {
            return;
        }
        
        let cancelled = false;
        let timer: ReturnType<typeof setTimeout>;
        let nextAllowed = 0;
        const checkedVisible = new Set<string>();
        const current = () => !cancelled && useStore.getState().spotifyConnected && useStore.getState().spotifySessionGeneration === generation;
        const schedule = (delay: number) => {
            clearTimeout(timer);
            if (current()) timer = setTimeout(processNextAlbum, delay);
        };
        const processNextAlbum = async () => {
            if (!current()) return;
            const cooldown = spotifyCooldownRemaining();
            if (cooldown > 0) { schedule(cooldown + 1000); return; }
            if (processingRef.current) { schedule(1000); return; }
            processingRef.current = true;
            
            try {
                const state = useStore.getState();
                const visible = visibleAlbums.find(({ name, artist, plexBacked, coverUrl }) => {
                    const key = `${name}::${artist}`;
                    return !plexBacked && !coverUrl && !state.albumMetadata[key] &&
                        !state.fetchingAlbums.has(key) && !checkedVisible.has(key);
                });
                if (visible) checkedVisible.add(`${visible.name}::${visible.artist}`);
                const album = visible ? { albumName: visible.name, artistName: visible.artist } : await api.getNextAlbumMetadata();
                if (!current()) return;

                if (!album) {
                    schedule(60000);
                    return;
                }
                
                console.log(`🔄 Background enrichment: checking "${album.albumName}" by ${album.artistName}`);
                
                await fetchAlbumMetadata(album.albumName, album.artistName);
                
                // Wait 5 seconds before processing next album (rate limiting)
                nextAllowed = Date.now() + 5000;
                schedule(5000);
            } catch (error) {
                console.warn('Background enrichment error:', error);
                // On error, wait longer before retrying
                nextAllowed = Date.now() + 30000;
                schedule(30000);
            } finally {
                processingRef.current = false;
            }
        };
        
        onVisibleAlbumsChanged = () => {
            if (!processingRef.current) schedule(Math.max(300, nextAllowed - Date.now()));
        };
        schedule(visibleAlbums.length ? 300 : 10000);
        
        return () => {
            cancelled = true;
            clearTimeout(timer);
            onVisibleAlbumsChanged = undefined;
        };
    }, [backendAvailable, spotifyConnected, generation, fetchAlbumMetadata]);

    useEffect(() => {
        if (!spotifyConnected || artists.length === 0) {
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
            if (cancelled || !useStore.getState().spotifyConnected || useStore.getState().spotifySessionGeneration !== generation || currentIndex >= artistsToProcess.length) {
                return;
            }

            const cooldown = spotifyCooldownRemaining();
            if (cooldown > 0) {
                artistTimerRef.current = setTimeout(processNextArtist, cooldown + 1000);
                return;
            }

            const artist = artistsToProcess[currentIndex];
            console.log(`🔄 Background enrichment: checking artist "${artist.name}"`);
            try {
                await fetchArtistMetadata(artist.name);
            } catch (error) {
                console.warn('Background artist enrichment error:', error);
            }
            // Metadata actions may consume errors. The shared transport deadline
            // still pauses both workers and preserves the artist for a later retry.
            if (spotifyCooldownRemaining() === 0) currentIndex++;

            if (!cancelled && useStore.getState().spotifyConnected && useStore.getState().spotifySessionGeneration === generation && currentIndex < artistsToProcess.length) {
                artistTimerRef.current = setTimeout(processNextArtist, Math.max(10000, spotifyCooldownRemaining() + 1000));
            }
        };

        // Leave room for initial profile and browsing requests before enrichment.
        artistTimerRef.current = setTimeout(processNextArtist, 10000);

        return () => {
            cancelled = true;
            if (artistTimerRef.current) {
                clearTimeout(artistTimerRef.current);
                artistTimerRef.current = null;
            }
        };
    }, [artists, fetchArtistMetadata, spotifyConnected, generation]);
}
