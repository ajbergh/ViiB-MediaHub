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
        // Reserve the BPM readout, percent readout and range button (with gaps)
        // so the track never pushes them into neighbouring controls.
        const availableHeight = parent.clientHeight - 84;
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

  const clampTempoToRange = useCallback((nextValue: number) => {
    const rangeMin = Math.max(0.5, 1 - maxChange);
    const rangeMax = Math.min(1.5, 1 + maxChange);
    return Math.max(rangeMin, Math.min(rangeMax, nextValue));
  }, [maxChange]);

  const handleKeyDown = useCallback((e: React.KeyboardEvent) => {
    if (disabled) return;

    let next: number | null = null;
    const smallStep = e.shiftKey ? 0.01 : 0.001;
    const pageStep = e.shiftKey ? 0.05 : 0.01;

    if (e.key === 'ArrowUp' || e.key === 'ArrowRight') {
      next = value + smallStep;
    } else if (e.key === 'ArrowDown' || e.key === 'ArrowLeft') {
      next = value - smallStep;
    } else if (e.key === 'PageUp') {
      next = value + pageStep;
    } else if (e.key === 'PageDown') {
      next = value - pageStep;
    } else if (e.key === 'Home' || e.key === 'Enter' || e.key === ' ') {
      next = 1.0;
    }

    if (next !== null) {
      e.preventDefault();
      onChange(clampTempoToRange(next));
    }
  }, [clampTempoToRange, disabled, onChange, value]);

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

  // Deck identity from tokens (Plan §10A.11): dark track, silver cap, deck-colored deviation fill.
  const accentColor = deck === 'A' ? 'var(--dj-deck-a-bright)' : 'var(--dj-deck-b-bright)';

  // Calculate percentage display
  const percentDisplay = tempoPercent >= 0
    ? `+${tempoPercent.toFixed(1)}%`
    : `${tempoPercent.toFixed(1)}%`;

  return (
    <div
      ref={containerRef}
      className="dj-fader flex flex-col items-center gap-1"
      onContextMenu={handleContextMenu}
    >
      {/* BPM Display */}
      <div className="dj-tempo-bpm dj-fader-readout">
        <span data-active={!!effectiveBpm}>{bpmDisplay}</span>
        <span className="dj-fader-unit">BPM</span>
      </div>

      {/* Slider Track */}
      <div
        ref={trackRef}
        className={`dj-fader-track ${disabled ? 'cursor-not-allowed' : 'cursor-ns-resize'}`}
        data-disabled={disabled}
        style={{ height: computedHeight }}
        role="slider"
        tabIndex={disabled ? -1 : 0}
        aria-label={`Deck ${deck} tempo`}
        aria-valuemin={Number((1 - maxChange).toFixed(3))}
        aria-valuemax={Number((1 + maxChange).toFixed(3))}
        aria-valuenow={Number(value.toFixed(3))}
        aria-valuetext={`${percentDisplay}, ${bpmDisplay} BPM`}
        onMouseDown={handleMouseDown}
        onTouchStart={handleMouseDown}
        onDoubleClick={handleDoubleClick}
        onKeyDown={handleKeyDown}
      >
        {/* Center line marker */}
        <div className="dj-fader-center" />

        {/* Range markers */}
        <div className="dj-fader-scale" style={{ top: 0 }}>+{tempoRange}</div>
        <div className="dj-fader-scale" style={{ bottom: 0 }}>-{tempoRange}</div>

        {/* Tick marks */}
        {[0.125, 0.25, 0.375, 0.625, 0.75, 0.875].map((pos, i) => (
          <div key={i} className="dj-fader-tick" style={{ top: `${pos * 100}%` }} />
        ))}

        {/* Deviation from center in deck color */}
        {value !== 1.0 && (
          <div
            className="dj-fader-fill"
            style={{
              backgroundColor: accentColor,
              top: value > 1 ? `${position * 100}%` : '50%',
              bottom: value < 1 ? `${(1 - position) * 100}%` : '50%',
              height: `${Math.abs(position - 0.5) * 100}%`,
            }}
          />
        )}

        {/* Slider cap */}
        <div
          className="dj-fader-cap"
          data-dragging={isDragging}
          style={{ top: `calc(${position * 100}% - 10px)`, ['--dj-fader-accent' as string]: accentColor }}
        >
          <span /><span /><span />
        </div>
      </div>

      {/* Percentage Display */}
      <div
        className="dj-fader-readout dj-fader-percent"
        style={Math.abs(tempoPercent) >= 0.1 ? { color: accentColor } : undefined}
      >
        {percentDisplay}
      </div>

      {/* Range indicator (click to change) */}
      <button
        type="button"
        onClick={() => {
          const currentIndex = TEMPO_RANGES.indexOf(tempoRange);
          const nextIndex = (currentIndex + 1) % TEMPO_RANGES.length;
          setTempoRange(TEMPO_RANGES[nextIndex]);
        }}
        className="dj-btn dj-btn-xs dj-btn-ghost"
        title={`Tempo range ±${tempoRange}% — click to cycle`}
        aria-label={`Tempo range ±${tempoRange}%, click to cycle`}
      >
        ±{tempoRange}%
      </button>
    </div>
  );
};

export default React.memo(DJTempoSlider);
