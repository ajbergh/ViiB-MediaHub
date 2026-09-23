/**
 * ViiB MediaHub - DJ WebGL Waveform Component
 * 
 * High-performance WebGL2-based dual waveform display.
 * Replaces Canvas 2D rendering with GPU-accelerated shaders
 * for 60+ FPS performance.
 * 
 * Features:
 * - Gradient, amplitude-level and solid deck color palettes
 * - Overview waveform strips with playhead markers
 * - Beat grid visualization
 * - Hot cue markers
 * - Click-to-seek support
 * - Automatic Canvas 2D fallback if WebGL unavailable
 * 
 * @module DJWebGLWaveform
 */

import React, { useRef, useEffect, useCallback, useState } from 'react';
import { useWaveformScratch } from '../../../../hooks/useWaveformScratch';
import { useStore } from '../../../../store';
import { useDJAudioEngineActions } from '../../../../hooks/useDJAudioEngine';
import { getDJAudioEngine } from '../../../../lib/djAudio';
import { useDJWebGL, useDJWebGLAnimation } from './useDJWebGL';
import type { DeckId } from '../../../../slices/djMixerSlice';
import { DJWaveformRenderState } from './DJWebGLRenderer';
import { shouldUseAdvancedWebGL } from '../../../../lib/webglSafety';

interface DJWebGLWaveformProps {
  /** Total height; negative fills the CSS-sized waveform surface. */
  height?: number;
  /** Visible time window in seconds */
  visibleSeconds?: number;
  /** Fallback to Canvas 2D if WebGL fails */
  allowFallback?: boolean;
}

const OVERVIEW_HEIGHT = 24;
const CanvasWaveform = React.lazy(() => import('../DJDualWaveform'));

