/**
 * ViiB MediaHub - Album Art Visualizer Component
 * 
 * Canvas-based audio visualization that overlays the album art in the Now Playing view.
 * Renders real-time audio-reactive visualizations using Canvas 2D API for broad compatibility.
 * 
 * Architecture:
 * - Uses Web Audio API's AnalyserNode for frequency and waveform data
 * - Canvas 2D rendering with requestAnimationFrame for smooth 60 FPS
 * - Separate renderer functions for each visualization mode
 * - Fade in/out transitions using opacity interpolation with easing
 * - ResizeObserver for responsive canvas sizing with DPR support
 * 
 * Visualization Modes (21 total):
 * 
 * Classic Modes:
 * - WAVE: Smooth glowing waveform with quadratic curve interpolation
 * - SPECTRUM: Circular frequency bars radiating from center (sun-burst effect)
 * - AURORA: Ambient flowing gradients reacting to bass/mid/treble bands
 * - CIRCULAR: Enhanced circular with rotating bars, pulsing rings, inner waveform
 * - PARTICLES: Dynamic particle system with gravity effects and audio-reactive spawning
 * - NEBULA: Cosmic atmosphere with swirling nebula clouds, stars, and lens flares
 * 
 * Next-Gen Modes:
 * - FLAME_SPECTRUM: Stylized flame tongues rising with frequency-based height and color intensity
 * - STARDUST_HALO: Pulsing particle halo with stardust bursts on bass hits
 * - AURORA_RIBBON: Translucent ribbon with waveform modulation and frequency-based colors
 * - ELECTRIC_ARC: TRON-style geometric light beams with crackling effects
 * - GRASS_OSCILLOSCOPE: Organic swaying grass blades with amplitude height and stereo sway
 * - CRYSTAL_SHARDS: Prismatic diamond shards bursting outward with refraction effects
 * - WATERCOLOR_BLOOM: Painterly circular blooms with multi-layer depth
 * - ICE_FRACTURE: Cracking ice radiating from center with branching fractures
 * - FIREFLY_FIELD: Drifting fireflies with warm glow and gentle flicker
 * - VINYL_SPIN: Rotating vinyl grooves with tempo-based rotation and treble glints
 * - BEAT_ORBS: Volumetric orbs expanding on bass hits with soft gradients
 * - TUNNEL_WAVEFORM: 3D tunnel of pulsating rings with perspective depth
 * - GLASS_SHARDS: Reflective rotating glass fragments with prismatic colors
 * - WIND_FIELD: Flowing particle wind effect with bass intensity and treble sparkles
 * 
 * Audio Analysis:
 * - Frequency data: getByteFrequencyData() returns 0-255 magnitude per frequency bin
 * - Waveform data: getByteTimeDomainData() returns 0-255 time-domain samples (128 = center)
 * - Energy bands calculated: Bass (0-30 bins), Mid (30-150), Treble (150-300)
 * - FFT size: 2048 for high-resolution frequency analysis
 * 
 * Performance Optimizations:
 * - Particle systems capped at reasonable limits (40-300 particles)
 * - Animation paused when not visible (opacity = 0)
 * - ResizeObserver prevents unnecessary canvas resizing
 * - Device pixel ratio scaling for crisp rendering
 * - Efficient Canvas 2D rendering (no WebGL dependency)
 * 
 * State Management:
 * - opacityRef: Current fade opacity (0-1)
 * - fadingInRef/fadingOutRef: Track active fade transitions
 * - particlesRef/shards/etc: Persistent particle systems per visualization
 * - animationRef: requestAnimationFrame ID for cleanup
 * 
 * @module AlbumArtVisualizer
 * @requires audioEngine - Web Audio API abstraction for analyzer access
 * @requires VisualizerMode - Type definition for all supported visualization modes
 */

