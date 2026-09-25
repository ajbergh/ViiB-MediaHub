import React, { useEffect, useRef, useState } from 'react';
import { api, type TrackAnalysisFeature } from '../../../services/api';
import { keyScalePitchClasses, KEY_NOTE_NAMES, startKeyReferenceTone, type VerifiedKeyMode } from '../../../lib/keyVerificationKeyboard';
import type { DeckId } from '../../../slices/djMixerSlice';
import { useStore } from '../../../store';

interface DJKeyVerificationKeyboardProps {
  trackID?: string;
  deck: DeckId;
}

export function DJKeyVerificationKeyboard({ trackID, deck }: DJKeyVerificationKeyboardProps) {
  const setDeckAnalysis = useStore(state => state.setDeckAnalysis);
  const [feature, setFeature] = useState<TrackAnalysisFeature | null>(null);
  const [selectedTonic, setSelectedTonic] = useState(0);
  const [selectedMode, setSelectedMode] = useState<VerifiedKeyMode>('major');
  const [busy, setBusy] = useState(false);
  const [status, setStatus] = useState('');
  const contextRef = useRef<AudioContext | null>(null);
  const stopToneRef = useRef<(() => void) | null>(null);
  const playbackGenerationRef = useRef(0);
  const actionGenerationRef = useRef(0);
  const deckKey = deck === 'A' ? 'djDeckA' : 'djDeckB';

  useEffect(() => {
    let current = true;
    playbackGenerationRef.current += 1;
    actionGenerationRef.current += 1;
    stopToneRef.current?.();
    stopToneRef.current = null;
    setFeature(null);
    setStatus('');
    setBusy(false);
    if (trackID) {
      api.getTrackAnalysisFeature(trackID).then(value => {
        if (!current) return;
        setFeature(value);
        setSelectedTonic(value.keyTonic ?? 0);
        setSelectedMode(value.keyMode ?? 'major');
      }).catch(() => current && setFeature(null));
    }
    return () => {
      current = false;
      playbackGenerationRef.current += 1;
      actionGenerationRef.current += 1;
      stopToneRef.current?.();
      stopToneRef.current = null;
    };
  }, [trackID]);

  useEffect(() => () => {
    stopToneRef.current?.();
    stopToneRef.current = null;
    if (contextRef.current) void contextRef.current.close();
    contextRef.current = null;
  }, []);

  const play = async (pitchClass: number) => {
    const generation = ++playbackGenerationRef.current;
    try {
      const AudioContextCtor = window.AudioContext || (window as typeof window & { webkitAudioContext?: typeof AudioContext }).webkitAudioContext;
      if (!AudioContextCtor) {
        setStatus('Reference note playback is not supported in this browser.');
        return;
      }
      const context = contextRef.current ?? new AudioContextCtor();
      contextRef.current = context;
      if (context.state === 'suspended') await context.resume();
      if (generation !== playbackGenerationRef.current || useStore.getState()[deckKey].track?.id !== trackID) return;
      stopToneRef.current?.();
      stopToneRef.current = startKeyReferenceTone(context, pitchClass);
      setStatus(`Playing ${KEY_NOTE_NAMES[pitchClass]} reference note.`);
    } catch {
      if (generation === playbackGenerationRef.current && useStore.getState()[deckKey].track?.id === trackID) {
        setStatus('Reference note could not be played.');
      }
    }
  };

  const updateDeckKey = (value: TrackAnalysisFeature) => {
    const current = useStore.getState()[deckKey];
    if (current.track?.id !== trackID) return false;
    setDeckAnalysis(deck, { key: value.key ?? null });
    return true;
  };

  const save = async () => {
    if (!trackID || busy) return;
    if (useStore.getState()[deckKey].track?.id !== trackID) {
      setStatus('Key edit ignored because the deck loaded a different track.');
      return;
    }
    const generation = ++actionGenerationRef.current;
    setBusy(true);
    setStatus('');
    try {
      const saved = await api.updateTrackKey(trackID, { tonic: selectedTonic, mode: selectedMode });
      if (generation !== actionGenerationRef.current || useStore.getState()[deckKey].track?.id !== trackID) return;
      setFeature(saved);
      if (updateDeckKey(saved)) setStatus(`Saved ${saved.key ?? 'manual key'} for this track.`);
      else setStatus('Key saved, but the deck changed tracks before the display updated.');
    } catch (cause) {
      if (generation !== actionGenerationRef.current) return;
      setStatus(cause instanceof Error ? cause.message : 'Key could not be saved.');
    } finally {
      if (generation === actionGenerationRef.current) setBusy(false);
    }
  };

  const reset = async () => {
    if (!trackID || busy) return;
    if (useStore.getState()[deckKey].track?.id !== trackID) {
      setStatus('Key reset ignored because the deck loaded a different track.');
      return;
    }
    const generation = ++actionGenerationRef.current;
    setBusy(true);
    setStatus('');
    try {
      const measured = await api.resetTrackKey(trackID);
      if (generation !== actionGenerationRef.current || useStore.getState()[deckKey].track?.id !== trackID) return;
      setFeature(measured);
      setSelectedTonic(measured.keyTonic ?? 0);
      setSelectedMode(measured.keyMode ?? 'major');
      if (updateDeckKey(measured)) setStatus(measured.keySource === 'measured' ? 'Restored the measured key.' : 'No measured key is available yet.');
      else setStatus('Key reset, but the deck changed tracks before the display updated.');
    } catch (cause) {
      if (generation !== actionGenerationRef.current) return;
      setStatus(cause instanceof Error ? cause.message : 'Key could not be reset.');
    } finally {
      if (generation === actionGenerationRef.current) setBusy(false);
    }
  };

  if (!trackID || !feature) return null;
  const detectedScale = feature.measuredKeyTonic == null || feature.measuredKeyMode == null
    ? []
    : keyScalePitchClasses(feature.measuredKeyTonic, feature.measuredKeyMode);

  return <details className="mx-2 mb-1 rounded border border-neutral-800 bg-neutral-950/70 text-[10px] text-neutral-300">
    <summary className="cursor-pointer list-none px-2 py-1">
      Key verifier · {feature.key ?? 'No measured key'} · {feature.keySource}
    </summary>
    <div className="space-y-2 border-t border-neutral-800 px-2 py-2">
      <p className="text-neutral-500">Select a tonic and mode, then audition the notes on your output. This reference tone does not use deck playback.</p>
      <div className="grid grid-cols-6 gap-1 sm:grid-cols-12">
        {KEY_NOTE_NAMES.map((name, pitchClass) => {
          const isTonic = feature.measuredKeyTonic === pitchClass;
          const inScale = detectedScale.includes(pitchClass);
          const selected = selectedTonic === pitchClass;
          return <button key={name} type="button" aria-label={`Audition ${name}`} aria-pressed={selected}
            title={`${name}${isTonic ? ' · measured tonic' : inScale ? ' · in measured scale' : ''}`}
            onClick={() => { setSelectedTonic(pitchClass); void play(pitchClass); }}
            className={`rounded border px-1 py-2 font-mono ${selected ? 'border-cyan-400 text-cyan-100' : 'border-neutral-700'} ${isTonic ? 'bg-amber-950/70 text-amber-200' : inScale ? 'bg-neutral-800 text-neutral-200' : 'bg-neutral-900 text-neutral-400'} disabled:opacity-40`}>
            {name}
          </button>;
        })}
      </div>
      <div className="flex flex-wrap items-center gap-2">
        <label className="flex items-center gap-1">Mode
          <select aria-label="Selected key mode" value={selectedMode} disabled={busy}
            onChange={event => setSelectedMode(event.target.value as VerifiedKeyMode)}
            className="rounded border border-neutral-700 bg-neutral-900 px-1 py-0.5">
            <option value="major">Major</option>
            <option value="minor">Minor</option>
          </select>
        </label>
        <button type="button" disabled={busy} onClick={() => void save()}
          className="rounded border border-cyan-500/50 px-2 py-0.5 text-cyan-200 hover:bg-cyan-950 disabled:opacity-40">Use as key</button>
        <button type="button" disabled={busy || feature.keySource !== 'manual'} onClick={() => void reset()}
          className="rounded border border-neutral-700 px-2 py-0.5 hover:border-cyan-500 disabled:cursor-not-allowed disabled:opacity-40">Reset to measured</button>
        {feature.keyConfidence != null && <span className="text-neutral-500">Measured confidence {Math.round(feature.keyConfidence * 100)}%</span>}
        {status && <span role="status" className="text-neutral-400">{status}</span>}
      </div>
    </div>
  </details>;
}

export default React.memo(DJKeyVerificationKeyboard);
