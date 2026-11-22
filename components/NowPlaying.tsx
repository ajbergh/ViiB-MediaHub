/**
 * ViiB MediaHub - Now Playing View
 *
 * Full-screen track view with playback controls, lyrics, queue and visualizers.
 * The background uses cover artwork when available, otherwise a deterministic
 * album-name gradient. Artwork is capped at 500px normally; party mode uses a
 * 70vh width capped at 80vw. The Spotify download button queues the streaming
 * track for offline download; it does not save artwork. Player state comes from
 * Zustand and cover URLs from the current song or useAlbumCovers.
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
import { VIIB, VIIB_COLOR_VALUES } from './ui/tokens';

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
        openSongInfoModal,
        songs
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

    const lowPower = !!audioSettings.lowPowerVisuals;
    const visualsActive = isPlaying && audioSettings.visualizerEnabled;
    const artworkVisualizer = visualsActive && showVisualizerOverlay && audioSettings.visualizerMode !== "OFF";
    const fullscreenVisualizer = visualsActive && !artworkVisualizer && audioSettings.visualizerFullscreenEnabled && audioSettings.visualizerBackgroundMode !== "OFF";
    const lightweightMode = audioSettings.visualizerMode === "WAVE" ? "WAVE" : "SPECTRUM";

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
     * Order: the 12 supported modes in the same sequence used by the global
     * keyboard shortcut, then back to OFF.
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
        <div className={`fixed inset-0 ${VIIB.zIndex.nowPlaying} bg-surface-0 text-text-main flex flex-col overflow-hidden animate-in fade-in slide-in-from-bottom-10 duration-300 motion-reduce:animate-none motion-reduce:transition-none`}>
            {/* Dynamic Background */}
            <div 
                className={lowPower ? "absolute inset-0 z-0 bg-surface-1 pointer-events-none" : "absolute inset-0 z-0 opacity-30 blur-3xl scale-110 pointer-events-none transition-all duration-1000 motion-reduce:transition-none"}
                style={{ 
                    background: lowPower ? undefined : coverUrl ? `${cssUrl(coverUrl)} center/cover` : generateGradient(currentSong.album),
                }}
            />
            
            {visualsActive && !artworkVisualizer && !fullscreenVisualizer && (audioSettings.visualizerMode === "SPECTRUM" || audioSettings.visualizerMode === "WAVE") && (
                <div className="absolute inset-0 z-0 opacity-40">
                    <Visualizer mode={audioSettings.visualizerMode} lowPower={lowPower} barColor={VIIB_COLOR_VALUES.visualizerMuted} />
                </div>
            )}

            {/* Fullscreen Background Visualizer - Behind all UI when enabled */}
            {/* Uses separate visualizerBackgroundMode so it can differ from album art overlay */}
            {fullscreenVisualizer && lowPower && (
                <div className="absolute inset-0 z-[1] opacity-30 pointer-events-none">
                    <Visualizer mode={lightweightMode} lowPower />
                </div>
            )}
            {fullscreenVisualizer && !lowPower && (
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

            <div className={lowPower ? "absolute inset-0 z-0 bg-surface-0/40 pointer-events-none" : "absolute inset-0 z-0 bg-surface-0/70 backdrop-blur-[60px] pointer-events-none"}></div>

            {/* Header - Hidden in party mode unless mouse active */}
            <div className={`relative z-10 flex items-center justify-between gap-2 p-4 md:p-8 transition-opacity duration-300 ${
                isPartyMode ? (showPartyControls ? 'opacity-100' : 'opacity-0') : 'opacity-100'
            }`}>
                <Button
                    onClick={() => {
                        if (isPartyMode) setPartyMode(false);
                        setNowPlayingOpen(false);
                    }}
                    variant="ghost"
                    className="rounded-full p-1.5 sm:p-2 shrink-0 bg-surface-1/40 hover:bg-surface-1/60 backdrop-blur-md"
                    aria-label="Close now playing"
                >
                    <X size={24} />
                </Button>
                <div className="flex flex-col items-center min-w-0 flex-1">
                    <span className="text-xs font-bold uppercase tracking-widest text-text-secondary">
                        {isPartyMode ? 'Party Mode' : 'Now Playing'}
                    </span>
                    <span 
                        className="text-sm font-semibold truncate max-w-full sm:max-w-[200px]"
                        onContextMenu={(e) => openContextMenu(e, ContextMenuType.ALBUM, { name: currentSong.album, artist: currentSong.artist })}
                    >
                        {currentSong.album}
                    </span>
                </div>
                <div className="flex items-center gap-1 sm:gap-2 shrink-0">
                    <Button
                        onClick={togglePartyMode}
                        variant="ghost"
                        className={`rounded-full p-1.5 sm:p-2 backdrop-blur-md ${
                            isPartyMode 
                                ? 'text-amber-400 bg-amber-400/20 ring-1 ring-amber-400/30' 
                                : 'text-text-secondary bg-surface-1/40 hover:text-amber-400 hover:bg-surface-1/60'
                        }`}
                        title={isPartyMode ? "Exit Party Mode" : "Enter Party Mode"}
                        aria-label={isPartyMode ? "Exit party mode" : "Enter party mode"}
                    >
                        {isPartyMode ? <Minimize2 size={24} /> : <Maximize2 size={24} />}
                    </Button>
                    <Button
                        onClick={() => setShowVisualizerSelector(true)}
                        variant="ghost"
                        className={`rounded-full p-1.5 sm:p-2 backdrop-blur-md ${
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
                        className={`rounded-full p-1.5 sm:p-2 backdrop-blur-md ${audioSettings.visualizerMode !== 'OFF' ? 'text-accent-green bg-surface-1/50 ring-1 ring-accent-green/20' : 'text-text-secondary bg-surface-1/40 hover:text-text-main hover:bg-surface-1/60'}`}
                        title={`Visualizer: ${audioSettings.visualizerMode}`}
                        aria-label="Change visualizer mode"
                    >
                        <Activity size={24} />
                    </Button>
                    {!isPartyMode && (
                        <Button
                            onClick={() => setActiveTab('QUEUE')}
                            variant="ghost"
                            className={`rounded-full p-1.5 sm:p-2 backdrop-blur-md ${activeTab === 'QUEUE' ? 'text-text-main bg-surface-1/60 ring-1 ring-text-main/10' : 'text-text-secondary bg-surface-1/40 hover:text-text-main hover:bg-surface-1/60'}`}
                            title="Queue"
                            aria-label="Show queue"
                        >
                            <ListMusic size={24} />
                        </Button>
                    )}
                </div>
            </div>

            {/* Main Content */}
            <div className={`relative z-10 flex-1 min-h-0 flex ${isPartyMode ? 'items-center justify-center' : 'flex-col md:flex-row gap-4 md:gap-12'} px-4 md:px-16 overflow-hidden`}>
                {/* Left Side: Artwork & Metadata - Full screen centered in party mode */}
                <div className={`flex flex-col ${isPartyMode ? 'items-center justify-center' : 'shrink-0 md:flex-1 justify-center max-w-2xl mx-auto w-full min-h-0'}`}>
                    <div 
                        className={`aspect-square bg-surface-2 rounded-xl shadow-2xl relative group overflow-hidden cursor-pointer ring-1 ring-surface-3/60 ${
                            isPartyMode 
                                ? 'w-[70vh] max-w-[80vw] max-h-[60vh] mb-8'
                                : 'w-auto h-[32vh] sm:h-[34vh] md:h-[46vh] max-h-[34vh] md:max-h-[46vh] max-w-[80vw] md:max-w-[500px] mx-auto mb-4 md:mb-10'
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
                                    opacity: artworkVisualizer && (lowPower || audioSettings.visualizerMode !== 'MILKDROP')
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
                                    opacity: artworkVisualizer && (lowPower || audioSettings.visualizerMode !== 'MILKDROP')
                                        ? (audioSettings.visualizerArtworkOpacity ?? 30) / 100
                                        : 1
                                }}
                            >
                                {currentSong.title.charAt(0)}
                            </div>
                         )}
                         
                         {artworkVisualizer && (
                            <div className="absolute inset-0 pointer-events-none">
                                {lowPower ? (
                                    <Visualizer mode={lightweightMode} lowPower />
                                ) : audioSettings.visualizerMode === "MILKDROP" ? (
                                    <MilkdropVisualizer settings={milkdropSettings} isActive
                                        onPresetChange={setMilkdropPreset} onPresetsLoaded={setMilkdropPresetKeys}
                                        style={{ opacity: 1 - ((audioSettings.visualizerArtworkOpacity ?? 30) / 100) }} />
                                ) : (
                                    <WebGLVisualizer mode={audioSettings.visualizerMode} isActive />
                                )}
                            </div>
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
                                onContextMenu={(e) => openContextMenu(e, ContextMenuType.ARTIST, {
                                    name: currentSong.artist,
                                    songCount: songs.filter(s => s.artist === currentSong.artist).length,
                                })}
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
                                        className="inline-flex items-center gap-1 min-h-[28px] px-2.5 py-1 rounded-full text-[11px] font-medium bg-surface-2 hover:bg-surface-3 text-text-secondary hover:text-text-main ring-1 ring-surface-border transition-colors"
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
                    <div className="flex-1 flex flex-col h-full min-h-0 max-w-xl mx-auto w-full bg-surface-1/30 rounded-t-2xl md:rounded-2xl ring-1 ring-surface-3/60 backdrop-blur-md overflow-hidden">
                        {/* Tabs Header */}
                        <div className="flex items-center border-b border-surface-3/60 p-2 gap-2">
                            <Button
                                onClick={() => setActiveTab('QUEUE')}
                                variant={activeTab === 'QUEUE' ? 'secondary' : 'ghost'}
                                className="flex-1 justify-center rounded-lg whitespace-nowrap"
                            >
                                Queue
                            </Button>
                            <Button
                                onClick={() => setActiveTab('LYRICS')}
                                variant={activeTab === 'LYRICS' ? 'secondary' : 'ghost'}
                                className="flex-1 justify-center rounded-lg whitespace-nowrap"
                            >
                                Lyrics
                            </Button>
                            <Button
                                onClick={() => setActiveTab('INFO')}
                                variant={activeTab === 'INFO' ? 'secondary' : 'ghost'}
                                className="flex-1 justify-center rounded-lg flex items-center gap-1.5 whitespace-nowrap"
                            >
                                <Info size={14} className="shrink-0" />
                                Track Info
                            </Button>
                        </div>

                        {/* Tab Content */}
                        <div className="flex-1 min-h-0 overflow-y-auto overflow-x-hidden p-4 md:p-6 relative">
                            {activeTab === 'LYRICS' && (
                                <div className="h-full flex flex-col">
                                    <LyricsView song={currentSong} currentTime={currentTime} onSeek={onSeek} />
                                </div>
                            )}

                            {activeTab === 'QUEUE' && (
                                <div className="h-full flex flex-col">
                                    <QueueList queue={queue} currentSongIndex={currentSongIndex} />
                                </div>
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
            <div className={`relative z-10 px-4 py-4 sm:px-8 sm:py-6 md:px-16 md:py-8 bg-gradient-to-t from-surface-0 via-surface-0/80 to-transparent transition-opacity duration-300 ${
                isPartyMode ? (showPartyControls ? 'opacity-100' : 'opacity-0 pointer-events-none') : 'opacity-100'
            }`}>
                 <div className="max-w-4xl mx-auto w-full flex flex-col gap-4 md:gap-6">
                     {/* Seek Bar */}
                    <div className="flex items-center gap-4 text-xs font-mono font-medium text-text-secondary">
                        <span className="w-10 text-right">{formatTime(currentTime)}</span>
                        <div className="flex-1 h-6 flex items-center relative group">
                            <div className="w-full h-1.5 bg-surface-3/60 rounded-full relative pointer-events-none">
                                <div
                                    className="absolute top-0 left-0 h-full bg-accent-green rounded-full transition-all"
                                    style={{ width: `${(currentTime / (duration || 1)) * 100}%` }}
                                >
                                    <div className="absolute right-0 top-1/2 -translate-y-1/2 w-3 h-3 bg-accent-green rounded-full opacity-0 group-hover:opacity-100 shadow-lg scale-150 transition-all"></div>
                                </div>
                            </div>
                            <input 
                                type="range" 
                                min="0" 
                                max={duration || 100} 
                                value={currentTime}
                                onChange={(e) => onSeek(Number(e.target.value))}
                                aria-label="Seek position"
                                className="absolute inset-0 w-full h-full opacity-0 cursor-pointer"
                            />
                        </div>
                        <span className="w-10">{formatTime(duration)}</span>
                    </div>

                    {/* Buttons */}
                    <div className="flex items-center justify-between gap-2">
                         <div className="flex items-center gap-2 sm:gap-4 flex-1 min-w-0">
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
                            <Button
                                variant="ghost"
                                disabled
                                aria-disabled="true"
                                className="rounded-full p-2 text-text-secondary hover:text-text-main hover:bg-surface-1/40"
                                aria-label="Shuffle (Not yet available)"
                                title="Not yet available"
                            >
                                <Shuffle size={20} />
                            </Button>
                         </div>

                         <div className="flex items-center justify-center gap-3 sm:gap-6 md:gap-8 shrink-0">
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

                         <div className="hidden sm:flex items-center justify-end gap-3 flex-1 min-w-0 group">
                            <Volume2 size={20} className="text-text-secondary shrink-0" aria-hidden="true" />
                            <div className="relative h-6 flex items-center w-24">
                                <div className="w-full h-1 bg-surface-3/60 rounded-lg pointer-events-none" />
                                <input
                                    type="range"
                                    min="0"
                                    max="1"
                                    step="0.05"
                                    value={volume}
                                    onChange={(e) => setVolume(parseFloat(e.target.value))}
                                    aria-label="Volume"
                                    className="absolute inset-0 w-full h-6 bg-transparent appearance-none cursor-pointer [&::-webkit-slider-thumb]:appearance-none [&::-webkit-slider-thumb]:w-3 [&::-webkit-slider-thumb]:h-3 [&::-webkit-slider-thumb]:rounded-full [&::-webkit-slider-thumb]:bg-accent-green"
                                />
                            </div>
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
