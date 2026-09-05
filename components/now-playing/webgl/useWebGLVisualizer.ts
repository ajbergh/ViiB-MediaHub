/**
 * useWebGLVisualizer
 * 
 * React hook for managing WebGL visualizer lifecycle.
 * Handles initialization, rendering loop, and cleanup.
 * 
 * Usage:
 * ```tsx
 * const { canvasRef, isSupported, error } = useWebGLVisualizer({
 *   mode: 'WAVE',
 *   isActive: true,
 *   color: [0.235, 0.812, 0.467]
 * });
 * 
 * return <canvas ref={canvasRef} className="..." />;
 * ```
 */

import { useRef, useEffect, useState, useCallback } from 'react';
import { WebGLVisualizerRenderer, WebGLVisualizerOptions } from './index';
import { VisualizerMode } from '../../../types';
import { audioEngine } from '../../../lib/audio';
import { shouldUseAdvancedWebGL } from '../../../lib/webglSafety';

interface UseWebGLVisualizerOptions extends WebGLVisualizerOptions {
    /** Current visualization mode */
    mode: VisualizerMode;
    /** Whether the visualizer is active/visible */
    isActive: boolean;
    /** Callback when WebGL initialization fails */
    onFallback?: () => void;
    /** Callback when fade transition completes */
    onFadeComplete?: (visible: boolean) => void;
}

interface UseWebGLVisualizerResult {
    /** Ref to attach to canvas element */
    canvasRef: React.RefObject<HTMLCanvasElement>;
    /** Whether WebGL is supported */
    isSupported: boolean;
    /** Any error that occurred during initialization */
    error: Error | null;
    /** Current opacity (for transitions) */
    opacity: number;
}

export function useWebGLVisualizer(options: UseWebGLVisualizerOptions): UseWebGLVisualizerResult {
    const {
        mode,
        isActive,
        color,
        accentColor,
        enableBloom,
        bloomIntensity,
        onFallback,
        onFadeComplete
    } = options;
    
    const canvasRef = useRef<HTMLCanvasElement>(null);
    const rendererRef = useRef<WebGLVisualizerRenderer | null>(null);
    const animationRef = useRef<number>(0);
    const opacityRef = useRef<number>(0);
    
    const [isSupported, setIsSupported] = useState(true);
    const [error, setError] = useState<Error | null>(null);
    const [opacity, setOpacity] = useState(0);
    
    // Fade parameters
    const FADE_DURATION = 400;
    const fadeStartTimeRef = useRef<number>(0);
    const fadingInRef = useRef<boolean>(false);
    const fadingOutRef = useRef<boolean>(false);
    
    // Audio data buffers
    const frequencyData = useRef(new Uint8Array(1024));
    const waveformData = useRef(new Uint8Array(1024));
    
    /**
     * Updates opacity during fade transitions
     */
    const updateOpacity = useCallback((timestamp: number): boolean => {
        if (!fadeStartTimeRef.current) {
            fadeStartTimeRef.current = timestamp;
        }
        
        const elapsed = timestamp - fadeStartTimeRef.current;
        const progress = Math.min(elapsed / FADE_DURATION, 1);
        
        // Ease in-out curve
        const eased = progress < 0.5
            ? 2 * progress * progress
            : 1 - Math.pow(-2 * progress + 2, 2) / 2;
        
        if (fadingInRef.current) {
            opacityRef.current = eased;
        } else if (fadingOutRef.current) {
            opacityRef.current = 1 - eased;
        }
        
        setOpacity(opacityRef.current);
        
        if (progress >= 1) {
            fadeStartTimeRef.current = 0;
            if (fadingInRef.current) {
                fadingInRef.current = false;
                onFadeComplete?.(true);
            } else if (fadingOutRef.current) {
                fadingOutRef.current = false;
                opacityRef.current = 0;
                setOpacity(0);
                onFadeComplete?.(false);
            }
            return true;
        }
        
        return false;
    }, [onFadeComplete]);
    
    /**
     * Initialize renderer on mount
     */
    useEffect(() => {
        const canvas = canvasRef.current;
        if (!canvas) return;

        // WKWebView WebGL failures take down the native Wails window rather
        // than merely this React tree. Use the Canvas renderer on macOS Wails.
        if (!shouldUseAdvancedWebGL()) {
            setIsSupported(false);
            setError(new Error('WebGL is disabled in the macOS desktop app'));
            onFallback?.();
            return;
        }
        
        // Check WebGL support
        if (!WebGLVisualizerRenderer.isSupported()) {
            setIsSupported(false);
            setError(new Error('WebGL not supported'));
            onFallback?.();
            return;
        }
        
        // Create renderer
        const renderer = new WebGLVisualizerRenderer({
            color,
            accentColor,
            enableBloom,
            bloomIntensity
        });
        
        if (!renderer.init(canvas)) {
            setIsSupported(false);
            setError(new Error('WebGL initialization failed'));
            onFallback?.();
            return;
        }
        
        rendererRef.current = renderer;
        
        // Handle resize
        const updateSize = () => {
            const width = canvas.clientWidth;
            const height = canvas.clientHeight;
            if (width > 0 && height > 0) {
                renderer.resize(width, height);
            }
        };
        
        updateSize();
        
        const resizeObserver = new ResizeObserver(updateSize);
        resizeObserver.observe(canvas);
        
        return () => {
            resizeObserver.disconnect();
            if (animationRef.current) {
                cancelAnimationFrame(animationRef.current);
            }
            renderer.dispose();
            rendererRef.current = null;
        };
    }, []); // Only run on mount
    
    /**
     * Update renderer options when they change
     */
    useEffect(() => {
        if (rendererRef.current) {
            rendererRef.current.setOptions({
                color,
                accentColor,
                enableBloom,
                bloomIntensity
            });
        }
    }, [color, accentColor, enableBloom, bloomIntensity]);
    
    /**
     * Handle active state changes (fade in/out)
     */
    useEffect(() => {
        if (isActive && mode !== 'OFF' && mode !== 'MILKDROP') {
            fadingInRef.current = true;
            fadingOutRef.current = false;
            fadeStartTimeRef.current = 0;
        } else {
            if (opacityRef.current > 0) {
                fadingOutRef.current = true;
                fadingInRef.current = false;
                fadeStartTimeRef.current = 0;
            }
        }
    }, [isActive, mode]);
    
    /**
     * Animation loop
     */
    useEffect(() => {
        if (!isSupported || !rendererRef.current) return;
        
        const renderer = rendererRef.current;
        
        const draw = (timestamp: number) => {
            animationRef.current = requestAnimationFrame(draw);
            
            // Update fade
            updateOpacity(timestamp);
            
            // Only render if active and visible mode
            if (!isActive || mode === 'OFF' || mode === 'MILKDROP') {
                return;
            }
            
            // Get audio data
            const analyser = audioEngine.getAnalyser();
            if (analyser) {
                analyser.getByteFrequencyData(frequencyData.current);
                analyser.getByteTimeDomainData(waveformData.current);
                renderer.setAudioData(frequencyData.current, waveformData.current);
            }
            
            // Render frame
            renderer.render(mode, timestamp);
        };
        
        animationRef.current = requestAnimationFrame(draw);
        
        return () => {
            if (animationRef.current) {
                cancelAnimationFrame(animationRef.current);
            }
        };
    }, [mode, isActive, isSupported, updateOpacity]);
    
    return {
        canvasRef,
        isSupported,
        error,
        opacity
    };
}
