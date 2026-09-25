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
import { calculateBeatJumpTimestamp, getBeatJumpControlAmounts } from '../../../lib/beatJump';

interface DJBeatJumpProps {
  deck: DeckId;
  /**
   * compact=true renders 6 flat inline buttons (◀32 ◀4 ◀1 1▶ 4▶ 32▶) without label —
   * designed for use in the deck controls bar (DJay Pro-style)
   */
  compact?: boolean;
}

function beatJumpTitle(beats: number): string {
  const direction = beats < 0 ? 'back' : 'forward';
  const amount = Math.abs(beats);
  const unit = amount === 1 ? 'beat' : 'beats';
  return `Jump ${direction} ${amount} ${unit}; uses stored beat grid when available, otherwise BPM`;
}

export const DJBeatJump = memo(function DJBeatJump({ deck, compact }: DJBeatJumpProps) {
  // Granular selectors — avoid re-renders from position updates
  const track = useStore(state => deck === 'A' ? state.djDeckA.track : state.djDeckB.track);
  const duration = useStore(state => deck === 'A' ? state.djDeckA.duration : state.djDeckB.duration);
  const effectiveBpm = useStore(state => deck === 'A' ? state.djDeckA.effectiveBpm : state.djDeckB.effectiveBpm);
  const originalBpm = useStore(state => deck === 'A' ? state.djDeckA.originalBpm : state.djDeckB.originalBpm);
  const beatGrid = useStore(state => deck === 'A' ? state.djDeckA.beatGrid : state.djDeckB.beatGrid);
  const beatGridOffset = useStore(state => deck === 'A' ? state.djDeckA.beatGridOffset : state.djDeckB.beatGridOffset);
  const { seek } = useDJAudioEngineActions();

  const handleJump = useCallback((beats: number) => {
    if (!track || !duration) return;
    
    const bpm = effectiveBpm || originalBpm;
    
    // Read position from store snapshot — not reactive, avoids dep on position
    const position = deck === 'A'
      ? useStore.getState().djDeckA.position
      : useStore.getState().djDeckB.position;
    
    const newPosition = calculateBeatJumpTimestamp({ position, beats, duration, beatGrid, beatGridOffset, bpm });
    if (newPosition !== null) seek(deck, newPosition);
  }, [deck, track, duration, effectiveBpm, originalBpm, beatGrid, beatGridOffset, seek]);

  const disabled = !track;
  const isA = deck === 'A';

  const hoverClasses = isA
    ? 'hover:bg-blue-500/20 hover:text-blue-300 hover:border-blue-500/40'
    : 'hover:bg-purple-500/20 hover:text-purple-300 hover:border-purple-500/40';

  if (compact) {
    // Compact deck row exposes short jumps plus a generic 32-beat jump.
    return (
      <div className='flex items-center gap-0.5'>
        {getBeatJumpControlAmounts(true, 'back').map(amt => (
          <button
            key={`c-back-${Math.abs(amt)}`}
            onClick={() => handleJump(amt)}
            disabled={disabled}
            className={`
              w-11 h-11 rounded text-[11px] font-bold flex items-center justify-center
              transition-all duration-75 border
              ${disabled
                ? 'bg-[#1a1a1a] text-neutral-700 border-[#222] cursor-not-allowed'
                : `bg-[#1e1e1e] text-neutral-400 border-[#333] ${hoverClasses} active:scale-95`
              }
            `}
            title={beatJumpTitle(amt)}
          >
            ◀{Math.abs(amt)}
          </button>
        ))}
        {getBeatJumpControlAmounts(true, 'forward').map(amt => (
          <button
            key={`c-fwd-${amt}`}
            onClick={() => handleJump(amt)}
            disabled={disabled}
            className={`
              w-11 h-11 rounded text-[11px] font-bold flex items-center justify-center
              transition-all duration-75 border
              ${disabled
                ? 'bg-[#1a1a1a] text-neutral-700 border-[#222] cursor-not-allowed'
                : `bg-[#1e1e1e] text-neutral-400 border-[#333] ${hoverClasses} active:scale-95`
              }
            `}
            title={beatJumpTitle(amt)}
          >
            {amt}▶
          </button>
        ))}
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-1 w-full px-2">
      <span className="text-[10px] font-bold text-[#555] uppercase tracking-wider text-center">BEAT JUMP</span>
      <div className="grid grid-cols-5 gap-1">
        {/* Backward jumps */}
        {getBeatJumpControlAmounts(false, 'back').map(amt => (
          <button
            key={`back-${Math.abs(amt)}`}
            onClick={() => handleJump(amt)}
            disabled={disabled}
            className={`
              h-8 rounded text-[10px] font-bold flex items-center justify-center
              transition-all duration-75 border
              ${disabled
                ? 'bg-[#1a1a1a] text-neutral-700 border-[#222] cursor-not-allowed'
                : `bg-[#1e1e1e] text-neutral-400 border-[#333] ${hoverClasses} active:scale-95`
              }
            `}
            title={beatJumpTitle(amt)}
          >
            ◀{Math.abs(amt)}
          </button>
        ))}
        
        {/* Forward jumps */}
        {getBeatJumpControlAmounts(false, 'forward').map(amt => (
          <button
            key={`fwd-${amt}`}
            onClick={() => handleJump(amt)}
            disabled={disabled}
            className={`
              h-8 rounded text-[10px] font-bold flex items-center justify-center
              transition-all duration-75 border
              ${disabled
                ? 'bg-[#1a1a1a] text-neutral-700 border-[#222] cursor-not-allowed'
                : `bg-[#1e1e1e] text-neutral-400 border-[#333] ${hoverClasses} active:scale-95`
              }
            `}
            title={beatJumpTitle(amt)}
          >
            {amt}▶
          </button>
        ))}
      </div>
    </div>
  );
});

export default DJBeatJump;
