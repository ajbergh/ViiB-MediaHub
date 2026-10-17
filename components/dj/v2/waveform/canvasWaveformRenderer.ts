import type { TrackThreeBand } from '../../../../services/trackThreeBand';
/**
 * Canvas 2D drawing helpers for one deck's scrolling waveform.
 *
 * Extracted from the former stacked DJDualWaveform so the split layout can
 * render each deck in its own lane. Drawing behavior (loop shading, beat grid,
 * cue marker, playhead) is unchanged; colors come from the shared palette.
 *
 * @module components/dj/v2/waveform/canvasWaveformRenderer
 */

import type { DeckId, HotCue, Loop } from '../../../../slices/djMixerSlice';
import { getHotCueMarkerStyle } from '../../../../lib/hotCueMarkerStyle';
import { DECK_WAVEFORM_PALETTE, LEVEL_COLORS, WAVEFORM_CHROME, canvasGradientStops, type WaveformColorMode } from './waveformPalette';

export type WaveformOrientation = 'horizontal' | 'vertical';
export type WaveformBandSelection = 'all' | 'low' | 'mid' | 'high';

export function getWaveformBandIndices(selection: WaveformBandSelection = 'all'): number[] {
  return selection === 'all' ? [0, 1, 2] : [{ low: 0, mid: 1, high: 2 }[selection]];
}

export interface MainWaveformFrame {
  threeBands?: { data: TrackThreeBand; peak: number };
  deck: DeckId;
  peaks: number[] | null;
  position: number;
  duration: number;
  beatGrid: number[] | null;
  beatGridOffset: number;
  cuePoint: number;
  hotCues: HotCue[];
  loop: Loop;
  visibleSeconds: number;
  colorMode: WaveformColorMode;
  hasTrack: boolean;
  orientation?: WaveformOrientation;
  bandSelection?: WaveformBandSelection;
}

/** Max absolute peak between two sample indices. */
function peakBetween(peaks: number[], first: number, last: number): number {
  let peak = 0;
  for (let index = first; index < last; index++) peak = Math.max(peak, peaks[index] || 0);
  return peak;
}

