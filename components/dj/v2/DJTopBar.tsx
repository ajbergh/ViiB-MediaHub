/**
 * ViiB MediaHub - DJ Top Bar Component (v2)
 * 
 * Professional top navigation bar with view mode tabs, record button, and track info.
 * Styled to match professional DJ software (PCDJ DEX / Serato).
 * 
 * @module components/dj/v2/DJTopBar
 */

import React, { useEffect, useCallback, useState } from 'react';
import { Maximize2, Minimize2 } from 'lucide-react';
import { useStore } from '../../../store';
import type { DJLayoutMode } from '../../../slices/djMixerSlice';

type ViewMode = 'timeline' | 'scope' | 'racks';

interface DJTopBarProps {
  viewMode: ViewMode;
  onViewModeChange: (mode: ViewMode) => void;
  isRecording?: boolean;
  onRecordToggle?: () => void;
  layoutMode?: DJLayoutMode;
  onLayoutModeChange?: (mode: DJLayoutMode) => void;
}

/** Persistent fullscreen toggle shown in the top-bar action row. */
const FullscreenButton: React.FC = () => {
  const [isFS, setIsFS] = useState(() =>
    typeof document !== 'undefined' ? !!document.fullscreenElement : false
  );

  useEffect(() => {
    const onFSChange = () => setIsFS(!!document.fullscreenElement);
    document.addEventListener('fullscreenchange', onFSChange);
    return () => document.removeEventListener('fullscreenchange', onFSChange);
  }, []);

  const toggle = useCallback(async () => {
    try {
      if (document.fullscreenElement) {
        await document.exitFullscreen();
      } else {
        await document.documentElement.requestFullscreen();
      }
    } catch {
      // Fullscreen not available (e.g. Wails WebView) — ignore
    }
  }, []);

  return (
    <button
      type="button"
      onClick={toggle}
      aria-label={isFS ? 'Exit fullscreen' : 'Enter fullscreen (F11)'}
      title={isFS ? 'Exit fullscreen' : 'Enter fullscreen (F11)'}
      className={`
        ml-1 flex items-center justify-center w-8 h-8 rounded
        transition-all duration-100 border
        ${isFS
          ? 'bg-brand/20 text-brand border-brand/40 hover:bg-brand/30'
          : 'bg-[#222] text-neutral-500 border-[#333] hover:bg-[#2a2a2a] hover:text-neutral-300 hover:border-[#444]'}
      `}
    >
      {isFS ? <Minimize2 size={14} aria-hidden="true" /> : <Maximize2 size={14} aria-hidden="true" />}
    </button>
  );
};

export const DJTopBar: React.FC<DJTopBarProps> = ({ 
  viewMode, 
  onViewModeChange,
  isRecording = false,
  onRecordToggle,
  layoutMode = 'perf',
  onLayoutModeChange,
}) => {
  // Granular selectors - avoid subscribing to position/volume/eq changes
  const deckATrack = useStore(state => state.djDeckA.track);
  const deckBTrack = useStore(state => state.djDeckB.track);

  return (
    <div className="h-11 bg-gradient-to-b from-[#1f1f1f] to-[#1a1a1a] border-b border-[#2a2a2a] flex items-center justify-between px-3">
      {/* Left - View Mode Tabs + Layout Mode Toggle */}
      <div className="flex items-center gap-1">
        {(['scope', 'timeline', 'racks'] as ViewMode[]).map(mode => (
          <button
            key={mode}
            onClick={() => onViewModeChange(mode)}
            aria-pressed={viewMode === mode}
            className={`
              px-3 min-h-[32px] flex items-center rounded text-[10px] font-bold uppercase tracking-wider
              transition-all duration-100 border
              ${viewMode === mode
                ? 'bg-[#333] text-white border-[#4a4a4a] shadow-inner'
                : 'bg-[#222] text-[#777] border-[#333] hover:text-white hover:bg-[#2a2a2a] hover:border-[#444]'}
            `}
          >
            {mode}
          </button>
        ))}

        {/* Separator */}
        <div className='w-px h-5 bg-[#333] mx-1.5' />

        {/* Layout Mode Toggle */}
        <div className='flex items-center bg-[#1a1a1a] rounded border border-[#333] p-0.5'>
          {(['perf', 'browse', 'fx'] as DJLayoutMode[]).map(mode => (
            <button
              key={mode}
              onClick={() => onLayoutModeChange?.(mode)}
              aria-pressed={layoutMode === mode}
              className={`
                px-2.5 min-h-[32px] flex items-center rounded text-[10px] font-bold uppercase tracking-wider
                transition-all duration-150
                ${layoutMode === mode
                  ? 'bg-brand text-white shadow-sm'
                  : 'text-[#666] hover:text-[#aaa]'}
              `}
              title={mode === 'perf' ? 'Performance layout — full decks' : mode === 'browse' ? 'Browse layout — expanded library' : 'FX layout — expanded effects'}
            >
              {mode === 'perf' ? 'PERF' : mode === 'browse' ? 'BROWSE' : 'FX'}
            </button>
          ))}
        </div>
      </div>

      {/* Center - Track Info + Record. (Times moved to deck headers — single source.) */}
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
                <div className="text-[10px] text-neutral-500 truncate">
                  {deckATrack.title}
                </div>
              </div>
            </>
          )}
        </div>

        {/* Record Button */}
        <button
          onClick={onRecordToggle}
          aria-label={isRecording ? 'Stop recording' : 'Start recording'}
          aria-pressed={isRecording}
          className={`
            flex items-center gap-2 px-3 py-1.5 min-h-[32px] rounded
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
          {deckBTrack && (
            <>
              <div className="max-w-[180px]">
                <div className="text-[10px] text-neutral-300 truncate font-medium">
                  {deckBTrack.artist}
                </div>
                <div className="text-[10px] text-neutral-500 truncate">
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

      {/* Right - Fullscreen toggle (zoom is handled via Ctrl+Scroll on the waveform — see help dialog) */}
      <div className="flex items-center gap-2 text-neutral-500">
        <FullscreenButton />
      </div>
    </div>
  );
};

export default DJTopBar;
