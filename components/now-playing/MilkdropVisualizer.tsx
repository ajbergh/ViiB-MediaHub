/**
 * ViiB MediaHub - Milkdrop Visualizer Component
 * 
 * WebGL-based Milkdrop visualization using the Butterchurn library.
 * Renders classic Winamp-style audio-reactive visualizations with
 * hundreds of preset options.
 * 
 * Features:
 * - Lazy loading of Butterchurn library
 * - Preset loading and smooth blending transitions
 * - Quality/resolution control
 * - Responsive canvas sizing
 * - Automatic cleanup on unmount
 * - WebGL 2 feature detection with graceful fallback
 * 
 * Audio Connection:
 * Uses the audioEngine's master gain node for audio analysis.
 * Butterchurn internally creates an AnalyserNode from the connected audio.
 * 
 * Performance:
 * - Rendering paused when not active
 * - Quality settings allow performance tuning
 * - WebGL context properly disposed on cleanup
 * 
 * @module MilkdropVisualizer
 * @requires butterchurn - WebGL Milkdrop implementation
 * @requires butterchurn-presets - Preset library
 */

import React, { useEffect, useRef, useState, useCallback, useMemo } from 'react';
import { audioEngine } from '../../lib/audio';
import { MilkdropSettings } from '../../types';
import { isMilkdropSupported, MILKDROP_QUALITY_MAP } from '../../lib/milkdropSupport';
import type { ButterchurnVisualizer } from 'butterchurn';

interface MilkdropVisualizerProps {
  /** Current Milkdrop settings */
  settings: MilkdropSettings;
  /** Whether the visualizer is currently visible/active */
  isActive: boolean;
  /** Callback when preset changes (for state sync) */
  onPresetChange?: (presetName: string) => void;
  /** Callback when presets are loaded (provides preset keys) */
  onPresetsLoaded?: (presetKeys: string[]) => void;
  /** Optional class names */
  className?: string;
  /** Optional inline styles (for opacity control) */
  style?: React.CSSProperties;
}

/**
 * MilkdropVisualizer - WebGL-based Milkdrop visualization
 * 
 * Renders audio-reactive visualizations using the Butterchurn library.
 * Falls back to showing an error message if WebGL 2 is not supported.
 */
