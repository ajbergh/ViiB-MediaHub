/**
 * ViiB MediaHub - DJ Deck View Component
 * 
 * Individual deck controls including:
 * - Album art / track info
 * - Play/Pause/Cue buttons
 * - Tempo fader
 * - Volume fader
 * - Sync button
 * - BPM display
 * 
 * @module components/dj/DeckView
 */

import React, { useCallback } from 'react';
import { useStore } from '../../store';
import { useDJAudioEngine } from '../../hooks/useDJAudioEngine';
import type { DeckId } from '../../slices/djMixerSlice';
import { Play, Pause, SkipBack, Volume2, RefreshCw } from 'lucide-react';

interface DeckViewProps {
  deck: DeckId;
}

export const DeckView: React.FC<DeckViewProps> = ({ deck }) => {
  const deckState = useStore(state => deck === 'A' ? state.djDeckA : state.djDeckB);
  const otherDeckState = useStore(state => deck === 'A' ? state.djDeckB : state.djDeckA);
  const activeDeck = useStore(state => state.djActiveDeck);
  const {
    setCuePoint,
    setActiveDeck,
    setDeckTempo,
  } = useStore();
  
  // Use audio engine hook for actual playback control
  const { togglePlay, setCue, returnToCue, setVolume, setTempo, seek } = useDJAudioEngine();

  const isActive = activeDeck === deck;
  const { track, isPlaying, position, duration, volume, cuePoint, originalBpm, effectiveBpm, tempo } = deckState;

  const handlePlayPause = useCallback(async () => {
    if (track) {
      await togglePlay(deck);
    }
  }, [deck, track, togglePlay]);

  const handleCue = useCallback(() => {
    if (track) {
      returnToCue(deck);
    }
  }, [deck, track, returnToCue]);

  const handleSetCue = useCallback(() => {
    if (track) {
      setCue(deck);
    }
  }, [deck, track, setCue]);

  const handleVolumeChange = useCallback((e: React.ChangeEvent<HTMLInputElement>) => {
    setVolume(deck, parseFloat(e.target.value));
  }, [deck, setVolume]);

  const handleTempoChange = useCallback((e: React.ChangeEvent<HTMLInputElement>) => {
    const newTempo = parseFloat(e.target.value);
    setTempo(deck, newTempo);
  }, [deck, setTempo]);

  const handleTempoReset = useCallback(() => {
    setTempo(deck, 1.0);
  }, [deck, setTempo]);

  const handleSync = useCallback(() => {
    // Calculate tempo needed to match other deck's BPM
    if (originalBpm && otherDeckState.effectiveBpm) {
      const targetBpm = otherDeckState.effectiveBpm;
      const newTempo = targetBpm / originalBpm;
      // Clamp to valid range (0.5 to 1.5)
      const clampedTempo = Math.max(0.5, Math.min(1.5, newTempo));
      setTempo(deck, clampedTempo);
    }
  }, [deck, originalBpm, otherDeckState.effectiveBpm, setTempo]);

  const handleProgressBarClick = useCallback((e: React.MouseEvent<HTMLDivElement>) => {
    if (!track || !duration) return;
    
    const rect = e.currentTarget.getBoundingClientRect();
    const clickX = e.clientX - rect.left;
    const ratio = clickX / rect.width;
    const seekTime = ratio * duration;
    
    seek(deck, Math.max(0, Math.min(duration, seekTime)));
  }, [deck, track, duration, seek]);

  const handleDeckClick = useCallback(() => {
    setActiveDeck(deck);
  }, [deck, setActiveDeck]);

  return (
    <div 
      className={`h-full p-4 flex flex-col ${isActive ? 'bg-surface-1/20' : ''}`}
      onClick={handleDeckClick}
    >
      {/* Track Info */}
      <div className="flex items-center gap-4 mb-4">
        {/* Album Art */}
        <div className="w-20 h-20 bg-surface-2 rounded-lg overflow-hidden flex-shrink-0">
          {track?.coverUrl ? (
            <img 
              src={track.coverUrl} 
              alt={track.album}
              className="w-full h-full object-cover"
            />
          ) : (
            <div className="w-full h-full flex items-center justify-center text-neutral-600">
              <Volume2 size={24} />
            </div>
          )}
        </div>

        {/* Track details */}
        <div className="flex-1 min-w-0">
          <div className="text-sm font-medium text-neutral-100 truncate">
            {track?.title || 'No Track Loaded'}
          </div>
          <div className="text-xs text-neutral-400 truncate">
            {track?.artist || 'Load a track to begin'}
          </div>
          <div className="text-xs text-neutral-500 truncate">
            {track?.album || ''}
          </div>
        </div>
      </div>

      {/* Deck Label */}
      <div className="text-center mb-4">
        <span className={`text-3xl font-bold ${isActive ? 'text-brand' : 'text-neutral-600'}`}>
          {deck}
        </span>
      </div>

      {/* Transport Controls */}
      <div className="flex items-center justify-center gap-4 mb-4">
        {/* Cue Button */}
        <button
          onClick={handleCue}
          onContextMenu={(e) => { e.preventDefault(); handleSetCue(); }}
          disabled={!track}
          className={`
            w-14 h-14 rounded-lg flex items-center justify-center
            transition-colors
            ${track 
              ? 'bg-amber-600 hover:bg-amber-500 text-white' 
              : 'bg-surface-2 text-neutral-600 cursor-not-allowed'}
          `}
          title="Cue (Right-click to set cue point)"
        >
          <SkipBack size={24} />
        </button>

        {/* Play/Pause Button */}
        <button
          onClick={handlePlayPause}
          disabled={!track}
          className={`
            w-16 h-16 rounded-full flex items-center justify-center
            transition-colors
            ${!track 
              ? 'bg-surface-2 text-neutral-600 cursor-not-allowed'
              : isPlaying 
                ? 'bg-brand hover:bg-brand/80 text-white' 
                : 'bg-green-600 hover:bg-green-500 text-white'}
          `}
        >
          {isPlaying ? <Pause size={28} /> : <Play size={28} className="ml-1" />}
        </button>
      </div>

      {/* BPM & Tempo Controls */}
      <div className="mb-4 p-3 bg-surface-1/50 rounded-lg">
        {/* BPM Display */}
        <div className="flex items-center justify-between mb-3">
          <div className="text-xs text-neutral-400">BPM</div>
          <div className="flex items-center gap-2">
            <span className={`text-lg font-mono font-bold ${originalBpm ? 'text-neutral-100' : 'text-neutral-500'}`}>
              {effectiveBpm ? effectiveBpm.toFixed(1) : originalBpm ? originalBpm.toFixed(1) : '---'}
            </span>
            {tempo !== 1.0 && originalBpm && (
              <span className="text-xs text-neutral-500">
                ({originalBpm.toFixed(0)} × {tempo.toFixed(2)})
              </span>
            )}
          </div>
        </div>

        {/* Tempo Fader */}
        <div className="flex items-center gap-2 mb-2">
          <span className="text-xs text-neutral-500 w-8">-50%</span>
          <input
            type="range"
            min="0.5"
            max="1.5"
            step="0.01"
            value={tempo}
            onChange={handleTempoChange}
            onDoubleClick={handleTempoReset}
            className="flex-1 accent-brand"
            title="Tempo (double-click to reset)"
          />
          <span className="text-xs text-neutral-500 w-8 text-right">+50%</span>
        </div>

        {/* Tempo percentage display */}
        <div className="flex items-center justify-between">
          <span className={`text-sm font-mono ${tempo !== 1.0 ? 'text-brand' : 'text-neutral-500'}`}>
            {tempo >= 1 ? '+' : ''}{((tempo - 1) * 100).toFixed(1)}%
          </span>
          
          {/* SYNC Button */}
          <button
            onClick={handleSync}
            disabled={!originalBpm || !otherDeckState.effectiveBpm}
            className={`
              px-3 py-1 rounded text-xs font-bold uppercase tracking-wider
              transition-colors flex items-center gap-1
              ${originalBpm && otherDeckState.effectiveBpm
                ? 'bg-brand hover:bg-brand/80 text-white'
                : 'bg-surface-2 text-neutral-600 cursor-not-allowed'}
            `}
            title="Sync BPM to other deck"
          >
            <RefreshCw size={12} />
            SYNC
          </button>
        </div>
      </div>

      {/* Time Display */}
      <div className="text-center mb-4 font-mono">
        <span className="text-2xl text-neutral-100">
          {formatTime(position)}
        </span>
        <span className="text-neutral-500 mx-2">/</span>
        <span className="text-lg text-neutral-400">
          {formatTime(duration)}
        </span>
      </div>

      {/* Progress Bar */}
      <div className="mb-4 cursor-pointer" onClick={handleProgressBarClick} title="Click to seek">
        <div className="h-2 bg-surface-2 rounded-full overflow-hidden">
          <div 
            className="h-full bg-brand transition-all duration-100"
            style={{ width: `${duration > 0 ? (position / duration) * 100 : 0}%` }}
          />
        </div>
        {/* Cue point marker */}
        {cuePoint > 0 && duration > 0 && (
          <div 
            className="relative"
            style={{ marginLeft: `${(cuePoint / duration) * 100}%` }}
          >
            <div className="absolute -top-2 w-0.5 h-2 bg-amber-500" />
          </div>
        )}
      </div>

      {/* Volume Fader */}
      <div className="flex-1 flex items-center gap-3">
        <Volume2 size={16} className="text-neutral-400" />
        <input
          type="range"
          min="0"
          max="1"
          step="0.01"
          value={volume}
          onChange={handleVolumeChange}
          className="flex-1 accent-brand"
        />
        <span className="text-xs font-mono text-neutral-400 w-8 text-right">
          {Math.round(volume * 100)}
        </span>
      </div>
    </div>
  );
};

function formatTime(seconds: number): string {
  if (!seconds || !isFinite(seconds)) return '00:00';
  const mins = Math.floor(seconds / 60);
  const secs = Math.floor(seconds % 60);
  return `${mins.toString().padStart(2, '0')}:${secs.toString().padStart(2, '0')}`;
}

export default DeckView;
