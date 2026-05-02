/**
 * ViiB MediaHub - DJ Mode V2 Deck Display Helpers
 *
 * Lightweight self-subscribing components that were previously inlined in
 * DJModeV2.tsx. Extracting them here keeps the page file readable and ensures
 * each component only re-renders when its own slice of state changes.
 *
 * Components:
 *  - DeckTimeDisplay  — rAF-driven elapsed/remaining time counter
 *  - DeckHasTrack     — render-guard: only renders children when a track is loaded
 *  - DeckBpmBadge     — small BPM readout badge
 *
 * @module components/dj/v2/DJDeckComponents
 */

import React, { useEffect, useRef } from 'react';
import { useStore } from '../../../store';
import type { DeckId } from '../../../slices/djMixerSlice';

// ---------------------------------------------------------------------------
// DeckTimeDisplay
// ---------------------------------------------------------------------------

interface DeckTimeDisplayProps {
  deck: DeckId;
  color: string;
  /** When true, shows remaining time as a negative value */
  showRemaining?: boolean;
  /** Tailwind text-size class. Default 'text-[11px]'. */
  sizeClass?: string;
}

/**
 * Self-subscribing time display driven by rAF rather than store subscriptions.
 * Throttles to ~4 fps while paused and ~2 fps when no track is loaded, so it
 * never contributes to position-driven parent re-renders.
 */
export const DeckTimeDisplay = React.memo<DeckTimeDisplayProps>(
  ({ deck, color, showRemaining = false, sizeClass = 'text-[11px]' }) => {
    const containerRef = useRef<HTMLDivElement>(null);

    useEffect(() => {
      let animId: number;
      let timeoutId: ReturnType<typeof setTimeout>;
      let lastPos = -1;

      const update = () => {
        const state = useStore.getState();
        const d = deck === 'A' ? state.djDeckA : state.djDeckB;
        const el = containerRef.current;

        if (el && d.duration > 0) {
          const pos = d.position;
          const remaining = d.duration - pos;
          const formatT = (s: number, neg = false) => {
            const m = Math.floor(Math.abs(s) / 60);
            const sec = Math.floor(Math.abs(s) % 60);
            return `${neg ? '-' : ''}${m}:${sec.toString().padStart(2, '0')}`;
          };

          el.textContent = showRemaining ? formatT(remaining, true) : formatT(pos);

          // Throttle to ~4 fps when paused
          if (!d.isPlaying && pos === lastPos) {
            timeoutId = setTimeout(() => { animId = requestAnimationFrame(update); }, 250);
          } else {
            animId = requestAnimationFrame(update);
          }
          lastPos = pos;
        } else {
          // No track — throttle to ~2 fps
          timeoutId = setTimeout(() => { animId = requestAnimationFrame(update); }, 500);
        }
      };

      animId = requestAnimationFrame(update);
      return () => {
        cancelAnimationFrame(animId);
        clearTimeout(timeoutId);
      };
    }, [deck, showRemaining]);

    return (
      <span
        ref={containerRef}
        className={`${sizeClass} font-mono tabular-nums`}
        style={{ color }}
      >
        --:--
      </span>
    );
  },
);
DeckTimeDisplay.displayName = 'DeckTimeDisplay';

// ---------------------------------------------------------------------------
// DeckHasTrack
// ---------------------------------------------------------------------------

interface DeckHasTrackProps {
  deck: DeckId;
  children: React.ReactNode;
}

/**
 * Render guard — only renders children when the deck has a loaded track with
 * valid duration. Uses two fine-grained selectors so the parent is not
 * re-rendered when position changes.
 */
export const DeckHasTrack = React.memo<DeckHasTrackProps>(({ deck, children }) => {
  const hasTrack = useStore(
    state => deck === 'A' ? !!state.djDeckA.track : !!state.djDeckB.track,
  );
  const hasDuration = useStore(
    state => deck === 'A' ? state.djDeckA.duration > 0 : state.djDeckB.duration > 0,
  );

  if (!hasTrack || !hasDuration) return null;
  return <>{children}</>;
});
DeckHasTrack.displayName = 'DeckHasTrack';

// ---------------------------------------------------------------------------
// DeckBpmBadge
// ---------------------------------------------------------------------------

interface DeckBpmBadgeProps {
  deck: DeckId;
  /** When true, render as a large prominent BPM display. */
  large?: boolean;
}

/**
 * Self-subscribing BPM readout. Isolates the `effectiveBpm` subscription
 * so parent components don't re-render when BPM changes during tempo dragging.
 */