export const DJWebGLWaveform: React.FC<DJWebGLWaveformProps> = ({
  height = 200,
  visibleSeconds: initialVisibleSeconds = 10,
  allowFallback = true,
}) => {
  const containerRef = useRef<HTMLDivElement>(null);
  const advancedWebGLEnabled = shouldUseAdvancedWebGL();
  const [useFallback, setUseFallback] = useState(() => !advancedWebGLEnabled);
  const [visibleSeconds, setVisibleSeconds] = useState(() => Math.max(2, Math.min(60, initialVisibleSeconds)));
  const [colorMode, setColorMode] = useState<0 | 1 | 2>(0);
  
  // Get only what we need from store — granular selectors to avoid re-renders
  const deckAWaveformPeaks = useStore(state => state.djDeckA.waveformPeaks);
  const deckBWaveformPeaks = useStore(state => state.djDeckB.waveformPeaks);
  const { seek } = useDJAudioEngineActions();
  
  const scratchA = useWaveformScratch('A', visibleSeconds, !useFallback);
  const scratchB = useWaveformScratch('B', visibleSeconds, !useFallback);
  // Calculate heights
  const TOOLBAR_HEIGHT = 32;
  const surfaceHeight = height < 0 ? '100%' : height;
  const mainHeight = height < 0 ? `calc((100% - ${TOOLBAR_HEIGHT + OVERVIEW_HEIGHT + 8}px) / 2)` : Math.floor((height - TOOLBAR_HEIGHT - OVERVIEW_HEIGHT - 8) / 2);
  
  // WebGL hooks for each canvas
  const overviewWebGL = useDJWebGL({ enabled: advancedWebGLEnabled });
  const deckAWebGL = useDJWebGL({ enabled: advancedWebGLEnabled });
  const deckBWebGL = useDJWebGL({ enabled: advancedWebGLEnabled });
  
  // Track initialization
  const [isReady, setIsReady] = useState(false);
  
  // Initialize and check WebGL support
  useEffect(() => {
    if (!advancedWebGLEnabled) return;

    // Give canvases time to mount
    const timer = setTimeout(() => {
      const info = deckAWebGL.getInfo();
      if (!info && allowFallback) {
        console.warn('[DJWebGLWaveform] WebGL not available, using fallback');
        setUseFallback(true);
      } else {
        setIsReady(true);
        console.log('[DJWebGLWaveform] WebGL ready:', info);
      }
    }, 100);
    
    return () => clearTimeout(timer);
  }, [advancedWebGLEnabled, allowFallback, deckAWebGL.getInfo]);
  
  // Depend on stable hook methods, not the fresh wrapper object: playback/UI
  // renders must not re-upload textures or restart the readiness timer.
  // Update waveform textures when peaks change
  useEffect(() => {
    if (!isReady) return;
    
    if (deckAWaveformPeaks) {
      deckAWebGL.updatePeaks('A', deckAWaveformPeaks);
      overviewWebGL.updatePeaks('A', deckAWaveformPeaks);
    }
  }, [deckAWaveformPeaks, isReady, deckAWebGL.updatePeaks, overviewWebGL.updatePeaks]);
  
  useEffect(() => {
    if (!isReady) return;
    
    if (deckBWaveformPeaks) {
      deckBWebGL.updatePeaks('B', deckBWaveformPeaks);
      overviewWebGL.updatePeaks('B', deckBWaveformPeaks);
    }
  }, [deckBWaveformPeaks, isReady, deckBWebGL.updatePeaks, overviewWebGL.updatePeaks]);
  
  // Track last idle state to skip redundant renders
  const lastIdleFrameRef = useRef({
    aPos: -1,
    bPos: -1,
    aTrackId: null as string | null,
    bTrackId: null as string | null,
    aCue: null as number | null,
    bCue: null as number | null,
    aHotCues: null as any,
    bHotCues: null as any,
    aBeatGrid: null as any,
    bBeatGrid: null as any,
    aPeaks: null as any,
    bPeaks: null as any,
    aDuration: 0,
    bDuration: 0,
    aGridOffset: 0,
    bGridOffset: 0,
    aLoop: null as any,
    bLoop: null as any,
    visibleSeconds: 0,
    colorMode: -1,
  });
  
  // Animation render callback — reads store directly via getState() to avoid stale closures
  const renderFrame = useCallback(() => {
    if (!isReady) return;
    
    const state = useStore.getState();
    const currentDeckA = state.djDeckA;
    const currentDeckB = state.djDeckB;
    const aTrackId = currentDeckA.track?.id ?? null;
    const bTrackId = currentDeckB.track?.id ?? null;
    const aPlaying = currentDeckA.isPlaying;
    const bPlaying = currentDeckB.isPlaying;


    // Read position from engine when playing for smooth 60fps,
    // fall back to store position when paused (store is throttled ~15fps)
    const engine = getDJAudioEngine();
    const idle = !aPlaying && !bPlaying && !engine.isScratching('A') && !engine.isScratching('B');
    const posA = (aPlaying || engine.isScratching('A')) && engine?.initialized
      ? engine.getPosition('A') : currentDeckA.position;
    const posB = (bPlaying || engine.isScratching('B')) && engine?.initialized
      ? engine.getPosition('B') : currentDeckB.position;

    // Skip render when idle and nothing visual has changed
    if (
      idle &&
      posA === lastIdleFrameRef.current.aPos &&
      posB === lastIdleFrameRef.current.bPos &&
      aTrackId === lastIdleFrameRef.current.aTrackId &&
      bTrackId === lastIdleFrameRef.current.bTrackId &&
      currentDeckA.cuePoint === lastIdleFrameRef.current.aCue &&
      currentDeckB.cuePoint === lastIdleFrameRef.current.bCue &&
      currentDeckA.hotCues === lastIdleFrameRef.current.aHotCues &&
      currentDeckB.hotCues === lastIdleFrameRef.current.bHotCues &&
      currentDeckA.beatGrid === lastIdleFrameRef.current.aBeatGrid &&
      currentDeckB.beatGrid === lastIdleFrameRef.current.bBeatGrid &&
      currentDeckA.waveformPeaks === lastIdleFrameRef.current.aPeaks &&
      currentDeckB.waveformPeaks === lastIdleFrameRef.current.bPeaks &&
      currentDeckA.duration === lastIdleFrameRef.current.aDuration &&
      currentDeckB.duration === lastIdleFrameRef.current.bDuration &&
      currentDeckA.beatGridOffset === lastIdleFrameRef.current.aGridOffset &&
      currentDeckB.beatGridOffset === lastIdleFrameRef.current.bGridOffset &&
      currentDeckA.loop === lastIdleFrameRef.current.aLoop &&
      currentDeckB.loop === lastIdleFrameRef.current.bLoop &&
      visibleSeconds === lastIdleFrameRef.current.visibleSeconds &&
      colorMode === lastIdleFrameRef.current.colorMode
    ) {
      return;
    }

    lastIdleFrameRef.current = {
      aPos: posA,
      bPos: posB,
      aTrackId,
      bTrackId,
      aCue: currentDeckA.cuePoint,
      bCue: currentDeckB.cuePoint,
      aHotCues: currentDeckA.hotCues,
      bHotCues: currentDeckB.hotCues,
      aBeatGrid: currentDeckA.beatGrid,
      bBeatGrid: currentDeckB.beatGrid,
      aPeaks: currentDeckA.waveformPeaks,
      bPeaks: currentDeckB.waveformPeaks,
      aDuration: currentDeckA.duration,
      bDuration: currentDeckB.duration,
      aGridOffset: currentDeckA.beatGridOffset,
      bGridOffset: currentDeckB.beatGridOffset,
      aLoop: currentDeckA.loop,
      bLoop: currentDeckB.loop,
      visibleSeconds,
      colorMode,
    };
    
    // Render Deck A waveform
    const stateA: DJWaveformRenderState = {
      peaks: currentDeckA.waveformPeaks,
      position: posA,
      duration: currentDeckA.duration,
      bpm: currentDeckA.effectiveBpm || currentDeckA.originalBpm || 0,
      beatGrid: currentDeckA.beatGrid,
      beatGridOffset: currentDeckA.beatGridOffset,
      cuePoint: currentDeckA.cuePoint,
      hotCues: currentDeckA.hotCues,
      loop: currentDeckA.loop,
      visibleSeconds,
      colorMode,
      deck: 'A',
    };
    deckAWebGL.renderWaveform(stateA);
    
    // Render Deck B waveform
    const stateB: DJWaveformRenderState = {
      peaks: currentDeckB.waveformPeaks,
      position: posB,
      duration: currentDeckB.duration,
      bpm: currentDeckB.effectiveBpm || currentDeckB.originalBpm || 0,
      beatGrid: currentDeckB.beatGrid,
      beatGridOffset: currentDeckB.beatGridOffset,
      cuePoint: currentDeckB.cuePoint,
      hotCues: currentDeckB.hotCues,
      loop: currentDeckB.loop,
      visibleSeconds,
      colorMode,
      deck: 'B',
    };
    deckBWebGL.renderWaveform(stateB);
    
    // Render overview
    overviewWebGL.renderOverview(
      {
        peaks: currentDeckA.waveformPeaks,
        position: posA,
        duration: currentDeckA.duration,
        loop: currentDeckA.loop,
      },
      {
        peaks: currentDeckB.waveformPeaks,
        position: posB,
        duration: currentDeckB.duration,
        loop: currentDeckB.loop,
      }
    );
  }, [isReady, visibleSeconds, colorMode, deckAWebGL.renderWaveform, deckBWebGL.renderWaveform, overviewWebGL.renderOverview]);
  
  // Run animation loop — throttle to 4fps when both decks idle
  const isTimelineIdle = useCallback(() => {
    const state = useStore.getState();
    const engine = getDJAudioEngine();
    return !state.djDeckA.isPlaying && !state.djDeckB.isPlaying &&
      !engine.isScratching('A') && !engine.isScratching('B');
  }, []);
  useDJWebGLAnimation(
    renderFrame,
    isReady && !useFallback,
    60,
    4,
    isTimelineIdle
  );
  
  // Handle waveform click to seek
  const handleWaveformClick = useCallback((
    e: React.MouseEvent<HTMLCanvasElement>,
    deck: DeckId
  ) => {
    const canvas = e.currentTarget;
    const rect = canvas.getBoundingClientRect();
    const x = e.clientX - rect.left;
    const width = rect.width;
    
    const state = useStore.getState();
    const deckState = deck === 'A' ? state.djDeckA : state.djDeckB;
    if (!deckState.duration) return;
    
    // Calculate time from click position
    const playheadX = width / 2;
    const secondsPerPixel = visibleSeconds / width;
    const clickTime = getDJAudioEngine().getPosition(deck) + ((x - playheadX) * secondsPerPixel);
    const clampedTime = Math.max(0, Math.min(deckState.duration, clickTime));
    
    seek(deck, clampedTime);
  }, [visibleSeconds, seek]);

  const handleOverviewClick = useCallback((e: React.MouseEvent<HTMLCanvasElement>) => {
    const rect = e.currentTarget.getBoundingClientRect();
    const x = e.clientX - rect.left;
    const deck: DeckId = x < rect.width / 2 ? 'A' : 'B';
    const state = useStore.getState();
    const deckState = deck === 'A' ? state.djDeckA : state.djDeckB;
    if (!deckState.duration) return;
    const localX = deck === 'A' ? x : x - rect.width / 2;
    seek(deck, Math.max(0, Math.min(deckState.duration, localX / (rect.width / 2) * deckState.duration)));
  }, [seek]);

  const handleWheelZoom = useCallback((e: React.WheelEvent<HTMLDivElement>) => {
    if (!e.ctrlKey) return;
    e.preventDefault();
    setVisibleSeconds(value => Math.max(2, Math.min(60, value * (e.deltaY < 0 ? 1 / 1.15 : 1.15))));
  }, []);
  
  // Render fallback Canvas 2D if WebGL not available
  if (useFallback) {
    return (
      <React.Suspense fallback={<div style={{ height: surfaceHeight }} className="bg-surface-0" />}>
        <CanvasWaveform height={height} responsive={height < 0} />
      </React.Suspense>
    );
  }
  
  return (
    <div ref={containerRef} className="w-full bg-surface-0 relative" style={{ height: surfaceHeight }} onWheel={handleWheelZoom}>
      <div className="h-8 flex items-center justify-end gap-1 px-2 bg-[#111]">
        {(['Gradient', 'Level', 'Solid'] as const).map((label, index) => (
          <button key={label} onClick={() => setColorMode(index as 0 | 1 | 2)} aria-pressed={colorMode === index}
            className={`px-2 h-6 rounded text-[9px] font-bold ${colorMode === index ? 'bg-brand/30 text-brand' : 'text-neutral-500 hover:text-neutral-200'}`}
            title={`Waveform: ${label}`}>{label}</button>
        ))}
        <span className="mx-1 h-4 w-px bg-neutral-700" />
        <button onClick={() => setVisibleSeconds(value => Math.max(2, value / 1.5))} aria-label="Zoom in" className="w-6 h-6 text-neutral-300">+</button>
        <button onClick={() => setVisibleSeconds(10)} aria-label="Reset zoom" className="px-1 h-6 text-[9px] text-neutral-400">{visibleSeconds.toFixed(0)}s</button>
        <button onClick={() => setVisibleSeconds(value => Math.min(60, value * 1.5))} aria-label="Zoom out" className="w-6 h-6 text-neutral-300">−</button>
      </div>
      {/* Overview waveforms */}
      <canvas
        ref={overviewWebGL.canvasRef}
        className="w-full cursor-pointer"
        style={{ height: OVERVIEW_HEIGHT }}
        onClick={handleOverviewClick}
      />
      
      {/* Separator */}
      <div className="h-1 bg-surface-1" />
      
      {/* Main waveform Deck A */}
      <canvas
        ref={deckAWebGL.canvasRef}
        {...scratchA}
        style={{ height: mainHeight, touchAction: 'none' }}
        onDoubleClick={(e) => handleWaveformClick(e, 'A')}
      />
      
      {/* Separator with crossfader indicator */}
      <div className="h-1 bg-surface-1 relative">
        <div className="absolute left-1/2 -translate-x-1/2 -top-0.5 w-4 h-2 bg-neutral-500 rounded-sm" />
      </div>
      
      {/* Main waveform Deck B */}
      <canvas
        ref={deckBWebGL.canvasRef}
        {...scratchB}
        style={{ height: mainHeight, touchAction: 'none' }}
        onDoubleClick={(e) => handleWaveformClick(e, 'B')}
      />
      
      {/* WebGL indicator (dev only) */}
      {process.env.NODE_ENV === 'development' && isReady && (
        <div className="absolute bottom-1 right-1 text-[8px] text-neutral-600 pointer-events-none">
          WebGL2
        </div>
      )}
    </div>
  );
};

export default DJWebGLWaveform;
