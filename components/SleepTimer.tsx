/**
 * ViiB MediaHub - Sleep Timer Component
 * 
 * Floating dialog for setting a sleep timer with volume fade.
 * 
 * Features:
 * - Preset time options (15, 30, 45, 60 minutes)
 * - Custom time input
 * - "End of current song" option
 * - "After X songs" option
 * - Gradual volume fade before stopping (last 30 seconds)
 * - Cancel/extend active timer
 * 
 * @module SleepTimer
 */

import React, { useState, useEffect, useCallback } from 'react';
import { X, Moon, Clock, Music, Timer, Plus, Minus } from 'lucide-react';
import { useStore } from '../store';
import { useFocusTrap } from '../hooks/useFocusTrap';

interface SleepTimerProps {
    isOpen: boolean;
    onClose: () => void;
}

export type SleepTimerMode = 'off' | 'time' | 'songs' | 'endOfSong';

export interface SleepTimerState {
    mode: SleepTimerMode;
    endTime: number | null; // timestamp when timer ends
    songsRemaining: number | null;
    fadeStarted: boolean;
    originalVolume: number | null;
}

/**
 * useSleepTimer - React hook to manage the sleep timer lifecycle.
 *
 * Returns:
 *  - timerState: SleepTimerState - Current state of the sleep timer
 *  - setTimer(minutes): void - Set a time-based sleep timer
 *  - setTimerBySongs(count): void - Set a song-count based sleep timer
 *  - setTimerEndOfSong(): void - Stop playback at the end of the current song
 *  - cancelTimer(): void - Cancel any active timers and restore volume
 *  - onSongEnd(): boolean - To be invoked when a song ends; returns true to stop playback if timer completes
 *  - getRemainingTime(): number | null - Remaining seconds for time-based timer
 */