export const DeckBpmBadge = React.memo<DeckBpmBadgeProps>(({ deck, large = false }) => {
  const bpm = useStore(
    s => deck === 'A' ? s.djDeckA.effectiveBpm : s.djDeckB.effectiveBpm,
  );
  const accent = deck === 'A' ? '#3b82f6' : '#8b5cf6';

  if (!bpm) {
    return large ? (
      <div className='flex flex-col items-center'>
        <span className='text-[10px] text-[#444] font-bold uppercase tracking-widest leading-none'>BPM</span>
        <span className='text-[24px] font-mono tabular-nums font-bold text-[#333] leading-tight'>--.--</span>
      </div>
    ) : null;
  }

  if (large) {
    return (
      <div className='flex flex-col items-center'>
        <span className='text-[10px] text-[#666] font-bold uppercase tracking-widest leading-none'>BPM</span>
        <span
          className='text-[26px] font-mono tabular-nums font-bold leading-tight'
          style={{ color: accent }}
        >
          {bpm.toFixed(1)}
        </span>
      </div>
    );
  }
  return (
    <span className='text-[11px] font-mono font-semibold text-neutral-300'>
      {bpm.toFixed(1)}
    </span>
  );
});
DeckBpmBadge.displayName = 'DeckBpmBadge';

// ---------------------------------------------------------------------------
// DeckHorizontalVU — wide stereo strip for use in the deck header
// ---------------------------------------------------------------------------

interface DeckHorizontalVUProps {
  /** Function returning the deck's stereo levels (0..1). */
  getLevels: () => { left: number; right: number };
  /** Bar width in px. */
  width?: number;
  /** Bar height per channel in px. */
  channelHeight?: number;
  /** Accent for the bar fill (defaults to a green→amber→red gradient). */
  accentColor?: string;
}

/**
 * Slim two-channel horizontal VU meter — drawn via canvas + rAF so it never
 * triggers React re-renders. Designed to live in the deck header right-edge,
 * giving DJs a quick "is this deck hot" glance without looking at the mixer
 * channel strip.
 */
export const DeckHorizontalVU = React.memo<DeckHorizontalVUProps>(
  ({ getLevels, width = 160, channelHeight = 6, accentColor }) => {
    const canvasRef = useRef<HTMLCanvasElement | null>(null);
    const peakRef = useRef<{ l: number; r: number; lt: number; rt: number }>({ l: 0, r: 0, lt: 0, rt: 0 });

    useEffect(() => {
      const canvas = canvasRef.current;
      if (!canvas) return;
      const dpr = window.devicePixelRatio || 1;
      const totalH = channelHeight * 2 + 2;
      canvas.width = width * dpr;
      canvas.height = totalH * dpr;
      canvas.style.width = `${width}px`;
      canvas.style.height = `${totalH}px`;
      const ctx = canvas.getContext('2d');
      if (!ctx) return;
      ctx.scale(dpr, dpr);

      let raf = 0;
      const draw = () => {
        const { left, right } = getLevels();
        const now = performance.now();
        // Peak hold (decay 0.4 per second)
        if (left  > peakRef.current.l) { peakRef.current.l = left;  peakRef.current.lt = now; }
        if (right > peakRef.current.r) { peakRef.current.r = right; peakRef.current.rt = now; }
        const decay = (val: number, t: number) => Math.max(0, val - (now - t) / 1000 * 0.4);

        // Track background
        ctx.fillStyle = '#0a0a0a';
        ctx.fillRect(0, 0, width, totalH);

        const drawBar = (y: number, lvl: number, peak: number) => {
          const barW = Math.min(1, lvl) * width;
          // Gradient: green → amber → red
          if (accentColor) {
            ctx.fillStyle = accentColor;
            ctx.fillRect(0, y, barW, channelHeight);
          } else {
            const grad = ctx.createLinearGradient(0, 0, width, 0);
            grad.addColorStop(0, '#22c55e');
            grad.addColorStop(0.6, '#22c55e');
            grad.addColorStop(0.78, '#f59e0b');
            grad.addColorStop(0.92, '#f59e0b');
            grad.addColorStop(1, '#ef4444');
            ctx.fillStyle = grad;
            ctx.fillRect(0, y, barW, channelHeight);
          }
          // Peak line
          if (peak > 0.02) {
            ctx.fillStyle = peak > 0.92 ? '#ef4444' : peak > 0.78 ? '#f59e0b' : '#86efac';
            ctx.fillRect(Math.min(width - 2, peak * width), y, 2, channelHeight);
          }
        };

        drawBar(0, left,  decay(peakRef.current.l, peakRef.current.lt));
        drawBar(channelHeight + 2, right, decay(peakRef.current.r, peakRef.current.rt));

        // Decay updates
        peakRef.current.l = decay(peakRef.current.l, peakRef.current.lt);
        peakRef.current.r = decay(peakRef.current.r, peakRef.current.rt);

        raf = requestAnimationFrame(draw);
      };
      raf = requestAnimationFrame(draw);
      return () => cancelAnimationFrame(raf);
    }, [getLevels, width, channelHeight, accentColor]);

    return <canvas ref={canvasRef} className='block rounded-sm' aria-hidden='true' />;
  },
);
DeckHorizontalVU.displayName = 'DeckHorizontalVU';
