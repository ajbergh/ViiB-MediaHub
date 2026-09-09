import React, { useEffect, useState } from 'react';
import { api, type TrackEnergyFeatures } from '../../../services/api';

export function DJEnergyInsights({ trackID }: { trackID?: string }) {
  const [features, setFeatures] = useState<TrackEnergyFeatures | null>(null);
  useEffect(() => {
    let live = true;
    setFeatures(null);
    if (trackID) api.getTrackEnergyFeatures(trackID).then(value => live && setFeatures(value)).catch(() => {});
    return () => { live = false; };
  }, [trackID]);
  if (!features) return null;
  return <section aria-label="Measured track energy" className="px-2 py-1 text-[10px] text-neutral-400">
    <div className="flex h-5 items-end gap-px">{features.energy.map((point, index) => <i key={index} className="w-1 bg-cyan-400/70" style={{ height: `${Math.max(2, point.value * 100)}%` }} />)}</div>
    <div className="mt-1 flex gap-2"><span>{features.integratedLufs.toFixed(1)} LUFS</span><span>{features.cueSuggestions.length} cues</span></div>
  </section>;
}
