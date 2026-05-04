/**
 * ViiB MediaHub - DJ Transport Buttons Component (v2)
 * 
 * Play, Cue, and Sync buttons styled like professional DJ controllers.
 * Enhanced with press animations and visual feedback.
 * 
 * @module components/dj/v2/DJTransportButtons
 */

import React, { useCallback, useRef, useEffect } from 'react';
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

  // Button refs — used for BPM glow via CSS variable (no React state at 60fps)
  const playButtonRef = useRef<HTMLButtonElement>(null);

  // BPM read via ref so the rAF loop doesn't tear down on every tempo nudge.
  const bpmRef = useRef(effectiveBpm || 0);
  useEffect(() => { bpmRef.current = effectiveBpm || 0; }, [effectiveBpm]);

  useEffect(() => {
    if (!isPlaying) {
      if (playButtonRef.current) playButtonRef.current.style.setProperty('--glow', '0');
      return;
    }

    let animId: number;
    const startTime = performance.now();

    const animate = (now: number) => {
      const bpm = bpmRef.current;
      if (bpm > 0 && playButtonRef.current) {
        const beatDuration = 60000 / bpm;
        const elapsed = now - startTime;
        const beatProgress = (elapsed % beatDuration) / beatDuration;
        const glow = beatProgress < 0.1 ? beatProgress * 10 : Math.max(0, 1 - (beatProgress - 0.1) * 1.1);
        playButtonRef.current.style.setProperty('--glow', String(glow));
      }
      animId = requestAnimationFrame(animate);
    };
    animId = requestAnimationFrame(animate);
    return () => cancelAnimationFrame(animId);
  }, [isPlaying]);  // ← only re-arms when play state flips, not every BPM tick

  const handlePlayPause = useCallback(async () => {
    if (track) await togglePlay(deck);
  }, [deck, track, togglePlay]);

  const handleCue = useCallback(() => {
    if (track) returnToCue(deck);
  }, [deck, track, returnToCue]);

  const handleSetCue = useCallback((e: React.MouseEvent) => {
    e.preventDefault();
    if (track) setCue(deck);
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
  // Bigger primary transport — Play is the hero, CUE/SYNC scale with it.
  const cueSize = compact ? 'w-12 h-12' : 'w-16 h-16';
  const playSize = compact ? 'w-12 h-12' : 'w-20 h-20';
  const iconSize = compact ? 18 : 28;

  // Press states are pure CSS — :active scales the button, no JS timer.
  return (
    <div className="flex items-center gap-2">
      {/* Cue Button */}
      <button
        onClick={handleCue}
        onContextMenu={handleSetCue}
        disabled={!track}
        aria-label={`Cue deck ${deck}`}
        title="Cue (Right-click to set)"
        className={`
          ${cueSize} rounded-lg flex items-center justify-center
          transition-transform duration-75 active:scale-[0.92]
          ${track
            ? 'bg-amber-600 hover:bg-amber-500 text-white shadow-[0_2px_8px_rgba(245,158,11,0.3)] active:shadow-[0_1px_4px_rgba(245,158,11,0.6)]'
            : 'bg-[#2a2a2a] text-neutral-600 cursor-not-allowed'}
        `}
      >
        <SkipBack size={iconSize} />
      </button>

      {/* Play/Pause Button */}
      <button
        ref={playButtonRef}
        onClick={handlePlayPause}
        disabled={!track}
        aria-label={isPlaying ? `Pause deck ${deck}` : `Play deck ${deck}`}
        className={`
          ${playSize} rounded-full flex items-center justify-center
          transition-transform duration-75 active:scale-[0.92]
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
            ? `0 3px 16px ${isPlaying ? accentColor + '60' : 'rgba(34, 197, 94, 0.4)'}`
            : undefined,
        }}
      >
        {isPlaying ? <Pause size={iconSize + 2} /> : <Play size={iconSize + 2} className="ml-0.5" />}
      </button>

      {/* Sync Button */}
      <button
        onClick={handleSync}
        disabled={!track || syncMode === 'off'}
        aria-label={`Sync deck ${deck} to other deck`}
        title={syncMode === 'off' ? 'Sync disabled — set sync mode in mixer' : `Sync to other deck (${syncMode})`}
        className={`
          ${compact ? 'h-12 px-4 text-[12px]' : 'h-16 px-7 text-[14px]'} rounded-lg font-bold uppercase tracking-wider
          transition-all duration-100 border flex items-center gap-1.5
          ${!track || syncMode === 'off'
            ? 'bg-[#2a2a2a] text-neutral-600 border-[#333] cursor-not-allowed'
            : syncMode === 'beat-phase'
              ? 'bg-amber-600 hover:bg-amber-500 text-white border-amber-500'
              : 'bg-[#333] hover:bg-[#444] text-white border-[#555]'}
        `}
      >
        SYNC
        {syncMode !== 'off' && track && <span className='w-2 h-2 rounded-full bg-current opacity-80' />}
      </button>
    </div>
  );
};

export default React.memo(DJTransportButtons);
