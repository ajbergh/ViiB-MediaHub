/**
 * WebGLVisualizer Component
 * 
 * React component for WebGL-based audio visualization.
 * Provides automatic fallback to Canvas 2D if WebGL is unavailable.
 * 
 * Features:
 * - WebGL2 with WebGL1 fallback
 * - Graceful degradation to Canvas 2D
 * - Fade in/out transitions
 * - Audio-reactive rendering
 * - All 10 visualization modes
 * 
 * @module WebGLVisualizer
 */

import React, { useCallback, useState } from 'react';
import { useWebGLVisualizer } from './useWebGLVisualizer';
import { VisualizerMode } from '../../../types';
import { VIIB_COLOR_VALUES } from '../../ui/tokens';

// Lazy import Canvas 2D fallback
const AlbumArtVisualizer = React.lazy(() => import('../AlbumArtVisualizer'));

interface WebGLVisualizerProps {
    /** Current visualization mode */
    mode: VisualizerMode;
    /** Whether the visualizer is currently visible/active */
    isActive: boolean;
    /** Primary color for visualization elements */
    color?: string;
    /** Secondary/accent color for gradients */
    accentColor?: string;
    /** Callback when fade transition completes */
    onFadeComplete?: (visible: boolean) => void;
    /** Optional class names */
    className?: string;
    /** Force Canvas 2D fallback (for testing) */
    forceCanvas2D?: boolean;
}

/**
 * Converts CSS color string to RGB array
 */
function colorToRGB(color: string): [number, number, number] {
    // Handle hex colors
    if (color.startsWith('#')) {
        const hex = color.slice(1);
        const bigint = parseInt(hex, 16);
        const r = ((bigint >> 16) & 255) / 255;
        const g = ((bigint >> 8) & 255) / 255;
        const b = (bigint & 255) / 255;
        return [r, g, b];
    }
    
    // Handle rgb/rgba
    const match = color.match(/rgba?\((\d+),\s*(\d+),\s*(\d+)/);
    if (match) {
        return [
            parseInt(match[1]) / 255,
            parseInt(match[2]) / 255,
            parseInt(match[3]) / 255
        ];
    }
    
    // Default to white
    return [1, 1, 1];
}

/**
 * WebGL Visualizer with Canvas 2D fallback
 */
export const WebGLVisualizer: React.FC<WebGLVisualizerProps> = ({
    mode,
    isActive,
    color = VIIB_COLOR_VALUES.playbackGreen,
    accentColor = VIIB_COLOR_VALUES.brandPurple,
    onFadeComplete,
    className = '',
    forceCanvas2D = false
}) => {
    const [useFallback, setUseFallback] = useState(forceCanvas2D);
    
    const handleFallback = useCallback(() => {
        console.log('[WebGLVisualizer] Falling back to Canvas 2D');
        setUseFallback(true);
    }, []);
    
    const { canvasRef, isSupported, opacity } = useWebGLVisualizer({
        mode,
        isActive,
        color: colorToRGB(color),
        accentColor: colorToRGB(accentColor),
        enableBloom: true,
        bloomIntensity: 1.2,
        onFallback: handleFallback,
        onFadeComplete
    });
    
    // Use Canvas 2D fallback if WebGL not supported or forced
    if (useFallback || !isSupported) {
        return (
            <React.Suspense fallback={null}>
                <AlbumArtVisualizer
                    mode={mode}
                    isActive={isActive}
                    color={color}
                    accentColor={accentColor}
                    onFadeComplete={onFadeComplete}
                    className={className}
                />
            </React.Suspense>
        );
    }
    
    // Always render canvas - the hook handles visibility via isActive
    // Parent controls visibility via wrapper div opacity
    return (
        <canvas
            ref={canvasRef}
            className={`absolute inset-0 w-full h-full z-10 pointer-events-none ${className}`}
        />
    );
};

export default WebGLVisualizer;
