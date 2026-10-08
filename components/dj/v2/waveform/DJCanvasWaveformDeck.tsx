import { reportAudioReadFailure } from '../../../../services/audioReadDiagnostics';
import { api } from '../../../../services/api';
import { loadLocalThreeBand, type LocalThreeBand } from '../../../../services/localThreeBand';
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
import type { DeckId } from '../../../../slices/djMixerSlice';
import { drawMainWaveform, timeAtLaneX } from './canvasWaveformRenderer';
import type { WaveformColorMode } from './waveformPalette';

interface DJCanvasWaveformDeckProps {
  localBands?: boolean;
  deck: DeckId;
  visibleSeconds: number;
  colorMode: WaveformColorMode;
}

export const DJCanvasWaveformDeck = React.memo(function DJCanvasWaveformDeck({ deck, visibleSeconds, colorMode, localBands = false }: DJCanvasWaveformDeckProps) {
  const track = useStore(s => (deck === 'A' ? s.djDeckA : s.djDeckB).track);
  const [bands,setBands] = useState<{track: typeof track; overview: LocalThreeBand['overview']; peak: number}>();
  useEffect(()=>{
   let active=true;let readingMetadata=true;setBands(undefined);
   if(localBands && track) void (async()=>{
    const metadata=await api.getTrackAnalysisFeature(track.id);
    if(!active || !metadata.sourceFingerprint)return;
    readingMetadata=false;
    const data=await loadLocalThreeBand(track.id,metadata.sourceFingerprint);
    if(!active || !data || data.sourceFingerprint!==metadata.sourceFingerprint)return;
    let peak=0;for(const band of [data.overview.low,data.overview.mid,data.overview.high])for(const value of band)peak=Math.max(peak,value);
    setBands({track,overview:data.overview,peak});
   })().catch(error=>{if(active) reportAudioReadFailure(readingMetadata ? 'audio_metadata' : 'local_bands',error);});
   return ()=>{active=false;};
  },[localBands,track]);
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
      const usableBands=localBands && bands?.track===d.track && d.duration>0 && Math.abs(bands.overview.frames/bands.overview.sampleRate-d.duration)<=0.1 ? bands : undefined;
      const visual = {
        bands: usableBands, position, peaks: d.waveformPeaks, duration: d.duration, grid: d.beatGrid, offset: d.beatGridOffset,
        cue: d.cuePoint, hot: d.hotCues, loop: d.loop, track: d.track?.id, w: canvas.width, h: canvas.height,
      };
      const unchanged = last && Object.keys(visual).every(key => (visual as Record<string, unknown>)[key] === last![key]);
      if (!unchanged) {
        ctx.setTransform(ratio, 0, 0, ratio, 0, 0);
        drawMainWaveform(ctx, width, height, {
          deck,
          localBands: usableBands,
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
  }, [deck, visibleSeconds, colorMode, localBands, bands]);

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
