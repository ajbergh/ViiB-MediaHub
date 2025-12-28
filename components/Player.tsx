/**
 * ViiB MediaHub - Player Component
 * 
 * Main audio player interface displayed at the bottom of the screen.
 * Provides playback controls, progress bar, volume, and quick access to features.
 * 
 * Features:
 * - Dual audio elements for crossfade transitions
 * - Album artwork with Now Playing expansion
 * - Play/pause, next/previous, shuffle, repeat controls
 * - Seekable progress bar with time display
 * - Volume slider with mute toggle
 * - Queue and equalizer panel toggles
 * - Sleep timer with volume fade
 * - Visualizer display (when enabled)
 * - Responsive layout: stacks controls on narrow screens
 * - Hidden secondary controls on mobile (shuffle, repeat, EQ)
 * - Windows SMTC integration via Media Session API
 * 
 * Keyboard shortcuts are handled by useKeyboardNavigation hook in Layout.
 * Integrates with useAudioPlayer hook for audio state management.
 * Uses useMediaSession hook for Windows media controls integration.
 * 
 * @module Player
 */

import React, { useState } from 'react';
import { useStore } from '../store';
import { useAudioPlayer } from '../hooks/useAudioPlayer';
import { Play, Pause, SkipBack, SkipForward, Repeat, Shuffle, Volume2, ListMusic, Maximize2, SlidersHorizontal, Loader2, AlertCircle, RefreshCw, Wifi, Moon } from 'lucide-react';
import { formatTime, generateGradient, cssUrl } from '../utils';
import { NowPlaying } from './NowPlaying';
import { ContextMenuType } from '../types';
import { Visualizer } from './Visualizer';
import { EqualizerPanel } from './Equalizer';
import { SleepTimer, useSleepTimer } from './SleepTimer';
import { useMediaSession } from '../hooks/useMediaSession';
import { Button } from './ui/Button';
import { VIIB_COLOR_VALUES } from './ui/tokens';

