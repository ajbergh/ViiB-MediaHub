/**
 * ViiB MediaHub - DJ Animation Hook
 * 
 * Custom hook for smooth animations using requestAnimationFrame.
 * Used for jog wheel rotation, waveform scrolling, and other DJ visualizations.
 * 
 * @module components/dj/v2/hooks/useAnimationFrame
 */

import { useRef, useEffect, useCallback } from 'react';

interface UseAnimationFrameOptions {
  /** Target FPS (default: 60) */
  fps?: number;
  /** Whether the animation is currently running */
  isActive?: boolean;
  /** Dependencies that should restart the animation when changed */
  deps?: any[];
}

/**
 * Hook that calls a callback on every animation frame.
 * Provides delta time and elapsed time for smooth animations.
 */
export function useAnimationFrame(
  callback: (deltaTime: number, elapsedTime: number) => void,
  options: UseAnimationFrameOptions = {}
) {
  const { fps = 60, isActive = true } = options;
  
  const requestRef = useRef<number | null>(null);
  const previousTimeRef = useRef<number | null>(null);
  const startTimeRef = useRef<number | null>(null);
  const frameInterval = 1000 / fps;

  const animate = useCallback((currentTime: number) => {
    if (startTimeRef.current === null) {
      startTimeRef.current = currentTime;
    }
    
    if (previousTimeRef.current === null) {
      previousTimeRef.current = currentTime;
    }

    const deltaTime = currentTime - previousTimeRef.current;
    const elapsedTime = currentTime - startTimeRef.current;

    // Throttle to target FPS
    if (deltaTime >= frameInterval) {
      callback(deltaTime / 1000, elapsedTime / 1000); // Convert to seconds
      previousTimeRef.current = currentTime;
    }

    requestRef.current = requestAnimationFrame(animate);
  }, [callback, frameInterval]);

  useEffect(() => {
    if (!isActive) {
      if (requestRef.current) {
        cancelAnimationFrame(requestRef.current);
        requestRef.current = null;
      }
      previousTimeRef.current = null;
      startTimeRef.current = null;
      return;
    }

    requestRef.current = requestAnimationFrame(animate);
    
    return () => {
      if (requestRef.current) {
        cancelAnimationFrame(requestRef.current);
      }
    };
  }, [animate, isActive]);

  // Reset on isActive change
  useEffect(() => {
    if (isActive) {
      previousTimeRef.current = null;
      startTimeRef.current = null;
    }
  }, [isActive]);
}

/**
 * Hook for smooth value interpolation (easing)
 */
export function useSpringValue(
  target: number,
  config: { stiffness?: number; damping?: number } = {}
) {
  const { stiffness = 0.1, damping = 0.8 } = config;
  
  const currentRef = useRef(target);
  const velocityRef = useRef(0);

  useAnimationFrame((deltaTime) => {
    const diff = target - currentRef.current;
    const acceleration = diff * stiffness;
    velocityRef.current = (velocityRef.current + acceleration) * damping;
    currentRef.current += velocityRef.current;
    
    // Snap to target when close enough
    if (Math.abs(diff) < 0.001 && Math.abs(velocityRef.current) < 0.001) {
      currentRef.current = target;
      velocityRef.current = 0;
    }
  }, { isActive: Math.abs(target - currentRef.current) > 0.001 });

  return currentRef.current;
}

/**
 * Hook for BPM-synced pulse animation
 */
export function useBpmPulse(bpm: number, isActive: boolean = true) {
  const pulseRef = useRef(0);
  
  useAnimationFrame((deltaTime) => {
    if (bpm <= 0) return;
    
    // Calculate beats per second
    const beatsPerSecond = bpm / 60;
    // Increment pulse value
    pulseRef.current = (pulseRef.current + deltaTime * beatsPerSecond) % 1;
  }, { isActive: isActive && bpm > 0 });

  // Return pulse value (0-1, resets each beat)
  return pulseRef.current;
}

/**
 * Hook for jog wheel rotation based on playback position
 */
export function useJogWheelRotation(
  position: number,
  bpm: number,
  isPlaying: boolean
) {
  // One full rotation per beat
  if (bpm <= 0 || !isPlaying) return 0;
  
  const beatsElapsed = (position / 60) * bpm;
  return (beatsElapsed * 360) % 360;
}

export default useAnimationFrame;
