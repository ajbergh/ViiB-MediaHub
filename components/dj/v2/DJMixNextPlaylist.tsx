import { useState } from 'react';
import { DJSaveResultsPlaylist } from './DJSaveResultsPlaylist';

export function DJMixNextPlaylist({ referenceId, referenceTitle, candidates }: {
 referenceId: string; referenceTitle: string;
 candidates: Array<{ songId: string; title: string; artist: string; score: number }>;
}) {
 const [excluded,setExcluded] = useState<Set<string>>(() => new Set());
 const seen = new Set([referenceId]);
 const ranked = candidates.filter(candidate => {
  if (seen.has(candidate.songId)) return false;
  seen.add(candidate.songId); return true;
 });
 const selected = ranked.filter(candidate => !excluded.has(candidate.songId));
 if (!ranked.length) return null;
 return <details className="mt-2 text-[var(--dj-text-secondary)]">
  <summary className="cursor-pointer">Build a playlist from {ranked.length} Mix Next candidates</summary>
  <p className="mt-2">Starts with {referenceTitle}, followed by your selected candidates in recommendation order. Each suitability score compares a candidate to this reference. Listen and adjust consecutive transitions.</p>
  <ol className="max-h-40 overflow-y-auto mt-2 space-y-1" aria-label="Ranked Mix Next playlist candidates">
   {ranked.map((candidate,index)=><li key={candidate.songId}><label className="flex items-start gap-2"><input type="checkbox" checked={!excluded.has(candidate.songId)} aria-label={`Include ${candidate.title} in playlist`} onChange={event => { const checked=event.target.checked; setExcluded(previous => { const next=new Set(previous); if(checked) next.delete(candidate.songId); else next.add(candidate.songId); return next; }); }} /><span>{index+1}. {candidate.title} - {candidate.artist} · Transition suitability {Math.round(candidate.score*100)}%</span></label></li>)}
  </ol>
  <DJSaveResultsPlaylist songIds={[referenceId,...selected.map(candidate=>candidate.songId)]} />
 </details>;
}
