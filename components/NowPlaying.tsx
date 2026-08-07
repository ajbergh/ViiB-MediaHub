/**
 * ViiB MediaHub - Now Playing View Component
 * 
 * Full-screen immersive view for the currently playing track with comprehensive controls and visualizations.
 * 
 * Architecture:
 * - Fixed full-screen overlay (z-index 100) with animated entrance
 * - Dynamic gradient background generated from album artwork colors
 * - Album art container with audio-reactive visualizer overlay
 * - Bottom control panel with player controls and metadata
 * 
 * Features:
 * 
 * Visual Elements:
 * - Large square album artwork (60vh) with aspect ratio preservation
 * - Dynamic gradient background blurred and faded from album colors
 * - Audio-reactive visualizer overlay with 21 modes (see AlbumArtVisualizer)
 * - Visualizer cycle button with active state indication
 * - Album art download functionality with progress indication
 * 
 * Playback Controls:
 * - Play/Pause toggle with icon transition
 * - Previous/Next track navigation
 * - Shuffle toggle (saved in player state)
 * - Repeat mode toggle (off → all → one)
 * - Seek bar with draggable progress indicator
 * - Volume slider with icon indication
 * 
 * Visualizer System:
 * - 22 visualization modes: OFF, WAVE, SPECTRUM, AURORA, CIRCULAR, PARTICLES, NEBULA,
 *   FLAME_SPECTRUM, STARDUST_HALO, AURORA_RIBBON, ELECTRIC_ARC, GRASS_OSCILLOSCOPE,
 *   CRYSTAL_SHARDS, WATERCOLOR_BLOOM, ICE_FRACTURE, FIREFLY_FIELD, VINYL_SPIN,
 *   BEAT_ORBS, TUNNEL_WAVEFORM, GLASS_SHARDS, WIND_FIELD, MILKDROP
 * - MILKDROP mode uses WebGL Butterchurn library for classic Winamp-style visualizations
 * - Cycle through modes via Activity button (bottom-left controls)
 * - Real-time audio analysis via Web Audio API
 * - Smooth fade transitions between modes
 * 
 * Additional Views:
 * - Lyrics View: Shows synchronized lyrics (when available)
 * - Queue View: Mini queue list showing upcoming tracks
 * - Both views accessible via bottom-right buttons
 * 
 * Interaction:
 * - Activated by clicking album art in the main player bar
 * - Close via X button (top-right) or Escape key
 * - Seek by clicking/dragging on progress bar
 * - Download album art via download button (saves to filesystem)
 * 
 * State Management:
 * - Uses Zustand store for player state (isPlaying, volume, shuffle, repeat)
 * - Local state for view toggles (lyrics, queue, download status)
 * - Album cover URLs fetched via useAlbumCovers hook
 * 
 * Performance:
 * - Visualizer paused when view is closed
 * - Album art lazy-loaded with fallback
 * - Gradient memoized per album
 * - Smooth animations via Tailwind CSS classes
 * 
 * @module NowPlaying
 * @requires AlbumArtVisualizer - Audio-reactive visualization renderer
 * @requires LyricsView - Synchronized lyrics display component
 * @requires QueueList - Mini queue preview component
 */

import React, { useState, useEffect, useCallback } from 'react';
import { useStore, useAlbumCovers } from '../store';
import { X, Play, Pause, SkipBack, SkipForward, Shuffle, ListMusic, Activity, SlidersHorizontal, Volume2, Download, Loader2, CheckCircle, Layers, Maximize2, Minimize2, Info, Sparkles, FileText, Tag, Calendar, BarChart3, Clock, Radio, Disc, Mic2 } from 'lucide-react';
import { formatTime, generateGradient, cssUrl, getAudioFormatInfo } from '../utils';
import { ContextMenuType, VisualizerMode } from '../types';
import { api } from '../services/api';
import { Visualizer } from './Visualizer';
import { LyricsView } from './now-playing/LyricsView';
import { QueueList } from './now-playing/QueueList';
import { WebGLVisualizer } from './now-playing/webgl';
import { MilkdropVisualizer } from './now-playing/MilkdropVisualizer';
import { VisualizerSelector } from './now-playing/VisualizerSelector';
import { LikeButton } from './LikeButton';
import { Button } from './ui/Button';
import { VIIB_COLOR_VALUES } from './ui/tokens';

interface Props {
    currentTime: number;
    duration: number;
    onSeek: (time: number) => void;
}

