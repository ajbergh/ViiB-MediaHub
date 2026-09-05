/**
 * ViiB MediaHub - DJ FX X-Y Pad Component (v2)
 *
 * Touch-style X-Y performance pad inspired by Pioneer DDJ FX Section
 * and Native Instruments Kontrol pads. Drag inside the pad to morph
 * two effect parameters simultaneously — a staple live-performance
 * gesture for sweeping filters and shaping reverbs.
 *
 * Default mapping:
 *   X axis → filter cutoff (low-pass at left → high-pass at right)
 *   Y axis → filter resonance (bottom = low Q, top = high Q)
 *
 * Target deck is selectable (A / B / Both). When released the pad
 * smoothly returns to centre (filter neutral) so the gesture is
 * non-latching, like a real performance ribbon.
 *
 * @module components/dj/v2/DJFXPad
 */

import React, { useCallback, useEffect, useRef, useState } from 'react';
import { useDJAudioEngineActions } from '../../../hooks/useDJAudioEngine';
import { useStore } from '../../../store';
import type { DeckId } from '../../../slices/djMixerSlice';

type Target = 'A' | 'B' | 'BOTH';

interface DJFXPadProps {
  size?: number;
}

type SetFilterFX = (deck: DeckId, enabled: boolean, type: 'lowpass' | 'highpass', frequency: number, resonance: number) => void;
type SetDeckFilter = (deck: DeckId, value: number) => void;

/**
 * Map normalized X (-1..+1) to a filter (lowpass below 0, highpass above 0)
 * exactly the way `handleFilterChange` in DJModeV2 does it — we re-implement
 * here so the pad and the deck filter knob feel identical.
 */
function applyFilterFor(
  setFilterFX: SetFilterFX,
  setDeckFilter: SetDeckFilter,
  deck: DeckId,
  x: number,         // -1..+1
  yResonance: number // 0..1 (0 = mild, 1 = aggressive)
) {
  const clampedX = Math.max(-1, Math.min(1, x));
  setDeckFilter(deck, clampedX);

  if (Math.abs(clampedX) < 0.05) {
    setFilterFX(deck, false, 'lowpass', 20000, 0.5);
    return;
  }
  const resonance = 0.5 + yResonance * 14.5; // 0.5..15
  if (clampedX < 0) {
    const freq = 200 * Math.pow(100, 1 + clampedX);  // 200Hz..20kHz
    setFilterFX(deck, true, 'lowpass', freq, resonance);
  } else {
    const freq = 20 + clampedX * 7980;               // 20Hz..8kHz
    setFilterFX(deck, true, 'highpass', freq, resonance);
  }
}

