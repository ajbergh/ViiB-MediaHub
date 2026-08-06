/**
 * ViiB MediaHub - Audio Player Hook
 * 
 * Custom React hook for managing dual-audio-element playback with crossfading.
 * Integrates with the AudioEngine for Web Audio API effects processing.
 * 
 * Features:
 * - Dual audio element management for seamless crossfading
 * - Current time and duration tracking for progress display
 * - Automatic song advancement and play count recording
 * - EQ band synchronization with audio settings
 * - Volume control through master gain node
 * - Pre-buffering of next track for smooth transitions
 * - Buffering state management for streaming tracks
 * 
 * Architecture:
 * - Uses two <audio> elements (primary/secondary) for transitions
 * - activePlayerIndex tracks which element is currently playing
 * - AudioEngine processes audio through EQ -> Analyser -> Master chain
 * 
 * @module useAudioPlayer
 */

import { useRef, useState, useEffect, useCallback } from 'react';
import { useStore } from '../store';
import { audioEngine } from '../lib/audio';
import { StreamingErrorType } from '../slices/types';
import { PlaybackContext } from '../types';
import {
    calculateReplayGain,
    isActivePlaybackEvent,
    normalizeCrossfadeDuration,
    resolvePlaybackContext,
} from '../lib/playbackLifecycle';

// Pre-buffer threshold: start preloading next track when X seconds remain
const PRELOAD_THRESHOLD_SECONDS = 15;

// Helper to determine error type from audio element error
const getStreamingErrorType = (error: MediaError | null): StreamingErrorType => {
    if (!error) return 'unknown';
    
    switch (error.code) {
        case MediaError.MEDIA_ERR_NETWORK:
            return 'network';
        case MediaError.MEDIA_ERR_SRC_NOT_SUPPORTED:
            return 'unavailable';
        case MediaError.MEDIA_ERR_DECODE:
            return 'unavailable';
        case MediaError.MEDIA_ERR_ABORTED:
            return 'unknown';
        default:
            return 'unknown';
    }
};

// Helper to get user-friendly error message
const getErrorMessage = (errorType: StreamingErrorType): string => {
    switch (errorType) {
        case 'network':
            return 'Network connection lost. Check your internet connection.';
        case 'auth':
            return 'Spotify session expired. Please log in again.';
        case 'unavailable':
            return 'This track is not available for streaming.';
        default:
            return 'An error occurred during playback.';
    }
};

