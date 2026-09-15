import type { DeckAnalysisPatch, DeckState } from '../slices/djMixerSlice';
import type { TrackAnalysisFeature, TrackBeatGrid } from '../services/api';

export type BeatGridSource = 'unknown' | 'generated' | 'measured' | 'manual';
export function validBeatGrid(beats: number[] | null | undefined): boolean {
  return !!beats && beats.length >= 2 && beats.every((beat, i) => Number.isFinite(beat) && beat >= 0 && (i === 0 || beat > beats[i - 1]));
}
export function canSyncBeatGrid(deck: Pick<DeckState, 'beatGrid' | 'beatGridSource' | 'beatGridLocked'>): boolean {
  // No calibrated phase confidence is available yet. A lock is the DJ's explicit review.
  return deck.beatGridSource === 'manual' && deck.beatGridLocked && validBeatGrid(deck.beatGrid);
}
export function resolvedGridPatch(feature: TrackAnalysisFeature | null, grid: TrackBeatGrid | null, _duration: number): DeckAnalysisPatch {
  const bpm = typeof feature?.bpm === 'number' && Number.isFinite(feature.bpm) && feature.bpm > 0 ? feature.bpm : null;
  const patch: DeckAnalysisPatch = { automatic: true, bpm, bpmConfidence: feature?.bpmConfidence ?? null };
  patch.tempoEvidence = feature?.bpmSource === 'measured' ? { source: 'server', bpm: bpm ?? undefined, score: feature.bpmConfidence, alternateBpm: feature.bpmAltCandidate ?? null, stability: feature.tempoStability ?? null } : null;
  if (feature?.key) patch.key = feature.key;
  if (grid && validBeatGrid(grid.beats)) {
    patch.beatGrid = grid.beats;
    patch.downbeatIndices = grid.downbeatIndices;
    patch.beatGridLocked = grid.locked;
    // Older servers have no provenance. Locked grids were saved by the manual editor.
    patch.beatGridSource = grid.source ?? (grid.locked ? 'manual' : 'unknown');
  } else {
    // BPM alone is never enough to manufacture a performance grid in the
    // browser. The backend analyzer persists phase-aligned grids atomically.
    patch.beatGrid = null;
    patch.beatGridSource = 'unknown';
    patch.beatGridLocked = false;
    patch.downbeatIndices = null;
  }
  return patch;
}
