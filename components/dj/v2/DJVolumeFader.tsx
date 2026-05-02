/**
 * ViiB MediaHub - DJ Volume Fader Component (v2)
 * 
 * Professional T-shaped vertical volume fader inspired by DJ mixers.
 * Features VU meter styling and visual feedback.
 * 
 * @module components/dj/v2/DJVolumeFader
 */

import React, { useCallback, useRef, useState } from 'react';

interface DJVolumeFaderProps {
  value: number;      // 0 to 1
  onChange: (value: number) => void;
  label?: string;
  height?: number;
  isPlaying?: boolean;
  accentColor?: string;
}

export const DJVolumeFader: React.FC<DJVolumeFaderProps> = ({ 
  value, 
  onChange, 
  label,
  height = 120,
  isPlaying = false,
  accentColor = '#8b5cf6'
}) => {
  const containerRef = useRef<HTMLDivElement>(null);
  const [isDragging, setIsDragging] = useState(false);

  const handlePointerDown = useCallback((e: React.PointerEvent) => {
    e.preventDefault();
    (e.target as HTMLElement).setPointerCapture(e.pointerId);
    setIsDragging(true);
    updateValueFromPointer(e.clientY);
  }, []);

  const handlePointerMove = useCallback((e: React.PointerEvent) => {
    if (!isDragging) return;
    updateValueFromPointer(e.clientY);
  }, [isDragging]);

  const handlePointerUp = useCallback((e: React.PointerEvent) => {
    (e.target as HTMLElement).releasePointerCapture(e.pointerId);
    setIsDragging(false);
  }, []);

  const updateValueFromPointer = (clientY: number) => {
    if (!containerRef.current) return;
    const rect = containerRef.current.getBoundingClientRect();
    const handleHeight = 28;
    const trackTop = rect.top + handleHeight / 2;
    const trackHeight = rect.height - handleHeight - 20; // Account for label and value
    const relativeY = clientY - trackTop;
    const newValue = 1 - Math.max(0, Math.min(1, relativeY / trackHeight));
    onChange(newValue);
  };

  // Calculate handle position (0% = top, 100% = bottom)
  const handlePercent = (1 - value) * 100;
  const trackHeight = height - 32; // Space for label and value

  return (
    <div
      ref={containerRef}
      className="flex flex-col items-center select-none"
      style={{ height }}
    >
      {/* Label */}
      {label && (
        <span
          className={`text-[10px] font-bold mb-1 transition-colors`}
          style={{ color: isPlaying ? accentColor : '#666' }}
        >
          {label}
        </span>
      )}

      {/* Fader track container */}
      <div
        className="relative cursor-pointer touch-none dj-focus-ring rounded"
        style={{ width: 48, height: trackHeight }}
        role="slider"
        tabIndex={0}
        aria-label={label || 'Volume'}
        aria-valuemin={0}
        aria-valuemax={1}
        aria-valuenow={Math.round(value * 100) / 100}
        onPointerDown={handlePointerDown}
        onPointerMove={handlePointerMove}
        onPointerUp={handlePointerUp}
        onPointerLeave={handlePointerUp}
        onKeyDown={(e) => {
          const big = e.shiftKey ? 0.1 : 0.02;
          if (e.key === 'ArrowUp')         { e.preventDefault(); onChange(Math.min(1, value + big)); }
          else if (e.key === 'ArrowDown')  { e.preventDefault(); onChange(Math.max(0, value - big)); }
          else if (e.key === 'Home')       { e.preventDefault(); onChange(1); }
          else if (e.key === 'End')        { e.preventDefault(); onChange(0); }
        }}
        onWheel={(e) => {
          e.preventDefault();
          const step = e.shiftKey ? 0.01 : 0.04;
          const next = Math.max(0, Math.min(1, value + (e.deltaY > 0 ? -step : step)));
          if (next !== value) onChange(next);
        }}
      >
        {/* Track background — wider for clearer visual + bigger touch zone */}
        <div
          className="absolute left-1/2 -translate-x-1/2 rounded"
          style={{
            width: 10,
            height: trackHeight - 14,
            top: 7,
            background: 'linear-gradient(to bottom, #0d0d0d, #1a1a1a)',
            boxShadow: 'inset 0 2px 4px rgba(0,0,0,0.5)',
          }}
        />

        {/* Active level fill */}
        <div
          className="absolute left-1/2 -translate-x-1/2 rounded-b transition-all duration-50"
          style={{
            width: 6,
            height: Math.max(0, value * (trackHeight - 14)),
            bottom: 7,
            background: `linear-gradient(to top, ${accentColor}60, ${accentColor})`,
            boxShadow: isPlaying ? `0 0 8px ${accentColor}40` : 'none',
          }}
        />

        {/* VU meter markers (outside the wider track) */}
        <div className="absolute right-0 top-0 bottom-0 flex flex-col justify-between py-1.5 pr-0.5">
          {[100, 80, 60, 40, 20, 0].map((pct) => (
            <div
              key={pct}
              className="flex items-center gap-0.5"
            >
              <div
                className="w-1.5 h-px"
                style={{
                  backgroundColor: pct >= 80 ? '#ef4444' : pct >= 60 ? '#f59e0b' : '#444'
                }}
              />
            </div>
          ))}
        </div>

        {/* T-shaped fader handle */}
        <div
          className={`absolute left-1/2 -translate-x-1/2 cursor-grab ${isDragging ? 'cursor-grabbing' : ''}`}
          style={{
            top: `calc(${handlePercent}% - 14px)`,
            transition: isDragging ? 'none' : 'top 0.05s',
          }}
        >
          {/* Horizontal bar (T-top) — bumped 30×10 → 44×16 */}
          <div
            className="rounded relative overflow-hidden"
            style={{
              width: 44,
              height: 16,
              background: isDragging
                ? 'linear-gradient(to bottom, #aaa, #888)'
                : 'linear-gradient(to bottom, #999, #777)',
              boxShadow: isDragging
                ? '0 4px 12px rgba(0,0,0,0.5), inset 0 1px 0 rgba(255,255,255,0.25)'
                : '0 2px 8px rgba(0,0,0,0.4), inset 0 1px 0 rgba(255,255,255,0.18)',
            }}
          >
            {/* Grip texture */}
            <div className="absolute inset-0 flex justify-center items-center gap-0.5">
              {[...Array(5)].map((_, i) => (
                <div
                  key={i}
                  className="w-px h-3"
                  style={{ backgroundColor: i % 2 === 0 ? '#555' : '#bbb' }}
                />
              ))}
            </div>

            {/* Top highlight */}
            <div className="absolute top-0 left-1 right-1 h-px bg-gradient-to-r from-transparent via-white/25 to-transparent" />
          </div>

          {/* Vertical stem (T-stem) */}
          <div
            className="mx-auto rounded-b"
            style={{
              width: 12,
              height: 14,
              background: 'linear-gradient(to bottom, #666, #444)',
              marginTop: -1,
            }}
          />
        </div>
      </div>

      {/* Value display */}
      <span
        className="text-[10px] font-mono mt-0.5 transition-colors"
        style={{ color: value > 0.8 ? '#ef4444' : value > 0 ? '#888' : '#444' }}
      >
        {Math.round(value * 100)}
      </span>
    </div>
  );
};

export default React.memo(DJVolumeFader);
