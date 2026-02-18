/**
 * useDJWebGL Hook
 * 
 * React hook for managing DJ WebGL renderer lifecycle.
 * Handles initialization, cleanup, resize, and animation frame management.
 * 
 * @module useDJWebGL
 */

import { useRef, useEffect, useCallback } from 'react';
import { DJWebGLRenderer, DJWaveformRenderState, DJWebGLRendererOptions, DeckId } from './DJWebGLRenderer';

export interface UseDJWebGLOptions extends DJWebGLRendererOptions {
  /** Target frame rate (default: 60) */
  targetFPS?: number;
}

export interface UseDJWebGLReturn {
  /** Ref to attach to canvas element */
  canvasRef: React.RefObject<HTMLCanvasElement>;
  /** Whether WebGL is ready */
  isReady: boolean;
  /** Update waveform peak data for a deck */
  updatePeaks: (deck: DeckId, peaks: number[] | Float32Array | null) => void;
  /** Render a waveform frame */
  renderWaveform: (state: DJWaveformRenderState) => void;
  /** Render overview strip */
  renderOverview: (
    deckA: { peaks: number[] | null; position: number; duration: number } | null,
    deckB: { peaks: number[] | null; position: number; duration: number } | null
  ) => void;
  /** Get WebGL info */
  getInfo: () => { webgl2: boolean; maxTextureSize: number } | null;
}

/**
 * Hook for managing DJ WebGL renderer
 */
export function useDJWebGL(options: UseDJWebGLOptions = {}): UseDJWebGLReturn {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const rendererRef = useRef<DJWebGLRenderer | null>(null);
  const isReadyRef = useRef(false);
  const resizeObserverRef = useRef<ResizeObserver | null>(null);

  // Initialize renderer on mount
  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;

    // Create renderer
    const renderer = new DJWebGLRenderer(options);
    
    // Initialize
    const success = renderer.init(canvas);
    if (success) {
      rendererRef.current = renderer;
      isReadyRef.current = true;
      console.log('[useDJWebGL] Renderer initialized');
    } else {
      console.error('[useDJWebGL] Failed to initialize renderer');
    }

    // Set up resize observer
    resizeObserverRef.current = new ResizeObserver((entries) => {
      for (const entry of entries) {
        const { width, height } = entry.contentRect;
        if (width > 0 && height > 0 && rendererRef.current) {
          rendererRef.current.resize(width, height);
        }
      }
    });
    resizeObserverRef.current.observe(canvas);

    // Cleanup
    return () => {
      if (resizeObserverRef.current) {
        resizeObserverRef.current.disconnect();
        resizeObserverRef.current = null;
      }
      if (rendererRef.current) {
        rendererRef.current.dispose();
        rendererRef.current = null;
      }
      isReadyRef.current = false;
      console.log('[useDJWebGL] Cleanup complete');
    };
  }, []); // Empty deps - only run on mount/unmount

  // Update peaks
  const updatePeaks = useCallback((deck: DeckId, peaks: number[] | Float32Array | null) => {
    if (rendererRef.current && peaks) {
      rendererRef.current.updateWaveformData(deck, peaks);
    }
  }, []);

  // Render waveform
  const renderWaveform = useCallback((state: DJWaveformRenderState) => {
    if (rendererRef.current) {
      rendererRef.current.renderWaveform(state);
    }
  }, []);

  // Render overview
  const renderOverview = useCallback((
    deckA: { peaks: number[] | null; position: number; duration: number } | null,
    deckB: { peaks: number[] | null; position: number; duration: number } | null
  ) => {
    if (rendererRef.current) {
      rendererRef.current.renderOverview(deckA, deckB);
    }
  }, []);

  // Get info
  const getInfo = useCallback(() => {
    if (rendererRef.current) {
      return rendererRef.current.getInfo();
    }
    return null;
  }, []);

  return {
    canvasRef,
    isReady: isReadyRef.current,
    updatePeaks,
    renderWaveform,
    renderOverview,
    getInfo,
  };
}

/**
 * Hook for animation frame-based rendering
 */
export function useDJWebGLAnimation(
  renderCallback: (timestamp: number) => void,
  isEnabled: boolean = true,
  targetFPS: number = 60
): void {
  const frameIdRef = useRef<number>(0);
  const lastFrameTimeRef = useRef<number>(0);
  const frameInterval = 1000 / targetFPS;

  useEffect(() => {
    if (!isEnabled) return;

    const animate = (timestamp: number) => {
      // Throttle to target FPS
      const elapsed = timestamp - lastFrameTimeRef.current;
      
      if (elapsed >= frameInterval) {
        lastFrameTimeRef.current = timestamp - (elapsed % frameInterval);
        renderCallback(timestamp);
      }
      
      frameIdRef.current = requestAnimationFrame(animate);
    };

    frameIdRef.current = requestAnimationFrame(animate);

    return () => {
      if (frameIdRef.current) {
        cancelAnimationFrame(frameIdRef.current);
      }
    };
  }, [renderCallback, isEnabled, frameInterval]);
}

export default useDJWebGL;