export function useSleepTimer() {
    const { isPlaying, volume, setVolume, currentSong, showToast } = useStore();
    const [timerState, setTimerState] = useState<SleepTimerState>({
        mode: 'off',
        endTime: null,
        songsRemaining: null,
        fadeStarted: false,
        originalVolume: null
    });

    // Calculate remaining time in seconds
    const getRemainingTime = useCallback(() => {
        if (timerState.mode !== 'time' || !timerState.endTime) return null;
        const remaining = Math.max(0, timerState.endTime - Date.now());
        return Math.ceil(remaining / 1000);
    }, [timerState.mode, timerState.endTime]);

    // Set timer by minutes
    const setTimer = useCallback((minutes: number) => {
        const endTime = Date.now() + minutes * 60 * 1000;
        setTimerState({
            mode: 'time',
            endTime,
            songsRemaining: null,
            fadeStarted: false,
            originalVolume: volume
        });
        showToast({ 
            type: 'info', 
            message: `Sleep timer set for ${minutes} minutes`,
            duration: 3000
        });
    }, [volume, showToast]);

    // Set timer by song count
    const setTimerBySongs = useCallback((count: number) => {
        setTimerState({
            mode: 'songs',
            endTime: null,
            songsRemaining: count,
            fadeStarted: false,
            originalVolume: volume
        });
        showToast({ 
            type: 'info', 
            message: `Sleep timer set for ${count} more song${count > 1 ? 's' : ''}`,
            duration: 3000
        });
    }, [volume, showToast]);

    // Set timer to end of current song
    const setTimerEndOfSong = useCallback(() => {
        setTimerState({
            mode: 'endOfSong',
            endTime: null,
            songsRemaining: 1,
            fadeStarted: false,
            originalVolume: volume
        });
        showToast({ 
            type: 'info', 
            message: 'Playback will stop at the end of this song',
            duration: 3000
        });
    }, [volume, showToast]);

    // Cancel timer
    const cancelTimer = useCallback(() => {
        // Restore volume if we were fading
        if (timerState.originalVolume !== null && timerState.fadeStarted) {
            setVolume(timerState.originalVolume);
        }
        setTimerState({
            mode: 'off',
            endTime: null,
            songsRemaining: null,
            fadeStarted: false,
            originalVolume: null
        });
        showToast({ 
            type: 'info', 
            message: 'Sleep timer cancelled',
            duration: 2000
        });
    }, [timerState.originalVolume, timerState.fadeStarted, setVolume, showToast]);

    // Handle song end - decrement counter for song-based timer
    const onSongEnd = useCallback(() => {
        if (timerState.mode === 'songs' || timerState.mode === 'endOfSong') {
            const remaining = (timerState.songsRemaining || 1) - 1;
            if (remaining <= 0) {
                // Timer complete
                setTimerState(prev => ({ ...prev, mode: 'off', songsRemaining: null }));
                return true; // Signal to stop playback
            }
            setTimerState(prev => ({ ...prev, songsRemaining: remaining }));
        }
        return false;
    }, [timerState.mode, timerState.songsRemaining]);

    // Timer tick effect for time-based timer
    useEffect(() => {
        if (timerState.mode !== 'time' || !timerState.endTime || !isPlaying) return;

        const interval = setInterval(() => {
            const remaining = timerState.endTime! - Date.now();
            
            // Last 30 seconds - start fading
            if (remaining <= 30000 && remaining > 0 && !timerState.fadeStarted) {
                setTimerState(prev => ({ ...prev, fadeStarted: true }));
            }
            
            // Fade volume
            if (remaining <= 30000 && remaining > 0 && timerState.originalVolume !== null) {
                const fadeProgress = remaining / 30000; // 1 -> 0 over 30 seconds
                const newVolume = timerState.originalVolume * fadeProgress;
                setVolume(Math.max(0, newVolume));
            }
            
            // Timer complete
            if (remaining <= 0) {
                setVolume(0);
                // Signal to pause playback
                const { togglePlay, isPlaying: stillPlaying } = useStore.getState();
                if (stillPlaying) {
                    togglePlay();
                }
                // Restore original volume for next play
                if (timerState.originalVolume !== null) {
                    setTimeout(() => setVolume(timerState.originalVolume!), 500);
                }
                setTimerState({
                    mode: 'off',
                    endTime: null,
                    songsRemaining: null,
                    fadeStarted: false,
                    originalVolume: null
                });
                showToast({ 
                    type: 'success', 
                    message: 'Sleep timer: Playback stopped',
                    duration: 4000
                });
            }
        }, 1000);

        return () => clearInterval(interval);
    }, [timerState.mode, timerState.endTime, timerState.fadeStarted, timerState.originalVolume, isPlaying, setVolume, showToast]);

    return {
        timerState,
        setTimer,
        setTimerBySongs,
        setTimerEndOfSong,
        cancelTimer,
        onSongEnd,
        getRemainingTime
    };
}

// Format seconds to MM:SS
function formatTime(seconds: number): string {
    const mins = Math.floor(seconds / 60);
    const secs = seconds % 60;
    return `${mins}:${secs.toString().padStart(2, '0')}`;
}

