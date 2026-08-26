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
 * - Skinny player mode with optional native always-on-top pinning
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
import { Play, Pause, SkipBack, SkipForward, Repeat, Shuffle, Volume2, ListMusic, Maximize2, Minimize2, Pin, PinOff, SlidersHorizontal, Loader2, AlertCircle, RefreshCw, Wifi, Moon, Info } from 'lucide-react';
import { formatTime, generateGradient, getAudioFormatInfo } from '../utils';
import { NowPlaying } from './NowPlaying';
import { ContextMenuType, Song } from '../types';
import { Visualizer } from './Visualizer';
import { EqualizerPanel } from './Equalizer';
import { SleepTimer, useSleepTimer } from './SleepTimer';
import { useMediaSession } from '../hooks/useMediaSession';
import { Button } from './ui/Button';
import { VIIB_COLOR_VALUES } from './ui/tokens';
import { isNativeWindowRuntimeAvailable } from '../services/skinnyWindowService';

const skinnyDragStyle = { '--wails-draggable': 'drag' } as React.CSSProperties;
const skinnyNoDragStyle = { '--wails-draggable': 'no-drag' } as React.CSSProperties;

type SkinnyPlayerProps = {
  currentSong: Song | null;
  currentTime: number;
  duration: number;
  isPlaying: boolean;
  isBuffering: boolean;
  volume: number;
  isVisualizerEnabled: boolean;
  isAlwaysOnTop: boolean;
  onPrevious: () => void;
  onTogglePlay: () => void;
  onNext: () => void;
  onSeek: (time: number) => void;
  onVolumeChange: (volume: number) => void;
  onAlwaysOnTopChange: (enabled: boolean) => void;
  onExit: () => void;
};

