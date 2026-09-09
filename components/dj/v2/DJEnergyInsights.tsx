import React, { useEffect, useState } from 'react';
import { useStore } from '../../../store';
import { api, type TrackEnergyFeatures, type TrackTransitionRecommendations } from '../../../services/api';

interface DJEnergyInsightsProps {
  trackID?: string;
  deck?: 'A' | 'B';
}

// This deliberately displays suggestions as opt-in actions. It never writes
// analysis output into a DJ's hot cues until the DJ accepts a specific cue.
export function DJEnergyInsights({ trackID, deck }: DJEnergyInsightsProps) {
  const [features, setFeatures] = useState<TrackEnergyFeatures | null>(null);
  const [recommendations, setRecommendations] = useState<TrackTransitionRecommendations | null>(null);
  const hotCues = useStore(state => deck === 'A' ? state.djDeckA.hotCues : state.djDeckB.hotCues);
  const setHotCue = useStore(state => state.setHotCue);

  useEffect(() => {
    let live = true;
    setFeatures(null);
    setRecommendations(null);
    if (trackID) {
      api.getTrackEnergyFeatures(trackID).then(value => live && setFeatures(value)).catch(() => {});
      api.getTrackTransitionRecommendations(trackID).then(value => live && setRecommendations(value)).catch(() => {});
    }
    return () => { live = false; };
  }, [trackID]);

  if (!features) return null;
  const acceptCue = (position: number, kind: string) => {
    if (!deck) return;
    const slot = Array.from({ length: 8 }, (_, index) => index + 1).find(candidate => !hotCues.some(cue => cue.slot === candidate));
    if (!slot) return;
    setHotCue(deck, slot, position, `Suggested ${kind}`, '#22d3ee');
  };
  const top = recommendations?.recommendations[0];
  return <section aria-label="Measured track energy" className="px-2 py-1 text-[10px] text-neutral-400">
    <div className="flex h-5 items-end gap-px" title="Measured 500 ms energy windows">
      {features.energy.map((point, index) => <i key={index} className="w-1 bg-cyan-400/70" style={{ height: `${Math.max(2, point.value * 100)}%` }} />)}
    </div>
    <div className="mt-1 flex flex-wrap items-center gap-2">
      <span>{features.integratedLufs.toFixed(1)} LUFS</span>
      <span>{features.cueSuggestions.length} advisory cues</span>
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
