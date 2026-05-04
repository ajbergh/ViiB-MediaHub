/**
 * ViiB MediaHub - DJ Hot Cue Panel Component
 * 
 * Displays 8 hot cue buttons for instant position jumps.
 * - Click: Jump to hot cue position
 * - Shift+Click (or right-click): Set hot cue at current position
 * - Long press: Delete hot cue
 * 
 * @module components/dj/DJHotCues
 */

import React, { useCallback, useState, useRef } from 'react';
import { useStore } from '../../store';
import { useDJAudioEngine } from '../../hooks/useDJAudioEngine';
import type { DeckId } from '../../slices/djMixerSlice';

interface DJHotCuesProps {
  deck: DeckId;
}

// Default colors for hot cue slots
const HOT_CUE_COLORS = [
  '#ef4444', // Red
  '#f97316', // Orange
  '#eab308', // Yellow
  '#22c55e', // Green
  '#06b6d4', // Cyan
  '#3b82f6', // Blue
  '#8b5cf6', // Purple
  '#ec4899', // Pink
];

export const DJHotCues: React.FC<DJHotCuesProps> = ({ deck }) => {
  // Only subscribe to specific state needed for rendering, NOT position
  // Position is read from store directly when setting hot cue to avoid re-renders
  const track = useStore(state => deck === 'A' ? state.djDeckA.track : state.djDeckB.track);
  const hotCues = useStore(state => deck === 'A' ? state.djDeckA.hotCues : state.djDeckB.hotCues);
  const { setHotCue, triggerHotCue, clearHotCue } = useStore();
  const { seek } = useDJAudioEngine();
  
  const [longPressSlot, setLongPressSlot] = useState<number | null>(null);
  const longPressTimerRef = useRef<NodeJS.Timeout | null>(null);

  // Handle hot cue click (jump to position)
  const handleClick = useCallback((slot: number) => {
    console.log(`🎯 DJHotCues: handleClick slot=${slot}, track=${track?.title}, hotCues=${JSON.stringify(hotCues)}`);
    if (!track) {
      console.log(`🎯 DJHotCues: No track loaded`);
      return;
    }
    
    const hotCue = hotCues.find(hc => hc.slot === slot);
    if (hotCue) {
      console.log(`🎯 DJHotCues: Jumping to hot cue at ${hotCue.position}s`);
      // Jump to hot cue position
      seek(deck, hotCue.position);
      triggerHotCue(deck, slot);
    } else {
      console.log(`🎯 DJHotCues: No hot cue at slot ${slot}`);
    }
  }, [deck, track, hotCues, seek, triggerHotCue]);

  // Handle setting a new hot cue (right-click or shift+click)
  const handleSetHotCue = useCallback((slot: number, e: React.MouseEvent) => {
    e.preventDefault();
    // Get current position directly from store to avoid stale closures and unnecessary re-renders
    const currentPosition = useStore.getState()[deck === 'A' ? 'djDeckA' : 'djDeckB'].position;
    console.log(`🎯 DJHotCues: handleSetHotCue slot=${slot}, track=${track?.title}, position=${currentPosition}`);
    if (!track) {
      console.log(`🎯 DJHotCues: No track loaded, can't set hot cue`);
      return;
    }
    
    const color = HOT_CUE_COLORS[slot - 1] || '#ffffff';
    setHotCue(deck, slot, currentPosition, color);
    console.log(`🎯 DJHotCues: Set hot cue at slot ${slot}, position ${currentPosition}, color ${color}`);
  }, [deck, track, setHotCue]);

  // Handle long press to delete
  const handleMouseDown = useCallback((slot: number) => {
    if (!track) return;
    
    const hotCue = hotCues.find(hc => hc.slot === slot);
    if (hotCue) {
      longPressTimerRef.current = setTimeout(() => {
        clearHotCue(deck, slot);
        setLongPressSlot(null);
      }, 800); // 800ms long press to delete
      setLongPressSlot(slot);
    }
  }, [deck, track, hotCues, clearHotCue]);

  const handleMouseUp = useCallback(() => {
    if (longPressTimerRef.current) {
      clearTimeout(longPressTimerRef.current);
      longPressTimerRef.current = null;
    }
    setLongPressSlot(null);
  }, []);

  const handleMouseLeave = useCallback(() => {
    handleMouseUp();
  }, [handleMouseUp]);

  return (
    <div className="flex gap-1 p-2">
      {[1, 2, 3, 4, 5, 6, 7, 8].map(slot => {
        const hotCue = hotCues.find(hc => hc.slot === slot);
        const isActive = !!hotCue;
        const isLongPress = longPressSlot === slot;
        const color = hotCue?.color || HOT_CUE_COLORS[slot - 1];
        
        return (
          <button
            key={slot}
            onClick={() => handleClick(slot)}
            onContextMenu={(e) => handleSetHotCue(slot, e)}
            onMouseDown={() => handleMouseDown(slot)}
            onMouseUp={handleMouseUp}
            onMouseLeave={handleMouseLeave}
            disabled={!track}
            className={`
              relative w-10 h-10 rounded-md font-bold text-sm
              transition-all duration-150
              ${!track 
                ? 'bg-surface-2 text-neutral-600 cursor-not-allowed' 
                : isActive
                  ? 'text-white shadow-lg'
                  : 'bg-surface-2 text-neutral-400 hover:bg-surface-1'}
              ${isLongPress ? 'scale-90 opacity-50' : ''}
            `}
            style={isActive && track ? { 
              backgroundColor: color,
              boxShadow: `0 0 10px ${color}50`
            } : undefined}
            title={
              hotCue 
                ? `Hot Cue ${slot}: ${formatTime(hotCue.position)}\nClick to jump, hold to delete`
                : `Hot Cue ${slot}: Empty\nRight-click to set`
            }
          >
            {slot}
            {/* Active indicator dot */}
            {isActive && (
              <div 
                className="absolute -top-1 -right-1 w-2 h-2 rounded-full bg-white"
                style={{ boxShadow: '0 0 4px rgba(255,255,255,0.8)' }}
              />
            )}
          </button>
        );
      })}
    </div>
  );
};

function formatTime(seconds: number): string {
  if (!seconds || !isFinite(seconds)) return '0:00';
  const mins = Math.floor(seconds / 60);
  const secs = Math.floor(seconds % 60);
  return `${mins}:${secs.toString().padStart(2, '0')}`;
}

export default DJHotCues;
