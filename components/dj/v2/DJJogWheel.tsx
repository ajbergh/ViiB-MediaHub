/**
 * ViiB MediaHub - DJ Jog Wheel Component (v2)
 * 
 * Professional circular jog wheel display inspired by PCDJ DEX / Serato.
 * Shows BPM, tempo percentage, elapsed time, and rotation animation.
 * Features high-quality SVG graphics with metallic textures.
 * 
 * @module components/dj/v2/DJJogWheel
 */

import React, { useRef, useCallback, useState, useEffect, useMemo } from 'react';
import { useStore } from '../../../store';
import { useDJAudioEngineActions } from '../../../hooks/useDJAudioEngine';
import { getDJAudioEngine } from '../../../lib/djAudio';
// BPM glow now handled directly in RAF loop (no useBpmGlow hook)
import type { DeckId } from '../../../slices/djMixerSlice';

interface DJJogWheelProps {
  deck: DeckId;
  size?: number;
  responsive?: boolean;
}

export const DJJogWheel: React.FC<DJJogWheelProps> = ({ deck, size = 180, responsive = false }) => {
  const containerRef = useRef<HTMLDivElement>(null);
  const [isDragging, setIsDragging] = useState(false);
  const isDraggingRef = useRef(false);
  const [computedSize, setComputedSize] = useState(size > 0 ? size : 140);
  const lastAngleRef = useRef<number | null>(null);
  const rotationRef = useRef(0);
  // Refs for RAF-driven elements (avoid React re-renders)
  const rotationDotRef = useRef<SVGCircleElement>(null);
  const glowRingRef = useRef<SVGCircleElement>(null);
  const bpmTextRef = useRef<SVGTextElement>(null);
  const tempoTextRef = useRef<SVGTextElement>(null);
  const timeTextRef = useRef<SVGTextElement>(null);
  const glowIntensityRef = useRef(0);
  const lastBeatTimeRef = useRef(performance.now());
  
  // Granular store selectors - only subscribe to infrequently-changing values
  const isPlaying = useStore(state => deck === 'A' ? state.djDeckA.isPlaying : state.djDeckB.isPlaying);
  const effectiveBpm = useStore(state => deck === 'A' ? state.djDeckA.effectiveBpm : state.djDeckB.effectiveBpm);
  const originalBpm = useStore(state => deck === 'A' ? state.djDeckA.originalBpm : state.djDeckB.originalBpm);
  const tempo = useStore(state => deck === 'A' ? state.djDeckA.tempo : state.djDeckB.tempo);
  const track = useStore(state => deck === 'A' ? state.djDeckA.track : state.djDeckB.track);
  const duration = useStore(state => deck === 'A' ? state.djDeckA.duration : state.djDeckB.duration);
  const { startScratch, updateScratch, endScratch } = useDJAudioEngineActions();
  
  const bpm = effectiveBpm || originalBpm || 0;
  const tempoPercent = ((tempo - 1) * 100).toFixed(1);
  const tempoDisplay = tempo >= 1 ? `+${tempoPercent}%` : `${tempoPercent}%`;
  
  // Responsive size calculation
  useEffect(() => {
    if (!responsive) {
      setComputedSize(size > 0 ? size : 140);
      return;
    }
    
    const container = containerRef.current;
    if (!container) return;
    
    const updateSize = () => {
      const parent = container.parentElement;
      if (parent) {
        const availableWidth = parent.clientWidth - 20; // padding
        const availableHeight = parent.clientHeight - 100; // space for controls below
        const newSize = Math.min(availableWidth, availableHeight, 500);
        setComputedSize(Math.max(100, newSize));
      }
    };
    
    updateSize();
    const resizeObserver = new ResizeObserver(updateSize);
    if (container.parentElement) {
      resizeObserver.observe(container.parentElement);
    }
    
    return () => resizeObserver.disconnect();
  }, [responsive, size]);
  
  // Combined animation loop: rotation + BPM glow + time display
  // Uses refs + direct DOM manipulation to avoid React re-renders entirely
  useEffect(() => {
    let animationId: number;
    let idleTimeoutId: ReturnType<typeof setTimeout>;
    const lastPosRef = { current: -1 };
    const lastGlowRef = { current: -1 };
    
    const scheduleNext = (idle: boolean) => {
      if (idle) {
        // Throttle to ~4fps when idle — allows GPU to enter low-power mode
        idleTimeoutId = setTimeout(() => { animationId = requestAnimationFrame(animate); }, 250);
      } else {
        animationId = requestAnimationFrame(animate);
      }
    };
    
    const animate = () => {
      // Read position directly from engine when playing/scratching for smooth 60fps,
      // fall back to store position when paused (store is throttled to ~15fps)
      const state = useStore.getState();
      const deckState = deck === 'A' ? state.djDeckA : state.djDeckB;
      const currentBpm = deckState.effectiveBpm || deckState.originalBpm || 0;
      const playing = deckState.isPlaying;
      const engine = getDJAudioEngine();
      const scratching = engine?.isScratching(deck) ?? false;
      const pos = (playing || scratching) && engine?.initialized
        ? engine.getPosition(deck)
        : deckState.position;
      const currentSize = computedSize;
      
      // Skip all DOM work when no track loaded — nothing to animate
      if (!deckState.track) {
        scheduleNext(true);
        return;
      }
      
      // Skip DOM work when paused, not dragging, and position hasn't changed
      if (!playing && !isDraggingRef.current && pos === lastPosRef.current) {
        scheduleNext(true);
        return;
      }
      lastPosRef.current = pos;
      
      // --- Rotation ---
      if ((playing || scratching || isDraggingRef.current) && currentBpm > 0 && typeof pos === 'number' && !isNaN(pos)) {
        const beatsElapsed = (pos / 60) * currentBpm;
        const targetRotation = (beatsElapsed * 360) % 360;
        
        if (!isNaN(targetRotation)) {
          const diff = targetRotation - rotationRef.current;
          const adjustedDiff = diff > 180 ? diff - 360 : diff < -180 ? diff + 360 : diff;
          rotationRef.current += adjustedDiff * 0.3;
          if (rotationRef.current > 360) rotationRef.current -= 360;
          if (rotationRef.current < 0) rotationRef.current += 360;
        }
      }
      
      // Apply rotation via DOM (no React state update)
      if (rotationDotRef.current) {
        rotationDotRef.current.setAttribute('transform', 
          `rotate(${rotationRef.current}, ${currentSize / 2}, ${currentSize / 2})`);
      }
      
      // --- BPM Glow (replaces useBpmGlow hook) ---
      if (playing && currentBpm > 0) {
        const beatDuration = 60000 / currentBpm;
        const now = performance.now();
        const elapsed = now - lastBeatTimeRef.current;
        const beatProgress = (elapsed % beatDuration) / beatDuration;
        const intensity = beatProgress < 0.1 
          ? beatProgress * 10
          : 1 - (beatProgress - 0.1) * 1.1;
        glowIntensityRef.current = Math.max(0, intensity);
      } else {
        glowIntensityRef.current = 0;
      }
      
      // Apply glow to ring via DOM — only update when intensity changes meaningfully
      // to avoid costly SVG Gaussian blur filter recomposition every frame
      if (glowRingRef.current) {
        const gi = glowIntensityRef.current;
        if (playing && gi > 0) {
          // Only update DOM when glow changes by >5% — reduces SVG filter recomp from 60fps to ~20fps
          if (Math.abs(gi - lastGlowRef.current) > 0.05) {
            lastGlowRef.current = gi;
            glowRingRef.current.setAttribute('stroke-width', String(2 + gi * 3));
            glowRingRef.current.setAttribute('opacity', String(0.3 + gi * 0.5));
          }
          glowRingRef.current.style.display = '';
        } else {
          if (lastGlowRef.current !== 0) {
            lastGlowRef.current = 0;
            glowRingRef.current.style.display = 'none';
          }
        }
      }
      
      // --- Time display update ---
      if (timeTextRef.current) {
        const mins = Math.floor(pos / 60);
        const secs = Math.floor(pos % 60);
        const tenths = Math.floor((pos % 1) * 10);
        timeTextRef.current.textContent = 
          `${mins.toString().padStart(2, '0')}:${secs.toString().padStart(2, '0')}.${tenths}`;
      }
      
      // --- Progress arc ---
      if (progressArcRef.current) {
        const dur = deckState.duration;
        if (dur > 0) {
          const progress = (pos / dur) * 100;
          if (progress > 0.5) {
            const progressAngle = (progress / 100) * 360;
            const center = currentSize / 2;
            const radius = currentSize / 2 - 6;
            const startRad = (-90 * Math.PI) / 180;
            const endRad = ((Math.min(progressAngle, 359.9) - 90) * Math.PI) / 180;
            const startX = center + radius * Math.cos(startRad);
            const startY = center + radius * Math.sin(startRad);
            const endX = center + radius * Math.cos(endRad);
            const endY = center + radius * Math.sin(endRad);
            const largeArc = progressAngle > 180 ? 1 : 0;
            progressArcRef.current.setAttribute('d', 
              `M ${startX} ${startY} A ${radius} ${radius} 0 ${largeArc} 1 ${endX} ${endY}`);
            progressArcRef.current.style.display = '';
          } else {
            progressArcRef.current.style.display = 'none';
          }
        } else {
          progressArcRef.current.style.display = 'none';
        }
      }
      
      scheduleNext(false);
    };
    
    animationId = requestAnimationFrame(animate);
    return () => { cancelAnimationFrame(animationId); clearTimeout(idleTimeoutId); };
  }, [deck, computedSize]); // Only depends on deck identity and size - NOT position/bpm/isPlaying

  // formatTime no longer needed in render - handled by RAF loop via DOM ref

  // Calculate angle from center
  const getAngleFromCenter = (clientX: number, clientY: number): number => {
    if (!containerRef.current) return 0;
    const rect = containerRef.current.getBoundingClientRect();
    const centerX = rect.left + rect.width / 2;
    const centerY = rect.top + rect.height / 2;
    return Math.atan2(clientY - centerY, clientX - centerX) * (180 / Math.PI);
  };

  const handlePointerDown = useCallback((e: React.PointerEvent) => {
    if (!track) return;
    e.preventDefault();
    (e.target as HTMLElement).setPointerCapture(e.pointerId);
    
    setIsDragging(true);
    isDraggingRef.current = true;
    lastAngleRef.current = getAngleFromCenter(e.clientX, e.clientY);
    startScratch(deck);
  }, [deck, track, startScratch]);

  const handlePointerMove = useCallback((e: React.PointerEvent) => {
    if (!isDragging || lastAngleRef.current === null) return;
    
    const currentAngle = getAngleFromCenter(e.clientX, e.clientY);
    let deltaAngle = currentAngle - lastAngleRef.current;
    
    if (deltaAngle > 180) deltaAngle -= 360;
    if (deltaAngle < -180) deltaAngle += 360;
    
    const deltaTime = (deltaAngle / 360) * 2;
    const velocity = deltaAngle / 10;
    
    updateScratch(deck, deltaTime, velocity);
    lastAngleRef.current = currentAngle;
  }, [deck, isDragging, updateScratch]);

  const handlePointerUp = useCallback((e: React.PointerEvent) => {
    if (!isDragging) return;
    (e.target as HTMLElement).releasePointerCapture(e.pointerId);
    
    setIsDragging(false);
    isDraggingRef.current = false;
    lastAngleRef.current = null;
    endScratch(deck, 0, true);
  }, [deck, isDragging, endScratch]);

  // Colors based on deck
  const accentColor = deck === 'A' ? '#3b82f6' : '#ec4899';
  // accentColorDim available if needed: deck === 'A' ? '#3b82f680' : '#ec489980'

  // Progress arc ref - updated in RAF
  const progressArcRef = useRef<SVGPathElement>(null);
  
  // Generate arc path for progress indicator
  const generateArc = (startAngle: number, endAngle: number, radius: number): string => {
    const center = computedSize / 2;
    const startRad = ((startAngle - 90) * Math.PI) / 180;
    const endRad = ((endAngle - 90) * Math.PI) / 180;
    
    const startX = center + radius * Math.cos(startRad);
    const startY = center + radius * Math.sin(startRad);
    const endX = center + radius * Math.cos(endRad);
    const endY = center + radius * Math.sin(endRad);
    
    const largeArcFlag = endAngle - startAngle > 180 ? 1 : 0;
    
    return `M ${startX} ${startY} A ${radius} ${radius} 0 ${largeArcFlag} 1 ${endX} ${endY}`;
  };

  // Generate tick marks
  const tickMarks = useMemo(() => {
    const marks = [];
    for (let i = 0; i < 12; i++) {
      const angle = i * 30;
      const isQuarter = angle % 90 === 0;
      marks.push({
        angle,
        length: isQuarter ? 12 : 8,
        width: isQuarter ? 2 : 1,
        color: isQuarter ? '#666' : '#444'
      });
    }
    return marks;
  }, []);

  return (
    <div 
      ref={containerRef}
      className={`relative select-none ${isDragging ? 'cursor-grabbing' : 'cursor-grab'}`}
      style={{ width: computedSize, height: computedSize }}
      onPointerDown={handlePointerDown}
      onPointerMove={handlePointerMove}
      onPointerUp={handlePointerUp}
      onPointerLeave={handlePointerUp}
    >
      <svg 
        width={computedSize} 
        height={computedSize} 
        viewBox={`0 0 ${computedSize} ${computedSize}`}
        className="touch-none"
      >
        <defs>
          {/* Metallic rim gradient */}
          <linearGradient id={`metalRim-${deck}`} x1="0%" y1="0%" x2="100%" y2="100%">
            <stop offset="0%" stopColor="#5a5a5a"/>
            <stop offset="20%" stopColor="#3d3d3d"/>
            <stop offset="40%" stopColor="#4a4a4a"/>
            <stop offset="60%" stopColor="#2a2a2a"/>
            <stop offset="80%" stopColor="#3d3d3d"/>
            <stop offset="100%" stopColor="#484848"/>
          </linearGradient>
          
          {/* Inner platter gradient */}
          <radialGradient id={`platter-${deck}`} cx="40%" cy="35%" r="65%">
            <stop offset="0%" stopColor="#2d2d2d"/>
            <stop offset="50%" stopColor="#1f1f1f"/>
            <stop offset="100%" stopColor="#151515"/>
          </radialGradient>
          
          {/* Center display gradient */}
          <radialGradient id={`centerDisplay-${deck}`} cx="50%" cy="40%" r="60%">
            <stop offset="0%" stopColor="#252525"/>
            <stop offset="100%" stopColor="#1a1a1a"/>
          </radialGradient>
          

          
          {/* Text glow */}
          <filter id={`textGlow-${deck}`}>
            <feGaussianBlur stdDeviation="1.5" result="blur"/>
            <feMerge>
              <feMergeNode in="blur"/>
              <feMergeNode in="SourceGraphic"/>
            </feMerge>
          </filter>
          
          {/* Drop shadow */}
          <filter id={`dropShadow-${deck}`}>
            <feDropShadow dx="0" dy="3" stdDeviation="5" floodColor="#000" floodOpacity="0.6"/>
          </filter>
        </defs>
        
        {/* Outer glow ring (pulses with BPM) - animated via ref */}
        <circle 
          ref={glowRingRef}
          cx={computedSize/2} 
          cy={computedSize/2} 
          r={computedSize/2 - 3} 
          fill="none" 
          stroke={accentColor} 
          strokeWidth="2"
          opacity="0.3"
          style={{ display: 'none' }}
        />
        
        {/* Outer metallic rim */}
        <circle 
          cx={computedSize/2} 
          cy={computedSize/2} 
          r={computedSize/2 - 6} 
          fill="none" 
          stroke={`url(#metalRim-${deck})`} 
          strokeWidth="8"
          filter={`url(#dropShadow-${deck})`}
        />
        
        {/* Rim highlight arc */}
        <path
          d={generateArc(200, 320, computedSize/2 - 6)}
          fill="none"
          stroke="rgba(255,255,255,0.1)"
          strokeWidth="2"
        />
        
        {/* Progress arc on rim - updated via ref in RAF */}
        <path 
          ref={progressArcRef}
          d=""
          fill="none"
          stroke={accentColor}
          strokeWidth="4"
          strokeLinecap="round"
          opacity="0.7"
          style={{ display: 'none' }}
        />
        
        {/* Tick marks around outer edge */}
        <g>
          {tickMarks.map(({ angle, length, width, color }) => (
            <line
              key={angle}
              x1={computedSize/2}
              y1={14}
              x2={computedSize/2}
              y2={14 + length}
              stroke={color}
              strokeWidth={width}
              transform={`rotate(${angle}, ${computedSize/2}, ${computedSize/2})`}
            />
          ))}
        </g>
        
        {/* Main platter */}
        <circle 
          cx={computedSize/2} 
          cy={computedSize/2} 
          r={computedSize/2 - 18} 
          fill={`url(#platter-${deck})`}
        />
        
        {/* Vinyl grooves (concentric circles) */}
        {[0.85, 0.75, 0.65, 0.55].map((ratio, i) => (
          <circle
            key={i}
            cx={computedSize/2}
            cy={computedSize/2}
            r={(computedSize/2 - 18) * ratio}
            fill="none"
            stroke="rgba(255,255,255,0.03)"
            strokeWidth="1"
          />
        ))}
        
        {/* Inner display area */}
        <circle 
          cx={computedSize/2} 
          cy={computedSize/2} 
          r={computedSize * 0.32} 
          fill={`url(#centerDisplay-${deck})`}
          stroke={track ? accentColor : '#333'}
          strokeWidth={track ? 2 : 1}
          opacity={track ? 1 : 0.6}
        />
        
        {/* BPM Display */}
        <text 
          x={computedSize/2} 
          y={computedSize * 0.4}
          fontFamily="'SF Mono', 'Consolas', monospace" 
          fontSize={computedSize * 0.18}
          fontWeight="700"
          fill={track ? '#ffffff' : '#444'}
          textAnchor="middle"
          dominantBaseline="middle"
          filter={track && isPlaying ? `url(#textGlow-${deck})` : undefined}
        >
          {bpm > 0 ? bpm.toFixed(1) : '---.--'}
        </text>
        
        {/* Tempo percentage display */}
        <text 
          x={computedSize * 0.35}
          y={computedSize * 0.58}
          fontFamily="'SF Mono', 'Consolas', monospace"
          fontSize={computedSize * 0.06}
          fill={tempo !== 1 ? accentColor : '#555'}
          textAnchor="middle"
        >
          {tempoDisplay}
        </text>
        
        {/* Range indicator */}
        <text 
          x={computedSize * 0.65}
          y={computedSize * 0.58}
          fontFamily="'SF Mono', 'Consolas', monospace"
          fontSize={computedSize * 0.05}
          fill="#444"
          textAnchor="middle"
        >
          ±16%
        </text>
        
        {/* Time Display - updated via ref in RAF loop */}
        <text 
          ref={timeTextRef}
          x={computedSize/2}
          y={computedSize * 0.72}
          fontFamily="'SF Mono', 'Consolas', monospace"
          fontSize={computedSize * 0.095}
          fontWeight="600"
          fill={track ? '#e0e0e0' : '#444'}
          textAnchor="middle"
          dominantBaseline="middle"
        >
          00:00.0
        </text>
        
        {/* Rotation indicator dot - transform updated via ref in RAF loop */}
        <circle 
          ref={rotationDotRef}
          cx={computedSize/2}
          cy={computedSize * 0.14}
          r={computedSize * 0.035}
          fill={accentColor}
          transform={`rotate(0, ${computedSize/2}, ${computedSize/2})`}
        />
        
        {/* Deck label */}
        <text 
          x={computedSize/2}
          y={computedSize - 8}
          fontFamily="system-ui, sans-serif"
          fontSize={computedSize * 0.055}
          fontWeight="700"
          fill="#3a3a3a"
          textAnchor="middle"
        >
          DECK {deck}
        </text>
      </svg>
      
      {/* Scratch indicator overlay */}
      {isDragging && (
        <div 
          className="absolute top-3 left-1/2 -translate-x-1/2 px-2 py-0.5 rounded-sm text-[9px] font-bold text-white uppercase tracking-wider"
          style={{ backgroundColor: accentColor, boxShadow: `0 0 10px ${accentColor}` }}
        >
          Scratch
        </div>
      )}
    </div>
  );
};

export default React.memo(DJJogWheel);
