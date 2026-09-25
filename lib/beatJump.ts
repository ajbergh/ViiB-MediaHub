/** Return the displayed signed jump sizes in visual order for each control row. */
export function getBeatJumpControlAmounts(compact: boolean, direction: 'back' | 'forward'): number[] {
  const amounts = compact ? [1, 4, 32] : [1, 4, 8, 16, 32];
  const ordered = direction === 'back' ? [...amounts].reverse() : amounts;
  return direction === 'back' ? ordered.map(amount => -amount) : ordered;
}

/**
 * Resolve a generic beat jump against persisted beat timestamps when possible.
 * A beat grid has no phrase-length metadata, so this does not imply phrase
 * alignment. Invalid or incomplete grids retain the legacy BPM-based behavior.
 */
export function calculateBeatJumpTimestamp({
  position,
  beats,
  duration,
  beatGrid,
  beatGridOffset = 0,
  bpm,
}: {
  position: number;
  beats: number;
  duration: number;
  beatGrid?: number[] | null;
  beatGridOffset?: number;
  bpm?: number | null;
}): number | null {
  if (!Number.isFinite(position) || !Number.isFinite(beats) || beats === 0) return null;
  if (!Number.isFinite(duration) || duration < 0) return null;

  const clamp = (timestamp: number) => Math.max(0, Math.min(duration, timestamp));
  const validGrid = Number.isFinite(beatGridOffset)
    && !!beatGrid
    && beatGrid.length >= 2
    && beatGrid.every((beat, index) => Number.isFinite(beat) && beat >= 0 && (index === 0 || beat > beatGrid[index - 1]));

  if (validGrid && position >= beatGrid[0] + beatGridOffset && position <= beatGrid[beatGrid.length - 1] + beatGridOffset) {
    const grid = beatGrid;
    let nearestIndex = 0;
    let nearestDistance = Math.abs(grid[0] + beatGridOffset - position);
    for (let index = 1; index < grid.length; index++) {
      const distance = Math.abs(grid[index] + beatGridOffset - position);
      // Strict comparison gives exact ties to the earlier beat timestamp.
      if (distance < nearestDistance) {
        nearestIndex = index;
        nearestDistance = distance;
      }
    }

    const targetIndex = nearestIndex + beats;
    if (Number.isInteger(targetIndex) && targetIndex >= 0 && targetIndex < grid.length) {
      return clamp(grid[targetIndex] + beatGridOffset);
    }
  }

  if (typeof bpm !== 'number' || !Number.isFinite(bpm) || bpm <= 0) return null;
  return clamp(position + beats * (60 / bpm));
}
