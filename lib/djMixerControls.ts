import type { DeckId, DeckState, MixerState } from '../slices/djMixerSlice';
import { canSyncBeatGrid } from './beatGridConfidence';

/** CUT opens the incoming channel over the first 5% of fader travel, then
 * keeps both channels open until the outgoing channel's final 5%. */
export function crossfaderGains(position: number, curve: MixerState['crossfaderCurve']) {
  const x = (Math.max(-1, Math.min(1, Number.isFinite(position) ? position : 0)) + 1) / 2;
  if (curve === 'linear') return { a: 1 - x, b: x };
  if (curve === 'sharp') return { a: Math.min(1, (1 - x) / 0.05), b: Math.min(1, x / 0.05) };
  return { a: Math.cos(x * Math.PI / 2), b: Math.sin(x * Math.PI / 2) };
}

type SyncState = { djDeckA: DeckState; djDeckB: DeckState; djMixer: Pick<MixerState, 'syncMode'> };
export function syncUnavailableReason(state: SyncState, deck: DeckId): string | null {
  if (state.djMixer.syncMode === 'off') return 'Sync disabled — choose BPM or PHASE in the mixer.';
  const target = deck === 'A' ? state.djDeckA : state.djDeckB;
  const source = deck === 'A' ? state.djDeckB : state.djDeckA;
  if (!target.track || !source.track) return 'Load a track on both decks to sync.';
  if (!Number.isFinite(target.originalBpm) || !target.originalBpm || target.originalBpm <= 0
    || !Number.isFinite(source.effectiveBpm) || !source.effectiveBpm || source.effectiveBpm <= 0) {
    return 'Both decks need BPM analysis before syncing.';
  }
  const tempo = source.effectiveBpm / target.originalBpm;
  if (tempo < 0.5 || tempo > 1.5) return 'Matching the other deck exceeds the supported tempo range (50–150%).';
  if (state.djMixer.syncMode === 'beat-phase' && (!canSyncBeatGrid(target) || !canSyncBeatGrid(source))) {
    return 'Verify and lock both grids in Edit Grid before PHASE sync. BPM sync is still available.';
  }
  return null;
}

/** Phase correction uses grid offsets and the shortest route across a beat
 * boundary. Before/after a finite grid, extrapolate its edge interval. */
export function beatPhaseCorrection(target: Pick<DeckState, 'beatGrid' | 'beatGridOffset'>,
  source: Pick<DeckState, 'beatGrid' | 'beatGridOffset'>, targetPosition: number, sourcePosition: number): number {
  const phase = (deck: typeof target, position: number) => {
    const grid = deck.beatGrid!;
    const time = position - (deck.beatGridOffset || 0);
    let lo = 0, hi = grid.length - 1;
    while (lo < hi) { const mid = Math.ceil((lo + hi) / 2); if (grid[mid] <= time) lo = mid; else hi = mid - 1; }
    const i = Math.min(lo, grid.length - 2);
    const duration = grid[i + 1] - grid[i];
    const fraction = ((time - grid[i]) / duration % 1 + 1) % 1;
    return { fraction, duration };
  };
  const a = phase(target, targetPosition), b = phase(source, sourcePosition);
  const difference = b.fraction - a.fraction;
  return (difference - Math.round(difference)) * a.duration;
}
