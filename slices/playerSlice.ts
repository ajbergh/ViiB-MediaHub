/**
 * ViiB MediaHub - Player State Slice
 * 
 * Zustand slice managing audio playback state and settings.
 * 
 * State:
 * - isPlaying: Current playback state
 * - currentSong: Currently loaded song object
 * - queue: Array of songs in playback queue
 * - volume: Master volume level (0-1)
 * - audioSettings: EQ, crossfade, visualizer configuration
 * 
 * Features:
 * - Song playback with automatic file handle resolution
 * - Queue management (add, remove, clear, shuffle)
 * - Auto-EQ based on song genre
 * - Navigation between songs (next/prev)
 * - Play count recording
 * - Audio settings persistence to backend database
 * 
 * @module playerSlice
 */

import { StateCreator } from 'zustand';
import { AppState, PlayerSlice, StreamingError, StreamingStats, StreamingEvent } from './types';
import { EQ_PRESETS } from '../utils';
import { libraryService } from '../services/libraryService';
import { api } from '../services/api';
import { AudioSettings, MilkdropSettings } from '../types';

// Maximum retry attempts for streaming errors
const MAX_RETRY_ATTEMPTS = 3;
// Base delay for exponential backoff (in ms)
const BASE_RETRY_DELAY = 1000;

// Debounce timeout for saving settings (prevent rapid saves)
let saveSettingsTimeout: ReturnType<typeof setTimeout> | null = null;

/**
 * Saves audio settings to the backend database with debouncing.
 * This ensures we don't spam the backend with every slider movement.
 */
const saveAudioSettingsToBackend = (settings: AudioSettings) => {
    if (saveSettingsTimeout) {
        clearTimeout(saveSettingsTimeout);
    }
    saveSettingsTimeout = setTimeout(async () => {
        try {
            await api.saveAudioSettings(settings);
            console.log('💾 Audio settings saved to backend');
        } catch (e) {
            console.warn('Failed to save audio settings to backend:', e);
        }
    }, 500); // 500ms debounce
};

// Initial streaming stats
const initialStreamingStats: StreamingStats = {
    totalStreams: 0,
    successfulStreams: 0,
    failedStreams: 0,
    totalBufferingTime: 0,
    bufferingEvents: 0,
    averageBufferingDuration: 0,
    errorsByType: {
        network: 0,
        auth: 0,
        unavailable: 0,
        unknown: 0
    },
    lastStreamedTrack: null,
    sessionStartTime: Date.now()
};

// Default Milkdrop settings
const defaultMilkdropSettings: MilkdropSettings = {
    enabled: false,
    currentPreset: null,
    presetCycleEnabled: true,
    presetCycleInterval: 30, // seconds
    blendDuration: 2.7, // seconds (classic Milkdrop default)
    quality: 'medium',
    favoritePresets: []
};

