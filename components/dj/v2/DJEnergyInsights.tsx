import React, { useEffect, useRef, useState } from 'react';
import { useStore } from '../../../store';
import { getDJAudioEngine } from '../../../lib/djAudio';
import { api, type TrackEnergyFeatures, type TrackTransitionRecommendations, type TransitionIntent } from '../../../services/api';

interface DJEnergyInsightsProps {
  trackID?: string;
  deck?: 'A' | 'B';
}

// This deliberately displays suggestions as opt-in actions. It never writes
// analysis output into a DJ's hot cues until the DJ accepts a specific cue.
export function DJEnergyInsights({ trackID, deck }: DJEnergyInsightsProps) {
  const [features, setFeatures] = useState<TrackEnergyFeatures | null>(null);
  const [recommendations, setRecommendations] = useState<TrackTransitionRecommendations | null>(null);
  const [intent, setIntent] = useState<TransitionIntent>('hold');
  const hotCues = useStore(state => deck === 'A' ? state.djDeckA.hotCues : state.djDeckB.hotCues);
  const analysisStatus = useStore(state => deck === 'A' ? state.djDeckA.analysisStatus : state.djDeckB.analysisStatus);
  const setHotCue = useStore(state => state.setHotCue);
  const progressRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!features || !deck) return;
    const updatePosition = () => {
      const progress = progressRef.current;
      if (!progress) return;
      const state = useStore.getState();
      const currentDeck = deck === 'A' ? state.djDeckA : state.djDeckB;
      const engine = getDJAudioEngine();
      const valid = currentDeck.track?.id === trackID && currentDeck.duration > 0;
      progress.hidden = !valid;
      if (!valid) return;
      const position = engine.initialized && (currentDeck.isPlaying || engine.isScratching(deck))
        ? engine.getPosition(deck) : currentDeck.position;
      const percent = Math.max(0, Math.min(100, position / currentDeck.duration * 100));
      progress.style.width = `${percent}%`;
      progress.setAttribute('aria-valuenow', String(Math.round(percent)));
      progress.setAttribute('aria-valuetext', `${Math.floor(position)} of ${Math.floor(currentDeck.duration)} seconds`);
    };
    updatePosition();
    const timer = window.setInterval(updatePosition, 50);
    return () => window.clearInterval(timer);
  }, [features, deck, trackID]);

  useEffect(() => {
    let live = true;
    setFeatures(null);
    setRecommendations(null);
    if (trackID && analysisStatus === 'available') {
      api.getTrackEnergyFeatures(trackID).then(value => live && setFeatures(value)).catch(() => {});
    }
    return () => { live = false; };
  }, [trackID, analysisStatus]);

  useEffect(() => {
    let live = true;
    setRecommendations(null);
    if (trackID && analysisStatus === 'available') {
      api.getTrackTransitionRecommendations(trackID, 3, intent).then(value => live && setRecommendations(value)).catch(() => {});
    }
    return () => { live = false; };
  }, [trackID, analysisStatus, intent]);

  if (analysisStatus === 'not_analyzed' || analysisStatus === 'error') return <div className="px-2 py-1 text-[10px] text-amber-400">{analysisStatus === 'not_analyzed' ? 'Track not analysed yet.' : 'Track analysis is unavailable.'} Energy insights and recommendations are unavailable.</div>;
  if (!features) return null;
  const acceptCue = (position: number, kind: string) => {
    if (!deck) return;
    const slot = Array.from({ length: 8 }, (_, index) => index + 1).find(candidate => !hotCues.some(cue => cue.slot === candidate));
    if (!slot) return;
    setHotCue(deck, slot, position, `Suggested ${kind}`);
  };
  const top = recommendations?.recommendations[0];
  return <section aria-label="Measured track energy" className="px-2 py-1 text-[10px] text-neutral-400">
    <div className="relative flex h-5 items-end gap-px overflow-hidden" title="Track energy · Highlight shows playback position">
      {features.energy.map((point, index) => <i key={index} className="w-1 bg-cyan-400/70" style={{ height: `${Math.max(2, point.value * 100)}%` }} />)}
      {deck && <div ref={progressRef} hidden role="progressbar" aria-label={`Deck ${deck} track position`}
        aria-valuemin={0} aria-valuemax={100} aria-valuenow={0}
        className="pointer-events-none absolute inset-y-0 left-0 bg-white/15" style={{ width: '0%' }}>
        <span className="absolute inset-y-0 right-0 w-2 translate-x-1/2 bg-white/20" />
        <span className="absolute inset-y-0 right-0 w-0.5 bg-white" />
        <span className="absolute right-0 top-0 h-1.5 w-1.5 translate-x-1/2 rotate-45 bg-white" />
      </div>}
    </div>
    <div className="mt-1 flex flex-wrap items-center gap-2">
      <span title="Unweighted RMS-based estimate; not BS.1770 LUFS">Loudness proxy: {features.integratedLufs.toFixed(1)} dB</span>
      <span>{features.cueSuggestions.length} advisory cues</span>
      <label className="inline-flex items-center gap-1">
        <span>Mix Next</span>
        <select aria-label="Mix Next direction" value={intent} onChange={event => setIntent(event.target.value as TransitionIntent)}
          className="rounded border border-violet-500/30 bg-neutral-950 px-1 text-violet-200">
          <option value="hold">Hold</option>
          <option value="lift">Lift (+1 energy)</option>
          <option value="reset">Reset (-1 energy)</option>
          <option value="harmonic">Harmonic</option>
        </select>
      </label>
      {features.cueSuggestions.slice(0, 3).map((cue, index) => {
        const accepted = hotCues.some(hotCue => Math.abs(hotCue.position - cue.position) < .01);
        return <button key={`${cue.kind}-${index}`} disabled={!deck || accepted} onClick={() => acceptCue(cue.position, cue.kind)} title={cue.rationale}
          className="rounded border border-cyan-500/30 px-1 text-cyan-300 disabled:border-neutral-700 disabled:text-neutral-600">
          {accepted ? `${cue.kind} added` : `Add ${cue.kind}`}
        </button>;
      })}
    </div>
    {top && <details className="mt-1 text-neutral-500">
      <summary className="cursor-pointer text-violet-300">Recommended next: {top.title} — {top.artist} ({Math.round(top.score * 100)}%)</summary>
      <ul className="mt-1 space-y-0.5 pl-3">
        {top.components.map(component => <li key={component.name} title={component.rationale}>
          {component.name}: {Math.round(component.score * 100)}% — {component.rationale}
        </li>)}
      </ul>
    </details>}
  </section>;
}
