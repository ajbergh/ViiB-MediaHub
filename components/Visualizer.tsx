import React, { useEffect, useRef } from 'react';
import { audioEngine } from '../lib/audio';
import { VisualizerMode } from '../types';

interface Props {
  mode: VisualizerMode;
  className?: string;
  barColor?: string;
}

export const Visualizer: React.FC<Props> = ({ mode, className = '', barColor = '#22c55e' }) => {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const animationRef = useRef<number>(0);

  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas || mode === 'OFF') return;

    const ctx = canvas.getContext('2d');
    if (!ctx) return;

    // Handle Resize
    const updateSize = () => {
       const dpr = window.devicePixelRatio || 1;
       // We use clientWidth/Height to get the CSS pixel size of the element
       const width = canvas.clientWidth;
       const height = canvas.clientHeight;
       
       if (canvas.width !== width * dpr || canvas.height !== height * dpr) {
         canvas.width = width * dpr;
         canvas.height = height * dpr;
         ctx.scale(dpr, dpr);
       }
    };

    // Initial size
    updateSize();

    // ResizeObserver is more robust than window.resize
    const resizeObserver = new ResizeObserver(() => {
        updateSize();
    });
    resizeObserver.observe(canvas);

    // Data buffer
    // Analyser frequencyBinCount is typically 1024
    const bufferLength = 1024; 
    const dataArray = new Uint8Array(bufferLength);

    const draw = () => {
      animationRef.current = requestAnimationFrame(draw);
      
      const analyser = audioEngine.getAnalyser();
      
      const width = canvas.clientWidth;
      const height = canvas.clientHeight;
      
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
      } else if (mode === 'AURORA') {
        analyser.getByteFrequencyData(dataArray);
        
        let bass = 0;
        let treble = 0;
        // Safety checks for array bounds
        for (let i = 0; i < 50 && i < dataArray.length; i++) bass += dataArray[i];
        for (let i = 200; i < 400 && i < dataArray.length; i++) treble += dataArray[i];
        
        bass = bass / 50 / 255;
        treble = treble / 200 / 255;

        const gradient = ctx.createLinearGradient(0, 0, width, height);
        gradient.addColorStop(0, `rgba(30, 215, 96, ${bass * 0.6})`);
        gradient.addColorStop(0.5, `rgba(139, 92, 246, ${(bass + treble) * 0.4})`);
        gradient.addColorStop(1, `rgba(59, 130, 246, ${treble * 0.6})`);
        
        ctx.fillStyle = gradient;
        ctx.fillRect(0, 0, width, height);
      }
    };

    draw();

    return () => {
      if (animationRef.current) cancelAnimationFrame(animationRef.current);
      resizeObserver.disconnect();
    };
  }, [mode, barColor]);

  if (mode === 'OFF') return null;

  return <canvas ref={canvasRef} className={`block w-full h-full ${className}`} />;
};