export const createPlayerSlice: StateCreator<AppState, [], [], PlayerSlice> = (set, get) => ({
    isPlaying: false,
    currentSong: null,
    currentSongIndex: -1,
    queue: [],
    volume: 0.8,
    audioSettings: {
        crossfadeDuration: 0,
        gapless: false,
        normalization: false,
        visualizerMode: 'SPECTRUM',
        visualizerEnabled: true,
        visualizerArtworkOpacity: 30,         // Show artwork at 30% when visualizer active
        visualizerFullscreenEnabled: false,   // Fullscreen background visualizer off by default
        visualizerFullscreenOpacity: 20,      // Fullscreen background at 20% opacity
        visualizerBackgroundMode: 'AURORA_RIBBON', // Background visualizer mode (can differ from album art)
        eqEnabled: false,
        eqBands: [0, 0, 0, 0, 0, 0, 0, 0, 0, 0],
        activePresetId: 'flat',
        mainOutputDevice: '',        // Empty = default device
        headphoneOutputDevice: '',   // Empty = default device (same as main)
    },
    isEqOpen: false,
    
    // Milkdrop visualization state
    milkdropSettings: { ...defaultMilkdropSettings },
    milkdropPresetKeys: [],
    
    // Buffering state for streaming
    isBuffering: false,
    bufferProgress: 0,
    preloadedTrackId: null,
    
    // Streaming error state
    streamError: null,
    retryCount: 0,
    
    // Streaming analytics
    streamingStats: { ...initialStreamingStats },

    playSong: async (song, context) => {
        // 1. Resolve URL if missing (e.g. after page reload)
        let playableSong = { ...song };

        // Check if this is a Spotify streaming song (has spotifyId but no local file)
        if (playableSong.spotifyId && (!playableSong.url || playableSong.isStreaming)) {
            // Get streaming preferences
            const { streamingEnabled, streamingQuality, preferLocalPlayback } = get();
            
            // If preferLocalPlayback is enabled, check for downloaded version first
            if (preferLocalPlayback) {
                const downloadedSong = await libraryService.getSongBySpotifyId(playableSong.spotifyId);
                
                if (downloadedSong && downloadedSong.fileHandle) {
                    // Use downloaded version instead of streaming
                    console.log('[Player] Found downloaded version, using local file:', downloadedSong.title);
                    playableSong = { ...downloadedSong };
                    playableSong.isStreaming = false;
                    
                    // Resolve file handle to blob URL
                    try {
                        const permitted = await libraryService.verifyPermission(downloadedSong.fileHandle);
                        if (permitted) {
                            const file = await downloadedSong.fileHandle.getFile();
                            playableSong.url = URL.createObjectURL(file);
                        } else {
                            console.warn('[Player] Permission denied for downloaded file, falling back to streaming');
                            // Check if streaming is enabled before falling back
                            if (streamingEnabled) {
                                const { api } = await import('../services/api');
                                playableSong.url = api.getSpotifyStreamUrl(playableSong.spotifyId!, streamingQuality);
                                playableSong.isStreaming = true;
                            } else {
                                console.warn('[Player] Streaming disabled, cannot play track');
                                return;
                            }
                        }
                    } catch (e) {
                        console.error('[Player] Failed to read downloaded file, falling back to streaming:', e);
                        // Check if streaming is enabled before falling back
                        if (streamingEnabled) {
                            const { api } = await import('../services/api');
                            playableSong.url = api.getSpotifyStreamUrl(playableSong.spotifyId!, streamingQuality);
                            playableSong.isStreaming = true;
                        } else {
                            console.warn('[Player] Streaming disabled, cannot play track');
                            return;
                        }
                    }
                } else {
                    // No downloaded version - use streaming if enabled
                    if (!streamingEnabled) {
                        console.warn('[Player] Streaming disabled, cannot play Spotify track without download');
                        return;
                    }
                    const { api } = await import('../services/api');
                    playableSong.url = api.getSpotifyStreamUrl(playableSong.spotifyId, streamingQuality);
                    playableSong.isStreaming = true;
                    console.log('[Player] Using Spotify stream URL:', playableSong.url);
                }
            } else {
                // preferLocalPlayback is false - always stream if enabled
                if (!streamingEnabled) {
                    console.warn('[Player] Streaming disabled, cannot play Spotify track');
                    return;
                }
                const { api } = await import('../services/api');
                playableSong.url = api.getSpotifyStreamUrl(playableSong.spotifyId, streamingQuality);
                playableSong.isStreaming = true;
                console.log('[Player] Streaming preferred, using Spotify stream URL:', playableSong.url);
            }
        }
        // Check if we need to regenerate Blob URL from File Handle
        else if ((!playableSong.url || playableSong.url.startsWith('blob:') === false) && playableSong.fileHandle) {
            try {
                // Verify permission or request it (browser might show prompt)
                const permitted = await libraryService.verifyPermission(playableSong.fileHandle);
                if (permitted) {
                    const file = await playableSong.fileHandle.getFile();
                    playableSong.url = URL.createObjectURL(file);
                    // We don't save this ephemeral URL to DB, just to state
                } else {
                    console.warn("Permission denied for file handle");
                    // Fallback or error handling
                }
            } catch (e) {
                console.error("Failed to resolve file handle for playback", e);
            }
        }

        const { songs } = get();
        const newQueue = context ? [...context] : [...songs];

        // Use ID to match, ensuring we map to the possibly updated playableSong object
        const index = newQueue.findIndex(s => s.id === song.id);
        const validIndex = index !== -1 ? index : 0;

        // If context was passed, we might need to update the object in queue if it was stale
        if (index !== -1) {
            newQueue[index] = playableSong;
        }
        const finalQueue = index !== -1 ? newQueue : [playableSong];

        // Simple Auto-EQ Logic based on Genre
        if (playableSong.genre && playableSong.genre.length > 0) {
            const genre = playableSong.genre[0].toLowerCase();
            let presetId = 'flat';
            if (genre.includes('rock') || genre.includes('metal')) presetId = 'rock';
            else if (genre.includes('pop')) presetId = 'pop';
            else if (genre.includes('jazz')) presetId = 'jazz';
            else if (genre.includes('classical')) presetId = 'classical';
            else if (genre.includes('electronic') || genre.includes('dance')) presetId = 'electronic';

            if (presetId !== 'flat') {
                const preset = EQ_PRESETS.find(p => p.id === presetId);
                if (preset) {
                    set(state => ({
                        audioSettings: {
                            ...state.audioSettings,
                            activePresetId: presetId,
                            eqBands: [...preset.gains]
                        }
                    }));
                }
            }
        }

        set({
            currentSong: playableSong,
            currentSongIndex: validIndex,
            queue: finalQueue,
            isPlaying: true
        });
    },

    togglePlay: () => set((state) => ({ isPlaying: !state.isPlaying })),

    nextSong: () => {
        const { queue, currentSongIndex, playSong } = get();
        if (queue.length === 0) return;

        if (currentSongIndex >= queue.length - 1) {
            set({ isPlaying: false });
            return;
        }

        const nextIndex = currentSongIndex + 1;
        // We call playSong to handle the file handle resolution for the next track
        playSong(queue[nextIndex], queue);
    },

    prevSong: () => {
        const { queue, currentSongIndex, playSong } = get();
        if (queue.length === 0) return;

        const prevIndex = (currentSongIndex - 1 + queue.length) % queue.length;
        playSong(queue[prevIndex], queue);
    },

    setVolume: (volume) => set({ volume }),

    addToQueue: (items) => {
        const { queue, currentSong } = get();
        const itemsArr = Array.isArray(items) ? items : [items];
        const newQueue = [...queue, ...itemsArr];

        if (!currentSong && itemsArr.length > 0) {
            // Play first item using playSong to resolve handle
            get().playSong(itemsArr[0], newQueue);
        } else {
            set({ queue: newQueue });
        }
    },

    playNext: (items) => {
        const { queue, currentSongIndex } = get();
        const itemsArr = Array.isArray(items) ? items : [items];
        const newQueue = [...queue];
        newQueue.splice(currentSongIndex + 1, 0, ...itemsArr);
        set({ queue: newQueue });
    },

    removeFromQueue: (index) => {
        const { queue, currentSongIndex } = get();
        const newQueue = [...queue];
        newQueue.splice(index, 1);

        let newIndex = currentSongIndex;
        if (index < currentSongIndex) {
            newIndex = Math.max(0, currentSongIndex - 1);
        }
        else if (index === currentSongIndex) {
            newIndex = index < newQueue.length ? index : Math.max(0, newQueue.length - 1);
        }

        set({
            queue: newQueue,
            currentSongIndex: newIndex,
            currentSong: newQueue.length > 0 ? newQueue[newIndex] : null,
            isPlaying: newQueue.length > 0 ? get().isPlaying : false
        });
    },

    clearQueue: () => {
        const { currentSong } = get();
        if (currentSong) {
            set({ queue: [currentSong], currentSongIndex: 0 });
        } else {
            set({ queue: [], currentSongIndex: -1, isPlaying: false });
        }
    },

    reorderQueue: (fromIndex, toIndex) => {
        const { queue, currentSongIndex } = get();
        const newQueue = [...queue];
        const [movedItem] = newQueue.splice(fromIndex, 1);
        newQueue.splice(toIndex, 0, movedItem);

        let newCurrentIndex = currentSongIndex;
        if (currentSongIndex === fromIndex) {
            newCurrentIndex = toIndex;
        } else if (currentSongIndex > fromIndex && currentSongIndex <= toIndex) {
            newCurrentIndex--;
        } else if (currentSongIndex < fromIndex && currentSongIndex >= toIndex) {
            newCurrentIndex++;
        }

        set({ queue: newQueue, currentSongIndex: newCurrentIndex });
    },

    playQueueItem: (index) => {
        const { queue, playSong } = get();
        if (index >= 0 && index < queue.length) {
            playSong(queue[index], queue);
        }
    },

    setVisualizerMode: (mode) => {
        set((state) => {
            const newSettings = { ...state.audioSettings, visualizerMode: mode };
            saveAudioSettingsToBackend(newSettings);
            return { audioSettings: newSettings };
        });
    },
    setVisualizerBackgroundMode: (mode) => {
        set((state) => {
            const newSettings = { ...state.audioSettings, visualizerBackgroundMode: mode };
            saveAudioSettingsToBackend(newSettings);
            return { audioSettings: newSettings };
        });
    },
    setVisualizerArtworkOpacity: (opacity) => {
        set((state) => {
            const newSettings = { ...state.audioSettings, visualizerArtworkOpacity: opacity };
            saveAudioSettingsToBackend(newSettings);
            return { audioSettings: newSettings };
        });
    },
    setVisualizerFullscreenEnabled: (enabled) => {
        set((state) => {
            const newSettings = { ...state.audioSettings, visualizerFullscreenEnabled: enabled };
            saveAudioSettingsToBackend(newSettings);
            return { audioSettings: newSettings };
        });
    },
    setVisualizerFullscreenOpacity: (opacity) => {
        set((state) => {
            const newSettings = { ...state.audioSettings, visualizerFullscreenOpacity: opacity };
            saveAudioSettingsToBackend(newSettings);
            return { audioSettings: newSettings };
        });
    },
    setEqEnabled: (enabled) => {
        set((state) => {
            const newSettings = { ...state.audioSettings, eqEnabled: enabled };
            saveAudioSettingsToBackend(newSettings);
            return { audioSettings: newSettings };
        });
    },
    setEqBand: (index, gain) => set((state) => {
        const newBands = [...state.audioSettings.eqBands];
        newBands[index] = gain;
        const newSettings = {
            ...state.audioSettings,
            eqBands: newBands,
            activePresetId: 'custom'
        };
        saveAudioSettingsToBackend(newSettings);
        return { audioSettings: newSettings };
    }),
    setEqPreset: (presetId) => set((state) => {
        const preset = EQ_PRESETS.find(p => p.id === presetId);
        if (!preset) return state;
        const newSettings = {
            ...state.audioSettings,
            activePresetId: presetId,
            eqBands: [...preset.gains]
        };
        saveAudioSettingsToBackend(newSettings);
        return { audioSettings: newSettings };
    }),
    setCrossfade: (val) => {
        set((state) => {
            const newSettings = { ...state.audioSettings, crossfadeDuration: val };
            saveAudioSettingsToBackend(newSettings);
            return { audioSettings: newSettings };
        });
    },
    setGapless: (val) => {
        set((state) => {
            const newSettings = { ...state.audioSettings, gapless: val };
            saveAudioSettingsToBackend(newSettings);
            return { audioSettings: newSettings };
        });
    },
    setNormalization: (val) => {
        set((state) => {
            const newSettings = { ...state.audioSettings, normalization: val };
            saveAudioSettingsToBackend(newSettings);
            return { audioSettings: newSettings };
        });
    },
    setMainOutputDevice: (deviceId) => {
        set((state) => {
            const newSettings = { ...state.audioSettings, mainOutputDevice: deviceId };
            saveAudioSettingsToBackend(newSettings);
            return { audioSettings: newSettings };
        });
    },
    setHeadphoneOutputDevice: (deviceId) => {
        set((state) => {
            const newSettings = { ...state.audioSettings, headphoneOutputDevice: deviceId };
            saveAudioSettingsToBackend(newSettings);
            return { audioSettings: newSettings };
        });
    },
    toggleEqPanel: () => set((state) => ({ isEqOpen: !state.isEqOpen })),
    
    // Milkdrop visualization actions
    setMilkdropSettings: (settings) => set((state) => ({
        milkdropSettings: { ...state.milkdropSettings, ...settings }
    })),
    
    setMilkdropPreset: (preset) => set((state) => ({
        milkdropSettings: { ...state.milkdropSettings, currentPreset: preset }
    })),
    
    toggleMilkdropFavorite: (preset) => set((state) => {
        const favorites = state.milkdropSettings.favoritePresets;
        const isFavorite = favorites.includes(preset);
        return {
            milkdropSettings: {
                ...state.milkdropSettings,
                favoritePresets: isFavorite
                    ? favorites.filter(p => p !== preset)
                    : [...favorites, preset]
            }
        };
    }),
    
    setMilkdropPresetKeys: (keys) => set({ milkdropPresetKeys: keys }),
    
    // Load audio settings from backend database on startup
    loadAudioSettings: async () => {
        try {
            const backendSettings = await api.getAudioSettings();
            if (backendSettings) {
                console.log('🔊 Loaded audio settings from backend:', backendSettings);
                set((state) => ({
                    audioSettings: {
                        ...state.audioSettings,
                        ...backendSettings
                    }
                }));
            } else {
                console.log('🔊 No audio settings in backend, using defaults');
            }
        } catch (e) {
            console.warn('Failed to load audio settings from backend:', e);
        }
    },
    
    // Buffering actions
    setBuffering: (isBuffering) => set({ isBuffering }),
    setBufferProgress: (bufferProgress) => set({ bufferProgress }),
    setPreloadedTrackId: (preloadedTrackId) => set({ preloadedTrackId }),
    
    // Pre-buffer the next track in queue for smoother playback
    preloadNextTrack: async () => {
        const { queue, currentSongIndex, preloadedTrackId, streamingQuality } = get();
        
        // Check if there's a next track
        if (currentSongIndex >= queue.length - 1) {
            return; // No next track to preload
        }
        
        const nextTrack = queue[currentSongIndex + 1];
        
        // Skip if already preloaded
        if (preloadedTrackId === nextTrack.id) {
            return;
        }
        
        // Only preload Spotify streaming tracks (local files don't need it)
        if (!nextTrack.spotifyId || !nextTrack.isStreaming) {
            return;
        }
        
        console.log('[Player] Pre-buffering next track:', nextTrack.title);
        
        try {
            // Create a hidden audio element to pre-buffer
            const preloadAudio = new Audio();
            const { api } = await import('../services/api');
            const streamUrl = api.getSpotifyStreamUrl(nextTrack.spotifyId, streamingQuality);
            
            preloadAudio.preload = 'auto';
            preloadAudio.src = streamUrl;
            
            // Load without playing - just buffer the data
            preloadAudio.load();
            
            // Wait for enough data to be buffered (canplaythrough event)
            await new Promise<void>((resolve, reject) => {
                const timeout = setTimeout(() => {
                    preloadAudio.src = ''; // Cleanup
                    reject(new Error('Preload timeout'));
                }, 30000); // 30 second timeout
                
                preloadAudio.addEventListener('canplaythrough', () => {
                    clearTimeout(timeout);
                    // Release the Audio element to prevent memory leak
                    preloadAudio.src = '';
                    preloadAudio.removeAttribute('src');
                    resolve();
                }, { once: true });
                
                preloadAudio.addEventListener('error', (e) => {
                    clearTimeout(timeout);
                    preloadAudio.src = '';
                    preloadAudio.removeAttribute('src');
                    reject(e);
                }, { once: true });
            });
            
            console.log('[Player] Next track pre-buffered successfully:', nextTrack.title);
            set({ preloadedTrackId: nextTrack.id });
            
        } catch (error) {
            console.warn('[Player] Failed to pre-buffer next track:', error);
            // Non-fatal - playback will still work, just might buffer when switching
        }
    },
    
    // Error recovery actions
    setStreamError: (error: StreamingError | null) => {
        if (error) {
            console.error('[Player] Stream error:', error.type, error.message);
        }
        set({ streamError: error });
    },
    
    clearStreamError: () => {
        set({ streamError: null, retryCount: 0 });
    },
    
    retryStream: async () => {
        const { currentSong, retryCount, playSong, queue, showToast, recordStreamEvent } = get();
        
        if (!currentSong) {
            console.warn('[Player] No current song to retry');
            return;
        }
        
        if (retryCount >= MAX_RETRY_ATTEMPTS) {
            console.warn('[Player] Max retry attempts reached');
            showToast({
                type: 'error',
                message: 'Unable to play track after multiple attempts. Try downloading it instead.',
                duration: 5000
            });
            set({ streamError: null, retryCount: 0 });
            return;
        }
        
        // Record retry event
        recordStreamEvent({
            type: 'retry',
            trackId: currentSong.spotifyId || currentSong.id,
            trackTitle: currentSong.title,
            timestamp: Date.now()
        });
        
        // Calculate delay with exponential backoff
        const delay = BASE_RETRY_DELAY * Math.pow(2, retryCount);
        console.log(`[Player] Retrying stream in ${delay}ms (attempt ${retryCount + 1}/${MAX_RETRY_ATTEMPTS})`);
        
        set({ retryCount: retryCount + 1, streamError: null, isBuffering: true });
        
        // Wait before retrying
        await new Promise(resolve => setTimeout(resolve, delay));
        
        // Retry playback
        try {
            await playSong(currentSong, queue);
            console.log('[Player] Stream retry successful');
            set({ retryCount: 0 });
        } catch (error) {
            console.error('[Player] Stream retry failed:', error);
            // The error handler in useAudioPlayer will handle this
        }
    },
    
    // Streaming analytics actions
    recordStreamEvent: (event: StreamingEvent) => {
        const { streamingStats, addLog } = get();
        const newStats = { ...streamingStats };
        
        // Log the event for debugging visibility
        const logLevel = event.type === 'error' ? 'error' : 
                         event.type === 'buffer_start' ? 'warn' : 'info';
        const logMessage = `[Streaming] ${event.type.toUpperCase()}: ${event.trackTitle || 'Unknown track'}`;
        addLog(logLevel, logMessage, event);
        
        console.log(`[StreamingAnalytics] ${event.type}:`, event);
        
        switch (event.type) {
            case 'start':
                newStats.totalStreams++;
                newStats.lastStreamedTrack = event.trackTitle || null;
                break;
                
            case 'complete':
                newStats.successfulStreams++;
                break;
                
            case 'error':
                newStats.failedStreams++;
                if (event.errorType) {
                    newStats.errorsByType[event.errorType]++;
                }
                break;
                
            case 'buffer_start':
                newStats.bufferingEvents++;
                break;
                
            case 'buffer_end':
                if (event.duration) {
                    const durationSecs = event.duration / 1000;
                    newStats.totalBufferingTime += durationSecs;
                    newStats.averageBufferingDuration = 
                        newStats.bufferingEvents > 0 
                            ? newStats.totalBufferingTime / newStats.bufferingEvents 
                            : 0;
                }
                break;
                
            case 'skip':
                // Track skips (e.g., when unavailable track is auto-skipped)
                break;
                
            case 'retry':
                // Retries are already counted in error handling
                break;
        }
        
        set({ streamingStats: newStats });
    },
    
    resetStreamingStats: () => {
        set({ streamingStats: { ...initialStreamingStats, sessionStartTime: Date.now() } });
    },
});