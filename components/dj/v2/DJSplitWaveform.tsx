/**
 * ViiB MediaHub - DJ Split Waveform (v2)
 *
 * Stacked parallel detail waveforms (Deck A on top, Deck B directly below)
 * sharing one aligned playhead position in a full-lane coordinate space.
 * Each lane places its badge, color mode, three-band and zoom controls at its
 * outer end, with the timeline filling the remaining width to the opposite edge.
 * Whole-track overview bars live in each Deck Header.
 *
 * Renderer: WebGL per lane when enabled and available, otherwise Canvas 2D.
 * A lane that fails to create a WebGL context falls back on its own.
 *
 * @module components/dj/v2/DJSplitWaveform
 */

import React, { useCallback, useState } from 'react';
import { useStore } from '../../../store';
import { shouldUseAdvancedWebGL } from '../../../lib/webglSafety';
import type { DeckId } from '../../../slices/djMixerSlice';
import { DJErrorBoundary } from './DJErrorBoundary';
import { setDeckWaveformView, useDeckWaveformView } from './DJDeckOverview';
import { DJCanvasWaveformDeck } from './waveform/DJCanvasWaveformDeck';
import type { WaveformOrientation } from './waveform/canvasWaveformRenderer';
import { DJWebGLWaveformDeck } from './webgl/DJWebGLWaveformDeck';
import { WAVEFORM_COLOR_MODES, type WaveformColorMode } from './waveform/waveformPalette';

const VISIBLE_SECONDS_MIN = 2;
const VISIBLE_SECONDS_MAX = 60;
const ZOOM_PRESETS = [4, 8, 10, 16, 32] as const;

const clampSeconds = (value: number) => Math.max(VISIBLE_SECONDS_MIN, Math.min(VISIBLE_SECONDS_MAX, value));

