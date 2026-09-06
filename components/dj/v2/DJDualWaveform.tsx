/**
 * ViiB MediaHub - DJ Dual Waveform Component (v2)
 * 
 * Stacked horizontal waveform display with frequency-based coloring.
 * Shows both decks in a single view with overview strips.
 * 
 * Features:
 * - Multi-colored frequency waveforms (bass=red, mid=green, high=blue)
 * - Overview waveform strips with hot cue markers
 * - Beat grid visualization
 * - Playhead indicators
 * - Click-to-seek support
 * - Smooth 60fps rendering via requestAnimationFrame
 * 
 * @module components/dj/v2/DJDualWaveform
 */

import React, { useRef, useEffect, useCallback, useState } from 'react';
import { useStore } from '../../../store';
import { useDJAudioEngineActions } from '../../../hooks/useDJAudioEngine';
import { getDJAudioEngine } from '../../../lib/djAudio';
import type { DeckId, HotCue } from '../../../slices/djMixerSlice';

interface DJDualWaveformProps {
  height?: number;
  responsive?: boolean;
}

// Waveform color modes (rekordbox-style)
type WaveformColorMode = 'rgb' | '3band' | 'single';

// Frequency band colors (RGB mode)
const FREQ_COLORS = {
  bass: '#ff4444',      // Red
  lowMid: '#ff8844',    // Orange
  mid: '#44ff44',       // Green
  highMid: '#44ffff',   // Cyan
  high: '#4444ff',      // Blue
};

// 3-Band colors
const BAND_COLORS = {
  bass: '#ff4444',      // Red
  mid: '#44ff44',       // Green  
  high: '#4488ff',      // Blue
};

// Single color per deck
const DECK_COLORS = {
  A: '#3b82f6',         // Blue
  B: '#8b5cf6',         // Purple
};

interface OverviewCache {
  canvas: HTMLCanvasElement;
  width: number;
  height: number;
  dpr: number;
  peaksA: number[] | null;
  durationA: number;
  peaksB: number[] | null;
  durationB: number;
}

/** Draw a deck's static peak bars as one canvas fill operation. */
function drawPeakBars(
  ctx: CanvasRenderingContext2D,
  peaks: number[] | null,
  width: number,
  height: number,
  xOffset: number,
  color: string,
) {
  if (!peaks?.length || width <= 0) return;

  const samplesPerPixel = peaks.length / width;
  const centerY = height / 2;
  const maxAmplitude = centerY - 2;
  ctx.fillStyle = color;
  ctx.beginPath();
  for (let x = 0; x < width; x++) {
    const peak = peaks[Math.floor(x * samplesPerPixel)] ?? 0;
    const amplitude = peak * maxAmplitude;
    if (amplitude > 0) ctx.rect(xOffset + x, centerY - amplitude, 1, amplitude * 2);
  }
  ctx.fill();
}

