/**
 * ViiB MediaHub - DJ Crossfader Component (v2)
 * 
 * Professional horizontal crossfader with styled handle and visual feedback.
 * Features center detent indicator and deck color fills.
 * 
 * @module components/dj/v2/DJCrossfader
 */

import React, { useCallback, useRef, useState, useEffect } from 'react';

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
    const handleWidth = 56;
    const trackLeft = rect.left + handleWidth / 2;
    const trackWidth = rect.width - handleWidth;
    const relativeX = clientX - trackLeft;
    const normalized = Math.max(0, Math.min(1, relativeX / trackWidth));
    const newValue = (normalized * 2) - 1;
    onChange(newValue);
  };

  // Calculate handle position (0-100%)
  const handlePercent = ((value + 1) / 2) * 100;
  const isCentered = Math.abs(value) < 0.02;

  return (
    <div 
      ref={containerRef}
      className="select-none"
      style={{ width: computedWidth }}
    >
      {/* Labels */}
      <div className="flex justify-between text-[10px] font-bold mb-1 px-2">
        <span className="text-[#3b82f6]">A</span>
        <span className="text-neutral-500 text-[10px] font-medium">Crossfader</span>
        <span className="text-[#8b5cf6]">B</span>
      </div>
      
      {/* Fader track container */}
      <div
        className="relative cursor-pointer touch-none bg-[#1a1a1a] rounded-lg p-1 dj-focus-ring"
        style={{ width: computedWidth, height: 48 }}
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
          const big = e.shiftKey ? 0.05 : 0.02;
          if (e.key === 'ArrowLeft')        { e.preventDefault(); onChange(Math.max(-1, value - big)); }
          else if (e.key === 'ArrowRight')  { e.preventDefault(); onChange(Math.min( 1, value + big)); }
          else if (e.key === 'Home' || e.key === 'Enter' || e.key === ' ') { e.preventDefault(); onChange(0); }
        }}
        onDoubleClick={() => onChange(0)}
      >
        {/* Track groove */}
        <div
          className="absolute top-1/2 -translate-y-1/2 rounded-full"
          style={{
            left: 28,
            right: 28,
            height: 12,
            background: 'linear-gradient(to bottom, #0d0d0d, #1a1a1a, #0d0d0d)',
            boxShadow: 'inset 0 2px 4px rgba(0,0,0,0.5)',
          }}
        />

        {/* Center marker line */}
        <div
          className="absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 rounded"
          style={{
            width: 3,
            height: 24,
            backgroundColor: isCentered ? '#22c55e' : '#333',
            transition: 'background-color 0.1s',
          }}
        />

        {/* Active fills */}
        {value < -0.02 && (
          <div
            className="absolute top-1/2 -translate-y-1/2 rounded-full"
            style={{
              left: 28 + ((value + 1) / 2) * (computedWidth - 56),
              width: (computedWidth / 2) - 28 - ((value + 1) / 2) * (computedWidth - 56),
              height: 8,
              background: 'linear-gradient(to right, #3b82f6, #3b82f640)',
              boxShadow: '0 0 8px #3b82f640',
            }}
          />
        )}
        {value > 0.02 && (
          <div
            className="absolute top-1/2 -translate-y-1/2 rounded-full"
            style={{
              left: computedWidth / 2,
              width: (value / 2) * (computedWidth - 56),
              height: 8,
              background: 'linear-gradient(to right, #8b5cf640, #8b5cf6)',
              boxShadow: '0 0 8px #8b5cf640',
            }}
          />
        )}

        {/* Handle */}
        <div
          className={`absolute top-1/2 -translate-y-1/2 cursor-grab ${isDragging ? 'cursor-grabbing scale-105' : ''}`}
          style={{
            left: `calc(${handlePercent}% - 28px)`,
            transition: isDragging ? 'none' : 'transform 0.1s',
          }}
        >
          {/* Handle body — bigger, easier to grab; centre detent dot inset */}
          <div
            className="rounded-md relative overflow-hidden"
            style={{
              width: 56,
              height: 36,
              background: 'linear-gradient(to bottom, #7a7a7a, #555, #444)',
              boxShadow: isDragging
                ? '0 4px 16px rgba(0,0,0,0.6), inset 0 1px 0 rgba(255,255,255,0.25)'
                : '0 2px 10px rgba(0,0,0,0.45), inset 0 1px 0 rgba(255,255,255,0.18)',
            }}
          >
            {/* Grip texture */}
            <div className="absolute inset-0 flex justify-center items-center gap-1">
              {[...Array(5)].map((_, i) => (
                <div
                  key={i}
                  className="w-px h-5"
                  style={{
                    backgroundColor: i % 2 === 0 ? '#666' : '#999'
                  }}
                />
              ))}
            </div>

            {/* Top highlight */}
            <div
              className="absolute top-0 left-2 right-2 h-px bg-gradient-to-r from-transparent via-white/35 to-transparent"
            />
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
            backgroundColor: isCentered ? '#22c55e' : '#333',
            boxShadow: isCentered ? '0 0 6px #22c55e' : 'none',
          }}
        />
      </div>
    </div>
  );
};

export default React.memo(DJCrossfader);
