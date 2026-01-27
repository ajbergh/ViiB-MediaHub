/**
 * ViiB MediaHub - DJ Loop Panel Component
 * 
 * Provides loop controls for each deck:
 * - Loop In/Out buttons
 * - Beat-synced loop sizes (1/4, 1/2, 1, 2, 4, 8, 16 beats)
 * - Loop toggle and clear
 * - Loop size double/halve
 * 
 * @module components/dj/DJLoopPanel
 */

import React, { useCallback, useState } from 'react';
import { useStore } from '../../store';
import { useDJAudioEngine } from '../../hooks/useDJAudioEngine';
import { ChevronDown, ChevronUp } from 'lucide-react';
import type { DeckId } from '../../slices/djMixerSlice';

interface DJLoopPanelProps {
  deck: DeckId;
  defaultCollapsed?: boolean;
}

// Beat sizes for loop buttons
const LOOP_BEAT_SIZES = [0.25, 0.5, 1, 2, 4, 8, 16];

export const DJLoopPanel: React.FC<DJLoopPanelProps> = ({ deck, defaultCollapsed = false }) => {
  const [isCollapsed, setIsCollapsed] = useState(defaultCollapsed);
  // Only subscribe to loop and BPM state, not the entire deck state (avoids re-renders on position updates)
  const loop = useStore(state => deck === 'A' ? state.djDeckA.loop : state.djDeckB.loop);
  const effectiveBpm = useStore(state => deck === 'A' ? state.djDeckA.effectiveBpm : state.djDeckB.effectiveBpm);
  const originalBpm = useStore(state => deck === 'A' ? state.djDeckA.originalBpm : state.djDeckB.originalBpm);
  const { 
    setLoopIn, 
    setLoopOut, 
    toggleLoop, 
    clearLoop, 
    setLoopBeats,
    doubleLoop,
    halveLoop 
  } = useDJAudioEngine();
  const bpm = effectiveBpm || originalBpm;
  
  // Calculate current loop length in beats
  const loopLengthSeconds = loop.end > loop.start ? loop.end - loop.start : 0;
  const loopLengthBeats = bpm && loopLengthSeconds > 0 
    ? loopLengthSeconds / (60 / bpm) 
    : 0;

  const handleLoopIn = useCallback(() => {
    setLoopIn(deck);
  }, [deck, setLoopIn]);

  const handleLoopOut = useCallback(() => {
    setLoopOut(deck);
  }, [deck, setLoopOut]);

  const handleToggleLoop = useCallback(() => {
    toggleLoop(deck);
  }, [deck, toggleLoop]);

  const handleClearLoop = useCallback(() => {
    clearLoop(deck);
  }, [deck, clearLoop]);

  const handleSetLoopBeats = useCallback((beats: number) => {
    setLoopBeats(deck, beats);
  }, [deck, setLoopBeats]);

  const handleDoubleLoop = useCallback(() => {
    doubleLoop(deck);
  }, [deck, doubleLoop]);

  const handleHalveLoop = useCallback(() => {
    halveLoop(deck);
  }, [deck, halveLoop]);

  // Format beat size for display
  const formatBeatSize = (beats: number): string => {
    if (beats < 1) {
      return `1/${Math.round(1/beats)}`;
    }
    return beats.toString();
  };

  return (
    <div className="bg-surface-1 rounded-lg p-2">
      {/* Header with collapse toggle */}
      <div 
        className="flex items-center justify-between cursor-pointer select-none"
        onClick={() => setIsCollapsed(!isCollapsed)}
      >
        <h3 className="text-xs font-bold text-neutral-300 uppercase tracking-wider">
          Loop - Deck {deck}
        </h3>
        <div className="flex items-center gap-2">
          {/* Loop active indicator */}
          {loop.enabled && (
            <span className="text-[10px] font-bold text-green-400 bg-green-500/20 px-1.5 py-0.5 rounded">
              {formatBeatSize(loopLengthBeats)}
            </span>
          )}
          {isCollapsed ? (
            <ChevronDown size={14} className="text-neutral-400" />
          ) : (
            <ChevronUp size={14} className="text-neutral-400" />
          )}
        </div>
      </div>

      {/* Collapsible content */}
      {!isCollapsed && (
        <>
          {/* Loop In/Out buttons */}
          <div className="flex gap-2 mt-2 mb-2">
            <button
              onClick={handleLoopIn}
              className={`
                flex-1 py-1.5 rounded text-xs font-bold uppercase
                transition-all duration-150
                ${loop.start > 0 
                  ? 'bg-green-500/30 text-green-300 border border-green-500/50' 
                  : 'bg-surface-2 text-neutral-400 hover:bg-surface-0'
                }
              `}
            >
              IN
            </button>
            <button
              onClick={handleLoopOut}
              className={`
                flex-1 py-1.5 rounded text-xs font-bold uppercase
                transition-all duration-150
                ${loop.end > loop.start 
                  ? 'bg-yellow-500/30 text-yellow-300 border border-yellow-500/50' 
                  : 'bg-surface-2 text-neutral-400 hover:bg-surface-0'
                }
              `}
            >
              OUT
            </button>
            <button
              onClick={handleToggleLoop}
              className={`
                flex-1 py-1.5 rounded text-xs font-bold uppercase
                transition-all duration-150
                ${loop.enabled
                  ? 'bg-brand text-white shadow-lg shadow-brand/30'
                  : 'bg-surface-2 text-neutral-400 hover:bg-surface-0'
                }
              `}
            >
              {loop.enabled ? '🔁' : 'LOOP'}
            </button>
          </div>

          {/* Beat-synced loop sizes */}
          <div className="flex gap-1 mb-2">
            {LOOP_BEAT_SIZES.map((beats) => (
              <button
                key={beats}
                onClick={() => handleSetLoopBeats(beats)}
                className={`
                  flex-1 py-1 rounded text-[9px] font-bold
                  transition-all duration-150
                  ${Math.abs(loopLengthBeats - beats) < 0.1 && loop.enabled
                    ? 'bg-brand text-white'
                    : 'bg-surface-2 text-neutral-400 hover:bg-surface-0'
                  }
                `}
              >
                {formatBeatSize(beats)}
              </button>
            ))}
          </div>

          {/* Loop size controls */}
          <div className="flex gap-2">
            <button
              onClick={handleHalveLoop}
              disabled={!loop.enabled}
              className={`
                flex-1 py-1 rounded text-xs font-bold
                transition-all duration-150
                ${loop.enabled
                  ? 'bg-surface-2 text-neutral-300 hover:bg-surface-0'
                  : 'bg-surface-2/50 text-neutral-600 cursor-not-allowed'
                }
              `}
            >
              ÷2
            </button>
            <button
              onClick={handleClearLoop}
              disabled={!loop.enabled && loop.start === 0}
              className={`
                flex-1 py-1 rounded text-xs font-bold
                transition-all duration-150
                ${(loop.enabled || loop.start > 0)
                  ? 'bg-red-500/30 text-red-300 hover:bg-red-500/40'
                  : 'bg-surface-2/50 text-neutral-600 cursor-not-allowed'
                }
              `}
            >
              CLEAR
            </button>
            <button
              onClick={handleDoubleLoop}
              disabled={!loop.enabled}
              className={`
                flex-1 py-1 rounded text-xs font-bold
                transition-all duration-150
                ${loop.enabled
                  ? 'bg-surface-2 text-neutral-300 hover:bg-surface-0'
                  : 'bg-surface-2/50 text-neutral-600 cursor-not-allowed'
                }
              `}
            >
              ×2
            </button>
          </div>
        </>
      )}
    </div>
  );
};

function formatTime(seconds: number): string {
  const mins = Math.floor(seconds / 60);
  const secs = Math.floor(seconds % 60);
  return `${mins}:${secs.toString().padStart(2, '0')}`;
}

export default DJLoopPanel;
