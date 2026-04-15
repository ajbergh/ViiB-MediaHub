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
        const availableWidth = parent.clientWidth * 0.4; // 40% of parent width
        setComputedWidth(Math.max(120, Math.min(availableWidth, 280)));
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
    const handleWidth = 36;
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
      <div className="flex justify-between text-[9px] font-bold mb-1 px-2">
        <span className="text-[#3b82f6]">A</span>
        <span className="text-neutral-500 text-[8px] font-medium">Crossfader</span>
        <span className="text-[#8b5cf6]">B</span>
      </div>
      
      {/* Fader track container */}
      <div
        className="relative cursor-pointer touch-none bg-[#1a1a1a] rounded-lg p-1"
        style={{ width: computedWidth, height: 36 }}
        onPointerDown={handlePointerDown}
        onPointerMove={handlePointerMove}
        onPointerUp={handlePointerUp}
        onPointerLeave={handlePointerUp}
      >
        {/* Track groove */}
        <div 
          className="absolute top-1/2 -translate-y-1/2 rounded-full"
          style={{
            left: 18,
            right: 18,
            height: 8,
            background: 'linear-gradient(to bottom, #0d0d0d, #1a1a1a, #0d0d0d)',
            boxShadow: 'inset 0 2px 4px rgba(0,0,0,0.5)',
          }}
        />
        
        {/* Center marker line */}
        <div 
          className="absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 rounded"
          style={{
            width: 3,
            height: 18,
            backgroundColor: isCentered ? '#22c55e' : '#333',
            transition: 'background-color 0.1s',
          }}
        />
        
        {/* Active fills */}
        {value < -0.02 && (
          <div 
            className="absolute top-1/2 -translate-y-1/2 rounded-full"
            style={{
              left: 18 + ((value + 1) / 2) * (computedWidth - 36),
              width: (computedWidth / 2) - 18 - ((value + 1) / 2) * (computedWidth - 36),
              height: 6,
              background: 'linear-gradient(to right, #3b82f6, #3b82f640)',
              boxShadow: '0 0 6px #3b82f640',
            }}
          />
        )}
        {value > 0.02 && (
          <div 
            className="absolute top-1/2 -translate-y-1/2 rounded-full"
            style={{
              left: computedWidth / 2,
              width: (value / 2) * (computedWidth - 36),
              height: 6,
              background: 'linear-gradient(to right, #8b5cf640, #8b5cf6)',
              boxShadow: '0 0 6px #8b5cf640',
            }}
          />
        )}
        
        {/* Handle */}
        <div 
          className={`absolute top-1/2 -translate-y-1/2 cursor-grab ${isDragging ? 'cursor-grabbing scale-105' : ''}`}
          style={{ 
            left: `calc(${handlePercent}% - 18px)`,
            transition: isDragging ? 'none' : 'transform 0.1s',
          }}
        >
          {/* Handle body */}
          <div
            className="rounded-md relative overflow-hidden"
            style={{
              width: 36,
              height: 26,
              background: 'linear-gradient(to bottom, #7a7a7a, #555, #444)',
              boxShadow: isDragging 
                ? '0 4px 12px rgba(0,0,0,0.5), inset 0 1px 0 rgba(255,255,255,0.2)' 
                : '0 2px 8px rgba(0,0,0,0.4), inset 0 1px 0 rgba(255,255,255,0.15)',
            }}
          >
            {/* Grip texture */}
            <div className="absolute inset-0 flex justify-center items-center gap-0.5">
              {[...Array(5)].map((_, i) => (
                <div 
                  key={i} 
                  className="w-px h-4"
                  style={{ 
                    backgroundColor: i % 2 === 0 ? '#666' : '#888'
                  }} 
                />
              ))}
            </div>
            
            {/* Top highlight */}
            <div 
              className="absolute top-0 left-1 right-1 h-px bg-gradient-to-r from-transparent via-white/30 to-transparent"
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
