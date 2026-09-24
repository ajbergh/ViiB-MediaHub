import React, { useEffect, useRef, useState } from 'react';
import { api, type AnalysisCueApplyMode, type AnalysisCueList } from '../../../services/api';
import { useStore } from '../../../store';
import type { DeckId, HotCue } from '../../../slices/djMixerSlice';
import { quantizeHotCuePosition, type HotCueQuantizeMode } from '../../../lib/hotCueQuantization';
import { convertHotCueToManual, moveHotCueToPosition, recolorHotCue, renameHotCue } from '../../../lib/hotCueEditor';
import { useDJAudioEngineActions } from '../../../hooks/useDJAudioEngine';

interface DJAnalysisCueEditorProps {
  trackID?: string;
  deck: DeckId;
}

export function DJAnalysisCueEditor({ trackID, deck }: DJAnalysisCueEditorProps) {
  const beatGrid = useStore(state => deck === 'A' ? state.djDeckA.beatGrid : state.djDeckB.beatGrid);
  const hotCues = useStore(state => deck === 'A' ? state.djDeckA.hotCues : state.djDeckB.hotCues);
  const loadHotCues = useStore(state => state.loadHotCues);
  const { seek } = useDJAudioEngineActions();
  const [cueList, setCueList] = useState<AnalysisCueList | null>(null);
  const [mode, setMode] = useState<AnalysisCueApplyMode>('fill-empty');
  const [quantizeMode, setQuantizeMode] = useState<HotCueQuantizeMode>('off');
  const [status, setStatus] = useState('');
  const [isApplying, setIsApplying] = useState(false);
  const saveQueueRef = useRef<Promise<void>>(Promise.resolve());
  const editRevisionRef = useRef(0);
  const deckKey = deck === 'A' ? 'djDeckA' : 'djDeckB';

  const updateCue = (slot: number, update: (cue: HotCue) => HotCue, allowLocked = false) => {
    if (!trackID || isApplying) return;
    const currentDeck = useStore.getState()[deckKey];
    if (currentDeck.track?.id !== trackID) {
      setStatus('Cue edit ignored because the deck loaded a different track.');
      return;
    }
    const currentCue = currentDeck.hotCues.find(cue => cue.slot === slot);
    if (!currentCue || (currentCue.locked && !allowLocked)) return;
    const nextCue = update(currentCue);
    if (nextCue === currentCue) return;
    const next = currentDeck.hotCues.map(cue => cue.slot === slot ? nextCue : cue);
    const revision = ++editRevisionRef.current;
    loadHotCues(deck, next);
    const save = saveQueueRef.current.catch(() => undefined).then(() => api.saveDJHotCues(trackID, next).then(() => undefined));
    saveQueueRef.current = save;
    void save.then(() => {
      const stillOnTrack = useStore.getState()[deckKey].track?.id === trackID;
      if (stillOnTrack && revision === editRevisionRef.current) setStatus('');
    }, () => {
      const stillOnTrack = useStore.getState()[deckKey].track?.id === trackID;
      if (stillOnTrack && revision === editRevisionRef.current) setStatus('Cue edit could not be saved.');
    });
  };

  const jumpToCue = (slot: number) => {
    if (!trackID) return;
    const currentDeck = useStore.getState()[deckKey];
    if (currentDeck.track?.id !== trackID) {
      setStatus('Cue jump ignored because the deck loaded a different track.');
      return;
    }
    const cue = currentDeck.hotCues.find(item => item.slot === slot);
    if (cue) seek(deck, cue.position);
  };

  useEffect(() => {
    let live = true;
    setCueList(null);
    setStatus('');
    if (trackID) api.getAnalysisCues(trackID).then(value => live && setCueList(value)).catch(() => live && setCueList(null));
    return () => { live = false; };
  }, [trackID]);

  const apply = async (selectedSlots?: number[]) => {
    if (!trackID || isApplying) return;
    setIsApplying(true);
    try {
      await saveQueueRef.current;
      if (useStore.getState()[deckKey].track?.id !== trackID) return;
      const revision = editRevisionRef.current;
      const result = await api.applyAnalysisCues(trackID, selectedSlots
        ? { mode: 'selected-only', selectedSlots }
        : { mode });
      if (useStore.getState()[deckKey].track?.id !== trackID || revision !== editRevisionRef.current) {
        setStatus('Cue results changed while refreshing and were not loaded into this deck.');
        return;
      }
      setCueList(result);
      loadHotCues(deck, result.hotCues);
      setStatus(result.appliedSlots.length
        ? `Applied slots ${result.appliedSlots.join(', ')}${result.blockedSlots.length ? ` · kept occupied ${result.blockedSlots.join(', ')}` : ''}`
        : 'No candidates applied; occupied or suppressed slots were kept.');
    } catch {
      setStatus('Cue suggestions could not be applied.');
    } finally {
      setIsApplying(false);
    }
  };

  const setLock = (slot: number, locked: boolean) => {
    updateCue(slot, cue => ({ ...cue, locked }), true);
  };

  const moveToPlayhead = (cue: HotCue) => {
    if (cue.locked) return;
    const playhead = useStore.getState()[deckKey].position;
    const position = quantizeHotCuePosition({ position: playhead }, beatGrid, quantizeMode) ?? playhead;
    updateCue(cue.slot, current => moveHotCueToPosition(current, position));
  };

  const snapCue = (cue: HotCue) => {
    updateCue(cue.slot, current => {
      const position = quantizeHotCuePosition(current, beatGrid, quantizeMode);
      return position === null ? current : moveHotCueToPosition(current, position);
    });
  };

  if (!trackID || !cueList) return null;
  return <details className="mx-2 mb-1 rounded border border-neutral-800 bg-neutral-950/70 text-[10px] text-neutral-300">
    <summary className="cursor-pointer list-none px-2 py-1 text-neutral-300">
      Cue editor · {hotCues.filter(cue => cue.origin === 'analysis').length} auto · {hotCues.filter(cue => cue.origin !== 'analysis').length} manual
    </summary>
    <div className="space-y-2 border-t border-neutral-800 px-2 py-2">
      {hotCues.length > 0 && <div className="grid grid-cols-[2rem_minmax(6rem,1fr)_auto_auto_auto_auto_auto_auto] items-center gap-1">
        {hotCues.map(cue => <React.Fragment key={cue.slot}>
          <span className="font-mono text-neutral-500">{cue.slot}</span>
          <label className="flex min-w-0 items-center gap-1">
            <input aria-label={`Cue ${cue.slot} label`} defaultValue={cue.label || cue.kind || `Cue ${cue.slot}`}
              disabled={!!cue.locked || isApplying}
              onBlur={event => {
                const label = event.currentTarget.value.trim();
                if (label && label !== (cue.label || cue.kind || `Cue ${cue.slot}`)) updateCue(cue.slot, current => renameHotCue(current, label));
              }} className="min-w-0 flex-1 rounded border border-neutral-800 bg-neutral-900 px-1 py-0.5" />
            <span title={cue.rationale || cue.kind || 'User cue'} className={cue.origin === 'analysis' ? 'text-cyan-300' : 'text-neutral-500'}>
              {cue.origin === 'analysis' ? 'AUTO' : 'USER'}{cue.confidence != null ? ` ${Math.round(cue.confidence * 100)}%` : ''}
            </span>
          </label>
          <button type="button" disabled={isApplying} onClick={() => jumpToCue(cue.slot)} title="Seek this deck to the cue position"
            className="rounded border border-cyan-500/40 px-1 py-0.5 hover:border-cyan-500 disabled:cursor-not-allowed disabled:opacity-40">Jump</button>
          <button type="button" disabled={!!cue.locked || isApplying} onClick={() => moveToPlayhead(cue)} className="rounded border border-neutral-700 px-1 py-0.5 hover:border-cyan-500 disabled:cursor-not-allowed disabled:opacity-40">Move</button>
          <button type="button" disabled={!!cue.locked || isApplying || quantizeMode === 'off' || !beatGrid?.length}
            title="Snap to the nearest beat subdivision in the stored beat grid; this does not correct downbeats."
            onClick={() => snapCue(cue)} className="rounded border border-neutral-700 px-1 py-0.5 hover:border-cyan-500 disabled:cursor-not-allowed disabled:opacity-40">Snap</button>
          <label className="flex items-center gap-1 text-neutral-400">
            <input type="checkbox" aria-label={`Lock cue ${cue.slot}`} checked={!!cue.locked} disabled={isApplying} onChange={event => setLock(cue.slot, event.target.checked)} /> Lock
          </label>
          <label className="flex items-center gap-1" title="Change cue color while preserving its provenance">
            <span className="sr-only">Recolor cue {cue.slot}</span>
            <input type="color" aria-label={`Recolor cue ${cue.slot}`} value={/^#[\da-f]{6}$/i.test(cue.color) ? cue.color : '#FF5500'}
              disabled={!!cue.locked || isApplying} onChange={event => updateCue(cue.slot, current => recolorHotCue(current, event.target.value))}
              className="h-5 w-6 cursor-pointer rounded border border-neutral-700 bg-neutral-900 disabled:cursor-not-allowed disabled:opacity-40" />
          </label>
          {cue.origin === 'analysis' ? <button type="button" aria-label={`Convert cue ${cue.slot} to manual`} disabled={!!cue.locked || isApplying}
            onClick={() => updateCue(cue.slot, convertHotCueToManual)} title="Keep this cue but remove generated-only metadata"
            className="rounded border border-amber-500/40 px-1 py-0.5 text-amber-200 hover:border-amber-400 disabled:cursor-not-allowed disabled:opacity-40">To Manual</button> : <span />}
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
        <label className="flex items-center gap-1" title="Nearest stored beat-grid subdivision; this does not correct downbeats.">
          Quantize
          <select aria-label="Cue quantize mode" value={quantizeMode} onChange={event => setQuantizeMode(event.target.value as HotCueQuantizeMode)}
            className="rounded border border-neutral-700 bg-neutral-900 px-1 py-0.5">
            <option value="off">Off</option>
            <option value="beat">Whole beat</option>
            <option value="half">Half beat</option>
            <option value="quarter">Quarter beat</option>
          </select>
        </label>
        <button type="button" disabled={isApplying} onClick={() => void apply()} className="rounded border border-cyan-500/50 px-2 py-0.5 text-cyan-200 hover:bg-cyan-950 disabled:opacity-40">Apply policy</button>
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
          <button type="button" disabled={isApplying} onClick={() => void apply([candidate.slot])} className="shrink-0 rounded border border-neutral-700 px-1 py-0.5 hover:border-cyan-500 disabled:opacity-40">Apply</button>
        </li>)}
      </ul>
      {cueList.suppressions.length > 0 && <p className="text-neutral-500">Suppressed generated slots: {cueList.suppressions.map(item => `${item.slot} ${item.kind}`).join(', ')}</p>}
    </div>
  </details>;
}
