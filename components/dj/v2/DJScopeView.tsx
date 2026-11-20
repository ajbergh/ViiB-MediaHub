/**
 * ViiB MediaHub - DJ Scope View Component
 * 
 * Real-time VU meter visualization rendered as vertical bars on a canvas.
 * Polls the supplied getVULevels callback in its animation loop; drawing does not set React state.
 * 
 * @module components/dj/v2/DJScopeView
 */

import React, { useRef, useEffect } from 'react';
import { djVisualScheduler } from '../../../lib/djVisualScheduler';

interface VULevels {
  deckA: { left: number; right: number };
  deckB: { left: number; right: number };
  master: { left: number; right: number };
}

interface DJScopeViewProps {
  getVULevels: () => VULevels;
}

const BAR_DEFS = [
  { label: 'A-L', color: '#3b82f6' },
  { label: 'A-R', color: '#3b82f6' },
  { label: 'M-L', color: '#22c55e' },
  { label: 'M-R', color: '#22c55e' },
  { label: 'B-L', color: '#8b5cf6' },
  { label: 'B-R', color: '#8b5cf6' },
] as const;

export const DJScopeView = React.memo(({ getVULevels }: DJScopeViewProps) => {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const getLevelsRef = useRef(getVULevels);
  getLevelsRef.current = getVULevels;

  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    const ctx = canvas.getContext('2d');
    if (!ctx) return;

    let cachedW = 0;
    let cachedH = 0;
    let cachedDpr = 0;
    let barWidth = 0;
    let maxH = 0;
    let barX: number[] = [];
    let gradients: CanvasGradient[] = [];
    let offscreen: HTMLCanvasElement | null = null;
    let forceDraw = true;

    const displayed = new Float64Array(BAR_DEFS.length);
    const lastDrawn = new Float64Array(BAR_DEFS.length).fill(-1);

    const ensureCache = (w: number, h: number, dpr: number): boolean => {
      if (w <= 0 || h <= 0) return false;
      if (cachedW === w && cachedH === h && cachedDpr === dpr && offscreen) {
        return true;
      }

      cachedW = w;
      cachedH = h;
      cachedDpr = dpr;

      const pixelW = Math.max(1, Math.round(w * dpr));
      const pixelH = Math.max(1, Math.round(h * dpr));
      canvas.width = pixelW;
      canvas.height = pixelH;
      ctx.setTransform(dpr, 0, 0, dpr, 0, 0);

      offscreen = document.createElement('canvas');
      offscreen.width = pixelW;
      offscreen.height = pixelH;
      const offCtx = offscreen.getContext('2d');

      barWidth = Math.max(1, Math.min(60, (w - 80) / BAR_DEFS.length));
      const gap = 8;
      const totalWidth = BAR_DEFS.length * (barWidth + gap) - gap;
      const startX = (w - totalWidth) / 2;
      maxH = Math.max(0, h - 30);

      barX = new Array(BAR_DEFS.length);
      gradients = new Array(BAR_DEFS.length);

      if (offCtx) {
        offCtx.setTransform(dpr, 0, 0, dpr, 0, 0);
        offCtx.fillStyle = '#0d0d0d';
        offCtx.fillRect(0, 0, w, h);
      }

      BAR_DEFS.forEach((bar, i) => {
        const x = startX + i * (barWidth + gap);
        barX[i] = x;

        const gradient = ctx.createLinearGradient(x, h - 15, x, h - 15 - maxH);
        gradient.addColorStop(0, bar.color);
        gradient.addColorStop(0.6, bar.color);
        gradient.addColorStop(0.85, '#eab308');
        gradient.addColorStop(1, '#ef4444');
        gradients[i] = gradient;

        if (offCtx) {
          offCtx.fillStyle = '#1a1a1a';
          offCtx.fillRect(x, h - 15 - maxH, barWidth, maxH);

          offCtx.fillStyle = '#666';
          offCtx.font = '9px system-ui';
          offCtx.textAlign = 'center';
          offCtx.fillText(bar.label, x + barWidth / 2, h - 3);
        }
      });

      if (offCtx) {
        offCtx.fillStyle = '#444';
        offCtx.font = 'bold 10px system-ui';
        offCtx.textAlign = 'center';
        offCtx.fillText('SCOPE VIEW — AUDIO LEVELS', w / 2, 14);
      }

      forceDraw = true;
      return true;
    };

    const drawFrame = (elapsed: number): boolean => {
      const w = canvas.clientWidth;
      const h = canvas.clientHeight;
      const dpr = window.devicePixelRatio || 1;
      if (!ensureCache(w, h, dpr) || !offscreen) return false;

      const levels = getLevelsRef.current();
      const raw = [
        levels.deckA.left,
        levels.deckA.right,
        levels.master.left,
        levels.master.right,
        levels.deckB.left,
        levels.deckB.right,
      ];

      const steps = elapsed / (1000 / 60);
      let active = false;
      let changed = forceDraw;

      for (let i = 0; i < BAR_DEFS.length; i++) {
        const target = Number.isFinite(raw[i]) ? Math.max(0, Math.min(1, raw[i])) : 0;
        const factor = 1 - Math.pow(target > displayed[i] ? 0.55 : 0.88, steps);
        displayed[i] += (target - displayed[i]) * factor;
        if (displayed[i] < 0.001 && target < 0.001) displayed[i] = 0;

        if (displayed[i] > 0.002 || target > 0.002) active = true;

        const barH = displayed[i] * maxH;
        if (
          Math.abs(barH - lastDrawn[i]) >= 0.25
          || (barH === 0 && lastDrawn[i] !== 0)
        ) {
          changed = true;
        }
      }

      if (!changed) return active;

      forceDraw = false;
      ctx.drawImage(offscreen, 0, 0, w, h);

      for (let i = 0; i < BAR_DEFS.length; i++) {
        const barH = displayed[i] * maxH;
        lastDrawn[i] = barH;
        if (barH > 0) {
          ctx.fillStyle = gradients[i];
          ctx.fillRect(barX[i], h - 15 - barH, barWidth, barH);
        }
      }

      return active;
    };

    const resizeObserver = typeof ResizeObserver !== 'undefined'
      ? new ResizeObserver(() => {
        forceDraw = true;
        drawFrame(1000 / 60);
      })
      : null;
    resizeObserver?.observe(canvas);

    const unsubscribe = djVisualScheduler.subscribe((_now, elapsed) => drawFrame(elapsed));
    return () => {
      unsubscribe();
      resizeObserver?.disconnect();
    };
  }, []);

  return <canvas ref={canvasRef} className='w-full h-full' />;
});

DJScopeView.displayName = 'DJScopeView';

export default DJScopeView;