export const SleepTimer: React.FC<SleepTimerProps & { 
    timerState: SleepTimerState;
    setTimer: (minutes: number) => void;
    setTimerBySongs: (count: number) => void;
    setTimerEndOfSong: () => void;
    cancelTimer: () => void;
    getRemainingTime: () => number | null;
}> = ({ 
    isOpen, 
    onClose, 
    timerState, 
    setTimer, 
    setTimerBySongs, 
    setTimerEndOfSong, 
    cancelTimer,
    getRemainingTime 
}) => {
    const [customMinutes, setCustomMinutes] = useState(30);
    const [songCount, setSongCount] = useState(3);
    const [activeTab, setActiveTab] = useState<'time' | 'songs'>('time');
    const dialogRef = useFocusTrap<HTMLDivElement>(isOpen, onClose);

    if (!isOpen) return null;

    const remainingSeconds = getRemainingTime();
    const isActive = timerState.mode !== 'off';

    const presetTimes = [15, 30, 45, 60, 90, 120];

    return (
        <div
            className="fixed inset-0 z-[900] flex items-center justify-center p-4 bg-black/50 backdrop-blur-sm animate-in fade-in duration-200 motion-reduce:animate-none motion-reduce:transition-none"
            onClick={onClose}
        >
            <div
                ref={dialogRef}
                role="dialog"
                aria-modal="true"
                aria-labelledby="sleep-timer-dialog-title"
                tabIndex={-1}
                onClick={(e) => e.stopPropagation()}
                className="w-full max-w-md bg-surface-1 border border-surface-border rounded-2xl shadow-2xl overflow-hidden outline-none"
            >
                {/* Header */}
                <div className="h-14 border-b border-surface-3 bg-surface-2 px-5 flex items-center justify-between">
                    <div className="flex items-center gap-3">
                        <Moon size={20} aria-hidden="true" className="text-brand" />
                        <h2 id="sleep-timer-dialog-title" className="text-lg font-bold text-text-main">Sleep Timer</h2>
                    </div>
                    <button
                        onClick={onClose}
                        aria-label="Close sleep timer"
                        className="p-2 text-text-subtle hover:text-text-main transition-colors rounded-full hover:bg-surface-3"
                    >
                        <X size={20} aria-hidden="true" />
                    </button>
                </div>

                <div className="p-5">
                    {/* Active Timer Display */}
                    {isActive && (
                        <div className="mb-5 p-4 bg-brand/10 border border-brand/30 rounded-xl">
                            <div className="flex items-center justify-between">
                                <div className="flex items-center gap-3">
                                    <div className="w-10 h-10 bg-brand/20 rounded-full flex items-center justify-center">
                                        <Timer size={20} className="text-brand" />
                                    </div>
                                    <div>
                                        <p className="text-sm text-text-secondary">Timer Active</p>
                                        {timerState.mode === 'time' && remainingSeconds !== null && (
                                            <p className="text-xl font-bold text-white">{formatTime(remainingSeconds)}</p>
                                        )}
                                        {timerState.mode === 'songs' && (
                                            <p className="text-xl font-bold text-white">
                                                {timerState.songsRemaining} song{timerState.songsRemaining !== 1 ? 's' : ''} left
                                            </p>
                                        )}
                                        {timerState.mode === 'endOfSong' && (
                                            <p className="text-xl font-bold text-white">End of this song</p>
                                        )}
                                    </div>
                                </div>
                                <button
                                    onClick={cancelTimer}
                                    className="px-4 py-2 bg-error/20 text-error rounded-lg text-sm font-medium hover:bg-error/30 transition-colors"
                                >
                                    Cancel
                                </button>
                            </div>
                            {timerState.fadeStarted && (
                                <p className="text-xs text-text-secondary mt-2">Volume fading...</p>
                            )}
                        </div>
                    )}

                    {/* Tab Selector */}
                    <div className="flex gap-2 mb-5 p-1 bg-surface-3 rounded-lg">
                        <button
                            onClick={() => setActiveTab('time')}
                            className={`flex-1 flex items-center justify-center gap-2 py-2 px-4 rounded-md text-sm font-medium transition-colors ${
                                activeTab === 'time' 
                                    ? 'bg-surface-1 text-white shadow' 
                                    : 'text-text-secondary hover:text-white'
                            }`}
                        >
                            <Clock size={16} />
                            By Time
                        </button>
                        <button
                            onClick={() => setActiveTab('songs')}
                            className={`flex-1 flex items-center justify-center gap-2 py-2 px-4 rounded-md text-sm font-medium transition-colors ${
                                activeTab === 'songs' 
                                    ? 'bg-surface-1 text-white shadow' 
                                    : 'text-text-secondary hover:text-white'
                            }`}
                        >
                            <Music size={16} />
                            By Songs
                        </button>
                    </div>

                    {/* Time-based Options */}
                    {activeTab === 'time' && (
                        <div className="space-y-4">
                            {/* Preset Times */}
                            <div className="grid grid-cols-3 gap-2">
                                {presetTimes.map((mins) => (
                                    <button
                                        key={mins}
                                        onClick={() => { setTimer(mins); onClose(); }}
                                        className="py-3 px-4 bg-surface-3 hover:bg-surface-hover text-white rounded-lg text-sm font-medium transition-colors"
                                    >
                                        {mins >= 60 ? `${mins / 60}h` : `${mins}m`}
                                    </button>
                                ))}
                            </div>

                            {/* Custom Time */}
                            <div className="flex items-center gap-3 p-3 bg-surface-3 rounded-lg">
                                <span className="text-sm text-text-secondary flex-shrink-0">Custom:</span>
                                <div className="flex items-center gap-2 flex-1">
                                    <button
                                        onClick={() => setCustomMinutes(Math.max(5, customMinutes - 5))}
                                        className="p-2 bg-surface-2 hover:bg-surface-hover rounded-lg transition-colors"
                                    >
                                        <Minus size={16} />
                                    </button>
                                    <input
                                        type="number"
                                        value={customMinutes}
                                        onChange={(e) => setCustomMinutes(Math.max(1, parseInt(e.target.value) || 1))}
                                        className="w-16 text-center bg-surface-2 rounded-lg py-2 text-white font-medium outline-none focus:ring-2 focus:ring-brand"
                                    />
                                    <span className="text-sm text-text-secondary">min</span>
                                    <button
                                        onClick={() => setCustomMinutes(customMinutes + 5)}
                                        className="p-2 bg-surface-2 hover:bg-surface-hover rounded-lg transition-colors"
                                    >
                                        <Plus size={16} />
                                    </button>
                                </div>
                                <button
                                    onClick={() => { setTimer(customMinutes); onClose(); }}
                                    className="px-4 py-2 bg-brand hover:bg-brand-hover text-black font-medium rounded-lg transition-colors"
                                >
                                    Set
                                </button>
                            </div>
                        </div>
                    )}

                    {/* Song-based Options */}
                    {activeTab === 'songs' && (
                        <div className="space-y-4">
                            {/* End of Current Song */}
                            <button
                                onClick={() => { setTimerEndOfSong(); onClose(); }}
                                className="w-full py-3 px-4 bg-surface-3 hover:bg-surface-hover text-white rounded-lg text-sm font-medium transition-colors flex items-center justify-center gap-2"
                            >
                                <Music size={16} />
                                End of This Song
                            </button>

                            {/* Song Count */}
                            <div className="flex items-center gap-3 p-3 bg-surface-3 rounded-lg">
                                <span className="text-sm text-text-secondary flex-shrink-0">After:</span>
                                <div className="flex items-center gap-2 flex-1">
                                    <button
                                        onClick={() => setSongCount(Math.max(1, songCount - 1))}
                                        className="p-2 bg-surface-2 hover:bg-surface-hover rounded-lg transition-colors"
                                    >
                                        <Minus size={16} />
                                    </button>
                                    <input
                                        type="number"
                                        value={songCount}
                                        onChange={(e) => setSongCount(Math.max(1, parseInt(e.target.value) || 1))}
                                        className="w-16 text-center bg-surface-2 rounded-lg py-2 text-white font-medium outline-none focus:ring-2 focus:ring-brand"
                                    />
                                    <span className="text-sm text-text-secondary">songs</span>
                                    <button
                                        onClick={() => setSongCount(songCount + 1)}
                                        className="p-2 bg-surface-2 hover:bg-surface-hover rounded-lg transition-colors"
                                    >
                                        <Plus size={16} />
                                    </button>
                                </div>
                                <button
                                    onClick={() => { setTimerBySongs(songCount); onClose(); }}
                                    className="px-4 py-2 bg-brand hover:bg-brand-hover text-black font-medium rounded-lg transition-colors"
                                >
                                    Set
                                </button>
                            </div>
                        </div>
                    )}

                    {/* Info */}
                    <p className="mt-5 text-xs text-text-subtle text-center">
                        Volume will gradually fade in the last 30 seconds
                    </p>
                </div>
            </div>
        </div>
    );
};

export default SleepTimer;