export const Player: React.FC = () => {
  const { 
      currentSong, isPlaying, togglePlay, nextSong, prevSong, volume, setVolume, 
      isQueueOpen, setQueueOpen, queue, isNowPlayingOpen, setNowPlayingOpen, 
      openContextMenu, audioSettings, toggleEqPanel, isBuffering, bufferProgress,
      streamError, retryStream, clearStreamError
  } = useStore();
  
  const { 
      primaryRef, secondaryRef,
      currentTime, duration, handleTimeUpdate, handleEnded, seek 
  } = useAudioPlayer();

  // Windows SMTC / Media Session API integration
  // Provides media key support and Windows media overlay controls
  useMediaSession(currentTime, duration, seek);

  // Sleep timer state
  const [isSleepTimerOpen, setIsSleepTimerOpen] = useState(false);
  const sleepTimer = useSleepTimer();

  if (!currentSong) {
      return (
          <>
            <div className="h-24 bg-surface-1 border-t border-surface-3 flex items-center justify-center text-text-subtle">
                <span className="text-sm">Select a song to start listening</span>
            </div>
            <EqualizerPanel />
          </>
      )
  }

  return (
    <>
                <div className="h-24 bg-surface-1 border-t border-surface-3 px-2 md:px-4 grid grid-cols-[1fr_auto_1fr] md:grid-cols-3 items-center z-50 relative gap-2 md:gap-4">
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
        <div className="flex items-center gap-2 md:gap-4 min-w-0">
            <div 
                className="w-12 h-12 md:w-14 md:h-14 rounded overflow-hidden flex-shrink-0 shadow-lg cursor-pointer group relative"
                style={{ background: currentSong.coverUrl ? cssUrl(currentSong.coverUrl) : generateGradient(currentSong.album) }}
                onClick={() => setNowPlayingOpen(true)}
                onContextMenu={(e) => openContextMenu(e, ContextMenuType.SONG, currentSong)}
            >
                {!currentSong.coverUrl && <div className="w-full h-full opacity-30"></div>}
                {currentSong.coverUrl && <img src={currentSong.coverUrl} alt="Cover" className="w-full h-full object-cover" />}
                
                <div className="absolute inset-0 bg-surface-0/70 opacity-0 group-hover:opacity-100 flex items-center justify-center transition-opacity">
                    <Maximize2 size={20} className="text-text-main" />
                </div>
            </div>
            <div className="flex flex-col min-w-0">
            <span 
                className="text-text-main font-medium truncate text-xs md:text-sm hover:underline cursor-pointer"
                onClick={() => setNowPlayingOpen(true)}
                onContextMenu={(e) => openContextMenu(e, ContextMenuType.SONG, currentSong)}
            >
                {currentSong.title}
            </span>
            <span 
                className="text-text-secondary text-xs truncate hover:underline cursor-pointer hidden sm:block"
                onContextMenu={(e) => openContextMenu(e, ContextMenuType.ARTIST, { name: currentSong.artist })}
            >
                {currentSong.artist}
            </span>
            </div>
        </div>

        {/* Controls */}
        <div className="flex flex-col items-center gap-1 md:gap-2">
            <div className="flex items-center gap-3 md:gap-6">
            <Button variant="ghost" className="hidden md:inline-flex rounded-full p-2" title="Shuffle" aria-label="Shuffle">
                <Shuffle size={18} />
            </Button>
            <Button onClick={prevSong} variant="ghost" className="rounded-full p-2" aria-label="Previous track">
                <SkipBack size={20} className="md:w-6 md:h-6 fill-current" />
            </Button>
            
            <div className="relative">
                {/* Buffering indicator */}
                {isBuffering && currentSong.isStreaming && (
                    <div className="absolute inset-0 flex items-center justify-center">
                        <div className="w-11 h-11 md:w-12 md:h-12 rounded-full border-2 border-accent-green/30 border-t-accent-green animate-spin" />
                    </div>
                )}
                <Button
                    onClick={togglePlay}
                    variant="primary"
                    accent="playback"
                    className="w-9 h-9 md:w-10 md:h-10 rounded-full p-0 hover:scale-105 transition-transform z-10 relative"
                    aria-label={isPlaying ? 'Pause' : 'Play'}
                >
                    {isBuffering && currentSong.isStreaming ? (
                        <Loader2 size={18} className="md:w-5 md:h-5 animate-spin text-surface-0/70" />
                    ) : isPlaying ? (
                        <Pause size={18} className="md:w-5 md:h-5 fill-current" />
                    ) : (
                        <Play size={18} className="md:w-5 md:h-5 fill-current ml-0.5" />
                    )}
                </Button>
            </div>

            <Button onClick={nextSong} variant="ghost" className="rounded-full p-2" aria-label="Next track">
                <SkipForward size={20} className="md:w-6 md:h-6 fill-current" />
            </Button>
            <Button variant="ghost" className="hidden md:inline-flex rounded-full p-2" title="Repeat" aria-label="Repeat">
                <Repeat size={18} />
            </Button>
            </div>
            
            {/* Progress bar with buffer indication */}
            <div className="w-full max-w-md flex items-center gap-2 text-xs text-text-secondary">
            <span className="w-10 text-right font-mono" aria-hidden="true">{formatTime(currentTime)}</span>
            
            {/* Error state - show error with retry */}
            {streamError && currentSong?.isStreaming ? (
                <div className="flex-1 flex items-center gap-2 px-2">
                    <AlertCircle size={14} className="text-accent-crimson flex-shrink-0" />
                    <span className="text-accent-crimson text-xs truncate">{streamError.message}</span>
                    {streamError.canRetry && (
                        <Button
                            onClick={() => retryStream()}
                            title="Retry playback"
                            variant="secondary"
                            className="h-7 px-2 py-0.5 text-xs rounded-md"
                            leftIcon={<RefreshCw size={12} aria-hidden="true" />}
                        >
                            Retry
                        </Button>
                    )}
                </div>
            ) : (
                <div className="flex-1 relative h-4 flex items-center">
                    {/* Buffer progress background for streaming tracks */}
                    {currentSong?.isStreaming && bufferProgress < 100 && (
                        <div 
                            className="absolute h-1 bg-accent-green/20 rounded-lg transition-all duration-300"
                            style={{ width: `${bufferProgress}%` }}
                        />
                    )}
                    
                    {/* Main progress slider */}
                    <input
                        type="range"
                        min="0"
                        max={duration || 100}
                        value={currentTime}
                        onChange={(e) => seek(Number(e.target.value))}
                        aria-label="Seek position"
                        aria-valuemin={0}
                        aria-valuemax={duration || 100}
                        aria-valuenow={currentTime}
                        aria-valuetext={`${formatTime(currentTime)} of ${formatTime(duration)}`}
                        className="w-full h-1 bg-surface-slider rounded-lg appearance-none cursor-pointer [&::-webkit-slider-thumb]:appearance-none [&::-webkit-slider-thumb]:w-3 [&::-webkit-slider-thumb]:h-3 [&::-webkit-slider-thumb]:rounded-full [&::-webkit-slider-thumb]:bg-text-main hover:[&::-webkit-slider-thumb]:scale-110 relative z-10"
                    />
                </div>
            )}
            
            <span className="w-10 font-mono" aria-hidden="true">{formatTime(duration)}</span>
            
            {/* Streaming indicator */}
            {currentSong?.isStreaming && !streamError && (
                <div className="flex items-center gap-1 text-accent-green" title={isBuffering ? 'Buffering...' : 'Streaming'}>
                    {isBuffering ? (
                        <Loader2 size={12} className="animate-spin" />
                    ) : (
                        <Wifi size={12} />
                    )}
                </div>
            )}
            </div>
        </div>

        {/* Volume & Extras - Hidden on mobile, shown on tablet+ */}
        <div className="hidden md:flex items-center justify-end gap-3">
            {/* Sleep Timer Toggle */}
            <Button 
                onClick={() => setIsSleepTimerOpen(true)}
                title="Sleep Timer"
                aria-label={`Sleep Timer ${sleepTimer.timerState.mode !== 'off' ? 'active' : ''}`}
                variant="ghost"
                className={`rounded-full p-2 ${sleepTimer.timerState.mode !== 'off' ? 'text-accent-green bg-surface-2/60' : ''}`}
            >
                <Moon size={18} />
            </Button>

            {/* EQ Toggle */}
            <Button 
                onClick={toggleEqPanel}
                title="Equalizer"
                aria-label={`Equalizer ${audioSettings.eqEnabled ? 'enabled' : 'disabled'}`}
                aria-pressed={audioSettings.eqEnabled}
                variant="ghost"
                className={`rounded-full p-2 ${audioSettings.eqEnabled ? 'text-accent-green bg-surface-2/60' : ''}`}
            >
                <SlidersHorizontal size={18} />
            </Button>

            {/* Mini Visualizer - Hidden on smaller screens */}
            <div className="hidden lg:flex w-20 h-8 mx-1 items-end opacity-90 pb-1" title="Spectrum Analyzer">
                {audioSettings.visualizerEnabled && (
                    <Visualizer mode="SPECTRUM" barColor={VIIB_COLOR_VALUES.playbackGreen} />
                )}
            </div>

            <Button 
                onClick={() => setQueueOpen(!isQueueOpen)}
                title="Queue"
                variant="ghost"
                className={`relative rounded-full p-2 ${isQueueOpen ? 'text-accent-green bg-surface-2/60' : ''}`}
            >
                <ListMusic size={20} />
                {queue.length > 0 && (
                    <div className="absolute -top-1 -right-1 min-w-[18px] h-[18px] px-1 bg-brand text-surface-0 text-[10px] leading-none font-bold rounded-full flex items-center justify-center shadow-sm border border-surface-0">
                        {queue.length}
                    </div>
                )}
            </Button>

            <div className="flex items-center gap-2 w-32 group">
                <Volume2 size={20} className="text-text-secondary group-hover:text-text-main" />
                <input
                    type="range"
                    min="0"
                    max="1"
                    step="0.05"
                    value={volume}
                    onChange={(e) => setVolume(parseFloat(e.target.value))}
                    className="w-full h-1 bg-surface-slider rounded-lg appearance-none cursor-pointer [&::-webkit-slider-thumb]:appearance-none [&::-webkit-slider-thumb]:w-3 [&::-webkit-slider-thumb]:h-3 [&::-webkit-slider-thumb]:rounded-full [&::-webkit-slider-thumb]:bg-text-main opacity-0 group-hover:opacity-100 transition-opacity"
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
        
        <SleepTimer 
            isOpen={isSleepTimerOpen}
            onClose={() => setIsSleepTimerOpen(false)}
            timerState={sleepTimer.timerState}
            setTimer={sleepTimer.setTimer}
            setTimerBySongs={sleepTimer.setTimerBySongs}
            setTimerEndOfSong={sleepTimer.setTimerEndOfSong}
            cancelTimer={sleepTimer.cancelTimer}
            getRemainingTime={sleepTimer.getRemainingTime}
        />
    </>
  );
};