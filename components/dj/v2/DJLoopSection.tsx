/**
 * ViiB MediaHub - DJ Loop Section Component (v2)
 * 
 * Compact loop controls with beat-synced presets.
 * 
 * @module components/dj/v2/DJLoopSection
 */

import React, { useCallback } from 'react';
import { useStore } from '../../../store';
import { useDJAudioEngine } from '../../../hooks/useDJAudioEngine';
import type { DeckId } from '../../../slices/djMixerSlice';
import { ChevronLeft, ChevronRight } from 'lucide-react';

interface DJLoopSectionProps {
  deck: DeckId;
}

const LOOP_SIZES = [0.25, 0.5, 1, 2, 4, 8];

export const DJLoopSection: React.FC<DJLoopSectionProps> = ({ deck }) => {
  const loop = useStore(state => deck === 'A' ? state.djDeckA.loop : state.djDeckB.loop);
  const effectiveBpm = useStore(state => deck === 'A' ? state.djDeckA.effectiveBpm : state.djDeckB.effectiveBpm);
  const originalBpm = useStore(state => deck === 'A' ? state.djDeckA.originalBpm : state.djDeckB.originalBpm);
  const track = useStore(state => deck === 'A' ? state.djDeckA.track : state.djDeckB.track);
  
  const { setLoopBeats, toggleLoop, halveLoop, doubleLoop } = useDJAudioEngine();
  
  const bpm = effectiveBpm || originalBpm;
  
  // Calculate current loop size in beats
  const loopLengthSeconds = loop.end > loop.start ? loop.end - loop.start : 0;
  const currentLoopBeats = bpm && loopLengthSeconds > 0 
    ? loopLengthSeconds / (60 / bpm) 
    : 0;

  const handleSetLoop = useCallback((beats: number) => {
    setLoopBeats(deck, beats);
  }, [deck, setLoopBeats]);

  const formatBeatSize = (beats: number): string => {
    if (beats < 1) return `1/${Math.round(1/beats)}`;
    return beats.toString();
  };

  // Find current loop size index for display
  const currentSizeIndex = LOOP_SIZES.findIndex(size => 
    Math.abs(size - currentLoopBeats) < 0.1
  );
  const displaySize = currentSizeIndex >= 0 
    ? formatBeatSize(LOOP_SIZES[currentSizeIndex])
    : currentLoopBeats > 0 
      ? currentLoopBeats.toFixed(1)
      : '---';

  return (
    <div className="flex items-center gap-1">
      {/* Loop label */}
      <span className="text-[10px] text-neutral-500 font-medium mr-1">LOOP</span>
      
      {/* Halve button */}
      <button
        onClick={() => halveLoop(deck)}
        disabled={!track || !loop.enabled}
        className={`
          w-6 h-6 rounded flex items-center justify-center
          transition-all duration-100
          ${track && loop.enabled
            ? 'bg-[#2a2a2a] hover:bg-[#3a3a3a] text-neutral-400'
            : 'bg-[#222] text-neutral-700 cursor-not-allowed'}
        `}
        title="Halve loop"
      >
        <ChevronLeft size={14} />
      </button>
      
      {/* Loop size display / toggle */}
      <button
        onClick={() => toggleLoop(deck)}
        disabled={!track}
        className={`
          w-10 h-6 rounded text-[11px] font-bold
          transition-all duration-100
          ${!track
            ? 'bg-[#222] text-neutral-700 cursor-not-allowed'
            : loop.enabled
              ? 'bg-green-500 text-white'
              : 'bg-[#2a2a2a] text-neutral-400 hover:bg-[#3a3a3a]'}
        `}
        style={{
          boxShadow: loop.enabled ? '0 0 8px rgba(34, 197, 94, 0.4)' : undefined,
        }}
        title="Toggle loop"
      >
        {displaySize}
      </button>
      
      {/* Double button */}
      <button
        onClick={() => doubleLoop(deck)}
        disabled={!track || !loop.enabled}
        className={`
          w-6 h-6 rounded flex items-center justify-center
          transition-all duration-100
          ${track && loop.enabled
            ? 'bg-[#2a2a2a] hover:bg-[#3a3a3a] text-neutral-400'
            : 'bg-[#222] text-neutral-700 cursor-not-allowed'}
        `}
        title="Double loop"
      >
        <ChevronRight size={14} />
      </button>
    </div>
  );
};

export default React.memo(DJLoopSection);
