/**
 * ViiB MediaHub - DJ Jog Wheel Component (v2)
 * 
 * Professional circular jog wheel display inspired by PCDJ DEX / Serato.
 * Shows BPM, tempo percentage, elapsed time, and rotation animation.
 * Token-driven platter: neutral rings with a deck-colored progress ring and position marker.
 * 
 * @module components/dj/v2/DJJogWheel
 */

import React, { useRef, useCallback, useState, useEffect, useMemo } from 'react';
import { useStore } from '../../../store';
import { useDJAudioEngineActions } from '../../../hooks/useDJAudioEngine';
import { useScratchAvailability } from '../../../hooks/useScratchAvailability';
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
  const pointerRef = useRef<number | null>(null);
  const lastMoveTimeRef = useRef(0);
  const releaseVelocityRef = useRef(0);
  const spindleRef = useRef(false);
  const lastPlaybackPositionRef = useRef<number | null>(null);
  // Refs for RAF-driven elements (avoid React re-renders)
  const rotationDotRef = useRef<SVGCircleElement>(null);
  const scratchLabelRef = useRef<HTMLDivElement>(null);
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
  const { updateScratch, endScratch } = useDJAudioEngineActions();
  
  const scratchAvailability = useScratchAvailability(deck);
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
        const availableHeight = parent.clientHeight - 16;
        // Cap raised 300 → 480 so the wheel fills the deck instead of leaving
        // big empty corners on the jog area at 1080p+.
        const newSize = Math.min(availableWidth, availableHeight, 480);
        setComputedSize(Math.max(180, newSize));
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
      if (scratchLabelRef.current) scratchLabelRef.current.textContent = engine.getScratchStatus(deck);
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
      // 33 1/3 RPM: one revolution corresponds to 1.8 seconds of audio.
      // Integrate playback motion to preserve the hand's angle after release.
      if (!isDraggingRef.current && lastPlaybackPositionRef.current !== null) {
        rotationRef.current = (rotationRef.current + (pos - lastPlaybackPositionRef.current) * 200) % 360;
      }
      lastPlaybackPositionRef.current = pos;

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
            const radius = currentSize / 2 - 7;
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

  useEffect(() => {
    setIsDragging(false);
    lastPlaybackPositionRef.current = null;
    return () => {
      endScratch(deck, 0, true);
      isDraggingRef.current = false;
      pointerRef.current = null;
      lastAngleRef.current = null;
    };
  }, [deck, track?.id, endScratch]);

  const handlePointerDown = useCallback((e: React.PointerEvent<HTMLDivElement>) => {
    if (!track || pointerRef.current !== null || e.button !== 0) return;
    if (!getDJAudioEngine().startScratch(deck)) return;
    e.preventDefault();
    e.currentTarget.setPointerCapture(e.pointerId);
    pointerRef.current = e.pointerId;
    setIsDragging(true);
    isDraggingRef.current = true;
    lastAngleRef.current = getAngleFromCenter(e.clientX, e.clientY);
    spindleRef.current = false;
    releaseVelocityRef.current = 0;
    lastMoveTimeRef.current = e.timeStamp;
  }, [deck, track]);

  const handlePointerMove = useCallback((e: React.PointerEvent<HTMLDivElement>) => {
    if (pointerRef.current !== e.pointerId || lastAngleRef.current === null) return;
    const rect = e.currentTarget.getBoundingClientRect();
    // Angle is unstable at the spindle; re-anchor when leaving that dead zone.
    if (Math.hypot(e.clientX - rect.left - rect.width / 2, e.clientY - rect.top - rect.height / 2) < rect.width * 0.08) {
      spindleRef.current = true;
      releaseVelocityRef.current = 0;
      lastAngleRef.current = getAngleFromCenter(e.clientX, e.clientY);
      lastMoveTimeRef.current = e.timeStamp;
      return;
    }
    const currentAngle = getAngleFromCenter(e.clientX, e.clientY);
    if (spindleRef.current) {
      spindleRef.current = false;
      lastAngleRef.current = currentAngle;
      lastMoveTimeRef.current = e.timeStamp;
      return;
    }
    let deltaAngle = currentAngle - lastAngleRef.current;
    if (deltaAngle > 180) deltaAngle -= 360;
    if (deltaAngle < -180) deltaAngle += 360;
    const deltaTime = deltaAngle / 200;
    const elapsed = Math.max(1, e.timeStamp - lastMoveTimeRef.current);
    const velocity = Math.max(-8, Math.min(8, deltaTime * 1000 / elapsed));
    const previous = releaseVelocityRef.current;
    // A reversal should change direction immediately, not inherit the previous flick.
    const weight = 1 - Math.exp(-elapsed / 24);
    releaseVelocityRef.current = previous * velocity < 0 || elapsed > 80
      ? velocity : previous + (velocity - previous) * weight;
    updateScratch(deck, deltaTime, velocity);
    rotationRef.current = (rotationRef.current + deltaAngle) % 360;
    rotationDotRef.current?.setAttribute('transform', `rotate(${rotationRef.current}, ${computedSize / 2}, ${computedSize / 2})`);
    lastAngleRef.current = currentAngle;
    lastMoveTimeRef.current = e.timeStamp;
  }, [deck, computedSize, updateScratch]);

  const handlePointerUp = useCallback((e: React.PointerEvent<HTMLDivElement>) => {
    if (pointerRef.current !== e.pointerId) return;
    pointerRef.current = null;
    setIsDragging(false);
    isDraggingRef.current = false;
    lastAngleRef.current = null;
    const velocity = e.type === 'pointerup' && e.timeStamp - lastMoveTimeRef.current < 80
      ? releaseVelocityRef.current : 0;
    releaseVelocityRef.current = 0;
    endScratch(deck, velocity, true);
    lastPlaybackPositionRef.current = getDJAudioEngine().getPosition(deck);
    if (e.currentTarget.hasPointerCapture(e.pointerId)) e.currentTarget.releasePointerCapture(e.pointerId);
  }, [deck, endScratch]);

  // Deck identity comes from tokens (Plan §10A.10); no bespoke per-deck hex.
  const accent = deck === 'A' ? 'var(--dj-deck-a-bright)' : 'var(--dj-deck-b-bright)';
  const accentDeep = deck === 'A' ? 'var(--dj-deck-a-deep)' : 'var(--dj-deck-b-deep)';
  const ringRadius = computedSize / 2 - 7;

  // Progress arc ref - updated in RAF
  const progressArcRef = useRef<SVGPathElement>(null);

  // Generate tick marks
  const tickMarks = useMemo(() => {
    const marks = [];
    for (let i = 0; i < 12; i++) {
      const angle = i * 30;
      const isQuarter = angle % 90 === 0;
      marks.push({ angle, length: isQuarter ? 10 : 6, width: isQuarter ? 2 : 1, quarter: isQuarter });
    }
    return marks;
  }, []);

  const c = computedSize / 2;
  return (
    <div
      ref={containerRef}
      className={`dj-jog relative select-none ${isDragging ? 'cursor-grabbing' : scratchAvailability.ready ? 'cursor-grab' : 'cursor-default'}`}
      data-deck={deck}
      data-loaded={!!track}
      style={{ width: computedSize, height: computedSize, touchAction: 'none' }}
      title={scratchAvailability.ready ? 'Drag to scratch · Flick to coast' : scratchAvailability.status}
      aria-label={`Deck ${deck} record: ${scratchAvailability.status}`}
      onPointerDown={handlePointerDown}
      onPointerMove={handlePointerMove}
      onPointerUp={handlePointerUp}
      onPointerCancel={handlePointerUp}
      onLostPointerCapture={handlePointerUp}
    >
      <svg width={computedSize} height={computedSize} viewBox={`0 0 ${computedSize} ${computedSize}`} className="touch-none" aria-hidden="true">
        <defs>
          {/* Dark platter with a subtle inner shadow for depth — no metal texture. */}
          <radialGradient id={`platter-${deck}`} cx="45%" cy="38%" r="70%">
            <stop offset="0%" style={{ stopColor: 'var(--dj-surface-3)' }} />
            <stop offset="70%" style={{ stopColor: 'var(--dj-surface-1)' }} />
            <stop offset="100%" style={{ stopColor: 'var(--dj-bg)' }} />
          </radialGradient>
          <filter id={`textGlow-${deck}`}>
            <feGaussianBlur stdDeviation="1.5" result="blur" />
            <feMerge><feMergeNode in="blur" /><feMergeNode in="SourceGraphic" /></feMerge>
          </filter>
        </defs>

        {/* Beat-pulse glow ring - animated via ref */}
        <circle ref={glowRingRef} cx={c} cy={c} r={computedSize / 2 - 3} fill="none" style={{ stroke: accent, display: 'none' }} strokeWidth="2" opacity="0.3" />

        {/* Outer ring track, then the deck-colored progress ring on top of it. */}
        <circle cx={c} cy={c} r={ringRadius} fill="none" className="dj-jog-ring-track" style={{ stroke: track ? accentDeep : undefined }} strokeWidth="6" />
        <path ref={progressArcRef} d="" fill="none" style={{ stroke: accent, display: 'none' }} strokeWidth="6" strokeLinecap="round" />

        {/* Neutral tick marks */}
        <g>
          {tickMarks.map(({ angle, length, width, quarter }) => (
            <line key={angle} x1={c} y1={16} x2={c} y2={16 + length} className={quarter ? 'dj-jog-tick-major' : 'dj-jog-tick'}
              strokeWidth={width} transform={`rotate(${angle}, ${c}, ${c})`} />
          ))}
        </g>

        {/* Platter and concentric neutral rings */}
        <circle cx={c} cy={c} r={computedSize / 2 - 20} fill={`url(#platter-${deck})`} />
        {[0.86, 0.76, 0.66].map((ratio, i) => (
          <circle key={i} cx={c} cy={c} r={(computedSize / 2 - 20) * ratio} fill="none" className="dj-jog-groove" strokeWidth="1" />
        ))}

        {/* Center display ring in deck color */}
        <circle cx={c} cy={c} r={computedSize * 0.32} className="dj-jog-center" style={{ stroke: track ? accent : undefined }} strokeWidth={track ? 2 : 1} />

        {/* BPM (primary) */}
        <text ref={bpmTextRef} x={c} y={computedSize * 0.42} fontFamily="ui-monospace, 'SF Mono', Consolas, monospace" fontSize={computedSize * 0.15}
          fontWeight="700" className="dj-jog-bpm" textAnchor="middle" dominantBaseline="middle"
          filter={track && isPlaying ? `url(#textGlow-${deck})` : undefined}>
          {bpm > 0 ? bpm.toFixed(1) : '--.-'}
        </text>

        {/* Pitch and range (secondary) */}
        <text ref={tempoTextRef} x={computedSize * 0.41} y={computedSize * 0.55} fontFamily="ui-monospace, 'SF Mono', Consolas, monospace"
          fontSize={computedSize * 0.052} className="dj-jog-secondary" style={{ fill: tempo !== 1 ? accent : undefined }} textAnchor="middle">
          {tempoDisplay}
        </text>
        <text x={computedSize * 0.61} y={computedSize * 0.55} fontFamily="ui-monospace, 'SF Mono', Consolas, monospace"
          fontSize={computedSize * 0.048} className="dj-jog-secondary" textAnchor="middle">±16</text>

        {/* Elapsed time (third level) - updated via ref in RAF loop */}
        <text ref={timeTextRef} x={c} y={computedSize * 0.67} fontFamily="ui-monospace, 'SF Mono', Consolas, monospace"
          fontSize={computedSize * 0.072} fontWeight="600" className="dj-jog-time" textAnchor="middle" dominantBaseline="middle">
          00:00.0
        </text>

        {/* Position marker on the ring - transform updated via ref in RAF loop */}
        <circle ref={rotationDotRef} cx={c} cy={7} r={Math.max(5, computedSize * 0.026)} style={{ fill: accent }}
          className="dj-jog-marker" transform={`rotate(0, ${c}, ${c})`} />
      </svg>

      {track && !scratchAvailability.ready && <div className="dj-jog-status" role="status">
        {scratchAvailability.status}
      </div>}
      {isDragging && (
        <div ref={scratchLabelRef} className="dj-jog-scratch" style={{ backgroundColor: accent }}>
          Scratch
        </div>
      )}
    </div>
  );
};

export default React.memo(DJJogWheel);
