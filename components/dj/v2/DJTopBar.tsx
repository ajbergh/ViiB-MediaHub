/**
 * ViiB MediaHub - DJ Top Bar Component (v2)
 * 
 * Professional top navigation bar with view mode tabs, record button, and track info.
 * Styled to match professional DJ software (PCDJ DEX / Serato).
 * 
 * @module components/dj/v2/DJTopBar
 */

import React, { useRef, useEffect } from 'react';
import { useStore } from '../../../store';

type ViewMode = 'timeline' | 'scope' | 'fx';

/** Self-subscribing time display — reads position via getState() in RAF, zero React re-renders */
const TopBarTimeDisplay: React.FC<{ deck: 'A' | 'B'; colorClass: string }> = React.memo(({ deck, colorClass }) => {
  const spanRef = useRef<HTMLSpanElement>(null);
  const isPlaying = useStore(state => deck === 'A' ? state.djDeckA.isPlaying : state.djDeckB.isPlaying);

  useEffect(() => {
    let rafId: number;
    const update = () => {
      const pos = deck === 'A'
        ? useStore.getState().djDeckA.position
        : useStore.getState().djDeckB.position;
      if (spanRef.current) {
        const mins = Math.floor(pos / 60);
        const secs = Math.floor(pos % 60);
        spanRef.current.textContent = `${mins.toString().padStart(2, '0')}:${secs.toString().padStart(2, '0')}`;
      }
      rafId = requestAnimationFrame(update);
    };
    rafId = requestAnimationFrame(update);
    return () => cancelAnimationFrame(rafId);
  }, [deck]);

  return (
    <span
      ref={spanRef}
      className={`font-mono text-sm tabular-nums px-2 py-0.5 rounded ${
        isPlaying ? colorClass : 'text-[#555]'
      }`}
    >
      00:00
    </span>
  );
});
TopBarTimeDisplay.displayName = 'TopBarTimeDisplay';

interface DJTopBarProps {
  viewMode: ViewMode;
  onViewModeChange: (mode: ViewMode) => void;
  isRecording?: boolean;
  onRecordToggle?: () => void;
}

export const DJTopBar: React.FC<DJTopBarProps> = ({ 
  viewMode, 
  onViewModeChange,
  isRecording = false,
  onRecordToggle
}) => {
  // Granular selectors - avoid subscribing to position/volume/eq changes
  const deckATrack = useStore(state => state.djDeckA.track);
  const deckBTrack = useStore(state => state.djDeckB.track);

  return (
    <div className="h-11 bg-gradient-to-b from-[#1f1f1f] to-[#1a1a1a] border-b border-[#2a2a2a] flex items-center justify-between px-3">
      {/* Left - View Mode Tabs */}
      <div className="flex items-center gap-1">
        {(['scope', 'timeline', 'fx'] as ViewMode[]).map(mode => (
          <button
            key={mode}
            onClick={() => onViewModeChange(mode)}
            className={`
              px-3 py-1.5 rounded text-[10px] font-bold uppercase tracking-wider
              transition-all duration-100 border
              ${viewMode === mode
                ? 'bg-[#333] text-white border-[#4a4a4a] shadow-inner'
                : 'bg-[#222] text-[#777] border-[#333] hover:text-white hover:bg-[#2a2a2a] hover:border-[#444]'}
            `}
          >
            {mode}
          </button>
        ))}
      </div>

      {/* Center - Track Info + Record */}
      <div className="flex items-center gap-6">
        {/* Deck A Info */}
        <div className="flex items-center gap-2">
          {deckATrack && (
            <>
              <div className="w-7 h-7 bg-[#252525] rounded overflow-hidden flex-shrink-0 border border-[#333]">
                {deckATrack.coverUrl ? (
                  <img 
                    src={deckATrack.coverUrl} 
                    alt=""
                    className="w-full h-full object-cover"
                  />
                ) : (
                  <div className="w-full h-full flex items-center justify-center text-[10px] text-neutral-600">♫</div>
                )}
              </div>
              <div className="max-w-[180px] text-right">
                <div className="text-[10px] text-neutral-300 truncate font-medium">
                  {deckATrack.artist}
                </div>
                <div className="text-[9px] text-neutral-500 truncate">
                  {deckATrack.title}
                </div>
              </div>
            </>
          )}
          <TopBarTimeDisplay deck="A" colorClass="bg-[#3b82f6]/20 text-[#3b82f6]" />
        </div>

        {/* Record Button */}
        <button
          onClick={onRecordToggle}
          className={`
            flex items-center gap-2 px-3 py-1.5 rounded
            transition-all duration-100 border
            ${isRecording
              ? 'bg-red-900/30 border-red-500/50 shadow-lg shadow-red-500/20'
              : 'bg-[#252525] border-[#333] hover:bg-[#2a2a2a] hover:border-[#444]'}
          `}
        >
          <div 
            className={`w-2.5 h-2.5 rounded-full transition-colors ${
              isRecording ? 'bg-red-500 animate-pulse shadow-lg shadow-red-500/50' : 'bg-[#555]'
            }`}
          />
          <span className={`text-[10px] font-bold tracking-wider ${
            isRecording ? 'text-red-400' : 'text-[#666]'
          }`}>
            REC
          </span>
        </button>

        {/* Deck B Info */}
        <div className="flex items-center gap-2">
          <TopBarTimeDisplay deck="B" colorClass="bg-[#8b5cf6]/20 text-[#8b5cf6]" />
          {deckBTrack && (
            <>
              <div className="max-w-[180px]">
                <div className="text-[10px] text-neutral-300 truncate font-medium">
                  {deckBTrack.artist}
                </div>
                <div className="text-[9px] text-neutral-500 truncate">
                  {deckBTrack.title}
                </div>
              </div>
              <div className="w-7 h-7 bg-[#252525] rounded overflow-hidden flex-shrink-0 border border-[#333]">
                {deckBTrack.coverUrl ? (
                  <img 
                    src={deckBTrack.coverUrl} 
                    alt=""
                    className="w-full h-full object-cover"
                  />
                ) : (
                  <div className="w-full h-full flex items-center justify-center text-[10px] text-neutral-600">♫</div>
                )}
              </div>
            </>
          )}
        </div>
      </div>

      {/* Right - Zoom Control */}
      <div className="flex items-center gap-2 text-neutral-500">
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
          <circle cx="11" cy="11" r="8"/>
          <path d="m21 21-4.35-4.35"/>
          <path d="M8 11h6"/>
        </svg>
        <div className="w-20 h-1.5 bg-[#252525] rounded-full relative">
          <div className="absolute inset-y-0 left-0 w-1/2 bg-[#444] rounded-full" />
          <div className="absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 w-3 h-3 bg-[#666] rounded-full border border-[#888]" />
        </div>
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
          <circle cx="11" cy="11" r="8"/>
          <path d="m21 21-4.35-4.35"/>
          <path d="M8 11h6M11 8v6"/>
        </svg>
      </div>
    </div>
  );
};

export default DJTopBar;
