import React, { useCallback, useEffect, useState } from 'react';
import { useDJAudioEngineActions } from '../../../hooks/useDJAudioEngine';
import { useStore } from '../../../store';
import type { DeckId } from '../../../slices/djMixerSlice';

export type DJStemBus = 'vocals' | 'drums' | 'bass' | 'music';
export type DJStemMode = 'full' | 'stems' | 'fallback';
export type DJStemMixPreset = 'full' | 'acapella' | 'instrumental';

export interface DJStemBusState {
  gain: number;
  muted: boolean;
  solo: boolean;
}

export type DJStemState = Record<DJStemBus, DJStemBusState>;
export type DJStemMuteState = Record<DJStemBus, boolean>;

export interface DJStemPresetTransition {
  activePreset: DJStemMixPreset | null;
  muteState: DJStemMuteState;
  restoreState: DJStemMuteState | null;
}

export function stemPresetMuteState(preset: DJStemMixPreset): DJStemMuteState {
  switch (preset) {
    case 'full': return { vocals: false, drums: false, bass: false, music: false };
    case 'acapella': return { vocals: false, drums: true, bass: true, music: true };
    case 'instrumental': return { vocals: true, drums: false, bass: false, music: false };
  }
}

// Presets capture and restore mute state only. Bus gains and solo state remain
// under the DJ's control when presets are applied or toggled back off.
export function transitionDJStemPreset(
  activePreset: DJStemMixPreset | null,
  selectedPreset: DJStemMixPreset,
  current: DJStemMuteState,
  restore: DJStemMuteState | null,
): DJStemPresetTransition {
  if (activePreset === selectedPreset) {
    return { activePreset: null, muteState: restore ? { ...restore } : { ...current }, restoreState: null };
  }
  return {
    activePreset: selectedPreset,
    muteState: stemPresetMuteState(selectedPreset),
    restoreState: restore ? { ...restore } : { ...current },
  };
}

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

/** Compact, per-deck four-bus controls. The audio engine remains the source of truth.
 * `compact` renders the FULL/STEMS switch plus a STEMS ▾ popover holding the
 * bus mutes, presets and gains so the deck toolbar fits on one row. */
