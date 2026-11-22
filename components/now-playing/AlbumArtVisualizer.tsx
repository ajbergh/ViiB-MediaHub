/**
 * Canvas 2D audio-reactive overlay for Now Playing album artwork.
 * Renders ten active modes: WAVE, SPECTRUM, FLAME_SPECTRUM, STARDUST_HALO,
 * AURORA_RIBBON, ELECTRIC_ARC, GRASS_OSCILLOSCOPE, FIREFLY_FIELD,
 * TUNNEL_WAVEFORM, and WIND_FIELD. OFF and MILKDROP are handled separately.
 *
 * Reads analyser frequency/time-domain data, manages particle state and fades,
 * sizes the canvas with ResizeObserver/device pixel ratio, and cleans up its
 * requestAnimationFrame loop. The host determines the actual frame rate.
 *
 * @module AlbumArtVisualizer
 */


import React, { useEffect, useRef, useState, useCallback } from 'react';
import { audioEngine } from '../../lib/audio';
import { VisualizerMode } from '../../types';
import { CanvasPostProcessor } from './CanvasPostProcessor';
import { drawVisualizerScene } from './visualizerScenes';
import { rgbaFromRgb, VIIB_COLOR_RGB, VIIB_COLOR_VALUES } from '../ui/tokens';

interface AlbumArtVisualizerProps {
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
}

/**
 * Album Art Visualizer - Fills album art area with audio-reactive visuals
 * 
 * Composites the audio-reactive scenes and existing spectrum and firefly modes.
 */
