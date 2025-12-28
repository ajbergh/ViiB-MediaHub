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
 * - 21 visualization modes: OFF, WAVE, SPECTRUM, AURORA, CIRCULAR, PARTICLES, NEBULA,
 *   FLAME_SPECTRUM, STARDUST_HALO, AURORA_RIBBON, ELECTRIC_ARC, GRASS_OSCILLOSCOPE,
 *   CRYSTAL_SHARDS, WATERCOLOR_BLOOM, ICE_FRACTURE, FIREFLY_FIELD, VINYL_SPIN,
 *   BEAT_ORBS, TUNNEL_WAVEFORM, GLASS_SHARDS, WIND_FIELD
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

import React, { useState } from 'react';
import { useStore, useAlbumCovers } from '../store';
import { X, Play, Pause, SkipBack, SkipForward, Shuffle, ListMusic, Activity, SlidersHorizontal, Image as ImageIcon, Volume2, Download, Loader2, CheckCircle } from 'lucide-react';
import { formatTime, generateGradient, cssUrl } from '../utils';
import { ContextMenuType, VisualizerMode } from '../types';
import { api } from '../services/api';
import { Visualizer } from './Visualizer';
import { LyricsView } from './now-playing/LyricsView';
import { QueueList } from './now-playing/QueueList';
import { AlbumArtVisualizer } from './now-playing/AlbumArtVisualizer';
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
        showToast
    } = useStore();
    
    const albumCovers = useAlbumCovers();
    const [activeTab, setActiveTab] = useState<'QUEUE' | 'LYRICS'>('LYRICS');
    const [isDownloading, setIsDownloading] = useState(false);
    const [showVisualizerOverlay, setShowVisualizerOverlay] = useState(false);

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
     * 
     * Triggered by clicking the Activity button in the Now Playing view.
     * Updates global audio settings via Zustand store.
     */
    const cycleVisualizer = () => {
        const modes: VisualizerMode[] = [
            'OFF', 
            'WAVE', 
            'SPECTRUM', 
            'AURORA', 
            'CIRCULAR', 
            'PARTICLES', 
            'NEBULA',
            'FLAME_SPECTRUM',
            'STARDUST_HALO',
            'AURORA_RIBBON',
            'ELECTRIC_ARC',
            'GRASS_OSCILLOSCOPE',
            'CRYSTAL_SHARDS',
            'WATERCOLOR_BLOOM',
            'ICE_FRACTURE',
            'FIREFLY_FIELD',
            'VINYL_SPIN',
            'BEAT_ORBS',
            'TUNNEL_WAVEFORM',
            'GLASS_SHARDS',
            'WIND_FIELD'
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

            <div className="absolute inset-0 z-0 bg-surface-0/70 backdrop-blur-[60px] pointer-events-none"></div>

            {/* Header */}
            <div className="relative z-10 flex items-center justify-between p-6 md:p-8">
                <Button
                    onClick={() => setNowPlayingOpen(false)}
                    variant="ghost"
                    className="rounded-full p-2 bg-surface-1/40 hover:bg-surface-1/60 backdrop-blur-md"
                    aria-label="Close now playing"
                >
                    <X size={24} />
                </Button>
                <div className="flex flex-col items-center">
                    <span className="text-xs font-bold uppercase tracking-widest text-text-secondary">Now Playing</span>
                    <span 
                        className="text-sm font-semibold truncate max-w-[200px]"
                        onContextMenu={(e) => openContextMenu(e, ContextMenuType.ALBUM, { name: currentSong.album, artist: currentSong.artist })}
                    >
                        {currentSong.album}
                    </span>
                </div>
                <div className="flex items-center gap-2">
                    <Button
                        onClick={cycleVisualizer}
                        variant="ghost"
                        className={`rounded-full p-2 backdrop-blur-md ${audioSettings.visualizerMode !== 'OFF' ? 'text-accent-green bg-surface-1/50 ring-1 ring-accent-green/20' : 'text-text-secondary bg-surface-1/40 hover:text-text-main hover:bg-surface-1/60'}`}
                        title={`Visualizer: ${audioSettings.visualizerMode}`}
                        aria-label="Change visualizer mode"
                    >
                        {audioSettings.visualizerMode === 'AURORA' ? <ImageIcon size={24} /> : <Activity size={24} />}
                    </Button>
                    <Button
                        onClick={() => setActiveTab('QUEUE')}
                        variant="ghost"
                        className={`rounded-full p-2 backdrop-blur-md ${activeTab === 'QUEUE' ? 'text-text-main bg-surface-1/60 ring-1 ring-text-main/10' : 'text-text-secondary bg-surface-1/40 hover:text-text-main hover:bg-surface-1/60'}`}
                        title="Queue"
                        aria-label="Show queue"
                    >
                        <ListMusic size={24} />
                    </Button>
                </div>
            </div>

            {/* Main Content */}
            <div className="relative z-10 flex-1 flex flex-col md:flex-row gap-8 md:gap-16 px-8 md:px-16 overflow-hidden">
                {/* Left Side: Artwork & Metadata */}
                <div className="flex-1 flex flex-col justify-center max-w-2xl mx-auto w-full min-h-[400px]">
                    <div 
                        className="aspect-square w-full max-w-[500px] mx-auto bg-surface-2 rounded-xl shadow-2xl relative group overflow-hidden mb-8 md:mb-12 cursor-pointer ring-1 ring-surface-3/60"
                        onClick={() => setShowVisualizerOverlay(!showVisualizerOverlay)}
                        onContextMenu={(e) => openContextMenu(e, ContextMenuType.SONG, currentSong)}
                    >
                         {coverUrl ? (
                            <img 
                                src={coverUrl} 
                                alt="Album Art" 
                                className={`w-full h-full object-contain transition-opacity duration-300 ${
                                    showVisualizerOverlay && audioSettings.visualizerMode !== 'OFF' ? 'opacity-30' : 'opacity-100'
                                }`} 
                            />
                         ) : (
                             <div 
                                className={`w-full h-full flex items-center justify-center text-8xl font-bold text-text-subtle/30 transition-opacity duration-300 ${
                                    showVisualizerOverlay && audioSettings.visualizerMode !== 'OFF' ? 'opacity-30' : 'opacity-100'
                                }`}
                                style={{ background: generateGradient(currentSong.album) }}
                            >
                                {currentSong.title.charAt(0)}
                            </div>
                         )}
                         
                         {/* Album Art Visualizer Overlay */}
                         <AlbumArtVisualizer 
                            mode={audioSettings.visualizerMode}
                            isActive={showVisualizerOverlay && audioSettings.visualizerMode !== 'OFF' && isPlaying}
                         />
                    </div>
                    
                    <div className="flex items-end justify-between mb-2">
                        <div className="flex flex-col min-w-0 pr-4">
                            <h1 className="text-3xl md:text-5xl font-bold truncate leading-tight mb-2" title={currentSong.title}>
                                {currentSong.title}
                            </h1>
                            <h2 
                                className="text-xl md:text-2xl text-text-secondary font-medium truncate cursor-pointer hover:underline hover:text-text-main transition-colors"
                                onContextMenu={(e) => openContextMenu(e, ContextMenuType.ARTIST, { name: currentSong.artist })}
                            >
                                {currentSong.artist}
                            </h2>
                        </div>
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
                    </div>
                </div>

                {/* Right Side: Tabs */}
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
                    </div>
                </div>
            </div>

            {/* Bottom Controls */}
            <div className="relative z-10 px-8 py-8 md:px-16 bg-gradient-to-t from-surface-0 via-surface-0/80 to-transparent">
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
        </div>
    );
};