export function DJStemControls({ deck, compact = false }: { deck: DeckId; compact?: boolean }) {
  const trackID = useStore(state => deck === 'A' ? state.djDeckA.track?.id : state.djDeckB.track?.id);
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
  const [activePreset, setActivePreset] = useState<DJStemMixPreset | null>(null);
  const restoreMuteState = React.useRef<Partial<Record<DeckId, DJStemMuteState>>>({});

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

  useEffect(() => {
    setActivePreset(null);
    restoreMuteState.current[deck] = undefined;
  }, [deck, trackID]);

  const chooseMode = async (mode: 'full' | 'stems') => {
    if (mode === 'stems' && !status.available) return;
    // Mode changes may reset the engine's stem buses; start the next stem
    // session with no stale preset indicator or mute-restore snapshot.
    setActivePreset(null);
    restoreMuteState.current[deck] = undefined;
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

  const choosePreset = (preset: DJStemMixPreset) => {
    const current = getStemState(deck);
    const currentMutes: DJStemMuteState = {
      vocals: current.vocals.muted,
      drums: current.drums.muted,
      bass: current.bass.muted,
      music: current.music.muted,
    };
    const transition = transitionDJStemPreset(activePreset, preset, currentMutes, restoreMuteState.current[deck] ?? null);
    for (const bus of BUSES) {
      if (currentMutes[bus.id] !== transition.muteState[bus.id]) {
        setStemMuted(deck, bus.id, transition.muteState[bus.id]);
      }
    }
    restoreMuteState.current[deck] = transition.restoreState ?? undefined;
    setActivePreset(transition.activePreset);
    refresh();
  };

  const toggleBusMute = (bus: DJStemBus, muted: boolean) => {
    setStemMuted(deck, bus, muted);
    restoreMuteState.current[deck] = undefined;
    setActivePreset(null);
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

  if (compact) {
    const statusLabel = [statusText, featureLimit].filter(Boolean).join(' · ');
    const statusState = status.mode === 'fallback' || (stemMode && status.bufferedSeconds < 0.2) ? 'warn' : status.available ? 'ok' : undefined;
    return (
      <section className="dj-toolbar-cluster dj-stems-compact" aria-label={`Deck ${deck} stem controls`}>
        <div className="dj-segmented" role="group" aria-label="Stem playback mode">
          <button type="button" className="dj-btn" data-deck-accent={deck} aria-pressed={!stemMode} onClick={() => void chooseMode('full')}
            disabled={changingMode} title="Play the original full track">FULL</button>
          <button type="button" className="dj-btn" data-deck-accent={deck} aria-pressed={stemMode} onClick={() => void chooseMode('stems')}
            disabled={changingMode || !status.available} title={status.available ? 'Play the four prepared stems' : 'No valid stem package is available for this track'}>STEMS</button>
        </div>
        <details className="dj-stems-popover">
          <summary className="dj-btn" aria-label={`Stem mix controls for Deck ${deck}`} title={statusLabel}>
            <span className="dj-status-dot" data-state={statusState} aria-hidden="true" />MIX ▾
          </summary>
          <div className="dj-popover dj-stems-panel" role="group" aria-label="Stem mutes, presets and levels">
            <p className="dj-label" role="status" aria-live="polite">{statusLabel}</p>
            <div className="dj-segmented" role="group" aria-label="Stem mute controls">
              {BUSES.map(({ id, label }) => {
                const busState = stemState[id];
                return (
                  <button key={id} type="button" className="dj-btn dj-btn-xs dj-btn-warn" aria-pressed={busState.muted}
                    aria-label={`${busState.muted ? 'Unmute' : 'Mute'} ${label.toLowerCase()} stem on Deck ${deck}`}
                    disabled={!canControlStems} onClick={() => toggleBusMute(id, !busState.muted)}>{label}</button>
                );
              })}
            </div>
            <div className="dj-segmented" role="group" aria-label={`Stem mix presets for Deck ${deck}`}>
              {(['full', 'acapella', 'instrumental'] as const).map(preset => (
                <button key={preset} type="button" className="dj-btn dj-btn-xs" data-deck-accent={deck} aria-pressed={activePreset === preset}
                  title={activePreset === preset ? 'Click again to restore the mute state from before the preset' : `Apply ${preset} mix; bus gains stay unchanged`}
                  disabled={!canControlStems} onClick={() => choosePreset(preset)}>{preset.toUpperCase()}</button>
              ))}
            </div>
            {BUSES.map(({ id, label }) => {
              const busState = stemState[id];
              return (
                <div key={id} className="dj-stem-row">
                  <label className="dj-label" htmlFor={`stem-${deck}-${id}-gain`}>{label}</label>
                  <input id={`stem-${deck}-${id}-gain`} type="range" min={0} max={1} step={0.01} value={clampGain(busState.gain)}
                    disabled={!canControlStems} aria-label={`${label} stem level for Deck ${deck}`}
                    onChange={event => setStemGain(deck, id, Number(event.currentTarget.value))} />
                  <button type="button" className="dj-btn dj-btn-xs dj-btn-warn" aria-pressed={busState.solo}
                    aria-label={`${busState.solo ? 'Unsolo' : 'Solo'} ${label.toLowerCase()} stem on Deck ${deck}`}
                    disabled={!canControlStems} onClick={() => setStemSolo(deck, id, !busState.solo)}>S</button>
                </div>
              );
            })}
          </div>
        </details>
      </section>
    );
  }

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
              onClick={() => toggleBusMute(id, !busState.muted)}
            >{label}</button>
          );
        })}
      </div>

      <details className="shrink-0">
        <summary className="flex min-h-6 cursor-pointer list-none items-center rounded border border-[var(--dj-border)] bg-[var(--dj-surface-0)] px-1.5 text-[9px] font-bold text-[var(--dj-text-secondary)] hover:text-[var(--dj-text-primary)] focus-visible:outline focus-visible:outline-2 focus-visible:outline-[var(--dj-info)]" aria-label={`Advanced stem mix controls for Deck ${deck}`}>
          MIX
        </summary>
        <div className="mt-1 w-64 rounded border border-[var(--dj-border-light)] bg-[var(--dj-surface-0)] p-2 shadow-xl" role="group" aria-label="Stem gain and solo controls">
          <div className="mb-2 flex items-center gap-1" role="group" aria-label={`Stem mix presets for Deck ${deck}`}>
            {(['full', 'acapella', 'instrumental'] as const).map(preset => (
              <button key={preset} type="button"
                className={`min-h-6 flex-1 rounded border px-1 text-[9px] font-bold ${activePreset === preset ? 'border-[var(--dj-info)] bg-[var(--dj-surface-2)] text-[var(--dj-info)]' : 'border-[var(--dj-border)] bg-[var(--dj-surface-0)] text-[var(--dj-text-secondary)] hover:text-[var(--dj-text-primary)]'}`}
                aria-pressed={activePreset === preset}
                title={activePreset === preset ? 'Click again to restore the mute state from before the preset' : `Apply ${preset} mix; bus gains stay unchanged`}
                disabled={!canControlStems}
                onClick={() => choosePreset(preset)}
              >{preset.toUpperCase()}</button>
            ))}
          </div>
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