export const DJDualWaveform: React.FC<DJDualWaveformProps> = ({ height = 200, responsive = false }) => {
  const overviewRef = useRef<HTMLCanvasElement>(null);
  const mainCanvasARef = useRef<HTMLCanvasElement>(null);
  const mainCanvasBRef = useRef<HTMLCanvasElement>(null);
  const containerRef = useRef<HTMLDivElement>(null);
  const overviewCacheRef = useRef<OverviewCache | null>(null);
  const [computedHeight, setComputedHeight] = useState(height > 0 ? height : 160);
  
  // Responsive height calculation
  useEffect(() => {
    if (!responsive) {
      setComputedHeight(height > 0 ? height : 160);
      return;
    }
    
    const container = containerRef.current;
    if (!container) return;
    
    const updateHeight = () => {
      const parentHeight = container.parentElement?.clientHeight || 160;
      setComputedHeight(parentHeight);
    };
    
    updateHeight();
    const resizeObserver = new ResizeObserver(updateHeight);
    if (container.parentElement) {
      resizeObserver.observe(container.parentElement);
    }
    
    return () => resizeObserver.disconnect();
  }, [responsive, height]);
  
  // Get deck states via non-reactive getState for RAF drawing
  // Only subscribe reactively to fields that affect layout/controls (not position)
  const { seek } = useDJAudioEngineActions();
  
  const VISIBLE_SECONDS_DEFAULT = 10;
  const VISIBLE_SECONDS_MIN = 2;
  const VISIBLE_SECONDS_MAX = 60;
  const [visibleSeconds, setVisibleSeconds] = useState(VISIBLE_SECONDS_DEFAULT);
  const [colorMode, setColorMode] = useState<WaveformColorMode>('rgb');
  const OVERVIEW_HEIGHT = 24;
  const MAIN_HEIGHT = (computedHeight - OVERVIEW_HEIGHT - 8) / 2; // Split between both decks

  // Zoom handlers
  const zoomIn = useCallback(() => {
    setVisibleSeconds(prev => Math.max(VISIBLE_SECONDS_MIN, prev / 1.5));
  }, []);
  const zoomOut = useCallback(() => {
    setVisibleSeconds(prev => Math.min(VISIBLE_SECONDS_MAX, prev * 1.5));
  }, []);
  const zoomReset = useCallback(() => {
    setVisibleSeconds(VISIBLE_SECONDS_DEFAULT);
  }, []);

  // Mouse wheel zoom on waveform area
  const handleWheelZoom = useCallback((e: React.WheelEvent) => {
    if (e.ctrlKey || e.metaKey) {
      e.preventDefault();
      if (e.deltaY < 0) zoomIn();
      else zoomOut();
    }
  }, [zoomIn, zoomOut]);

  // Draw overview waveform (mini strip showing entire track)
  const drawOverview = useCallback((
    canvas: HTMLCanvasElement | null,
    peaks: number[] | null,
    position: number,
    duration: number,
    hotCues: HotCue[],
    deck: DeckId
  ) => {
    if (!canvas) return;
    const ctx = canvas.getContext('2d');
    if (!ctx) return;

    // Use CSS dimensions (canvas stores device pixels = css * dpr)
    const dpr = window.devicePixelRatio || 1;
    const width = canvas.width / dpr;
    const h = canvas.height / dpr;
    const centerY = h / 2;

    // Clear
    ctx.fillStyle = '#1a1a1a';
    ctx.fillRect(0, 0, width, h);

    // Draw waveform peaks
    if (peaks && peaks.length > 0 && duration > 0) {
      const samplesPerPixel = peaks.length / width;
      ctx.fillStyle = deck === 'A' ? '#3b82f6' : '#8b5cf6';
      
      for (let x = 0; x < width; x++) {
        const sampleIndex = Math.floor(x * samplesPerPixel);
        const peak = peaks[sampleIndex] || 0;
        const amplitude = peak * (h / 2 - 2);
        ctx.fillRect(x, centerY - amplitude, 1, amplitude * 2);
      }
    }

    // Draw hot cue markers
    if (duration > 0) {
      hotCues.forEach(hc => {
        const x = (hc.position / duration) * width;
        ctx.fillStyle = hc.color || '#22c55e';
        // Triangle marker
        ctx.beginPath();
        ctx.moveTo(x - 4, 0);
        ctx.lineTo(x + 4, 0);
        ctx.lineTo(x, 8);
        ctx.closePath();
        ctx.fill();
      });
    }

    // Draw playhead position
    if (duration > 0) {
      const playheadX = (position / duration) * width;
      ctx.strokeStyle = '#ffffff';
      ctx.lineWidth = 2;
      ctx.beginPath();
      ctx.moveTo(playheadX, 0);
      ctx.lineTo(playheadX, h);
      ctx.stroke();
    }

    // Border
    ctx.strokeStyle = '#333';
    ctx.lineWidth = 1;
    ctx.strokeRect(0, 0, width, h);
  }, []);

  // Draw main scrolling waveform with frequency colors
  const drawMainWaveform = useCallback((
    canvas: HTMLCanvasElement | null,
    peaks: number[] | null,
    position: number,
    duration: number,
    beatGrid: number[] | null,
    cuePoint: number,
    deck: DeckId,
    beatGridOffset: number = 0
  ) => {
    if (!canvas) return;
    const ctx = canvas.getContext('2d');
    if (!ctx) return;

    // Use CSS dimensions (canvas stores device pixels = css * dpr)
    const dpr = window.devicePixelRatio || 1;
    const width = canvas.width / dpr;
    const h = canvas.height / dpr;
    const centerY = h / 2;

    // Clear
    ctx.fillStyle = '#121212';
    ctx.fillRect(0, 0, width, h);

    if (!peaks || peaks.length === 0 || !duration || duration <= 0) {
      // Enhanced placeholder with visual styling
      ctx.fillStyle = '#444';
      ctx.textAlign = 'center';
      ctx.font = '11px system-ui';
      ctx.fillText('No waveform data', width / 2, h / 2);
      
      // Draw subtle grid lines for visual interest
      ctx.strokeStyle = '#222';
      ctx.lineWidth = 1;
      for (let x = 0; x < width; x += 50) {
        ctx.beginPath();
        ctx.moveTo(x, 0);
        ctx.lineTo(x, h);
        ctx.stroke();
      }
      
      // Center playhead line still visible
      ctx.strokeStyle = '#ff333380';
      ctx.lineWidth = 2;
      ctx.beginPath();
      ctx.moveTo(width / 2, 0);
      ctx.lineTo(width / 2, h);
      ctx.stroke();
      
      // Deck label
      ctx.fillStyle = deck === 'A' ? '#3b82f640' : '#8b5cf640';
      ctx.textAlign = 'left';
      ctx.font = 'bold 10px system-ui';
      ctx.fillText(`DECK ${deck}`, 8, 14);
      return;
    }

    // Calculate visible range
    const playheadX = width / 2;
    const secondsPerPixel = visibleSeconds / width;
    const visibleStartTime = Math.max(0, position - (playheadX * secondsPerPixel));
    const visibleEndTime = Math.min(duration, position + ((width - playheadX) * secondsPerPixel));

    // Map waveform data to visible range
    const peaksPerSecond = peaks.length / duration;

    // Draw frequency-colored waveform
    // For now, simulate frequency bands with amplitude zones
    const maxAmplitude = h / 2 - 4;

    // Pre-compute RGB gradient once per frame (reused for all bars)
    let rgbGradient: CanvasGradient | null = null;
    if (colorMode === 'rgb') {
      rgbGradient = ctx.createLinearGradient(0, centerY - maxAmplitude, 0, centerY + maxAmplitude);
      rgbGradient.addColorStop(0, FREQ_COLORS.bass);
      rgbGradient.addColorStop(0.25, FREQ_COLORS.lowMid);
      rgbGradient.addColorStop(0.5, FREQ_COLORS.mid);
      rgbGradient.addColorStop(0.75, FREQ_COLORS.highMid);
      rgbGradient.addColorStop(1, FREQ_COLORS.bass);
    }

    if (colorMode === '3band') {
      // 3-Band varies its paint by amplitude, so it retains separate fills.
      for (let x = 0; x < width; x++) {
        const pixelTime = visibleStartTime + (x * secondsPerPixel);
        if (pixelTime < 0 || pixelTime > duration) continue;

        const peak = peaks[Math.floor(pixelTime * peaksPerSecond)] || 0;
        const amplitude = peak * maxAmplitude;
        if (amplitude <= 0) continue;

        ctx.fillStyle = peak > 0.6 ? BAND_COLORS.bass : peak > 0.3 ? BAND_COLORS.mid : BAND_COLORS.high;
        ctx.fillRect(x, centerY - amplitude, 1, amplitude * 2);
      }
    } else {
      // RGB and single-deck modes have one paint. Batch their bars so the
      // Canvas fallback submits a single fill per deck per frame.
      ctx.fillStyle = colorMode === 'single' ? DECK_COLORS[deck] : rgbGradient!;
      ctx.beginPath();
      for (let x = 0; x < width; x++) {
        const pixelTime = visibleStartTime + (x * secondsPerPixel);
        if (pixelTime < 0 || pixelTime > duration) continue;

        const peak = peaks[Math.floor(pixelTime * peaksPerSecond)] || 0;
        const amplitude = peak * maxAmplitude;
        if (amplitude > 0) ctx.rect(x, centerY - amplitude, 1, amplitude * 2);
      }
      ctx.fill();
    }

    // Draw beat grid markers (with offset applied)
    if (beatGrid && beatGrid.length > 0) {
      let beatCount = 0;
      for (const rawBeatTime of beatGrid) {
        const beatTime = rawBeatTime + beatGridOffset;
        if (beatTime >= visibleStartTime && beatTime <= visibleEndTime) {
          const beatX = playheadX + ((beatTime - position) / secondsPerPixel);
          
          // Downbeat (every 4) more prominent
          if (beatCount % 4 === 0) {
            ctx.strokeStyle = 'rgba(255, 255, 255, 0.5)';
            ctx.lineWidth = 2;
          } else {
            ctx.strokeStyle = 'rgba(255, 255, 255, 0.2)';
            ctx.lineWidth = 1;
          }
          
          ctx.beginPath();
          ctx.moveTo(beatX, 0);
          ctx.lineTo(beatX, h);
          ctx.stroke();
        }
        beatCount++;
      }
    }

    // Draw cue point marker
    if (cuePoint > 0 && cuePoint >= visibleStartTime && cuePoint <= visibleEndTime) {
      const cueX = playheadX + ((cuePoint - position) / secondsPerPixel);
      ctx.strokeStyle = '#f59e0b';
      ctx.lineWidth = 2;
      ctx.beginPath();
      ctx.moveTo(cueX, 0);
      ctx.lineTo(cueX, h);
      ctx.stroke();
      
      // Cue marker triangle
      ctx.fillStyle = '#f59e0b';
      ctx.beginPath();
      ctx.moveTo(cueX - 6, 0);
      ctx.lineTo(cueX + 6, 0);
      ctx.lineTo(cueX, 10);
      ctx.closePath();
      ctx.fill();
    }

    // Draw playhead glow
    ctx.strokeStyle = 'rgba(255, 51, 51, 0.25)';
    ctx.lineWidth = 6;
    ctx.beginPath();
    ctx.moveTo(playheadX, 0);
    ctx.lineTo(playheadX, h);
    ctx.stroke();

    // Draw playhead
    ctx.strokeStyle = '#ff3333';
    ctx.lineWidth = 2;
    ctx.beginPath();
    ctx.moveTo(playheadX, 0);
    ctx.lineTo(playheadX, h);
    ctx.stroke();

    // Playhead triangle at top
    ctx.fillStyle = '#ff3333';
    ctx.beginPath();
    ctx.moveTo(playheadX - 6, 0);
    ctx.lineTo(playheadX + 6, 0);
    ctx.lineTo(playheadX, 8);
    ctx.closePath();
    ctx.fill();

    // Deck label
    ctx.fillStyle = '#ffffff40';
    ctx.textAlign = 'left';
    ctx.font = 'bold 10px system-ui';
    ctx.fillText(`DECK ${deck}`, 8, 14);
  }, [visibleSeconds, colorMode]);

  // Handle resize
  useEffect(() => {
    const resizeCanvases = () => {
      const container = containerRef.current;
      if (!container) return;

      const width = container.clientWidth;
      const dpr = window.devicePixelRatio || 1;

      // Set canvas sizes
      [overviewRef.current, mainCanvasARef.current, mainCanvasBRef.current].forEach((canvas, index) => {
        if (!canvas) return;
        const h = index === 0 ? OVERVIEW_HEIGHT : MAIN_HEIGHT;
        canvas.width = width * dpr;
        canvas.height = h * dpr;
        canvas.style.width = `${width}px`;
        canvas.style.height = `${h}px`;
        canvas.getContext('2d')?.scale(dpr, dpr);
      });
    };

    resizeCanvases();
    window.addEventListener('resize', resizeCanvases);
    return () => window.removeEventListener('resize', resizeCanvases);
  }, [MAIN_HEIGHT]);

  // Deck state read from getState() inside RAF loop - no refs needed

  // Smooth RAF-based waveform rendering
  useEffect(() => {
    const container = containerRef.current;
    if (!container) return;
    
    let animationId: number;
    let idleTimeoutId: ReturnType<typeof setTimeout>;
    let lastFrameTime = 0;
    const targetFps = 60;
    const frameInterval = 1000 / targetFps;
    let lastPosA = -1;
    let lastPosB = -1;
    let needsInitialDraw = true;

    const scheduleNext = (idle: boolean, ts?: number) => {
      if (idle) {
        // Throttle to ~4fps when idle — allows GPU to enter low-power mode
        idleTimeoutId = setTimeout(() => { animationId = requestAnimationFrame(drawFrame); }, 250);
      } else {
        animationId = requestAnimationFrame(drawFrame);
      }
    };

    const drawFrame = (timestamp: number) => {
      // Throttle to target FPS
      const elapsed = timestamp - lastFrameTime;
      if (elapsed < frameInterval) {
        animationId = requestAnimationFrame(drawFrame);
        return;
      }
      lastFrameTime = timestamp - (elapsed % frameInterval);

      const width = container.clientWidth;
      // Guard against zero width (can happen during mount/unmount)
      if (width <= 0) {
        scheduleNext(true);
        return;
      }
      
      const currentDeckA = useStore.getState().djDeckA;
      const currentDeckB = useStore.getState().djDeckB;
      
      // Read position from engine when playing for smooth 60fps,
      // fall back to store position when paused (store is throttled ~15fps)
      const engine = getDJAudioEngine();
      const aPlaying = currentDeckA.isPlaying;
      const bPlaying = currentDeckB.isPlaying;

      // Skip redraw if neither deck is playing AND positions haven't changed
      // (allows one initial draw for placeholder text, then idles)
      const posA = aPlaying && engine?.initialized
        ? engine.getPosition('A') : currentDeckA.position;
      const posB = bPlaying && engine?.initialized
        ? engine.getPosition('B') : currentDeckB.position;
      const bothIdle = !aPlaying && !bPlaying;
      if (bothIdle && posA === lastPosA && posB === lastPosB && !needsInitialDraw) {
        scheduleNext(true);
        return;
      }
      lastPosA = posA;
      lastPosB = posB;
      needsInitialDraw = false;

      // Draw overview (combined view)
      const overviewCanvas = overviewRef.current;
      if (overviewCanvas) {
        const ctx = overviewCanvas.getContext('2d');
        if (ctx) {
          ctx.setTransform(1, 0, 0, 1, 0, 0);
          const dpr = window.devicePixelRatio || 1;
          ctx.scale(dpr, dpr);
          
          const halfWidth = Math.max(1, width / 2); // Ensure at least 1px to avoid division by zero
          let cache = overviewCacheRef.current;
          const cacheNeedsRedraw = !cache
            || cache.width !== overviewCanvas.width
            || cache.height !== overviewCanvas.height
            || cache.dpr !== dpr
            || cache.peaksA !== currentDeckA.waveformPeaks
            || cache.durationA !== currentDeckA.duration
            || cache.peaksB !== currentDeckB.waveformPeaks
            || cache.durationB !== currentDeckB.duration;

          if (cacheNeedsRedraw) {
            const cacheCanvas = cache?.canvas ?? document.createElement('canvas');
            cacheCanvas.width = overviewCanvas.width;
            cacheCanvas.height = overviewCanvas.height;
            const cacheCtx = cacheCanvas.getContext('2d');
            if (cacheCtx) {
              cacheCtx.setTransform(dpr, 0, 0, dpr, 0, 0);
              cacheCtx.fillStyle = '#1a1a1a';
              cacheCtx.fillRect(0, 0, width, OVERVIEW_HEIGHT);
              drawPeakBars(cacheCtx, currentDeckA.waveformPeaks, halfWidth, OVERVIEW_HEIGHT, 0, '#3b82f680');
              drawPeakBars(cacheCtx, currentDeckB.waveformPeaks, halfWidth, OVERVIEW_HEIGHT, halfWidth, '#8b5cf680');
              cache = {
                canvas: cacheCanvas,
                width: cacheCanvas.width,
                height: cacheCanvas.height,
                dpr,
                peaksA: currentDeckA.waveformPeaks,
                durationA: currentDeckA.duration,
                peaksB: currentDeckB.waveformPeaks,
                durationB: currentDeckB.duration,
              };
              overviewCacheRef.current = cache;
            }
          }

          if (cache) ctx.drawImage(cache.canvas, 0, 0, width, OVERVIEW_HEIGHT);
          
          // Deck A (left half)
          if (currentDeckA.waveformPeaks && currentDeckA.waveformPeaks.length > 0 && currentDeckA.duration > 0) {
            // Playhead - guard against NaN
            const playheadX = (posA / currentDeckA.duration) * halfWidth;
            if (!isNaN(playheadX) && isFinite(playheadX)) {
              ctx.strokeStyle = '#3b82f6';
              ctx.lineWidth = 2;
              ctx.beginPath();
              ctx.moveTo(playheadX, 0);
              ctx.lineTo(playheadX, OVERVIEW_HEIGHT);
              ctx.stroke();
            }
            
            // Hot cue markers
            currentDeckA.hotCues.forEach(hc => {
              const x = (hc.position / currentDeckA.duration) * halfWidth;
              if (!isNaN(x) && isFinite(x)) {
                ctx.fillStyle = hc.color || '#22c55e';
                ctx.beginPath();
                ctx.moveTo(x - 3, 0);
                ctx.lineTo(x + 3, 0);
                ctx.lineTo(x, 6);
                ctx.closePath();
                ctx.fill();
              }
            });
          }
          
          // Deck B (right half)
          if (currentDeckB.waveformPeaks && currentDeckB.waveformPeaks.length > 0 && currentDeckB.duration > 0) {
            // Playhead - guard against NaN
            const playheadX = halfWidth + (posB / currentDeckB.duration) * halfWidth;
            if (!isNaN(playheadX) && isFinite(playheadX)) {
              ctx.strokeStyle = '#8b5cf6';
              ctx.lineWidth = 2;
              ctx.beginPath();
              ctx.moveTo(playheadX, 0);
              ctx.lineTo(playheadX, OVERVIEW_HEIGHT);
              ctx.stroke();
            }
            
            // Hot cue markers
            currentDeckB.hotCues.forEach(hc => {
              const x = halfWidth + (hc.position / currentDeckB.duration) * halfWidth;
              if (!isNaN(x) && isFinite(x)) {
                ctx.fillStyle = hc.color || '#22c55e';
                ctx.beginPath();
                ctx.moveTo(x - 3, 0);
                ctx.lineTo(x + 3, 0);
                ctx.lineTo(x, 6);
                ctx.closePath();
                ctx.fill();
              }
            });
          }
          
          // Center divider
          ctx.strokeStyle = '#333';
          ctx.lineWidth = 2;
          ctx.beginPath();
          ctx.moveTo(halfWidth, 0);
          ctx.lineTo(halfWidth, OVERVIEW_HEIGHT);
          ctx.stroke();
          
          // Border
          ctx.strokeStyle = '#333';
          ctx.lineWidth = 1;
          ctx.strokeRect(0, 0, width, OVERVIEW_HEIGHT);

          // Overview label
          ctx.fillStyle = 'rgba(255, 255, 255, 0.2)';
          ctx.font = 'bold 9px system-ui';
          ctx.textAlign = 'left';
          ctx.fillText('OVERVIEW', 4, OVERVIEW_HEIGHT - 4);
        }
      }

      // Draw main waveforms
      drawMainWaveform(
        mainCanvasARef.current,
        currentDeckA.waveformPeaks,
        posA,
        currentDeckA.duration,
        currentDeckA.beatGrid,
        currentDeckA.cuePoint,
        'A',
        currentDeckA.beatGridOffset
      );

      drawMainWaveform(
        mainCanvasBRef.current,
        currentDeckB.waveformPeaks,
        posB,
        currentDeckB.duration,
        currentDeckB.beatGrid,
        currentDeckB.cuePoint,
        'B',
        currentDeckB.beatGridOffset
      );

      animationId = requestAnimationFrame(drawFrame);
    };

    animationId = requestAnimationFrame(drawFrame);
    return () => { cancelAnimationFrame(animationId); clearTimeout(idleTimeoutId); };
  }, [drawMainWaveform, MAIN_HEIGHT]);

  // Handle click to seek
  const handleWaveformClick = useCallback((e: React.MouseEvent<HTMLCanvasElement>, deck: DeckId) => {
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
    const clickTime = deckState.position + ((x - playheadX) * secondsPerPixel);
    const clampedTime = Math.max(0, Math.min(deckState.duration, clickTime));
    
    seek(deck, clampedTime);
  }, [seek, visibleSeconds]);

  return (
    <div ref={containerRef} className="w-full bg-surface-0 relative" style={{ height }} onWheel={handleWheelZoom}>
      {/* Waveform controls overlay (top-right) */}
      <div className="absolute top-1 right-2 z-30 flex items-center gap-1 bg-[#111]/80 rounded px-1.5 py-1 backdrop-blur-sm border border-[#333]/50">
        {/* Color mode selector */}
        {(['rgb', '3band', 'single'] as WaveformColorMode[]).map(mode => (
          <button
            key={mode}
            onClick={() => setColorMode(mode)}
            aria-pressed={colorMode === mode}
            className={`px-2 min-h-[28px] flex items-center justify-center text-[10px] font-bold rounded transition-colors ${
              colorMode === mode
                ? 'bg-brand/30 text-brand border border-brand/50'
                : 'text-neutral-500 hover:text-neutral-300 hover:bg-[#333]'
            }`}
            title={`Waveform: ${mode === 'rgb' ? 'RGB (multi-color)' : mode === '3band' ? '3-Band' : 'Single color'}`}
          >
            {mode === 'rgb' ? 'RGB' : mode === '3band' ? '3B' : 'CLR'}
          </button>
        ))}
        <div className="w-px h-5 bg-[#444]" />
        {/* Zoom controls */}
        <button
          onClick={zoomIn}
          className="w-7 h-7 flex items-center justify-center text-[14px] text-neutral-400 hover:text-white hover:bg-[#333] rounded transition-colors"
          title="Zoom in (Ctrl+Scroll up)"
          aria-label="Zoom in"
        >+</button>
        <button
          onClick={zoomReset}
          className="px-1.5 h-7 min-w-[36px] flex items-center justify-center text-[10px] text-neutral-500 hover:text-white hover:bg-[#333] rounded transition-colors font-mono"
          title="Reset zoom"
          aria-label="Reset zoom"
        >{visibleSeconds.toFixed(0)}s</button>
        <button
          onClick={zoomOut}
          className="w-7 h-7 flex items-center justify-center text-[14px] text-neutral-400 hover:text-white hover:bg-[#333] rounded transition-colors"
          title="Zoom out (Ctrl+Scroll down)"
          aria-label="Zoom out"
        >−</button>
      </div>

      {/* Overview waveforms */}
      <canvas 
        ref={overviewRef}
        className="w-full cursor-pointer"
        style={{ height: OVERVIEW_HEIGHT }}
      />
      
      {/* Overview → Main separator */}
      <div className="h-px bg-[#444]" />
      
      {/* Main waveform Deck A */}
      <canvas 
        ref={mainCanvasARef}
        className="w-full cursor-crosshair"
        style={{ height: MAIN_HEIGHT }}
        onClick={(e) => handleWaveformClick(e, 'A')}
      />
      
      {/* Separator with crossfader indicator */}
      <div className="h-1 bg-surface-1 relative">
        <div className="absolute left-1/2 -translate-x-1/2 -top-0.5 w-4 h-2 bg-neutral-500 rounded-sm" />
      </div>
      
      {/* Main waveform Deck B */}
      <canvas 
        ref={mainCanvasBRef}
        className="w-full cursor-crosshair"
        style={{ height: MAIN_HEIGHT }}
        onClick={(e) => handleWaveformClick(e, 'B')}
      />
    </div>
  );
};

export default React.memo(DJDualWaveform);

