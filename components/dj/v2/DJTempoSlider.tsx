/**
 * ViiB MediaHub - DJ Tempo/Pitch Slider Component (v2)
 * 
 * Vertical tempo slider for adjusting playback speed.
 * Range: ±50% (0.5x to 1.5x)
 * Center detent at 100% (1.0x)
 * 
 * @module components/dj/v2/DJTempoSlider
 */

import React, { useRef, useCallback, useState, useEffect } from 'react';
import type { DeckId } from '../../../slices/djMixerSlice';

interface DJTempoSliderProps {
  deck: DeckId;
  value: number;        // 0.5 to 1.5
  onChange: (value: number) => void;
  height?: number;
  originalBpm?: number | null;
  effectiveBpm?: number | null;
  disabled?: boolean;
  responsive?: boolean;
}

// Tempo range presets
const TEMPO_RANGES = [8, 16, 24, 50] as const;
type TempoRange = typeof TEMPO_RANGES[number];

export const DJTempoSlider: React.FC<DJTempoSliderProps> = ({
  deck,
  value,
  onChange,
  height = 140,
  originalBpm,
  effectiveBpm,
  disabled = false,
  responsive = false,
}) => {
  const trackRef = useRef<HTMLDivElement>(null);
  const containerRef = useRef<HTMLDivElement>(null);
  const [isDragging, setIsDragging] = useState(false);
  const [tempoRange, setTempoRange] = useState<TempoRange>(16);
  const [computedHeight, setComputedHeight] = useState(height > 0 ? height : 140);
  
  // Responsive height calculation
  useEffect(() => {
    if (!responsive) {
      setComputedHeight(height > 0 ? height : 140);
      return;
    }
    
    const container = containerRef.current;
    if (!container) return;
    
    const updateHeight = () => {
      const parent = container.parentElement;
      if (parent) {
        const availableHeight = parent.clientHeight - 40;
        setComputedHeight(Math.max(80, Math.min(availableHeight, 580)));
      }
    };
    
    updateHeight();
    const resizeObserver = new ResizeObserver(updateHeight);
    if (container.parentElement) {
      resizeObserver.observe(container.parentElement);
    }
    
    return () => resizeObserver.disconnect();
  }, [responsive, height]);
  
  // Calculate tempo percentage from value
  const tempoPercent = (value - 1) * 100;
  
  // Calculate effective BPM display
  const bpmDisplay = effectiveBpm?.toFixed(1) || originalBpm?.toFixed(1) || '--';
  
  // Map value (0.5-1.5) to position (0-1 where 0.5 = center)
  // At ±16 range: 1.16 = top, 1.0 = center, 0.84 = bottom
  const maxChange = tempoRange / 100;
  const clampedPercent = Math.max(-maxChange, Math.min(maxChange, tempoPercent / 100));
  const position = 0.5 - (clampedPercent / maxChange) * 0.5;
  
  const handleMouseDown = useCallback((e: React.MouseEvent | React.TouchEvent) => {
    if (disabled) return;
    e.preventDefault();
    setIsDragging(true);
    handleMove(e);
  }, [disabled]);

  const handleMove = useCallback((e: MouseEvent | TouchEvent | React.MouseEvent | React.TouchEvent) => {
    if (!trackRef.current) return;
    
    const rect = trackRef.current.getBoundingClientRect();
    const clientY = 'touches' in e ? e.touches[0].clientY : e.clientY;
    
    // Calculate position from top (0 = top, 1 = bottom)
    const relativeY = (clientY - rect.top) / rect.height;
    const clampedY = Math.max(0, Math.min(1, relativeY));
    
    // Convert to tempo: top = +range%, center = 0%, bottom = -range%
    const rangePercent = (0.5 - clampedY) * 2 * tempoRange; // -range to +range
    const newTempo = 1 + (rangePercent / 100);
    
    // Clamp to valid range
    const clampedTempo = Math.max(0.5, Math.min(1.5, newTempo));
    onChange(clampedTempo);
  }, [onChange, tempoRange]);

  const handleMouseUp = useCallback(() => {
    setIsDragging(false);
  }, []);

  // Double-click to reset to center (1.0)
  const handleDoubleClick = useCallback(() => {
    if (!disabled) {
      onChange(1.0);
    }
  }, [disabled, onChange]);

  // Cycle tempo range on right-click
  const handleContextMenu = useCallback((e: React.MouseEvent) => {
    e.preventDefault();
    const currentIndex = TEMPO_RANGES.indexOf(tempoRange);
    const nextIndex = (currentIndex + 1) % TEMPO_RANGES.length;
    setTempoRange(TEMPO_RANGES[nextIndex]);
  }, [tempoRange]);

  // Global mouse/touch events
  useEffect(() => {
    if (!isDragging) return;
    
    const onMove = (e: MouseEvent | TouchEvent) => handleMove(e);
    const onUp = () => handleMouseUp();
    
    window.addEventListener('mousemove', onMove);
    window.addEventListener('mouseup', onUp);
    window.addEventListener('touchmove', onMove);
    window.addEventListener('touchend', onUp);
    
    return () => {
      window.removeEventListener('mousemove', onMove);
      window.removeEventListener('mouseup', onUp);
      window.removeEventListener('touchmove', onMove);
      window.removeEventListener('touchend', onUp);
    };
  }, [isDragging, handleMove, handleMouseUp]);

  const accentColor = deck === 'A' ? '#3b82f6' : '#8b5cf6';
  const accentColorDim = deck === 'A' ? 'rgba(59, 130, 246, 0.3)' : 'rgba(139, 92, 246, 0.3)';
  
  // Calculate percentage display
  const percentDisplay = tempoPercent >= 0 
    ? `+${tempoPercent.toFixed(1)}%` 
    : `${tempoPercent.toFixed(1)}%`;

  return (
    <div 
      ref={containerRef}
      className="flex flex-col items-center gap-1"
      onContextMenu={handleContextMenu}
    >
      {/* BPM Display */}
      <div className="text-[10px] font-mono text-neutral-400 text-center">
        <span className={effectiveBpm ? 'text-green-400' : ''}>{bpmDisplay}</span>
        <span className="text-neutral-600 ml-0.5">BPM</span>
      </div>
      
      {/* Slider Track */}
      <div 
        ref={trackRef}
        className={`
          relative rounded-full bg-[#1a1a1a] border border-[#333]
          ${disabled ? 'opacity-50 cursor-not-allowed' : 'cursor-ns-resize'}
        `}
        style={{ width: 24, height: computedHeight }}
        onMouseDown={handleMouseDown}
        onTouchStart={handleMouseDown}
        onDoubleClick={handleDoubleClick}
      >
        {/* Track gradient overlay */}
        <div 
          className="absolute inset-0 rounded-full pointer-events-none"
          style={{
            background: `linear-gradient(to bottom, 
              ${accentColorDim} 0%, 
              transparent 45%, 
              transparent 55%, 
              ${accentColorDim} 100%
            )`
          }}
        />
        
        {/* Center line marker */}
        <div 
          className="absolute left-0 right-0 h-[2px] bg-[#444]"
          style={{ top: '50%', transform: 'translateY(-50%)' }}
        />
        
        {/* Range markers */}
        <div className="absolute -left-1 top-0 text-[8px] text-neutral-600 font-mono">
          +{tempoRange}
        </div>
        <div className="absolute -left-1 bottom-0 text-[8px] text-neutral-600 font-mono">
          -{tempoRange}
        </div>
        
        {/* Tick marks */}
        {[0.125, 0.25, 0.375, 0.625, 0.75, 0.875].map((pos, i) => (
          <div
            key={i}
            className="absolute left-0 right-0 h-[1px] bg-[#333]"
            style={{ top: `${pos * 100}%` }}
          />
        ))}
        
        {/* Position indicator line (shows deviation from center) */}
        {value !== 1.0 && (
          <div 
            className="absolute left-1/2 w-[2px] -translate-x-1/2 rounded-full"
            style={{
              backgroundColor: accentColor,
              top: value > 1 ? `${position * 100}%` : '50%',
              bottom: value < 1 ? `${(1 - position) * 100}%` : '50%',
              height: `${Math.abs(position - 0.5) * 100}%`,
              boxShadow: `0 0 6px ${accentColor}`,
            }}
          />
        )}
        
        {/* Slider cap */}
        <div 
          className={`
            absolute left-1/2 -translate-x-1/2 w-8 h-5 rounded
            transition-shadow duration-100
            ${isDragging ? 'shadow-lg' : ''}
          `}
          style={{ 
            top: `calc(${position * 100}% - 10px)`,
            background: `linear-gradient(to bottom, #666 0%, #444 50%, #333 100%)`,
            boxShadow: isDragging 
              ? `0 0 10px ${accentColor}, 0 2px 4px rgba(0,0,0,0.5)` 
              : '0 2px 4px rgba(0,0,0,0.5)',
            border: '1px solid #555',
          }}
        >
          {/* Cap grip lines */}
          <div className="absolute inset-x-1 top-1/2 -translate-y-1/2 flex flex-col gap-[2px]">
            <div className="h-[1px] bg-[#777]" />
            <div className="h-[1px] bg-[#555]" />
            <div className="h-[1px] bg-[#777]" />
          </div>
        </div>
      </div>
      
      {/* Percentage Display */}
      <div 
        className={`
          text-[11px] font-mono font-bold text-center min-w-[48px]
          ${Math.abs(tempoPercent) < 0.1 ? 'text-neutral-500' : 'text-white'}
        `}
        style={Math.abs(tempoPercent) >= 0.1 ? { color: accentColor } : undefined}
      >
        {percentDisplay}
      </div>
      
      {/* Range indicator (click to change) */}
      <button
        onClick={() => {
          const currentIndex = TEMPO_RANGES.indexOf(tempoRange);
          const nextIndex = (currentIndex + 1) % TEMPO_RANGES.length;
          setTempoRange(TEMPO_RANGES[nextIndex]);
        }}
        className="text-[9px] text-neutral-600 hover:text-neutral-400 transition-colors"
        title="Click to change tempo range"
      >
        ±{tempoRange}%
      </button>
    </div>
  );
};

export default React.memo(DJTempoSlider);
