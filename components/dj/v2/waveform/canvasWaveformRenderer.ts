/**
 * Canvas 2D drawing helpers for one deck's scrolling waveform.
 *
 * Extracted from the former stacked DJDualWaveform so the split layout can
 * render each deck in its own lane. Drawing behavior (loop shading, beat grid,
 * cue marker, playhead) is unchanged; colors come from the shared palette.
 *
 * @module components/dj/v2/waveform/canvasWaveformRenderer
 */

import type { DeckId, Loop } from '../../../../slices/djMixerSlice';
import { DECK_WAVEFORM_PALETTE, LEVEL_COLORS, WAVEFORM_CHROME, canvasGradientStops, type WaveformColorMode } from './waveformPalette';

export interface MainWaveformFrame {
  deck: DeckId;
  peaks: number[] | null;
  position: number;
  duration: number;
  beatGrid: number[] | null;
  beatGridOffset: number;
  cuePoint: number;
  loop: Loop;
  visibleSeconds: number;
  colorMode: WaveformColorMode;
  hasTrack: boolean;
}

/** Max absolute peak between two sample indices. */
function peakBetween(peaks: number[], first: number, last: number): number {
  let peak = 0;
  for (let index = first; index < last; index++) peak = Math.max(peak, peaks[index] || 0);
  return peak;
}

