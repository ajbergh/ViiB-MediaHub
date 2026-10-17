import { describe, expect, it, vi } from 'vitest';
import { createJSONStorage } from 'zustand/middleware';
import { deduplicatingPersistStorage } from './deduplicatingPersistStorage';

describe('deduplicating persistence', () => {
  it('skips unchanged preferences before JSON serialization but persists changed preferences', () => {
    const disk = { getItem: vi.fn(() => null), setItem: vi.fn(), removeItem: vi.fn() };
    const toJSON = vi.fn(() => ({ volume: 0.8 }));
    const mixer = { volume: 0.8, toJSON };
    const storage = deduplicatingPersistStorage(createJSONStorage<{ mixer: typeof mixer }>(() => disk))!;
    storage.setItem('prefs', { state: { mixer }, version: 1 });
    for (let i = 0; i < 200; i++) storage.setItem('prefs', { state: { mixer }, version: 1 });
    expect(disk.setItem).toHaveBeenCalledTimes(1);
    expect(toJSON).toHaveBeenCalledTimes(1);
    storage.setItem('prefs', { state: { mixer: { ...mixer, volume: 0.5 } }, version: 1 });
    expect(disk.setItem).toHaveBeenCalledTimes(2);
    storage.setItem('prefs', { state: { mixer }, version: 2 });
    expect(disk.setItem).toHaveBeenCalledTimes(3);
    storage.removeItem('prefs');
    storage.setItem('prefs', { state: { mixer }, version: 2 });
    expect(disk.setItem).toHaveBeenCalledTimes(4);
  });

  it('retries failed writes and keeps storage keys independent', () => {
    const disk = { getItem: vi.fn(() => null), setItem: vi.fn().mockImplementationOnce(() => { throw new Error('quota'); }), removeItem: vi.fn() };
    const storage = deduplicatingPersistStorage(createJSONStorage<{ volume: number }>(() => disk))!;
    const value = { state: { volume: 1 } };
    expect(() => storage.setItem('a', value)).toThrow('quota');
    storage.setItem('a', value);
    storage.setItem('b', value);
    expect(disk.setItem).toHaveBeenCalledTimes(3);
  });
});
