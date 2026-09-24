import React, { useCallback, useEffect, useState } from 'react';
import { useDJAudioEngineActions } from '../../../hooks/useDJAudioEngine';
import type { DeckId } from '../../../slices/djMixerSlice';

export type DJStemBus = 'vocals' | 'drums' | 'bass' | 'music';
export type DJStemMode = 'full' | 'stems' | 'fallback';

export interface DJStemBusState {
  gain: number;
  muted: boolean;
  solo: boolean;
}

export type DJStemState = Record<DJStemBus, DJStemBusState>;

export interface DJStemStatus {
  mode: DJStemMode;
  available: boolean;
  bufferedSeconds: number;
  underruns: number;
  supportsKeyLock: boolean;
  supportsScratch: boolean;
  supportsSampleAccurateLoop: boolean;
  error?: string;
}

const BUSES: Array<{ id: DJStemBus; label: string }> = [
  { id: 'vocals', label: 'VOCAL' },
  { id: 'drums', label: 'DRUMS' },
  { id: 'bass', label: 'BASS' },
  { id: 'music', label: 'MUSIC' },
];

const DEFAULT_STEM_STATE: DJStemState = {
  vocals: { gain: 1, muted: false, solo: false },
  drums: { gain: 1, muted: false, solo: false },
  bass: { gain: 1, muted: false, solo: false },
  music: { gain: 1, muted: false, solo: false },
};

const DEFAULT_STEM_STATUS: DJStemStatus = {
  mode: 'full',
  available: false,
  bufferedSeconds: 0,
  underruns: 0,
  supportsKeyLock: false,
  supportsScratch: false,
  supportsSampleAccurateLoop: false,
};

export function formatDJStemStatus(status: DJStemStatus): string {
  if (status.mode === 'fallback') {
    return status.error ? `Full track · stems unavailable (${status.error})` : 'Full track · stem playback unavailable';
  }
  if (!status.available) return 'Full track · no stem package';
  if (status.mode === 'stems' && status.bufferedSeconds < 0.2) return 'Stems · buffering';
  if (status.mode === 'stems' && status.underruns > 0) return `Stems · ${status.underruns} audio underrun${status.underruns === 1 ? '' : 's'}`;
  return status.mode === 'stems' ? `Stems ready · ${status.bufferedSeconds.toFixed(1)} s buffered` : 'Stem package ready';
}

function clampGain(value: number): number {
  return Math.max(0, Math.min(1, value));
}

