import type { HotCue } from '../slices/djMixerSlice';

export type HotCueQuantizeMode = 'off' | 'beat' | 'half' | 'quarter';

/**
 * Snap a cue to the nearest subdivision of the persisted beat timestamps.
 * Subdivisions are interpolated within each neighboring pair, so dynamic
 * grids retain their varying beat lengths. No BPM-based grid is invented.
 */
export function quantizeHotCuePosition(
  cue: Pick<HotCue, 'position' | 'locked'>,
  beatGrid: number[] | null | undefined,
  mode: HotCueQuantizeMode,
): number | null {
  if (cue.locked || mode === 'off' || !Number.isFinite(cue.position)) return null;
  if (!beatGrid || beatGrid.length < 2) return null;
  if (!beatGrid.every((beat, index) => Number.isFinite(beat) && beat >= 0 && (index === 0 || beat > beatGrid[index - 1]))) return null;
  if (cue.position < beatGrid[0] || cue.position > beatGrid[beatGrid.length - 1]) return null;

  const divisions = mode === 'quarter' ? 4 : mode === 'half' ? 2 : 1;
  let nearest = beatGrid[0];
  let nearestDistance = Math.abs(cue.position - nearest);
  const consider = (candidate: number) => {
    const distance = Math.abs(cue.position - candidate);
    // Strict comparison keeps exact ties at the earlier timestamp.
    if (distance < nearestDistance) {
      nearest = candidate;
      nearestDistance = distance;
    }
  };

  for (let index = 0; index < beatGrid.length - 1; index++) {
    const start = beatGrid[index];
    const interval = beatGrid[index + 1] - start;
    for (let step = 1; step <= divisions; step++) {
      consider(start + interval * (step / divisions));
    }
  }

  return nearest === cue.position ? null : nearest;
}
