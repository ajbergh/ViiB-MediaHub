import React, { useCallback, useEffect, useRef } from 'react';
import { useStore } from '../../../store';
import { useDJAudioEngineActions } from '../../../hooks/useDJAudioEngine';
import { getHotCueMarkerStyle } from '../../../lib/hotCueMarkerStyle';
import type { DeckId } from '../../../slices/djMixerSlice';
import { DECK_WAVEFORM_PALETTE, WAVEFORM_CHROME } from './waveform/waveformPalette';

interface DJDeckOverviewProps {
  deck: DeckId;
  /** Main-lane zoom window in seconds; draws the visible region box when set. */
  visibleSeconds?: number;
}

/** Whole-track overview: peak data, cues, loop, zoom window and playhead,
 * without a position subscription that would re-render the surrounding deck.
 * Clicking seeks this deck only. Drawn with Canvas 2D so the split waveform
 * keeps WebGL contexts to the two main lanes. */
export const DJDeckOverview = React.memo(({ deck, visibleSeconds }: DJDeckOverviewProps) => {
  const ref = useRef<HTMLCanvasElement>(null);
  const { seek } = useDJAudioEngineActions();

  useEffect(() => {
    const canvas = ref.current;
    if (!canvas) return;
    const ctx = canvas.getContext('2d');
    if (!ctx) return;
    const palette = DECK_WAVEFORM_PALETTE[deck];
    // Static peak bars are cached and only rebuilt when peaks or size change.
    const cache = document.createElement('canvas');
    let cacheKey: unknown[] = [];

    const draw = () => {
      const state = useStore.getState();
      const d = deck === 'A' ? state.djDeckA : state.djDeckB;
      const width = canvas.clientWidth;
      const height = canvas.clientHeight;
      if (width <= 0 || height <= 0) return;
      const ratio = window.devicePixelRatio || 1;
      if (canvas.width !== Math.round(width * ratio) || canvas.height !== Math.round(height * ratio)) {
        canvas.width = Math.round(width * ratio);
        canvas.height = Math.round(height * ratio);
      }
      const peaks = d.waveformPeaks;
      const key = [peaks, canvas.width, canvas.height];
      if (key.some((value, index) => value !== cacheKey[index])) {
        cacheKey = key;
        cache.width = canvas.width;
        cache.height = canvas.height;
        const cctx = cache.getContext('2d');
        if (cctx) {
          cctx.setTransform(ratio, 0, 0, ratio, 0, 0);
          cctx.fillStyle = WAVEFORM_CHROME.overviewBackground;
          cctx.fillRect(0, 0, width, height);
          if (peaks?.length) {
            cctx.fillStyle = palette.overview;
            cctx.beginPath();
            for (let x = 0; x < width; x++) {
              const first = Math.floor(x / width * peaks.length);
              const end = Math.min(peaks.length, Math.max(first + 1, Math.floor((x + 1) / width * peaks.length)));
              let peak = 0;
              for (let i = first; i < end; i++) peak = Math.max(peak, Math.abs(peaks[i]));
              const h = Math.max(1, Math.min(1, peak) * (height - 2));
              cctx.rect(x, (height - h) / 2, 1, h);
            }
            cctx.fill();
          }
        }
      }
      ctx.setTransform(1, 0, 0, 1, 0, 0);
      ctx.drawImage(cache, 0, 0);
      ctx.setTransform(ratio, 0, 0, ratio, 0, 0);

      if (d.duration > 0) {
        const toX = (seconds: number) => Math.max(0, Math.min(width, seconds / d.duration * width));
        if (d.loop.end > d.loop.start) {
          ctx.fillStyle = d.loop.enabled ? WAVEFORM_CHROME.overviewLoopActive : WAVEFORM_CHROME.overviewLoopInactive;
          ctx.fillRect(toX(d.loop.start), 0, Math.max(1, toX(d.loop.end) - toX(d.loop.start)), height);
        }
        if (visibleSeconds && peaks?.length) {
          const left = toX(d.position - visibleSeconds / 2);
          const right = toX(d.position + visibleSeconds / 2);
          ctx.fillStyle = WAVEFORM_CHROME.overviewWindow;
          ctx.fillRect(left, 0, Math.max(2, right - left), height);
          ctx.strokeStyle = palette.bright;
          ctx.lineWidth = 1;
          ctx.strokeRect(left + 0.5, 0.5, Math.max(2, right - left) - 1, height - 1);
        }
        for (const cue of d.hotCues) {
          const style = getHotCueMarkerStyle(cue);
          ctx.fillStyle = style.color;
          ctx.fillRect(Math.min(width - 2, toX(cue.position)), 0, 2, height);
        }
        ctx.fillStyle = WAVEFORM_CHROME.overviewPlayhead;
        ctx.fillRect(Math.min(width - 2, toX(d.position)), 0, 2, height);
      } else if (!peaks?.length) {
        ctx.fillStyle = WAVEFORM_CHROME.placeholderText;
        ctx.font = '10px system-ui';
        ctx.fillText(d.track ? 'Overview preparing…' : 'No track', 8, height / 2 + 3);
      }
    };
    draw();
    const timer = window.setInterval(draw, 80);
    return () => window.clearInterval(timer);
  }, [deck, visibleSeconds]);

  const handleClick = useCallback((event: React.MouseEvent<HTMLCanvasElement>) => {
    const d = deck === 'A' ? useStore.getState().djDeckA : useStore.getState().djDeckB;
    if (!d.duration) return;
    const rect = event.currentTarget.getBoundingClientRect();
    const fraction = (event.clientX - rect.left) / rect.width;
    seek(deck, Math.max(0, Math.min(d.duration, fraction * d.duration)));
  }, [deck, seek]);

  return (
    <canvas
      ref={ref}
      className='dj-deck-overview'
      role='img'
      aria-label={`Deck ${deck} track overview with cue markers and playhead; click to seek`}
      data-dj-overview-deck={deck}
      onClick={handleClick}
    />
  );
});