/** Compact player used when the desktop app is reduced to a slim control strip. */
const SkinnyPlayer: React.FC<SkinnyPlayerProps> = ({
  currentSong,
  currentTime,
  duration,
  isPlaying,
  isBuffering,
  volume,
  isVisualizerEnabled,
  isAlwaysOnTop,
  onPrevious,
  onTogglePlay,
  onNext,
  onSeek,
  onVolumeChange,
  onAlwaysOnTopChange,
  onExit,
}) => {
  const hasSong = Boolean(currentSong);
  const hasNativeWindowControls = isNativeWindowRuntimeAvailable();

  return (
    <div
      className="h-[108px] min-h-[108px] border-y border-surface-3 bg-surface-1 shadow-lg shadow-black/20"
      style={hasNativeWindowControls ? skinnyDragStyle : undefined}
    >
      <div className="flex h-full w-full min-w-0 items-center gap-3 px-3 sm:px-4">
        <div
          className="h-16 w-16 flex-shrink-0 overflow-hidden rounded-md bg-surface-2 shadow-md"
          style={{ background: !currentSong?.coverUrl ? generateGradient(currentSong?.album || '') : undefined }}
        >
          {currentSong?.coverUrl ? (
            <img src={currentSong.coverUrl} alt={`Cover for ${currentSong.album}`} className="h-full w-full object-cover" />
          ) : (
            <div className="h-full w-full bg-surface-0/20" aria-hidden="true" />
          )}
        </div>

        <div className="min-w-[8rem] flex-1 overflow-hidden">
          <p className="truncate text-sm font-semibold text-text-main">
            {currentSong?.title || 'Nothing playing'}
          </p>
          <p className="truncate text-xs text-text-secondary">
            {currentSong ? `${currentSong.artist} · ${currentSong.album}` : 'Choose a song to start listening'}
          </p>
        </div>

        <div className="flex flex-shrink-0 items-center gap-1 sm:gap-2" style={skinnyNoDragStyle}>
          <Button
            onClick={onPrevious}
            disabled={!hasSong}
            variant="ghost"
            className="h-9 w-9 rounded-full p-0"
            aria-label="Previous track"
            title="Previous track"
          >
            <SkipBack size={19} className="fill-current" />
          </Button>
          <Button
            onClick={onTogglePlay}
            disabled={!hasSong}
            variant="primary"
            accent="playback"
            className="h-10 w-10 rounded-full p-0"
            aria-label={isPlaying ? 'Pause' : 'Play'}
            title={isPlaying ? 'Pause' : 'Play'}
          >
            {isBuffering && currentSong?.isStreaming ? (
              <Loader2 size={19} className="animate-spin text-surface-0/70" />
            ) : isPlaying ? (
              <Pause size={19} className="fill-current" />
            ) : (
              <Play size={19} className="ml-0.5 fill-current" />
            )}
          </Button>
          <Button
            onClick={onNext}
            disabled={!hasSong}
            variant="ghost"
            className="h-9 w-9 rounded-full p-0"
            aria-label="Next track"
            title="Next track"
          >
            <SkipForward size={19} className="fill-current" />
          </Button>
        </div>

        <div className="hidden min-w-[180px] flex-[1.4] items-center gap-2 text-[11px] text-text-secondary md:flex" style={skinnyNoDragStyle}>
          <span className="w-9 text-right font-mono">{formatTime(currentTime)}</span>
          <input
            type="range"
            min="0"
            max={duration || 100}
            value={currentTime}
            disabled={!hasSong}
            onChange={(event) => onSeek(Number(event.target.value))}
            aria-label="Seek position"
            aria-valuetext={`${formatTime(currentTime)} of ${formatTime(duration)}`}
            className="h-1 w-full cursor-pointer appearance-none rounded-lg bg-surface-slider disabled:cursor-default [&::-webkit-slider-thumb]:h-3 [&::-webkit-slider-thumb]:w-3 [&::-webkit-slider-thumb]:appearance-none [&::-webkit-slider-thumb]:rounded-full [&::-webkit-slider-thumb]:bg-text-main"
          />
          <span className="w-9 font-mono">{formatTime(duration)}</span>
        </div>

        <div className="hidden w-28 flex-shrink-0 items-center gap-2 sm:flex" style={skinnyNoDragStyle}>
          <Volume2 size={17} className="text-text-secondary" aria-hidden="true" />
          <input
            type="range"
            min="0"
            max="1"
            step="0.05"
            value={volume}
            onChange={(event) => onVolumeChange(Number(event.target.value))}
            aria-label="Volume"
            aria-valuetext={`${Math.round(volume * 100)} percent`}
            className="h-1 w-full cursor-pointer appearance-none rounded-lg bg-surface-slider [&::-webkit-slider-thumb]:h-3 [&::-webkit-slider-thumb]:w-3 [&::-webkit-slider-thumb]:appearance-none [&::-webkit-slider-thumb]:rounded-full [&::-webkit-slider-thumb]:bg-text-main"
          />
        </div>

        <div className="hidden h-8 w-20 flex-shrink-0 items-end pb-1 xl:flex" title="Spectrum Analyzer">
          {isVisualizerEnabled && (
            <Visualizer mode="SPECTRUM" barColor={VIIB_COLOR_VALUES.playbackGreen} />
          )}
        </div>

        <div className="flex flex-shrink-0 items-center gap-1 border-l border-surface-3 pl-2" style={skinnyNoDragStyle}>
          <Button
            onClick={() => onAlwaysOnTopChange(!isAlwaysOnTop)}
            disabled={!hasNativeWindowControls}
            variant="ghost"
            className={`h-9 w-9 rounded-lg p-0 ${
              isAlwaysOnTop
                ? 'bg-accent-green text-surface-0 shadow-md shadow-accent-green/20 ring-1 ring-accent-green'
                : 'text-text-secondary hover:bg-surface-2 hover:text-text-main'
            }`}
            aria-label={isAlwaysOnTop ? 'Disable always on top' : 'Keep window always on top'}
            aria-pressed={isAlwaysOnTop}
            title={hasNativeWindowControls ? (isAlwaysOnTop ? 'Disable always on top' : 'Keep window always on top') : 'Always on top is available in the desktop app'}
          >
            {isAlwaysOnTop ? (
              <Pin size={17} fill="currentColor" strokeWidth={2.5} aria-hidden="true" />
            ) : (
              <PinOff size={17} strokeWidth={2} aria-hidden="true" />
            )}
          </Button>
          <Button
            onClick={onExit}
            variant="ghost"
            className="h-9 w-9 rounded-full p-0"
            aria-label="Exit skinny player mode"
            title="Exit skinny player mode"
          >
            <Maximize2 size={17} aria-hidden="true" />
          </Button>
        </div>
      </div>
    </div>
  );
};