export const DJFXPad: React.FC<DJFXPadProps> = React.memo(({ size = 160 }) => {
  const padRef = useRef<HTMLDivElement>(null);
  const [target, setTarget] = useState<Target>('A');
  const [active, setActive] = useState(false);
  // Position is purely visual; engine writes are debounced via ref to avoid
  // re-render churn at 60fps drag rate.
  const [pos, setPos] = useState<{ x: number; y: number }>({ x: 0.5, y: 0.5 });
  const lastEmittedRef = useRef<{ x: number; y: number }>({ x: 0.5, y: 0.5 });
  const rafRef = useRef<number | null>(null);

  const setDeckFilter = useStore(s => s.setDeckFilter);
  const { setFilterFX } = useDJAudioEngineActions();

  /** Push pad position to the engine (1 write per rAF tick). */
  const flush = useCallback(() => {
    rafRef.current = null;
    const { x, y } = lastEmittedRef.current;
    const xNorm = x * 2 - 1;     // -1..+1
    const yResonance = 1 - y;    // top = high resonance
    if (target === 'A' || target === 'BOTH') {
      applyFilterFor(setFilterFX, setDeckFilter, 'A', xNorm, yResonance);
    }
    if (target === 'B' || target === 'BOTH') {
      applyFilterFor(setFilterFX, setDeckFilter, 'B', xNorm, yResonance);
    }
  }, [target, setFilterFX, setDeckFilter]);

  const schedule = useCallback(() => {
    if (rafRef.current !== null) return;
    rafRef.current = requestAnimationFrame(flush);
  }, [flush]);

  const updateFromPointer = useCallback((clientX: number, clientY: number) => {
    const rect = padRef.current?.getBoundingClientRect();
    if (!rect) return;
    const x = Math.max(0, Math.min(1, (clientX - rect.left) / rect.width));
    const y = Math.max(0, Math.min(1, (clientY - rect.top) / rect.height));
    setPos({ x, y });
    lastEmittedRef.current = { x, y };
    schedule();
  }, [schedule]);

  const handlePointerDown = useCallback((e: React.PointerEvent) => {
    e.preventDefault();
    (e.target as HTMLElement).setPointerCapture(e.pointerId);
    setActive(true);
    updateFromPointer(e.clientX, e.clientY);
  }, [updateFromPointer]);

  const handlePointerMove = useCallback((e: React.PointerEvent) => {
    if (!active) return;
    updateFromPointer(e.clientX, e.clientY);
  }, [active, updateFromPointer]);

  /** Smoothly return to centre on release — non-latching like a Kaossilator. */
  const handlePointerUp = useCallback((e: React.PointerEvent) => {
    try { (e.target as HTMLElement).releasePointerCapture(e.pointerId); } catch { /* ignore */ }
    setActive(false);

    // Animate position back to centre and bypass the filter at end.
    const start = performance.now();
    const from = { ...lastEmittedRef.current };
    const dur = 220;
    const animate = (now: number) => {
      const t = Math.min(1, (now - start) / dur);
      // Ease-out cubic
      const e2 = 1 - Math.pow(1 - t, 3);
      const x = from.x + (0.5 - from.x) * e2;
      const y = from.y + (0.5 - from.y) * e2;
      setPos({ x, y });
      lastEmittedRef.current = { x, y };
      if (target === 'A' || target === 'BOTH') {
        applyFilterFor(setFilterFX, setDeckFilter, 'A', x * 2 - 1, 1 - y);
      }
      if (target === 'B' || target === 'BOTH') {
        applyFilterFor(setFilterFX, setDeckFilter, 'B', x * 2 - 1, 1 - y);
      }
      if (t < 1) requestAnimationFrame(animate);
    };
    requestAnimationFrame(animate);
  }, [target, setFilterFX, setDeckFilter]);

  // Tear down rAF on unmount
  useEffect(() => () => {
    if (rafRef.current !== null) cancelAnimationFrame(rafRef.current);
  }, []);

  // Visual: glow follows the puck, decays when released
  const puckSize = Math.max(28, Math.round(size * 0.18));
  const padPx = size;

  return (
    <div className='flex flex-col items-center gap-1 select-none'>
      {/* Header — label + target selector */}
      <div className='flex items-center justify-between w-full px-1' style={{ minWidth: Math.max(170, padPx) }}>
        <span className='text-[10px] font-bold text-[#666] uppercase tracking-widest'>FX Pad</span>
        <div className='flex gap-0.5 bg-[#0d0d0d] rounded p-0.5 border border-[#222]'>
          {(['A', 'B', 'BOTH'] as Target[]).map(t => (
            <button
              key={t}
              onClick={() => setTarget(t)}
              aria-pressed={target === t}
              className={`px-2.5 min-h-[28px] min-w-[28px] flex items-center justify-center text-[10px] font-bold uppercase tracking-wider rounded transition-colors
                ${target === t
                  ? t === 'A'
                    ? 'bg-blue-600 text-white'
                    : t === 'B'
                      ? 'bg-purple-600 text-white'
                      : 'bg-amber-500 text-black'
                  : 'text-neutral-500 hover:text-neutral-200'}
              `}
            >
              {t}
            </button>
          ))}
        </div>
      </div>

      {/* The pad itself */}
      <div
        ref={padRef}
        role='application'
        aria-label='FX X-Y performance pad — X axis filter cutoff, Y axis resonance'
        className={`relative rounded-lg cursor-crosshair touch-none border transition-shadow
          ${active ? 'border-amber-500/60 shadow-[0_0_20px_rgba(245,158,11,0.35)]' : 'border-[#2a2a2a]'}
        `}
        style={{
          width: padPx,
          height: padPx,
          background: 'radial-gradient(circle at 50% 50%, #1a1a1a 0%, #0a0a0a 70%)',
        }}
        onPointerDown={handlePointerDown}
        onPointerMove={handlePointerMove}
        onPointerUp={handlePointerUp}
        onPointerLeave={(e) => active && handlePointerUp(e)}
      >
        {/* Crosshair grid */}
        <div className='absolute inset-0 pointer-events-none'>
          <div className='absolute left-0 right-0 top-1/2 h-px bg-[#222]' />
          <div className='absolute top-0 bottom-0 left-1/2 w-px bg-[#222]' />
          {/* Quadrant dots */}
          {[ [0.25,0.25],[0.75,0.25],[0.25,0.75],[0.75,0.75] ].map(([qx,qy], i) => (
            <div key={i} className='absolute w-1 h-1 rounded-full bg-[#2a2a2a]'
              style={{ left: `calc(${qx*100}% - 2px)`, top: `calc(${qy*100}% - 2px)` }} />
          ))}
        </div>

        {/* Trail / glow lines from centre to puck */}
        <div className='absolute inset-0 pointer-events-none' aria-hidden>
          <svg width={padPx} height={padPx}>
            <line
              x1={padPx / 2}
              y1={padPx / 2}
              x2={pos.x * padPx}
              y2={pos.y * padPx}
              stroke={active ? '#f59e0b' : '#444'}
              strokeWidth={active ? 1.5 : 1}
              strokeDasharray='2 2'
              opacity={active ? 0.6 : 0.25}
            />
          </svg>
        </div>

        {/* Axis labels */}
        <div className='absolute left-1 top-1 text-[8px] text-[#444] uppercase tracking-wider pointer-events-none'>HP-Q</div>
        <div className='absolute right-1 top-1 text-[8px] text-[#444] uppercase tracking-wider pointer-events-none'>HP+Q</div>
        <div className='absolute left-1 bottom-1 text-[8px] text-[#444] uppercase tracking-wider pointer-events-none'>LP-Q</div>
        <div className='absolute right-1 bottom-1 text-[8px] text-[#444] uppercase tracking-wider pointer-events-none'>LP+Q</div>
        <div className='absolute left-1/2 -translate-x-1/2 top-0.5 text-[8px] text-[#555] uppercase pointer-events-none'>RES↑</div>
        <div className='absolute left-1/2 -translate-x-1/2 bottom-0.5 text-[8px] text-[#555] uppercase pointer-events-none'>FILT</div>

        {/* Puck */}
        <div
          className='absolute rounded-full pointer-events-none transition-transform'
          style={{
            width: puckSize,
            height: puckSize,
            left: `calc(${pos.x * 100}% - ${puckSize / 2}px)`,
            top: `calc(${pos.y * 100}% - ${puckSize / 2}px)`,
            background: active
              ? 'radial-gradient(circle at 35% 30%, #fde68a, #f59e0b 60%, #b45309)'
              : 'radial-gradient(circle at 35% 30%, #888, #444 60%, #222)',
            boxShadow: active
              ? '0 0 20px rgba(245,158,11,0.6), 0 4px 12px rgba(0,0,0,0.6), inset 0 1px 0 rgba(255,255,255,0.4)'
              : '0 2px 8px rgba(0,0,0,0.5), inset 0 1px 0 rgba(255,255,255,0.15)',
          }}
        />
      </div>

      {/* Footer readout */}
      <div className='flex justify-between w-full px-1 text-[9px] font-mono text-[#555]' style={{ minWidth: Math.max(170, padPx) }}>
        <span>X {((pos.x - 0.5) * 2).toFixed(2)}</span>
        <span>Y {(1 - pos.y).toFixed(2)}</span>
      </div>
    </div>
  );
});

DJFXPad.displayName = 'DJFXPad';

export default DJFXPad;