export const NowPlaying: React.FC<Props> = ({ currentTime, duration, onSeek }) => {
    const { 
        currentSong, 
        isPlaying, 
        togglePlay, 
        nextSong, 
        prevSong, 
        volume, 
        setVolume, 
        setNowPlayingOpen,
        queue,
        currentSongIndex,
        openContextMenu,
        audioSettings,
        setVisualizerMode,
        toggleEqPanel,
        showToast,
        // Milkdrop state
        milkdropSettings,
        setMilkdropPreset,
        setMilkdropPresetKeys,
        // Party mode state
        isPartyMode,
        togglePartyMode,
        setPartyMode,
        openSongInfoModal
    } = useStore();
    
    const albumCovers = useAlbumCovers();
    const [activeTab, setActiveTab] = useState<'QUEUE' | 'LYRICS' | 'INFO'>('LYRICS');
    const [isDownloading, setIsDownloading] = useState(false);
    const [showVisualizerOverlay, setShowVisualizerOverlay] = useState(false);
    const [showVisualizerSelector, setShowVisualizerSelector] = useState(false);
    const [showPartyControls, setShowPartyControls] = useState(false);
    
    // Handle escape key to exit party mode or close now playing
    useEffect(() => {
        const handleKeyDown = (e: KeyboardEvent) => {
            if (e.key === 'Escape') {
                if (isPartyMode) {
                    setPartyMode(false);
                } else {
                    setNowPlayingOpen(false);
                }
            }
        };
        
        window.addEventListener('keydown', handleKeyDown);
        return () => window.removeEventListener('keydown', handleKeyDown);
    }, [isPartyMode, setPartyMode, setNowPlayingOpen]);
    
    // Auto-hide controls in party mode after inactivity
    useEffect(() => {
        if (!isPartyMode) return;
        
        let timeout: NodeJS.Timeout;
        
        const showControls = () => {
            setShowPartyControls(true);
            clearTimeout(timeout);
            timeout = setTimeout(() => setShowPartyControls(false), 3000);
        };
        
        window.addEventListener('mousemove', showControls);
        window.addEventListener('click', showControls);
        
        // Show initially then fade
        showControls();
        
        return () => {
            window.removeEventListener('mousemove', showControls);
            window.removeEventListener('click', showControls);
            clearTimeout(timeout);
        };
    }, [isPartyMode]);

    if (!currentSong) return null;

    const coverUrl = currentSong.coverUrl || albumCovers[currentSong.album];
    
    // Check if current track is a streaming Spotify track (not downloaded)
    const isSpotifyStreaming = currentSong.spotifyId && currentSong.isStreaming;
    
    const handleDownloadTrack = async () => {
        if (!currentSong.spotifyId || isDownloading) return;
        
        setIsDownloading(true);
        try {
            await api.downloadTrack(
                currentSong.spotifyId,
                currentSong.title,
                currentSong.artist,
                currentSong.album,
                currentSong.duration
            );
            showToast({ type: 'success', message: `Queued for download: ${currentSong.title}` });
        } catch (error) {
            console.error('Failed to queue download:', error);
            showToast({ type: 'error', message: 'Failed to queue download' });
        } finally {
            setIsDownloading(false);
        }
    };
    
    /**
     * Cycles through all available visualizer modes in order.
     * 
     * Order: OFF → Classic modes (6) → Next-gen modes (14) → back to OFF
     * 
     * Classic Modes:
     * - WAVE: Smooth waveform
     * - SPECTRUM: Circular frequency bars
     * - AURORA: Ambient gradients
     * - CIRCULAR: Rotating bars with pulse
     * - PARTICLES: Dynamic particle system
     * - NEBULA: Cosmic nebula clouds
     * 
     * Next-Gen Modes:
     * - FLAME_SPECTRUM: Rising flame tongues
     * - STARDUST_HALO: Pulsing particle ring
     * - AURORA_RIBBON: Flowing ribbon
     * - ELECTRIC_ARC: TRON light beams
     * - GRASS_OSCILLOSCOPE: Swaying grass blades
     * - CRYSTAL_SHARDS: Bursting prisms
     * - WATERCOLOR_BLOOM: Painterly blooms
     * - ICE_FRACTURE: Cracking ice
     * - FIREFLY_FIELD: Drifting fireflies
     * - VINYL_SPIN: Rotating grooves
     * - BEAT_ORBS: Expanding orbs
     * - TUNNEL_WAVEFORM: 3D ring tunnel
     * - GLASS_SHARDS: Reflective fragments
     * - WIND_FIELD: Flowing particles
     * - MILKDROP: WebGL Milkdrop/Butterchurn visualization
     * 
     * Triggered by clicking the Activity button in the Now Playing view.
     * Updates global audio settings via Zustand store.
     */
    const cycleVisualizer = () => {
        const modes: VisualizerMode[] = [
            'OFF', 
            'WAVE', 
            'SPECTRUM', 
            'FLAME_SPECTRUM',
            'STARDUST_HALO',
            'AURORA_RIBBON',
            'ELECTRIC_ARC',
            'GRASS_OSCILLOSCOPE',
            'FIREFLY_FIELD',
            'TUNNEL_WAVEFORM',
            'WIND_FIELD',
            'MILKDROP'
        ];
        const currentIdx = modes.indexOf(audioSettings.visualizerMode);
        const nextIdx = (currentIdx + 1) % modes.length;
        setVisualizerMode(modes[nextIdx]);
    };

    return (
        <div className="fixed inset-0 z-[100] bg-surface-0 text-text-main flex flex-col overflow-hidden animate-in fade-in slide-in-from-bottom-10 duration-300 motion-reduce:animate-none motion-reduce:transition-none">
            {/* Dynamic Background */}
            <div 
                className="absolute inset-0 z-0 opacity-30 blur-3xl scale-110 pointer-events-none transition-all duration-1000 motion-reduce:transition-none"
                style={{ 
                    background: coverUrl ? `${cssUrl(coverUrl)} center/cover` : generateGradient(currentSong.album),
                }}
            />
            
            {/* Visualizer Layer */}
            <div className="absolute inset-0 z-0 opacity-40">
                <Visualizer mode={audioSettings.visualizerMode} barColor={VIIB_COLOR_VALUES.visualizerMuted} />
            </div>

            {/* Fullscreen Background Visualizer - Behind all UI when enabled */}
            {/* Uses separate visualizerBackgroundMode so it can differ from album art overlay */}
            {audioSettings.visualizerFullscreenEnabled && audioSettings.visualizerBackgroundMode !== 'OFF' && isPlaying && (
                <>
                    {audioSettings.visualizerBackgroundMode === 'MILKDROP' ? (
                        <MilkdropVisualizer
                            settings={milkdropSettings}
                            isActive={true}
                            onPresetChange={setMilkdropPreset}
                            onPresetsLoaded={setMilkdropPresetKeys}
                            className="z-[1] pointer-events-none"
                            style={{ opacity: (audioSettings.visualizerFullscreenOpacity ?? 20) / 100 }}
                        />
                    ) : (
                        <div 
                            className="absolute inset-0 z-[1] pointer-events-none"
                            style={{ opacity: (audioSettings.visualizerFullscreenOpacity ?? 20) / 100 }}
                        >
                            <WebGLVisualizer 
                                mode={audioSettings.visualizerBackgroundMode}
                                isActive={true}
                            />
                        </div>
                    )}
                </>
            )}

            <div className="absolute inset-0 z-0 bg-surface-0/70 backdrop-blur-[60px] pointer-events-none"></div>

            {/* Header - Hidden in party mode unless mouse active */}
            <div className={`relative z-10 flex items-center justify-between p-6 md:p-8 transition-opacity duration-300 ${
                isPartyMode ? (showPartyControls ? 'opacity-100' : 'opacity-0') : 'opacity-100'
            }`}>
                <Button
                    onClick={() => isPartyMode ? setPartyMode(false) : setNowPlayingOpen(false)}
                    variant="ghost"
                    className="rounded-full p-2 bg-surface-1/40 hover:bg-surface-1/60 backdrop-blur-md"
                    aria-label={isPartyMode ? "Exit party mode" : "Close now playing"}
                >
                    {isPartyMode ? <Minimize2 size={24} /> : <X size={24} />}
                </Button>
                <div className="flex flex-col items-center">
                    <span className="text-xs font-bold uppercase tracking-widest text-text-secondary">
                        {isPartyMode ? 'Party Mode' : 'Now Playing'}
                    </span>
                    <span 
                        className="text-sm font-semibold truncate max-w-[200px]"
                        onContextMenu={(e) => openContextMenu(e, ContextMenuType.ALBUM, { name: currentSong.album, artist: currentSong.artist })}
                    >
                        {currentSong.album}
                    </span>
                </div>
                <div className="flex items-center gap-2">
                    <Button
                        onClick={togglePartyMode}
                        variant="ghost"
                        className={`rounded-full p-2 backdrop-blur-md ${
                            isPartyMode 
                                ? 'text-amber-400 bg-amber-400/20 ring-1 ring-amber-400/30' 
                                : 'text-text-secondary bg-surface-1/40 hover:text-amber-400 hover:bg-surface-1/60'
                        }`}
                        title={isPartyMode ? "Exit Party Mode" : "Enter Party Mode"}
                        aria-label={isPartyMode ? "Exit party mode" : "Enter party mode"}
                    >
                        <Maximize2 size={24} />
                    </Button>
                    <Button
                        onClick={() => setShowVisualizerSelector(true)}
                        variant="ghost"
                        className={`rounded-full p-2 backdrop-blur-md ${
                            audioSettings.visualizerMode !== 'OFF' || audioSettings.visualizerFullscreenEnabled 
                                ? 'text-brand bg-surface-1/50 ring-1 ring-brand/20' 
                                : 'text-text-secondary bg-surface-1/40 hover:text-text-main hover:bg-surface-1/60'
                        }`}
                        title="Visualizer Layers"
                        aria-label="Open visualizer layer selector"
                    >
                        <Layers size={24} />
                    </Button>
                    <Button
                        onClick={cycleVisualizer}
                        variant="ghost"
                        className={`rounded-full p-2 backdrop-blur-md ${audioSettings.visualizerMode !== 'OFF' ? 'text-accent-green bg-surface-1/50 ring-1 ring-accent-green/20' : 'text-text-secondary bg-surface-1/40 hover:text-text-main hover:bg-surface-1/60'}`}
                        title={`Visualizer: ${audioSettings.visualizerMode}`}
                        aria-label="Change visualizer mode"
                    >
                        <Activity size={24} />
                    </Button>
                    {!isPartyMode && (
                        <Button
                            onClick={() => setActiveTab('QUEUE')}
                            variant="ghost"
                            className={`rounded-full p-2 backdrop-blur-md ${activeTab === 'QUEUE' ? 'text-text-main bg-surface-1/60 ring-1 ring-text-main/10' : 'text-text-secondary bg-surface-1/40 hover:text-text-main hover:bg-surface-1/60'}`}
                            title="Queue"
                            aria-label="Show queue"
                        >
                            <ListMusic size={24} />
                        </Button>
                    )}
                </div>
            </div>

            {/* Main Content */}
            <div className={`relative z-10 flex-1 flex ${isPartyMode ? 'items-center justify-center' : 'flex-col md:flex-row gap-8 md:gap-16'} px-8 md:px-16 overflow-hidden`}>
                {/* Left Side: Artwork & Metadata - Full screen centered in party mode */}
                <div className={`flex flex-col ${isPartyMode ? 'items-center justify-center' : 'flex-1 justify-center max-w-2xl mx-auto w-full min-h-[400px]'}`}>
                    <div 
                        className={`aspect-square bg-surface-2 rounded-xl shadow-2xl relative group overflow-hidden cursor-pointer ring-1 ring-surface-3/60 ${
                            isPartyMode 
                                ? 'w-[70vh] max-w-[80vw] mb-8' 
                                : 'w-full max-w-[500px] mx-auto mb-8 md:mb-12'
                        }`}
                        onClick={() => setShowVisualizerOverlay(!showVisualizerOverlay)}
                        onContextMenu={(e) => openContextMenu(e, ContextMenuType.SONG, currentSong)}
                    >
                         {/* Album Art - opacity controlled by artwork setting when legacy visualizer is active */}
                         {coverUrl ? (
                            <img 
                                src={coverUrl} 
                                alt="Album Art" 
                                className="w-full h-full object-contain transition-opacity duration-300"
                                style={{
                                    // Legacy visualizers: album art dims, visualizer at full opacity
                                    // Milkdrop: album art stays full, visualizer dims
                                    opacity: showVisualizerOverlay && audioSettings.visualizerMode !== 'OFF' && audioSettings.visualizerMode !== 'MILKDROP' && isPlaying
                                        ? (audioSettings.visualizerArtworkOpacity ?? 30) / 100
                                        : 1
                                }}
                            />
                         ) : (
                             <div 
                                className="w-full h-full flex items-center justify-center text-display font-bold text-text-subtle/30 transition-opacity duration-300"
                                style={{ 
                                    background: generateGradient(currentSong.album),
                                    // Legacy visualizers: album art dims, visualizer at full opacity
                                    // Milkdrop: album art stays full, visualizer dims
                                    opacity: showVisualizerOverlay && audioSettings.visualizerMode !== 'OFF' && audioSettings.visualizerMode !== 'MILKDROP' && isPlaying
                                        ? (audioSettings.visualizerArtworkOpacity ?? 30) / 100
                                        : 1
                                }}
                            >
                                {currentSong.title.charAt(0)}
                            </div>
                         )}
                         
                         {/* Album Art Visualizer Overlay - WebGL modes with Canvas 2D fallback */}
                         {/* Legacy visualizers: render at full opacity over dimmed album art */}
                         {audioSettings.visualizerMode !== 'MILKDROP' && (
                             <div 
                                className="absolute inset-0 transition-opacity duration-300"
                                style={{ 
                                    opacity: showVisualizerOverlay && audioSettings.visualizerMode !== 'OFF' && isPlaying
                                        ? 1
                                        : 0
                                }}
                             >
                                <WebGLVisualizer 
                                    mode={audioSettings.visualizerMode}
                                    isActive={showVisualizerOverlay && audioSettings.visualizerMode !== 'OFF' && isPlaying}
                                />
                             </div>
                         )}
                         
                         {/* Milkdrop Visualizer Overlay - WebGL mode */}
                         {/* Milkdrop: visualizer opacity is reduced to let album art show through */}
                         {audioSettings.visualizerMode === 'MILKDROP' && showVisualizerOverlay && isPlaying && (
                             <MilkdropVisualizer
                                settings={milkdropSettings}
                                isActive={true}
                                onPresetChange={setMilkdropPreset}
                                onPresetsLoaded={setMilkdropPresetKeys}
                                className="transition-opacity duration-300"
                                style={{ opacity: 1 - ((audioSettings.visualizerArtworkOpacity ?? 30) / 100) }}
                             />
                         )}
                    </div>
                    
                    {/* Track info - centered in party mode */}
                    <div className={`flex ${isPartyMode ? 'flex-col items-center text-center' : 'items-end justify-between'} mb-2`}>
                        <div className={`flex flex-col min-w-0 ${isPartyMode ? 'items-center' : 'pr-4'}`}>
                            <h1 className={`font-bold truncate leading-tight mb-2 ${
                                isPartyMode ? 'text-display md:text-4xl max-w-[80vw]' : 'text-section md:text-display'
                            }`} title={currentSong.title}>
                                {currentSong.title}
                            </h1>
                            <h2 
                                className={`text-text-secondary font-medium truncate cursor-pointer hover:underline hover:text-text-main transition-colors ${
                                    isPartyMode ? 'text-section max-w-[80vw]' : 'text-card md:text-section'
                                }`}
                                onContextMenu={(e) => openContextMenu(e, ContextMenuType.ARTIST, { name: currentSong.artist })}
                            >
                                {currentSong.artist}
                            </h2>

                            {/* Inline Song Metadata Strip */}
                            {!isPartyMode && (
                                <div className="flex items-center gap-2 mt-2 flex-wrap">
                                    {/* Audio Format Badge */}
                                    {(() => {
                                        const fmt = getAudioFormatInfo(currentSong);
                                        return (
                                            <span className={`inline-flex items-center px-2 py-0.5 rounded-full text-[11px] font-mono font-bold ring-1 ${fmt.colorClass}`}>
                                                {fmt.label}
                                            </span>
                                        );
                                    })()}
                                    {/* ReplayGain Badge */}
                                    {(currentSong.replayGainDb !== undefined || audioSettings.normalization) && (
                                        <span
                                            className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-[11px] font-mono font-bold bg-accent-blue/20 text-accent-blue ring-1 ring-accent-blue/30"
                                            title={currentSong.replayGainDb !== undefined ? `ReplayGain Loudness: ${currentSong.replayGainDb > 0 ? '+' : ''}${currentSong.replayGainDb.toFixed(2)} dB` : 'Loudness Normalization Active'}
                                        >
                                            <Volume2 size={10} />
                                            {currentSong.replayGainDb !== undefined ? `${currentSong.replayGainDb > 0 ? '+' : ''}${currentSong.replayGainDb.toFixed(1)}dB` : 'RG'}
                                        </span>
                                    )}
                                    {currentSong.originalYear && (
                                        <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-[11px] font-semibold bg-amber-500/20 text-amber-300 ring-1 ring-amber-500/30">
                                            <Calendar size={10} />
                                            {currentSong.originalYear} Original
                                        </span>
                                    )}
                                    {currentSong.bpm && (
                                        <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-[11px] font-bold bg-accent-pink/20 text-accent-pink ring-1 ring-accent-pink/30">
                                            <Activity size={10} />
                                            {currentSong.bpm} BPM
                                        </span>
                                    )}
                                    {currentSong.mood && (
                                        <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-[11px] font-semibold bg-brand/20 text-brand ring-1 ring-brand/30 capitalize">
                                            <Sparkles size={10} />
                                            {currentSong.mood}
                                        </span>
                                    )}
                                    {currentSong.genre && currentSong.genre.length > 0 && (
                                        <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-[11px] font-medium bg-surface-2/80 text-text-secondary ring-1 ring-surface-border">
                                            <Tag size={10} />
                                            {currentSong.genre[0]}
                                        </span>
                                    )}
                                    <button
                                        type="button"
                                        onClick={() => openSongInfoModal(currentSong)}
                                        className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-[11px] font-medium bg-surface-2 hover:bg-surface-3 text-text-secondary hover:text-text-main ring-1 ring-surface-border transition-colors"
                                        title="View complete song metadata & properties"
                                    >
                                        <Info size={10} />
                                        Info
                                    </button>
                                </div>
                            )}
                        </div>
                        {!isPartyMode && (
                            <div className="flex items-center gap-3">
                                {/* Download button for streaming Spotify tracks */}
                                {isSpotifyStreaming && (
                                    <Button
                                        onClick={handleDownloadTrack}
                                        disabled={isDownloading}
                                        variant="ghost"
                                        className="rounded-full p-2 text-text-secondary hover:text-accent-green hover:bg-surface-1/40 hover:scale-110 transition-all disabled:opacity-50 disabled:cursor-not-allowed"
                                        title="Download for offline"
                                        aria-label="Download for offline"
                                    >
                                        {isDownloading ? (
                                            <Loader2 size={28} className="animate-spin" />
                                        ) : (
                                            <Download size={28} />
                                        )}
                                    </Button>
                                )}
                                {/* Show checkmark if track is downloaded (has spotifyId but not streaming) */}
                                {currentSong.spotifyId && !currentSong.isStreaming && (
                                    <div className="text-accent-green" title="Downloaded">
                                        <CheckCircle size={28} />
                                    </div>
                                )}
                                <LikeButton songId={currentSong.id} size={32} />
                            </div>
                        )}
                    </div>
                </div>

                {/* Right Side: Tabs - Hidden in party mode */}
                {!isPartyMode && (
                    <div className="flex-1 flex flex-col h-full min-h-[400px] max-w-xl mx-auto w-full bg-surface-1/30 rounded-t-2xl md:rounded-2xl ring-1 ring-surface-3/60 backdrop-blur-md overflow-hidden">
                        {/* Tabs Header */}
                        <div className="flex items-center border-b border-surface-3/60 p-2 gap-2">
                            <Button
                                onClick={() => setActiveTab('QUEUE')}
                                variant={activeTab === 'QUEUE' ? 'secondary' : 'ghost'}
                                className="flex-1 justify-center rounded-lg"
                            >
                                Queue
                            </Button>
                            <Button
                                onClick={() => setActiveTab('LYRICS')}
                                variant={activeTab === 'LYRICS' ? 'secondary' : 'ghost'}
                                className="flex-1 justify-center rounded-lg"
                            >
                                Lyrics
                            </Button>
                            <Button
                                onClick={() => setActiveTab('INFO')}
                                variant={activeTab === 'INFO' ? 'secondary' : 'ghost'}
                                className="flex-1 justify-center rounded-lg flex items-center gap-1.5"
                            >
                                <Info size={14} />
                                Track Info
                            </Button>
                        </div>

                        {/* Tab Content */}
                        <div className="flex-1 overflow-y-auto overflow-x-hidden p-6 relative">
                            {activeTab === 'LYRICS' && (
                                <div className="h-full flex flex-col">
                                    <LyricsView song={currentSong} currentTime={currentTime} onSeek={onSeek} />
                                </div>
                            )}

                            {activeTab === 'QUEUE' && (
                                <QueueList queue={queue} currentSongIndex={currentSongIndex} />
                            )}

                            {activeTab === 'INFO' && (
                                <div className="space-y-4 text-xs text-text-main animate-in fade-in duration-200">
                                    {/* Core Card */}
                                    <div className="p-4 rounded-xl bg-surface-2/60 border border-surface-border/60 space-y-2">
                                        <div className="flex items-center justify-between border-b border-surface-border/40 pb-2 mb-2">
                                            <span className="font-bold text-sm text-brand">{currentSong.title}</span>
                                            <Button
                                                variant="secondary"
                                                className="text-xs py-1 px-2.5"
                                                onClick={() => openSongInfoModal(currentSong)}
                                                leftIcon={<Info size={12} />}
                                            >
                                                Full Properties
                                            </Button>
                                        </div>
                                        <div className="grid grid-cols-2 gap-2 text-text-secondary">
                                            <div><span className="text-text-subtle font-medium">Artist:</span> {currentSong.artist}</div>
                                            <div><span className="text-text-subtle font-medium">Album:</span> {currentSong.album}</div>
                                            {currentSong.albumArtist && <div><span className="text-text-subtle font-medium">Album Artist:</span> {currentSong.albumArtist}</div>}
                                            {currentSong.year && <div><span className="text-text-subtle font-medium">Year:</span> {currentSong.year}</div>}
                                            {currentSong.originalYear && <div><span className="text-text-subtle font-medium">Original Release:</span> {currentSong.originalYear}</div>}
                                            <div><span className="text-text-subtle font-medium">Duration:</span> {formatTime(currentSong.duration)}</div>
                                        </div>
                                    </div>

                                    {/* AI & Vibe Profile */}
                                    <div className="p-4 rounded-xl bg-surface-2/60 border border-surface-border/60 space-y-2">
                                        <div className="flex items-center gap-1.5 font-semibold text-text-main text-xs mb-2">
                                            <Sparkles size={14} className="text-accent-pink" />
                                            <span>AI Vibe & Audio Profile</span>
                                        </div>
                                        <div className="grid grid-cols-2 sm:grid-cols-4 gap-2 text-center">
                                            <div className="p-2 rounded-lg bg-surface-3/60">
                                                <span className="text-[10px] text-text-subtle block uppercase">Mood</span>
                                                <span className="font-bold text-accent-pink capitalize">{currentSong.mood || 'N/A'}</span>
                                            </div>
                                            <div className="p-2 rounded-lg bg-surface-3/60">
                                                <span className="text-[10px] text-text-subtle block uppercase">Energy</span>
                                                <span className="font-bold text-amber-400 capitalize">{currentSong.energy || 'N/A'}</span>
                                            </div>
                                            <div className="p-2 rounded-lg bg-surface-3/60">
                                                <span className="text-[10px] text-text-subtle block uppercase">Tempo</span>
                                                <span className="font-bold text-accent-blue capitalize">{currentSong.tempo || 'N/A'}</span>
                                            </div>
                                            <div className="p-2 rounded-lg bg-surface-3/60">
                                                <span className="text-[10px] text-text-subtle block uppercase">BPM</span>
                                                <span className="font-bold text-accent-green font-mono">{currentSong.bpm || 'N/A'}</span>
                                            </div>
                                        </div>
                                        {currentSong.genre && currentSong.genre.length > 0 && (
                                            <div className="pt-2 flex items-center gap-1.5 flex-wrap">
                                                <span className="text-text-subtle">Genres:</span>
                                                {currentSong.genre.map((g, idx) => (
                                                    <span key={idx} className="px-2 py-0.5 rounded-full bg-surface-3 font-medium text-text-main text-[11px]">
                                                        {g}
                                                    </span>
                                                ))}
                                            </div>
                                        )}
                                    </div>

                                    {/* Listening Metrics */}
                                    <div className="p-4 rounded-xl bg-surface-2/60 border border-surface-border/60 space-y-2">
                                        <div className="flex items-center gap-1.5 font-semibold text-text-main text-xs mb-1">
                                            <BarChart3 size={14} className="text-brand" />
                                            <span>Listening Statistics</span>
                                        </div>
                                        <div className="grid grid-cols-3 gap-2 text-center">
                                            <div className="p-2 rounded-lg bg-surface-3/60">
                                                <span className="text-[10px] text-text-subtle block uppercase">Local Plays</span>
                                                <span className="font-bold font-mono text-accent-green">{currentSong.playCount || 0}</span>
                                            </div>
                                            <div className="p-2 rounded-lg bg-surface-3/60">
                                                <span className="text-[10px] text-text-subtle block uppercase">Skips</span>
                                                <span className="font-bold font-mono text-amber-400">{currentSong.skipCount || 0}</span>
                                            </div>
                                            <div className="p-2 rounded-lg bg-surface-3/60">
                                                <span className="text-[10px] text-text-subtle block uppercase">Last.fm Listeners</span>
                                                <span className="font-bold font-mono text-text-main">{currentSong.lastfmListeners ? currentSong.lastfmListeners.toLocaleString() : 'N/A'}</span>
                                            </div>
                                        </div>
                                    </div>
                                </div>
                            )}
                        </div>
                    </div>
                )}
            </div>

            {/* Bottom Controls - Hidden in party mode unless mouse active */}
            <div className={`relative z-10 px-8 py-8 md:px-16 bg-gradient-to-t from-surface-0 via-surface-0/80 to-transparent transition-opacity duration-300 ${
                isPartyMode ? (showPartyControls ? 'opacity-100' : 'opacity-0 pointer-events-none') : 'opacity-100'
            }`}>
                 <div className="max-w-4xl mx-auto w-full flex flex-col gap-6">
                     {/* Seek Bar */}
                    <div className="flex items-center gap-4 text-xs font-mono font-medium text-text-secondary">
                        <span className="w-10 text-right">{formatTime(currentTime)}</span>
                        <div className="flex-1 h-1.5 bg-surface-3/60 rounded-full relative group">
                            <div 
                                className="absolute top-0 left-0 h-full bg-accent-green rounded-full transition-all"
                                style={{ width: `${(currentTime / (duration || 1)) * 100}%` }}
                            >
                                <div className="absolute right-0 top-1/2 -translate-y-1/2 w-3 h-3 bg-accent-green rounded-full opacity-0 group-hover:opacity-100 shadow-lg scale-150 transition-all"></div>
                            </div>
                            <input 
                                type="range" 
                                min="0" 
                                max={duration || 100} 
                                value={currentTime}
                                onChange={(e) => onSeek(Number(e.target.value))}
                                className="absolute inset-0 w-full opacity-0 cursor-pointer"
                            />
                        </div>
                        <span className="w-10">{formatTime(duration)}</span>
                    </div>

                    {/* Buttons */}
                    <div className="flex items-center justify-between">
                         <div className="flex items-center gap-4 w-1/3">
                            <Button 
                                onClick={toggleEqPanel}
                                variant="ghost"
                                className={`rounded-full p-2 ${audioSettings.eqEnabled ? 'text-accent-green bg-surface-1/40' : 'text-text-secondary hover:text-text-main hover:bg-surface-1/40'}`}
                                title="EQ"
                                aria-label="Equalizer"
                                aria-pressed={audioSettings.eqEnabled}
                            >
                                <SlidersHorizontal size={20} />
                            </Button>
                            <Button variant="ghost" className="rounded-full p-2 text-text-secondary hover:text-text-main hover:bg-surface-1/40" aria-label="Shuffle" title="Shuffle">
                                <Shuffle size={20} />
                            </Button>
                         </div>

                         <div className="flex items-center justify-center gap-8 w-1/3">
                             <Button onClick={prevSong} variant="ghost" className="rounded-full p-2" aria-label="Previous track">
                                 <SkipBack size={32} className="fill-current" />
                             </Button>
                             <Button 
                                onClick={togglePlay}
                                variant="primary"
                                accent="playback"
                                          className="w-16 h-16 rounded-full p-0 hover:scale-105 transition-transform shadow-lg shadow-black/30"
                                aria-label={isPlaying ? 'Pause' : 'Play'}
                             >
                                 {isPlaying ? <Pause size={32} className="fill-current" /> : <Play size={32} className="fill-current ml-2" />}
                             </Button>
                             <Button onClick={nextSong} variant="ghost" className="rounded-full p-2" aria-label="Next track">
                                 <SkipForward size={32} className="fill-current" />
                             </Button>
                         </div>

                         <div className="flex items-center justify-end gap-3 w-1/3 group">
                            <Volume2 size={20} className="text-text-secondary" />
                            <input
                                type="range"
                                min="0"
                                max="1"
                                step="0.05"
                                value={volume}
                                onChange={(e) => setVolume(parseFloat(e.target.value))}
                                className="w-24 h-1 bg-surface-3/60 rounded-lg appearance-none cursor-pointer [&::-webkit-slider-thumb]:appearance-none [&::-webkit-slider-thumb]:w-3 [&::-webkit-slider-thumb]:h-3 [&::-webkit-slider-thumb]:rounded-full [&::-webkit-slider-thumb]:bg-accent-green"
                            />
                         </div>
                    </div>
                 </div>
            </div>
            
            {/* Visualizer Layer Selector Modal */}
            <VisualizerSelector 
                isOpen={showVisualizerSelector} 
                onClose={() => setShowVisualizerSelector(false)} 
            />
        </div>
    );
};
