/** Durable beat-grid correction controls. Analysis itself happens in the library job. */
import React, { useCallback, useMemo, useState } from 'react';
import { ChevronsLeft, ChevronLeft, ChevronRight, ChevronsRight, Lock, RotateCcw, Unlock } from 'lucide-react';
import type { DeckId } from '../../../slices/djMixerSlice';
import { useStore } from '../../../store';
import { api } from '../../../services/api';

interface DJBeatGridEditProps { deck: DeckId; }

const BTN = 'w-11 h-11 rounded bg-[#222] text-neutral-400 hover:text-white border border-[#333] hover:border-neutral-500 transition-colors flex items-center justify-center flex-shrink-0 disabled:cursor-not-allowed disabled:opacity-40';

export const DJBeatGridEdit: React.FC<DJBeatGridEditProps> = ({ deck }) => {
  const deckState = useStore(state => deck === 'A' ? state.djDeckA : state.djDeckB);
  const setDeckAnalysis = useStore(state => state.setDeckAnalysis);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const beats = deckState.beatGrid;
  const downbeats = deckState.downbeatIndices ?? [];
  const locked = deckState.beatGridLocked;

  const isDynamic = useMemo(() => {
    if (!beats || beats.length < 3) return false;
    const first = beats[1] - beats[0];
    return beats.slice(2).some((beat, index) => Math.abs(beat - beats[index + 1] - first) > .003);
  }, [beats]);

  const persist = useCallback(async (nextBeats: number[], nextDownbeats: number[], nextLocked: boolean, bpm?: number) => {
    if (!deckState.track || nextBeats.length < 2) return;
    setSaving(true); setError(null);
    try {
      const saved = await api.updateTrackBeatGrid(deckState.track.id, {
        beats: nextBeats, downbeatIndices: nextDownbeats, locked: nextLocked,
        ...(isDynamic ? {} : { bpm: bpm ?? deckState.originalBpm ?? undefined }),
      });
      const current = deck === 'A' ? useStore.getState().djDeckA : useStore.getState().djDeckB;
      if (current.track?.id !== deckState.track.id) return;
      setDeckAnalysis(deck, {
        beatGridSource: 'manual', beatGrid: saved.beats, downbeatIndices: saved.downbeatIndices,
        beatGridLocked: saved.locked, ...(bpm === undefined ? {} : { bpm }),
      });
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Could not save grid');
    } finally { setSaving(false); }
  }, [deck, deckState.originalBpm, deckState.track, isDynamic, setDeckAnalysis]);

  const shift = useCallback((delta: number) => {
    if (!beats || locked || saving) return;
    void persist(beats.map(beat => beat + Math.max(delta, -beats[0])), downbeats, true);
  }, [beats, downbeats, locked, persist, saving]);
  const rescale = useCallback((factor: number) => {
    if (!beats || locked || saving) return;
    const first = beats[0];
    void persist(beats.map(beat => first + (beat - first) * factor), downbeats, true,
      deckState.originalBpm ? deckState.originalBpm / factor : undefined);
  }, [beats, deckState.originalBpm, downbeats, locked, persist, saving]);
  const reset = useCallback(async () => {
    if (!deckState.track || saving) return;
    setSaving(true); setError(null);
    try {
      await api.resetTrackBeatGrid(deckState.track.id);
      const current = deck === 'A' ? useStore.getState().djDeckA : useStore.getState().djDeckB;
      if (current.track?.id === deckState.track.id) {
        setDeckAnalysis(deck, { beatGridSource: 'unknown', beatGrid: null, downbeatIndices: null, beatGridLocked: false });
      }
    } catch (cause) { setError(cause instanceof Error ? cause.message : 'Could not reset grid'); }
    finally { setSaving(false); }
  }, [deck, deckState.track, saving, setDeckAnalysis]);

  if (!beats?.length) return null;
  const disabled = saving;
  return <div className='flex flex-col items-center gap-2 px-1 py-0.5' aria-live='polite'>
    <span className='text-[10px] text-text-secondary'>{locked ? 'Reviewed grid' : 'Check intro, middle, and outro before locking.'}</span>
    <span className='text-[10px] text-neutral-500 font-bold uppercase tracking-wider'>GRID {isDynamic ? 'DYNAMIC' : 'STRAIGHT'}</span>
    <div className='flex items-center gap-1'>
      <button disabled={disabled || locked} onClick={() => shift(-.01)} className={BTN} aria-label='Shift beat grid -10ms'><ChevronsLeft size={18} /></button>
      <button disabled={disabled || locked} onClick={() => shift(-.001)} className={BTN} aria-label='Shift beat grid -1ms'><ChevronLeft size={18} /></button>
      <button disabled={disabled || locked} onClick={() => shift(.001)} className={BTN} aria-label='Shift beat grid +1ms'><ChevronRight size={18} /></button>
      <button disabled={disabled || locked} onClick={() => shift(.01)} className={BTN} aria-label='Shift beat grid +10ms'><ChevronsRight size={18} /></button>
    </div>
    <div className='flex items-center gap-1'>
      <button disabled={disabled || locked} onClick={() => rescale(2)} className={BTN} aria-label='Halve beatgrid BPM'>½×</button>
      <button disabled={disabled || locked} onClick={() => rescale(.5)} className={BTN} aria-label='Double beatgrid BPM'>2×</button>
      <button disabled={disabled} onClick={() => void persist(beats, downbeats, !locked)} className={BTN} aria-label={locked ? 'Unlock beatgrid' : 'Lock beatgrid'}>{locked ? <Lock size={16} /> : <Unlock size={16} />}</button>
      <button disabled={disabled} onClick={() => void reset()} className={BTN} aria-label='Reset beatgrid to library analysis'><RotateCcw size={16} /></button>
    </div>
    {!locked && <button disabled={disabled} onClick={() => void persist(beats, downbeats, true)} className='rounded border border-brand px-3 py-1.5 text-xs text-brand'>Verify & lock grid</button>}
    {error && <span className='max-w-40 text-center text-[9px] text-red-300'>{error}</span>}
  </div>;
};

export default React.memo(DJBeatGridEdit);
