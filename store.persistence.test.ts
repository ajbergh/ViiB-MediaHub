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

it('retains Milkdrop cycle preferences while starting each new session without a previous preset', async () => {
  const initial = useStore.getState();
  try {
    useStore.getState().setMilkdropSettings({ presetCycleEnabled: false, presetCycleInterval: 60 });
    const write = vi.spyOn(Storage.prototype, 'setItem');
    try {
      useStore.getState().setMilkdropPreset('Previous preset');
      expect(write).not.toHaveBeenCalled();
    } finally {
      write.mockRestore();
    }
    const saved = JSON.parse(localStorage.getItem('mediahub-storage')!);
    expect(saved.state.milkdropSettings.currentPreset).toBeNull();

    // Older preferences included the active preset.
    saved.state.milkdropSettings.currentPreset = 'Legacy preset';
    localStorage.setItem('mediahub-storage', JSON.stringify(saved));
    await useStore.persist.rehydrate();
    expect(useStore.getState().milkdropSettings).toMatchObject({
      currentPreset: null, presetCycleEnabled: false, presetCycleInterval: 60,
    });

    useStore.getState().setNowPlayingOpen(true);
    useStore.getState().setMilkdropPreset('Runtime preset');
    useStore.getState().setNowPlayingOpen(true);
    expect(useStore.getState().milkdropSettings.currentPreset).toBe('Runtime preset');
    useStore.getState().setNowPlayingOpen(false);
    useStore.getState().setNowPlayingOpen(true);
    expect(useStore.getState().milkdropSettings.currentPreset).toBeNull();
  } finally {
    useStore.setState(initial);
  }
});
