import { describe, expect, it } from 'vitest';
import { canSyncBeatGrid, resolvedGridPatch, validBeatGrid } from './beatGridConfidence';
import type { TrackAnalysisFeature } from '../services/api';

const feature: TrackAnalysisFeature = { songId: 'one', status: 'complete', bpm: 120, bpmConfidence: 0.95, bpmSource: 'measured', keySource: 'unknown', syncAllowed: true };
describe('beat-grid provenance', () => {
  it('does not invent a browser grid from BPM alone', () => {
    const patch = resolvedGridPatch(feature, null, 10);
    expect(patch.beatGrid).toBeNull();
    expect(patch.beatGridSource).toBe('unknown');
    expect(patch.bpmConfidence).toBe(0.95);
    expect(canSyncBeatGrid({ beatGrid: patch.beatGrid, beatGridSource: patch.beatGridSource!, beatGridLocked: false })).toBe(false);
  });
  it('preserves a manual grid even when scalar analysis is unavailable', () => {
    const patch = resolvedGridPatch(null, { songId: 'one', beats: [0.1, 0.6], downbeatIndices: [0], locked: true, algorithmVersion: 'v1', source: 'manual' }, 10);
    expect(patch.beatGrid).toEqual([0.1, 0.6]);
    expect(patch.beatGridSource).toBe('manual');
    expect(patch.bpm).toBeNull();
    expect(canSyncBeatGrid({ beatGrid: patch.beatGrid!, beatGridSource: 'manual', beatGridLocked: true })).toBe(true);
  });
  it.each(['measured', 'generated', 'unknown'] as const)('requires review for %s grids', source => {
    expect(canSyncBeatGrid({ beatGrid: [0, 0.5], beatGridSource: source, beatGridLocked: false })).toBe(false);
  });
  it('rejects invalid beat positions and does not invent an unknown BPM', () => {
    for (const beats of [[], [0], [0, NaN], [1, 0], [0, 0], [-1, 0]]) expect(validBeatGrid(beats)).toBe(false);
    expect(resolvedGridPatch(null, null, 10)).toMatchObject({ bpm: null, beatGrid: null, beatGridSource: 'unknown' });
  });
});
