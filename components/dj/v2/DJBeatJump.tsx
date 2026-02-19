/**
 * ViiB MediaHub - DJ Beat Jump Component
 * 
 * Compact beat jump buttons for precise navigation within tracks.
 * Jumps forward or backward by specified number of beats.
 * 
 * @module components/dj/v2/DJBeatJump
 */

import React, { memo, useCallback } from 'react';
import { useStore } from '../../../store';
import { useDJAudioEngineActions } from '../../../hooks/useDJAudioEngine';
import type { DeckId } from '../../../slices/djMixerSlice';

interface DJBeatJumpProps {
  deck: DeckId;
}

const BEAT_AMOUNTS = [1, 4, 8, 16];

export const DJBeatJump = memo(function DJBeatJump({ deck }: DJBeatJumpProps) {
  // Granular selectors — avoid re-renders from position updates
  const track = useStore(state => deck === 'A' ? state.djDeckA.track : state.djDeckB.track);
  const duration = useStore(state => deck === 'A' ? state.djDeckA.duration : state.djDeckB.duration);
  const effectiveBpm = useStore(state => deck === 'A' ? state.djDeckA.effectiveBpm : state.djDeckB.effectiveBpm);
  const originalBpm = useStore(state => deck === 'A' ? state.djDeckA.originalBpm : state.djDeckB.originalBpm);
  const { seek } = useDJAudioEngineActions();

  const handleJump = useCallback((beats: number) => {
    if (!track || !duration) return;
    
    // Calculate seconds per beat from effective BPM
    const bpm = effectiveBpm || originalBpm;
    if (!bpm || bpm <= 0) return;
    
    // Read position from store snapshot — not reactive, avoids dep on position
    const position = deck === 'A'
      ? useStore.getState().djDeckA.position
      : useStore.getState().djDeckB.position;
    
    const secondsPerBeat = 60 / bpm;
    const jumpAmount = beats * secondsPerBeat;
    const newPosition = Math.max(0, Math.min(duration, position + jumpAmount));
    
    seek(deck, newPosition);
  }, [deck, track, duration, effectiveBpm, originalBpm, seek]);

  const disabled = !track;
  const accentColor = deck === 'A' ? 'blue' : 'purple';

  return (
    <div className="flex flex-col gap-0.5">
      <span className="text-[7px] font-bold text-[#777] uppercase tracking-wider text-center">BEAT JUMP</span>
      <div className="flex items-center gap-0.5">
        {/* Backward jumps */}
        {BEAT_AMOUNTS.map(amt => (
          <button
            key={`back-${amt}`}
            onClick={() => handleJump(-amt)}
            disabled={disabled}
            className={`
              h-5 min-w-[22px] px-0.5 rounded text-[8px] font-bold
              transition-all duration-75 border
              ${disabled
                ? 'bg-[#1a1a1a] text-neutral-700 border-[#222] cursor-not-allowed'
                : `bg-[#1e1e1e] text-neutral-400 border-[#333] hover:bg-${accentColor}-500/20 hover:text-${accentColor}-300 hover:border-${accentColor}-500/40 active:scale-95`
              }
            `}
            title={`Jump back ${amt} beat${amt > 1 ? 's' : ''}`}
          >
            ◀{amt}
          </button>
        ))}
        
        {/* Divider */}
        <div className="w-px h-4 bg-[#333] mx-0.5" />
        
        {/* Forward jumps */}
        {BEAT_AMOUNTS.map(amt => (
          <button
            key={`fwd-${amt}`}
            onClick={() => handleJump(amt)}
            disabled={disabled}
            className={`
              h-5 min-w-[22px] px-0.5 rounded text-[8px] font-bold
              transition-all duration-75 border
              ${disabled
                ? 'bg-[#1a1a1a] text-neutral-700 border-[#222] cursor-not-allowed'
                : `bg-[#1e1e1e] text-neutral-400 border-[#333] hover:bg-${accentColor}-500/20 hover:text-${accentColor}-300 hover:border-${accentColor}-500/40 active:scale-95`
              }
            `}
            title={`Jump forward ${amt} beat${amt > 1 ? 's' : ''}`}
          >
            {amt}▶
          </button>
        ))}
      </div>
    </div>
  );
});

export default DJBeatJump;