export const MilkdropVisualizer: React.FC<MilkdropVisualizerProps> = ({
  settings,
  isActive,
  onPresetChange,
  onPresetsLoaded,
  className = '',
  style
}) => {
  const containerRef = useRef<HTMLDivElement>(null);
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const visualizerRef = useRef<ButterchurnVisualizer | null>(null);
  const animationRef = useRef<number>(0);
  const presetsRef = useRef<Record<string, object>>({});
  const presetKeysRef = useRef<string[]>([]);
  
  const [isLoaded, setIsLoaded] = useState(false);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [canvasSize, setCanvasSize] = useState({ width: 512, height: 512 });
  
  // Get resolution from quality setting
  const resolution = useMemo(() => 
    MILKDROP_QUALITY_MAP[settings.quality], 
    [settings.quality]
  );
  
  // Track container size for canvas dimensions
  useEffect(() => {
    const container = containerRef.current;
    if (!container) return;
    
    const updateSize = () => {
      const rect = container.getBoundingClientRect();
      if (rect.width > 0 && rect.height > 0) {
        setCanvasSize({ 
          width: Math.floor(rect.width * (window.devicePixelRatio || 1)), 
          height: Math.floor(rect.height * (window.devicePixelRatio || 1))
        });
      }
    };
    
    // Initial size measurement
    updateSize();
    
    // Re-measure after animation/layout settles (300ms matches the fade-in animation)
    const timeoutId = setTimeout(updateSize, 350);
    
    // Also re-measure on RAF to catch layout changes
    const rafId = requestAnimationFrame(updateSize);
    
    // Watch for resize
    const resizeObserver = new ResizeObserver(updateSize);
    resizeObserver.observe(container);
    
    return () => {
      resizeObserver.disconnect();
      clearTimeout(timeoutId);
      cancelAnimationFrame(rafId);
    };
  }, []);
  
  // Update visualizer size when canvas size changes
  useEffect(() => {
    if (visualizerRef.current && isLoaded) {
      visualizerRef.current.setRendererSize(canvasSize.width, canvasSize.height);
    }
  }, [canvasSize, isLoaded]);
  
  // Load a random preset (avoiding current if possible)
  const loadRandomPreset = useCallback(() => {
    if (!visualizerRef.current || presetKeysRef.current.length === 0) return;
    
    const keys = presetKeysRef.current;
    const currentPreset = settings.currentPreset;
    
    // Try to pick a different preset
    let randomKey: string;
    if (keys.length > 1 && currentPreset) {
      const filteredKeys = keys.filter(k => k !== currentPreset);
      randomKey = filteredKeys[Math.floor(Math.random() * filteredKeys.length)];
    } else {
      randomKey = keys[Math.floor(Math.random() * keys.length)];
    }
    
    visualizerRef.current.loadPreset(
      presetsRef.current[randomKey],
      settings.blendDuration
    );
    onPresetChange?.(randomKey);
  }, [settings.currentPreset, settings.blendDuration, onPresetChange]);
  
  // Initialize Butterchurn
  useEffect(() => {
    if (!isActive || isLoaded || isLoading) return;
    // Wait for container to have dimensions
    if (canvasSize.width <= 0 || canvasSize.height <= 0) return;
    
    const initButterchurn = async () => {
      setIsLoading(true);
      setError(null);
      
      try {
        // Feature detection
        if (!isMilkdropSupported()) {
          setError('Milkdrop requires WebGL 2 which is not supported by your browser');
          setIsLoading(false);
          return;
        }
        
        const canvas = canvasRef.current;
        if (!canvas) {
          setIsLoading(false);
          return;
        }
        
        // Set canvas dimensions explicitly for WebGL
        canvas.width = canvasSize.width;
        canvas.height = canvasSize.height;
        
        // Lazy load Butterchurn
        console.log('[Milkdrop] Loading Butterchurn library...');
        const [butterchurn, butterchurnPresets] = await Promise.all([
          import('butterchurn'),
          import('butterchurn-presets')
        ]);
        
        // Get audio context (ensure audio engine is initialized)
        const audioContext = audioEngine.getAudioContext();
        if (!audioContext) {
          // Try to init audio engine
          audioEngine.init();
          const ctx = audioEngine.getAudioContext();
          if (!ctx) {
            setError('Audio context not available. Please play a track first.');
            setIsLoading(false);
            return;
          }
        }
        
        const ctx = audioEngine.getAudioContext()!;
        
        // Create visualizer using canvas size
        console.log('[Milkdrop] Creating visualizer...', canvasSize);
        visualizerRef.current = butterchurn.default.createVisualizer(
          ctx,
          canvas,
          {
            width: canvasSize.width,
            height: canvasSize.height,
            pixelRatio: 1, // Already accounted for in canvasSize
            textureRatio: 1,
          }
        );
        
        // Connect audio from master gain node
        const audioNode = audioEngine.getMasterGainNode();
        if (audioNode) {
          visualizerRef.current.connectAudio(audioNode);
          console.log('[Milkdrop] Audio connected');
        } else {
          console.warn('[Milkdrop] No audio node available yet');
        }
        
        // Load presets
        presetsRef.current = butterchurnPresets.default.getPresets();
        presetKeysRef.current = Object.keys(presetsRef.current);
        console.log(`[Milkdrop] Loaded ${presetKeysRef.current.length} presets`);
        
        // Notify parent of available presets
        onPresetsLoaded?.(presetKeysRef.current);
        
        // Load initial preset
        if (settings.currentPreset && presetsRef.current[settings.currentPreset]) {
          visualizerRef.current.loadPreset(
            presetsRef.current[settings.currentPreset],
            0 // Instant load for initial preset
          );
        } else {
          // Load random preset
          const randomKey = presetKeysRef.current[
            Math.floor(Math.random() * presetKeysRef.current.length)
          ];
          visualizerRef.current.loadPreset(presetsRef.current[randomKey], 0);
          onPresetChange?.(randomKey);
        }
        
        setIsLoaded(true);
        console.log('[Milkdrop] Initialization complete');
        
      } catch (err) {
        console.error('[Milkdrop] Failed to initialize:', err);
        setError('Failed to load Milkdrop visualizer');
      } finally {
        setIsLoading(false);
      }
    };
    
    initButterchurn();
    
    // Cleanup on unmount or when dependencies change
    return () => {
      if (animationRef.current) {
        cancelAnimationFrame(animationRef.current);
        animationRef.current = 0;
      }
      if (visualizerRef.current) {
        visualizerRef.current.loseGLContext?.();
        visualizerRef.current = null;
      }
      setIsLoaded(false);
    };
  }, [isActive, canvasSize.width, canvasSize.height]); // Re-init when activation or size changes
  
  // Render loop
  useEffect(() => {
    if (!isLoaded || !isActive) {
      if (animationRef.current) {
        cancelAnimationFrame(animationRef.current);
        animationRef.current = 0;
      }
      return;
    }
    
    const render = () => {
      if (visualizerRef.current) {
        visualizerRef.current.render();
      }
      animationRef.current = requestAnimationFrame(render);
    };
    
    render();
    
    return () => {
      if (animationRef.current) {
        cancelAnimationFrame(animationRef.current);
        animationRef.current = 0;
      }
    };
  }, [isLoaded, isActive]);
  
  // Handle preset changes
  useEffect(() => {
    if (!isLoaded || !settings.currentPreset) return;
    
    const preset = presetsRef.current[settings.currentPreset];
    if (preset && visualizerRef.current) {
      visualizerRef.current.loadPreset(preset, settings.blendDuration);
    }
  }, [settings.currentPreset, settings.blendDuration, isLoaded]);
  
  // Handle quality/resolution changes
  useEffect(() => {
    if (!visualizerRef.current || !isLoaded) return;
    
    visualizerRef.current.setRendererSize(resolution.width, resolution.height);
  }, [resolution, isLoaded]);
  
  // Auto-cycle presets
  useEffect(() => {
    if (!isLoaded || !isActive || !settings.presetCycleEnabled) return;
    
    const cycleInterval = setInterval(() => {
      loadRandomPreset();
    }, settings.presetCycleInterval * 1000);
    
    return () => clearInterval(cycleInterval);
  }, [isLoaded, isActive, settings.presetCycleEnabled, settings.presetCycleInterval, loadRandomPreset]);
  
  // Error state
  if (error) {
    return (
      <div className={`absolute inset-0 flex items-center justify-center ${className}`}>
        <div className="text-center p-4">
          <p className="text-status-error text-sm mb-2">{error}</p>
          <p className="text-text-subtle text-xs">
            Try a different visualizer mode
          </p>
        </div>
      </div>
    );
  }
  
  // Always render the container and canvas for consistent refs
  return (
    <div ref={containerRef} className={`absolute inset-0 ${className}`} style={style}>
      {/* Canvas always rendered for WebGL context */}
      <canvas
        ref={canvasRef}
        width={canvasSize.width}
        height={canvasSize.height}
        className="w-full h-full"
        style={{ 
          display: (isLoaded && isActive) ? 'block' : 'none',
        }}
      />
      
      {/* Loading overlay */}
      {isLoading && (
        <div className="absolute inset-0 flex items-center justify-center">
          <div className="text-center">
            <div className="w-8 h-8 border-2 border-brand border-t-transparent rounded-full animate-spin mx-auto mb-2" />
            <p className="text-text-secondary text-sm">Loading Milkdrop...</p>
          </div>
        </div>
      )}
    </div>
  );
};

export default MilkdropVisualizer;
