/**
 * ViiB MediaHub - Now Playing View Component
 * 
 * Full-screen overlay showing currently playing track with enhanced controls.
 * 
 * Features:
 * - Large album artwork display
 * - Playback controls (play/pause, next/prev, shuffle, repeat)
 * - Progress bar with seeking
 * - Volume control
 * - Visualizer toggle (off, wave, spectrum, aurora)
 * - Lyrics view (placeholder)
 * - Mini queue list
 * - Like button (placeholder)
 * 
 * Activated by clicking album art in the player bar.
 * 
 * @module NowPlaying
 */

import React, { useState } from 'react';
import { useStore, useAlbumCovers } from '../store';
import { X, Play, Pause, SkipBack, SkipForward, Shuffle, Heart, ListMusic, Activity, SlidersHorizontal, Image as ImageIcon, Volume2, Download, Loader2, CheckCircle } from 'lucide-react';
import { formatTime, generateGradient, cssUrl } from '../utils';
import { ContextMenuType, VisualizerMode } from '../types';
import { api } from '../services/api';
import { Visualizer } from './Visualizer';
import { LyricsView } from './now-playing/LyricsView';
import { QueueList } from './now-playing/QueueList';
import { AlbumArtVisualizer } from './now-playing/AlbumArtVisualizer';

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
    
    const cycleVisualizer = () => {
        const modes: VisualizerMode[] = ['OFF', 'WAVE', 'SPECTRUM', 'AURORA', 'CIRCULAR', 'PARTICLES', 'NEBULA'];
        const currentIdx = modes.indexOf(audioSettings.visualizerMode);
        const nextIdx = (currentIdx + 1) % modes.length;
        setVisualizerMode(modes[nextIdx]);
    };

    return (
        <div className="fixed inset-0 z-[100] bg-surface-0 text-white flex flex-col overflow-hidden animate-in fade-in slide-in-from-bottom-10 duration-300">
            {/* Dynamic Background */}
            <div 
                className="absolute inset-0 z-0 opacity-30 blur-3xl scale-110 pointer-events-none transition-all duration-1000"
                style={{ 
                    background: coverUrl ? `${cssUrl(coverUrl)} center/cover` : generateGradient(currentSong.album),
                }}
            />
            
            {/* Visualizer Layer */}
            <div className="absolute inset-0 z-0 opacity-40">
                <Visualizer mode={audioSettings.visualizerMode} barColor="rgba(255,255,255,0.4)" />
            </div>

            <div className="absolute inset-0 z-0 bg-black/60 backdrop-blur-[60px] pointer-events-none"></div>

            {/* Header */}
            <div className="relative z-10 flex items-center justify-between p-6 md:p-8">
                <button 
                    onClick={() => setNowPlayingOpen(false)}
                    className="p-2 rounded-full bg-white/10 hover:bg-white/20 transition-colors backdrop-blur-md"
                >
                    <X size={24} />
                </button>
                <div className="flex flex-col items-center">
                    <span className="text-xs font-bold uppercase tracking-widest text-white/60">Now Playing</span>
                    <span 
                        className="text-sm font-semibold truncate max-w-[200px]"
                        onContextMenu={(e) => openContextMenu(e, ContextMenuType.ALBUM, { name: currentSong.album, artist: currentSong.artist })}
                    >
                        {currentSong.album}
                    </span>
                </div>
                <div className="flex items-center gap-2">
                    <button 
                        onClick={cycleVisualizer}
                        className={`p-2 rounded-full transition-colors backdrop-blur-md ${audioSettings.visualizerMode !== 'OFF' ? 'bg-green-500/20 text-green-500' : 'bg-white/10 text-white/50 hover:text-white'}`}
                        title={`Visualizer: ${audioSettings.visualizerMode}`}
                    >
                        {audioSettings.visualizerMode === 'AURORA' ? <ImageIcon size={24} /> : <Activity size={24} />}
                    </button>
                    <button 
                        onClick={() => setActiveTab('QUEUE')}
                        className={`p-2 rounded-full transition-colors backdrop-blur-md ${activeTab === 'QUEUE' ? 'bg-white/20 text-white' : 'bg-white/10 text-white/50 hover:text-white'}`}
                    >
                        <ListMusic size={24} />
                    </button>
                </div>
            </div>

            {/* Main Content */}
            <div className="relative z-10 flex-1 flex flex-col md:flex-row gap-8 md:gap-16 px-8 md:px-16 overflow-hidden">
                {/* Left Side: Artwork & Metadata */}
                <div className="flex-1 flex flex-col justify-center max-w-2xl mx-auto w-full min-h-[400px]">
                    <div 
                        className="aspect-square w-full max-w-[500px] mx-auto bg-[#222] rounded-xl shadow-2xl relative group overflow-hidden mb-8 md:mb-12 cursor-pointer border border-white/10"
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
                                className={`w-full h-full flex items-center justify-center text-8xl font-bold text-white/20 transition-opacity duration-300 ${
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
                                className="text-xl md:text-2xl text-white/70 font-medium truncate cursor-pointer hover:underline hover:text-white transition-colors"
                                onContextMenu={(e) => openContextMenu(e, ContextMenuType.ARTIST, { name: currentSong.artist })}
                            >
                                {currentSong.artist}
                            </h2>
                        </div>
                        <div className="flex items-center gap-3">
                            {/* Download button for streaming Spotify tracks */}
                            {isSpotifyStreaming && (
                                <button 
                                    onClick={handleDownloadTrack}
                                    disabled={isDownloading}
                                    className="text-white/50 hover:text-brand hover:scale-110 transition-all disabled:opacity-50 disabled:cursor-not-allowed"
                                    title="Download for offline"
                                >
                                    {isDownloading ? (
                                        <Loader2 size={28} className="animate-spin" />
                                    ) : (
                                        <Download size={28} />
                                    )}
                                </button>
                            )}
                            {/* Show checkmark if track is downloaded (has spotifyId but not streaming) */}
                            {currentSong.spotifyId && !currentSong.isStreaming && (
                                <div className="text-brand" title="Downloaded">
                                    <CheckCircle size={28} />
                                </div>
                            )}
                            <button className="text-white/50 hover:text-green-500 hover:scale-110 transition-all">
                                <Heart size={32} />
                            </button>
                        </div>
                    </div>
                </div>

                {/* Right Side: Tabs */}
                <div className="flex-1 flex flex-col h-full min-h-[400px] max-w-xl mx-auto w-full bg-white/5 rounded-t-2xl md:rounded-2xl border border-white/10 backdrop-blur-md overflow-hidden">
                    {/* Tabs Header */}
                    <div className="flex items-center border-b border-white/10 p-1">
                        <button 
                            onClick={() => setActiveTab('QUEUE')}
                            className={`flex-1 py-3 text-sm font-bold uppercase tracking-wider transition-colors rounded-t-lg ${activeTab === 'QUEUE' ? 'bg-white/10 text-white' : 'text-white/50 hover:text-white hover:bg-white/5'}`}
                        >
                            Queue
                        </button>
                        <button 
                            onClick={() => setActiveTab('LYRICS')}
                            className={`flex-1 py-3 text-sm font-bold uppercase tracking-wider transition-colors rounded-t-lg ${activeTab === 'LYRICS' ? 'bg-white/10 text-white' : 'text-white/50 hover:text-white hover:bg-white/5'}`}
                        >
                            Lyrics
                        </button>
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
            <div className="relative z-10 px-8 py-8 md:px-16 bg-gradient-to-t from-black via-black/80 to-transparent">
                 <div className="max-w-4xl mx-auto w-full flex flex-col gap-6">
                     {/* Seek Bar */}
                    <div className="flex items-center gap-4 text-xs font-mono font-medium text-white/50">
                        <span className="w-10 text-right">{formatTime(currentTime)}</span>
                        <div className="flex-1 h-1.5 bg-white/20 rounded-full relative group">
                            <div 
                                className="absolute top-0 left-0 h-full bg-white rounded-full transition-all"
                                style={{ width: `${(currentTime / (duration || 1)) * 100}%` }}
                            >
                                <div className="absolute right-0 top-1/2 -translate-y-1/2 w-3 h-3 bg-white rounded-full opacity-0 group-hover:opacity-100 shadow-lg scale-150 transition-all"></div>
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
                            <button 
                                onClick={toggleEqPanel} 
                                className={`transition-colors ${audioSettings.eqEnabled ? 'text-green-500' : 'text-white/50 hover:text-white'}`} 
                                title="EQ"
                            >
                                <SlidersHorizontal size={20} />
                            </button>
                            <button className="text-white/50 hover:text-white transition-colors"><Shuffle size={20} /></button>
                         </div>

                         <div className="flex items-center justify-center gap-8 w-1/3">
                             <button onClick={prevSong} className="text-white hover:text-white/80 transition-colors">
                                 <SkipBack size={32} className="fill-current" />
                             </button>
                             <button 
                                onClick={togglePlay}
                                className="w-16 h-16 bg-white text-black rounded-full flex items-center justify-center hover:scale-105 transition-transform shadow-[0_0_20px_rgba(255,255,255,0.3)]"
                             >
                                 {isPlaying ? <Pause size={32} className="fill-current" /> : <Play size={32} className="fill-current ml-2" />}
                             </button>
                             <button onClick={nextSong} className="text-white hover:text-white/80 transition-colors">
                                 <SkipForward size={32} className="fill-current" />
                             </button>
                         </div>

                         <div className="flex items-center justify-end gap-3 w-1/3 group">
                            <Volume2 size={20} className="text-white/50" />
                            <input
                                type="range"
                                min="0"
                                max="1"
                                step="0.05"
                                value={volume}
                                onChange={(e) => setVolume(parseFloat(e.target.value))}
                                className="w-24 h-1 bg-white/20 rounded-lg appearance-none cursor-pointer [&::-webkit-slider-thumb]:appearance-none [&::-webkit-slider-thumb]:w-3 [&::-webkit-slider-thumb]:h-3 [&::-webkit-slider-thumb]:rounded-full [&::-webkit-slider-thumb]:bg-white"
                            />
                         </div>
                    </div>
                 </div>
            </div>
        </div>
    );
};
