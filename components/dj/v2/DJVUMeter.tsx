/**
 * ViiB MediaHub - DJ VU Meter Component
 * 
 * Canvas level meter that polls a caller-supplied getLevel callback.
 * Uses requestAnimationFrame for canvas updates at the host’s available cadence,
 * avoiding React re-renders for performance.
 * 
 * Supports vertical bar style with green → yellow → red color gradient,
 * peak hold indicators, and configurable segment counts.
 * 
 * @module components/dj/v2/DJVUMeter
 */

import React, { useRef, useEffect, memo } from 'react';
import { djVisualScheduler } from '../../../lib/djVisualScheduler';
import { canvasRoundedRect } from '../../../lib/canvasRoundedRect';

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


interface MeterCanvasProps {
  readLevels: () => number[];
  channels: number;
  height: number;
  width: number;
  gap: number;
  segments: number;
  showPeak: boolean;
  peakHoldTime?: number;
  peakFallSpeed?: number;
}

const MeterCanvas = memo(function MeterCanvas({ readLevels, channels, height, width, gap, segments, showPeak,
  peakHoldTime = 1000, peakFallSpeed = 0.15 }: MeterCanvasProps) {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const readRef = useRef(readLevels);
  readRef.current = readLevels;
  useEffect(() => {
    const canvas = canvasRef.current;
    const ctx = canvas?.getContext('2d');
    if (!canvas || !ctx) return;
    const totalWidth = channels * width + (channels - 1) * gap;
    const segmentHeight = Math.max(1, Math.floor((height - (segments - 1)) / segments));
    const background = document.createElement('canvas');
    background.width = totalWidth; background.height = height;
    const bg = background.getContext('2d');
    if (!bg) return;
    const geometry = Array.from({ length: segments }, (_, i) => ({
      y: height - (i + 1) * (segmentHeight + 1), color: getSegmentColor(i, segments), dim: getSegmentDimColor(i, segments),
    }));
    for (let channel = 0; channel < channels; channel++) for (const segment of geometry) {
      bg.fillStyle = segment.dim; bg.globalAlpha = 0.4;
      bg.beginPath(); canvasRoundedRect(bg, channel * (width + gap), segment.y, width, segmentHeight, 1.5); bg.fill();
    }
    const states = Array.from({ length: channels }, () => ({ level: 0, peak: 0, holdUntil: 0, overloadUntil: 0 }));
    let lastKey = '';
    const danger = getComputedStyle(canvas).getPropertyValue('--dj-danger').trim();
    return djVisualScheduler.subscribe((now, elapsed) => {
      const raw = readRef.current(); // Once per stereo pair per shared visual frame.
      const steps = elapsed / (1000 / 60);
      let active = false;
      const frames = states.map((state, index) => {
        const value = Math.max(0, Math.min(1, Number.isFinite(raw[index]) ? raw[index] : 0));
        if (value >= 1) state.overloadUntil = now + 1200;
        const factor = 1 - Math.pow(value > state.level ? 0.6 : 0.92, steps);
        state.level += (value - state.level) * factor;
        if (state.level < 0.001) state.level = 0;
        const lit = Math.round(state.level * segments);
        if (lit > state.peak) { state.peak = lit; state.holdUntil = now + peakHoldTime; }
        else if (now > state.holdUntil) state.peak = Math.max(lit, state.peak - peakFallSpeed * steps);
        const peak = showPeak && state.peak >= 1 ? Math.min(segments - 1, Math.floor(state.peak)) : -1;
        const overload = now < state.overloadUntil;
        active ||= state.level > 0 || state.peak >= 1 || overload;
        return { lit, peak, overload };
      });
      const key = frames.map(frame => frame.lit + ':' + frame.peak + ':' + frame.overload).join('|');
      if (key === lastKey) return active;
      lastKey = key;
      ctx.clearRect(0, 0, totalWidth, height); ctx.drawImage(background, 0, 0);
      frames.forEach((frame, channel) => {
        const x = channel * (width + gap);
        geometry.forEach((segment, i) => {
          if (i >= frame.lit && (i !== frame.peak || i < frame.lit)) return;
          ctx.fillStyle = segment.color; ctx.globalAlpha = i < frame.lit ? 0.95 : 0.8;
          ctx.beginPath(); canvasRoundedRect(ctx, x, segment.y, width, segmentHeight, 1.5); ctx.fill();
        });
        ctx.globalAlpha = 1;
        if (frame.overload) { ctx.fillStyle = danger; ctx.fillRect(x, 0, width, 5); }
      });
      return active;
    });
  }, [channels, height, width, gap, segments, showPeak, peakHoldTime, peakFallSpeed]);
  return <canvas ref={canvasRef} width={channels * width + (channels - 1) * gap} height={height}
    className='block' data-dj-meter-channels={channels} style={{ width: channels * width + (channels - 1) * gap, height }} />;
});

const DJVUMeter = memo(function DJVUMeter({ getLevel, height = 120, width = 8, segments = 16,
  showPeak = true, peakHoldTime = 1000, peakFallSpeed = 0.15 }: DJVUMeterProps) {
  return <MeterCanvas readLevels={() => [getLevel()]} channels={1} height={height} width={width} gap={0}
    segments={segments} showPeak={showPeak} peakHoldTime={peakHoldTime} peakFallSpeed={peakFallSpeed} />;
});

interface DJStereoVUMeterProps {
  getLevels: () => { left: number; right: number };
  height?: number;
  channelWidth?: number;
  gap?: number;
  segments?: number;
  label?: string;
  showPeak?: boolean;
}

export const DJStereoVUMeter = memo(function DJStereoVUMeter({ getLevels, height = 120, channelWidth = 6,
  gap = 2, segments = 16, label, showPeak = true }: DJStereoVUMeterProps) {
  return <div className='flex flex-col items-center'>
    {label && <span className='dj-label mb-0.5'>{label}</span>}
    <div className='relative flex items-end'>
      <div aria-hidden='true' className='absolute -left-4 top-0 h-full flex flex-col justify-between dj-meter-scale pointer-events-none'>
        <span>1</span><span>½</span><span>0</span>
      </div>
      <MeterCanvas readLevels={() => { const levels = getLevels(); return [levels.left, levels.right]; }}
        channels={2} height={height} width={channelWidth} gap={gap} segments={segments} showPeak={showPeak} />
    </div>
  </div>;
});

export default DJVUMeter;
