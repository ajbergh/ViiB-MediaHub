import { describe, expect, it, vi } from 'vitest';
import { calculateReplayGain, ManagedObjectUrlRegistry, normalizeCrossfadeDuration } from './playbackLifecycle';

describe('normalizeCrossfadeDuration', () => {
  it('preserves an explicit zero', () => {
    expect(normalizeCrossfadeDuration(0, false)).toBe(0);
  });
  it('forces zero for gapless mode', () => {
    expect(normalizeCrossfadeDuration(5, true)).toBe(0);
  });
});

describe('ManagedObjectUrlRegistry', () => {
  it('revokes managed URLs exactly once', () => {
    const create = vi.spyOn(URL, 'createObjectURL').mockReturnValue('blob:test');
    const revoke = vi.spyOn(URL, 'revokeObjectURL').mockImplementation(() => undefined);
    const registry = new ManagedObjectUrlRegistry();
    const url = registry.create(new Blob(['audio']));
    registry.release(url);
    registry.release(url);
    expect(create).toHaveBeenCalledOnce();
    expect(revoke).toHaveBeenCalledOnce();
  });
});


describe('calculateReplayGain', () => {
  it('converts dB to linear gain', () => {
    expect(calculateReplayGain(-6)).toBeCloseTo(0.501, 2);
  });
  it('limits gain to prevent clipping when peak metadata is present', () => {
    expect(calculateReplayGain(6, 0.8)).toBeCloseTo(1.25, 4);
  });
  it('returns unity without metadata', () => {
    expect(calculateReplayGain()).toBe(1);
  });
});
