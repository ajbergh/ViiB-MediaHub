import React from 'react';
import { useStore } from '../../../store';
import { canSyncBeatGrid } from '../../../lib/beatGridConfidence';
import type { DeckId } from '../../../slices/djMixerSlice';

export function DJBeatGridStatus({ deck }: { deck: DeckId }) {
  const source = useStore(s => (deck === 'A' ? s.djDeckA : s.djDeckB).beatGridSource);
  const confidence = useStore(s => (deck === 'A' ? s.djDeckA : s.djDeckB).bpmConfidence);
  const ready = useStore(s => canSyncBeatGrid(deck === 'A' ? s.djDeckA : s.djDeckB));
  const hasGrid = useStore(s => !!(deck === 'A' ? s.djDeckA : s.djDeckB).beatGrid?.length);
  const label = ready ? 'Verified grid' : !hasGrid ? 'No grid' : source === 'generated' ? 'Estimated grid' : source === 'measured' ? 'Measured grid' : source === 'manual' ? 'Manual grid' : 'Unverified grid';
  const tempo = Number.isFinite(confidence) && confidence !== null ? `Tempo evidence: ${Math.round(Math.max(0, Math.min(1, confidence)) * 100)}%. ` : 'Tempo evidence unavailable. ';
  return <span className={`text-[12px] font-semibold whitespace-nowrap ${ready ? 'text-brand' : 'text-text-secondary'}`}
    title={ready ? 'Reviewed and locked by you. Beat-phase sync is available when both decks are verified.' : `${tempo}Tempo evidence is not an accuracy probability. Check alignment across the track, then verify and lock in Edit Grid. BPM-only sync remains available.`}>
    {label}{!ready && hasGrid ? ' · Review' : ''}
  </span>;
}
