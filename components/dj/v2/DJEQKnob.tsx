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
  /** Render the label under the knob (FX rack modules, per the mock-up). */
  labelBelow?: boolean;
  /** Accessible name when the visible label is ambiguous (e.g. FX parameters). */
  ariaLabel?: string;
  /** Unipolar pot (0..100% sweep from -135° to +135°, used for FX params and Master level). Defaults to false (bipolar center-detent at 0 dB = 12 o'clock). */
  unipolar?: boolean;
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
  labelBelow = false,
  ariaLabel,
  unipolar = false,
}) => {
  const containerRef = useRef<HTMLDivElement>(null);
  const [isDragging, setIsDragging] = useState(false);
  const startYRef = useRef<number>(0);
  const startValueRef = useRef<number>(0);

  // Per-instance unique IDs for SVG <defs> — without this, knobs sharing a label
  // (e.g. HIGH on both decks) collide globally and share filter/gradient state.
  const uid = useId().replace(/[:]/g, '');
  const capGradId = `knobCap-${uid}`;
  const skirtGradId = `knobSkirt-${uid}`;
  const shadowId = `knobShadow-${uid}`;

  // Bipolar (EQ/Trim/Filter): 0 dB sits at 0° (12 o'clock), -24 at -135°, +12 at +135°.
  // Unipolar (FX/Main): -24..+12 maps linearly across -135°..+135° (midpoint -6 at 12 o'clock).
  const valueToAngle = (v: number): number => {
    const clamped = Math.max(-24, Math.min(12, v));
    if (unipolar) {
      return ((clamped + 24) / 36) * 270 - 135;
    }
    if (clamped <= 0) {
      return (clamped / 24) * 135;
    }
    return (clamped / 12) * 135;
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

  // Calculate indicator bar width (centered at 50% for 0 dB)
  const indicatorPercent = unipolar
    ? ((Math.max(-24, Math.min(12, value)) + 24) / 36) * 100
    : value <= 0
      ? 50 + (Math.max(-24, value) / 24) * 50
      : 50 + (Math.min(12, value) / 12) * 50;
  const isNegative = value < 0;

  // SVG arc helper (0° = 12 o'clock, clockwise positive)
  const arcRadius = 17;
  const polarToCartesian = (cx: number, cy: number, r: number, deg: number) => {
    const rad = ((deg - 90) * Math.PI) / 180;
    return {
      x: cx + r * Math.cos(rad),
      y: cy + r * Math.sin(rad),
    };
  };

  const describeArc = (startDeg: number, endDeg: number) => {
    const start = polarToCartesian(20, 20, arcRadius, startDeg);
    const end = polarToCartesian(20, 20, arcRadius, endDeg);
    const sweep = endDeg >= startDeg ? 1 : 0;
    const largeArc = Math.abs(endDeg - startDeg) > 180 ? 1 : 0;
    return `M ${start.x.toFixed(2)} ${start.y.toFixed(2)} A ${arcRadius} ${arcRadius} 0 ${largeArc} ${sweep} ${end.x.toFixed(2)} ${end.y.toFixed(2)}`;
  };

  const trackArcPath = describeArc(-135, 135);
  const hasActiveArc = unipolar ? angle > -133 : Math.abs(value) > 0.05;
  const activeArcPath = hasActiveArc
    ? unipolar
      ? describeArc(-135, angle)
      : describeArc(0, angle)
    : '';

  return (
    <div 
      ref={containerRef}
      className={`group relative flex flex-col items-center select-none ${className}`}
      style={{ width: size }}
    >
      {/* Label */}
      <span
        className={`dj-eq-knob-label text-[11px] font-bold uppercase tracking-wider ${compact ? 'leading-none mb-0.5' : 'mb-0.5'} ${labelBelow ? 'order-last mt-1' : ''}`}
        style={{ color: value !== 0 ? 'var(--dj-text-primary)' : 'var(--dj-text-secondary)' }}
      >
        {label}
      </span>
      
      {/* Knob SVG */}
      <div
        className={`relative cursor-grab rounded-full focus:outline-none focus-visible:ring-2 focus-visible:ring-brand/60 ${isDragging ? 'cursor-grabbing' : ''}`}
        style={{ width: knobSize, height: knobSize }}
        role="slider"
        tabIndex={0}
        aria-label={ariaLabel ?? label}
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
          className="touch-none overflow-visible"
        >
          <defs>
            {/* Machined anodized cap gradient */}
            <linearGradient id={capGradId} x1="0%" y1="0%" x2="0%" y2="100%">
              <stop offset="0%" stopColor="var(--dj-border)" />
              <stop offset="48%" stopColor="var(--dj-surface-2)" />
              <stop offset="100%" stopColor="var(--dj-bg-elevated)" />
            </linearGradient>

            {/* Outer skirt bevel gradient */}
            <linearGradient id={skirtGradId} x1="0%" y1="0%" x2="100%" y2="100%">
              <stop offset="0%" stopColor="var(--dj-surface-3)" />
              <stop offset="100%" stopColor="var(--dj-bg)" />
            </linearGradient>

            {/* Hardware drop shadow */}
            <filter id={shadowId} x="-25%" y="-25%" width="150%" height="150%">
              <feDropShadow dx="0" dy="1.5" stdDeviation="1.5" floodColor="#000" floodOpacity="0.75" />
            </filter>
          </defs>

          {/* 270° Recessed LED Track Arc */}
          <path
            d={trackArcPath}
            fill="none"
            stroke="rgba(255, 255, 255, 0.1)"
            strokeWidth="2.2"
            strokeLinecap="round"
          />

          {/* 12 o'clock Center Detent Calibration Tick */}
          <line
            x1="20"
            y1="0.6"
            x2="20"
            y2="2.6"
            stroke={value === 0 ? 'rgba(255, 255, 255, 0.55)' : 'rgba(255, 255, 255, 0.25)'}
            strokeWidth="1.2"
            strokeLinecap="round"
          />

          {/* Min / Max End Ticks (-135° and +135°) */}
          <line
            x1="20"
            y1="1.2"
            x2="20"
            y2="2.6"
            stroke="rgba(255, 255, 255, 0.22)"
            strokeWidth="1"
            transform="rotate(-135, 20, 20)"
          />
          <line
            x1="20"
            y1="1.2"
            x2="20"
            y2="2.6"
            stroke="rgba(255, 255, 255, 0.22)"
            strokeWidth="1"
            transform="rotate(135, 20, 20)"
          />

          {/* Active LED Value Arc */}
          {hasActiveArc && (
            <path
              d={activeArcPath}
              fill="none"
              stroke={color}
              strokeWidth="2.4"
              strokeLinecap="round"
            />
          )}

          {/* Outer Knurled Skirt */}
          <circle
            cx="20"
            cy="20"
            r="14.2"
            fill={`url(#${skirtGradId})`}
            stroke="rgba(255, 255, 255, 0.08)"
            strokeWidth="0.75"
            filter={`url(#${shadowId})`}
          />

          {/* Subtle Knurl Ticks on Skirt */}
          {[-135, -90, -45, 0, 45, 90, 135].map((markAngle) => (
            <line
              key={markAngle}
              x1="20"
              y1="5.8"
              x2="20"
              y2="7.2"
              stroke="rgba(255, 255, 255, 0.14)"
              strokeWidth="0.8"
              transform={`rotate(${markAngle}, 20, 20)`}
            />
          ))}

          {/* Machined Top Cap */}
          <circle
            cx="20"
            cy="20"
            r="11.8"
            fill={`url(#${capGradId})`}
            stroke="rgba(255, 255, 255, 0.18)"
            strokeWidth="0.75"
          />

          {/* Inner Recessed Dish */}
          <circle
            cx="20"
            cy="20"
            r="8.8"
            fill="var(--dj-surface-1)"
            stroke="rgba(255, 255, 255, 0.06)"
            strokeWidth="0.6"
          />

          {/* Pointer Slot Cutout + High-Contrast Indicator Line */}
          <g transform={`rotate(${angle}, 20, 20)`}>
            <line
              x1="20"
              y1="7.2"
              x2="20"
              y2="16.5"
              stroke="var(--dj-bg)"
              strokeWidth="3.2"
              strokeLinecap="round"
            />
            <line
              x1="20"
              y1="7.6"
              x2="20"
              y2="16.2"
              stroke={value !== 0 ? color : 'var(--dj-text-primary)'}
              strokeWidth="1.8"
              strokeLinecap="round"
            />
          </g>
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
          {(unipolar ? indicatorPercent > 0 : value !== 0) && (
            <div
              className="absolute h-full rounded-sm transition-all duration-50"
              style={{
                backgroundColor: color,
                left: unipolar ? '0%' : isNegative ? `${indicatorPercent}%` : '50%',
                width: unipolar ? `${indicatorPercent}%` : isNegative
                  ? `${50 - indicatorPercent}%`
                  : `${indicatorPercent - 50}%`,
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
