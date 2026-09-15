/**
 * ViiB MediaHub - DJ Beat Grid Edit Component
 * 
 * Compact beat grid alignment editor.
 * Allows nudging the beat grid offset in ±1ms and ±10ms increments.
 * 
 * @module components/dj/v2/DJBeatGridEdit
 */

import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { ChevronsLeft, ChevronLeft, ChevronRight, ChevronsRight, Lock, RotateCcw, Unlock } from 'lucide-react';
import type { DeckId } from '../../../slices/djMixerSlice';
import { useStore } from '../../../store';
import { DJBeatGridStatus } from './DJBeatGridStatus';
import { detectBPM, type BPMResult } from '../../../lib/bpmDetection';
import { measureGridAlignment } from '../../../lib/tempoEvidence';
import { api } from '../../../services/api';

interface DJBeatGridEditProps {
  deck: DeckId;
}

const BTN = 'w-11 h-11 rounded bg-[#222] text-neutral-400 hover:text-white border border-[#333] hover:border-neutral-500 transition-colors flex items-center justify-center flex-shrink-0';

export const DJBeatGridEdit: React.FC<DJBeatGridEditProps> = ({ deck }) => {
  const deckState = useStore(state => deck === 'A' ? state.djDeckA : state.djDeckB);
  const setDeckAnalysis = useStore(state => state.setDeckAnalysis);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [analysis, setAnalysis] = useState<BPMResult | null>(null);
  const [progress, setProgress] = useState<number | null>(null);
  const request = useRef(0);
  useEffect(() => {
    request.current++;
    setAnalysis(null); setProgress(null); setError(null);
    return () => { request.current++; };
  }, [deckState.track?.id]);
  const analyze = async () => {
    if (!deckState.track || progress !== null) return;
    const id = deckState.track.id, token = ++request.current;
    setError(null); setProgress(0); setAnalysis(null);
    try {
      const result = await detectBPM(`/api/audio/${encodeURIComponent(id)}`, p => {
        if (request.current === token) setProgress(p);
      });
      const current = deck === 'A' ? useStore.getState().djDeckA : useStore.getState().djDeckB;
      if (request.current !== token || current.track?.id !== id) return;
      setAnalysis(result);
      setDeckAnalysis(deck, { tempoEvidence: { ...result.evidence, alignment: measureGridAlignment(result.onsetSections, current.beatGrid ?? []) } });
      if (!result.bpm) setError('Not enough rhythmic evidence. Keep the current grid and review it manually.');
    } catch (cause) {
      if (request.current === token) setError(cause instanceof Error ? cause.message : 'Analysis failed');
    } finally { if (request.current === token) setProgress(null); }
  };
  const applyAnalysis = () => {
    if (!analysis?.bpm || deckState.beatGridLocked || saving) return;
    setDeckAnalysis(deck, { bpm: analysis.bpm, bpmConfidence: analysis.confidence, beatGrid: analysis.beatGrid,
      beatGridSource: 'measured', beatGridLocked: false, downbeatIndices: null, tempoEvidence: analysis.evidence });
  };
  const evidence = deckState.tempoEvidence;
  const percent = (value: number | null | undefined) => value != null && Number.isFinite(value) ? `${Math.round(Math.max(0, Math.min(1, value)) * 100)}%` : 'unknown';
  const alignment = evidence?.alignment;
  const beats = deckState.beatGrid;
  const downbeats = deckState.downbeatIndices ?? [];
  const locked = deckState.beatGridLocked;
  const offsetMs = Math.round(deckState.beatGridOffset * 1000);

  const isDynamic = useMemo(() => {
    if (!beats || beats.length < 3) return false;
    const intervals = beats.slice(1).map((beat, index) => beat - beats[index]);
    const first = intervals[0];
    return intervals.some(interval => Math.abs(interval - first) > 0.003);
  }, [beats]);

  const persist = useCallback(async (nextBeats: number[], nextDownbeats: number[], nextLocked: boolean, bpm?: number) => {
    if (!deckState.track || nextBeats.length === 0) return;
    setSaving(true);
    setError(null);
    try {
      const saved = await api.updateTrackBeatGrid(deckState.track.id, {
        beats: nextBeats,
        downbeatIndices: nextDownbeats,
        locked: nextLocked,
        ...(isDynamic ? {} : { bpm: bpm ?? deckState.originalBpm ?? undefined }),
      });
      if ((deck === 'A' ? useStore.getState().djDeckA : useStore.getState().djDeckB).track?.id !== deckState.track.id) return;
      setDeckAnalysis(deck, {
        beatGridSource: 'manual',
        beatGrid: saved.beats,
        downbeatIndices: saved.downbeatIndices,
        beatGridLocked: saved.locked,
        ...(bpm === undefined ? {} : { bpm }),
      });
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Could not save beatgrid');
    } finally {
      setSaving(false);
    }
  }, [deck, deckState.track, deckState.originalBpm, isDynamic, setDeckAnalysis]);

  const shift = useCallback((delta: number) => {
    if (!beats || locked || saving) return;
    // Never manufacture a negative timeline; preserve exact spacing and make
    // the edit durable rather than storing a display-only offset.
    const safeDelta = Math.max(delta, -beats[0]);
    void persist(beats.map(beat => beat + safeDelta), downbeats, true);
  }, [beats, downbeats, locked, persist, saving]);

  const reset = useCallback(() => {
    if (!beats || locked || saving) return;
    void persist(beats, downbeats, false);
  }, [beats, downbeats, locked, persist, saving]);

  const rescale = useCallback((factor: number) => {
    if (!beats || beats.length === 0 || locked || saving) return;
    const first = beats[0];
    const bpm = deckState.originalBpm ? deckState.originalBpm / factor : undefined;
    void persist(beats.map(beat => first + (beat - first) * factor), downbeats, true, bpm);
  }, [beats, deckState.originalBpm, downbeats, locked, persist, saving]);

  const toggleLock = useCallback(() => {
    if (!beats || saving) return;
    void persist(beats, downbeats, !locked);
  }, [beats, downbeats, locked, persist, saving]);

  const clear = useCallback(async () => {
    if (!deckState.track || saving) return;
    setSaving(true);
    setError(null);
    try {
      await api.resetTrackBeatGrid(deckState.track.id);
      if ((deck === 'A' ? useStore.getState().djDeckA : useStore.getState().djDeckB).track?.id !== deckState.track.id) return;
      setDeckAnalysis(deck, { beatGridSource: 'unknown', beatGrid: null, downbeatIndices: null, beatGridLocked: false });
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Could not reset beatgrid');
    } finally {
      setSaving(false);
    }
  }, [deck, deckState.track, saving, setDeckAnalysis]);

  const disabled = !deckState.track || !beats?.length || saving;

  return (
    <div className='flex flex-col items-center gap-1 px-2 flex-shrink-0' aria-live='polite'>
      <DJBeatGridStatus deck={deck} />
      <div className='max-w-64 space-y-1 text-center text-[10px] text-text-secondary'>
        <p>Tempo evidence: {percent(evidence?.score ?? deckState.bpmConfidence)}{evidence ? ` · ${evidence.source === 'browser' ? 'Track scan' : 'Library analysis'}` : ''}</p>
        {evidence?.bpm != null && evidence.bpm > 0 && <p>Detected: {evidence.bpm.toFixed(2)} BPM{evidence.alternateBpm != null ? ` · Alternative: ${evidence.alternateBpm.toFixed(1)}` : ''}</p>}
        <p>Section agreement: {percent(evidence?.stability)}{evidence?.sections != null ? ` (${evidence.sections} rhythmic sections)` : ''}</p>
        <p>Grid review: {locked && deckState.beatGridSource === 'manual' ? 'reviewed & locked' : 'needs review'}</p>
        {alignment ? <>
          <p>Onsets within 35 ms: {percent(alignment.matched)}</p>
          <p title="Median onset offset from the current grid. Section shift compares the earliest and latest measured sections; offsets wrap to the nearest beat, so this cannot rule out large drift.">Typical offset: {Math.round(alignment.offsetMs)} ms · Section shift: {alignment.driftMs == null ? 'unknown' : `${Math.round(alignment.driftMs)} ms`}</p>
        </> : <p>Alignment &amp; drift: not measured</p>}
        <p>Scores describe rhythmic evidence, not accuracy probability. Check the intro, middle and outro before locking.</p>
      </div>
      <button disabled={!deckState.track || progress !== null || saving} onClick={() => void analyze()} className='rounded border border-brand px-3 py-2 text-xs text-brand'>
        {progress === null ? 'Analyze track' : `Analyzing ${Math.round(progress * 100)}%`}
      </button>
      {!!analysis?.bpm && !locked && <button disabled={saving || progress !== null} onClick={applyAnalysis} className='rounded border border-brand px-3 py-2 text-xs text-brand'>Use detected tempo &amp; grid</button>}
      <span className='text-[10px] text-neutral-500 font-bold uppercase tracking-wider'>
        GRID {isDynamic ? 'DYNAMIC' : 'STRAIGHT'}
      </span>
      <div className='flex items-center gap-1'>
        <button disabled={disabled || locked} onClick={() => shift(-0.01)} className={BTN} aria-label='Shift beat grid -10ms' title='Shift grid -10ms and save'>
          <ChevronsLeft size={18} aria-hidden />
        </button>
        <button disabled={disabled || locked} onClick={() => shift(-0.001)} className={BTN} aria-label='Shift beat grid -1ms' title='Shift grid -1ms and save'>
          <ChevronLeft size={18} aria-hidden />
        </button>
        <button
          disabled={disabled || locked}
          onClick={reset}
          className={`w-12 h-11 rounded border transition-colors font-mono text-[11px] flex items-center justify-center flex-shrink-0
            ${offsetMs !== 0
              ? 'bg-amber-600/20 text-amber-300 border-amber-500/40 hover:bg-amber-600/30'
              : 'bg-[#222] text-neutral-500 border-[#333] hover:text-neutral-400'}`}
          title='Clear manual beatgrid lock and retain current timing'
          aria-label='Clear manual beatgrid lock'
        >
          {saving ? '…' : locked ? 'LOCK' : offsetMs === 0 ? 'AUTO' : `${offsetMs >= 0 ? '+' : ''}${offsetMs}`}
        </button>
        <button disabled={disabled || locked} onClick={() => shift(0.001)} className={BTN} aria-label='Shift beat grid +1ms' title='Shift grid +1ms and save'>
          <ChevronRight size={18} aria-hidden />
        </button>
        <button disabled={disabled || locked} onClick={() => shift(0.01)} className={BTN} aria-label='Shift beat grid +10ms' title='Shift grid +10ms and save'>
          <ChevronsRight size={18} aria-hidden />
        </button>
      </div>
      <div className='flex items-center gap-1'>
        <button disabled={disabled || locked} onClick={() => rescale(2)} className={BTN} aria-label='Halve beatgrid BPM' title='Halve BPM and preserve first downbeat'>½×</button>
        <button disabled={disabled || locked} onClick={() => rescale(.5)} className={BTN} aria-label='Double beatgrid BPM' title='Double BPM and preserve first downbeat'>2×</button>
        <button disabled={disabled} onClick={toggleLock} className={BTN} aria-label={locked ? 'Unlock beatgrid' : 'Lock beatgrid'} title={locked ? 'Unlock beatgrid for editing' : 'Lock beatgrid against reanalysis'}>
          {locked ? <Lock size={16} aria-hidden /> : <Unlock size={16} aria-hidden />}
        </button>
        <button disabled={disabled} onClick={() => void clear()} className={BTN} aria-label='Reset beatgrid to auto analysis' title='Remove manual beatgrid and use next automatic analysis'><RotateCcw size={16} aria-hidden /></button>
      </div>
      {!locked && <button disabled={disabled} onClick={() => void persist(beats!, downbeats, true)} className='rounded border border-brand px-3 py-2 text-xs text-brand'>Verify &amp; lock grid</button>}
      <span className='text-[9px] text-neutral-600'>DB {downbeats.length ? downbeats[0] + 1 : '—'}</span>
      {error && <span className='max-w-32 text-center text-[9px] text-red-300'>{error}</span>}
    </div>
  );
};

export default React.memo(DJBeatGridEdit);


