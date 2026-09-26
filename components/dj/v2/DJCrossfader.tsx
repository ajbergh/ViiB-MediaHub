/**
 * ViiB MediaHub - DJ Crossfader Component (v2)
 * 
 * Professional horizontal crossfader with styled handle and visual feedback.
 * Thin A→B gradient track, center detent and a slim silver cap.
 * 
 * @module components/dj/v2/DJCrossfader
 */

import React, { useCallback, useRef, useState, useEffect } from 'react';

/** Handle width in layout px; the track is inset by half of it on each side. */
const HANDLE_WIDTH = 28;

interface DJCrossfaderProps {
  value: number;      // -1 to +1
  onChange: (value: number) => void;
  width?: number;
  responsive?: boolean;
}

export const DJCrossfader: React.FC<DJCrossfaderProps> = ({ 
  value, 
  onChange, 
  width = 200,
  responsive = false 
}) => {
  const containerRef = useRef<HTMLDivElement>(null);
  const [isDragging, setIsDragging] = useState(false);
  const [computedWidth, setComputedWidth] = useState(width > 0 ? width : 180);
  const computedWidthRef = useRef(computedWidth);
  computedWidthRef.current = computedWidth;

  // Responsive width calculation
  useEffect(() => {
    if (!responsive) {
      setComputedWidth(width > 0 ? width : 180);
      return;
    }
    
    const container = containerRef.current;
    if (!container) return;
    
    const updateWidth = () => {
      const parent = container.parentElement;
      if (parent) {
        // Use 88% of parent width with a 360 px max — crossfader is the hero
        // mixer control and should not be artificially clamped to 280 px
        // when the mixer column has been widened.
        const availableWidth = parent.clientWidth * 0.88;
        setComputedWidth(Math.max(180, Math.min(availableWidth, 360)));
      }
    };
    
    updateWidth();
    const resizeObserver = new ResizeObserver(updateWidth);
    if (container.parentElement) {
      resizeObserver.observe(container.parentElement);
    }
    
    return () => resizeObserver.disconnect();
  }, [responsive, width]);

  const handlePointerDown = useCallback((e: React.PointerEvent) => {
    e.preventDefault();
    (e.target as HTMLElement).setPointerCapture(e.pointerId);
    setIsDragging(true);
    updateValueFromPointer(e.clientX);
  }, []);

  const handlePointerMove = useCallback((e: React.PointerEvent) => {
    if (!isDragging) return;
    updateValueFromPointer(e.clientX);
  }, [isDragging]);

  const handlePointerUp = useCallback((e: React.PointerEvent) => {
    (e.target as HTMLElement).releasePointerCapture(e.pointerId);
    setIsDragging(false);
  }, []);

  const updateValueFromPointer = (clientX: number) => {
    if (!containerRef.current) return;
    const rect = containerRef.current.getBoundingClientRect();
    // rect is post-transform (the DJ canvas is scaled); express the handle
    // inset as a fraction of the layout width so pointer and handle agree.
    const inset = rect.width * (HANDLE_WIDTH / 2) / computedWidthRef.current;
    const trackLeft = rect.left + inset;
    const trackWidth = rect.width - inset * 2;
    const relativeX = clientX - trackLeft;
    const normalized = Math.max(0, Math.min(1, relativeX / trackWidth));
    const newValue = (normalized * 2) - 1;
    onChange(newValue);
  };

  const isCentered = Math.abs(value) < 0.02;

  return (
    <div 
      ref={containerRef}
      className="select-none"
      style={{ width: computedWidth }}
    >
      {/* Labels */}
      <div className="flex justify-between text-[12px] font-bold mb-1 px-2">
        <span className="text-[var(--dj-deck-a)]">A</span>
        <span className="text-[var(--dj-text-secondary)] text-[12px] font-medium">Crossfader</span>
        <span className="text-[var(--dj-deck-b)]">B</span>
      </div>
      
      {/* Fader track container */}
      <div
        className="relative cursor-pointer touch-none rounded-lg dj-focus-ring"
        style={{ width: computedWidth, height: 40 }}
        role="slider"
        tabIndex={0}
        aria-label="Crossfader"
        aria-valuemin={-1}
        aria-valuemax={1}
        aria-valuenow={Math.round(value * 100) / 100}
        onPointerDown={handlePointerDown}
        onPointerMove={handlePointerMove}
        onPointerUp={handlePointerUp}
        onPointerLeave={handlePointerUp}
        onKeyDown={(e) => {
          // Keep the fader usable from the keyboard in the same way as a
          // native range input: arrows make small adjustments, Shift makes
          // larger adjustments, and Home/End jump to either deck.
          const step = e.shiftKey ? 0.1 : 0.02;
          let nextValue: number | null = null;

          if (e.key === 'ArrowLeft') nextValue = Math.max(-1, value - step);
          else if (e.key === 'ArrowRight') nextValue = Math.min(1, value + step);
          else if (e.key === 'Home') nextValue = -1;
          else if (e.key === 'End') nextValue = 1;

          if (nextValue !== null) {
            e.preventDefault();
            if (nextValue !== value) onChange(nextValue);
          }
        }}
        onDoubleClick={() => onChange(0)}
      >
        {/* Thin A→B gradient track (Plan §10A.13) */}
        <div
          className="absolute top-1/2 -translate-y-1/2 rounded-full"
          style={{
            left: HANDLE_WIDTH / 2,
            right: HANDLE_WIDTH / 2,
            height: 6,
            background: 'linear-gradient(to right, var(--dj-deck-a-bright), color-mix(in srgb, var(--dj-deck-a) 55%, var(--dj-deck-b)) 50%, var(--dj-deck-b-bright))',
            boxShadow: '0 0 8px color-mix(in srgb, var(--dj-deck-b) 25%, transparent)',
          }}
        />

        {/* Center detent */}
        <div
          className="absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 rounded"
          style={{
            width: 2,
            height: 20,
            backgroundColor: isCentered ? 'var(--dj-play)' : 'var(--dj-text-muted)',
            transition: 'background-color 0.1s',
          }}
        />

        {/* Handle — slim silver cap; centre sits on the track position */}
        <div
          className={`absolute top-1/2 -translate-y-1/2 cursor-grab ${isDragging ? 'cursor-grabbing' : ''}`}
          style={{
            left: ((value + 1) / 2) * (computedWidth - HANDLE_WIDTH),
            transition: isDragging ? 'none' : 'left 0.08s',
          }}
        >
          <div
            className="rounded relative overflow-hidden flex items-center justify-center gap-[3px]"
            style={{
              width: HANDLE_WIDTH,
              height: 36,
              background: 'linear-gradient(to bottom, var(--dj-text-primary), var(--dj-text-secondary) 55%, var(--dj-text-muted))',
              border: '1px solid var(--dj-border-hover)',
              boxShadow: isDragging
                ? '0 4px 14px rgba(0,0,0,0.6)'
                : '0 2px 8px rgba(0,0,0,0.45)',
            }}
          >
            {[0, 1, 2].map(i => <div key={i} className="w-px h-5" style={{ backgroundColor: 'var(--dj-border-light)' }} />)}
          </div>
        </div>
      </div>
      
      {/* Center snap indicator */}
      <div className="flex justify-center mt-1">
        <div 
          className="rounded-full transition-all duration-100"
          style={{
            width: 5,
            height: 5,
            backgroundColor: isCentered ? 'var(--dj-play)' : 'var(--dj-border-light)',
            boxShadow: isCentered ? '0 0 6px var(--dj-play)' : 'none',
          }}
        />
      </div>
    </div>
  );
};

export default React.memo(DJCrossfader);
