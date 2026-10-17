/**
 * One deck's scrolling waveform lane rendered with Canvas 2D.
 *
 * Owns a single canvas and its own animation loop. Deck state is read with
 * `useStore.getState()` inside the loop, so position ticks never re-render
 * React. Idle decks throttle to ~4 fps.
 *
 * @module components/dj/v2/waveform/DJCanvasWaveformDeck
 */

import React, { useCallback, useEffect, useRef, useState } from 'react';
import { useStore } from '../../../../store';
import { useWaveformScratch } from '../../../../hooks/useWaveformScratch';
import { useDJAudioEngineActions } from '../../../../hooks/useDJAudioEngine';
import { getDJAudioEngine } from '../../../../lib/djAudio';
import { currentDJThreeBands, useDJThreeBands } from '../../../../hooks/useDJThreeBands';
import type { DeckId } from '../../../../slices/djMixerSlice';
import { timeAtLaneX, type WaveformOrientation, type WaveformBandSelection } from './canvasWaveformRenderer';
import { CanvasWaveformDriver } from './canvasWaveformDriver';
import { djTrackSourceIdentity } from '../../../../lib/djBpmCorrection';
import type { WaveformColorMode } from './waveformPalette';

interface DJCanvasWaveformDeckProps {
  localBands?: boolean;
  retryGeneration?: number;
  deck: DeckId;
  visibleSeconds: number;
  colorMode: WaveformColorMode;
  orientation?: WaveformOrientation;
  bandSelection?: WaveformBandSelection;
}

export const DJCanvasWaveformDeck = React.memo(function DJCanvasWaveformDeck({ deck, visibleSeconds, colorMode, localBands = false, retryGeneration = 0, orientation = 'horizontal', bandSelection = 'all' }: DJCanvasWaveformDeckProps) {
  const bands = useDJThreeBands(deck, localBands, retryGeneration);
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const scratch = useWaveformScratch(deck, visibleSeconds, true, orientation);
  const { seek } = useDJAudioEngineActions();

  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    const ctx = canvas.getContext('2d');
    if (!ctx) return;
    const driver = new CanvasWaveformDriver(canvas, ctx);

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
      const usableBands = currentDJThreeBands(bands, d.track, d.duration, state.spotifySessionGeneration);
      const visual = {
        bands: usableBands, position, peaks: d.waveformPeaks, duration: d.duration, grid: d.beatGrid, offset: d.beatGridOffset,
        cue: d.cuePoint, hot: d.hotCues, loop: d.loop, track: d.track?.id, w: canvas.width, h: canvas.height, orientation,
      };
      const unchanged = last && Object.keys(visual).every(key => (visual as Record<string, unknown>)[key] === last![key]);
      if (!unchanged) {
        driver.draw({
          deck,
          sourceKey: JSON.stringify([d.track?.id, djTrackSourceIdentity(d.track), state.spotifySessionGeneration]),
          threeBands: usableBands,
          peaks: d.waveformPeaks,
          position,
          duration: d.duration,
          beatGrid: d.beatGrid,
          beatGridOffset: d.beatGridOffset,
          cuePoint: d.cuePoint,
          hotCues: d.hotCues,
          loop: d.loop,
          visibleSeconds,
          colorMode,
          hasTrack: !!d.track,
          orientation,
          bandSelection,
        }, width, height, ratio);
        last = visual;
      }
      if (moving && !document.hidden) frameId = requestAnimationFrame(draw);
      else idleTimer = setTimeout(() => { frameId = requestAnimationFrame(draw); }, 250);
    };

    frameId = requestAnimationFrame(draw);
    return () => {
      cancelled = true;
      cancelAnimationFrame(frameId);
      if (idleTimer) clearTimeout(idleTimer);
      driver.dispose();
    };
  }, [deck, visibleSeconds, colorMode, localBands, bands, orientation, bandSelection]);

  const handleDoubleClick = useCallback((event: React.MouseEvent<HTMLCanvasElement>) => {
    const rect = event.currentTarget.getBoundingClientRect();
    const d = deck === 'A' ? useStore.getState().djDeckA : useStore.getState().djDeckB;
    if (!d.duration) return;
    if (orientation === 'vertical') {
      const y = (event.clientY - rect.top) * (event.currentTarget.clientHeight / rect.height);
      seek(deck, timeAtLaneX(y, event.currentTarget.clientHeight, getDJAudioEngine().getPosition(deck), visibleSeconds, d.duration));
      return;
    }
    // Rect is post-transform; convert click x back into canvas CSS px.
    const x = (event.clientX - rect.left) * (event.currentTarget.clientWidth / rect.width);
    seek(deck, timeAtLaneX(x, event.currentTarget.clientWidth, getDJAudioEngine().getPosition(deck), visibleSeconds, d.duration));
  }, [deck, seek, visibleSeconds, orientation]);

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
