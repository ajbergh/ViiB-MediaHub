/**
 * ViiB MediaHub - DJ EQ Knob Component (v2)
 * 
 * Professional rotary knob control for EQ bands inspired by DJ mixers.
 * Features high-quality SVG with metallic textures and value indicators.
 * 
 * @module components/dj/v2/DJEQKnob
 */

import React, { useCallback, useId, useRef, useState } from 'react';

interface DJEQKnobProps {
  label: string;
  value: number;       // -24 to +12
  onChange: (value: number) => void;
  color?: string;
  size?: number;
  /** Compact mode: hide value bar + numeric readout to save vertical space (used in EQ strip column). */
  compact?: boolean;
  valueText?: string;
  className?: string;
}

export const DJEQKnob: React.FC<DJEQKnobProps> = React.memo(({
  label,
  value,
  onChange,
  color = 'var(--dj-text-secondary)',
  size = 44,
  compact = false,
  valueText,
  className = '',
}) => {
  const containerRef = useRef<HTMLDivElement>(null);
  const [isDragging, setIsDragging] = useState(false);
  const startYRef = useRef<number>(0);
  const startValueRef = useRef<number>(0);

  // Per-instance unique IDs for SVG <defs> — without this, knobs sharing a label
  // (e.g. HIGH on both decks) collide globally and share filter/gradient state.
  const uid = useId().replace(/[:]/g, '');
  const metalId  = `knobMetal-${uid}`;
  const innerId  = `knobInner-${uid}`;
  const glowId   = `knobGlow-${uid}`;

  // Map value (-24 to +12) to angle (-135° to +135°)
  const valueToAngle = (v: number): number => {
    const normalized = (v + 24) / 36;
    return (normalized * 270) - 135;
  };

  const angle = valueToAngle(value);
  // Visual size matches `size` prop. Previously knob was `size - 8` which
  // silently shrunk every consumer's hit area.
  const knobSize = size;

  const handlePointerDown = useCallback((e: React.PointerEvent) => {
    e.preventDefault();
    (e.target as HTMLElement).setPointerCapture(e.pointerId);
    setIsDragging(true);
    startYRef.current = e.clientY;
    startValueRef.current = value;
  }, [value]);

  const handlePointerMove = useCallback((e: React.PointerEvent) => {
    if (!isDragging) return;

    const deltaY = startYRef.current - e.clientY;
    // Shift = fine (0.2x), default 0.5
    const sensitivity = e.shiftKey ? 0.1 : 0.5;
    const deltaValue = deltaY * sensitivity;

    const newValue = Math.max(-24, Math.min(12, startValueRef.current + deltaValue));
    onChange(Math.round(newValue));
  }, [isDragging, onChange]);

  const handlePointerUp = useCallback((e: React.PointerEvent) => {
    (e.target as HTMLElement).releasePointerCapture(e.pointerId);
    setIsDragging(false);
  }, []);

  const handleDoubleClick = useCallback(() => {
    onChange(0);
  }, [onChange]);

  // Mouse-wheel fine adjust
  const handleWheel = useCallback((e: React.WheelEvent) => {
    e.preventDefault();
    const step = e.shiftKey ? 1 : 2;
    const delta = e.deltaY > 0 ? -step : step;
    const newValue = Math.max(-24, Math.min(12, value + delta));
    if (newValue !== value) onChange(newValue);
  }, [value, onChange]);

  // Keyboard adjust (Arrow keys, Home reset)
  const handleKeyDown = useCallback((e: React.KeyboardEvent) => {
    let next: number | null = null;
    const big = e.shiftKey ? 5 : 1;
    if (e.key === 'ArrowUp' || e.key === 'ArrowRight') next = Math.min(12, value + big);
    else if (e.key === 'ArrowDown' || e.key === 'ArrowLeft') next = Math.max(-24, value - big);
    else if (e.key === 'Home' || e.key === 'Enter' || e.key === ' ') next = 0;
    else if (e.key === 'PageUp') next = Math.min(12, value + 5);
    else if (e.key === 'PageDown') next = Math.max(-24, value - 5);
    if (next !== null && next !== value) {
      e.preventDefault();
      onChange(next);
    }
  }, [value, onChange]);

  const roundedValue = Math.round(value * 10) / 10;
  const displayValue = roundedValue > 0 ? `+${roundedValue}` : `${roundedValue}`;
  
  // Calculate indicator bar width (centered at 0)
  const indicatorPercent = ((value + 24) / 36) * 100;
  const isPositive = value > 0;
  const isNegative = value < 0;

  return (
    <div 
      ref={containerRef}
      className={`group relative flex flex-col items-center select-none ${className}`}
      style={{ width: size }}
    >
      {/* Label */}
      <span
        className={`dj-eq-knob-label text-[12px] font-bold uppercase tracking-wider ${compact ? 'leading-none mb-0' : 'mb-0.5'}`}
        style={{ color: '#bac6d6' }}
      >
        {label}
      </span>
      
      {/* Knob SVG */}
      <div
        className={`relative cursor-grab rounded-full focus:outline-none focus-visible:ring-2 focus-visible:ring-brand/60 ${isDragging ? 'cursor-grabbing' : ''}`}
        style={{ width: knobSize, height: knobSize }}
        role="slider"
        tabIndex={0}
        aria-label={label}
        aria-valuetext={valueText ?? `${displayValue} dB`}
        title={`${label}: ${valueText ?? `${displayValue} dB`}`}
        aria-valuemin={-24}
        aria-valuemax={12}
        aria-valuenow={value}
        onPointerDown={handlePointerDown}
        onPointerMove={handlePointerMove}
        onPointerUp={handlePointerUp}
        onPointerLeave={handlePointerUp}
        onDoubleClick={handleDoubleClick}
        onWheel={handleWheel}
        onKeyDown={handleKeyDown}
      >
        <svg
          width={knobSize}
          height={knobSize}
          viewBox="0 0 40 40"
          className="touch-none"
        >
          <defs>
            {/* Metallic gradient */}
            <radialGradient id={metalId} cx="35%" cy="30%" r="70%">
              <stop offset="0%" stopColor="var(--dj-text-muted)"/>
              <stop offset="40%" stopColor="var(--dj-border-hover)"/>
              <stop offset="100%" stopColor="var(--dj-surface-3)"/>
            </radialGradient>

            {/* Inner shadow */}
            <filter id={innerId}>
              <feDropShadow dx="1" dy="2" stdDeviation="2" floodColor="#000" floodOpacity="0.5"/>
            </filter>

            {/* Glow when adjusted */}
            <filter id={glowId} x="-50%" y="-50%" width="200%" height="200%">
              <feGaussianBlur stdDeviation="2" result="blur"/>
              <feMerge>
                <feMergeNode in="blur"/>
                <feMergeNode in="SourceGraphic"/>
              </feMerge>
            </filter>
          </defs>

          {/* Knob shadow ring */}
          <circle cx="20" cy="20" r="16" fill="var(--dj-surface-2)" />

          {/* Knob body */}
          <circle
            cx="20"
            cy="20"
            r="14"
            fill={`url(#${metalId})`}
            filter={`url(#${innerId})`}
          />

          {/* Rim highlight */}
          <circle cx="20" cy="20" r="14" fill="none" stroke="var(--dj-text-muted)" strokeWidth="0.5" />

          {/* Inner circle detail */}
          <circle cx="20" cy="20" r="10" fill="none" stroke="var(--dj-border-light)" strokeWidth="0.5" />

          {/* Notch marks around edge */}
          {[-135, -90, -45, 0, 45, 90, 135].map((markAngle) => (
            <line
              key={markAngle}
              x1="20"
              y1="5"
              x2="20"
              y2={markAngle === 0 ? 8 : 7}
              stroke={markAngle === 0 ? 'var(--dj-text-muted)' : 'var(--dj-border-hover)'}
              strokeWidth={markAngle === 0 ? 2 : 1}
              transform={`rotate(${markAngle}, 20, 20)`}
            />
          ))}

          {/* Dark outline keeps the pointer legible over metallic highlights. */}
          <line x1="20" y1="7" x2="20" y2="18" stroke="var(--dj-bg)" strokeWidth="5" strokeLinecap="round" transform={`rotate(${angle}, 20, 20)`} />
          {/* White pointer rotates with the actual knob value, including at zero. */}
          <line
            x1="20"
            y1="7"
            x2="20"
            y2="18"
            stroke="var(--dj-text-primary)"
            strokeWidth="3"
            strokeLinecap="round"
            transform={`rotate(${angle}, 20, 20)`}
          />

          {/* Center dot */}
          <circle cx="20" cy="20" r="2" fill="var(--dj-surface-3)" stroke="var(--dj-border-light)" strokeWidth="0.5" />
        </svg>
      </div>
      {compact && (
        <span className={`absolute top-1/2 left-1/2 -translate-x-1/2 z-40 pointer-events-none whitespace-nowrap rounded border border-slate-500 bg-slate-950 px-2 py-1 text-[12px] font-mono text-white shadow-lg ${isDragging ? 'block' : 'hidden group-hover:block group-focus-within:block'}`}>
          {valueText ?? `${displayValue} dB`}
        </span>
      )}
      
      {/* Value indicator bar (hidden in compact mode to save vertical space) */}
      {!compact && (
        <div className="w-full h-1 bg-[var(--dj-surface-3)] rounded-sm mt-1 overflow-hidden relative">
          {/* Center marker */}
          <div className="absolute left-1/2 -translate-x-1/2 top-0 w-0.5 h-full bg-[var(--dj-border-hover)]" />

          {/* Value fill */}
          {value !== 0 && (
            <div
              className="absolute h-full rounded-sm transition-all duration-50"
              style={{
                backgroundColor: color,
                left: isNegative ? `${indicatorPercent}%` : '66.7%',
                width: isNegative
                  ? `${66.7 - indicatorPercent}%`
                  : `${indicatorPercent - 66.7}%`,
                boxShadow: `0 0 4px color-mix(in srgb, ${color} 25%, transparent)`,
              }}
            />
          )}
        </div>
      )}

      {/* Value display */}
      {!compact && (
        <span
          className="dj-eq-knob-readout text-[12px] font-mono mt-0.5 transition-colors whitespace-nowrap"
          style={{ color: 'var(--dj-text-primary)' }}
        >
          {valueText ?? displayValue}
        </span>
      )}
    </div>
  );
});

export default DJEQKnob;
