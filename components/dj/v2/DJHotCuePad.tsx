/**
 * ViiB MediaHub - DJ Hot Cue Pad Component (v2)
 * 
 * Professional hot cue buttons inspired by DJ controllers.
 * Features colored slots with glow effects and visual feedback.
 * 
 * @module components/dj/v2/DJHotCuePad
 */

import React, { useCallback, useState, useRef } from 'react';
import { useStore } from '../../../store';
import { useDJAudioEngineActions } from '../../../hooks/useDJAudioEngine';
import type { DeckId } from '../../../slices/djMixerSlice';

interface DJHotCuePadProps {
  deck: DeckId;
  slots?: number[];
  compact?: boolean;
}

// Hot cue colors matching professional DJ software
const HOT_CUE_COLORS = [
  '#22c55e', // Green
  '#22c55e', // Green
  '#22c55e', // Green
  '#eab308', // Yellow
  '#f97316', // Orange
  '#3b82f6', // Blue
  '#8b5cf6', // Purple
  '#ec4899', // Pink
];

export const DJHotCuePad: React.FC<DJHotCuePadProps> = ({ 
  deck, 
  slots = [1, 2, 3, 4, 5, 6, 7, 8],
  compact = false 
}) => {
  const track = useStore(state => deck === 'A' ? state.djDeckA.track : state.djDeckB.track);
  const hotCues = useStore(state => deck === 'A' ? state.djDeckA.hotCues : state.djDeckB.hotCues);
  const setHotCue = useStore(state => state.setHotCue);
  const triggerHotCue = useStore(state => state.triggerHotCue);
  const clearHotCue = useStore(state => state.clearHotCue);
  const { seek } = useDJAudioEngineActions();
  
  const [longPressSlot, setLongPressSlot] = useState<number | null>(null);
  const [pressedSlot, setPressedSlot] = useState<number | null>(null);
  const longPressTimerRef = useRef<NodeJS.Timeout | null>(null);

  const handleClick = useCallback((slot: number) => {
    if (!track) return;
    
    const hotCue = hotCues.find(hc => hc.slot === slot);
    if (hotCue) {
      seek(deck, hotCue.position);
      triggerHotCue(deck, slot);
    }
  }, [deck, track, hotCues, seek, triggerHotCue]);

  const handleSetHotCue = useCallback((slot: number, e: React.MouseEvent) => {
    e.preventDefault();
    if (!track) return;
    
    const currentPosition = useStore.getState()[deck === 'A' ? 'djDeckA' : 'djDeckB'].position;
    const color = HOT_CUE_COLORS[slot - 1] || '#22c55e';
    setHotCue(deck, slot, currentPosition, color);
  }, [deck, track, setHotCue]);

  const handleMouseDown = useCallback((slot: number) => {
    if (!track) return;
    setPressedSlot(slot);
    
    const hotCue = hotCues.find(hc => hc.slot === slot);
    if (hotCue) {
      longPressTimerRef.current = setTimeout(() => {
        clearHotCue(deck, slot);
        setLongPressSlot(null);
      }, 800);
      setLongPressSlot(slot);
    }
  }, [deck, track, hotCues, clearHotCue]);

  const handleMouseUp = useCallback(() => {
    if (longPressTimerRef.current) {
      clearTimeout(longPressTimerRef.current);
      longPressTimerRef.current = null;
    }
    setLongPressSlot(null);
    setPressedSlot(null);
  }, []);

  return (
    <div className="grid grid-cols-4 gap-0.5">
      {slots.map(slot => {
        const hotCue = hotCues.find(hc => hc.slot === slot);
        const isActive = !!hotCue;
        const isLongPress = longPressSlot === slot;
        const isPressed = pressedSlot === slot;
        const color = hotCue?.color || HOT_CUE_COLORS[slot - 1] || '#22c55e';
        const displayNum = slot;
        
        const buttonSize = compact ? 'w-5 h-5 text-[9px]' : 'w-8 h-7 text-[10px]';
        
        return (
          <button
            key={slot}
            onClick={() => handleClick(slot)}
            onContextMenu={(e) => handleSetHotCue(slot, e)}
            onMouseDown={() => handleMouseDown(slot)}
            onMouseUp={handleMouseUp}
            onMouseLeave={handleMouseUp}
            disabled={!track}
            className={`
              relative ${buttonSize} rounded font-bold
              transition-all duration-75 border
              ${!track 
                ? 'bg-[#1f1f1f] text-neutral-700 border-[#2a2a2a] cursor-not-allowed' 
                : isActive
                  ? 'text-white border-transparent'
                  : 'bg-[#252525] text-neutral-500 border-[#333] hover:bg-[#2d2d2d] hover:border-[#444]'}
              ${isLongPress ? 'scale-90 opacity-50' : ''}
              ${isPressed && !isLongPress ? 'scale-95' : ''}
            `}
            style={isActive && track ? {
              backgroundColor: color,
              boxShadow: `0 2px 8px ${color}50, inset 0 1px 0 rgba(255,255,255,0.25)`,
            } : undefined}
            title={
              hotCue 
                ? `Hot Cue ${displayNum}: ${formatTime(hotCue.position)}\nClick=Jump, Hold=Delete`
                : `Hot Cue ${displayNum}: Empty\nRight-click to set`
            }
          >
            {displayNum}
          </button>
        );
      })}
    </div>
  );
};

function formatTime(seconds: number): string {
  const mins = Math.floor(seconds / 60);
  const secs = Math.floor(seconds % 60);
  return `${mins}:${secs.toString().padStart(2, '0')}`;
}

export default React.memo(DJHotCuePad);
