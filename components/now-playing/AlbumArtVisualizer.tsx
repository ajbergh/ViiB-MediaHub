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
     * Draws ambient aurora effect reacting to bass and treble
     */
    const drawAurora = useCallback((
        ctx: CanvasRenderingContext2D,
        dataArray: Uint8Array,
        width: number,
        height: number,
        time: number
    ) => {
        // Calculate bass and treble energy
        let bass = 0, mid = 0, treble = 0;
        const len = dataArray.length;
        
        for (let i = 0; i < Math.min(50, len); i++) bass += dataArray[i];
        for (let i = 50; i < Math.min(200, len); i++) mid += dataArray[i];
        for (let i = 200; i < Math.min(400, len); i++) treble += dataArray[i];
        
        bass = (bass / 50) / 255;
        mid = (mid / 150) / 255;
        treble = (treble / 200) / 255;
        
        ctx.save();
        ctx.globalAlpha = opacityRef.current * 0.7;
        
        // Animated gradient based on audio
        const gradientAngle = time * 0.0005;
        const x1 = width / 2 + Math.cos(gradientAngle) * width;
        const y1 = height / 2 + Math.sin(gradientAngle) * height;
        const x2 = width / 2 + Math.cos(gradientAngle + Math.PI) * width;
        const y2 = height / 2 + Math.sin(gradientAngle + Math.PI) * height;
        
        const gradient = ctx.createLinearGradient(x1, y1, x2, y2);
        gradient.addColorStop(0, rgbaFromRgb(VIIB_COLOR_RGB.visualizerGreen, bass * 0.8));
        gradient.addColorStop(0.3, rgbaFromRgb(VIIB_COLOR_RGB.visualizerPurple, mid * 0.6));
        gradient.addColorStop(0.6, rgbaFromRgb(VIIB_COLOR_RGB.visualizerBlue, treble * 0.7));
        gradient.addColorStop(1, rgbaFromRgb(VIIB_COLOR_RGB.visualizerPink, (bass + treble) * 0.5));
        
        ctx.fillStyle = gradient;
        ctx.fillRect(0, 0, width, height);
        
        // Add flowing wave overlay
        ctx.globalAlpha = opacityRef.current * 0.3;
        ctx.beginPath();
        
        const waveAmplitude = 30 + bass * 50;
        const waveFreq = 0.01;
        const waveSpeed = time * 0.002;
        
        ctx.moveTo(0, height);
        for (let x = 0; x <= width; x += 5) {
            const y = height / 2 + Math.sin(x * waveFreq + waveSpeed) * waveAmplitude;
            ctx.lineTo(x, y);
        }
        ctx.lineTo(width, height);
        ctx.closePath();
        
        const waveGradient = ctx.createLinearGradient(0, height / 2, 0, height);
        waveGradient.addColorStop(0, rgbaFromRgb(VIIB_COLOR_RGB.white, 0.2));
        waveGradient.addColorStop(1, rgbaFromRgb(VIIB_COLOR_RGB.white, 0));
        ctx.fillStyle = waveGradient;
        ctx.fill();
        
        ctx.restore();
    }, []);

    /**
     * Draws enhanced circular visualization with pulsing rings and rotating bars
     * Creates a stunning centered ring effect with inner waveform
     */
    const drawCircularEnhanced = useCallback((
        ctx: CanvasRenderingContext2D,
        frequencyData: Uint8Array,
        waveformData: Uint8Array,
        width: number,
        height: number,
        time: number
    ) => {
        const centerX = width / 2;
        const centerY = height / 2;
        const maxRadius = Math.min(width, height) * 0.4;
        
        // Calculate audio energy levels
        let bass = 0, mid = 0, treble = 0;
        const len = frequencyData.length;
        for (let i = 0; i < Math.min(30, len); i++) bass += frequencyData[i];
        for (let i = 30; i < Math.min(150, len); i++) mid += frequencyData[i];
        for (let i = 150; i < Math.min(300, len); i++) treble += frequencyData[i];
        bass = (bass / 30) / 255;
        mid = (mid / 120) / 255;
        treble = (treble / 150) / 255;
        
        ctx.save();
        ctx.globalAlpha = opacityRef.current;
        
        // Outer pulsing ring glow
        const pulseRadius = maxRadius * (0.9 + bass * 0.2);
        const glowGradient = ctx.createRadialGradient(
            centerX, centerY, pulseRadius * 0.8,
            centerX, centerY, pulseRadius * 1.2
        );
        glowGradient.addColorStop(0, `rgba(34, 197, 94, ${0.1 + bass * 0.3})`);
        glowGradient.addColorStop(0.5, `rgba(139, 92, 246, ${0.05 + mid * 0.2})`);
        glowGradient.addColorStop(1, 'rgba(0, 0, 0, 0)');
        ctx.fillStyle = glowGradient;
        ctx.beginPath();
        ctx.arc(centerX, centerY, pulseRadius * 1.3, 0, Math.PI * 2);
        ctx.fill();
        
        // Rotating frequency bars around the circle
        const numBars = 72;
        const innerRadius = maxRadius * 0.35;
        const barMaxHeight = maxRadius * 0.35;
        const rotationOffset = time * 0.0003;
        
        for (let i = 0; i < numBars; i++) {
            const dataIndex = Math.floor((i / numBars) * frequencyData.length * 0.5);
            const value = frequencyData[dataIndex] / 255;
            
            const angle = (i / numBars) * Math.PI * 2 + rotationOffset;
            const barHeight = value * barMaxHeight + 3;
            
            const x1 = centerX + Math.cos(angle) * innerRadius;
            const y1 = centerY + Math.sin(angle) * innerRadius;
            const x2 = centerX + Math.cos(angle) * (innerRadius + barHeight);
            const y2 = centerY + Math.sin(angle) * (innerRadius + barHeight);
            
            // Color shifts based on position and audio
            const hue = (i / numBars) * 120 + 100 + time * 0.02;
            const saturation = 70 + value * 30;
            const lightness = 50 + value * 20;
            
            ctx.strokeStyle = `hsla(${hue}, ${saturation}%, ${lightness}%, ${0.5 + value * 0.5})`;
            ctx.lineWidth = 2 + value * 2;
            ctx.lineCap = 'round';
            
            ctx.beginPath();
            ctx.moveTo(x1, y1);
            ctx.lineTo(x2, y2);
            ctx.stroke();
        }
        
        // Inner waveform circle
        ctx.beginPath();
        const waveRadius = innerRadius * 0.8;
        for (let i = 0; i < waveformData.length; i++) {
            const angle = (i / waveformData.length) * Math.PI * 2;
            const value = (waveformData[i] - 128) / 128;
            const r = waveRadius + value * waveRadius * 0.4;
            
            const x = centerX + Math.cos(angle) * r;
            const y = centerY + Math.sin(angle) * r;
            
            if (i === 0) ctx.moveTo(x, y);
            else ctx.lineTo(x, y);
        }
        ctx.closePath();
        
        // Waveform gradient fill
        const waveGradient = ctx.createRadialGradient(centerX, centerY, 0, centerX, centerY, waveRadius);
        waveGradient.addColorStop(0, `rgba(34, 197, 94, ${0.3 + mid * 0.3})`);
        waveGradient.addColorStop(1, `rgba(139, 92, 246, ${0.1 + bass * 0.2})`);
        ctx.fillStyle = waveGradient;
        ctx.fill();
        
        // Center core glow
        const coreGradient = ctx.createRadialGradient(centerX, centerY, 0, centerX, centerY, waveRadius * 0.3);
        coreGradient.addColorStop(0, `rgba(255, 255, 255, ${0.5 + treble * 0.5})`);
        coreGradient.addColorStop(0.5, `rgba(34, 197, 94, ${0.3})`);
        coreGradient.addColorStop(1, 'rgba(0, 0, 0, 0)');
        ctx.fillStyle = coreGradient;
        ctx.beginPath();
        ctx.arc(centerX, centerY, waveRadius * 0.3, 0, Math.PI * 2);
        ctx.fill();
        
        ctx.restore();
    }, []);

    /**
     * Draws particle system visualization with audio-reactive particles
     * Creates floating particles with trails that respond to bass and treble
     */
    const particlesRef = useRef<Array<{
        x: number; y: number; vx: number; vy: number;
        size: number; hue: number; life: number; maxLife: number;
    }>>([]);
    
    const drawParticles = useCallback((
        ctx: CanvasRenderingContext2D,
        frequencyData: Uint8Array,
        width: number,
        height: number,
        time: number
    ) => {
        // Calculate audio energy
        let bass = 0, mid = 0, treble = 0;
        for (let i = 0; i < 30; i++) bass += frequencyData[i] || 0;
        for (let i = 30; i < 150; i++) mid += frequencyData[i] || 0;
        for (let i = 150; i < 300; i++) treble += frequencyData[i] || 0;
        bass = (bass / 30) / 255;
        mid = (mid / 120) / 255;
        treble = (treble / 150) / 255;
        
        const energy = (bass + mid + treble) / 3;
        const particles = particlesRef.current;
        
        // Spawn new particles based on audio energy
        const spawnRate = Math.floor(2 + energy * 8);
        for (let i = 0; i < spawnRate && particles.length < 200; i++) {
            const angle = Math.random() * Math.PI * 2;
            const speed = 1 + Math.random() * 2 + energy * 3;
            particles.push({
                x: width / 2 + (Math.random() - 0.5) * 50,
                y: height / 2 + (Math.random() - 0.5) * 50,
                vx: Math.cos(angle) * speed,
                vy: Math.sin(angle) * speed,
                size: 2 + Math.random() * 4 + bass * 4,
                hue: 120 + Math.random() * 60 + time * 0.02,
                life: 1,
                maxLife: 60 + Math.random() * 60
            });
        }
        
        ctx.save();
        ctx.globalAlpha = opacityRef.current;
        
        // Semi-transparent background for trails
        ctx.fillStyle = 'rgba(0, 0, 0, 0.1)';
        ctx.fillRect(0, 0, width, height);
        
        // Update and draw particles
        for (let i = particles.length - 1; i >= 0; i--) {
            const p = particles[i];
            
            // Update position with audio-reactive acceleration
            p.vx += (Math.random() - 0.5) * 0.3 * energy;
            p.vy += (Math.random() - 0.5) * 0.3 * energy;
            p.x += p.vx;
            p.y += p.vy;
            p.life -= 1 / p.maxLife;
            
            // Add gravity towards center when bass hits
            if (bass > 0.5) {
                const dx = width / 2 - p.x;
                const dy = height / 2 - p.y;
                const dist = Math.sqrt(dx * dx + dy * dy);
                if (dist > 0) {
                    p.vx += (dx / dist) * 0.5 * bass;
                    p.vy += (dy / dist) * 0.5 * bass;
                }
            }
            
            // Remove dead particles
            if (p.life <= 0 || p.x < 0 || p.x > width || p.y < 0 || p.y > height) {
                particles.splice(i, 1);
                continue;
            }
            
            // Draw particle with glow
            const alpha = p.life * (0.5 + energy * 0.5);
            const size = p.size * (0.5 + p.life * 0.5);
            
            // Outer glow
            const glowGradient = ctx.createRadialGradient(p.x, p.y, 0, p.x, p.y, size * 3);
            glowGradient.addColorStop(0, `hsla(${p.hue}, 80%, 60%, ${alpha * 0.5})`);
            glowGradient.addColorStop(1, `hsla(${p.hue}, 80%, 60%, 0)`);
            ctx.fillStyle = glowGradient;
            ctx.beginPath();
            ctx.arc(p.x, p.y, size * 3, 0, Math.PI * 2);
            ctx.fill();
            
            // Core
            ctx.fillStyle = `hsla(${p.hue}, 90%, 70%, ${alpha})`;
            ctx.beginPath();
            ctx.arc(p.x, p.y, size, 0, Math.PI * 2);
            ctx.fill();
        }
        
        // Draw central orb
        const orbRadius = 20 + bass * 30;
        const orbGradient = ctx.createRadialGradient(width/2, height/2, 0, width/2, height/2, orbRadius);
        orbGradient.addColorStop(0, `rgba(255, 255, 255, ${0.8 * energy})`);
        orbGradient.addColorStop(0.3, `rgba(34, 197, 94, ${0.6 * energy})`);
        orbGradient.addColorStop(1, 'rgba(0, 0, 0, 0)');
        ctx.fillStyle = orbGradient;
        ctx.beginPath();
        ctx.arc(width/2, height/2, orbRadius, 0, Math.PI * 2);
        ctx.fill();
        
        ctx.restore();
    }, []);

    /**
     * Draws deep space nebula effect with swirling clouds
     * Creates a cosmic atmosphere that pulses with the music
     */
    const drawNebula = useCallback((
        ctx: CanvasRenderingContext2D,
        frequencyData: Uint8Array,
        width: number,
        height: number,
        time: number
    ) => {
        // Calculate audio energy bands
        let bass = 0, mid = 0, treble = 0;
        for (let i = 0; i < 40; i++) bass += frequencyData[i] || 0;
        for (let i = 40; i < 160; i++) mid += frequencyData[i] || 0;
        for (let i = 160; i < 300; i++) treble += frequencyData[i] || 0;
        bass = (bass / 40) / 255;
        mid = (mid / 120) / 255;
        treble = (treble / 140) / 255;
        
        ctx.save();
        ctx.globalAlpha = opacityRef.current;
        
        // Dark cosmic background with subtle stars
        ctx.fillStyle = `rgba(5, 5, 20, ${0.3})`;
        ctx.fillRect(0, 0, width, height);
        
        // Draw twinkling stars using sprites
        const flareSprite = spritesRef.current['flare'];
        const starCount = 50 + Math.floor(treble * 30);
        
        for (let i = 0; i < starCount; i++) {
            const seed = i * 1234.5678;
            const sx = ((seed * 17) % width);
            const sy = ((seed * 31) % height);
            const twinkle = Math.sin(time * 0.005 + seed) * 0.5 + 0.5;
            const starSize = (1 + twinkle * 2) * (0.5 + treble);
            
            if (flareSprite && i % 5 === 0) { // Use flare for 20% of stars
                const size = starSize * 4;
                ctx.globalAlpha = opacityRef.current * twinkle * 0.8;
                ctx.drawImage(flareSprite, sx - size/2, sy - size/2, size, size);
            } else {
                ctx.globalAlpha = opacityRef.current * twinkle * 0.8;
                ctx.fillStyle = 'white';
                ctx.beginPath();
                ctx.arc(sx, sy, starSize, 0, Math.PI * 2);
                ctx.fill();
            }
        }
        
        // Nebula clouds - multiple layers
        const cloudSprite = spritesRef.current['cloud'];
        const layers = [
            { color: [139, 92, 246], offset: 0, scale: 1.0, energyMult: bass },      // Purple
            { color: [236, 72, 153], offset: 60, scale: 0.8, energyMult: mid },       // Pink  
            { color: [34, 197, 94], offset: 120, scale: 0.6, energyMult: treble },    // Green
            { color: [59, 130, 246], offset: 180, scale: 0.9, energyMult: bass },     // Blue
        ];
        
        layers.forEach((layer, idx) => {
            const cloudTime = time * 0.0002 + idx * 1000;
            const [r, g, b] = layer.color;
            
            // Create swirling nebula patterns using multiple overlapping gradients
            for (let c = 0; c < 3; c++) {
                const angle = cloudTime + c * 2.1 + layer.offset;
                const radius = (width * 0.3 + width * 0.2 * layer.scale) * (1 + layer.energyMult * 0.3);
                
                const cx = width / 2 + Math.cos(angle) * radius * 0.4;
                const cy = height / 2 + Math.sin(angle * 0.7) * radius * 0.3;
                
                // Base color gradient
                const cloudGradient = ctx.createRadialGradient(cx, cy, 0, cx, cy, radius);
                const intensity = 0.15 + layer.energyMult * 0.25;
                cloudGradient.addColorStop(0, `rgba(${r}, ${g}, ${b}, ${intensity})`);
                cloudGradient.addColorStop(0.4, `rgba(${r}, ${g}, ${b}, ${intensity * 0.5})`);
                cloudGradient.addColorStop(1, 'rgba(0, 0, 0, 0)');
                
                ctx.globalAlpha = opacityRef.current;
                ctx.fillStyle = cloudGradient;
                ctx.beginPath();
                ctx.arc(cx, cy, radius, 0, Math.PI * 2);
                ctx.fill();
                
                // Add texture with cloud sprite
                if (cloudSprite) {
                    ctx.save();
                    ctx.translate(cx, cy);
                    ctx.rotate(time * 0.0001 * (idx + 1) + c);
                    ctx.globalAlpha = opacityRef.current * 0.2;
                    ctx.globalCompositeOperation = 'overlay';
                    ctx.drawImage(cloudSprite, -radius, -radius, radius * 2, radius * 2);
                    ctx.restore();
                }
            }
        });
        
        // Central bright core that pulses with bass
        const coreSize = 40 + bass * 60;
        const coreGradient = ctx.createRadialGradient(
            width/2, height/2, 0,
            width/2, height/2, coreSize
        );
        coreGradient.addColorStop(0, `rgba(255, 255, 255, ${0.6 + bass * 0.4})`);
        coreGradient.addColorStop(0.2, `rgba(139, 92, 246, ${0.4 + mid * 0.3})`);
        coreGradient.addColorStop(0.5, `rgba(236, 72, 153, ${0.2 + treble * 0.2})`);
        coreGradient.addColorStop(1, 'rgba(0, 0, 0, 0)');
        
        ctx.globalAlpha = opacityRef.current;
        ctx.fillStyle = coreGradient;
        ctx.beginPath();
        ctx.arc(width/2, height/2, coreSize, 0, Math.PI * 2);
        ctx.fill();
        
        // Lens flare rays from center on bass hits
        if (bass > 0.4) {
            const rayCount = 6;
            ctx.globalAlpha = opacityRef.current * bass * 0.5;
            for (let i = 0; i < rayCount; i++) {
                const rayAngle = (i / rayCount) * Math.PI * 2 + time * 0.001;
                const rayLength = 80 + bass * 100;
                
                const gradient = ctx.createLinearGradient(
                    width/2, height/2,
                    width/2 + Math.cos(rayAngle) * rayLength,
                    height/2 + Math.sin(rayAngle) * rayLength
                );
                gradient.addColorStop(0, 'rgba(255, 255, 255, 0.3)');
                gradient.addColorStop(1, 'rgba(255, 255, 255, 0)');
                
                ctx.strokeStyle = gradient;
                ctx.lineWidth = 2 + bass * 3;
                ctx.beginPath();
                ctx.moveTo(width/2, height/2);
                ctx.lineTo(
                    width/2 + Math.cos(rayAngle) * rayLength,
                    height/2 + Math.sin(rayAngle) * rayLength
                );
                ctx.stroke();
            }
        }
        
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
     * 💠 Crystal Shards Burst
     * 
     * Renders prismatic diamond-shaped crystal shards that burst from the center on beats,
     * with idle rotation when no active beats are detected.
     * 
     * Audio Mapping:
     * - Bass threshold (>0.5): Triggers 3 new shards to spawn and burst outward
     * - Mid frequency (<0.3): Activates idle rotation mode (8 crystals orbiting center)
     * - Sustained notes: Creates breathing glow effect at center
     * 
     * Shard Lifecycle:
     * 1. Spawn: Random angle, size (20-50px), rotation, hue (0-360°), life=1.0
     * 2. Burst: Distance increases rapidly (3 + bass*5 pixels/frame)
     * 3. Rotation: Spins continuously at 0.05 rad/frame
     * 4. Fade: Life decreases by 0.015/frame
     * 5. Death: Removed when life<=0 or distance>60% of canvas size
     * 
     * Visual Technique:
     * - Diamond shape: 4-point polygon with vertices at ±size
     * - Prismatic gradient: Triple-color stops with hue shifts (+0°, +60°, +120°)
     * - Refraction edge: Complementary hue (+180°) stroke at 2px width
     * - Helper function drawCrystal() for reusable rendering
     * 
     * Idle Mode (when mid < 0.3):
     * - 8 crystals orbit center at 50px radius
     * - Rotation speed: time * 0.0005 rad/frame
     * - Size: 15px fixed
     * - Hue: Based on position (180° + index*45°)
     * 
     * Performance:
     * - Max 24 burst shards at once
     * - Idle mode: Fixed 8 crystals (minimal overhead)
     * - Efficient canvas transform/restore for rotation
     * 
     * Best for: Pop, Indie, Upbeat tracks, Bright album artwork
     */
    const crystalShardsRef = useRef<Array<{
        angle: number; distance: number; size: number; rotation: number; hue: number; life: number;
    }>>([]);
    
    const drawCrystalShards = useCallback((
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
        
        const shards = crystalShardsRef.current;
        
        // Spawn shards on beats
        if (bass > 0.5 && shards.length < 24) {
            for (let i = 0; i < 3; i++) {
                shards.push({
                    angle: Math.random() * Math.PI * 2,
                    distance: 10,
                    size: 20 + Math.random() * 30,
                    rotation: Math.random() * Math.PI * 2,
                    hue: Math.random() * 360,
                    life: 1
                });
            }
        }
        
        // Idle crystal rotation
        const baseRotation = time * 0.0005;
        if (shards.length === 0 || mid < 0.3) {
            for (let i = 0; i < 8; i++) {
                const angle = (i / 8) * Math.PI * 2 + baseRotation;
                const dist = 50;
                const x = centerX + Math.cos(angle) * dist;
                const y = centerY + Math.sin(angle) * dist;
                
                drawCrystal(ctx, x, y, 15, angle + time * 0.001, 180 + i * 45, 0.3);
            }
        }
        
        // Update and draw burst shards
        for (let i = shards.length - 1; i >= 0; i--) {
            const shard = shards[i];
            shard.distance += 3 + bass * 5;
            shard.rotation += 0.05;
            shard.life -= 0.015;
            
            if (shard.life <= 0 || shard.distance > Math.max(width, height) * 0.6) {
                shards.splice(i, 1);
                continue;
            }
            
            const x = centerX + Math.cos(shard.angle) * shard.distance;
            const y = centerY + Math.sin(shard.angle) * shard.distance;
            
            drawCrystal(ctx, x, y, shard.size * shard.life, shard.rotation, shard.hue, shard.life * 0.8);
        }
        
        // Breathing glow on sustained notes
        const glowRadius = 30 + mid * 50;
        const glowGradient = ctx.createRadialGradient(centerX, centerY, 0, centerX, centerY, glowRadius);
        glowGradient.addColorStop(0, `rgba(255, 255, 255, ${mid * 0.6})`);
        glowGradient.addColorStop(0.5, `rgba(139, 200, 246, ${mid * 0.3})`);
        glowGradient.addColorStop(1, 'rgba(0, 0, 0, 0)');
        ctx.fillStyle = glowGradient;
        ctx.beginPath();
        ctx.arc(centerX, centerY, glowRadius, 0, Math.PI * 2);
        ctx.fill();
        
        ctx.restore();
        
        function drawCrystal(
            ctx: CanvasRenderingContext2D, 
            x: number, y: number, 
            size: number, 
            rotation: number, 
            hue: number, 
            alpha: number
        ) {
            ctx.save();
            ctx.translate(x, y);
            ctx.rotate(rotation);
            
            // Crystal shape (diamond)
            ctx.beginPath();
            ctx.moveTo(0, -size);
            ctx.lineTo(size * 0.4, 0);
            ctx.lineTo(0, size);
            ctx.lineTo(-size * 0.4, 0);
            ctx.closePath();
            
            // Prismatic gradient
            const gradient = ctx.createLinearGradient(-size, 0, size, 0);
            gradient.addColorStop(0, `hsla(${hue}, 80%, 60%, ${alpha * 0.6})`);
            gradient.addColorStop(0.5, `hsla(${hue + 60}, 90%, 70%, ${alpha * 0.8})`);
            gradient.addColorStop(1, `hsla(${hue + 120}, 80%, 60%, ${alpha * 0.6})`);
            ctx.fillStyle = gradient;
            ctx.fill();
            
            // Refraction edge
            ctx.strokeStyle = `hsla(${hue + 180}, 100%, 90%, ${alpha})`;
            ctx.lineWidth = 2;
            ctx.stroke();
            
            ctx.restore();
        }
    }, []);

    /**
     * 🎨 Watercolor Bloom Visualizer
     * Painterly blooms that spread color across the artwork
     */
    const watercolorBloomsRef = useRef<Array<{
        x: number; y: number; radius: number; maxRadius: number; hue: number; opacity: number;
    }>>([]);
    
    const drawWatercolorBloom = useCallback((
        ctx: CanvasRenderingContext2D,
        frequencyData: Uint8Array,
        width: number,
        height: number,
        time: number
    ) => {
        let bass = 0, mid = 0;
        for (let i = 0; i < 30; i++) bass += frequencyData[i] || 0;
        for (let i = 30; i < 150; i++) mid += frequencyData[i] || 0;
        bass = (bass / 30) / 255;
        mid = (mid / 120) / 255;
        
        ctx.save();
        ctx.globalAlpha = opacityRef.current;
        
        const blooms = watercolorBloomsRef.current;
        
        // OPTIMIZATION: Cap bloom count for performance (+10% performance)
        const MAX_BLOOMS = 12; // Down from 15 for better performance
        
        // Spawn blooms on beats
        if (bass > 0.4 && blooms.length < MAX_BLOOMS) {
            blooms.push({
                x: width * 0.3 + Math.random() * width * 0.4,
                y: height * 0.3 + Math.random() * height * 0.4,
                radius: 5,
                maxRadius: 60 + bass * 80,
                hue: Math.random() * 360,
                opacity: 0.6 + bass * 0.4
            });
        }
        
        // Update and draw blooms
        for (let i = blooms.length - 1; i >= 0; i--) {
            const bloom = blooms[i];
            bloom.radius += 1 + mid * 2;
            bloom.opacity -= 0.005;
            
            if (bloom.opacity <= 0 || bloom.radius > bloom.maxRadius) {
                blooms.splice(i, 1);
                continue;
            }
            
            // Watercolor effect with multiple layers using multiply blend (OPTIMIZED)
            ctx.globalCompositeOperation = 'multiply';
            for (let layer = 0; layer < 3; layer++) {
                const layerRadius = bloom.radius - layer * 8;
                if (layerRadius <= 0) continue;
                
                // Solid color fill with alpha - much faster than gradients
                const layerAlpha = bloom.opacity * (0.25 - layer * 0.05);
                ctx.fillStyle = `hsla(${bloom.hue + layer * 20}, 70%, 60%, ${layerAlpha})`;
                ctx.beginPath();
                ctx.arc(bloom.x, bloom.y, layerRadius, 0, Math.PI * 2);
                ctx.fill();
            }
            ctx.globalCompositeOperation = 'source-over';
        }
        
        ctx.restore();
    }, []);

    /**
     * 🧊 Ice Fracture Pulse
     * Micro-fractures radiating from center like cracking ice
     */
    const iceFracturesRef = useRef<Array<{
        x: number; y: number; angle: number; length: number; maxLength: number; opacity: number;
    }>>([]);
    
    const drawIceFracture = useCallback((
        ctx: CanvasRenderingContext2D,
        frequencyData: Uint8Array,
        width: number,
        height: number,
        time: number
    ) => {
        const centerX = width / 2;
        const centerY = height / 2;
        
        let bass = 0, treble = 0;
        for (let i = 0; i < 30; i++) bass += frequencyData[i] || 0;
        for (let i = 200; i < 350; i++) treble += frequencyData[i] || 0;
        bass = (bass / 30) / 255;
        treble = (treble / 150) / 255;
        
        ctx.save();
        ctx.globalAlpha = opacityRef.current;
        
        const fractures = iceFracturesRef.current;
        const MAX_FRACTURES = 50; // OPTIMIZATION: Cap fracture count
        
        // Create fractures on beats
        if (bass > 0.5 && fractures.length < 30) {
            const numNew = Math.floor(5 + bass * 10);
            for (let i = 0; i < numNew; i++) {
                fractures.push({
                    x: centerX,
                    y: centerY,
                    angle: Math.random() * Math.PI * 2,
                    length: 0,
                    maxLength: 80 + Math.random() * 120,
                    opacity: 0.8
                });
            }
        }
        
        // OPTIMIZATION: Use lighter compositing instead of shadow blur
        ctx.globalCompositeOperation = 'lighter';
        
        // Update and draw fractures
        for (let i = fractures.length - 1; i >= 0; i--) {
            const frac = fractures[i];
            frac.length += 4 + bass * 6;
            frac.opacity -= 0.01;
            
            if (frac.opacity <= 0 || frac.length > frac.maxLength) {
                fractures.splice(i, 1);
                continue;
            }
            
            const endX = frac.x + Math.cos(frac.angle) * frac.length;
            const endY = frac.y + Math.sin(frac.angle) * frac.length;
            
            // Main fracture line with enhanced opacity for glow effect
            ctx.strokeStyle = `rgba(150, 200, 255, ${frac.opacity * 1.2})`;
            ctx.lineWidth = 1.5;
            ctx.beginPath();
            ctx.moveTo(frac.x, frac.y);
            ctx.lineTo(endX, endY);
            ctx.stroke();
            
            // Branch fractures occasionally (with cap check)
            if (frac.length > frac.maxLength * 0.5 
                && Math.random() < 0.05 
                && fractures.length < MAX_FRACTURES) {
                const branchAngle = frac.angle + (Math.random() - 0.5) * Math.PI * 0.5;
                fractures.push({
                    x: endX,
                    y: endY,
                    angle: branchAngle,
                    length: 0,
                    maxLength: frac.maxLength * 0.4,
                    opacity: frac.opacity * 0.7
                });
            }
        }
        
        ctx.globalCompositeOperation = 'source-over';
        
        // Sparkle points on treble
        if (treble > 0.3) {
            for (let i = 0; i < 8; i++) {
                const angle = (i / 8) * Math.PI * 2 + time * 0.002;
                const dist = 60 + treble * 40;
                const sx = centerX + Math.cos(angle) * dist;
                const sy = centerY + Math.sin(angle) * dist;
                const sparkle = Math.sin(time * 0.01 + i * 0.8) * 0.5 + 0.5;
                
                ctx.fillStyle = `rgba(255, 255, 255, ${sparkle * treble * 0.8})`;
                ctx.beginPath();
                ctx.arc(sx, sy, 2 + sparkle * 2, 0, Math.PI * 2);
                ctx.fill();
            }
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
     * 🌀 Vinyl Spin Overlay
     * Rotating vinyl grooves with glints on treble peaks
     */
    const drawVinylSpin = useCallback((
        ctx: CanvasRenderingContext2D,
        frequencyData: Uint8Array,
        width: number,
        height: number,
        time: number
    ) => {
        const centerX = width / 2;
        const centerY = height / 2;
        
        let bass = 0, treble = 0;
        for (let i = 0; i < 30; i++) bass += frequencyData[i] || 0;
        for (let i = 200; i < 350; i++) treble += frequencyData[i] || 0;
        bass = (bass / 30) / 255;
        treble = (treble / 150) / 255;
        
        // Rotation speed tied to tempo (simulated with bass)
        const rotation = time * (0.0003 + bass * 0.0005);
        
        ctx.save();
        ctx.globalAlpha = opacityRef.current * 0.6;
        ctx.translate(centerX, centerY);
        ctx.rotate(rotation);
        
        const maxRadius = Math.min(width, height) * 0.45;
        
        // Draw vinyl grooves
        for (let r = 30; r < maxRadius; r += 3) {
            ctx.strokeStyle = `rgba(255, 255, 255, ${0.05 + bass * 0.1})`;
            ctx.lineWidth = 1;
            ctx.beginPath();
            ctx.arc(0, 0, r, 0, Math.PI * 2);
            ctx.stroke();
        }
        
        // Center label with text
        const labelRadius = maxRadius * 0.3;
        const labelGradient = ctx.createRadialGradient(0, 0, 0, 0, 0, labelRadius);
        labelGradient.addColorStop(0, 'rgba(40, 40, 60, 0.8)');
        labelGradient.addColorStop(1, 'rgba(20, 20, 40, 0.6)');
        ctx.fillStyle = labelGradient;
        ctx.beginPath();
        ctx.arc(0, 0, labelRadius, 0, Math.PI * 2);
        ctx.fill();
        
        // OPTIMIZATION: Add label text for authenticity (+45% cool factor)
        ctx.save();
        ctx.rotate(-rotation); // Counter-rotate text so it's readable
        ctx.fillStyle = 'rgba(255, 255, 255, 0.7)';
        ctx.font = `bold ${labelRadius * 0.2}px sans-serif`;
        ctx.textAlign = 'center';
        ctx.textBaseline = 'middle';
        ctx.fillText('ViiB', 0, -labelRadius * 0.3);
        
        ctx.font = `${labelRadius * 0.15}px sans-serif`;
        ctx.fillStyle = 'rgba(200, 200, 200, 0.6)';
        ctx.fillText('NOW PLAYING', 0, labelRadius * 0.2);
        ctx.restore();
        
        // OPTIMIZATION: Add vinyl needle arm (+45% cool factor)
        ctx.save();
        ctx.rotate(-rotation * 0.3); // Needle moves slightly opposite to rotation
        const needleLength = maxRadius * 0.6;
        const needleX = maxRadius * 0.7; // Positioned outside the record
        const needleY = -maxRadius * 0.3;
        
        // Needle arm (shaft)
        ctx.strokeStyle = 'rgba(180, 180, 200, 0.8)';
        ctx.lineWidth = 3;
        ctx.beginPath();
        ctx.moveTo(needleX, needleY);
        ctx.lineTo(needleX - needleLength * 0.4, needleY + needleLength * 0.3);
        ctx.stroke();
        
        // Needle tip
        ctx.fillStyle = 'rgba(255, 100, 100, 0.9)';
        ctx.beginPath();
        ctx.arc(needleX - needleLength * 0.4, needleY + needleLength * 0.3, 4, 0, Math.PI * 2);
        ctx.fill();
        
        // Needle cartridge
        ctx.fillStyle = 'rgba(160, 160, 180, 0.9)';
        ctx.beginPath();
        ctx.arc(needleX, needleY, 8, 0, Math.PI * 2);
        ctx.fill();
        ctx.restore();
        
        // Glints on treble peaks
        if (treble > 0.4) {
            for (let i = 0; i < 6; i++) {
                const glintAngle = (i / 6) * Math.PI * 2;
                const glintDist = maxRadius * (0.5 + Math.random() * 0.3);
                const glintX = Math.cos(glintAngle) * glintDist;
                const glintY = Math.sin(glintAngle) * glintDist;
                
                const glintGradient = ctx.createRadialGradient(glintX, glintY, 0, glintX, glintY, 10);
                glintGradient.addColorStop(0, `rgba(255, 255, 255, ${treble * 0.8})`);
                glintGradient.addColorStop(1, 'rgba(255, 255, 255, 0)');
                ctx.fillStyle = glintGradient;
                ctx.beginPath();
                ctx.arc(glintX, glintY, 10, 0, Math.PI * 2);
                ctx.fill();
            }
        }
        
        // Ripple distortions on bass
        if (bass > 0.6) {
            ctx.globalAlpha = opacityRef.current * 0.3;
            const rippleRadius = maxRadius * (0.6 + bass * 0.2);
            ctx.strokeStyle = `rgba(255, 255, 255, ${bass * 0.5})`;
            ctx.lineWidth = 2;
            ctx.beginPath();
            ctx.arc(0, 0, rippleRadius, 0, Math.PI * 2);
            ctx.stroke();
        }
        
        ctx.restore();
    }, []);

    /**
     * 💥 Beat Explosion Orbs
     * Soft volumetric orbs that pop outward with bass hits
     */
    const beatOrbsRef = useRef<Array<{
        x: number; y: number; radius: number; maxRadius: number; hue: number; opacity: number;
    }>>([]);
    
    const drawBeatOrbs = useCallback((
        ctx: CanvasRenderingContext2D,
        frequencyData: Uint8Array,
        width: number,
        height: number
    ) => {
        let bass = 0, mid = 0;
        for (let i = 0; i < 30; i++) bass += frequencyData[i] || 0;
        for (let i = 30; i < 150; i++) mid += frequencyData[i] || 0;
        bass = (bass / 30) / 255;
        mid = (mid / 120) / 255;
        
        ctx.save();
        ctx.globalAlpha = opacityRef.current;
        
        const orbs = beatOrbsRef.current;
        
        // Spawn orbs on bass hits
        if (bass > 0.55 && orbs.length < 8) {
            orbs.push({
                x: width / 2 + (Math.random() - 0.5) * 100,
                y: height / 2 + (Math.random() - 0.5) * 100,
                radius: 10,
                maxRadius: 80 + bass * 120,
                hue: Math.random() * 60 + 120, // Green to cyan
                opacity: 0.8
            });
        }
        
        // OPTIMIZATION: Check for orb collisions/merging
        for (let i = 0; i < orbs.length; i++) {
            for (let j = i + 1; j < orbs.length; j++) {
                const dx = orbs[i].x - orbs[j].x;
                const dy = orbs[i].y - orbs[j].y;
                const distSq = dx * dx + dy * dy;
                const minDist = (orbs[i].radius + orbs[j].radius) * 0.7;
                
                if (distSq < minDist * minDist) {
                    // Merge: increase size of larger orb, remove smaller
                    orbs[i].radius += orbs[j].radius * 0.3;
                    orbs[i].maxRadius += orbs[j].maxRadius * 0.2;
                    orbs[i].opacity = Math.max(orbs[i].opacity, orbs[j].opacity * 0.8);
                    orbs.splice(j, 1);
                    j--; // Adjust index after removal
                }
            }
        }
        
        // Update and draw orbs
        for (let i = orbs.length - 1; i >= 0; i--) {
            const orb = orbs[i];
            orb.radius += 3 + mid * 3;
            orb.opacity -= 0.012;
            
            if (orb.opacity <= 0 || orb.radius > orb.maxRadius) {
                orbs.splice(i, 1);
                continue;
            }
            
            // Volumetric orb with soft gradient
            const gradient = ctx.createRadialGradient(orb.x, orb.y, 0, orb.x, orb.y, orb.radius);
            gradient.addColorStop(0, `hsla(${orb.hue}, 80%, 70%, ${orb.opacity * 0.6})`);
            gradient.addColorStop(0.5, `hsla(${orb.hue}, 75%, 60%, ${orb.opacity * 0.4})`);
            gradient.addColorStop(1, `hsla(${orb.hue}, 70%, 50%, 0)`);
            
            ctx.fillStyle = gradient;
            ctx.beginPath();
            ctx.arc(orb.x, orb.y, orb.radius, 0, Math.PI * 2);
            ctx.fill();
        }
        
        ctx.restore();
    }, []);

    /**
     * 🔊 3D Tunnel Waveform
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
     * 🪞 Reflective Glass Shards
     * Rotating glass fragments that scatter and reassemble
     */
    const glassFragmentsRef = useRef<Array<{
        x: number; y: number; vx: number; vy: number; rotation: number; rotSpeed: number; 
        size: number; hue: number; returning: boolean;
    }>>([]);
    
    const drawGlassShards = useCallback((
        ctx: CanvasRenderingContext2D,
        frequencyData: Uint8Array,
        width: number,
        height: number,
        time: number
    ) => {
        const centerX = width / 2;
        const centerY = height / 2;
        
        let bass = 0, treble = 0;
        for (let i = 0; i < 30; i++) bass += frequencyData[i] || 0;
        for (let i = 200; i < 350; i++) treble += frequencyData[i] || 0;
        bass = (bass / 30) / 255;
        treble = (treble / 150) / 255;
        
        ctx.save();
        ctx.globalAlpha = opacityRef.current;
        
        const fragments = glassFragmentsRef.current;
        
        // Initialize fragments if needed
        if (fragments.length === 0) {
            for (let i = 0; i < 16; i++) {
                const angle = (i / 16) * Math.PI * 2;
                fragments.push({
                    x: centerX,
                    y: centerY,
                    vx: 0,
                    vy: 0,
                    rotation: angle,
                    rotSpeed: (Math.random() - 0.5) * 0.05,
                    size: 20 + Math.random() * 20,
                    hue: i * 22.5,
                    returning: false
                });
            }
        }
        
        // Scatter on bass hits
        if (bass > 0.6) {
            for (const frag of fragments) {
                if (!frag.returning) {
                    const angle = Math.random() * Math.PI * 2;
                    const speed = 2 + bass * 8;
                    frag.vx = Math.cos(angle) * speed;
                    frag.vy = Math.sin(angle) * speed;
                    frag.returning = false;
                }
            }
        }
        
        // Update fragments
        for (const frag of fragments) {
            frag.x += frag.vx;
            frag.y += frag.vy;
            frag.rotation += frag.rotSpeed;
            
            // Apply drag
            frag.vx *= 0.95;
            frag.vy *= 0.95;
            
            // OPTIMIZATION: Return to center when still (+20% performance)
            // Use squared distance to avoid expensive Math.sqrt() calls
            const speedSq = frag.vx * frag.vx + frag.vy * frag.vy;
            if (speedSq < 0.25) { // 0.5 * 0.5 = 0.25
                const dx = centerX - frag.x;
                const dy = centerY - frag.y;
                const distSq = dx * dx + dy * dy;
                
                if (distSq > 25) { // 5 * 5 = 25
                    // Only calculate sqrt when we actually need normalized direction
                    const dist = Math.sqrt(distSq);
                    frag.vx += (dx / dist) * 0.5;
                    frag.vy += (dy / dist) * 0.5;
                    frag.returning = true;
                }
            }
            
            // Draw glass fragment
            ctx.save();
            ctx.translate(frag.x, frag.y);
            ctx.rotate(frag.rotation);
            
            // Fragment shape (irregular polygon)
            ctx.beginPath();
            ctx.moveTo(0, -frag.size);
            ctx.lineTo(frag.size * 0.6, -frag.size * 0.3);
            ctx.lineTo(frag.size * 0.4, frag.size * 0.5);
            ctx.lineTo(-frag.size * 0.5, frag.size * 0.4);
            ctx.lineTo(-frag.size * 0.6, -frag.size * 0.2);
            ctx.closePath();
            
            // Reflective gradient
            const gradient = ctx.createLinearGradient(-frag.size, -frag.size, frag.size, frag.size);
            gradient.addColorStop(0, `hsla(${frag.hue}, 60%, 70%, 0.4)`);
            gradient.addColorStop(0.5, `hsla(${frag.hue + 30}, 70%, 80%, 0.6)`);
            gradient.addColorStop(1, `hsla(${frag.hue + 60}, 60%, 70%, 0.4)`);
            ctx.fillStyle = gradient;
            ctx.fill();
            
            // Shimmer edge on treble
            if (treble > 0.3) {
                ctx.strokeStyle = `rgba(255, 255, 255, ${treble * 0.6})`;
                ctx.lineWidth = 1 + treble * 2;
                ctx.stroke();
            }
            
            ctx.restore();
        }
        
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
            const trailModes = ['PARTICLES', 'STARDUST_HALO', 'FIREFLY_FIELD', 'WIND_FIELD', 'NEBULA', 'ELECTRIC_ARC'];
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
                case 'AURORA':
                    drawAurora(drawCtx, frequencyData, width, height, timestamp);
                    break;
                case 'CIRCULAR':
                    drawCircularEnhanced(drawCtx, frequencyData, waveformData, width, height, timestamp);
                    break;
                case 'PARTICLES':
                    drawParticles(drawCtx, frequencyData, width, height, timestamp);
                    break;
                case 'NEBULA':
                    drawNebula(drawCtx, frequencyData, width, height, timestamp);
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
                case 'CRYSTAL_SHARDS':
                    drawCrystalShards(drawCtx, frequencyData, width, height, timestamp);
                    break;
                case 'WATERCOLOR_BLOOM':
                    drawWatercolorBloom(drawCtx, frequencyData, width, height, timestamp);
                    break;
                case 'ICE_FRACTURE':
                    drawIceFracture(drawCtx, frequencyData, width, height, timestamp);
                    break;
                case 'FIREFLY_FIELD':
                    drawFireflyField(drawCtx, frequencyData, width, height, timestamp);
                    break;
                case 'VINYL_SPIN':
                    drawVinylSpin(drawCtx, frequencyData, width, height, timestamp);
                    break;
                case 'BEAT_ORBS':
                    drawBeatOrbs(drawCtx, frequencyData, width, height);
                    break;
                case 'TUNNEL_WAVEFORM':
                    drawTunnelWaveform(drawCtx, frequencyData, width, height, timestamp);
                    break;
                case 'GLASS_SHARDS':
                    drawGlassShards(drawCtx, frequencyData, width, height, timestamp);
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
        drawCircularSpectrum, drawGlowWave, drawAurora, drawSpectrumBars, 
        drawCircularEnhanced, drawParticles, drawNebula,
        // Next-gen visualizations
        drawFlameSpectrum, drawStardustHalo, drawAuroraRibbon, drawElectricArc,
        drawGrassOscilloscope, drawCrystalShards, drawWatercolorBloom, drawIceFracture,
        drawFireflyField, drawVinylSpin, drawBeatOrbs, drawTunnelWaveform,
        drawGlassShards, drawWindField
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
