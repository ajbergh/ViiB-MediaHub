import { reportAudioReadFailure, withSharedAudioMetadataRead } from '../../../services/audioReadDiagnostics';
/** Draws a whole-track overview with position, loop, and cue markers. */

import React, { useCallback, useEffect, useRef, useState } from 'react';
import { api } from '../../../services/api';
import { loadLocalThreeBand, type LocalThreeBand } from '../../../services/localThreeBand';
import { djTrackSourceIdentity } from '../../../lib/djBpmCorrection';
import { useStore } from '../../../store';
import { useDJAudioEngineActions } from '../../../hooks/useDJAudioEngine';
import { getHotCueMarkerStyle } from '../../../lib/hotCueMarkerStyle';
import type { DeckId } from '../../../slices/djMixerSlice';
import { DECK_WAVEFORM_PALETTE, WAVEFORM_CHROME } from './waveform/waveformPalette';

interface DJDeckOverviewProps {
  deck: DeckId;
  localBands?: boolean;
  /** Main-lane zoom window in seconds; draws the visible region box when set. */
  visibleSeconds?: number;
}

/** Whole-track overview: peak data, cues, loop, zoom window and playhead,
 * without a position subscription that would re-render the surrounding deck.
 * Clicking seeks this deck only. Drawn with Canvas 2D so the split waveform
 * keeps WebGL contexts to the two main lanes. */
export const DJDeckOverview = React.memo(({ deck, visibleSeconds, localBands = false }: DJDeckOverviewProps) => {
  const track = useStore(s => (deck === 'A' ? s.djDeckA : s.djDeckB).track);
  const duration = useStore(s => (deck === 'A' ? s.djDeckA : s.djDeckB).duration);
  const spotifySessionGeneration = useStore(s => s.spotifySessionGeneration);
  const trackSourceIdentity = djTrackSourceIdentity(track);
  const [bands, setBands] = useState<{ track: typeof track; data: LocalThreeBand }>();
  const [bandsReadFailed, setBandsReadFailed] = useState(false);
  const [retryGeneration, setRetryGeneration] = useState(0);
  useEffect(() => {
    let active = true;
    let readingMetadata = true;
    setBands(undefined);
    setBandsReadFailed(false);
    if (localBands && track) void (async () => {
      const metadata = await withSharedAudioMetadataRead(JSON.stringify([track.id, trackSourceIdentity, spotifySessionGeneration]), () => api.getTrackAnalysisFeature(track.id), () => active);
      if (!active || !metadata.sourceFingerprint) return;
      readingMetadata = false;
      const data = await loadLocalThreeBand(track.id, metadata.sourceFingerprint, () => active);
      if (active && data?.sourceFingerprint === metadata.sourceFingerprint) setBands({ track, data });
    })().catch(error => { if (active) { setBandsReadFailed(true); reportAudioReadFailure(readingMetadata ? 'audio_metadata' : 'local_bands', error); } });
    return () => { active = false; };
  }, [track, localBands, trackSourceIdentity, spotifySessionGeneration, retryGeneration]);
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
    let frameKey: unknown[] = [];

    const draw = () => {
      const state = useStore.getState();
      const d = deck === 'A' ? state.djDeckA : state.djDeckB;
      const width = canvas.clientWidth;
      const height = canvas.clientHeight;
      if (width <= 0 || height <= 0) return;
      const ratio = window.devicePixelRatio || 1;
      // Skip the redraw while nothing visible has moved (a paused deck costs nothing).
      const playheadPixel = d.duration > 0 ? Math.round(d.position / d.duration * width * ratio) : -1;
      const overview = localBands && bands?.track === d.track ? bands.data.overview : undefined;
      const bandDuration = overview ? overview.frames / overview.sampleRate : 0;
      const usableBands = overview && d.duration > 0 && Math.abs(bandDuration - d.duration) <= 0.1 ? overview : undefined;
      const nextFrameKey = [usableBands, d.waveformPeaks, width, height, ratio, d.duration, playheadPixel, d.loop.start, d.loop.end, d.loop.enabled, d.hotCues, d.track];
      if (nextFrameKey.every((value, index) => value === frameKey[index])) return;
      frameKey = nextFrameKey;
      if (canvas.width !== Math.round(width * ratio) || canvas.height !== Math.round(height * ratio)) {
        canvas.width = Math.round(width * ratio);
        canvas.height = Math.round(height * ratio);
      }
      const peaks = d.waveformPeaks;
      const key = [usableBands, peaks, canvas.width, canvas.height];
      if (key.some((value, index) => value !== cacheKey[index])) {
        cacheKey = key;
        cache.width = canvas.width;
        cache.height = canvas.height;
        const cctx = cache.getContext('2d');
        if (cctx) {
          cctx.setTransform(ratio, 0, 0, ratio, 0, 0);
          cctx.fillStyle = WAVEFORM_CHROME.overviewBackground;
          cctx.fillRect(0, 0, width, height);
          if (usableBands) {
            const all = [usableBands.low, usableBands.mid, usableBands.high];
            let scale = 0;
            for (const band of all) for (const value of band) scale = Math.max(scale, value);
            all.forEach((band, index) => {
              cctx.fillStyle = [palette.gradient.center, palette.gradient.mid, palette.gradient.edge][index];
              cctx.beginPath();
              const lane = height / 3;
              for (let x = 0; x < width; x++) {
                const first = Math.floor(x / width * band.length);
                const end = Math.min(band.length, Math.max(first + 1, Math.floor((x + 1) / width * band.length)));
                let peak = 0;
                for (let i = first; i < end; i++) peak = Math.max(peak, band[i]);
                const h = Math.max(1, scale > 0 ? peak / scale * (lane - 1) : 0);
                cctx.rect(x, index * lane + (lane - h) / 2, 1, h);
              }
              cctx.fill();
            });
          } else if (peaks?.length) {
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
        if (visibleSeconds && (peaks?.length || usableBands)) {
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
  }, [deck, visibleSeconds, localBands, bands]);

  const handleClick = useCallback((event: React.MouseEvent<HTMLCanvasElement>) => {
    const d = deck === 'A' ? useStore.getState().djDeckA : useStore.getState().djDeckB;
    if (!d.duration) return;
    const rect = event.currentTarget.getBoundingClientRect();
    const fraction = (event.clientX - rect.left) / rect.width;
    seek(deck, Math.max(0, Math.min(d.duration, fraction * d.duration)));
  }, [deck, seek]);

  const eligible = bands && bands.track === track && duration > 0 && Math.abs(bands.data.overview.frames / bands.data.overview.sampleRate - duration) <= 0.1;
  return (<>
    {localBands && <span className='dj-label' role='status'>{eligible ? 'Local bands - Low / Mid / High - Common peak scale' : 'Amplitude overview - Local bands unavailable or duration differs'}</span>}
    {localBands && bandsReadFailed && track && <span className='dj-label' role='status'>Local bands could not be loaded. Amplitude overview remains available. <button type='button' onClick={() => setRetryGeneration(value => value + 1)}>Retry local bands</button></span>}
    <canvas
      ref={ref}
      className='dj-deck-overview'
      role='img'
      aria-label={`Deck ${deck} ${localBands ? 'local band overview (low, mid, high); amplitude fallback when unavailable' : 'amplitude overview'} with cue markers and playhead; click to seek`}
      data-dj-overview-deck={deck}
      onClick={handleClick}
    />
  </>);
});
