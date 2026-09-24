import React, { useEffect, useState } from 'react';
import { api, type AnalysisCueApplyMode, type AnalysisCueList } from '../../../services/api';
import { useStore } from '../../../store';
import type { DeckId, HotCue } from '../../../slices/djMixerSlice';

interface DJAnalysisCueEditorProps {
  trackID?: string;
  deck: DeckId;
}

export function DJAnalysisCueEditor({ trackID, deck }: DJAnalysisCueEditorProps) {
  const hotCues = useStore(state => deck === 'A' ? state.djDeckA.hotCues : state.djDeckB.hotCues);
  const setHotCue = useStore(state => state.setHotCue);
  const loadHotCues = useStore(state => state.loadHotCues);
  const [cueList, setCueList] = useState<AnalysisCueList | null>(null);
  const [mode, setMode] = useState<AnalysisCueApplyMode>('fill-empty');
  const [status, setStatus] = useState('');

  useEffect(() => {
    let live = true;
    setCueList(null);
    setStatus('');
    if (trackID) api.getAnalysisCues(trackID).then(value => live && setCueList(value)).catch(() => live && setCueList(null));
    return () => { live = false; };
  }, [trackID]);

  const apply = async (selectedSlots?: number[]) => {
    if (!trackID) return;
    try {
      const result = await api.applyAnalysisCues(trackID, selectedSlots
        ? { mode: 'selected-only', selectedSlots }
        : { mode });
      setCueList(result);
      loadHotCues(deck, result.hotCues);
      setStatus(result.appliedSlots.length
        ? `Applied slots ${result.appliedSlots.join(', ')}${result.blockedSlots.length ? ` · kept occupied ${result.blockedSlots.join(', ')}` : ''}`
        : 'No candidates applied; occupied or suppressed slots were kept.');
    } catch {
      setStatus('Cue suggestions could not be applied.');
    }
  };

  const setLock = async (slot: number, locked: boolean) => {
    if (!trackID) return;
    const next = hotCues.map(cue => cue.slot === slot ? { ...cue, locked } : cue);
    try {
      await api.saveDJHotCues(trackID, next);
      loadHotCues(deck, next);
    } catch {
      setStatus('Cue lock could not be saved.');
    }
  };

  const moveToPlayhead = (cue: HotCue) => {
    const deckState = useStore.getState()[deck === 'A' ? 'djDeckA' : 'djDeckB'];
    setHotCue(deck, cue.slot, deckState.position, cue.label, cue.color);
  };

  if (!trackID || !cueList) return null;
  return <details className="mx-2 mb-1 rounded border border-neutral-800 bg-neutral-950/70 text-[10px] text-neutral-300">
    <summary className="cursor-pointer list-none px-2 py-1 text-neutral-300">
      Cue editor · {hotCues.filter(cue => cue.origin === 'analysis').length} auto · {hotCues.filter(cue => cue.origin !== 'analysis').length} manual
    </summary>
    <div className="space-y-2 border-t border-neutral-800 px-2 py-2">
      {hotCues.length > 0 && <div className="grid grid-cols-[2rem_1fr_auto_auto] items-center gap-1">
        {hotCues.map(cue => <React.Fragment key={cue.slot}>
          <span className="font-mono text-neutral-500">{cue.slot}</span>
          <label className="flex min-w-0 items-center gap-1">
            <input aria-label={`Cue ${cue.slot} label`} defaultValue={cue.label || cue.kind || `Cue ${cue.slot}`}
              onBlur={event => {
                const label = event.currentTarget.value.trim();
                if (label && label !== (cue.label || cue.kind || `Cue ${cue.slot}`)) setHotCue(deck, cue.slot, cue.position, label, cue.color);
              }} className="min-w-0 flex-1 rounded border border-neutral-800 bg-neutral-900 px-1 py-0.5" />
            <span title={cue.rationale || cue.kind || 'User cue'} className={cue.origin === 'analysis' ? 'text-cyan-300' : 'text-neutral-500'}>
              {cue.origin === 'analysis' ? 'AUTO' : 'USER'}{cue.confidence != null ? ` ${Math.round(cue.confidence * 100)}%` : ''}
            </span>
          </label>
          <button type="button" onClick={() => moveToPlayhead(cue)} className="rounded border border-neutral-700 px-1 py-0.5 hover:border-cyan-500">Move</button>
          <label className="flex items-center gap-1 text-neutral-400">
            <input type="checkbox" aria-label={`Lock cue ${cue.slot}`} checked={!!cue.locked} onChange={event => void setLock(cue.slot, event.target.checked)} /> Lock
          </label>
        </React.Fragment>)}
      </div>}
      <div className="flex flex-wrap items-center gap-2 border-t border-neutral-800 pt-2">
        <label className="flex items-center gap-1">
          Candidate policy
          <select aria-label="Generated cue apply policy" value={mode} onChange={event => setMode(event.target.value as AnalysisCueApplyMode)}
            className="rounded border border-neutral-700 bg-neutral-900 px-1 py-0.5">
            <option value="fill-empty">Fill empty slots</option>
            <option value="replace-generated">Refresh generated</option>
          </select>
        </label>
        <button type="button" onClick={() => void apply()} className="rounded border border-cyan-500/50 px-2 py-0.5 text-cyan-200 hover:bg-cyan-950">Apply policy</button>
        <span className="text-neutral-500">{cueList.generatorVersion} · source {cueList.sourceFingerprint.slice(0, 12)}</span>
        {status && <span role="status" className="text-neutral-400">{status}</span>}
      </div>
      <ul className="grid gap-1 border-t border-neutral-800 pt-2 sm:grid-cols-2">
        {cueList.generatedCandidates.map(candidate => <li key={candidate.slot} className="flex items-start justify-between gap-2 rounded bg-neutral-900/70 px-2 py-1">
          <span className="min-w-0">
            <strong className="text-neutral-200">{candidate.slot}. {candidate.label}</strong>
            <span className="ml-1 text-cyan-300">AUTO {Math.round(candidate.confidence * 100)}%</span>
            <span className="ml-1 text-neutral-500">{candidate.downbeatAligned ? 'measured/manual downbeat' : 'not downbeat-verified'}</span>
            <span className="block truncate text-neutral-500" title={candidate.rationale}>{candidate.rationale}</span>
          </span>
          <button type="button" onClick={() => void apply([candidate.slot])} className="shrink-0 rounded border border-neutral-700 px-1 py-0.5 hover:border-cyan-500">Apply</button>
        </li>)}
      </ul>
      {cueList.suppressions.length > 0 && <p className="text-neutral-500">Suppressed generated slots: {cueList.suppressions.map(item => `${item.slot} ${item.kind}`).join(', ')}</p>}
    </div>
  </details>;
}