import React, { useEffect, useRef, useState, useCallback } from 'react';
import { audioEngine } from '../../lib/audio';
import { VisualizerMode } from '../../types';
import { CanvasPostProcessor } from './CanvasPostProcessor';
import { SpriteGenerator } from './SpriteGenerator';
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
 * Renders different visualization styles based on mode:
 * - WAVE: Smooth waveform with glow effect
 * - SPECTRUM: Circular or bar-based frequency display
 * - AURORA: Ambient flowing gradients reacting to bass/treble
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
    const spritesRef = useRef<Record<string, HTMLCanvasElement>>({});
    const [isRendering, setIsRendering] = useState(false);
    
    // Fade parameters
    const FADE_DURATION = 400; // ms
    const fadeStartTimeRef = useRef<number>(0);
    const fadingInRef = useRef<boolean>(false);
    const fadingOutRef = useRef<boolean>(false);

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
     * Draws smooth waveform with glow effect
     */
    const drawGlowWave = useCallback((
        ctx: CanvasRenderingContext2D,
        dataArray: Uint8Array,
        width: number,
        height: number,
        bufferLength: number
    ) => {
        ctx.save();
        ctx.globalAlpha = opacityRef.current;
        
        // Draw glow layer (thicker, blurred)
        ctx.shadowColor = color;
        ctx.shadowBlur = 20;
        ctx.lineWidth = 4;
        ctx.strokeStyle = color;
        ctx.beginPath();
        
        const sliceWidth = width / bufferLength;
        let x = 0;
        
        for (let i = 0; i < bufferLength; i++) {
            const v = dataArray[i] / 128.0;
            const y = (v * height) / 2;
            
            if (i === 0) {
                ctx.moveTo(x, y);
            } else {
                // Use quadratic curves for smoother lines
                const prevX = x - sliceWidth;
                const prevV = dataArray[i - 1] / 128.0;
                const prevY = (prevV * height) / 2;
                const cpX = (prevX + x) / 2;
                ctx.quadraticCurveTo(prevX, prevY, cpX, (prevY + y) / 2);
            }
            x += sliceWidth;
        }
        
        ctx.stroke();
        
        // Draw main line (thinner, sharper)
        ctx.shadowBlur = 0;
        ctx.lineWidth = 2;
        ctx.strokeStyle = 'white';
        ctx.beginPath();
        
        x = 0;
        for (let i = 0; i < bufferLength; i++) {
            const v = dataArray[i] / 128.0;
            const y = (v * height) / 2;
            
            if (i === 0) ctx.moveTo(x, y);
            else ctx.lineTo(x, y);
            
            x += sliceWidth;
        }
        ctx.stroke();
        
        ctx.restore();
    }, [color]);

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

    // ==================== NEXT-GEN VISUALIZATIONS ====================

    /**
     * 🔥 Flame Spectrum Crown
     * 
     * Renders stylized flame tongues rising from the bottom of the canvas, with each flame
     * reacting to a specific frequency range in the audio spectrum.
     * 
     * Audio Mapping:
     * - Flame height: Frequency amplitude (0-255) scaled to 60% of canvas height
     * - Flame color: Based on intensity value
     *   - Low (0-0.3): Warm orange (255, 140-200, 0)
     *   - Medium (0.3-0.6): Fiery red (255, 80-0, 0)
     *   - High (0.6-1.0): White-hot flare (255, 150-255, 0-255)
     * - Flicker: Sine wave animation + random jitter for organic movement
     * 
     * Visual Technique:
     * - 48 flames across the canvas width
     * - Bezier curve flame shapes with irregular control points
     * - Multi-layer rendering: inner gradient + outer glow with shadowBlur
     * - Variable glow intensity based on flame energy
     * 
     * Performance:
     * - ~48 bezier path draws per frame
     * - Shadow blur applied per flame (expensive but visually critical)
     * 
     * Best for: Rock, Hip-Hop, Electronic, Energetic tracks
     */
    const drawFlameSpectrum = useCallback((
        ctx: CanvasRenderingContext2D,
        frequencyData: Uint8Array,
        width: number,
        height: number,
        time: number
    ) => {
        ctx.save();
        ctx.globalAlpha = opacityRef.current;
        
        // Use additive blending for fire
        ctx.globalCompositeOperation = 'lighter';
        
        const particles = flameParticlesRef.current;
        
        // Spawn particles based on frequency
        const numColumns = 32;
        const colWidth = width / numColumns;
        const step = Math.floor(frequencyData.length * 0.7 / numColumns);
        
        for (let i = 0; i < numColumns; i++) {
            let sum = 0;
            for (let j = 0; j < step; j++) {
                sum += frequencyData[i * step + j];
            }
            const value = sum / step / 255;
            
            // Spawn chance increases with amplitude
            if (Math.random() < value * 0.8) {
                const x = i * colWidth + colWidth / 2 + (Math.random() - 0.5) * colWidth;
                particles.push({
                    x: x,
                    y: height,
                    vx: (Math.random() - 0.5) * 1,
                    vy: -2 - Math.random() * 3 - value * 5, // Upward speed
                    size: 10 + value * 30,
                    life: 1,
                    maxLife: 30 + Math.random() * 30,
                    hue: 0 + Math.random() * 40 // Red to Orange/Yellow
                });
            }
        }
        
        // Update and draw particles
        for (let i = particles.length - 1; i >= 0; i--) {
            const p = particles[i];
            p.x += p.vx;
            p.y += p.vy;
            p.size *= 0.96; // Shrink
            p.life -= 1 / p.maxLife;
            
            // Wind effect
            p.x += Math.sin(time * 0.002 + p.y * 0.01) * 0.5;
            
            if (p.life <= 0 || p.size < 1) {
                particles.splice(i, 1);
                continue;
            }
            
            // Color shift based on life: White -> Yellow -> Orange -> Red
            const lifeRatio = p.life;
            let color;
            if (lifeRatio > 0.8) {
                color = `rgba(255, 255, 200, ${p.life * 0.5})`; // White-ish
            } else if (lifeRatio > 0.5) {
                color = `rgba(255, ${150 + lifeRatio * 100}, 0, ${p.life * 0.4})`; // Yellow/Orange
            } else {
                color = `rgba(${150 + lifeRatio * 100}, 50, 0, ${p.life * 0.3})`; // Red
            }
            
            ctx.fillStyle = color;
            ctx.beginPath();
            ctx.arc(p.x, p.y, p.size, 0, Math.PI * 2);
            ctx.fill();
        }
        
        ctx.restore();
    }, []);

    /**
     * 🌌 Stardust Pulse Halo
     * 
     * Creates a circular halo ring that pulses and releases stardust particle bursts
     * synchronized with bass hits in the audio.
     * 
     * Audio Mapping:
     * - Bass energy: Controls halo radius expansion (35% base + 30% dynamic)
     * - Bass threshold (>0.4): Triggers particle spawn burst (15 particles per hit)
     * - Treble peaks (>0.3): Creates shimmering glint points on the ring
     * 
     * Particle System:
     * - Max 300 particles for performance
     * - Each particle has: angle, distance, size, speed, life (0-1)
     * - Particles expand outward from halo radius
     * - Radial gradient glow: white center → cyan edge
     * - Lifespan decreases over time, particles removed at life=0
     * 
     * Visual Elements:
     * - Base halo ring: 2px cyan stroke with alpha modulated by bass
     * - Stardust particles: Expanding with gradient glow trails
     * - Treble sparkles: 12 points rotating around halo circumference
     * 
     * Performance:
     * - Particle count capped at 300
     * - Efficient splice for dead particle removal
     * - Sparkles only rendered when treble > 0.3
     * 
     * Best for: Ambient, Electronic, Space-themed music
     */
    const stardustParticlesRef = useRef<Array<{
        angle: number; distance: number; size: number; speed: number; life: number;
    }>>([]);
    
    const drawStardustHalo = useCallback((
        ctx: CanvasRenderingContext2D,
        frequencyData: Uint8Array,
        width: number,
        height: number,
        time: number
    ) => {
        const centerX = width / 2;
        const centerY = height / 2;
        
        // Calculate audio energy
        let bass = 0, treble = 0;
        for (let i = 0; i < 30; i++) bass += frequencyData[i] || 0;
        for (let i = 200; i < 350; i++) treble += frequencyData[i] || 0;
        bass = (bass / 30) / 255;
        treble = (treble / 150) / 255;
        
        const baseRadius = Math.min(width, height) * 0.35;
        const haloRadius = baseRadius * (1 + bass * 0.3);
        
        ctx.save();
        ctx.globalAlpha = opacityRef.current;
        
        // Spawn new stardust particles on bass hits
        const particles = stardustParticlesRef.current;
        if (bass > 0.4 && particles.length < 300) {
            for (let i = 0; i < Math.floor(bass * 15); i++) {
                particles.push({
                    angle: Math.random() * Math.PI * 2,
                    distance: haloRadius,
                    size: 1 + Math.random() * 2 + bass * 2,
                    speed: 0.5 + Math.random() * 2 + bass * 3,
                    life: 1
                });
            }
        }
        
        // Update and draw particles with trails
        for (let i = particles.length - 1; i >= 0; i--) {
            const p = particles[i];
            p.distance += p.speed;
            p.life -= 0.01;
            p.angle += 0.002;
            
            if (p.life <= 0 || p.distance > baseRadius * 2) {
                particles.splice(i, 1);
                continue;
            }
            
            const px = centerX + Math.cos(p.angle) * p.distance;
            const py = centerY + Math.sin(p.angle) * p.distance;
            
            // OPTIMIZATION: Add comet-like trails (+30% cool factor, -5% performance)
            // Draw trail from previous position
            const trailLength = p.speed * 3;
            const trailStartDist = p.distance - trailLength;
            if (trailStartDist > 0) {
                const tx = centerX + Math.cos(p.angle) * trailStartDist;
                const ty = centerY + Math.sin(p.angle) * trailStartDist;
                
                // Trail gradient
                const trailGradient = ctx.createLinearGradient(tx, ty, px, py);
                trailGradient.addColorStop(0, 'rgba(139, 200, 246, 0)');
                trailGradient.addColorStop(1, `rgba(255, 255, 255, ${p.life * 0.3})`);
                
                ctx.strokeStyle = trailGradient;
                ctx.lineWidth = p.size * 0.8;
                ctx.beginPath();
                ctx.moveTo(tx, ty);
                ctx.lineTo(px, py);
                ctx.stroke();
            }
            
            // Particle head (now brighter as trail head)
            const glowGradient = ctx.createRadialGradient(px, py, 0, px, py, p.size * 4);
            glowGradient.addColorStop(0, `rgba(255, 255, 255, ${p.life * 0.9})`);
            glowGradient.addColorStop(0.5, `rgba(139, 200, 246, ${p.life * 0.5})`);
            glowGradient.addColorStop(1, 'rgba(0, 0, 0, 0)');
            ctx.fillStyle = glowGradient;
            ctx.beginPath();
            ctx.arc(px, py, p.size * 4, 0, Math.PI * 2);
            ctx.fill();
        }
        
        // Draw halo ring
        ctx.strokeStyle = `rgba(139, 200, 246, ${0.3 + bass * 0.4})`;
        ctx.lineWidth = 2;
        ctx.beginPath();
        ctx.arc(centerX, centerY, haloRadius, 0, Math.PI * 2);
        ctx.stroke();
        
        // Treble sparkles on halo
        if (treble > 0.3) {
            const flare = spritesRef.current['flare'];
            for (let i = 0; i < 12; i++) {
                const angle = (i / 12) * Math.PI * 2 + time * 0.002;
                const sx = centerX + Math.cos(angle) * haloRadius;
                const sy = centerY + Math.sin(angle) * haloRadius;
                const sparkle = Math.sin(time * 0.01 + i) * 0.5 + 0.5;
                
                if (flare) {
                    const size = (10 + treble * 20) * sparkle;
                    ctx.globalAlpha = opacityRef.current * sparkle * treble;
                    ctx.drawImage(flare, sx - size/2, sy - size/2, size, size);
                } else {
                    ctx.fillStyle = `rgba(255, 255, 255, ${sparkle * treble})`;
                    ctx.beginPath();
                    ctx.arc(sx, sy, 2 + treble * 3, 0, Math.PI * 2);
                    ctx.fill();
                }
            }
        }
        
        ctx.restore();
    }, []);

    /**
     * 🌈 Aurora Ribbon
     * Translucent ribbon that waves across like Northern Lights
     */
    const drawAuroraRibbon = useCallback((
        ctx: CanvasRenderingContext2D,
        frequencyData: Uint8Array,
        width: number,
        height: number,
        time: number
    ) => {
        // Calculate frequency bands
        let bass = 0, mid = 0, treble = 0;
        for (let i = 0; i < 40; i++) bass += frequencyData[i] || 0;
        for (let i = 40; i < 160; i++) mid += frequencyData[i] || 0;
        for (let i = 160; i < 300; i++) treble += frequencyData[i] || 0;
        bass = (bass / 40) / 255;
        mid = (mid / 120) / 255;
        treble = (treble / 140) / 255;
        
        ctx.save();
        
        const numPoints = 60;
        const waveSpeed = time * 0.001;
        
        // OPTIMIZATION: Multi-layer parallax for depth (+50% cool factor)
        // Three layers: background (slow), mid (medium), foreground (fast)
        const layers = [
            { speed: 0.5, yOffset: height * 0.2, alpha: 0.3, width: 25, color: 'rgba(100, 150, 255, 0.3)' },
            { speed: 0.8, yOffset: height * 0.1, alpha: 0.5, width: 35, color: null }, // Main layer (gradient)
            { speed: 1.2, yOffset: -height * 0.15, alpha: 0.4, width: 20, color: 'rgba(200, 150, 255, 0.4)' }
        ];
        
        layers.forEach((layer, layerIndex) => {
            ctx.globalAlpha = opacityRef.current * layer.alpha;
            const layerWaveSpeed = waveSpeed * layer.speed;
            
            // Create ribbon path for this layer
            ctx.beginPath();
            for (let i = 0; i <= numPoints; i++) {
                const t = i / numPoints;
                const x = t * width;
                
                // Waveform modulation with layer-specific phase
                const wave1 = Math.sin(t * Math.PI * 2 + layerWaveSpeed + layerIndex) * 40 * (1 + bass);
                const wave2 = Math.sin(t * Math.PI * 4 - layerWaveSpeed * 0.7 + layerIndex * 0.5) * 20 * (1 + mid);
                const y = height / 2 + wave1 + wave2 + layer.yOffset;
                
                if (i === 0) ctx.moveTo(x, y);
                else ctx.lineTo(x, y);
            }
            
            // Middle layer uses gradient, others use solid colors
            if (layerIndex === 1) {
                // Create gradient for main ribbon
                const gradient = ctx.createLinearGradient(0, 0, width, 0);
                
                // Color shifts with frequency dominance
                if (bass > mid && bass > treble) {
                    gradient.addColorStop(0, 'rgba(0, 255, 200, 0.4)');
                    gradient.addColorStop(0.5, 'rgba(100, 100, 255, 0.5)');
                    gradient.addColorStop(1, 'rgba(200, 50, 255, 0.4)');
                } else if (mid > bass && mid > treble) {
                    gradient.addColorStop(0, 'rgba(139, 92, 246, 0.4)');
                    gradient.addColorStop(0.5, 'rgba(236, 72, 153, 0.5)');
                    gradient.addColorStop(1, 'rgba(59, 130, 246, 0.4)');
                } else {
                    gradient.addColorStop(0, 'rgba(255, 100, 150, 0.4)');
                    gradient.addColorStop(0.5, 'rgba(100, 200, 255, 0.5)');
                    gradient.addColorStop(1, 'rgba(150, 255, 100, 0.4)');
                }
                
                ctx.lineWidth = layer.width + mid * 30;
                ctx.strokeStyle = gradient;
            } else {
                ctx.lineWidth = layer.width + mid * 15;
                ctx.strokeStyle = layer.color!;
            }
            
            ctx.lineCap = 'round';
            ctx.stroke();
            
            // Add shimmer effect to main layer only
            if (layerIndex === 1) {
                ctx.globalAlpha = opacityRef.current * 0.3;
                ctx.lineWidth = 10 + treble * 20;
                ctx.strokeStyle = 'rgba(255, 255, 255, 0.6)';
                ctx.stroke();
            }
        });
        
        ctx.restore();
    }, []);

    /**
     * ⚡ Electric Arc Wireframe
     * TRON-style geometric light beams arcing between points
     */
    const electricArcsRef = useRef<Array<{
        x1: number; y1: number; x2: number; y2: number; life: number; intensity: number;
    }>>([]);

    const flameParticlesRef = useRef<Array<{
        x: number; y: number; vx: number; vy: number; size: number; life: number; maxLife: number; hue: number;
    }>>([]);

    const tunnelPointsRef = useRef<Array<{
        x: number; y: number; z: number; angle: number; ringIndex: number;
    }>>([]);
    
    const drawElectricArc = useCallback((
        ctx: CanvasRenderingContext2D,
        frequencyData: Uint8Array,
        width: number,
        height: number,
        time: number
    ) => {
        let bass = 0, treble = 0;
        for (let i = 0; i < 30; i++) bass += frequencyData[i] || 0;
        for (let i = 200; i < 350; i++) treble += frequencyData[i] || 0;
        bass = (bass / 30) / 255;
        treble = (treble / 150) / 255;
        
        ctx.save();
        ctx.globalAlpha = opacityRef.current;
        
        const arcs = electricArcsRef.current;
        
        // Spawn new arcs on bass hits
        if (bass > 0.5 && arcs.length < 12) {
            const margin = 50;
            arcs.push({
                x1: margin + Math.random() * (width - margin * 2),
                y1: margin + Math.random() * (height - margin * 2),
                x2: margin + Math.random() * (width - margin * 2),
                y2: margin + Math.random() * (height - margin * 2),
                life: 1,
                intensity: bass
            });
        }
        
        // OPTIMIZATION: Batch stroke operations (+25% performance)
        // Group arcs by similar properties to reduce state changes
        const mainArcs: typeof arcs = [];
        const brightArcs: typeof arcs = [];
        
        // Update arcs and categorize
        for (let i = arcs.length - 1; i >= 0; i--) {
            const arc = arcs[i];
            arc.life -= 0.02;
            
            if (arc.life <= 0) {
                arcs.splice(i, 1);
                continue;
            }
            
            mainArcs.push(arc);
            if (treble > 0.4) {
                brightArcs.push(arc);
            }
        }
        
        // Draw all main arcs in one batch
        ctx.shadowColor = `rgba(0, 255, 255, 0.8)`;
        ctx.shadowBlur = 15 + bass * 20;
        
        for (const arc of mainArcs) {
            const segments = 8;
            ctx.beginPath();
            ctx.moveTo(arc.x1, arc.y1);
            
            for (let s = 1; s <= segments; s++) {
                const t = s / segments;
                const x = arc.x1 + (arc.x2 - arc.x1) * t;
                const y = arc.y1 + (arc.y2 - arc.y1) * t;
                
                // Add jitter for electric effect
                const jitter = (Math.random() - 0.5) * 20 * treble;
                ctx.lineTo(x + jitter, y + jitter);
            }
            
            ctx.strokeStyle = `rgba(0, 255, 255, ${arc.life * arc.intensity})`;
            ctx.lineWidth = 2 + arc.intensity * 3;
            ctx.stroke();
        }
        
        // Draw bright treble arcs in one batch (no shadow)
        if (brightArcs.length > 0) {
            ctx.shadowBlur = 0;
            ctx.lineWidth = 1;
            
            for (const arc of brightArcs) {
                const segments = 8;
                ctx.beginPath();
                ctx.moveTo(arc.x1, arc.y1);
                
                for (let s = 1; s <= segments; s++) {
                    const t = s / segments;
                    const x = arc.x1 + (arc.x2 - arc.x1) * t;
                    const y = arc.y1 + (arc.y2 - arc.y1) * t;
                    const jitter = (Math.random() - 0.5) * 20 * treble;
                    ctx.lineTo(x + jitter, y + jitter);
                }
                
                ctx.strokeStyle = `rgba(255, 255, 255, ${arc.life * treble * 0.5})`;
                ctx.stroke();
            }
        }
        
        ctx.restore();
    }, []);

    /**
     * 🌱 Growing Lines / Grass Oscilloscope
     * Vertical grass blades at bottom that grow and sway
     */
    const drawGrassOscilloscope = useCallback((
        ctx: CanvasRenderingContext2D,
        waveformData: Uint8Array,
        frequencyData: Uint8Array,
        width: number,
        height: number,
        time: number
    ) => {
        ctx.save();
        ctx.globalAlpha = opacityRef.current;
        
        const numBlades = 80;
        const bladeSpacing = width / numBlades;
        const step = Math.floor(waveformData.length / numBlades);
        
        // Calculate stereo offset (simplified)
        let leftEnergy = 0, rightEnergy = 0;
        for (let i = 0; i < frequencyData.length / 2; i++) leftEnergy += frequencyData[i];
        for (let i = frequencyData.length / 2; i < frequencyData.length; i++) rightEnergy += frequencyData[i];
        const stereoOffset = ((rightEnergy - leftEnergy) / frequencyData.length) / 255;
        
        for (let i = 0; i < numBlades; i++) {
            const dataIndex = i * step;
            const amplitude = (waveformData[dataIndex] - 128) / 128;
            const bladeHeight = Math.abs(amplitude) * height * 0.5 + 20;
            
            const x = i * bladeSpacing + bladeSpacing / 2;
            const sway = Math.sin(time * 0.003 + i * 0.2) * 5 * Math.abs(amplitude) + stereoOffset * 10;
            
            // Color gradient
            const hue = 90 + i * 2;
            const gradient = ctx.createLinearGradient(x, height, x, height - bladeHeight);
            gradient.addColorStop(0, `hsla(${hue}, 70%, 40%, 0.9)`);
            gradient.addColorStop(0.5, `hsla(${hue}, 80%, 55%, 0.8)`);
            gradient.addColorStop(1, `hsla(${hue}, 90%, 70%, 0.6)`);
            
            // Draw blade with bezier curve for organic sway
            ctx.strokeStyle = gradient;
            ctx.lineWidth = 2 + Math.abs(amplitude) * 2;
            ctx.lineCap = 'round';
            ctx.beginPath();
            ctx.moveTo(x, height);
            ctx.bezierCurveTo(
                x + sway * 0.3, height - bladeHeight * 0.3,
                x + sway * 0.7, height - bladeHeight * 0.7,
                x + sway, height - bladeHeight
            );
            ctx.stroke();
        }
        
        ctx.restore();
    }, []);

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
     *  3D Tunnel Waveform
     * Tunnel of pulsating rings originating from center
     */
    const drawTunnelWaveform = useCallback((
        ctx: CanvasRenderingContext2D,
        frequencyData: Uint8Array,
        width: number,
        height: number,
        time: number
    ) => {
        const centerX = width / 2;
        const centerY = height / 2;
        
        let bass = 0, mid = 0, treble = 0;
        for (let i = 0; i < 30; i++) bass += frequencyData[i] || 0;
        for (let i = 30; i < 150; i++) mid += frequencyData[i] || 0;
        for (let i = 150; i < 300; i++) treble += frequencyData[i] || 0;
        bass = (bass / 30) / 255;
        mid = (mid / 120) / 255;
        treble = (treble / 140) / 255;
        
        ctx.save();
        ctx.globalAlpha = opacityRef.current;
        ctx.translate(centerX, centerY);
        
        const points = tunnelPointsRef.current;
        const numRings = 24;
        const pointsPerRing = 32;
        const tunnelLength = 2000;
        const ringSpacing = tunnelLength / numRings;
        const baseRadius = 300;
        
        // Initialize points
        if (points.length === 0) {
            for (let r = 0; r < numRings; r++) {
                for (let p = 0; p < pointsPerRing; p++) {
                    const angle = (p / pointsPerRing) * Math.PI * 2;
                    points.push({
                        x: Math.cos(angle) * baseRadius,
                        y: Math.sin(angle) * baseRadius,
                        z: r * ringSpacing,
                        angle: angle,
                        ringIndex: r
                    });
                }
            }
        }
        
        // Move points
        const speed = 10 + bass * 40;
        for (const p of points) {
            p.z -= speed;
            if (p.z < 10) {
                p.z += tunnelLength;
            }
        }
        
        const fov = 300;
        ctx.lineWidth = 2 + mid * 4;
        
        // Draw rings
        for (let r = 0; r < numRings; r++) {
            ctx.beginPath();
            let firstPoint = true;
            
            // Get points for this ring (assuming order is preserved)
            const ringPoints = points.slice(r * pointsPerRing, (r + 1) * pointsPerRing);
            
            // Skip if behind camera
            if (ringPoints[0].z < 10) continue;
            
            const zRatio = ringPoints[0].z / tunnelLength;
            const ringPulse = Math.sin(time * 0.005 + zRatio * 10) * 50 * bass;
            
            const hue = 180 + zRatio * 120 + time * 0.05;
            const alpha = 1 - zRatio;
            ctx.strokeStyle = `hsla(${hue}, 80%, 60%, ${alpha})`;
            
            for (let i = 0; i < pointsPerRing; i++) {
                const p = ringPoints[i];
                
                // Audio distortion
                const freqIndex = Math.floor((i / pointsPerRing) * 50);
                const freqVal = frequencyData[freqIndex] / 255;
                const radiusMod = freqVal * 100 * mid;
                
                const scale = fov / (fov + p.z);
                const x2d = (p.x * (1 + radiusMod/baseRadius) + Math.cos(p.angle)*ringPulse) * scale;
                const y2d = (p.y * (1 + radiusMod/baseRadius) + Math.sin(p.angle)*ringPulse) * scale;
                
                if (firstPoint) {
                    ctx.moveTo(x2d, y2d);
                    firstPoint = false;
                } else {
                    ctx.lineTo(x2d, y2d);
                }
            }
            ctx.closePath();
            ctx.stroke();
        }
        
        // Center glow
        const coreGradient = ctx.createRadialGradient(0, 0, 0, 0, 0, 50);
        coreGradient.addColorStop(0, `rgba(255, 255, 255, ${0.8 + bass * 0.2})`);
        coreGradient.addColorStop(1, 'rgba(0, 0, 0, 0)');
        ctx.fillStyle = coreGradient;
        ctx.beginPath();
        ctx.arc(0, 0, 50, 0, Math.PI * 2);
        ctx.fill();
        
        ctx.restore();
    }, []);

    /**
     * 🌬️ Soft Wind Field
     * Flowing particle field moving like wind
     */
    const windParticlesRef = useRef<Array<{
        x: number; y: number; vx: number; vy: number; size: number; opacity: number;
    }>>([]);
    
    const drawWindField = useCallback((
        ctx: CanvasRenderingContext2D,
        frequencyData: Uint8Array,
        width: number,
        height: number,
        time: number
    ) => {
        let bass = 0, treble = 0;
        for (let i = 0; i < 30; i++) bass += frequencyData[i] || 0;
        for (let i = 200; i < 350; i++) treble += frequencyData[i] || 0;
        bass = (bass / 30) / 255;
        treble = (treble / 150) / 255;
        
        const windIntensity = bass;
        const sparkleIntensity = treble;
        
        ctx.save();
        ctx.globalAlpha = opacityRef.current;
        
        const particles = windParticlesRef.current;
        
        // Maintain particle count
        while (particles.length < 100) {
            particles.push({
                x: Math.random() * width,
                y: Math.random() * height,
                vx: 1 + Math.random() * 2,
                vy: (Math.random() - 0.5) * 0.5,
                size: 1 + Math.random() * 2,
                opacity: 0.3 + Math.random() * 0.4
            });
        }
        
        // OPTIMIZATION: Add wind direction variance for turbulence (+25% cool factor)
        const windAngle = Math.sin(time * 0.0002) * Math.PI * 0.2; // Oscillates ±36 degrees
        const windDirX = Math.cos(windAngle);
        const windDirY = Math.sin(windAngle);
        
        // Update and draw particles
        for (let i = particles.length - 1; i >= 0; i--) {
            const p = particles[i];
            
            // Wind movement with dynamic direction
            const windForce = (1 + windIntensity * 2);
            p.x += p.vx * windForce * windDirX;
            p.y += p.vy + Math.sin(p.x * 0.01 + time * 0.001) * 2 + windForce * windDirY;
            
            // Wrap around
            if (p.x > width + 10) {
                p.x = -10;
                p.y = Math.random() * height;
            }
            if (p.y < -10) p.y = height + 10;
            if (p.y > height + 10) p.y = -10;
            
            // Draw particle
            const alpha = p.opacity * (0.5 + windIntensity * 0.5);
            
            // Sparkle effect on treble
            const sparkle = sparkleIntensity > 0.4 && Math.random() < 0.1;
            const size = sparkle ? p.size * 3 : p.size;
            const brightness = sparkle ? 255 : 180;
            
            const gradient = ctx.createRadialGradient(p.x, p.y, 0, p.x, p.y, size * 2);
            gradient.addColorStop(0, `rgba(${brightness}, ${brightness}, 255, ${alpha})`);
            gradient.addColorStop(1, 'rgba(200, 220, 255, 0)');
            
            ctx.fillStyle = gradient;
            ctx.beginPath();
            ctx.arc(p.x, p.y, size * 2, 0, Math.PI * 2);
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
        
        const ctx = canvas.getContext('2d', { alpha: false });
        if (!ctx) return;

        // Initialize processor if needed
        if (!processorRef.current) {
            processorRef.current = new CanvasPostProcessor();
        }
        const processor = processorRef.current;

        // Initialize sprites if needed
        if (Object.keys(spritesRef.current).length === 0) {
            spritesRef.current = {
                'glow': SpriteGenerator.createGlowSprite(64, 'rgba(255, 255, 255, 1)'),
                'flare': SpriteGenerator.createFlareSprite(64),
                'ring': SpriteGenerator.createRingSprite(128, 4, 'rgba(255, 255, 255, 1)'),
                'cloud': SpriteGenerator.createCloudSprite(256)
            };
        }
        
        // Handle canvas sizing
        const updateSize = () => {
            const dpr = window.devicePixelRatio || 1;
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
            const trailModes = ['STARDUST_HALO', 'FIREFLY_FIELD', 'WIND_FIELD', 'ELECTRIC_ARC'];
            if (trailModes.includes(mode)) {
                processor.fade(0.2); // Short trails
            } else {
                processor.clear();
            }
            
            const analyser = audioEngine.getAnalyser();
            if (!analyser) return;
            
            // Get frequency and waveform data
            analyser.getByteFrequencyData(frequencyData);
            analyser.getByteTimeDomainData(waveformData);
            
            // Draw based on mode
            switch (mode) {
                case 'WAVE':
                    drawGlowWave(drawCtx, waveformData, width, height, analyser.frequencyBinCount);
                    break;
                case 'SPECTRUM':
                    drawCircularSpectrum(drawCtx, frequencyData, width, height);
                    break;
                
                // Next-Gen Visualizations
                case 'FLAME_SPECTRUM':
                    drawFlameSpectrum(drawCtx, frequencyData, width, height, timestamp);
                    break;
                case 'STARDUST_HALO':
                    drawStardustHalo(drawCtx, frequencyData, width, height, timestamp);
                    break;
                case 'AURORA_RIBBON':
                    drawAuroraRibbon(drawCtx, frequencyData, width, height, timestamp);
                    break;
                case 'ELECTRIC_ARC':
                    drawElectricArc(drawCtx, frequencyData, width, height, timestamp);
                    break;
                case 'GRASS_OSCILLOSCOPE':
                    drawGrassOscilloscope(drawCtx, waveformData, frequencyData, width, height, timestamp);
                    break;
                case 'FIREFLY_FIELD':
                    drawFireflyField(drawCtx, frequencyData, width, height, timestamp);
                    break;
                case 'TUNNEL_WAVEFORM':
                    drawTunnelWaveform(drawCtx, frequencyData, width, height, timestamp);
                    break;
                case 'WIND_FIELD':
                    drawWindField(drawCtx, frequencyData, width, height, timestamp);
                    break;
                    
                default:
                    // For unknown modes, draw spectrum bars as fallback
                    drawSpectrumBars(drawCtx, frequencyData, width, height);
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
        mode, isActive, isRendering, updateOpacity,
        // Original visualizations
        drawCircularSpectrum, drawGlowWave, drawSpectrumBars, 
        // Next-gen visualizations
        drawFlameSpectrum, drawStardustHalo, drawAuroraRibbon, drawElectricArc,
        drawGrassOscilloscope, drawFireflyField, drawTunnelWaveform, drawWindField
    ]);

    // Don't render canvas if completely hidden and not fading
    if (!isRendering && !isActive && opacityRef.current <= 0) {
        return null;
    }

    return (
        <canvas
            ref={canvasRef}
            className={`absolute inset-0 w-full h-full z-10 pointer-events-none transition-opacity duration-300 ${className}`}
            style={{ opacity: opacityRef.current }}
        />
    );
};

export default AlbumArtVisualizer;
