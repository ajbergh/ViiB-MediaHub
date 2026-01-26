/**
 * ViiB MediaHub - Milkdrop Preset Cycle Hook
 * 
 * Custom hook to automatically cycle through Milkdrop presets at a configurable interval.
 * Avoids repeating the same preset consecutively when possible.
 * 
 * Features:
 * - Configurable cycle interval
 * - Enabled/disabled toggle
 * - Avoids immediate preset repetition
 * - Proper cleanup on unmount
 * 
 * @module useMilkdropCycle
 */

import { useEffect, useRef, useCallback } from 'react';

interface UseMilkdropCycleOptions {
  /** Whether preset cycling is enabled */
  enabled: boolean;
  /** Seconds between preset changes */
  interval: number;
  /** Available preset keys to cycle through */
  presets: string[];
  /** Currently active preset (to avoid immediate repeat) */
  currentPreset: string | null;
  /** Callback when a new preset should be loaded */
  onPresetChange: (preset: string) => void;
}

/**
 * Hook to auto-cycle through Milkdrop presets
 * 
 * @example
 * ```tsx
 * useMilkdropCycle({
 *   enabled: settings.presetCycleEnabled,
 *   interval: settings.presetCycleInterval,
 *   presets: presetKeys,
 *   currentPreset: settings.currentPreset,
 *   onPresetChange: setMilkdropPreset
 * });
 * ```
 */
export function useMilkdropCycle({
  enabled,
  interval,
  presets,
  currentPreset,
  onPresetChange
}: UseMilkdropCycleOptions): void {
  // Track recent presets to avoid immediate repetition
  const recentPresetsRef = useRef<string[]>([]);
  
  // Get a random preset, avoiding recent ones if possible
  const getRandomPreset = useCallback(() => {
    if (presets.length === 0) return null;
    
    // If only one preset, return it
    if (presets.length === 1) return presets[0];
    
    // Filter out recent presets
    const recentCount = Math.min(3, Math.floor(presets.length / 3));
    const availablePresets = presets.filter(
      p => !recentPresetsRef.current.slice(-recentCount).includes(p)
    );
    
    // If all presets are recent, use full list
    const candidates = availablePresets.length > 0 ? availablePresets : presets;
    
    // Pick random from candidates
    const randomIndex = Math.floor(Math.random() * candidates.length);
    return candidates[randomIndex];
  }, [presets]);
  
  // Cycle to next preset
  const cyclePreset = useCallback(() => {
    const nextPreset = getRandomPreset();
    if (nextPreset) {
      // Track this preset as recent
      recentPresetsRef.current = [...recentPresetsRef.current.slice(-5), nextPreset];
      onPresetChange(nextPreset);
    }
  }, [getRandomPreset, onPresetChange]);
  
  // Set up interval
  useEffect(() => {
    if (!enabled || presets.length === 0 || interval <= 0) {
      return;
    }
    
    const timer = setInterval(cyclePreset, interval * 1000);
    
    return () => {
      clearInterval(timer);
    };
  }, [enabled, interval, presets.length, cyclePreset]);
  
  // Track current preset in recent list when it changes externally
  useEffect(() => {
    if (currentPreset && !recentPresetsRef.current.includes(currentPreset)) {
      recentPresetsRef.current = [...recentPresetsRef.current.slice(-5), currentPreset];
    }
  }, [currentPreset]);
}

export default useMilkdropCycle;
