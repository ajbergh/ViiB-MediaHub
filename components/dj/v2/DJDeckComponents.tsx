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
}

/**
 * Self-subscribing time display driven by rAF rather than store subscriptions.
 * Throttles to ~4 fps while paused and ~2 fps when no track is loaded, so it
 * never contributes to position-driven parent re-renders.
 */
export const DeckTimeDisplay = React.memo<DeckTimeDisplayProps>(
  ({ deck, color, showRemaining = false }) => {
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
        className='text-[11px] font-mono'
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
}

/**
 * Tiny self-subscribing BPM readout. Isolates the `effectiveBpm` subscription
 * so parent components don't re-render when BPM changes during tempo dragging.
 */
export const DeckBpmBadge = React.memo<DeckBpmBadgeProps>(({ deck }) => {
  const bpm = useStore(
    s => deck === 'A' ? s.djDeckA.effectiveBpm : s.djDeckB.effectiveBpm,
  );

  if (!bpm) return null;
  return (
    <span className='text-[11px] font-mono font-semibold text-neutral-300'>
      {bpm.toFixed(1)}
    </span>
  );
});
DeckBpmBadge.displayName = 'DeckBpmBadge';
