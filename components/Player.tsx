import React from 'react';
import { useStore } from '../store';
import { useAudioPlayer } from '../hooks/useAudioPlayer';
import { Play, Pause, SkipBack, SkipForward, Repeat, Shuffle, Volume2, ListMusic, Maximize2, SlidersHorizontal } from 'lucide-react';
import { formatTime, generateGradient, cssUrl } from '../utils';
import { NowPlaying } from './NowPlaying';
import { ContextMenuType } from '../types';
import { Visualizer } from './Visualizer';
import { EqualizerPanel } from './Equalizer';

export const Player: React.FC = () => {
  const { 
      currentSong, isPlaying, togglePlay, nextSong, prevSong, volume, setVolume, 
      isQueueOpen, setQueueOpen, queue, isNowPlayingOpen, setNowPlayingOpen, 
      openContextMenu, audioSettings, toggleEqPanel
  } = useStore();
  
  const { 
      primaryRef, secondaryRef,
      currentTime, duration, handleTimeUpdate, handleEnded, seek 
  } = useAudioPlayer();

  if (!currentSong) {
      return (
          <>
            <div className="h-24 bg-surface-1 border-t border-surface-highlight flex items-center justify-center text-text-subtle">
                <span className="text-sm">Select a song to start listening</span>
            </div>
            <EqualizerPanel />
          </>
      )
  }

  return (
    <>
        <div className="h-24 bg-surface-0 border-t border-surface-highlight px-4 grid grid-cols-3 items-center z-50 relative">
        {/* Dual Audio Elements for Crossfading */}
        <audio
            ref={primaryRef}
            onTimeUpdate={() => handleTimeUpdate(0)}
            onEnded={() => handleEnded(0)}
            crossOrigin="anonymous" 
        />
        <audio
            ref={secondaryRef}
            onTimeUpdate={() => handleTimeUpdate(1)}
            onEnded={() => handleEnded(1)}
            crossOrigin="anonymous" 
        />

        {/* Song Info */}
        <div className="flex items-center gap-4">
            <div 
                className="w-14 h-14 rounded overflow-hidden flex-shrink-0 shadow-lg cursor-pointer group relative"
                style={{ background: currentSong.coverUrl ? cssUrl(currentSong.coverUrl) : generateGradient(currentSong.album) }}
                onClick={() => setNowPlayingOpen(true)}
                onContextMenu={(e) => openContextMenu(e, ContextMenuType.SONG, currentSong)}
            >
                {!currentSong.coverUrl && <div className="w-full h-full opacity-30"></div>}
                {currentSong.coverUrl && <img src={currentSong.coverUrl} alt="Cover" className="w-full h-full object-cover" />}
                
                <div className="absolute inset-0 bg-black/50 opacity-0 group-hover:opacity-100 flex items-center justify-center transition-opacity">
                    <Maximize2 size={20} className="text-text-main" />
                </div>
            </div>
            <div className="flex flex-col min-w-0">
            <span 
                className="text-text-main font-medium truncate text-sm hover:underline cursor-pointer"
                onClick={() => setNowPlayingOpen(true)}
                onContextMenu={(e) => openContextMenu(e, ContextMenuType.SONG, currentSong)}
            >
                {currentSong.title}
            </span>
            <span 
                className="text-text-secondary text-xs truncate hover:underline cursor-pointer"
                onContextMenu={(e) => openContextMenu(e, ContextMenuType.ARTIST, { name: currentSong.artist })}
            >
                {currentSong.artist}
            </span>
            </div>
        </div>

        {/* Controls */}
        <div className="flex flex-col items-center gap-2">
            <div className="flex items-center gap-6">
            <button className="text-text-secondary hover:text-text-main transition-colors" title="Shuffle">
                <Shuffle size={18} />
            </button>
            <button onClick={prevSong} className="text-text-secondary hover:text-text-main transition-colors">
                <SkipBack size={24} className="fill-current" />
            </button>
            
            <div className="relative">
                <button
                    onClick={togglePlay}
                    className="w-10 h-10 rounded-full bg-white text-black flex items-center justify-center hover:scale-105 transition-transform z-10 relative"
                >
                    {isPlaying ? <Pause size={20} className="fill-current" /> : <Play size={20} className="fill-current ml-1" />}
                </button>
            </div>

            <button onClick={nextSong} className="text-text-secondary hover:text-text-main transition-colors">
                <SkipForward size={24} className="fill-current" />
            </button>
            <button className="text-text-secondary hover:text-text-main transition-colors" title="Repeat">
                <Repeat size={18} />
            </button>
            </div>
            
            <div className="w-full max-w-md flex items-center gap-2 text-xs text-text-secondary">
            <span className="w-10 text-right font-mono">{formatTime(currentTime)}</span>
            <input
                type="range"
                min="0"
                max={duration || 100}
                value={currentTime}
                onChange={(e) => seek(Number(e.target.value))}
                className="flex-1 h-1 bg-surface-slider rounded-lg appearance-none cursor-pointer [&::-webkit-slider-thumb]:appearance-none [&::-webkit-slider-thumb]:w-3 [&::-webkit-slider-thumb]:h-3 [&::-webkit-slider-thumb]:rounded-full [&::-webkit-slider-thumb]:bg-white hover:[&::-webkit-slider-thumb]:scale-110"
            />
            <span className="w-10 font-mono">{formatTime(duration)}</span>
            </div>
        </div>

        {/* Volume & Extras */}
        <div className="flex items-center justify-end gap-3">
            {/* EQ Toggle */}
            <button 
                onClick={toggleEqPanel}
                className={`p-2 rounded-full transition-colors ${audioSettings.eqEnabled ? 'text-brand' : 'text-text-secondary hover:text-text-main'}`}
                title="Equalizer"
            >
                <SlidersHorizontal size={18} />
            </button>

            {/* Mini Visualizer */}
            <div className="w-20 h-8 mx-1 flex items-end opacity-90 pb-1" title="Spectrum Analyzer">
                {audioSettings.visualizerEnabled && (
                    <Visualizer mode="SPECTRUM" barColor="#22c55e" />
                )}
            </div>

            <button 
                onClick={() => setQueueOpen(!isQueueOpen)}
                className={`relative p-2 rounded-full transition-colors ${isQueueOpen ? 'text-brand bg-surface-hover' : 'text-text-secondary hover:text-text-main'}`}
                title="Queue"
            >
                <ListMusic size={20} />
                {queue.length > 0 && (
                    <div className="absolute -top-1 -right-1 min-w-[18px] h-[18px] px-1 bg-brand text-black text-[10px] leading-none font-bold rounded-full flex items-center justify-center shadow-sm border border-surface-0">
                        {queue.length}
                    </div>
                )}
            </button>

            <div className="flex items-center gap-2 w-32 group">
                <Volume2 size={20} className="text-text-secondary group-hover:text-text-main" />
                <input
                    type="range"
                    min="0"
                    max="1"
                    step="0.05"
                    value={volume}
                    onChange={(e) => setVolume(parseFloat(e.target.value))}
                    className="w-full h-1 bg-surface-slider rounded-lg appearance-none cursor-pointer [&::-webkit-slider-thumb]:appearance-none [&::-webkit-slider-thumb]:w-3 [&::-webkit-slider-thumb]:h-3 [&::-webkit-slider-thumb]:rounded-full [&::-webkit-slider-thumb]:bg-white opacity-0 group-hover:opacity-100 transition-opacity"
                />
            </div>
        </div>
        </div>

        {isNowPlayingOpen && (
            <NowPlaying 
                currentTime={currentTime} 
                duration={duration} 
                onSeek={seek} 
            />
        )}
        
        <EqualizerPanel />
    </>
  );
};