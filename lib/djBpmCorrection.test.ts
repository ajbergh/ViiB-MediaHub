import { describe, expect, it } from 'vitest';
import { djTrackSourceIdentity, formatManualBpm, tapTempoBpm } from './djBpmCorrection';

describe('manual DJ BPM controls', () => {
  it('distinguishes source changes even when the catalog song ID stays the same', () => {
    const local = { id: 'song', source: 'local' as const, path: 'crate/song.wav', fileHash: 'hash-a' };
    expect(djTrackSourceIdentity(local)).toBe(djTrackSourceIdentity({ ...local }));
    expect(djTrackSourceIdentity(local)).not.toBe(djTrackSourceIdentity({ ...local, fileHash: 'hash-b' }));
    expect(djTrackSourceIdentity(local)).not.toBe(djTrackSourceIdentity({ ...local, source: 'plex', sourceName: 'server' }));
  });

  it('formats fractional BPM without rounding to integers', () => {
    expect(formatManualBpm(127.375)).toBe('127.38');
    expect(formatManualBpm(128)).toBe('128');
    expect(formatManualBpm(Number.NaN)).toBe('Unknown');
    expect(formatManualBpm(Number.POSITIVE_INFINITY)).toBe('Unknown');
  });

  it('uses four taps and a median interval, rejecting implausible gaps', () => {
    expect(tapTempoBpm([0, 500, 1002, 1498])).toBeCloseTo(120, 0);
    expect(tapTempoBpm([0, 500, 1000])).toBeUndefined();
    expect(tapTempoBpm([0, 500, 4000, 4500])).toBeUndefined();
  });
});
