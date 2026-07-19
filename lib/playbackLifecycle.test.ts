import { describe, expect, it, vi } from 'vitest';
import { ManagedObjectUrlRegistry, normalizeCrossfadeDuration } from './playbackLifecycle';

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
