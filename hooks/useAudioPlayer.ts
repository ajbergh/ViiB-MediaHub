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

export const useAudioPlayer = () => {
    // Dual Refs for Crossfading
    const primaryRef = useRef<HTMLAudioElement>(null);
    const secondaryRef = useRef<HTMLAudioElement>(null);
    const activePlayerIndex = useRef<number>(0); // 0 or 1
    
    // Local state for UI updates
    const [currentTime, setCurrentTime] = useState(0);
    const [duration, setDuration] = useState(0);

    const { 
        currentSong, isPlaying, volume, audioSettings,
        nextSong, recordPlay
    } = useStore();

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
            const nextIndex = (currentIndex + 1) % 2;
            const nextPlayer = nextIndex === 0 ? primary : secondary;
            
            nextPlayer.src = currentSong.url;
            nextPlayer.load();

            if (isPlaying) {
                 const fadeDuration = audioSettings.crossfadeDuration || 0.2; 
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

    }, [currentSong, isPlaying, audioSettings.crossfadeDuration]);

    // Handlers
    const handleTimeUpdate = (playerIndex: number) => {
        if (playerIndex !== activePlayerIndex.current) return;
        
        const player = playerIndex === 0 ? primaryRef.current : secondaryRef.current;
        if (player) {
            setCurrentTime(player.currentTime);
            setDuration(player.duration || 0);
        }
    };

    const handleEnded = (playerIndex: number) => {
        if (playerIndex !== activePlayerIndex.current) return;
        
        if (currentSong) {
            recordPlay(currentSong.id);
        }
        nextSong();
    };
    
    // Seek needs to apply to active player
    const seek = useCallback((time: number) => {
        const player = activePlayerIndex.current === 0 ? primaryRef.current : secondaryRef.current;
        if (player) {
            player.currentTime = time;
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