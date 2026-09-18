import React, { useEffect, useRef } from 'react';
import { useStore } from '../../../store';
import type { DeckId } from '../../../slices/djMixerSlice';

/** Whole-track overview: peak data, cue positions and playhead, without a
 * position subscription that would re-render the surrounding deck. */
export const DJDeckOverview = React.memo(({ deck }: { deck: DeckId }) => {
  const ref = useRef<HTMLCanvasElement>(null);
  useEffect(() => {
    const canvas = ref.current;
    if (!canvas) return;
    const ctx = canvas.getContext('2d');
    if (!ctx) return;
    const styles = getComputedStyle(canvas);
    const deckColor = styles.getPropertyValue(deck === 'A' ? '--dj-deck-a' : '--dj-deck-b').trim();
    const cueColor = styles.getPropertyValue('--dj-warning').trim();
    const playheadColor = styles.getPropertyValue('--dj-text-primary').trim();
    const draw = () => {
      const state = useStore.getState();
      const d = deck === 'A' ? state.djDeckA : state.djDeckB;
      const width = canvas.clientWidth;
      const height = canvas.clientHeight;
      const ratio = window.devicePixelRatio || 1;
      if (canvas.width !== Math.round(width * ratio) || canvas.height !== Math.round(height * ratio)) {
        canvas.width = Math.round(width * ratio);
        canvas.height = Math.round(height * ratio);
      }
      ctx.setTransform(ratio, 0, 0, ratio, 0, 0);
      ctx.clearRect(0, 0, width, height);
      const peaks = d.waveformPeaks;
      ctx.fillStyle = deckColor;
      if (peaks?.length) {
        for (let x = 0; x < width; x += 2) {
          const first = Math.floor(x / width * peaks.length);
          const end = Math.min(peaks.length, Math.max(first + 1, Math.floor((x + 2) / width * peaks.length)));
          let peak = 0;
          for (let i = first; i < end; i++) peak = Math.max(peak, Math.abs(peaks[i]));
          const h = Math.max(1, Math.min(1, peak) * (height - 16));
          ctx.fillRect(x, (height - h) / 2, 1, h);
        }
      } else {
        ctx.font = '12px sans-serif';
        ctx.fillText(d.track ? 'Waveform preparing…' : 'Load a track to view waveform', 12, height / 2 + 4);
      }
      if (d.duration > 0) {
        ctx.font = 'bold 10px sans-serif';
        for (const cue of d.hotCues) {
          const x = Math.max(0, Math.min(width - 12, cue.position / d.duration * width));
          ctx.fillStyle = cueColor;
          ctx.fillRect(x, 0, 2, height);
          ctx.fillText(String(cue.slot), x + 3, 10);
        }
        ctx.fillStyle = playheadColor;
        ctx.fillRect(Math.min(width - 2, Math.max(0, d.position / d.duration * width)), 0, 2, height);
      }
    };
    draw();
    const timer = window.setInterval(draw, 80);
    return () => window.clearInterval(timer);
  }, [deck]);
  return <canvas ref={ref} className='dj-deck-overview' role='img' aria-label={`Deck ${deck} track overview with cue markers and playhead`} />;
});
