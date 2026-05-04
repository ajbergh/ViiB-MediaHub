/**
 * ViiB MediaHub - DJ Scope View Component
 * 
 * Real-time VU meter visualization rendered as vertical bars on a canvas.
 * Self-subscribing via rAF — zero React re-renders during playback.
 * 
 * @module components/dj/v2/DJScopeView
 */

import React, { useRef, useEffect } from 'react';

interface VULevels {
  deckA: { left: number; right: number };
  deckB: { left: number; right: number };
  master: { left: number; right: number };
}

interface DJScopeViewProps {
  getVULevels: () => VULevels;
}

export const DJScopeView = React.memo(({ getVULevels }: DJScopeViewProps) => {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  
  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    
    let animId: number;
    let idleTimeoutId: ReturnType<typeof setTimeout> | undefined;
    const scheduleNext = (idle: boolean) => {
      if (idle) {
        idleTimeoutId = setTimeout(() => { animId = requestAnimationFrame(draw); }, 250);
      } else {
        animId = requestAnimationFrame(draw);
      }
    };

    const draw = () => {
      const ctx = canvas.getContext('2d');
      if (!ctx) { scheduleNext(false); return; }
      
      const w = canvas.clientWidth;
      const h = canvas.clientHeight;
      if (canvas.width !== w * 2) { canvas.width = w * 2; canvas.height = h * 2; ctx.scale(2, 2); }
      
      ctx.fillStyle = '#0d0d0d';
      ctx.fillRect(0, 0, w, h);
      
      const levels = getVULevels();
      const bars = [
        { label: 'A-L', value: levels.deckA.left, color: '#3b82f6' },
        { label: 'A-R', value: levels.deckA.right, color: '#3b82f6' },
        { label: 'M-L', value: levels.master.left, color: '#22c55e' },
        { label: 'M-R', value: levels.master.right, color: '#22c55e' },
        { label: 'B-L', value: levels.deckB.left, color: '#8b5cf6' },
        { label: 'B-R', value: levels.deckB.right, color: '#8b5cf6' },
      ];
      
      const barWidth = Math.min(60, (w - 80) / bars.length);
      const gap = 8;
      const totalWidth = bars.length * (barWidth + gap) - gap;
      const startX = (w - totalWidth) / 2;
      const maxH = h - 30;
      
      bars.forEach((bar, i) => {
        const x = startX + i * (barWidth + gap);
        const barH = bar.value * maxH;
        
        // Bar
        const gradient = ctx.createLinearGradient(x, h - 15, x, h - 15 - maxH);
        gradient.addColorStop(0, bar.color);
        gradient.addColorStop(0.6, bar.color);
        gradient.addColorStop(0.85, '#eab308');
        gradient.addColorStop(1, '#ef4444');
        ctx.fillStyle = gradient;
        ctx.fillRect(x, h - 15 - barH, barWidth, barH);
        
        // Background
        ctx.fillStyle = '#1a1a1a';
        ctx.fillRect(x, h - 15 - maxH, barWidth, maxH - barH);
        
        // Label
        ctx.fillStyle = '#666';
        ctx.font = '9px system-ui';
        ctx.textAlign = 'center';
        ctx.fillText(bar.label, x + barWidth / 2, h - 3);
      });
      
      // Title
      ctx.fillStyle = '#444';
      ctx.font = 'bold 10px system-ui';
      ctx.textAlign = 'center';
      ctx.fillText('SCOPE VIEW — AUDIO LEVELS', w / 2, 14);
      
      const maxLevel = Math.max(
        levels.deckA.left,
        levels.deckA.right,
        levels.deckB.left,
        levels.deckB.right,
        levels.master.left,
        levels.master.right
      );
      const idle = (typeof document !== 'undefined' && document.hidden) || maxLevel < 0.002;
      scheduleNext(idle);
    };
    
    animId = requestAnimationFrame(draw);
    return () => {
      cancelAnimationFrame(animId);
      if (idleTimeoutId) clearTimeout(idleTimeoutId);
    };
  }, [getVULevels]);
  
  return <canvas ref={canvasRef} className='w-full h-full' />;
});

DJScopeView.displayName = 'DJScopeView';

export default DJScopeView;
