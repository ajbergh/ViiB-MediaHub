/**
 * ViiB MediaHub - Media Session Hook
 * 
 * Integrates with the Web Media Session API to provide Windows System Media
 * Transport Controls (SMTC) support. This enables:
 * 
 * - Display of now playing metadata in Windows media overlay
 * - Album artwork in media controls
 * - Hardware media key support (play, pause, next, previous)
 * - Lock screen controls
 * - Bluetooth headset button support
 * - Timeline/seek bar in media controls
 * 
 * The Media Session API is supported by:
 * - Chrome 73+, Edge 79+, Firefox 82+, Safari 15+
 * - WebView2 (Wails build) - Chromium-based, full support
 * 
 * @see https://developer.mozilla.org/en-US/docs/Web/API/Media_Session_API
 * @see WINDOWS_SMTC_IMPLEMENTATION.md for implementation details
 * 
 * @module useMediaSession
 */

import { useEffect, useCallback } from 'react';
import { useStore } from '../store';

/**
 * Hook to integrate with the Media Session API for Windows SMTC support.
 * 
 * @param currentTime - Current playback position in seconds
 * @param duration - Total duration of current track in seconds
 * @param onSeek - Callback to seek to a specific time
 */
export const useMediaSession = (
    currentTime: number,
    duration: number,
    onSeek: (time: number) => void
) => {
    const { 
        currentSong, 
        isPlaying, 
        togglePlay, 
        nextSong, 
        prevSong 
    } = useStore();

    // =========================================================================
    // Phase 1: Metadata Updates
    // =========================================================================

    /**
     * Update Media Session metadata when current song changes.
     * Displays title, artist, album, and artwork in Windows SMTC.
     */
    useEffect(() => {
        if (!('mediaSession' in navigator)) {
            console.log('[MediaSession] API not supported');
            return;
        }

        if (!currentSong) {
            // Clear metadata when no song is playing
            navigator.mediaSession.metadata = null;
            return;
        }

        console.log('[MediaSession] Updating metadata:', currentSong.title);

        // Build artwork array - Media Session supports multiple sizes
        const artwork: MediaImage[] = [];
        if (currentSong.coverUrl) {
            // Add cover URL with multiple sizes for different display contexts
            artwork.push(
                { src: currentSong.coverUrl, sizes: '96x96', type: 'image/jpeg' },
                { src: currentSong.coverUrl, sizes: '128x128', type: 'image/jpeg' },
                { src: currentSong.coverUrl, sizes: '192x192', type: 'image/jpeg' },
                { src: currentSong.coverUrl, sizes: '256x256', type: 'image/jpeg' },
                { src: currentSong.coverUrl, sizes: '384x384', type: 'image/jpeg' },
                { src: currentSong.coverUrl, sizes: '512x512', type: 'image/jpeg' }
            );
        }

        try {
            navigator.mediaSession.metadata = new MediaMetadata({
                title: currentSong.title || 'Unknown Title',
                artist: currentSong.artist || 'Unknown Artist',
                album: currentSong.album || 'Unknown Album',
                artwork: artwork
            });
        } catch (error) {
            console.error('[MediaSession] Error setting metadata:', error);
        }
    }, [currentSong]);

    /**
     * Update playback state when play/pause changes.
     * Ensures Windows SMTC shows correct play/pause button.
     */
    useEffect(() => {
        if (!('mediaSession' in navigator)) return;

        navigator.mediaSession.playbackState = isPlaying ? 'playing' : 'paused';
        console.log('[MediaSession] Playback state:', isPlaying ? 'playing' : 'paused');
    }, [isPlaying]);

    // =========================================================================
    // Phase 2: Action Handlers
    // =========================================================================

    /**
     * Handle play action from media keys or SMTC.
     */
    const handlePlay = useCallback(() => {
        console.log('[MediaSession] Play action triggered');
        if (!isPlaying) {
            togglePlay();
        }
    }, [isPlaying, togglePlay]);

    /**
     * Handle pause action from media keys or SMTC.
     */
    const handlePause = useCallback(() => {
        console.log('[MediaSession] Pause action triggered');
        if (isPlaying) {
            togglePlay();
        }
    }, [isPlaying, togglePlay]);

    /**
     * Handle next track action from media keys or SMTC.
     */
    const handleNextTrack = useCallback(() => {
        console.log('[MediaSession] Next track action triggered');
        nextSong();
    }, [nextSong]);

    /**
     * Handle previous track action from media keys or SMTC.
     */
    const handlePreviousTrack = useCallback(() => {
        console.log('[MediaSession] Previous track action triggered');
        prevSong();
    }, [prevSong]);

    /**
     * Handle stop action - pause and return to beginning.
     */
    const handleStop = useCallback(() => {
        console.log('[MediaSession] Stop action triggered');
        if (isPlaying) {
            togglePlay();
        }
        onSeek(0);
    }, [isPlaying, togglePlay, onSeek]);

    /**
     * Register all action handlers with the Media Session API.
     */
    useEffect(() => {
        if (!('mediaSession' in navigator)) return;

        console.log('[MediaSession] Registering action handlers');

        // Play/Pause actions
        try {
            navigator.mediaSession.setActionHandler('play', handlePlay);
        } catch (e) {
            console.log('[MediaSession] "play" action not supported');
        }

        try {
            navigator.mediaSession.setActionHandler('pause', handlePause);
        } catch (e) {
            console.log('[MediaSession] "pause" action not supported');
        }

        // Track navigation actions
        try {
            navigator.mediaSession.setActionHandler('previoustrack', handlePreviousTrack);
        } catch (e) {
            console.log('[MediaSession] "previoustrack" action not supported');
        }

        try {
            navigator.mediaSession.setActionHandler('nexttrack', handleNextTrack);
        } catch (e) {
            console.log('[MediaSession] "nexttrack" action not supported');
        }

        try {
            navigator.mediaSession.setActionHandler('stop', handleStop);
        } catch (e) {
            console.log('[MediaSession] "stop" action not supported');
        }

        // Cleanup: unregister handlers when component unmounts
        return () => {
            if (!('mediaSession' in navigator)) return;
            
            try {
                navigator.mediaSession.setActionHandler('play', null);
                navigator.mediaSession.setActionHandler('pause', null);
                navigator.mediaSession.setActionHandler('previoustrack', null);
                navigator.mediaSession.setActionHandler('nexttrack', null);
                navigator.mediaSession.setActionHandler('stop', null);
            } catch (e) {
                // Ignore cleanup errors
            }
        };
    }, [handlePlay, handlePause, handlePreviousTrack, handleNextTrack, handleStop]);

    // =========================================================================
    // Phase 3: Timeline/Seek Support
    // =========================================================================

    /**
     * Handle seek to specific time from SMTC timeline scrubbing.
     */
    const handleSeekTo = useCallback((details: MediaSessionActionDetails) => {
        if (details.seekTime !== undefined) {
            console.log('[MediaSession] Seek to:', details.seekTime);
            onSeek(details.seekTime);
        }
    }, [onSeek]);

    /**
     * Handle seek backward action (rewind).
     */
    const handleSeekBackward = useCallback((details: MediaSessionActionDetails) => {
        const skipTime = details.seekOffset || 10; // Default 10 seconds
        const newTime = Math.max(currentTime - skipTime, 0);
        console.log('[MediaSession] Seek backward:', skipTime, 'seconds to', newTime);
        onSeek(newTime);
    }, [currentTime, onSeek]);

    /**
     * Handle seek forward action (fast forward).
     */
    const handleSeekForward = useCallback((details: MediaSessionActionDetails) => {
        const skipTime = details.seekOffset || 10; // Default 10 seconds
        const newTime = Math.min(currentTime + skipTime, duration);
        console.log('[MediaSession] Seek forward:', skipTime, 'seconds to', newTime);
        onSeek(newTime);
    }, [currentTime, duration, onSeek]);

    /**
     * Register seek action handlers.
     */
    useEffect(() => {
        if (!('mediaSession' in navigator)) return;

        // Seek to specific time
        try {
            navigator.mediaSession.setActionHandler('seekto', handleSeekTo);
        } catch (e) {
            console.log('[MediaSession] "seekto" action not supported');
        }

        // Seek backward/forward
        try {
            navigator.mediaSession.setActionHandler('seekbackward', handleSeekBackward);
        } catch (e) {
            console.log('[MediaSession] "seekbackward" action not supported');
        }

        try {
            navigator.mediaSession.setActionHandler('seekforward', handleSeekForward);
        } catch (e) {
            console.log('[MediaSession] "seekforward" action not supported');
        }

        return () => {
            if (!('mediaSession' in navigator)) return;
            
            try {
                navigator.mediaSession.setActionHandler('seekto', null);
                navigator.mediaSession.setActionHandler('seekbackward', null);
                navigator.mediaSession.setActionHandler('seekforward', null);
            } catch (e) {
                // Ignore cleanup errors
            }
        };
    }, [handleSeekTo, handleSeekBackward, handleSeekForward]);

    /**
     * Update position state for SMTC timeline display.
     * Called when currentTime or duration changes.
     */
    useEffect(() => {
        if (!('mediaSession' in navigator)) return;
        if (!currentSong) return;
        if (!duration || duration <= 0) return;

        // Only update position state when we have valid values
        if (!('setPositionState' in navigator.mediaSession)) {
            return;
        }

        try {
            navigator.mediaSession.setPositionState({
                duration: duration,
                playbackRate: 1.0,
                position: Math.min(currentTime, duration)
            });
        } catch (error) {
            // setPositionState can throw if values are invalid
            console.warn('[MediaSession] Error setting position state:', error);
        }
    }, [currentTime, duration, currentSong]);
};

export default useMediaSession;
