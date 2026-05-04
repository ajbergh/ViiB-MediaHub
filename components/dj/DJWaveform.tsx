/**
 * ViiB MediaHub - DJ Waveform Component
 * 
 * Canvas-based waveform display for DJ decks.
 * Shows precomputed peak data with a scrolling playhead.
 * 
 * Features:
 * - Scrolling waveform centered on playhead
 * - Click-to-seek functionality
 * - Drag-to-scratch (vinyl scratch simulation)
 * - Beat grid markers (Phase 2+)
 * - Color-coded frequency bands (Phase 3+)
 * 
 * @module components/dj/DJWaveform
 */

import React, { useRef, useEffect, useCallback, useState } from 'react';
import { useStore } from '../../store';
import { useDJAudioEngine } from '../../hooks/useDJAudioEngine';
import type { DeckId } from '../../slices/djMixerSlice';

interface DJWaveformProps {
  deck: DeckId;
}

// Scratch state interface
interface ScratchState {
  isDragging: boolean;
  lastX: number;
  lastTime: number;
  velocityHistory: number[];
}

export const DJWaveform: React.FC<DJWaveformProps> = ({ deck }) => {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const containerRef = useRef<HTMLDivElement>(null);
  const scratchStateRef = useRef<ScratchState>({
    isDragging: false,
    lastX: 0,
    lastTime: 0,
    velocityHistory: [],
  });
  
  // State for scratch indicator (triggers re-render)
  const [isScratchActive, setIsScratchActive] = useState(false);
  
  const deckState = useStore(state => deck === 'A' ? state.djDeckA : state.djDeckB);
  const { seek, startScratch, updateScratch, endScratch } = useDJAudioEngine();
  
  const { track, position, duration, waveformPeaks, isPlaying, cuePoint, beatGrid } = deckState;

  // Constants for scratch behavior
  const VISIBLE_SECONDS = 10; // How many seconds to show total
  const VELOCITY_SAMPLES = 5; // Number of velocity samples to average for momentum

  // Debug logging for waveform data
  useEffect(() => {
    console.log(`🎨 DJWaveform Deck ${deck}: track=${track?.title}, peaks=${waveformPeaks?.length || 0}, duration=${duration}`);
  }, [deck, track, waveformPeaks, duration]);

  // Draw waveform
  useEffect(() => {
    const canvas = canvasRef.current;
    const container = containerRef.current;
    if (!canvas || !container) return;

    const ctx = canvas.getContext('2d');
    if (!ctx) return;

    // Set canvas size
    const rect = container.getBoundingClientRect();
    const dpr = window.devicePixelRatio || 1;
    canvas.width = rect.width * dpr;
    canvas.height = rect.height * dpr;
    canvas.style.width = `${rect.width}px`;
    canvas.style.height = `${rect.height}px`;
    ctx.scale(dpr, dpr);

    const width = rect.width;
    const height = rect.height;
    const centerY = height / 2;

    // Clear canvas
    ctx.fillStyle = '#1a1a1a';
    ctx.fillRect(0, 0, width, height);

    // If no waveform data, show placeholder
    if (!waveformPeaks || waveformPeaks.length === 0) {
      ctx.fillStyle = '#666';
      ctx.textAlign = 'center';
      ctx.font = '12px system-ui';
      
      if (track) {
        ctx.fillText(
          'Generating waveform...',
          width / 2,
          height / 2
        );
        // Draw subtle placeholder bars
        ctx.fillStyle = '#333';
        for (let i = 0; i < width; i += 4) {
          const h = Math.sin(i * 0.05) * 10 + Math.random() * 5;
          ctx.fillRect(i, centerY - h, 2, h * 2);
        }
      } else {
        ctx.fillText(
          'No track loaded',
          width / 2,
          height / 2
        );
      }
      return;
    }

    // Need valid duration to calculate waveform display
    if (!duration || duration <= 0) {
      ctx.fillStyle = '#666';
      ctx.textAlign = 'center';
      ctx.font = '12px system-ui';
      ctx.fillText(
        'Loading audio...',
        width / 2,
        height / 2
      );
      return;
    }

    // Calculate visible range
    // Show ~10 seconds of audio centered on playhead (adjustable zoom)
    const VISIBLE_SECONDS = 10; // How many seconds to show total
    const playheadX = width / 2; // Playhead is always at center
    const secondsPerPixel = VISIBLE_SECONDS / width;
    const visibleStartTime = Math.max(0, position - (playheadX * secondsPerPixel));
    const visibleEndTime = Math.min(duration, position + ((width - playheadX) * secondsPerPixel));

    // Map waveform data to visible range
    const peaksPerSecond = waveformPeaks.length / duration;
    const startPeakIndex = Math.floor(visibleStartTime * peaksPerSecond);
    const endPeakIndex = Math.ceil(visibleEndTime * peaksPerSecond);

    // Draw waveform
    const maxAmplitude = height / 2 - 4;

    ctx.beginPath();
    ctx.strokeStyle = deck === 'A' ? '#3b82f6' : '#8b5cf6'; // Blue for A, Purple for B
    ctx.lineWidth = 1;

    for (let i = startPeakIndex; i < endPeakIndex && i < waveformPeaks.length; i++) {
      const peakTime = (i / waveformPeaks.length) * duration;
      const x = playheadX + ((peakTime - position) / secondsPerPixel);
      const peak = waveformPeaks[i] || 0;
      const amplitude = peak * maxAmplitude;

      ctx.moveTo(x, centerY - amplitude);
      ctx.lineTo(x, centerY + amplitude);
    }
    ctx.stroke();

    // Draw beat grid markers
    if (beatGrid && beatGrid.length > 0) {
      ctx.strokeStyle = 'rgba(255, 255, 255, 0.2)';
      ctx.lineWidth = 1;
      
      let beatCount = 0;
      for (const beatTime of beatGrid) {
        if (beatTime >= visibleStartTime && beatTime <= visibleEndTime) {
          const beatX = playheadX + ((beatTime - position) / secondsPerPixel);
          
          // Draw every 4th beat (downbeat) more prominently
          if (beatCount % 4 === 0) {
            ctx.strokeStyle = 'rgba(255, 255, 255, 0.4)';
            ctx.lineWidth = 2;
          } else {
            ctx.strokeStyle = 'rgba(255, 255, 255, 0.15)';
            ctx.lineWidth = 1;
          }
          
          ctx.beginPath();
          ctx.moveTo(beatX, 0);
          ctx.lineTo(beatX, height);
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
      ctx.lineTo(cueX, height);
      ctx.stroke();
    }

    // Draw playhead
    ctx.strokeStyle = '#ef4444';
    ctx.lineWidth = 2;
    ctx.beginPath();
    ctx.moveTo(playheadX, 0);
    ctx.lineTo(playheadX, height);
    ctx.stroke();

    // Draw playhead triangle
    ctx.fillStyle = '#ef4444';
    ctx.beginPath();
    ctx.moveTo(playheadX - 6, 0);
    ctx.lineTo(playheadX + 6, 0);
    ctx.lineTo(playheadX, 8);
    ctx.closePath();
    ctx.fill();

  }, [track, position, duration, waveformPeaks, deck, cuePoint, beatGrid]);

  // Calculate time from X position
  const getTimeFromX = useCallback((x: number, width: number): number => {
    const playheadX = width / 2;
    const secondsPerPixel = VISIBLE_SECONDS / width;
    return position + ((x - playheadX) * secondsPerPixel);
  }, [position]);

  // Handle mouse down - start potential scratch
  const handleMouseDown = useCallback((e: React.MouseEvent<HTMLCanvasElement>) => {
    if (!track || !duration) return;

    const canvas = canvasRef.current;
    if (!canvas) return;

    const rect = canvas.getBoundingClientRect();
    const x = e.clientX - rect.left;

    // Initialize scratch state
    scratchStateRef.current = {
      isDragging: true,
      lastX: x,
      lastTime: Date.now(),
      velocityHistory: [],
    };

    // Start scratch mode in audio engine
    startScratch(deck);
    setIsScratchActive(true);

    // Change cursor to grabbing
    canvas.style.cursor = 'grabbing';

    console.log(`🎛️ DJWaveform: Scratch started at x=${x}`);
  }, [deck, track, duration, startScratch]);

  // Handle mouse move - update scratch position
  const handleMouseMove = useCallback((e: React.MouseEvent<HTMLCanvasElement>) => {
    const state = scratchStateRef.current;
    if (!state.isDragging || !track || !duration) return;

    const canvas = canvasRef.current;
    if (!canvas) return;

    const rect = canvas.getBoundingClientRect();
    const x = e.clientX - rect.left;
    const width = rect.width;
    const now = Date.now();

    // Calculate movement
    const deltaX = x - state.lastX;
    const deltaMs = Math.max(1, now - state.lastTime);
    
    // Convert pixel movement to time
    const secondsPerPixel = VISIBLE_SECONDS / width;
    const deltaTime = deltaX * secondsPerPixel;
    
    // Calculate velocity (pixels per ms)
    const velocity = deltaX / deltaMs;

    // Track velocity history for momentum calculation
    state.velocityHistory.push(velocity);
    if (state.velocityHistory.length > VELOCITY_SAMPLES) {
      state.velocityHistory.shift();
    }

    // Update audio engine with scratch movement
    updateScratch(deck, deltaTime, velocity);

    // Update state
    state.lastX = x;
    state.lastTime = now;

  }, [deck, track, duration, updateScratch]);

  // Handle mouse up - end scratch with momentum
  const handleMouseUp = useCallback((e: React.MouseEvent<HTMLCanvasElement>) => {
    const state = scratchStateRef.current;
    if (!state.isDragging) return;

    const canvas = canvasRef.current;
    if (!canvas) return;

    // Calculate final velocity from history (average)
    const avgVelocity = state.velocityHistory.length > 0
      ? state.velocityHistory.reduce((a, b) => a + b, 0) / state.velocityHistory.length
      : 0;

    console.log(`🎛️ DJWaveform: Scratch ended, avgVelocity=${avgVelocity.toFixed(3)}`);

    // End scratch mode with momentum
    endScratch(deck, avgVelocity, true);

    // Reset state
    state.isDragging = false;
    state.velocityHistory = [];
    setIsScratchActive(false);

    // Restore cursor
    canvas.style.cursor = 'grab';
  }, [deck, endScratch]);

  // Handle mouse leave - end scratch if dragging
  const handleMouseLeave = useCallback((e: React.MouseEvent<HTMLCanvasElement>) => {
    const state = scratchStateRef.current;
    if (!state.isDragging) return;

    // End scratch without momentum since we left the area
    endScratch(deck, 0, true);

    state.isDragging = false;
    state.velocityHistory = [];
    setIsScratchActive(false);

    const canvas = canvasRef.current;
    if (canvas) {
      canvas.style.cursor = 'grab';
    }
  }, [deck, endScratch]);

  // Handle simple click (no drag) - seek to position
  const handleClick = useCallback((e: React.MouseEvent<HTMLCanvasElement>) => {
    // Only treat as click if mouse didn't move significantly
    // (this is handled naturally since click fires after mouseup if no significant drag)
    // The scratch handlers already handle seeking during drag
  }, []);

  return (
    <div 
      ref={containerRef}
      className="w-full h-full relative"
    >
      <canvas 
        ref={canvasRef}
        className="w-full h-full cursor-grab"
        onMouseDown={handleMouseDown}
        onMouseMove={handleMouseMove}
        onMouseUp={handleMouseUp}
        onMouseLeave={handleMouseLeave}
        onClick={handleClick}
      />
      {/* Deck label overlay */}
      <div className="absolute top-2 left-2 text-xs font-bold text-white/50">
        DECK {deck}
      </div>
      {/* Playing indicator */}
      {isPlaying && !isScratchActive && (
        <div className="absolute top-2 right-2 w-2 h-2 rounded-full bg-green-500 animate-pulse" />
      )}
      {/* Scratch mode indicator */}
      {isScratchActive && (
        <div className="absolute bottom-2 left-2 px-2 py-1 rounded bg-orange-500/80 text-xs font-bold text-white">
          SCRATCH
        </div>
      )}
    </div>
  );
};

export default DJWaveform;