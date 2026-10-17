// @vitest-environment jsdom
import { expect, it, vi } from 'vitest';
import { useStore } from './store';

it('does not persist deck position ticks but immediately saves changed mixer preferences', () => {
  const write = vi.spyOn(Storage.prototype, 'setItem');
  const initial = useStore.getState();
  try {
    useStore.getState().setDeckPosition('A', 0); // Seed the preference snapshot.
    write.mockClear();
    for (let i = 0; i < 100; i++) {
      useStore.getState().setDeckPosition('A', i / 20);
      useStore.getState().setDeckPosition('B', i / 20);
    }
    expect(write).not.toHaveBeenCalled();
    useStore.getState().setCrossfader(0.25);
    expect(write).toHaveBeenCalledTimes(1);
    const saved = JSON.parse(write.mock.calls[0][1]);
    expect(saved.state.djMixer.crossfader).toBe(0.25);
    expect(saved.state.djDeckA).toBeUndefined();
  } finally {
    useStore.setState(initial);
    write.mockRestore();
  }
});
