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
    const handleHeight = 20;
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
          className={`text-[9px] font-bold mb-1 transition-colors`}
          style={{ color: isPlaying ? accentColor : '#666' }}
        >
          {label}
        </span>
      )}
      
      {/* Fader track container */}
      <div
        className="relative cursor-pointer touch-none"
        style={{ width: 36, height: trackHeight }}
        onPointerDown={handlePointerDown}
        onPointerMove={handlePointerMove}
        onPointerUp={handlePointerUp}
        onPointerLeave={handlePointerUp}
      >
        {/* Track background */}
        <div 
          className="absolute left-1/2 -translate-x-1/2 rounded"
          style={{
            width: 6,
            height: trackHeight - 10,
            top: 5,
            background: 'linear-gradient(to bottom, #0d0d0d, #1a1a1a)',
            boxShadow: 'inset 0 2px 4px rgba(0,0,0,0.5)',
          }}
        />
        
        {/* Active level fill */}
        <div 
          className="absolute left-1/2 -translate-x-1/2 rounded-b transition-all duration-50"
          style={{
            width: 4,
            height: Math.max(0, value * (trackHeight - 10)),
            bottom: 5,
            background: `linear-gradient(to top, ${accentColor}60, ${accentColor})`,
            boxShadow: isPlaying ? `0 0 8px ${accentColor}40` : 'none',
          }}
        />
        
        {/* VU meter markers */}
        <div className="absolute right-0 top-0 bottom-0 flex flex-col justify-between py-1.5 pr-0.5">
          {[100, 80, 60, 40, 20, 0].map((pct, i) => (
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
            top: `calc(${handlePercent}% - 10px)`,
            transition: isDragging ? 'none' : 'top 0.05s',
          }}
        >
          {/* Horizontal bar (T-top) */}
          <div 
            className="rounded relative overflow-hidden"
            style={{
              width: 30,
              height: 10,
              background: isDragging
                ? 'linear-gradient(to bottom, #999, #777)'
                : 'linear-gradient(to bottom, #888, #666)',
              boxShadow: isDragging
                ? '0 3px 8px rgba(0,0,0,0.4), inset 0 1px 0 rgba(255,255,255,0.2)'
                : '0 2px 6px rgba(0,0,0,0.3), inset 0 1px 0 rgba(255,255,255,0.15)',
            }}
          >
            {/* Grip texture */}
            <div className="absolute inset-0 flex justify-center items-center gap-0.5">
              {[...Array(3)].map((_, i) => (
                <div 
                  key={i} 
                  className="w-px h-2"
                  style={{ backgroundColor: i % 2 === 0 ? '#555' : '#999' }} 
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
              width: 10,
              height: 14,
              background: 'linear-gradient(to bottom, #666, #444)',
              marginTop: -1,
            }}
          />
        </div>
      </div>
      
      {/* Value display */}
      <span 
        className="text-[8px] font-mono mt-0.5 transition-colors"
        style={{ color: value > 0.8 ? '#ef4444' : value > 0 ? '#888' : '#444' }}
      >
        {Math.round(value * 100)}
      </span>
    </div>
  );
};

export default React.memo(DJVolumeFader);
