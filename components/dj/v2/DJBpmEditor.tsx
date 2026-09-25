import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { api, type TrackAnalysisFeature } from '../../../services/api';
import { djTrackSourceIdentity, formatManualBpm, tapTempoBpm } from '../../../lib/djBpmCorrection';
import type { Song } from '../../../types';
import type { DeckId } from '../../../slices/djMixerSlice';
import { useStore } from '../../../store';

interface DJBpmEditorProps {
  track: Song | null;
  deck: DeckId;
}

function emptyFeature(songId: string): TrackAnalysisFeature {
  return {
    songId,
    status: 'pending',
    bpmSource: 'unknown',
    keySource: 'unknown',
    syncAllowed: false,
  };
}

function parseBpm(value: string): number | undefined {
  if (!value.trim()) return undefined;
  const parsed = Number(value);
  return Number.isFinite(parsed) && parsed > 0 && parsed <= 1000 ? parsed : undefined;
}

export function DJBpmEditor({ track, deck }: DJBpmEditorProps) {
  const deckKey = deck === 'A' ? 'djDeckA' : 'djDeckB';
  const setDeckAnalysis = useStore(state => state.setDeckAnalysis);
  const sourceIdentity = useMemo(() => djTrackSourceIdentity(track), [track?.id, track?.source, track?.sourceName, track?.path, track?.fileHash, track?.fileHandle?.name]);
  const [feature, setFeature] = useState<TrackAnalysisFeature | null>(null);
  const [bpmInput, setBpmInput] = useState('');
  const [busy, setBusy] = useState(false);
  const [status, setStatus] = useState('');
  const actionGeneration = useRef(0);
  const tapTimes = useRef<number[]>([]);

  useEffect(() => {
    let current = true;
    actionGeneration.current += 1;
    tapTimes.current = [];
    setFeature(null);
    setBpmInput('');
    setBusy(false);
    setStatus('');
    if (track?.id) {
      api.getTrackBPM(track.id).then(value => {
        if (!current) return;
        setFeature(value);
        setBpmInput(value.bpm == null ? '' : formatManualBpm(value.bpm));
      }).catch(cause => {
        if (!current) return;
        setFeature(emptyFeature(track.id));
        setStatus(cause instanceof Error ? cause.message : 'Current BPM could not be loaded.');
      });
    }
    return () => {
      current = false;
      actionGeneration.current += 1;
    };
  }, [track?.id, sourceIdentity]);

  const isCurrentSourceLoaded = useCallback(() => {
    const current = useStore.getState()[deckKey];
    return djTrackSourceIdentity(current.track) === sourceIdentity;
  }, [deckKey, sourceIdentity]);

  const save = async () => {
    if (!track || busy) return;
    const bpm = parseBpm(bpmInput);
    if (bpm === undefined) {
      setStatus('Enter a finite BPM greater than 0 and at most 1000.');
      return;
    }
    if (!feature?.sourceFingerprint) {
      setStatus('The current source could not be verified. Reload the track before editing BPM.');
      return;
    }
    if (!isCurrentSourceLoaded()) {
      setStatus('BPM edit ignored because the deck source changed.');
      return;
    }
    const generation = ++actionGeneration.current;
    const identity = sourceIdentity;
    setBusy(true);
    setStatus('');
    try {
      const saved = await api.updateTrackBPM(track.id, bpm, feature.sourceFingerprint);
      if (generation !== actionGeneration.current || identity !== sourceIdentity || !isCurrentSourceLoaded()) return;
      setFeature(saved);
      setBpmInput(saved.bpm == null ? '' : formatManualBpm(saved.bpm));
      setDeckAnalysis(deck, { bpm: saved.bpm ?? null, bpmConfidence: saved.bpmConfidence ?? null });
      setStatus(`Saved ${formatManualBpm(bpm)} BPM for this source. Existing beat-grid timestamps were not moved.`);
    } catch (cause) {
      if (generation !== actionGeneration.current) return;
      setStatus(cause instanceof Error ? cause.message : 'BPM could not be saved.');
    } finally {
      if (generation === actionGeneration.current) setBusy(false);
    }
  };

  const reset = async () => {
    if (!track || busy || !feature?.sourceFingerprint) return;
    if (!isCurrentSourceLoaded()) {
      setStatus('BPM reset ignored because the deck source changed.');
      return;
    }
    const generation = ++actionGeneration.current;
    const identity = sourceIdentity;
    setBusy(true);
    setStatus('');
    try {
      const measured = await api.resetTrackBPM(track.id, feature.sourceFingerprint);
      if (generation !== actionGeneration.current || identity !== sourceIdentity || !isCurrentSourceLoaded()) return;
      setFeature(measured);
      setBpmInput(measured.bpm == null ? '' : formatManualBpm(measured.bpm));
      setDeckAnalysis(deck, { bpm: measured.bpm ?? null, bpmConfidence: measured.bpmConfidence ?? null });
      setStatus(measured.bpmSource === 'measured' ? `Restored measured ${formatManualBpm(measured.bpm!)} BPM.` : 'No measured BPM is available for this source.');
    } catch (cause) {
      if (generation !== actionGeneration.current) return;
      setStatus(cause instanceof Error ? cause.message : 'BPM could not be reset.');
    } finally {
      if (generation === actionGeneration.current) setBusy(false);
    }
  };

  const adjustBpm = (factor: number) => {
    const base = parseBpm(bpmInput) ?? feature?.bpm ?? undefined;
    if (!base) {
      setStatus('Enter a BPM or use Tap BPM first.');
      return;
    }
    const next = base * factor;
    if (!Number.isFinite(next) || next <= 0 || next > 1000) {
      setStatus('Adjusted BPM must be greater than 0 and at most 1000.');
      return;
    }
    setBpmInput(formatManualBpm(next));
    setStatus(`${formatManualBpm(next)} BPM ready to save. Existing beat-grid timestamps will not move.`);
  };

  const tap = () => {
    const now = performance.now();
    const previous = tapTimes.current;
    const next = previous.length && now - previous[previous.length - 1] > 2000 ? [now] : [...previous, now].slice(-4);
    tapTimes.current = next;
    const estimate = tapTempoBpm(next);
    if (estimate === undefined) {
      setStatus(`Tap BPM ${next.length}/4. Keep a steady beat; long gaps restart the count.`);
      return;
    }
    setBpmInput(formatManualBpm(estimate));
    setStatus(`Tap estimate ${formatManualBpm(estimate)} BPM ready to save. Existing beat-grid timestamps will not move.`);
  };

  if (!track) return null;

  return <details className="mx-2 mb-1 rounded border border-neutral-800 bg-neutral-950/70 text-[10px] text-neutral-300">
    <summary className="cursor-pointer list-none px-2 py-1">
      BPM correction · {feature?.bpm == null ? 'Unknown BPM' : `${formatManualBpm(feature.bpm)} · ${feature.bpmSource}`}
    </summary>
    <div className="space-y-2 border-t border-neutral-800 px-2 py-2">
      <p className="text-neutral-500">These controls change scalar BPM only; they do not move or re-align beat-grid timestamps. Use Edit Grid separately for timing corrections.</p>
      <div className="flex flex-wrap items-center gap-2">
        <label className="flex items-center gap-1">BPM
          <input aria-label={`Deck ${deck} manual BPM`} type="number" min="0.01" max="1000" step="0.01"
            value={bpmInput} disabled={busy} onChange={event => setBpmInput(event.target.value)}
            className="w-20 rounded border border-neutral-700 bg-neutral-900 px-1.5 py-1 font-mono text-neutral-100" />
        </label>
        <button type="button" disabled={busy} onClick={() => adjustBpm(2)} aria-label="Double BPM"
          className="rounded border border-neutral-700 px-2 py-1 hover:border-cyan-500 disabled:opacity-40">×2</button>
        <button type="button" disabled={busy} onClick={() => adjustBpm(0.5)} aria-label="Halve BPM"
          className="rounded border border-neutral-700 px-2 py-1 hover:border-cyan-500 disabled:opacity-40">÷2</button>
        <button type="button" disabled={busy} onClick={tap} aria-label="Tap BPM"
          className="rounded border border-neutral-700 px-2 py-1 hover:border-cyan-500 disabled:opacity-40">Tap BPM</button>
        <button type="button" disabled={busy || !feature?.sourceFingerprint} onClick={() => void save()}
          className="rounded border border-cyan-500/50 px-2 py-1 text-cyan-200 hover:bg-cyan-950 disabled:opacity-40">Save BPM</button>
        <button type="button" disabled={busy || !feature?.sourceFingerprint || feature.bpmSource !== 'manual'} onClick={() => void reset()}
          className="rounded border border-neutral-700 px-2 py-1 hover:border-cyan-500 disabled:cursor-not-allowed disabled:opacity-40">Reset to measured</button>
        {feature?.bpmSource === 'measured' && <span className="text-neutral-500">Measured {feature.bpm == null ? 'unknown' : `${formatManualBpm(feature.bpm)} BPM`}</span>}
        {status && <span role="status" className="text-neutral-400">{status}</span>}
      </div>
    </div>
  </details>;
}

export default React.memo(DJBpmEditor);
