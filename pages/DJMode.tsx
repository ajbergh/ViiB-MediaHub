/**
 * ViiB MediaHub - DJ Mode Page
 * 
 * Main page component for DJ Mode - a two-deck mixing interface.
 * Provides deck controls, waveforms, mixer, and library browser
 * in a professional DJ layout.
 * 
 * Layout:
 * ┌─────────────────────────────────────────────────┐
 * │              TOP BAR (BPM, Key, Time)           │
 * ├─────────────────────────────────────────────────┤
 * │   Waveform A          │         Waveform B     │
 * ├─────────────┬─────────┼─────────┬──────────────┤
 * │   Deck A    │  Mixer  │         │    Deck B    │
 * │   Controls  │ EQ/Xfdr │         │   Controls   │
 * ├─────────────┴─────────┴─────────┴──────────────┤
 * │               Library Browser                   │
 * └─────────────────────────────────────────────────┘
 * 
 * @module pages/DJMode
 */

import React, { useEffect, useCallback, useState } from 'react';
import { useStore } from '../store';
import { useDJAudioEngine } from '../hooks/useDJAudioEngine';
import { DeckView, DJWaveform, DJMixer, DJLibraryBrowser, DJHotCues, DJFXPanel, DJLoopPanel } from '../components/dj';
import { ChevronDown, ChevronUp } from 'lucide-react';
import type { DeckId } from '../slices/djMixerSlice';

