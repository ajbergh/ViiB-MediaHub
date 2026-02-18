/**
 * ViiB MediaHub - DJ VU Meter Component
 * 
 * Real-time VU meter that reads analyser data from the DJ Audio Engine.
 * Uses requestAnimationFrame for smooth 60fps updates via canvas rendering,
 * avoiding React re-renders for performance.
 * 
 * Supports vertical bar style with green → yellow → red color gradient,
 * peak hold indicators, and configurable segment counts.
 * 
 * @module components/dj/v2/DJVUMeter
 */

import React, { useRef, useEffect, useCallback, memo } from 'react';

interface DJVUMeterProps {
  /** Function that returns current level (0-1) when called */
  getLevel: () => number;
  /** Height in pixels */
  height?: number;
  /** Width in pixels */
  width?: number;
  /** Number of LED segments */
  segments?: number;
  /** Whether to show peak hold indicator */
  showPeak?: boolean;
  /** Peak hold time in ms */
  peakHoldTime?: number;
  /** Peak fall speed (segments per frame) */
  peakFallSpeed?: number;
  /** Orientation */
  orientation?: 'vertical' | 'horizontal';
  /** Label text (e.g., "L", "R") */
  label?: string;
}

// Color thresholds for LED segments (as fraction of total segments)
const GREEN_THRESHOLD = 0.6;
const YELLOW_THRESHOLD = 0.85;

function getSegmentColor(segmentIndex: number, totalSegments: number): string {
  const fraction = segmentIndex / totalSegments;
  if (fraction < GREEN_THRESHOLD) return '#22c55e'; // green-500
  if (fraction < YELLOW_THRESHOLD) return '#eab308'; // yellow-500  
  return '#ef4444'; // red-500
}

function getSegmentDimColor(segmentIndex: number, totalSegments: number): string {
  const fraction = segmentIndex / totalSegments;
  if (fraction < GREEN_THRESHOLD) return '#052e16'; // very dark green
  if (fraction < YELLOW_THRESHOLD) return '#422006'; // very dark yellow
  return '#450a0a'; // very dark red
}

const DJVUMeter = memo(function DJVUMeter({
  getLevel,
  height = 120,
  width = 8,
  segments = 16,
  showPeak = true,
  peakHoldTime = 1000,
  peakFallSpeed = 0.15,
}: DJVUMeterProps) {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const rafRef = useRef<number>(0);
  const peakRef = useRef(0);
  const peakHoldRef = useRef(0);
  const peakTimerRef = useRef(0);
  const smoothLevelRef = useRef(0);

  const draw = useCallback(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    const ctx = canvas.getContext('2d');
    if (!ctx) return;

    const rawLevel = getLevel();
    // Smooth the level (fast attack, slow release) for natural meter response
    const target = rawLevel;
    const current = smoothLevelRef.current;
    smoothLevelRef.current = target > current
      ? current + (target - current) * 0.4  // fast attack
      : current + (target - current) * 0.08; // slow release (ballistic)
    
    const level = smoothLevelRef.current;

    const segHeight = Math.floor((height - (segments - 1)) / segments);
    const gap = 1;
    const activeSegments = Math.round(level * segments);

    // Peak tracking
    const now = performance.now();
    if (activeSegments > peakRef.current) {
      peakRef.current = activeSegments;
      peakTimerRef.current = now;
      peakHoldRef.current = activeSegments;
    } else if (now - peakTimerRef.current > peakHoldTime) {
      // Peak falling
      peakHoldRef.current -= peakFallSpeed;
      if (peakHoldRef.current < activeSegments) {
        peakHoldRef.current = activeSegments;
      }
      peakRef.current = Math.max(peakHoldRef.current, 0);
    }

    // Clear
    ctx.clearRect(0, 0, width, height);

    // Draw segments (bottom to top)
    for (let i = 0; i < segments; i++) {
      const y = height - (i + 1) * (segHeight + gap);
      const isActive = i < activeSegments;
      const isPeak = showPeak && Math.floor(peakHoldRef.current) === i && i >= activeSegments;

      if (isActive) {
        ctx.fillStyle = getSegmentColor(i, segments);
        ctx.globalAlpha = 0.95;
      } else if (isPeak) {
        ctx.fillStyle = getSegmentColor(i, segments);
        ctx.globalAlpha = 0.8;
      } else {
        ctx.fillStyle = getSegmentDimColor(i, segments);
        ctx.globalAlpha = 0.4;
      }

      // Rounded rectangle for each segment
      const radius = Math.min(1.5, segHeight / 2, width / 2);
      ctx.beginPath();
      ctx.roundRect(0, y, width, segHeight, radius);
      ctx.fill();
      ctx.globalAlpha = 1;
    }

    rafRef.current = requestAnimationFrame(draw);
  }, [getLevel, height, width, segments, showPeak, peakHoldTime, peakFallSpeed]);

  useEffect(() => {
    rafRef.current = requestAnimationFrame(draw);
    return () => {
      if (rafRef.current) cancelAnimationFrame(rafRef.current);
    };
  }, [draw]);

  return (
    <canvas
      ref={canvasRef}
      width={width}
      height={height}
      className="block"
      style={{ width, height }}
    />
  );
});

// ============================================================================
// Stereo VU Meter (L+R pair with optional label)
// ============================================================================

interface DJStereoVUMeterProps {
  /** Function that returns { left, right } levels (0-1) */
  getLevels: () => { left: number; right: number };
  /** Height in pixels */
  height?: number;
  /** Width per channel in pixels */
  channelWidth?: number;
  /** Gap between L/R channels */
  gap?: number;
  /** Number of LED segments */
  segments?: number;
  /** Label (e.g., "A", "B", "M") */
  label?: string;
  /** Show peak hold */
  showPeak?: boolean;
}

export const DJStereoVUMeter = memo(function DJStereoVUMeter({
  getLevels,
  height = 120,
  channelWidth = 6,
  gap = 2,
  segments = 16,
  label,
  showPeak = true,
}: DJStereoVUMeterProps) {
  const getLevelLeft = useCallback(() => getLevels().left, [getLevels]);
  const getLevelRight = useCallback(() => getLevels().right, [getLevels]);

  return (
    <div className="flex flex-col items-center">
      {label && (
        <span className="text-[7px] text-[#555] font-bold mb-0.5 tracking-wider">{label}</span>
      )}
      <div className="flex items-end" style={{ gap }}>
        <DJVUMeter
          getLevel={getLevelLeft}
          height={height}
          width={channelWidth}
          segments={segments}
          showPeak={showPeak}
        />
        <DJVUMeter
          getLevel={getLevelRight}
          height={height}
          width={channelWidth}
          segments={segments}
          showPeak={showPeak}
        />
      </div>
    </div>
  );
});

export default DJVUMeter;
