/**
 * ViiB MediaHub - DJ WebGL Waveform Component
 * 
 * High-performance WebGL2-based dual waveform display.
 * Replaces Canvas 2D rendering with GPU-accelerated shaders
 * for 60+ FPS performance.
 * 
 * Features:
 * - Multi-colored frequency waveforms (bass=red, mid=green, high=blue)
 * - Overview waveform strips with playhead markers
 * - Beat grid visualization
 * - Hot cue markers
 * - Click-to-seek support
 * - Automatic Canvas 2D fallback if WebGL unavailable
 * 
 * @module DJWebGLWaveform
 */

import React, { useRef, useEffect, useCallback, useState, useMemo } from 'react';
import { useStore } from '../../../../store';
import { useDJAudioEngine } from '../../../../hooks/useDJAudioEngine';
import { useDJWebGL, useDJWebGLAnimation } from './useDJWebGL';
import type { DeckId } from '../../../../slices/djMixerSlice';
import { DJWaveformRenderState } from './DJWebGLRenderer';

interface DJWebGLWaveformProps {
  /** Total height of the component */
  height?: number;
  /** Visible time window in seconds */
  visibleSeconds?: number;
  /** Fallback to Canvas 2D if WebGL fails */
  allowFallback?: boolean;
}

const OVERVIEW_HEIGHT = 24;

export const DJWebGLWaveform: React.FC<DJWebGLWaveformProps> = ({
  height = 200,
  visibleSeconds = 10,
  allowFallback = true,
}) => {
  const containerRef = useRef<HTMLDivElement>(null);
  const [useFallback, setUseFallback] = useState(false);
  
  // Get deck states from store
  const deckA = useStore(state => state.djDeckA);
  const deckB = useStore(state => state.djDeckB);
  const { seek } = useDJAudioEngine();
  
  // Calculate heights
  const mainHeight = (height - OVERVIEW_HEIGHT - 8) / 2;
  
  // WebGL hooks for each canvas
  const overviewWebGL = useDJWebGL();
  const deckAWebGL = useDJWebGL();
  const deckBWebGL = useDJWebGL();
  
  // Track initialization
  const [isReady, setIsReady] = useState(false);
  
  // Initialize and check WebGL support
  useEffect(() => {
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
  }, [allowFallback, deckAWebGL]);
  
  // Update waveform textures when peaks change
  useEffect(() => {
    if (!isReady) return;
    
    if (deckA.waveformPeaks) {
      deckAWebGL.updatePeaks('A', deckA.waveformPeaks);
      overviewWebGL.updatePeaks('A', deckA.waveformPeaks);
    }
  }, [deckA.waveformPeaks, isReady, deckAWebGL, overviewWebGL]);
  
  useEffect(() => {
    if (!isReady) return;
    
    if (deckB.waveformPeaks) {
      deckBWebGL.updatePeaks('B', deckB.waveformPeaks);
      overviewWebGL.updatePeaks('B', deckB.waveformPeaks);
    }
  }, [deckB.waveformPeaks, isReady, deckBWebGL, overviewWebGL]);
  
  // Store refs for RAF to avoid stale closures
  const deckARef = useRef(deckA);
  const deckBRef = useRef(deckB);
  
  useEffect(() => {
    deckARef.current = deckA;
    deckBRef.current = deckB;
  }, [deckA, deckB]);
  
  // Animation render callback
  const renderFrame = useCallback(() => {
    if (!isReady) return;
    
    const currentDeckA = deckARef.current;
    const currentDeckB = deckBRef.current;
    
    // Render Deck A waveform
    const stateA: DJWaveformRenderState = {
      peaks: currentDeckA.waveformPeaks,
      position: currentDeckA.position,
      duration: currentDeckA.duration,
      bpm: currentDeckA.effectiveBpm || currentDeckA.originalBpm || 0,
      beatGrid: currentDeckA.beatGrid,
      cuePoint: currentDeckA.cuePoint,
      hotCues: currentDeckA.hotCues,
      visibleSeconds,
      deck: 'A',
    };
    deckAWebGL.renderWaveform(stateA);
    
    // Render Deck B waveform
    const stateB: DJWaveformRenderState = {
      peaks: currentDeckB.waveformPeaks,
      position: currentDeckB.position,
      duration: currentDeckB.duration,
      bpm: currentDeckB.effectiveBpm || currentDeckB.originalBpm || 0,
      beatGrid: currentDeckB.beatGrid,
      cuePoint: currentDeckB.cuePoint,
      hotCues: currentDeckB.hotCues,
      visibleSeconds,
      deck: 'B',
    };
    deckBWebGL.renderWaveform(stateB);
    
    // Render overview
    overviewWebGL.renderOverview(
      {
        peaks: currentDeckA.waveformPeaks,
        position: currentDeckA.position,
        duration: currentDeckA.duration,
      },
      {
        peaks: currentDeckB.waveformPeaks,
        position: currentDeckB.position,
        duration: currentDeckB.duration,
      }
    );
  }, [isReady, visibleSeconds, deckAWebGL, deckBWebGL, overviewWebGL]);
  
  // Run animation loop
  useDJWebGLAnimation(renderFrame, isReady && !useFallback, 60);
  
  // Handle waveform click to seek
  const handleWaveformClick = useCallback((
    e: React.MouseEvent<HTMLCanvasElement>,
    deck: DeckId
  ) => {
    const canvas = e.currentTarget;
    const rect = canvas.getBoundingClientRect();
    const x = e.clientX - rect.left;
    const width = rect.width;
    
    const deckState = deck === 'A' ? deckA : deckB;
    if (!deckState.duration) return;
    
    // Calculate time from click position
    const playheadX = width / 2;
    const secondsPerPixel = visibleSeconds / width;
    const clickTime = deckState.position + ((x - playheadX) * secondsPerPixel);
    const clampedTime = Math.max(0, Math.min(deckState.duration, clickTime));
    
    seek(deck, clampedTime);
  }, [deckA, deckB, visibleSeconds, seek]);
  
  // Render fallback Canvas 2D if WebGL not available
  if (useFallback) {
    // Import the original Canvas 2D component as fallback
    const DJDualWaveform = React.lazy(() => import('../DJDualWaveform'));
    return (
      <React.Suspense fallback={<div style={{ height }} className="bg-surface-0" />}>
        <DJDualWaveform height={height} />
      </React.Suspense>
    );
  }
  
  return (
    <div ref={containerRef} className="w-full bg-surface-0" style={{ height }}>
      {/* Overview waveforms */}
      <canvas
        ref={overviewWebGL.canvasRef}
        className="w-full cursor-pointer"
        style={{ height: OVERVIEW_HEIGHT }}
      />
      
      {/* Separator */}
      <div className="h-1 bg-surface-1" />
      
      {/* Main waveform Deck A */}
      <canvas
        ref={deckAWebGL.canvasRef}
        className="w-full cursor-crosshair"
        style={{ height: mainHeight }}
        onClick={(e) => handleWaveformClick(e, 'A')}
      />
      
      {/* Separator with crossfader indicator */}
      <div className="h-1 bg-surface-1 relative">
        <div className="absolute left-1/2 -translate-x-1/2 -top-0.5 w-4 h-2 bg-neutral-500 rounded-sm" />
      </div>
      
      {/* Main waveform Deck B */}
      <canvas
        ref={deckBWebGL.canvasRef}
        className="w-full cursor-crosshair"
        style={{ height: mainHeight }}
        onClick={(e) => handleWaveformClick(e, 'B')}
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
