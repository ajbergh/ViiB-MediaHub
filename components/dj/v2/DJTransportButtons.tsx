/**
 * ViiB MediaHub - DJ Transport Buttons Component (v2)
 * 
 * Play, Cue, and Sync buttons styled like professional DJ controllers.
 * Enhanced with press animations and visual feedback.
 * 
 * @module components/dj/v2/DJTransportButtons
 */

import React, { useCallback, useState, useRef, useEffect } from 'react';
import { useStore } from '../../../store';
import { useDJAudioEngineActions } from '../../../hooks/useDJAudioEngine';
import type { DeckId } from '../../../slices/djMixerSlice';
import { Play, Pause, SkipBack } from 'lucide-react';

interface DJTransportButtonsProps {
  deck: DeckId;
  compact?: boolean;
}

export const DJTransportButtons: React.FC<DJTransportButtonsProps> = ({ 
  deck,
  compact = false 
}) => {
  // Granular selectors - only subscribe to what we need (NOT full deckState)
  const track = useStore(state => deck === 'A' ? state.djDeckA.track : state.djDeckB.track);
  const isPlaying = useStore(state => deck === 'A' ? state.djDeckA.isPlaying : state.djDeckB.isPlaying);
  const originalBpm = useStore(state => deck === 'A' ? state.djDeckA.originalBpm : state.djDeckB.originalBpm);
  const effectiveBpm = useStore(state => deck === 'A' ? state.djDeckA.effectiveBpm : state.djDeckB.effectiveBpm);
  const otherEffectiveBpm = useStore(state => deck === 'A' ? state.djDeckB.effectiveBpm : state.djDeckA.effectiveBpm);
  const syncMode = useStore(state => state.djMixer.syncMode);
  
  const { togglePlay, returnToCue, setCue, setTempo, syncBeatPhase } = useDJAudioEngineActions();
  
  // Button press states
  const [cuePressed, setCuePressed] = useState(false);
  const [playPressed, setPlayPressed] = useState(false);
  
  // BPM glow via ref + CSS variable (no React state updates at 60fps)
  const playButtonRef = useRef<HTMLButtonElement>(null);
  
  useEffect(() => {
    if (!isPlaying || !(effectiveBpm || 0)) return;
    const bpm = effectiveBpm || 0;
    if (bpm <= 0) return;
    
    const beatDuration = 60000 / bpm;
    let animId: number;
    const lastBeatTime = performance.now();
    
    const animate = (now: number) => {
      const elapsed = now - lastBeatTime;
      const beatProgress = (elapsed % beatDuration) / beatDuration;
      const glow = beatProgress < 0.1 ? beatProgress * 10 : Math.max(0, 1 - (beatProgress - 0.1) * 1.1);
      
      if (playButtonRef.current) {
        playButtonRef.current.style.setProperty('--glow', String(glow));
      }
      animId = requestAnimationFrame(animate);
    };
    animId = requestAnimationFrame(animate);
    return () => cancelAnimationFrame(animId);
  }, [isPlaying, effectiveBpm]);

  const handlePlayPause = useCallback(async () => {
    if (track) {
      setPlayPressed(true);
      setTimeout(() => setPlayPressed(false), 100);
      await togglePlay(deck);
    }
  }, [deck, track, togglePlay]);

  const handleCue = useCallback(() => {
    if (track) {
      setCuePressed(true);
      setTimeout(() => setCuePressed(false), 100);
      returnToCue(deck);
    }
  }, [deck, track, returnToCue]);

  const handleSetCue = useCallback((e: React.MouseEvent) => {
    e.preventDefault();
    if (track) {
      setCue(deck);
    }
  }, [deck, track, setCue]);

  const handleSync = useCallback(() => {
    if (syncMode === 'off' || !originalBpm || !otherEffectiveBpm) return;
    
    const targetBpm = otherEffectiveBpm;
    const newTempo = targetBpm / originalBpm;
    const clampedTempo = Math.max(0.5, Math.min(1.5, newTempo));
    setTempo(deck, clampedTempo);
    
    if (syncMode === 'beat-phase') {
      syncBeatPhase(deck);
    }
  }, [deck, originalBpm, otherEffectiveBpm, setTempo, syncMode, syncBeatPhase]);

  const accentColor = deck === 'A' ? '#3b82f6' : '#8b5cf6';
  const cueSize = compact ? 'w-10 h-10' : 'w-11 h-11';
  const playSize = compact ? 'w-11 h-11' : 'w-12 h-12';
  const iconSize = compact ? 16 : 18;

  return (
    <div className="flex items-center gap-2">
      {/* Cue Button */}
      <button
        onClick={handleCue}
        onContextMenu={handleSetCue}
        onMouseDown={() => setCuePressed(true)}
        onMouseUp={() => setCuePressed(false)}
        onMouseLeave={() => setCuePressed(false)}
        disabled={!track}
        className={`
          ${cueSize} rounded-lg flex items-center justify-center
          transition-all duration-75
          ${track 
            ? 'bg-amber-600 hover:bg-amber-500 text-white' 
            : 'bg-[#2a2a2a] text-neutral-600 cursor-not-allowed'}
        `}
        style={{
          boxShadow: track ? `0 2px 8px rgba(245, 158, 11, ${cuePressed ? 0.6 : 0.3})` : undefined,
          transform: cuePressed ? 'scale(0.92)' : 'scale(1)',
        }}
        title="Cue (Right-click to set)"
      >
        <SkipBack size={iconSize} />
      </button>

      {/* Play/Pause Button */}
      <button
        ref={playButtonRef}
        onClick={handlePlayPause}
        onMouseDown={() => setPlayPressed(true)}
        onMouseUp={() => setPlayPressed(false)}
        onMouseLeave={() => setPlayPressed(false)}
        disabled={!track}
        className={`
          ${compact ? 'w-11 h-11' : 'w-12 h-12'} rounded-full flex items-center justify-center
          transition-all duration-75
          ${!track 
            ? 'bg-[#2a2a2a] text-neutral-600 cursor-not-allowed'
            : isPlaying 
              ? 'text-white' 
              : 'bg-green-600 hover:bg-green-500 text-white'}
        `}
        style={{
          ['--glow' as any]: '0',
          backgroundColor: track && isPlaying ? accentColor : undefined,
          boxShadow: track 
            ? `0 ${playPressed ? '1' : '3'}px ${8 + 8}px ${isPlaying ? accentColor + (playPressed ? '80' : '60') : 'rgba(34, 197, 94, 0.4)'}` 
            : undefined,
          transform: playPressed ? 'scale(0.92)' : 'scale(1)',
        }}
      >
        {isPlaying ? <Pause size={iconSize + 2} /> : <Play size={iconSize + 2} className="ml-0.5" />}
      </button>

      {/* Sync Button */}
      <button
        onClick={handleSync}
        disabled={!track || syncMode === 'off'}
        className={`
          ${compact ? 'h-8 px-3' : 'h-9 px-4'} rounded-lg text-[10px] font-bold uppercase tracking-wider
          transition-all duration-100 border flex items-center gap-1
          ${!track || syncMode === 'off'
            ? 'bg-[#2a2a2a] text-neutral-600 border-[#333] cursor-not-allowed'
            : syncMode === 'beat-phase'
              ? 'bg-amber-600 hover:bg-amber-500 text-white border-amber-500'
              : 'bg-[#333] hover:bg-[#444] text-white border-[#555]'}
        `}
        title={syncMode === 'off' ? 'Sync disabled — set sync mode in mixer' : `Sync to other deck (${syncMode})`}
      >
        {syncMode === 'beat-phase' ? 'SYNC' : 'SYNC'}
        {syncMode !== 'off' && track && <span className='w-1.5 h-1.5 rounded-full bg-current opacity-80' />}
      </button>
    </div>
  );
};

export default React.memo(DJTransportButtons);