export const useAudioPlayer = () => {
    // Dual Refs for Crossfading
    const primaryRef = useRef<HTMLAudioElement>(null);
    const secondaryRef = useRef<HTMLAudioElement>(null);
    const activePlayerIndex = useRef<number>(0); // 0 or 1
    
    // Track if we've already triggered preload for current song
    const hasTriggeredPreload = useRef<string | null>(null);
    
    // Track if we've already fixed duration for current song
    const hasFixedDuration = useRef<string | null>(null);

    // Throttle timeupdate: skip state updates when change is negligible
    const lastReportedTime = useRef<number>(0);
    
    // Track listening events for AI DJ preference learning
    // Stores the current song being tracked and its cumulative play duration
    const listenTrackingRef = useRef<{
        songId: string;
        songDuration: number;
        accumulatedPlayTime: number;
        lastMediaTime: number;
        context: PlaybackContext;
        isTracking: boolean;
    } | null>(null);
    
    // Local state for UI updates
    const [currentTime, setCurrentTime] = useState(0);
    const [duration, setDuration] = useState(0);
    
    // Track buffering start time for duration calculation
    const bufferStartTime = useRef<number | null>(null);

    // Only destructure reactive state needed for effect dependencies and rendering.
    // Action functions are accessed via useStore.getState() inside handlers to avoid stale closures (H-6).
    const { currentSong, isPlaying, volume, audioSettings } = useStore();

    // Init Engine & EQ
    useEffect(() => {
        // Ensure engine is awake if we have a song
        if (currentSong) {
            audioEngine.resume();
        }
    }, [currentSong]);

    useEffect(() => {
        if (audioSettings.eqEnabled) {
            audioEngine.setEqBands(audioSettings.eqBands);
        } else {
            audioEngine.setEqBands(new Array(10).fill(0));
        }
    }, [audioSettings.eqBands, audioSettings.eqEnabled]);

    useEffect(() => {
        audioEngine.setVolume(volume);
    }, [volume]);

    useEffect(() => {
        const gain = audioSettings.normalization
            ? calculateReplayGain(currentSong?.replayGainDb, currentSong?.replayPeak)
            : 1;
        audioEngine.setNormalizationGain(gain);
    }, [audioSettings.normalization, currentSong?.id, currentSong?.replayGainDb, currentSong?.replayPeak]);

    useEffect(() => {
        const applySink = async (element: HTMLAudioElement | null) => {
            if (!element) return;
            const sinkCapable = element as HTMLAudioElement & { setSinkId?: (id: string) => Promise<void> };
            if (typeof sinkCapable.setSinkId !== 'function') return;
            try {
                await sinkCapable.setSinkId(audioSettings.mainOutputDevice || '');
            } catch (error) {
                console.warn('[AudioPlayer] Failed to route output device', error);
            }
        };
        void applySink(primaryRef.current);
        void applySink(secondaryRef.current);
    }, [audioSettings.mainOutputDevice]);
    
    // Network recovery detection — all state via getState() to avoid stale closures
    useEffect(() => {
        const handleOnline = () => {
            const { currentSong, streamError, retryCount, showToast, retryStream } = useStore.getState();
            
            if (streamError?.type === 'network' && currentSong?.isStreaming) {
                console.log('[AudioPlayer] Network recovered, attempting to resume playback');
                showToast({
                    type: 'info',
                    message: 'Connection restored. Resuming playback...',
                    duration: 3000
                });
                
                if (retryCount < 3) {
                    retryStream();
                }
            }
        };
        
        const handleOffline = () => {
            const { currentSong, showToast } = useStore.getState();
            if (currentSong?.isStreaming) {
                console.warn('[AudioPlayer] Network went offline');
                showToast({
                    type: 'warning',
                    message: 'Network connection lost',
                    duration: 3000
                });
            }
        };
        
        window.addEventListener('online', handleOnline);
        window.addEventListener('offline', handleOffline);
        
        return () => {
            window.removeEventListener('online', handleOnline);
            window.removeEventListener('offline', handleOffline);
        };
    }, []);
    
    // Buffering and error event handlers for streaming tracks — all state via getState()
    useEffect(() => {
        const primary = primaryRef.current;
        const secondary = secondaryRef.current;
        if (!primary || !secondary) return;
        
        const activeElement = () => activePlayerIndex.current === 0 ? primaryRef.current : secondaryRef.current;
        const handleWaiting = (event: Event) => {
            if (!isActivePlaybackEvent(event.currentTarget, activeElement())) return;
            const { currentSong, setBuffering, recordStreamEvent } = useStore.getState();
            if (currentSong?.isStreaming) {
                console.log('[AudioPlayer] Buffering started...');
                setBuffering(true);
                
                bufferStartTime.current = Date.now();
                
                recordStreamEvent({
                    type: 'buffer_start',
                    trackId: currentSong.spotifyId || currentSong.id,
                    trackTitle: currentSong.title,
                    timestamp: Date.now()
                });
            }
        };
        
        const handleCanPlay = (event: Event) => {
            if (!isActivePlaybackEvent(event.currentTarget, activeElement())) return;
            const { currentSong, setBuffering, clearStreamError, recordStreamEvent } = useStore.getState();
            if (currentSong?.isStreaming) {
                console.log('[AudioPlayer] Buffering complete, can play');
                setBuffering(false);
                clearStreamError();
                
                if (bufferStartTime.current) {
                    const bufferDuration = Date.now() - bufferStartTime.current;
                    recordStreamEvent({
                        type: 'buffer_end',
                        trackId: currentSong.spotifyId || currentSong.id,
                        trackTitle: currentSong.title,
                        duration: bufferDuration,
                        timestamp: Date.now()
                    });
                    bufferStartTime.current = null;
                }
            }
        };
        
        const handleProgress = (e: Event) => {
            if (!isActivePlaybackEvent(e.currentTarget, activeElement())) return;
            const audio = e.target as HTMLAudioElement;
            if (audio.buffered.length > 0 && audio.duration > 0) {
                const bufferedEnd = audio.buffered.end(audio.buffered.length - 1);
                const progress = Math.round((bufferedEnd / audio.duration) * 100);
                useStore.getState().setBufferProgress(progress);
            }
        };
        
        const handleError = (e: Event) => {
            if (!isActivePlaybackEvent(e.currentTarget, activeElement())) return;
            const audio = e.target as HTMLAudioElement;
            const { currentSong, retryCount, setBuffering, setStreamError, showToast, retryStream, nextSong, recordStreamEvent } = useStore.getState();
            
            if (!currentSong?.isStreaming) return;
            
            const errorType = getStreamingErrorType(audio.error);
            const message = getErrorMessage(errorType);
            
            console.error('[AudioPlayer] Stream error:', errorType, audio.error?.message);
            
            recordStreamEvent({
                type: 'error',
                trackId: currentSong.spotifyId || currentSong.id,
                trackTitle: currentSong.title,
                errorType,
                timestamp: Date.now()
            });
            
            setBuffering(false);
            setStreamError({
                type: errorType,
                message,
                canRetry: errorType === 'network' || errorType === 'unknown',
                timestamp: Date.now()
            });
            
            if (errorType === 'network' && retryCount < 3) {
                console.log('[AudioPlayer] Auto-retrying after network error...');
                showToast({
                    type: 'warning',
                    message: 'Connection interrupted. Retrying...',
                    duration: 3000
                });
                retryStream();
            } else if (errorType === 'network') {
                showToast({
                    type: 'error',
                    message: 'Unable to stream. Check your connection or try downloading the track.',
                    duration: 5000
                });
            } else if (errorType === 'auth') {
                showToast({
                    type: 'error',
                    message: 'Spotify session expired. Please log in again.',
                    duration: 5000
                });
            } else if (errorType === 'unavailable') {
                showToast({
                    type: 'error',
                    message: 'This track is not available for streaming.',
                    duration: 4000
                });
                recordStreamEvent({
                    type: 'skip',
                    trackId: currentSong.spotifyId || currentSong.id,
                    trackTitle: currentSong.title,
                    errorType: 'unavailable',
                    timestamp: Date.now()
                });
                setTimeout(() => useStore.getState().nextSong(), 2000);
            }
        };
        
        const handleStalled = (event: Event) => {
            if (!isActivePlaybackEvent(event.currentTarget, activeElement())) return;
            const { currentSong, setBuffering } = useStore.getState();
            if (currentSong?.isStreaming) {
                console.warn('[AudioPlayer] Stream stalled');
                setBuffering(true);
            }
        };
        
        // Add listeners to both audio elements
        primary.addEventListener('waiting', handleWaiting);
        primary.addEventListener('canplay', handleCanPlay);
        primary.addEventListener('canplaythrough', handleCanPlay);
        primary.addEventListener('progress', handleProgress);
        primary.addEventListener('error', handleError);
        primary.addEventListener('stalled', handleStalled);
        
        secondary.addEventListener('waiting', handleWaiting);
        secondary.addEventListener('canplay', handleCanPlay);
        secondary.addEventListener('canplaythrough', handleCanPlay);
        secondary.addEventListener('progress', handleProgress);
        secondary.addEventListener('error', handleError);
        secondary.addEventListener('stalled', handleStalled);
        
        return () => {
            primary.removeEventListener('waiting', handleWaiting);
            primary.removeEventListener('canplay', handleCanPlay);
            primary.removeEventListener('canplaythrough', handleCanPlay);
            primary.removeEventListener('progress', handleProgress);
            primary.removeEventListener('error', handleError);
            primary.removeEventListener('stalled', handleStalled);
            
            secondary.removeEventListener('waiting', handleWaiting);
            secondary.removeEventListener('canplay', handleCanPlay);
            secondary.removeEventListener('canplaythrough', handleCanPlay);
            secondary.removeEventListener('progress', handleProgress);
            secondary.removeEventListener('error', handleError);
            secondary.removeEventListener('stalled', handleStalled);
        };
    }, []);

    // Playback Logic (Transition Handling)
    useEffect(() => {
        const primary = primaryRef.current;
        const secondary = secondaryRef.current;
        if (!primary || !secondary) return;
        
        // Register both with the engine
        audioEngine.register(primary);
        audioEngine.register(secondary);

        if (!currentSong) {
            primary.pause();
            secondary.pause();
            return;
        }

        const currentIndex = activePlayerIndex.current;
        const currentPlayer = currentIndex === 0 ? primary : secondary;
        
        // Check if song changed by comparing src
        // Note: We use .getAttribute('src') or check exact string match to avoid resolved URL issues
        if (currentPlayer.getAttribute('src') !== currentSong.url) {
            // Song Changed!
            
            // Record listen event for the PREVIOUS song before switching
            if (listenTrackingRef.current && listenTrackingRef.current.isTracking) {
                // Calculate final play duration
                const finalPlayTime = listenTrackingRef.current.accumulatedPlayTime;
                
                // Only record if we actually played for some time
                if (finalPlayTime > 0.5) {
                    useStore.getState().recordListenEvent(
                        listenTrackingRef.current.songId,
                        finalPlayTime,
                        listenTrackingRef.current.songDuration,
                        listenTrackingRef.current.context
                    );
                    console.log(`[AudioPlayer] Listen event recorded: ${finalPlayTime.toFixed(1)}s / ${listenTrackingRef.current.songDuration.toFixed(1)}s`);
                }
            }
            
            // Start tracking the NEW song
            const playbackContext = resolvePlaybackContext(
                currentSong.playbackContext,
                listenTrackingRef.current?.context,
            );
            listenTrackingRef.current = {
                songId: currentSong.id,
                songDuration: currentSong.duration || 0,
                accumulatedPlayTime: 0,
                lastMediaTime: 0,
                context: playbackContext,
                isTracking: true
            };
            
            const nextIndex = (currentIndex + 1) % 2;
            const nextPlayer = nextIndex === 0 ? primary : secondary;
            
            // Reset preload trigger for new song
            hasTriggeredPreload.current = null;
            
            // Reset duration fix tracker for new song
            hasFixedDuration.current = null;
            
            // Reset buffering state for new track
            const { setBuffering, setBufferProgress, recordStreamEvent } = useStore.getState();
            if (currentSong.isStreaming) {
                setBuffering(true);
                setBufferProgress(0);
                
                // Record stream start event
                recordStreamEvent({
                    type: 'start',
                    trackId: currentSong.spotifyId || currentSong.id,
                    trackTitle: currentSong.title,
                    timestamp: Date.now()
                });
            } else {
                setBuffering(false);
                setBufferProgress(100);
            }
            
            if (nextPlayer.getAttribute('src') !== currentSong.url) {
                nextPlayer.src = currentSong.url;
                nextPlayer.load();
            }

            if (isPlaying) {
                 const fadeDuration = normalizeCrossfadeDuration(audioSettings.crossfadeDuration, audioSettings.gapless);
                 audioEngine.transition(currentPlayer, nextPlayer, fadeDuration);
            } else {
                 // Not playing, just switch context silently
                 currentPlayer.pause();
                 nextPlayer.currentTime = 0;
                 // We don't play nextPlayer yet
            }
            
            activePlayerIndex.current = nextIndex;
        } else {
            // Same Song - Play/Pause Toggle
            if (isPlaying) {
                 // If paused, resume. 
                 if (currentPlayer.paused) {
                    audioEngine.transition(null, currentPlayer, 0.3);
                 }
            } else {
                 currentPlayer.pause();
            }
        }

    }, [currentSong, isPlaying, audioSettings.crossfadeDuration, audioSettings.gapless]);

    // Handlers — use getState() for fresh state to avoid stale closures
    const handleTimeUpdate = (playerIndex: number) => {
        if (playerIndex !== activePlayerIndex.current) return;
        
        const player = playerIndex === 0 ? primaryRef.current : secondaryRef.current;
        if (player) {
            const time = player.currentTime;
            const dur = player.duration || 0;

            const tracking = listenTrackingRef.current;
            if (tracking && tracking.songId === useStore.getState().currentSong?.id) {
                const delta = time - tracking.lastMediaTime;
                if (delta > 0 && delta < 5) tracking.accumulatedPlayTime += delta;
                tracking.lastMediaTime = time;
            }

            // Throttle state updates: only update when time changed by >= 0.25s
            const timeDelta = Math.abs(time - lastReportedTime.current);
            if (timeDelta >= 0.25 || time === 0) {
                lastReportedTime.current = time;
                setCurrentTime(time);
                setDuration(dur);
            }
            
            const { currentSong, updateSongDuration, preloadNextTrack } = useStore.getState();
            
            // Fix duration if it differs significantly from stored metadata
            // Only do this once per song, when we have a valid duration
            if (dur > 0 && currentSong && hasFixedDuration.current !== currentSong.id) {
                hasFixedDuration.current = currentSong.id;
                // Check if stored duration differs by more than 5%
                const diff = Math.abs(currentSong.duration - dur) / Math.max(currentSong.duration, dur);
                if (diff > 0.05) {
                    updateSongDuration(currentSong.id, dur);
                }
            }
            
            // Gapless preload uses the inactive player that will perform the handoff,
            // rather than a disposable Audio element whose buffer cannot be reused.
            const state = useStore.getState();
            if (state.audioSettings.gapless && dur > 0 && (dur - time) <= PRELOAD_THRESHOLD_SECONDS) {
                const nextTrack = state.queue[state.currentSongIndex + 1];
                const inactivePlayer = activePlayerIndex.current === 0 ? secondaryRef.current : primaryRef.current;
                if (nextTrack?.url && inactivePlayer && inactivePlayer.getAttribute('src') !== nextTrack.url) {
                    inactivePlayer.preload = 'auto';
                    inactivePlayer.src = nextTrack.url;
                    inactivePlayer.load();
                }
            }

            // Trigger pre-buffering of next track when approaching end of current track
            if (dur > 0 && (dur - time) <= PRELOAD_THRESHOLD_SECONDS) {
                // Only trigger once per song
                if (hasTriggeredPreload.current !== currentSong?.id) {
                    hasTriggeredPreload.current = currentSong?.id || null;
                    console.log('[AudioPlayer] Triggering pre-buffer of next track');
                    preloadNextTrack();
                }
            }
        }
    };

    const handleEnded = (playerIndex: number) => {
        if (playerIndex !== activePlayerIndex.current) return;
        
        const { currentSong, recordPlay, nextSong, recordListenEvent, recordStreamEvent } = useStore.getState();
        
        if (currentSong) {
            recordPlay(currentSong.id);
            
            // Record listen event as play_complete (full song played)
            // Calculate the total play time including any accumulated from pause/resume
            if (listenTrackingRef.current && listenTrackingRef.current.songId === currentSong.id) {
                const finalPlayTime = listenTrackingRef.current.accumulatedPlayTime;
                
                // Use actual song duration if available, otherwise use tracked duration
                const songDuration = currentSong.duration || listenTrackingRef.current.songDuration;
                
                recordListenEvent(
                    currentSong.id,
                    finalPlayTime,
                    songDuration,
                    listenTrackingRef.current.context
                );
                console.log(`[AudioPlayer] Song completed: ${finalPlayTime.toFixed(1)}s / ${songDuration.toFixed(1)}s`);
                
                // Clear tracking for this song
                listenTrackingRef.current = null;
            }
            
            // Record stream complete event for streaming tracks
            if (currentSong.isStreaming) {
                recordStreamEvent({
                    type: 'complete',
                    trackId: currentSong.spotifyId || currentSong.id,
                    trackTitle: currentSong.title,
                    timestamp: Date.now()
                });
            }
        }
        nextSong();
    };
    
    // Seek needs to apply to active player
    const seek = useCallback((time: number) => {
        const player = activePlayerIndex.current === 0 ? primaryRef.current : secondaryRef.current;
        if (player) {
            player.currentTime = time;
            if (listenTrackingRef.current) listenTrackingRef.current.lastMediaTime = time;
            setCurrentTime(time);
        }
    }, []);

    return {
        primaryRef,
        secondaryRef,
        currentTime,
        duration,
        handleTimeUpdate,
        handleEnded,
        seek
    };
};
