/**
 * One deck's scrolling waveform lane rendered with WebGL.
 *
 * Owns exactly one WebGL context. The split layout renders one of these per
 * deck and draws overviews with Canvas 2D, so the page holds two WebGL
 * contexts instead of the three used by the former stacked layout.
 *
 * Reports `onUnavailable` when the context cannot be created so the lane can
 * fall back to the Canvas renderer.
 *
 * @module components/dj/v2/webgl/DJWebGLWaveformDeck
 */

import React, { useCallback, useEffect, useRef, useState } from 'react';
import { useStore } from '../../../../store';
import { useWaveformScratch } from '../../../../hooks/useWaveformScratch';
import { useDJAudioEngineActions } from '../../../../hooks/useDJAudioEngine';
import { getDJAudioEngine } from '../../../../lib/djAudio';
import { shouldUseAdvancedWebGL } from '../../../../lib/webglSafety';
import type { DeckId } from '../../../../slices/djMixerSlice';
import { useDJWebGL, useDJWebGLAnimation } from './useDJWebGL';
import type { DJWaveformRenderState } from './DJWebGLRenderer';
import { timeAtLaneX } from '../waveform/canvasWaveformRenderer';
import { WEBGL_COLOR_MODE, type WaveformColorMode } from '../waveform/waveformPalette';

interface DJWebGLWaveformDeckProps {
  deck: DeckId;
  visibleSeconds: number;
  colorMode: WaveformColorMode;
  onUnavailable: () => void;
}

export const DJWebGLWaveformDeck = React.memo(function DJWebGLWaveformDeck({ deck, visibleSeconds, colorMode, onUnavailable }: DJWebGLWaveformDeckProps) {
  const advancedWebGLEnabled = shouldUseAdvancedWebGL();
  const webgl = useDJWebGL({ enabled: advancedWebGLEnabled });
  const [isReady, setIsReady] = useState(false);
  const peaks = useStore(state => (deck === 'A' ? state.djDeckA : state.djDeckB).waveformPeaks);
  const scratch = useWaveformScratch(deck, visibleSeconds, isReady);
  const { seek } = useDJAudioEngineActions();
  const lastFrame = useRef<Record<string, unknown> | null>(null);

  // Give the canvas time to mount, then confirm the context exists.
  useEffect(() => {
    if (!advancedWebGLEnabled) { onUnavailable(); return; }
    const timer = setTimeout(() => {
      if (webgl.getInfo()) setIsReady(true);
      else onUnavailable();
    }, 100);
    return () => clearTimeout(timer);
  }, [advancedWebGLEnabled, onUnavailable, webgl.getInfo]);

  // Upload textures only when peaks change, never on playback renders.
  useEffect(() => {
    if (isReady && peaks) webgl.updatePeaks(deck, peaks);
    lastFrame.current = null;
  }, [deck, isReady, peaks, webgl.updatePeaks]);

  const renderFrame = useCallback(() => {
    const d = deck === 'A' ? useStore.getState().djDeckA : useStore.getState().djDeckB;
    const engine = getDJAudioEngine();
    const moving = d.isPlaying || engine.isScratching(deck);
    const position = moving && engine?.initialized ? engine.getPosition(deck) : d.position;
    const visual = {
      position, track: d.track?.id, cue: d.cuePoint, hot: d.hotCues, grid: d.beatGrid, peaks: d.waveformPeaks,
      duration: d.duration, offset: d.beatGridOffset, loop: d.loop, visibleSeconds, colorMode,
      // Resizes clear the drawing buffer, so they must force a redraw.
      w: webgl.canvasRef.current?.width, h: webgl.canvasRef.current?.height,
    };
    const previous = lastFrame.current;
    if (!moving && previous && Object.keys(visual).every(key => (visual as Record<string, unknown>)[key] === previous[key])) return;
    lastFrame.current = visual;
    const state: DJWaveformRenderState = {
      peaks: d.waveformPeaks,
      position,
      duration: d.duration,
      bpm: d.effectiveBpm || d.originalBpm || 0,
      beatGrid: d.beatGrid,
      beatGridOffset: d.beatGridOffset,
      cuePoint: d.cuePoint,
      hotCues: d.hotCues,
      loop: d.loop,
      visibleSeconds,
      colorMode: WEBGL_COLOR_MODE[colorMode],
      deck,
    };
    webgl.renderWaveform(state);
  }, [deck, visibleSeconds, colorMode, webgl.renderWaveform, webgl.canvasRef]);

  const isIdle = useCallback(() => {
    const d = deck === 'A' ? useStore.getState().djDeckA : useStore.getState().djDeckB;
    return !d.isPlaying && !getDJAudioEngine().isScratching(deck);
  }, [deck]);
  useDJWebGLAnimation(renderFrame, isReady, 60, 4, isIdle);

  const handleDoubleClick = useCallback((event: React.MouseEvent<HTMLCanvasElement>) => {
    const rect = event.currentTarget.getBoundingClientRect();
    const d = deck === 'A' ? useStore.getState().djDeckA : useStore.getState().djDeckB;
    if (!d.duration) return;
    const x = (event.clientX - rect.left) * (event.currentTarget.clientWidth / rect.width);
    seek(deck, timeAtLaneX(x, event.currentTarget.clientWidth, getDJAudioEngine().getPosition(deck), visibleSeconds, d.duration));
  }, [deck, seek, visibleSeconds]);

  return (
    <canvas
      ref={webgl.canvasRef}
      {...scratch}
      className={`${scratch.className} dj-waveform-main`}
      style={{ touchAction: 'none' }}
      onDoubleClick={handleDoubleClick}
      data-dj-renderer='webgl'
    />
  );
});