/** Draw one deck's main scrolling waveform into a CSS-pixel sized context. */
export function drawMainWaveform(ctx: CanvasRenderingContext2D, width: number, h: number, frame: MainWaveformFrame): void {
  const { deck, peaks, position, duration, beatGrid, beatGridOffset, cuePoint, loop, visibleSeconds, colorMode } = frame;
  const palette = DECK_WAVEFORM_PALETTE[deck];
  const centerY = h / 2;
  const playheadX = width / 2;

  ctx.fillStyle = WAVEFORM_CHROME.background;
  ctx.fillRect(0, 0, width, h);

  if (!peaks || peaks.length === 0 || !duration || duration <= 0) {
    ctx.strokeStyle = WAVEFORM_CHROME.placeholderGrid;
    ctx.lineWidth = 1;
    for (let x = 0; x < width; x += 48) {
      ctx.beginPath(); ctx.moveTo(x + 0.5, 0); ctx.lineTo(x + 0.5, h); ctx.stroke();
    }
    ctx.fillStyle = WAVEFORM_CHROME.placeholderText;
    ctx.textAlign = 'center';
    ctx.font = '12px system-ui';
    ctx.fillText(frame.hasTrack ? 'Waveform preparing…' : `Load a track to Deck ${deck}`, width / 2, centerY + 4);
    ctx.strokeStyle = WAVEFORM_CHROME.playheadGlow;
    ctx.lineWidth = 2;
    ctx.beginPath(); ctx.moveTo(playheadX, 0); ctx.lineTo(playheadX, h); ctx.stroke();
    return;
  }

  const secondsPerPixel = visibleSeconds / width;
  const visibleStartTime = position - playheadX * secondsPerPixel;
  const visibleEndTime = position + (width - playheadX) * secondsPerPixel;
  const peaksPerSecond = peaks.length / duration;
  const maxAmplitude = h / 2 - 4;

  const sampleAt = (x: number): number => {
    const pixelTime = position + (x - playheadX) * secondsPerPixel;
    if (pixelTime < 0 || pixelTime > duration) return 0;
    const first = Math.max(0, Math.floor((pixelTime - secondsPerPixel / 2) * peaksPerSecond));
    const last = Math.min(peaks.length, Math.max(first + 1, Math.ceil((pixelTime + secondsPerPixel / 2) * peaksPerSecond)));
    return peakBetween(peaks, first, last);
  };

  if (colorMode === '3band') {
    // Level mode uses amplitude thresholds and therefore separate fills.
    for (let x = 0; x < width; x++) {
      const peak = sampleAt(x);
      const amplitude = peak * maxAmplitude;
      if (amplitude <= 0) continue;
      ctx.fillStyle = peak > 0.6 ? LEVEL_COLORS.loud : peak > 0.3 ? LEVEL_COLORS.medium : LEVEL_COLORS.quiet;
      ctx.fillRect(x, centerY - amplitude, 1, amplitude * 2);
    }
  } else {
    // Gradient and solid modes have one paint: batch into a single fill.
    if (colorMode === 'single') {
      ctx.fillStyle = palette.deck;
    } else {
      const gradient = ctx.createLinearGradient(0, centerY - maxAmplitude, 0, centerY + maxAmplitude);
      for (const [offset, color] of canvasGradientStops(deck)) gradient.addColorStop(offset, color);
      ctx.fillStyle = gradient;
    }
    ctx.beginPath();
    for (let x = 0; x < width; x++) {
      const amplitude = sampleAt(x) * maxAmplitude;
      if (amplitude > 0) ctx.rect(x, centerY - amplitude, 1, amplitude * 2);
    }
    ctx.fill();
  }

  // Loop region and boundaries sit behind the grid/playhead markers.
  if (loop.end > loop.start) {
    const loopLeft = playheadX + (loop.start - position) / secondsPerPixel;
    const loopRight = playheadX + (loop.end - position) / secondsPerPixel;
    const left = Math.max(0, loopLeft);
    const right = Math.min(width, loopRight);
    if (right > left) {
      ctx.fillStyle = loop.enabled ? WAVEFORM_CHROME.loopActiveFill : WAVEFORM_CHROME.loopInactiveFill;
      ctx.fillRect(left, 0, right - left, h);
      ctx.strokeStyle = loop.enabled ? WAVEFORM_CHROME.loopActiveEdge : WAVEFORM_CHROME.loopInactiveEdge;
      ctx.lineWidth = 1;
      for (const edge of [loopLeft, loopRight]) {
        if (edge < 0 || edge > width) continue;
        ctx.beginPath(); ctx.moveTo(edge, 0); ctx.lineTo(edge, h); ctx.stroke();
      }
    }
  }

  // Beat grid (downbeat every 4 beats is more prominent).
  if (beatGrid && beatGrid.length > 0) {
    let beatCount = 0;
    for (const rawBeatTime of beatGrid) {
      const beatTime = rawBeatTime + beatGridOffset;
      if (beatTime >= visibleStartTime && beatTime <= visibleEndTime) {
        const beatX = playheadX + (beatTime - position) / secondsPerPixel;
        const downbeat = beatCount % 4 === 0;
        ctx.strokeStyle = downbeat ? WAVEFORM_CHROME.downbeat : WAVEFORM_CHROME.beat;
        ctx.lineWidth = downbeat ? 2 : 1;
        ctx.beginPath(); ctx.moveTo(beatX, 0); ctx.lineTo(beatX, h); ctx.stroke();
      }
      beatCount++;
    }
  }

  // Cue point marker.
  if (cuePoint > 0 && cuePoint >= visibleStartTime && cuePoint <= visibleEndTime) {
    const cueX = playheadX + (cuePoint - position) / secondsPerPixel;
    ctx.strokeStyle = WAVEFORM_CHROME.cue;
    ctx.lineWidth = 2;
    ctx.beginPath(); ctx.moveTo(cueX, 0); ctx.lineTo(cueX, h); ctx.stroke();
    ctx.fillStyle = WAVEFORM_CHROME.cue;
    ctx.beginPath(); ctx.moveTo(cueX - 6, 0); ctx.lineTo(cueX + 6, 0); ctx.lineTo(cueX, 10); ctx.closePath(); ctx.fill();
  }

  // Playhead with glow and top triangle.
  ctx.strokeStyle = WAVEFORM_CHROME.playheadGlow;
  ctx.lineWidth = 6;
  ctx.beginPath(); ctx.moveTo(playheadX, 0); ctx.lineTo(playheadX, h); ctx.stroke();
  ctx.strokeStyle = WAVEFORM_CHROME.playhead;
  ctx.lineWidth = 2;
  ctx.beginPath(); ctx.moveTo(playheadX, 0); ctx.lineTo(playheadX, h); ctx.stroke();
  ctx.fillStyle = WAVEFORM_CHROME.playhead;
  ctx.beginPath(); ctx.moveTo(playheadX - 6, 0); ctx.lineTo(playheadX + 6, 0); ctx.lineTo(playheadX, 8); ctx.closePath(); ctx.fill();
}

/** Seek target for a click at `x` in a scrolling lane of `width` CSS px. */
export function timeAtLaneX(x: number, width: number, position: number, visibleSeconds: number, duration: number): number {
  const secondsPerPixel = visibleSeconds / width;
  return Math.max(0, Math.min(duration, position + (x - width / 2) * secondsPerPixel));
}