export const Player: React.FC = () => {
  const { 
      currentSong, isPlaying, togglePlay, nextSong, prevSong, volume, setVolume, 
      isQueueOpen, setQueueOpen, queue, isNowPlayingOpen, setNowPlayingOpen, 
      openContextMenu, audioSettings, toggleEqPanel, isBuffering, bufferProgress,
      streamError, retryStream, clearStreamError, openSongInfoModal,
      isSkinnyMode, isSkinnyAlwaysOnTop, setSkinnyMode, setSkinnyAlwaysOnTop
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

  // Keep the media elements mounted while switching layouts so playback and
  // crossfade state are not interrupted by entering or leaving skinny mode.
  const audioElements = (
    <>
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
    </>
  );

  if (isSkinnyMode) {
    return (
      <>
        {audioElements}
        <SkinnyPlayer
          currentSong={currentSong}
          currentTime={currentTime}
          duration={duration}
          isPlaying={isPlaying}
          isBuffering={isBuffering}
          volume={volume}
          isVisualizerEnabled={audioSettings.visualizerEnabled}
          isAlwaysOnTop={isSkinnyAlwaysOnTop}
          onPrevious={prevSong}
          onTogglePlay={togglePlay}
          onNext={nextSong}
          onSeek={seek}
          onVolumeChange={setVolume}
          onAlwaysOnTopChange={setSkinnyAlwaysOnTop}
          onExit={() => setSkinnyMode(false)}
        />
      </>
    );
  }

  if (!currentSong) {
      return (
          <>
            {audioElements}
            <div className="h-24 bg-surface-1 border-t border-surface-3 flex items-center justify-center text-text-subtle">
                <span className="text-sm">Select a song to start listening</span>
            </div>
            <EqualizerPanel />
          </>
      )
  }

  return (
    <>
                {audioElements}
                <div className="h-24 bg-surface-1 border-t border-surface-3 px-2 md:px-4 grid grid-cols-[1fr_auto_1fr] md:grid-cols-3 items-center z-50 relative gap-2 md:gap-4">
        {/* Song Info */}
        <div className="flex items-center gap-2 md:gap-4 min-w-0">
            <div 
                className="w-12 h-12 md:w-14 md:h-14 rounded overflow-hidden flex-shrink-0 shadow-lg cursor-pointer group relative"
                style={{ background: !currentSong.coverUrl ? generateGradient(currentSong.album) : undefined }}
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

            {/* Audio Quality & ReplayGain Badges */}
            <div className="flex items-center gap-1.5 mt-0.5 flex-wrap">
              {currentSong && (() => {
                const formatInfo = getAudioFormatInfo(currentSong);
                const hasReplayGain = currentSong.replayGainDb !== undefined || audioSettings.normalization;
                return (
                  <>
                    <span
                      className={`inline-flex items-center px-1.5 py-0.2 rounded text-[9px] font-mono font-bold ring-1 ${formatInfo.colorClass}`}
                      title={`Audio Format: ${formatInfo.label}`}
                    >
                      {formatInfo.label}
                    </span>
                    {hasReplayGain && (
                      <span
                        className="inline-flex items-center gap-0.5 px-1.5 py-0.2 rounded text-[9px] font-mono font-bold bg-accent-blue/15 text-accent-blue ring-1 ring-accent-blue/30"
                        title={
                          currentSong.replayGainDb !== undefined
                            ? `ReplayGain Loudness: ${currentSong.replayGainDb > 0 ? '+' : ''}${currentSong.replayGainDb.toFixed(2)} dB (Peak: ${currentSong.replayPeak?.toFixed(2) || '1.0'})`
                            : 'Loudness Normalization Active'
                        }
                      >
                        <Volume2 size={9} />
                        {currentSong.replayGainDb !== undefined
                          ? `${currentSong.replayGainDb > 0 ? '+' : ''}${currentSong.replayGainDb.toFixed(1)}dB`
                          : 'RG'}
                      </span>
                    )}
                  </>
                );
              })()}
            </div>
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
            {/* Song Info Properties */}
            <Button
                onClick={() => currentSong && openSongInfoModal(currentSong)}
                title="Song Information & Properties"
                aria-label="Song Information & Properties"
                variant="ghost"
                className="rounded-full p-2 text-text-secondary hover:text-text-main"
            >
                <Info size={18} />
            </Button>

            <Button
                onClick={() => setSkinnyMode(true)}
                title="Enter skinny player mode"
                aria-label="Enter skinny player mode"
                variant="ghost"
                className="rounded-full p-2 text-text-secondary hover:text-text-main"
            >
                <Minimize2 size={18} />
            </Button>

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
                aria-label={`Queue, ${queue.length} ${queue.length === 1 ? 'track' : 'tracks'}`}
                aria-pressed={isQueueOpen}
                aria-expanded={isQueueOpen}
                variant="ghost"
                className={`relative rounded-full p-2 ${isQueueOpen ? 'text-accent-green bg-surface-2/60' : ''}`}
            >
                <ListMusic size={20} aria-hidden="true" />
                {queue.length > 0 && (
                    <div aria-hidden="true" className="absolute -top-1 -right-1 min-w-[18px] h-[18px] px-1 bg-brand text-surface-0 text-[10px] leading-none font-bold rounded-full flex items-center justify-center shadow-sm border border-surface-0">
                        {queue.length}
                    </div>
                )}
            </Button>

            <div className="flex items-center gap-2 w-32 group">
                <Volume2 size={20} aria-hidden="true" className="text-text-secondary group-hover:text-text-main" />
                <input
                    type="range"
                    min="0"
                    max="1"
                    step="0.05"
                    value={volume}
                    onChange={(e) => setVolume(parseFloat(e.target.value))}
                    aria-label="Volume"
                    aria-valuetext={`${Math.round(volume * 100)} percent`}
                    className="w-full h-1 bg-surface-slider rounded-lg appearance-none cursor-pointer [&::-webkit-slider-thumb]:appearance-none [&::-webkit-slider-thumb]:w-3 [&::-webkit-slider-thumb]:h-3 [&::-webkit-slider-thumb]:rounded-full [&::-webkit-slider-thumb]:bg-text-main opacity-60 group-hover:opacity-100 focus-visible:opacity-100 transition-opacity"
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
