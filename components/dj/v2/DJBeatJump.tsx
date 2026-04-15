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
  /**
   * compact=true renders 4 flat inline buttons (◀4 ◀1 1▶ 4▶) without label —
   * designed for use in the deck controls bar (DJay Pro-style)
   */
  compact?: boolean;
}

const BEAT_AMOUNTS = [1, 4, 8, 16];

export const DJBeatJump = memo(function DJBeatJump({ deck, compact }: DJBeatJumpProps) {
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
  const isA = deck === 'A';

  const hoverClasses = isA
    ? 'hover:bg-blue-500/20 hover:text-blue-300 hover:border-blue-500/40'
    : 'hover:bg-purple-500/20 hover:text-purple-300 hover:border-purple-500/40';

  if (compact) {
    // DJay Pro-style: 4 flat inline buttons (◀4 ◀1 1▶ 4▶)
    const COMPACT_AMOUNTS = [4, 1] as const;
    return (
      <div className='flex items-center gap-0.5'>
        {[...COMPACT_AMOUNTS].reverse().map(amt => (
          <button
            key={`c-back-${amt}`}
            onClick={() => handleJump(-amt)}
            disabled={disabled}
            className={`
              w-8 h-7 rounded text-[9px] font-bold flex items-center justify-center
              transition-all duration-75 border
              ${disabled
                ? 'bg-[#1a1a1a] text-neutral-700 border-[#222] cursor-not-allowed'
                : `bg-[#1e1e1e] text-neutral-400 border-[#333] ${hoverClasses} active:scale-95`
              }
            `}
            title={`Jump back ${amt} beat${amt > 1 ? 's' : ''}`}
          >
            ◀{amt}
          </button>
        ))}
        {COMPACT_AMOUNTS.map(amt => (
          <button
            key={`c-fwd-${amt}`}
            onClick={() => handleJump(amt)}
            disabled={disabled}
            className={`
              w-8 h-7 rounded text-[9px] font-bold flex items-center justify-center
              transition-all duration-75 border
              ${disabled
                ? 'bg-[#1a1a1a] text-neutral-700 border-[#222] cursor-not-allowed'
                : `bg-[#1e1e1e] text-neutral-400 border-[#333] ${hoverClasses} active:scale-95`
              }
            `}
            title={`Jump forward ${amt} beat${amt > 1 ? 's' : ''}`}
          >
            {amt}▶
          </button>
        ))}
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-1 w-full px-2">
      <span className="text-[9px] font-bold text-[#555] uppercase tracking-wider text-center">BEAT JUMP</span>
      <div className="grid grid-cols-4 gap-1">
        {/* Backward jumps */}
        {[...BEAT_AMOUNTS].reverse().map(amt => (
          <button
            key={`back-${amt}`}
            onClick={() => handleJump(-amt)}
            disabled={disabled}
            className={`
              h-7 rounded text-[9px] font-bold flex items-center justify-center
              transition-all duration-75 border
              ${disabled
                ? 'bg-[#1a1a1a] text-neutral-700 border-[#222] cursor-not-allowed'
                : `bg-[#1e1e1e] text-neutral-400 border-[#333] ${hoverClasses} active:scale-95`
              }
            `}
            title={`Jump back ${amt} beat${amt > 1 ? 's' : ''}`}
          >
            ◀{amt}
          </button>
        ))}
        
        {/* Forward jumps */}
        {BEAT_AMOUNTS.map(amt => (
          <button
            key={`fwd-${amt}`}
            onClick={() => handleJump(amt)}
            disabled={disabled}
            className={`
              h-7 rounded text-[9px] font-bold flex items-center justify-center
              transition-all duration-75 border
              ${disabled
                ? 'bg-[#1a1a1a] text-neutral-700 border-[#222] cursor-not-allowed'
                : `bg-[#1e1e1e] text-neutral-400 border-[#333] ${hoverClasses} active:scale-95`
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
