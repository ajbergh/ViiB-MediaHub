/** BPM · Camelot · key secondary line. Isolated so tempo drags only re-render it. */
import React from 'react';
import { useStore } from '../../../store';
import { describeKey } from '../../../lib/keyDetection';
import type { DeckId } from '../../../slices/djMixerSlice';

export const DeckFacts = React.memo(function DeckFacts({ deck }: { deck: DeckId }) {
  const effectiveBpm = useStore(s => (deck === 'A' ? s.djDeckA : s.djDeckB).effectiveBpm);
  const originalBpm = useStore(s => (deck === 'A' ? s.djDeckA : s.djDeckB).originalBpm);
  const key = useStore(s => (deck === 'A' ? s.djDeckA : s.djDeckB).key);
  const analysisStatus = useStore(s => (deck === 'A' ? s.djDeckA : s.djDeckB).analysisStatus);
  const validBpm = (value: number | null | undefined) => typeof value === 'number' && Number.isFinite(value) && value > 0;
  const bpm = validBpm(effectiveBpm) ? effectiveBpm : validBpm(originalBpm) ? originalBpm : null;
  const described = describeKey(key);

  if (bpm === null && (analysisStatus === 'not_analyzed' || analysisStatus === 'error')) {
    return (
      <div className='dj-deck-facts dj-deck-facts-warn' title='Analyze this track to enable measured BPM and beat-grid features'>
        {analysisStatus === 'error' ? 'Analysis unavailable' : 'Not analysed'}
      </div>
    );
  }
  return (
    <div className='dj-deck-facts'>
      <span>{bpm !== null ? `${bpm.toFixed(1)} BPM` : analysisStatus === 'loading' ? 'Analysing…' : '--.- BPM'}</span>
      {described?.camelot && <span>{described.camelot}</span>}
      {described && <span>{described.label}</span>}
    </div>
  );
});