export const DJMode: React.FC = () => {
  const {
    djMixerEnabled,
    setDJMixerEnabled,
    djActiveDeck,
    setActiveDeck,
    toggleActiveDeck,
    djDeckA,
    djDeckB,
    djMixer,
  } = useStore();
  
  // Collapse state for sections
  const [fxCollapsed, setFxCollapsed] = useState(false);
  const [libraryCollapsed, setLibraryCollapsed] = useState(false);
  
  // Initialize DJ audio engine and get control functions
  const { initialize, isInitialized, togglePlay, returnToCue, setCrossfader, seek } = useDJAudioEngine();

  // Enable DJ mode and initialize audio engine when page mounts
  useEffect(() => {
    setDJMixerEnabled(true);
    
    // Initialize audio engine (requires user interaction, but we defer to first play)
    // Audio engine will auto-initialize on first track load
    
    return () => {
      // Optionally disable when leaving - but keep state for quick return
      // setDJMixerEnabled(false);
    };
  }, [setDJMixerEnabled]);

  // Keyboard shortcuts
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      // Ignore if typing in input
      if (e.target instanceof HTMLInputElement || e.target instanceof HTMLTextAreaElement) {
        return;
      }

      switch (e.key.toLowerCase()) {
        // Deck A controls
        case 'q':
          returnToCue('A');
          break;
        case 'w':
          togglePlay('A');
          break;
        case 'e':
          // Sync A (Phase 2)
          break;

        // Deck B controls
        case 'o':
          returnToCue('B');
          break;
        case 'p':
          togglePlay('B');
          break;
        case '[':
          // Sync B (Phase 2)
          break;

        // Active deck play/pause with space
        case ' ':
          e.preventDefault();
          togglePlay(djActiveDeck);
          break;

        // Switch active deck
        case 'tab':
          e.preventDefault();
          toggleActiveDeck();
          break;

        // Crossfader
        case 'z':
          setCrossfader(-1); // Full left
          break;
        case 'x':
          setCrossfader(0);  // Center
          break;
        case 'c':
          setCrossfader(1);  // Full right
          break;
        case 'arrowleft':
          if (e.shiftKey) {
            setCrossfader(Math.max(-1, djMixer.crossfader - 0.1));
          }
          break;
        case 'arrowright':
          if (e.shiftKey) {
            setCrossfader(Math.min(1, djMixer.crossfader + 0.1));
          }
          break;

        // Hot cues for active deck (1-8)
        case '1':
        case '2':
        case '3':
        case '4':
        case '5':
        case '6':
        case '7':
        case '8':
          {
            const slot = parseInt(e.key);
            const activeDeckState = djActiveDeck === 'A' ? djDeckA : djDeckB;
            const hotCue = activeDeckState.hotCues.find(hc => hc.slot === slot);
            
            if (e.shiftKey) {
              // Shift + number = set hot cue
              const HOT_CUE_COLORS = ['#ef4444', '#f97316', '#eab308', '#22c55e', '#06b6d4', '#3b82f6', '#8b5cf6', '#ec4899'];
              useStore.getState().setHotCue(djActiveDeck, slot, activeDeckState.position, HOT_CUE_COLORS[slot - 1]);
            } else if (hotCue) {
              // Number = trigger hot cue
              seek(djActiveDeck, hotCue.position);
              useStore.getState().triggerHotCue(djActiveDeck, slot);
            }
          }
          break;
      }
    };

    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [djActiveDeck, djMixer.crossfader, djDeckA, djDeckB, togglePlay, returnToCue, setCrossfader, toggleActiveDeck, seek]);

  return (
    <div className="h-full flex flex-col bg-surface-0 overflow-hidden">
      {/* Top Bar - BPM, Key, Time displays */}
      <div className="flex-shrink-0 h-12 bg-surface-1 border-b border-white/10 flex items-center justify-between px-4">
        {/* Deck A Info */}
        <div className="flex items-center gap-4">
          <div className={`text-lg font-mono ${djDeckA.isPlaying ? 'text-brand' : 'text-neutral-400'}`}>
            {formatTime(djDeckA.position)}
          </div>
          <div className="text-sm text-neutral-400">
            {djDeckA.track?.title || 'No Track'}
          </div>
          {djDeckA.effectiveBpm && (
            <div className="text-sm font-mono text-brand">
              {djDeckA.effectiveBpm.toFixed(1)} BPM
            </div>
          )}
          {djDeckA.key && (
            <div className="text-sm font-mono text-cyan-400">
              {djDeckA.key}
            </div>
          )}
        </div>

        {/* Center - Mode indicator */}
        <div className="flex items-center gap-3">
          <span className="text-xs uppercase tracking-wider text-neutral-500">DJ Mode</span>
          <div className={`w-2 h-2 rounded-full ${djMixerEnabled ? 'bg-brand' : 'bg-neutral-600'}`} />
        </div>

        {/* Deck B Info */}
        <div className="flex items-center gap-4">
          {djDeckB.key && (
            <div className="text-sm font-mono text-cyan-400">
              {djDeckB.key}
            </div>
          )}
          {djDeckB.effectiveBpm && (
            <div className="text-sm font-mono text-brand">
              {djDeckB.effectiveBpm.toFixed(1)} BPM
            </div>
          )}
          <div className="text-sm text-neutral-400">
            {djDeckB.track?.title || 'No Track'}
          </div>
          <div className={`text-lg font-mono ${djDeckB.isPlaying ? 'text-brand' : 'text-neutral-400'}`}>
            {formatTime(djDeckB.position)}
          </div>
        </div>
      </div>

      {/* Waveforms Section */}
      <div className="flex-shrink-0 bg-surface-1/50 border-b border-white/10">
        {/* Waveforms */}
        <div className="h-28 flex">
          <div className="flex-1 border-r border-white/10">
            <DJWaveform deck="A" />
          </div>
          <div className="flex-1">
            <DJWaveform deck="B" />
          </div>
        </div>
        {/* Hot Cues */}
        <div className="h-14 flex border-t border-white/5">
          <div className="flex-1 flex justify-center items-center border-r border-white/10">
            <DJHotCues deck="A" />
          </div>
          <div className="flex-1 flex justify-center items-center">
            <DJHotCues deck="B" />
          </div>
        </div>
      </div>

      {/* Main Controls Area */}
      <div className="flex-1 flex min-h-0">
        {/* Deck A */}
        <div className={`flex-1 border-r border-white/5 ${djActiveDeck === 'A' ? 'ring-1 ring-brand/30' : ''}`}>
          <DeckView deck="A" />
        </div>

        {/* Mixer */}
        <div className="w-72 flex-shrink-0 bg-surface-1/30">
          <DJMixer />
        </div>

        {/* Deck B */}
        <div className={`flex-1 border-l border-white/5 ${djActiveDeck === 'B' ? 'ring-1 ring-brand/30' : ''}`}>
          <DeckView deck="B" />
        </div>
      </div>

      {/* FX & Loop Section - Collapsible */}
      <div className="flex-shrink-0 bg-surface-0 border-t border-white/10">
        {/* Section Header */}
        <div 
          className="flex items-center justify-between px-4 py-1.5 cursor-pointer select-none hover:bg-white/5 transition-colors"
          onClick={() => setFxCollapsed(!fxCollapsed)}
        >
          <h3 className="text-xs font-bold text-neutral-400 uppercase tracking-wider">
            FX & Loop Controls
          </h3>
          {fxCollapsed ? (
            <ChevronDown size={14} className="text-neutral-400" />
          ) : (
            <ChevronUp size={14} className="text-neutral-400" />
          )}
        </div>
        
        {/* Collapsible content */}
        {!fxCollapsed && (
          <div className="p-2 pt-0">
            <div className="flex gap-4">
              {/* Deck A FX & Loop */}
              <div className="flex-1 flex gap-2">
                <div className="flex-1">
                  <DJFXPanel deck="A" />
                </div>
                <div className="w-48">
                  <DJLoopPanel deck="A" />
                </div>
              </div>
              {/* Deck B FX & Loop */}
              <div className="flex-1 flex gap-2">
                <div className="w-48">
                  <DJLoopPanel deck="B" />
                </div>
                <div className="flex-1">
                  <DJFXPanel deck="B" />
                </div>
              </div>
            </div>
          </div>
        )}
      </div>

      {/* Library Browser - Collapsible */}
      <div className={`flex-shrink-0 bg-surface-1 border-t border-white/10 ${libraryCollapsed ? '' : 'h-64'}`}>
        {/* Section Header */}
        <div 
          className="flex items-center justify-between px-4 py-1.5 cursor-pointer select-none hover:bg-white/5 transition-colors"
          onClick={() => setLibraryCollapsed(!libraryCollapsed)}
        >
          <h3 className="text-xs font-bold text-neutral-400 uppercase tracking-wider">
            Library
          </h3>
          {libraryCollapsed ? (
            <ChevronDown size={14} className="text-neutral-400" />
          ) : (
            <ChevronUp size={14} className="text-neutral-400" />
          )}
        </div>
        
        {/* Collapsible content */}
        {!libraryCollapsed && (
          <div className="h-[calc(100%-28px)]">
            <DJLibraryBrowser />
          </div>
        )}
      </div>
    </div>
  );
};

// Helper function to format time as MM:SS
function formatTime(seconds: number): string {
  const mins = Math.floor(seconds / 60);
  const secs = Math.floor(seconds % 60);
  return `${mins.toString().padStart(2, '0')}:${secs.toString().padStart(2, '0')}`;
}

export default DJMode;
