/**
 * ViiB MediaHub - DJ Split Waveform (v2)
 *
 * Deck A owns the left 50% of the waveform band and Deck B the right 50%.
 * Each lane has its own toolbar (color mode + zoom), main scrolling waveform
 * and whole-track overview beneath it. Zoom is per deck.
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
import { DJDeckOverview } from './DJDeckOverview';
import { DJCanvasWaveformDeck } from './waveform/DJCanvasWaveformDeck';
import { DJWebGLWaveformDeck } from './webgl/DJWebGLWaveformDeck';
import { WAVEFORM_COLOR_MODES, type WaveformColorMode } from './waveform/waveformPalette';

const VISIBLE_SECONDS_DEFAULT = 10;
const VISIBLE_SECONDS_MIN = 2;
const VISIBLE_SECONDS_MAX = 60;
const ZOOM_PRESETS = [4, 8, 10, 16, 32] as const;

const clampSeconds = (value: number) => Math.max(VISIBLE_SECONDS_MIN, Math.min(VISIBLE_SECONDS_MAX, value));

const DJWaveformLane = React.memo(function DJWaveformLane({ deck, useWebGL }: { deck: DeckId; useWebGL: boolean }) {
  const [visibleSeconds, setVisibleSeconds] = useState(VISIBLE_SECONDS_DEFAULT);
  const [colorMode, setColorMode] = useState<WaveformColorMode>('rgb');
  const [webglUnavailable, setWebglUnavailable] = useState(false);
  const markUnavailable = useCallback(() => setWebglUnavailable(true), []);
  const renderWebGL = useWebGL && !webglUnavailable;

  const handleWheel = useCallback((event: React.WheelEvent) => {
    if (!event.ctrlKey && !event.metaKey) return;
    event.preventDefault();
    setVisibleSeconds(value => clampSeconds(value * (event.deltaY < 0 ? 1 / 1.15 : 1.15)));
  }, []);

  const toolbar = (
    <div className='dj-waveform-toolbar'>
      <div className='dj-segmented' role='group' aria-label={`Deck ${deck} waveform color`}>
        {WAVEFORM_COLOR_MODES.map(({ mode, label, title }) => (
          <button key={mode} type='button' className='dj-btn dj-btn-xs' data-deck-accent={deck}
            aria-pressed={colorMode === mode} title={title} onClick={() => setColorMode(mode)}>{label}</button>
        ))}
      </div>
      <div className='dj-segmented' role='group' aria-label={`Deck ${deck} waveform zoom`}>
        <button type='button' className='dj-btn dj-btn-xs dj-btn-icon' aria-label={`Zoom in Deck ${deck} waveform`}
          title='Zoom in (Ctrl+Scroll up)' onClick={() => setVisibleSeconds(value => clampSeconds(value / 1.5))}>+</button>
        <label className='dj-select-wrap'>
          <span className='sr-only'>{`Deck ${deck} visible seconds`}</span>
          <select className='dj-select dj-btn-xs' value={ZOOM_PRESETS.includes(Math.round(visibleSeconds) as typeof ZOOM_PRESETS[number]) ? Math.round(visibleSeconds) : ''}
            onChange={event => setVisibleSeconds(clampSeconds(Number(event.currentTarget.value)))}
            aria-label={`Deck ${deck} visible seconds`}>
            {!ZOOM_PRESETS.includes(Math.round(visibleSeconds) as typeof ZOOM_PRESETS[number]) && <option value=''>{`${visibleSeconds.toFixed(0)}s`}</option>}
            {ZOOM_PRESETS.map(seconds => <option key={seconds} value={seconds}>{`${seconds}s`}</option>)}
          </select>
        </label>
        <button type='button' className='dj-btn dj-btn-xs dj-btn-icon' aria-label={`Zoom out Deck ${deck} waveform`}
          title='Zoom out (Ctrl+Scroll down)' onClick={() => setVisibleSeconds(value => clampSeconds(value * 1.5))}>−</button>
      </div>
    </div>
  );
  const badge = <span className='dj-deck-badge dj-deck-badge-sm' data-deck={deck} aria-hidden='true'>{deck}</span>;

  return (
    <section className='dj-waveform-lane' data-dj-waveform-deck={deck} aria-label={`Deck ${deck} waveform`} onWheel={handleWheel}>
      <header className='dj-waveform-lane-header'>
        {deck === 'A' ? <>{badge}{toolbar}</> : <>{toolbar}{badge}</>}
      </header>
      <div className='dj-waveform-main-wrap'>
        <DJErrorBoundary componentName={`Deck ${deck} waveform`}>
          {renderWebGL
            ? <DJWebGLWaveformDeck deck={deck} visibleSeconds={visibleSeconds} colorMode={colorMode} onUnavailable={markUnavailable} />
            : <DJCanvasWaveformDeck deck={deck} visibleSeconds={visibleSeconds} colorMode={colorMode} />}
        </DJErrorBoundary>
      </div>
      <DJDeckOverview deck={deck} visibleSeconds={visibleSeconds} />
    </section>
  );
});

export const DJSplitWaveform: React.FC = React.memo(() => {
  const useWebGLWaveform = useStore(s => s.djMixer?.useWebGLWaveform);
  const useWebGL = shouldUseAdvancedWebGL() && !!useWebGLWaveform;
  return (
    <div className='dj-waveform-split' data-dj-waveform-split data-dj-renderer={useWebGL ? 'webgl' : 'canvas'}>
      <DJWaveformLane deck='A' useWebGL={useWebGL} />
      <DJWaveformLane deck='B' useWebGL={useWebGL} />
    </div>
  );
});
DJSplitWaveform.displayName = 'DJSplitWaveform';

export default DJSplitWaveform;
