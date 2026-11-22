/**
 * ViiB MediaHub - Audio Visualizer Component
 * 
 * Canvas-based audio visualization using Web Audio API analyzer data.
 * 
 * Modes:
 * - OFF: No visualization
 * - WAVE: Waveform display (time domain)
 * - SPECTRUM: Frequency bar graph (FFT)
 * 
 * Uses requestAnimationFrame; actual frame cadence depends on the host.
 * Automatically cleans up animation on unmount or mode change.
 * 
 * @module Visualizer
 */

import React, { useEffect, useRef } from 'react';
import { audioEngine } from '../lib/audio';
import { VisualizerMode } from '../types';
import { rgbaFromRgb, VIIB_COLOR_RGB, VIIB_COLOR_VALUES } from './ui/tokens';

interface Props {
  mode: VisualizerMode;
  className?: string;
  barColor?: string;
  active?: boolean;
  lowPower?: boolean;
}

export const Visualizer: React.FC<Props> = ({ mode, className = '', barColor = VIIB_COLOR_VALUES.playbackGreen, active = true, lowPower = false }) => {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const animationRef = useRef<number>(0);

  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas || mode === 'OFF' || !active) return;

    const ctx = canvas.getContext('2d');
    if (!ctx) return;

    let width = 0;
    let height = 0;
    let lastDraw = -Infinity;
    const updateSize = () => {
      width = canvas.clientWidth;
      height = canvas.clientHeight;
      const dpr = lowPower ? 1 : Math.min(window.devicePixelRatio || 1, 2);
      const pixelsWide = Math.round(width * dpr);
      const pixelsHigh = Math.round(height * dpr);
      if (canvas.width !== pixelsWide || canvas.height !== pixelsHigh) {
        canvas.width = pixelsWide;
        canvas.height = pixelsHigh;
        ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
      }
      if (width && height && !animationRef.current) animationRef.current = requestAnimationFrame(draw);
    };

    // Data buffer
    // Analyser frequencyBinCount is typically 1024
    const bufferLength = 1024; 
    const dataArray = new Uint8Array(bufferLength);

    const draw = (time: number) => {
      animationRef.current = 0;
      if (!width || !height) return;
      animationRef.current = requestAnimationFrame(draw);
      if (lowPower && time - lastDraw < 1000 / 30 - 1) return;
      lastDraw = time;
      const analyser = audioEngine.getAnalyser();

      ctx.clearRect(0, 0, width, height);

      // If no analyser yet (audio engine not init) or not running, just clear and wait
      if (!analyser) {
          return;
      }

      if (mode === 'SPECTRUM') {
        analyser.getByteFrequencyData(dataArray);
        
        const gap = 2;
        const barWidth = 3;
        const totalBarWidth = barWidth + gap;
        const numBars = Math.floor(width / totalBarWidth);
        
        // Use first ~2/3 of bins which usually contain the audible energy
        const effectiveBufferLength = Math.floor(analyser.frequencyBinCount * 0.7);
        const step = Math.max(1, Math.floor(effectiveBufferLength / numBars));

        let x = 0;

        for (let i = 0; i < numBars; i++) {
          let sum = 0;
          let count = 0;
          
          for (let j = 0; j < step; j++) {
            const index = (i * step) + j;
            if (index < dataArray.length) {
                sum += dataArray[index];
                count++;
            }
          }
          
          const average = count > 0 ? sum / count : 0;
          const val = Math.max(0, average); 
          const barHeight = (val / 255) * height;

          ctx.fillStyle = barColor;
          
          if (typeof ctx.roundRect === 'function') {
             ctx.beginPath();
             ctx.roundRect(x, height - barHeight, barWidth, barHeight, [2, 2, 0, 0]);
             ctx.fill();
          } else {
             ctx.fillRect(x, height - barHeight, barWidth, barHeight);
          }
          
          x += totalBarWidth;
        }
      } else if (mode === 'WAVE') {
        analyser.getByteTimeDomainData(dataArray);
        ctx.lineWidth = 2;
        ctx.strokeStyle = barColor;
        ctx.beginPath();

        const sliceWidth = width * 1.0 / analyser.frequencyBinCount;
        let x = 0;

        for (let i = 0; i < analyser.frequencyBinCount; i++) {
          const v = dataArray[i] / 128.0;
          const y = v * height / 2;

          if (i === 0) ctx.moveTo(x, y);
          else ctx.lineTo(x, y);

          x += sliceWidth;
        }

        ctx.lineTo(width, height / 2);
        ctx.stroke();
      }
    };

    const resizeObserver = new ResizeObserver(updateSize);
    resizeObserver.observe(canvas);
    updateSize();

    return () => {
      if (animationRef.current) cancelAnimationFrame(animationRef.current);
      animationRef.current = 0;
      resizeObserver.disconnect();
    };
  }, [mode, barColor, active, lowPower]);

  if (mode === 'OFF') return null;

  return <canvas ref={canvasRef} className={`block w-full h-full ${className}`} />;
};