export const DJWaveformLane = React.memo(function DJWaveformLane({
  deck,
  useWebGL = false,
  orientation = 'horizontal',
}: {
  deck: DeckId;
  useWebGL?: boolean;
  orientation?: WaveformOrientation;
}) {
  const { visibleSeconds, localBands, retryGeneration, bandSelection } = useDeckWaveformView(deck);
  const [colorMode, setColorMode] = useState<WaveformColorMode>('rgb');
  const [webglUnavailable, setWebglUnavailable] = useState(false);
  const markUnavailable = useCallback(() => setWebglUnavailable(true), []);
  const renderWebGL = orientation === 'horizontal' && useWebGL && !webglUnavailable && !localBands;

  const updateVisibleSeconds = useCallback((next: number | ((prev: number) => number)) => {
    const resolved = clampSeconds(typeof next === 'function' ? next(visibleSeconds) : next);
    setDeckWaveformView(deck, { visibleSeconds: resolved });
  }, [deck, visibleSeconds]);

  const toggleLocalBands = useCallback(() => {
    setDeckWaveformView(deck, { localBands: !localBands });
  }, [deck, localBands]);

  const handleWheel = useCallback((event: React.WheelEvent) => {
    if (!event.ctrlKey && !event.metaKey) return;
    event.preventDefault();
    updateVisibleSeconds(value => value * (event.deltaY < 0 ? 1 / 1.15 : 1.15));
  }, [updateVisibleSeconds]);

  const toolbar = (
    <div className='dj-waveform-toolbar'>
      {localBands ? (
        <div className='dj-segmented dj-waveform-band-selector' role='group' aria-label={`Deck ${deck} waveform band`}>
          {(['all', 'low', 'mid', 'high'] as const).map(band => (
            <button key={band} type='button' className='dj-btn dj-btn-xs' data-deck-accent={deck}
              aria-pressed={bandSelection === band}
              title={band === 'all' ? 'Show all three bands' : `Isolate ${band} band at full size`}
              onClick={() => setDeckWaveformView(deck, { bandSelection: band })}>
              {band.toUpperCase()}
            </button>
          ))}
        </div>
      ) : <div className='dj-segmented' role='group' aria-label={`Deck ${deck} waveform color`}>
        {WAVEFORM_COLOR_MODES.map(({ mode, label, title }) => (
          <button key={mode} type='button' className='dj-btn dj-btn-xs' data-deck-accent={deck}
            aria-pressed={colorMode === mode} title={title} onClick={() => setColorMode(mode)}>{label}</button>
        ))}
      </div>}
      <button type='button' className='dj-btn dj-btn-xs' aria-pressed={localBands} title='Scrolling lane and overview: Spotify low/mid/high first, local estimate only when provider data is unavailable; amplitude fallback when timelines differ' onClick={toggleLocalBands}>Three-band</button>
      <div className='dj-segmented' role='group' aria-label={`Deck ${deck} waveform zoom`}>
        <button type='button' className='dj-btn dj-btn-xs dj-btn-icon' aria-label={`Zoom in Deck ${deck} waveform`}
          title='Zoom in (Ctrl+Scroll up)' onClick={() => updateVisibleSeconds(value => value / 1.5)}>+</button>
        <label className='dj-select-wrap'>
          <span className='sr-only'>{`Deck ${deck} visible seconds`}</span>
          <select className='dj-select dj-btn-xs' value={ZOOM_PRESETS.includes(Math.round(visibleSeconds) as typeof ZOOM_PRESETS[number]) ? Math.round(visibleSeconds) : ''}
            onChange={event => updateVisibleSeconds(Number(event.currentTarget.value))}
            aria-label={`Deck ${deck} visible seconds`}>
            {!ZOOM_PRESETS.includes(Math.round(visibleSeconds) as typeof ZOOM_PRESETS[number]) && <option value=''>{`${visibleSeconds.toFixed(0)}s`}</option>}
            {ZOOM_PRESETS.map(seconds => <option key={seconds} value={seconds}>{`${seconds}s`}</option>)}
          </select>
        </label>
        <button type='button' className='dj-btn dj-btn-xs dj-btn-icon' aria-label={`Zoom out Deck ${deck} waveform`}
          title='Zoom out (Ctrl+Scroll down)' onClick={() => updateVisibleSeconds(value => value * 1.5)}>−</button>
      </div>
    </div>
  );
  const badge = <span className='dj-deck-badge dj-deck-badge-sm' data-deck={deck} aria-hidden='true'>{deck}</span>;

  return (
    <section
      className='dj-waveform-lane'
      data-dj-waveform-deck={deck}
      data-dj-waveform-orientation={orientation}
      aria-label={`Deck ${deck} waveform`}
      onWheel={handleWheel}
    >
      <header className='dj-waveform-lane-header'>
        {badge}
        {toolbar}
      </header>
      <div className='dj-waveform-main-wrap'>
        <DJErrorBoundary componentName={`Deck ${deck} waveform`}>
          {renderWebGL
            ? <DJWebGLWaveformDeck deck={deck} visibleSeconds={visibleSeconds} colorMode={colorMode} onUnavailable={markUnavailable} />
            : <DJCanvasWaveformDeck deck={deck} visibleSeconds={visibleSeconds} colorMode={colorMode} localBands={localBands} retryGeneration={retryGeneration} orientation={orientation} bandSelection={bandSelection} />}
        </DJErrorBoundary>
      </div>
    </section>
  );
});

export const DJSplitWaveform: React.FC<{ orientation?: WaveformOrientation }> = React.memo(({ orientation = 'horizontal' }) => {
  const useWebGLWaveform = useStore(s => s.djMixer?.useWebGLWaveform);
  const useWebGL = shouldUseAdvancedWebGL() && !!useWebGLWaveform && orientation === 'horizontal';
  return (
    <div
      className='dj-waveform-split'
      data-dj-waveform-split
      data-dj-waveform-orientation={orientation}
      data-dj-renderer={useWebGL ? 'webgl' : 'canvas'}
    >
      <DJWaveformLane deck='A' useWebGL={useWebGL} orientation={orientation} />
      <DJWaveformLane deck='B' useWebGL={useWebGL} orientation={orientation} />
    </div>
  );
});
DJSplitWaveform.displayName = 'DJSplitWaveform';

export default DJSplitWaveform;