export const AlbumArtVisualizer: React.FC<AlbumArtVisualizerProps> = ({
    mode,
    isActive,
    color = VIIB_COLOR_VALUES.playbackGreen,
    accentColor = VIIB_COLOR_VALUES.brandPurple,
    onFadeComplete,
    className = ''
}) => {
    const canvasRef = useRef<HTMLCanvasElement>(null);
    const animationRef = useRef<number>(0);
    const opacityRef = useRef<number>(0);
    const processorRef = useRef<CanvasPostProcessor | null>(null);
    const [isRendering, setIsRendering] = useState(false);
    
    // Fade parameters
    const FADE_DURATION = 400; // ms
    const fadeStartTimeRef = useRef<number>(0);
    const fadingInRef = useRef<boolean>(false);
    const fadingOutRef = useRef<boolean>(false);
    const drawsScene = mode !== 'OFF' && mode !== 'MILKDROP' && mode !== 'SPECTRUM' && mode !== 'FIREFLY_FIELD';

    /**
     * Interpolates opacity during fade transitions using easeInOut curve
     */
    const updateOpacity = useCallback((timestamp: number): boolean => {
        if (!fadeStartTimeRef.current) {
            fadeStartTimeRef.current = timestamp;
        }
        
        const elapsed = timestamp - fadeStartTimeRef.current;
        const progress = Math.min(elapsed / FADE_DURATION, 1);
        
        // Ease in-out curve for smooth transition
        const eased = progress < 0.5 
            ? 2 * progress * progress 
            : 1 - Math.pow(-2 * progress + 2, 2) / 2;
        
        if (fadingInRef.current) {
            opacityRef.current = eased;
        } else if (fadingOutRef.current) {
            opacityRef.current = 1 - eased;
        }
        
        // Return true if fade is complete
        if (progress >= 1) {
            fadeStartTimeRef.current = 0;
            if (fadingInRef.current) {
                fadingInRef.current = false;
                onFadeComplete?.(true);
            } else if (fadingOutRef.current) {
                fadingOutRef.current = false;
                opacityRef.current = 0;
                onFadeComplete?.(false);
            }
            return true;
        }
        
        return false;
    }, [onFadeComplete]);

    /**
     * Draws circular spectrum visualization
     * Bars radiate outward from center, creating a sun-burst effect
     */
    const drawCircularSpectrum = useCallback((
        ctx: CanvasRenderingContext2D, 
        dataArray: Uint8Array, 
        width: number, 
        height: number
    ) => {
        const centerX = width / 2;
        const centerY = height / 2;
        const radius = Math.min(width, height) * 0.25;
        const maxBarHeight = Math.min(width, height) * 0.2;
        const numBars = 64;
        
        // Only use first portion of frequency data for visible bars
        const effectiveLength = Math.floor(dataArray.length * 0.5);
        const step = Math.floor(effectiveLength / numBars);
        
        ctx.save();
        ctx.globalAlpha = opacityRef.current;
        
        for (let i = 0; i < numBars; i++) {
            // Average the frequency bins for this bar
            let sum = 0;
            for (let j = 0; j < step; j++) {
                const index = i * step + j;
                if (index < dataArray.length) sum += dataArray[index];
            }
            const value = sum / step / 255;
            
            const angle = (i / numBars) * Math.PI * 2 - Math.PI / 2;
            const barHeight = value * maxBarHeight + 2;
            
            const x1 = centerX + Math.cos(angle) * radius;
            const y1 = centerY + Math.sin(angle) * radius;
            const x2 = centerX + Math.cos(angle) * (radius + barHeight);
            const y2 = centerY + Math.sin(angle) * (radius + barHeight);
            
            // Color gradient based on position
            const hue = (i / numBars) * 60 + 120; // Green to cyan range
            const colorStr = `hsl(${hue}, 80%, 60%)`;
            
            // OPTIMIZATION: Add dynamic glow for cool factor
            ctx.shadowColor = colorStr;
            ctx.shadowBlur = 10 + value * 15; // Dynamic glow based on amplitude
            ctx.strokeStyle = `hsla(${hue}, 80%, 60%, ${0.6 + value * 0.4})`;
            ctx.lineWidth = 3;
            ctx.lineCap = 'round';
            
            ctx.beginPath();
            ctx.moveTo(x1, y1);
            ctx.lineTo(x2, y2);
            ctx.stroke();
        }
        
        // Reset shadow for inner circle
        ctx.shadowBlur = 0;
        
        // Inner circle glow
        const gradient = ctx.createRadialGradient(centerX, centerY, radius * 0.8, centerX, centerY, radius);
        gradient.addColorStop(0, rgbaFromRgb(VIIB_COLOR_RGB.visualizerGreen, 0.1));
        gradient.addColorStop(1, rgbaFromRgb(VIIB_COLOR_RGB.visualizerGreen, 0));
        ctx.fillStyle = gradient;
        ctx.beginPath();
        ctx.arc(centerX, centerY, radius, 0, Math.PI * 2);
        ctx.fill();
        
        ctx.restore();
    }, []);

    /**
     * Draws spectrum bars from bottom
     */
    const drawSpectrumBars = useCallback((
        ctx: CanvasRenderingContext2D,
        dataArray: Uint8Array,
        width: number,
        height: number
    ) => {
        ctx.save();
        ctx.globalAlpha = opacityRef.current;
        
        const gap = 3;
        const barWidth = 4;
        const totalBarWidth = barWidth + gap;
        const numBars = Math.floor(width / totalBarWidth);
        const effectiveLength = Math.floor(dataArray.length * 0.6);
        const step = Math.max(1, Math.floor(effectiveLength / numBars));
        
        let x = (width - numBars * totalBarWidth) / 2; // Center bars
        
        for (let i = 0; i < numBars; i++) {
            let sum = 0;
            for (let j = 0; j < step; j++) {
                const index = i * step + j;
                if (index < dataArray.length) sum += dataArray[index];
            }
            
            const value = (sum / step) / 255;
            const barHeight = Math.max(2, value * height * 0.8);
            
            // Gradient color based on height
            const gradient = ctx.createLinearGradient(x, height, x, height - barHeight);
            gradient.addColorStop(0, color);
            gradient.addColorStop(0.5, accentColor);
            gradient.addColorStop(1, 'white');
            
            ctx.fillStyle = gradient;
            
            // Rounded top
            ctx.beginPath();
            ctx.moveTo(x, height);
            ctx.lineTo(x, height - barHeight + 2);
            ctx.quadraticCurveTo(x, height - barHeight, x + barWidth / 2, height - barHeight);
            ctx.quadraticCurveTo(x + barWidth, height - barHeight, x + barWidth, height - barHeight + 2);
            ctx.lineTo(x + barWidth, height);
            ctx.closePath();
            ctx.fill();
            
            x += totalBarWidth;
        }
        
        ctx.restore();
    }, [color, accentColor]);

    /**
     * 🌟 Holiday Firefly Field
     * Drifting firefly particles with gentle glow
     */
    const fireflyParticlesRef = useRef<Array<{
        x: number; y: number; vx: number; vy: number; brightness: number; phase: number;
    }>>([]);
    
    const drawFireflyField = useCallback((
        ctx: CanvasRenderingContext2D,
        frequencyData: Uint8Array,
        width: number,
        height: number,
        time: number
    ) => {
        let mid = 0, treble = 0;
        for (let i = 30; i < 150; i++) mid += frequencyData[i] || 0;
        for (let i = 150; i < 300; i++) treble += frequencyData[i] || 0;
        mid = (mid / 120) / 255;
        treble = (treble / 150) / 255;
        
        ctx.save();
        ctx.globalAlpha = opacityRef.current;
        
        const fireflies = fireflyParticlesRef.current;
        
        // Maintain firefly count
        while (fireflies.length < 40) {
            fireflies.push({
                x: Math.random() * width,
                y: Math.random() * height,
                vx: (Math.random() - 0.5) * 0.5,
                vy: (Math.random() - 0.5) * 0.5,
                brightness: 0.5 + Math.random() * 0.5,
                phase: Math.random() * Math.PI * 2
            });
        }
        
        // Update and draw fireflies
        for (const fly of fireflies) {
            // Slow drift movement
            fly.x += fly.vx;
            fly.y += fly.vy;
            fly.phase += 0.02;
            
            // Wrap around edges
            if (fly.x < -20) fly.x = width + 20;
            if (fly.x > width + 20) fly.x = -20;
            if (fly.y < -20) fly.y = height + 20;
            if (fly.y > height + 20) fly.y = -20;
            
            // Flicker based on mids/highs
            const flicker = Math.sin(fly.phase) * 0.5 + 0.5;
            const intensity = fly.brightness * (0.5 + mid * 0.3) * (0.7 + flicker * 0.3);
            
            // Occasional bright flash on treble peaks
            const flash = treble > 0.5 && Math.random() < 0.1 ? 1 : 0;
            
            // Warm glow
            const glowSize = 8 + intensity * 12 + flash * 10;
            const glowGradient = ctx.createRadialGradient(fly.x, fly.y, 0, fly.x, fly.y, glowSize);
            glowGradient.addColorStop(0, `rgba(255, 230, 150, ${intensity + flash * 0.5})`);
            glowGradient.addColorStop(0.4, `rgba(255, 200, 100, ${intensity * 0.5 + flash * 0.3})`);
            glowGradient.addColorStop(1, 'rgba(255, 180, 80, 0)');
            
            ctx.fillStyle = glowGradient;
            ctx.beginPath();
            ctx.arc(fly.x, fly.y, glowSize, 0, Math.PI * 2);
            ctx.fill();
            
            // Core bright point
            ctx.fillStyle = `rgba(255, 255, 200, ${intensity * 0.9 + flash})`;
            ctx.beginPath();
            ctx.arc(fly.x, fly.y, 2, 0, Math.PI * 2);
            ctx.fill();
        }
        
        ctx.restore();
    }, []);

    /**
     * Main animation loop
     */
    useEffect(() => {
        const canvas = canvasRef.current;
        if (!canvas) return;
        
        const ctx = canvas.getContext('2d', { alpha: drawsScene });
        if (!ctx) return;

        // Initialize processor if needed
        if (!processorRef.current) {
            processorRef.current = new CanvasPostProcessor();
        }
        const processor = processorRef.current;

        // Handle canvas sizing
        const updateSize = () => {
            const dpr = drawsScene ? Math.min(window.devicePixelRatio || 1, 1.5) : window.devicePixelRatio || 1;
            const width = canvas.clientWidth;
            const height = canvas.clientHeight;
            
            if (canvas.width !== width * dpr || canvas.height !== height * dpr) {
                canvas.width = width * dpr;
                canvas.height = height * dpr;
                // Processor handles scaling internally
                processor.resize(width, height, dpr);
            }
        };
        
        updateSize();
        
        const resizeObserver = new ResizeObserver(updateSize);
        resizeObserver.observe(canvas);
        
        // Frequency data buffer
        const bufferLength = 1024;
        const frequencyData = new Uint8Array(bufferLength);
        const waveformData = new Uint8Array(bufferLength);
        
        // Start rendering when active
        if (isActive && mode !== 'OFF') {
            setIsRendering(true);
            fadingInRef.current = true;
            fadingOutRef.current = false;
            fadeStartTimeRef.current = 0;
        } else if (!isActive || mode === 'OFF') {
            if (isRendering && opacityRef.current > 0) {
                fadingOutRef.current = true;
                fadingInRef.current = false;
                fadeStartTimeRef.current = 0;
            }
        }
        
        const draw = (timestamp: number) => {
            animationRef.current = requestAnimationFrame(draw);
            
            const width = canvas.clientWidth;
            const height = canvas.clientHeight;
            
            // Update fade transitions
            updateOpacity(timestamp);
            if (drawsScene) canvas.style.opacity = String(opacityRef.current);
            
            // Stop rendering if faded out completely
            if (opacityRef.current <= 0 && !fadingInRef.current) {
                ctx.clearRect(0, 0, canvas.width, canvas.height);
                if (fadingOutRef.current) {
                    fadingOutRef.current = false;
                    setIsRendering(false);
                }
                return;
            }
            
            // Use processor context for drawing
            const drawCtx = processor.getDrawContext();
            
            // Clear or fade for trails
            if (mode === 'FIREFLY_FIELD') {
                processor.fade(0.2); // Short trails
            } else {
                processor.clear();
            }
            
            const analyser = audioEngine.getAnalyser();
            if (!analyser) return;
            
            // Get frequency and waveform data
            analyser.getByteFrequencyData(frequencyData);
            analyser.getByteTimeDomainData(waveformData);
            
            switch (mode) {
                case 'OFF':
                case 'MILKDROP':
                    drawSpectrumBars(drawCtx, frequencyData, width, height);
                    break;
                case 'SPECTRUM':
                    drawCircularSpectrum(drawCtx, frequencyData, width, height);
                    break;
                case 'FIREFLY_FIELD':
                    drawFireflyField(drawCtx, frequencyData, width, height, timestamp);
                    break;
                default:
                    drawVisualizerScene(drawCtx, mode, width, height, timestamp, frequencyData, waveformData);
            }

            // Render to main canvas with bloom
            processor.render(ctx, {
                enableBloom: mode !== 'OFF',
                bloomIntensity: 1.2,
                bloomRadius: 20
            });
        };
        
        if (isRendering || isActive) {
            animationRef.current = requestAnimationFrame(draw);
        }
        
        return () => {
            if (animationRef.current) {
                cancelAnimationFrame(animationRef.current);
            }
            resizeObserver.disconnect();
        };
    }, [
        mode, drawsScene, isActive, isRendering, updateOpacity, drawCircularSpectrum, drawSpectrumBars, drawFireflyField
    ]);

    // Don't render canvas if completely hidden and not fading
    if (!isRendering && !isActive && opacityRef.current <= 0) {
        return null;
    }

    return (
        <canvas
            key={drawsScene ? 'transparent' : 'legacy'}
            ref={canvasRef}
            className={`absolute inset-0 w-full h-full z-10 pointer-events-none transition-opacity duration-300 ${className}`}
            style={{ opacity: opacityRef.current }}
        />
    );
};

export default AlbumArtVisualizer;
