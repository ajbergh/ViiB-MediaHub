/**
 * ViiB MediaHub - DJ Visual Effects Hook
 * 
 * Custom hook for DJ visual effects including:
 * - Button press feedback
 * - BPM-synced glow effects
 * - VU meter level calculations
 * 
 * @module components/dj/v2/hooks/useDJEffects
 */

import { useState, useCallback, useRef, useEffect } from 'react';

/**
 * Hook for button press animation with auto-release
 */
export function useButtonPress(duration: number = 150) {
  const [isPressed, setIsPressed] = useState(false);
  const timeoutRef = useRef<NodeJS.Timeout | null>(null);

  const press = useCallback(() => {
    setIsPressed(true);
    
    if (timeoutRef.current) {
      clearTimeout(timeoutRef.current);
    }
    
    timeoutRef.current = setTimeout(() => {
      setIsPressed(false);
    }, duration);
  }, [duration]);

  const release = useCallback(() => {
    if (timeoutRef.current) {
      clearTimeout(timeoutRef.current);
    }
    setIsPressed(false);
  }, []);

  // Cleanup on unmount
  useEffect(() => {
    return () => {
      if (timeoutRef.current) {
        clearTimeout(timeoutRef.current);
      }
    };
  }, []);

  return { isPressed, press, release };
}

/**
 * Calculate VU meter level from audio data (0-1 normalized)
 */
export function calculateVULevel(
  audioData: Float32Array | null,
  smoothingFactor: number = 0.8
): number {
  if (!audioData || audioData.length === 0) return 0;

  // Calculate RMS (Root Mean Square) for volume level
  let sum = 0;
  for (let i = 0; i < audioData.length; i++) {
    sum += audioData[i] * audioData[i];
  }
  const rms = Math.sqrt(sum / audioData.length);

  // Convert to 0-1 range with some headroom
  return Math.min(1, rms * 2);
}

/**
 * Hook for tracking VU meter levels with smoothing
 */
export function useVUMeter(audioLevel: number, smoothingFactor: number = 0.9) {
  const [displayLevel, setDisplayLevel] = useState(0);
  const targetRef = useRef(audioLevel);

  useEffect(() => {
    targetRef.current = audioLevel;
  }, [audioLevel]);

  useEffect(() => {
    let animationFrame: number;

    const animate = () => {
      setDisplayLevel((prev) => {
        const newLevel = prev * smoothingFactor + targetRef.current * (1 - smoothingFactor);
        // Apply decay when level drops
        if (targetRef.current < prev) {
          return prev * 0.95; // Faster decay
        }
        return newLevel;
      });
      animationFrame = requestAnimationFrame(animate);
    };

    animationFrame = requestAnimationFrame(animate);
    return () => cancelAnimationFrame(animationFrame);
  }, [smoothingFactor]);

  return displayLevel;
}

/**
 * Hook for BPM pulse glow effect
 * Returns opacity value (0-1) that pulses on beat
 */
export function useBpmGlow(bpm: number, isActive: boolean = true): number {
  const [glowIntensity, setGlowIntensity] = useState(0);
  
  useEffect(() => {
    if (!isActive || bpm <= 0) {
      setGlowIntensity(0);
      return;
    }

    const beatDuration = 60000 / bpm; // ms per beat
    let animationFrame: number;
    let lastBeatTime = performance.now();

    const animate = (currentTime: number) => {
      const elapsed = currentTime - lastBeatTime;
      const beatProgress = (elapsed % beatDuration) / beatDuration;
      
      // Quick attack, slow decay
      const intensity = beatProgress < 0.1 
        ? beatProgress * 10 // Quick rise
        : 1 - (beatProgress - 0.1) * 1.1; // Slow decay

      setGlowIntensity(Math.max(0, intensity));
      animationFrame = requestAnimationFrame(animate);
    };

    animationFrame = requestAnimationFrame(animate);
    return () => cancelAnimationFrame(animationFrame);
  }, [bpm, isActive]);

  return glowIntensity;
}

/**
 * Generate CSS box-shadow string for glow effects
 */
export function getGlowShadow(
  color: string,
  intensity: number,
  spread: number = 8
): string {
  const alpha = Math.min(1, intensity);
  return `0 0 ${spread * intensity}px ${color}${Math.round(alpha * 255).toString(16).padStart(2, '0')}`;
}

/**
 * Hook for tracking deck activity (for glow effects)
 */
export function useDeckActivity(isPlaying: boolean, bpm: number) {
  const glow = useBpmGlow(bpm, isPlaying);
  
  return {
    glowIntensity: glow,
    isActive: isPlaying,
    pulseOpacity: 0.3 + glow * 0.7, // Minimum 0.3, max 1.0
  };
}

export default useButtonPress;
