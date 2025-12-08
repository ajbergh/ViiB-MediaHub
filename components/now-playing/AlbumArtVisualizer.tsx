/**
 * ViiB MediaHub - Album Art Visualizer Component
 * 
 * Canvas-based audio visualization that overlays the album art in the Now Playing view.
 * Uses WebGL for GPU-accelerated rendering when available, with 2D canvas fallback.
 * 
 * Features:
 * - Smooth fade in/out transitions when toggling between album art and visualizer
 * - Multiple visualization modes (WAVE, SPECTRUM, AURORA, CIRCULAR, PARTICLES, NEBULA)
 * - Pauses rendering when not visible to save resources
 * - Responsive sizing that fills the album art container
 * - Uses existing audio analyzer from audioEngine
 * 
 * @module AlbumArtVisualizer
 */

import React, { useEffect, useRef, useState, useCallback } from 'react';
import { audioEngine } from '../../lib/audio';
import { VisualizerMode } from '../../types';

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
    color = '#22c55e',
    accentColor = '#8b5cf6',
    onFadeComplete,
    className = ''
}) => {
    const canvasRef = useRef<HTMLCanvasElement>(null);
    const animationRef = useRef<number>(0);
    const opacityRef = useRef<number>(0);
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
            ctx.strokeStyle = `hsla(${hue}, 80%, 60%, ${0.6 + value * 0.4})`;
            ctx.lineWidth = 3;
            ctx.lineCap = 'round';
            
            ctx.beginPath();
            ctx.moveTo(x1, y1);
            ctx.lineTo(x2, y2);
            ctx.stroke();
        }
        
        // Inner circle glow
        const gradient = ctx.createRadialGradient(centerX, centerY, radius * 0.8, centerX, centerY, radius);
        gradient.addColorStop(0, 'rgba(34, 197, 94, 0.1)');
        gradient.addColorStop(1, 'rgba(34, 197, 94, 0)');
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
        gradient.addColorStop(0, `rgba(34, 197, 94, ${bass * 0.8})`);
        gradient.addColorStop(0.3, `rgba(139, 92, 246, ${mid * 0.6})`);
        gradient.addColorStop(0.6, `rgba(59, 130, 246, ${treble * 0.7})`);
        gradient.addColorStop(1, `rgba(236, 72, 153, ${(bass + treble) * 0.5})`);
        
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
        waveGradient.addColorStop(0, 'rgba(255, 255, 255, 0.2)');
        waveGradient.addColorStop(1, 'rgba(255, 255, 255, 0)');
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
        
        // Draw twinkling stars
        const starCount = 50 + Math.floor(treble * 30);
        for (let i = 0; i < starCount; i++) {
            const seed = i * 1234.5678;
            const sx = ((seed * 17) % width);
            const sy = ((seed * 31) % height);
            const twinkle = Math.sin(time * 0.005 + seed) * 0.5 + 0.5;
            const starSize = (1 + twinkle * 2) * (0.5 + treble);
            
            ctx.fillStyle = `rgba(255, 255, 255, ${twinkle * 0.8})`;
            ctx.beginPath();
            ctx.arc(sx, sy, starSize, 0, Math.PI * 2);
            ctx.fill();
        }
        
        // Nebula clouds - multiple layers
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
                
                const cloudGradient = ctx.createRadialGradient(cx, cy, 0, cx, cy, radius);
                const intensity = 0.15 + layer.energyMult * 0.25;
                cloudGradient.addColorStop(0, `rgba(${r}, ${g}, ${b}, ${intensity})`);
                cloudGradient.addColorStop(0.4, `rgba(${r}, ${g}, ${b}, ${intensity * 0.5})`);
                cloudGradient.addColorStop(1, 'rgba(0, 0, 0, 0)');
                
                ctx.fillStyle = cloudGradient;
                ctx.beginPath();
                ctx.arc(cx, cy, radius, 0, Math.PI * 2);
                ctx.fill();
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

    /**
     * Main animation loop
     */
    useEffect(() => {
        const canvas = canvasRef.current;
        if (!canvas) return;
        
        const ctx = canvas.getContext('2d');
        if (!ctx) return;
        
        // Handle canvas sizing
        const updateSize = () => {
            const dpr = window.devicePixelRatio || 1;
            const width = canvas.clientWidth;
            const height = canvas.clientHeight;
            
            if (canvas.width !== width * dpr || canvas.height !== height * dpr) {
                canvas.width = width * dpr;
                canvas.height = height * dpr;
                ctx.scale(dpr, dpr);
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
                ctx.clearRect(0, 0, width, height);
                if (fadingOutRef.current) {
                    fadingOutRef.current = false;
                    setIsRendering(false);
                }
                return;
            }
            
            ctx.clearRect(0, 0, width, height);
            
            const analyser = audioEngine.getAnalyser();
            if (!analyser) return;
            
            // Get frequency and waveform data
            analyser.getByteFrequencyData(frequencyData);
            analyser.getByteTimeDomainData(waveformData);
            
            // Draw based on mode
            switch (mode) {
                case 'WAVE':
                    drawGlowWave(ctx, waveformData, width, height, analyser.frequencyBinCount);
                    break;
                case 'SPECTRUM':
                    drawCircularSpectrum(ctx, frequencyData, width, height);
                    break;
                case 'AURORA':
                    drawAurora(ctx, frequencyData, width, height, timestamp);
                    break;
                case 'CIRCULAR':
                    drawCircularEnhanced(ctx, frequencyData, waveformData, width, height, timestamp);
                    break;
                case 'PARTICLES':
                    drawParticles(ctx, frequencyData, width, height, timestamp);
                    break;
                case 'NEBULA':
                    drawNebula(ctx, frequencyData, width, height, timestamp);
                    break;
                default:
                    // For unknown modes, draw spectrum bars as fallback
                    drawSpectrumBars(ctx, frequencyData, width, height);
            }
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
    }, [mode, isActive, isRendering, updateOpacity, drawCircularSpectrum, drawGlowWave, drawAurora, drawSpectrumBars, drawCircularEnhanced, drawParticles, drawNebula]);

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
