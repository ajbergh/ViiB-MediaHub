/**
 * ViiB MediaHub - DJ Loop Section Component (v2)
 * 
 * Compact loop controls with beat-synced presets.
 * 
 * @module components/dj/v2/DJLoopSection
 */

import React, { useCallback, useState } from 'react';
import { useStore } from '../../../store';
import { useDJAudioEngineActions } from '../../../hooks/useDJAudioEngine';
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
  const [selectedBeats, setSelectedBeats] = useState(4);
  
  const { setLoopBeats, toggleLoop, halveLoop, doubleLoop } = useDJAudioEngineActions();
  
  const bpm = effectiveBpm || originalBpm || 120;
  
  // Calculate current loop size in beats
  const loopLengthSeconds = loop.end > loop.start ? loop.end - loop.start : 0;
  const currentLoopBeats = loopLengthSeconds > 0
    ? loopLengthSeconds / (60 / bpm) 
    : 0;

  const handleSetLoop = useCallback((beats: number) => {
    setSelectedBeats(beats);
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
  const displaySize = currentLoopBeats > 0 && currentSizeIndex >= 0
    ? formatBeatSize(LOOP_SIZES[currentSizeIndex])
    : currentLoopBeats > 0
      ? currentLoopBeats.toFixed(1)
      : formatBeatSize(selectedBeats);

  const validLoop = loop.end > loop.start;

  return (
    <div className="flex flex-col items-center gap-1 px-2 flex-shrink-0">
      <div className="flex items-center gap-1">
        {/* Halve button */}
        <button
          onClick={() => {
            if (validLoop && loop.enabled) halveLoop(deck);
            else handleSetLoop(Math.max(0.25, selectedBeats / 2));
          }}
          disabled={!track}
          aria-label="Halve loop"
          title="Halve loop"
          className={`
            w-11 h-11 rounded flex items-center justify-center flex-shrink-0
            transition-all duration-100
            ${track
              ? 'bg-[#2a2a2a] hover:bg-[#3a3a3a] text-neutral-400'
              : 'bg-[#222] text-neutral-700 cursor-not-allowed'}
          `}
        >
          <ChevronLeft size={20} aria-hidden />
        </button>

        {/* Loop size display / toggle */}
        <button
          onClick={() => validLoop ? toggleLoop(deck) : handleSetLoop(selectedBeats)}
          disabled={!track}
          aria-pressed={loop.enabled && validLoop}
          title={validLoop ? 'Toggle loop' : `Create ${selectedBeats}-beat loop`}
          className={`
            w-16 h-11 rounded text-[13px] font-bold flex-shrink-0
            transition-all duration-100
            ${!track
              ? 'bg-[#222] text-neutral-700 cursor-not-allowed'
              : loop.enabled && validLoop
                ? 'bg-green-500 text-white'
                : 'bg-[#2a2a2a] text-neutral-400 hover:bg-[#3a3a3a]'}
          `}
          style={{
            boxShadow: loop.enabled && validLoop ? '0 0 8px rgba(34, 197, 94, 0.4)' : undefined,
          }}
        >
          {displaySize}
        </button>

        {/* Double button */}
        <button
          onClick={() => {
            if (validLoop && loop.enabled) doubleLoop(deck);
            else handleSetLoop(Math.min(8, selectedBeats * 2));
          }}
          disabled={!track}
          aria-label="Double loop"
          title="Double loop"
          className={`
            w-11 h-11 rounded flex items-center justify-center flex-shrink-0
            transition-all duration-100
            ${track
              ? 'bg-[#2a2a2a] hover:bg-[#3a3a3a] text-neutral-400'
              : 'bg-[#222] text-neutral-700 cursor-not-allowed'}
          `}
        >
          <ChevronRight size={20} aria-hidden />
        </button>
      </div>
      <div className="flex items-center gap-0.5" aria-label="Loop beat length">
        {LOOP_SIZES.map(beats => (
          <button
            key={beats}
            onClick={() => handleSetLoop(beats)}
            disabled={!track}
            aria-pressed={selectedBeats === beats}
            className={`rounded px-1 py-0.5 text-[9px] ${selectedBeats === beats ? 'bg-neutral-600 text-white' : 'text-neutral-500 hover:text-neutral-200'} disabled:opacity-40`}
            title={`Set ${beats} beat loop`}
          >{formatBeatSize(beats)}</button>
        ))}
      </div>
    </div>
  );
};

export default React.memo(DJLoopSection);
