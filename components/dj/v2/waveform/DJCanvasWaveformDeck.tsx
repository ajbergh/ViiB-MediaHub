/**
 * One deck's scrolling waveform lane rendered with Canvas 2D.
 *
 * Owns a single canvas and its own animation loop. Deck state is read with
 * `useStore.getState()` inside the loop, so position ticks never re-render
 * React. Idle decks throttle to ~4 fps.
 *
 * @module components/dj/v2/waveform/DJCanvasWaveformDeck
 */

import React, { useCallback, useEffect, useRef } from 'react';
import { useStore } from '../../../../store';
import { useWaveformScratch } from '../../../../hooks/useWaveformScratch';
import { useDJAudioEngineActions } from '../../../../hooks/useDJAudioEngine';
import { getDJAudioEngine } from '../../../../lib/djAudio';
import type { DeckId } from '../../../../slices/djMixerSlice';
import { drawMainWaveform, timeAtLaneX } from './canvasWaveformRenderer';
import type { WaveformColorMode } from './waveformPalette';

interface DJCanvasWaveformDeckProps {
  deck: DeckId;
  visibleSeconds: number;
  colorMode: WaveformColorMode;
}

export const DJCanvasWaveformDeck = React.memo(function DJCanvasWaveformDeck({ deck, visibleSeconds, colorMode }: DJCanvasWaveformDeckProps) {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const scratch = useWaveformScratch(deck, visibleSeconds);
  const { seek } = useDJAudioEngineActions();

  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    const ctx = canvas.getContext('2d');
    if (!ctx) return;

    let frameId = 0;
    let idleTimer: ReturnType<typeof setTimeout> | undefined;
    let last: Record<string, unknown> | null = null;
    let cancelled = false;

    const draw = () => {
      if (cancelled) return;
      const width = canvas.clientWidth;
      const height = canvas.clientHeight;
      const ratio = window.devicePixelRatio || 1;
      if (width <= 0 || height <= 0) {
        idleTimer = setTimeout(() => { frameId = requestAnimationFrame(draw); }, 250);
        return;
      }
      if (canvas.width !== Math.round(width * ratio) || canvas.height !== Math.round(height * ratio)) {
        canvas.width = Math.round(width * ratio);
        canvas.height = Math.round(height * ratio);
        last = null;
      }

      const state = useStore.getState();
      const d = deck === 'A' ? state.djDeckA : state.djDeckB;
      const engine = getDJAudioEngine();
      const scratching = engine.isScratching(deck);
      const moving = d.isPlaying || scratching;
      // Read position from the engine while moving for smooth 60 fps; the
      // store position is throttled.
      const position = moving && engine?.initialized ? engine.getPosition(deck) : d.position;
      const visual = {
        position, peaks: d.waveformPeaks, duration: d.duration, grid: d.beatGrid, offset: d.beatGridOffset,
        cue: d.cuePoint, loop: d.loop, track: d.track?.id, w: canvas.width, h: canvas.height,
      };
      const unchanged = last && Object.keys(visual).every(key => (visual as Record<string, unknown>)[key] === last![key]);
      if (!unchanged) {
        ctx.setTransform(ratio, 0, 0, ratio, 0, 0);
        drawMainWaveform(ctx, width, height, {
          deck,
          peaks: d.waveformPeaks,
          position,
          duration: d.duration,
          beatGrid: d.beatGrid,
          beatGridOffset: d.beatGridOffset,
          cuePoint: d.cuePoint,
          loop: d.loop,
          visibleSeconds,
          colorMode,
          hasTrack: !!d.track,
        });
        last = visual;
      }
      if (moving) frameId = requestAnimationFrame(draw);
      else idleTimer = setTimeout(() => { frameId = requestAnimationFrame(draw); }, 250);
    };

    frameId = requestAnimationFrame(draw);
    return () => {
      cancelled = true;
      cancelAnimationFrame(frameId);
      if (idleTimer) clearTimeout(idleTimer);
    };
  }, [deck, visibleSeconds, colorMode]);

  const handleDoubleClick = useCallback((event: React.MouseEvent<HTMLCanvasElement>) => {
    const rect = event.currentTarget.getBoundingClientRect();
    const d = deck === 'A' ? useStore.getState().djDeckA : useStore.getState().djDeckB;
    if (!d.duration) return;
    // Rect is post-transform; convert click x back into canvas CSS px.
    const x = (event.clientX - rect.left) * (event.currentTarget.clientWidth / rect.width);
    seek(deck, timeAtLaneX(x, event.currentTarget.clientWidth, getDJAudioEngine().getPosition(deck), visibleSeconds, d.duration));
  }, [deck, seek, visibleSeconds]);

  return (
    <canvas
      ref={canvasRef}
      {...scratch}
      className={`${scratch.className} dj-waveform-main`}
      style={{ touchAction: 'none' }}
      onDoubleClick={handleDoubleClick}
    />
  );
});