/** Compact, per-deck four-bus controls. The audio engine remains the source of truth. */
export function DJStemControls({ deck }: { deck: DeckId }) {
  const {
    setStemMode,
    setStemGain,
    setStemMuted,
    setStemSolo,
    getStemState,
    getStemStatus,
  } = useDJAudioEngineActions();
  const [stemState, setLocalStemState] = useState<DJStemState>(DEFAULT_STEM_STATE);
  const [status, setStatus] = useState<DJStemStatus>(DEFAULT_STEM_STATUS);
  const [changingMode, setChangingMode] = useState(false);

  const refresh = useCallback(() => {
    try {
      setLocalStemState(getStemState(deck));
      setStatus(getStemStatus(deck));
    } catch {
      setStatus(DEFAULT_STEM_STATUS);
    }
  }, [deck, getStemState, getStemStatus]);

  useEffect(() => {
    refresh();
    const timer = window.setInterval(refresh, 500);
    return () => window.clearInterval(timer);
  }, [refresh]);

  const chooseMode = async (mode: 'full' | 'stems') => {
    if (mode === 'stems' && !status.available) return;
    setChangingMode(true);
    let operationError: string | undefined;
    try {
      await setStemMode(deck, mode);
    } catch (error) {
      operationError = error instanceof Error ? error.message : 'Stem mode could not start';
    } finally {
      setChangingMode(false);
      refresh();
      if (operationError) {
        setStatus(current => ({ ...current, mode: 'fallback', error: operationError }));
      }
    }
  };

  const canControlStems = status.available && status.mode !== 'fallback';
  const stemMode = status.mode === 'stems';
  const statusText = formatDJStemStatus(status);
  const featureLimit = stemMode && (!status.supportsKeyLock || !status.supportsScratch || !status.supportsSampleAccurateLoop)
    ? [
        !status.supportsKeyLock ? 'Key Lock unavailable in Stem Mode' : '',
        !status.supportsScratch ? 'Scratch unavailable in Stem Mode' : '',
        !status.supportsSampleAccurateLoop ? 'Stem loops use standard seek timing' : '',
      ].filter(Boolean).join(' · ')
    : '';

  return (
    <section className="flex min-w-0 items-start gap-1.5" aria-label={`Deck ${deck} stem controls`}>
      <div className="flex items-center gap-0.5" role="group" aria-label="Stem playback mode">
        <button
          type="button"
          className={`min-h-6 rounded border px-1.5 text-[9px] font-bold ${!stemMode ? 'border-[var(--dj-border-hover)] bg-[var(--dj-surface-3)] text-[var(--dj-text-primary)]' : 'border-[var(--dj-border)] bg-[var(--dj-surface-0)] text-[var(--dj-text-secondary)] hover:text-[var(--dj-text-primary)]'}`}
          aria-pressed={!stemMode}
          onClick={() => void chooseMode('full')}
          disabled={changingMode}
          title="Play the original full track"
        >FULL</button>
        <button
          type="button"
          className={`min-h-6 rounded border px-1.5 text-[9px] font-bold ${stemMode ? 'border-[var(--dj-info)] bg-[var(--dj-surface-2)] text-[var(--dj-info)]' : 'border-[var(--dj-border)] bg-[var(--dj-surface-0)] text-[var(--dj-text-secondary)] hover:text-[var(--dj-text-primary)]'}`}
          aria-pressed={stemMode}
          onClick={() => void chooseMode('stems')}
          disabled={changingMode || !status.available}
          title={status.available ? 'Play the four prepared stems' : 'No valid stem package is available for this track'}
        >STEMS</button>
      </div>

      <div className="flex items-center gap-0.5" role="group" aria-label="Stem mute controls">
        {BUSES.map(({ id, label }) => {
          const busState = stemState[id];
          return (
            <button
              key={id}
              type="button"
              className={`min-h-6 rounded border px-1.5 text-[9px] font-bold tracking-wide ${busState.muted ? 'border-[var(--dj-warning)] bg-[var(--dj-surface-2)] text-[var(--dj-warning)]' : 'border-[var(--dj-border)] bg-[var(--dj-surface-0)] text-[var(--dj-text-secondary)] hover:border-[var(--dj-border-hover)] hover:text-[var(--dj-text-primary)]'}`}
              aria-label={`${busState.muted ? 'Unmute' : 'Mute'} ${label.toLowerCase()} stem on Deck ${deck}`}
              aria-pressed={!busState.muted}
              title={`${label} · click to ${busState.muted ? 'unmute' : 'mute'}`}
              disabled={!canControlStems}
              onClick={() => setStemMuted(deck, id, !busState.muted)}
            >{label}</button>
          );
        })}
      </div>

      <details className="shrink-0">
        <summary className="flex min-h-6 cursor-pointer list-none items-center rounded border border-[var(--dj-border)] bg-[var(--dj-surface-0)] px-1.5 text-[9px] font-bold text-[var(--dj-text-secondary)] hover:text-[var(--dj-text-primary)] focus-visible:outline focus-visible:outline-2 focus-visible:outline-[var(--dj-info)]" aria-label={`Advanced stem mix controls for Deck ${deck}`}>
          MIX
        </summary>
        <div className="mt-1 w-64 rounded border border-[var(--dj-border-light)] bg-[var(--dj-surface-0)] p-2 shadow-xl" role="group" aria-label="Stem gain and solo controls">
          {BUSES.map(({ id, label }) => {
            const busState = stemState[id];
            return (
              <div key={id} className="grid grid-cols-[3.5rem_1fr_2rem] items-center gap-2 py-1">
                <label className="text-[9px] font-bold text-[var(--dj-text-secondary)]" htmlFor={`stem-${deck}-${id}-gain`}>{label}</label>
                <input
                  id={`stem-${deck}-${id}-gain`}
                  type="range"
                  min={0}
                  max={1}
                  step={0.01}
                  value={clampGain(busState.gain)}
                  disabled={!canControlStems}
                  aria-label={`${label} stem level for Deck ${deck}`}
                  onChange={event => setStemGain(deck, id, Number(event.currentTarget.value))}
                />
                <button
                  type="button"
                  className={`min-h-6 rounded border text-[9px] font-bold ${busState.solo ? 'border-[var(--dj-warning)] bg-[var(--dj-surface-2)] text-[var(--dj-warning)]' : 'border-[var(--dj-border)] bg-[var(--dj-surface-0)] text-[var(--dj-text-secondary)]'}`}
                  aria-label={`${busState.solo ? 'Unsolo' : 'Solo'} ${label.toLowerCase()} stem on Deck ${deck}`}
                  aria-pressed={busState.solo}
                  disabled={!canControlStems}
                  onClick={() => setStemSolo(deck, id, !busState.solo)}
                >S</button>
              </div>
            );
          })}
        </div>
      </details>

      <span
        className={`min-w-0 truncate text-[9px] ${status.mode === 'fallback' || (!status.available && stemMode) ? 'text-[var(--dj-warning)]' : status.mode === 'stems' && status.bufferedSeconds < 0.2 ? 'text-[var(--dj-warning)]' : 'text-[var(--dj-text-muted)]'}`}
        role="status"
        aria-live="polite"
        aria-label={[statusText, featureLimit].filter(Boolean).join(' · ')}
        title={[statusText, featureLimit].filter(Boolean).join(' · ')}
      >
        {statusText}{featureLimit ? ` · ${featureLimit}` : ''}
      </span>
    </section>
  );
}

export default React.memo(DJStemControls);