/** Draw one deck's main scrolling waveform into a CSS-pixel sized context. */
export function drawMainWaveform(ctx: CanvasRenderingContext2D, width: number, h: number, frame: MainWaveformFrame): void {
  if (frame.orientation === 'vertical') {
    drawVerticalMainWaveform(ctx, width, h, frame);
    return;
  }
  const { deck, peaks, position, duration, beatGrid, beatGridOffset, cuePoint, loop, visibleSeconds, colorMode } = frame;
  const palette = DECK_WAVEFORM_PALETTE[deck];
  const centerY = h / 2;
  const playheadX = width / 2;

  ctx.fillStyle = WAVEFORM_CHROME.background;
  ctx.fillRect(0, 0, width, h);

  if ((!frame.threeBands && (!peaks || peaks.length === 0)) || !duration || duration <= 0) {
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
  const peaksPerSecond = (peaks?.length ?? 0) / duration;
  const maxAmplitude = h / 2 - 4;

  const sampleAt = (x: number): number => {
    const pixelTime = position + (x - playheadX) * secondsPerPixel;
    if (pixelTime < 0 || pixelTime > duration) return 0;
    const first = Math.max(0, Math.floor((pixelTime - secondsPerPixel / 2) * peaksPerSecond));
    const last = Math.min(peaks?.length ?? 0, Math.max(first + 1, Math.ceil((pixelTime + secondsPerPixel / 2) * peaksPerSecond)));
    return peakBetween(peaks ?? [], first, last);
  };

  if (frame.threeBands) {
    const { data, peak } = frame.threeBands;
    const arrays = [data.low, data.mid, data.high];
    const colors = [palette.gradient.center, palette.gradient.mid, palette.gradient.edge];
    const samplesPerSecond = arrays[0].length / data.durationSeconds;
    const indices = getWaveformBandIndices(frame.bandSelection);
    for (const [lane, band] of indices.entries()) {
      const center = (lane + 0.5) * h / indices.length;
      const height = indices.length === 1 ? Math.max(0, maxAmplitude) : Math.max(0, h / 6 - 2);
      ctx.fillStyle = colors[band];
      ctx.beginPath();
      for (let x = 0; x < width; x++) {
        const time = position + (x - playheadX) * secondsPerPixel;
        if (time < 0 || time > duration) continue;
        const first = Math.max(0, Math.floor((time - secondsPerPixel / 2) * samplesPerSecond));
        const last = Math.min(arrays[band].length, Math.max(first + 1, Math.ceil((time + secondsPerPixel / 2) * samplesPerSecond)));
        const amplitude = peak > 0 ? peakBetween(arrays[band], first, last) / peak * height : 0;
        if (amplitude > 0) ctx.rect(x, center - amplitude, 1, amplitude * 2);
      }
      ctx.fill();
      ctx.font = '9px system-ui';
      ctx.textAlign = 'left';
      ctx.fillText(`${data.source === 'spotify' ? 'Spotify' : 'Local'} ${['Low', 'Mid', 'High'][band]}`, 4, lane * h / indices.length + 10);
    }
  } else if (colorMode === '3band') {
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

  // Hot cues: thin line plus a numbered flag at the top (mock-up). Generated
  // cues are outlined, user cues filled, matching the overview markers.
  for (const cue of frame.hotCues) {
    if (cue.position < visibleStartTime || cue.position > visibleEndTime) continue;
    const x = Math.round(playheadX + (cue.position - position) / secondsPerPixel) + 0.5;
    const style = getHotCueMarkerStyle(cue);
    ctx.strokeStyle = style.color;
    ctx.fillStyle = style.color;
    ctx.lineWidth = 1;
    ctx.setLineDash(style.dash);
    ctx.globalAlpha = 0.75;
    ctx.beginPath(); ctx.moveTo(x, 0); ctx.lineTo(x, h); ctx.stroke();
    ctx.globalAlpha = 1;
    ctx.setLineDash([]);
    if (style.fill) ctx.fillRect(x - 0.5, 0, 12, 12);
    else ctx.strokeRect(x, 0.5, 11, 11);
    ctx.fillStyle = style.fill ? WAVEFORM_CHROME.flagText : style.color;
    ctx.font = 'bold 9px system-ui';
    ctx.textAlign = 'center';
    ctx.fillText(String(cue.slot), x + 5.5, 9);
    // Bottom tick so the marker reads against the lower half too.
    ctx.fillStyle = style.color;
    ctx.beginPath(); ctx.moveTo(x - 4, h); ctx.lineTo(x + 4, h); ctx.lineTo(x, h - 5); ctx.closePath(); ctx.fill();
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

/**
 * Draw one deck's vertical scrolling waveform into a CSS-pixel sized context.
 * Time runs vertically along `y` (`playheadY = h / 2`), and amplitude expands
 * horizontally from `centerX = width / 2`.
 */
function drawVerticalMainWaveform(ctx: CanvasRenderingContext2D, width: number, h: number, frame: MainWaveformFrame): void {
  const { deck, peaks, position, duration, beatGrid, beatGridOffset, cuePoint, loop, visibleSeconds, colorMode } = frame;
  const palette = DECK_WAVEFORM_PALETTE[deck];
  const centerX = width / 2;
  const playheadY = h / 2;

  ctx.fillStyle = WAVEFORM_CHROME.background;
  ctx.fillRect(0, 0, width, h);

  // Subtle center spine
  ctx.strokeStyle = 'rgba(255, 255, 255, 0.06)';
  ctx.lineWidth = 1;
  ctx.beginPath(); ctx.moveTo(centerX + 0.5, 0); ctx.lineTo(centerX + 0.5, h); ctx.stroke();

  if ((!frame.threeBands && (!peaks || peaks.length === 0)) || !duration || duration <= 0) {
    ctx.strokeStyle = WAVEFORM_CHROME.placeholderGrid;
    ctx.lineWidth = 1;
    for (let y = 0; y < h; y += 48) {
      ctx.beginPath(); ctx.moveTo(0, y + 0.5); ctx.lineTo(width, y + 0.5); ctx.stroke();
    }
    ctx.fillStyle = WAVEFORM_CHROME.placeholderText;
    ctx.textAlign = 'center';
    ctx.font = '10px system-ui';
    ctx.fillText(frame.hasTrack ? 'Preparing…' : `DECK ${deck}`, centerX, playheadY - 10);
    ctx.strokeStyle = WAVEFORM_CHROME.playheadGlow;
    ctx.lineWidth = 2;
    ctx.beginPath(); ctx.moveTo(0, playheadY); ctx.lineTo(width, playheadY); ctx.stroke();
    return;
  }

  const secondsPerPixel = visibleSeconds / h;
  const visibleStartTime = position - playheadY * secondsPerPixel;
  const visibleEndTime = position + (h - playheadY) * secondsPerPixel;
  const peaksPerSecond = (peaks?.length ?? 0) / duration;
  const maxAmplitude = Math.max(4, width / 2 - 6);

  const sampleAt = (y: number): number => {
    const pixelTime = position + (y - playheadY) * secondsPerPixel;
    if (pixelTime < 0 || pixelTime > duration) return 0;
    const first = Math.max(0, Math.floor((pixelTime - secondsPerPixel / 2) * peaksPerSecond));
    const last = Math.min(peaks?.length ?? 0, Math.max(first + 1, Math.ceil((pixelTime + secondsPerPixel / 2) * peaksPerSecond)));
    return peakBetween(peaks ?? [], first, last);
  };

  if (frame.threeBands) {
    const { data, peak } = frame.threeBands;
    const arrays = [data.low, data.mid, data.high];
    const colors = [palette.gradient.center, palette.gradient.mid, palette.gradient.edge];
    const samplesPerSecond = arrays[0].length / data.durationSeconds;
    const indices = getWaveformBandIndices(frame.bandSelection);
    for (const [lane, band] of indices.entries()) {
      const bandCenter = (lane + 0.5) * width / indices.length;
      const halfWidth = indices.length === 1 ? maxAmplitude : Math.max(0, width / 6 - 2);
      ctx.fillStyle = colors[band];
      ctx.beginPath();
      for (let y = 0; y < h; y++) {
        const time = position + (y - playheadY) * secondsPerPixel;
        if (time < 0 || time > duration) continue;
        const first = Math.max(0, Math.floor((time - secondsPerPixel / 2) * samplesPerSecond));
        const last = Math.min(arrays[band].length, Math.max(first + 1, Math.ceil((time + secondsPerPixel / 2) * samplesPerSecond)));
        const amplitude = peak > 0 ? peakBetween(arrays[band], first, last) / peak * halfWidth : 0;
        if (amplitude > 0) ctx.rect(bandCenter - amplitude, y, amplitude * 2, 1);
      }
      ctx.fill();
    }
  } else if (colorMode === '3band') {
    for (let y = 0; y < h; y++) {
      const peak = sampleAt(y);
      const amplitude = peak * maxAmplitude;
      if (amplitude <= 0) continue;
      ctx.fillStyle = peak > 0.6 ? LEVEL_COLORS.loud : peak > 0.3 ? LEVEL_COLORS.medium : LEVEL_COLORS.quiet;
      ctx.fillRect(centerX - amplitude, y, amplitude * 2, 1);
    }
  } else {
    if (colorMode === 'single') {
      ctx.fillStyle = palette.deck;
    } else {
      const gradient = ctx.createLinearGradient(centerX - maxAmplitude, 0, centerX + maxAmplitude, 0);
      for (const [offset, color] of canvasGradientStops(deck)) gradient.addColorStop(offset, color);
      ctx.fillStyle = gradient;
    }
    ctx.beginPath();
    for (let y = 0; y < h; y++) {
      const amplitude = sampleAt(y) * maxAmplitude;
      if (amplitude > 0) ctx.rect(centerX - amplitude, y, amplitude * 2, 1);
    }
    ctx.fill();
  }

  // Loop region (horizontal bands)
  if (loop.end > loop.start) {
    const loopTop = playheadY + (loop.start - position) / secondsPerPixel;
    const loopBottom = playheadY + (loop.end - position) / secondsPerPixel;
    const top = Math.max(0, loopTop);
    const bottom = Math.min(h, loopBottom);
    if (bottom > top) {
      ctx.fillStyle = loop.enabled ? WAVEFORM_CHROME.loopActiveFill : WAVEFORM_CHROME.loopInactiveFill;
      ctx.fillRect(0, top, width, bottom - top);
      ctx.strokeStyle = loop.enabled ? WAVEFORM_CHROME.loopActiveEdge : WAVEFORM_CHROME.loopInactiveEdge;
      ctx.lineWidth = 1;
      for (const edge of [loopTop, loopBottom]) {
        if (edge < 0 || edge > h) continue;
        ctx.beginPath(); ctx.moveTo(0, edge); ctx.lineTo(width, edge); ctx.stroke();
      }
    }
  }

  // Beat grid (horizontal lines across the column; downbeat every 4 beats)
  if (beatGrid && beatGrid.length > 0) {
    let beatCount = 0;
    for (const rawBeatTime of beatGrid) {
      const beatTime = rawBeatTime + beatGridOffset;
      if (beatTime >= visibleStartTime && beatTime <= visibleEndTime) {
        const beatY = playheadY + (beatTime - position) / secondsPerPixel;
        const downbeat = beatCount % 4 === 0;
        ctx.strokeStyle = downbeat ? WAVEFORM_CHROME.downbeat : WAVEFORM_CHROME.beat;
        ctx.lineWidth = downbeat ? 2 : 1;
        ctx.beginPath(); ctx.moveTo(0, beatY); ctx.lineTo(width, beatY); ctx.stroke();
      }
      beatCount++;
    }
  }

  // Cue point marker
  if (cuePoint > 0 && cuePoint >= visibleStartTime && cuePoint <= visibleEndTime) {
    const cueY = playheadY + (cuePoint - position) / secondsPerPixel;
    ctx.strokeStyle = WAVEFORM_CHROME.cue;
    ctx.lineWidth = 2;
    ctx.beginPath(); ctx.moveTo(0, cueY); ctx.lineTo(width, cueY); ctx.stroke();
    ctx.fillStyle = WAVEFORM_CHROME.cue;
    ctx.beginPath(); ctx.moveTo(0, cueY - 5); ctx.lineTo(0, cueY + 5); ctx.lineTo(8, cueY); ctx.closePath(); ctx.fill();
  }

  // Hot cues
  for (const cue of frame.hotCues) {
    if (cue.position < visibleStartTime || cue.position > visibleEndTime) continue;
    const y = Math.round(playheadY + (cue.position - position) / secondsPerPixel) + 0.5;
    const style = getHotCueMarkerStyle(cue);
    ctx.strokeStyle = style.color;
    ctx.fillStyle = style.color;
    ctx.lineWidth = 1;
    ctx.setLineDash(style.dash);
    ctx.globalAlpha = 0.75;
    ctx.beginPath(); ctx.moveTo(0, y); ctx.lineTo(width, y); ctx.stroke();
    ctx.globalAlpha = 1;
    ctx.setLineDash([]);
    if (style.fill) ctx.fillRect(0, y - 6, 12, 12);
    else ctx.strokeRect(0.5, y - 5.5, 11, 11);
    ctx.fillStyle = style.fill ? WAVEFORM_CHROME.flagText : style.color;
    ctx.font = 'bold 9px system-ui';
    ctx.textAlign = 'center';
    ctx.fillText(String(cue.slot), 6, y + 3);
  }

  // Horizontal center playhead with glow and side triangles
  ctx.strokeStyle = WAVEFORM_CHROME.playheadGlow;
  ctx.lineWidth = 6;
  ctx.beginPath(); ctx.moveTo(0, playheadY); ctx.lineTo(width, playheadY); ctx.stroke();
  ctx.strokeStyle = WAVEFORM_CHROME.playhead;
  ctx.lineWidth = 2;
  ctx.beginPath(); ctx.moveTo(0, playheadY); ctx.lineTo(width, playheadY); ctx.stroke();
  ctx.fillStyle = WAVEFORM_CHROME.playhead;
  ctx.beginPath(); ctx.moveTo(0, playheadY - 5); ctx.lineTo(0, playheadY + 5); ctx.lineTo(7, playheadY); ctx.closePath(); ctx.fill();
  ctx.beginPath(); ctx.moveTo(width, playheadY - 5); ctx.lineTo(width, playheadY + 5); ctx.lineTo(width - 7, playheadY); ctx.closePath(); ctx.fill();
}

/** Seek target for a click at `x` (or `y` in vertical mode) in a scrolling lane of `width` (or `height`) CSS px. */
export function timeAtLaneX(x: number, width: number, position: number, visibleSeconds: number, duration: number): number {
  const secondsPerPixel = visibleSeconds / width;
  return Math.max(0, Math.min(duration, position + (x - width / 2) * secondsPerPixel));
}
