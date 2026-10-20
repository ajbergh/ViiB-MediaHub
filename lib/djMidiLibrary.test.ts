import { expect, it, vi } from 'vitest';
import { browseDJMidiLibrary, loadDJMidiLibrary, registerDJMidiLibrary, registerDJMidiLibraryOpener, toggleDJMidiLibrary } from './djMidiLibrary';

it('opens the drawer, defers its first browse until mounting, and disconnects cleanly', () => {
  const open = vi.fn(); const browse = vi.fn(); const load = vi.fn();
  const removeOpener = registerDJMidiLibraryOpener(open);
  browseDJMidiLibrary(1);
  expect(open).toHaveBeenCalledOnce();
  const removeBrowser = registerDJMidiLibrary({ browse, load });
  expect(browse).toHaveBeenCalledWith(1);
  browseDJMidiLibrary(-2); loadDJMidiLibrary('B');
  expect(browse).toHaveBeenLastCalledWith(-2);
  expect(load).toHaveBeenCalledWith('B');
  removeBrowser(); removeOpener();
  browse.mockClear(); browseDJMidiLibrary(1);
  expect(browse).not.toHaveBeenCalled();
});

it('toggles only on Browse presses, keeping dial and load operations open-only', () => {
  let visible = false;
  const open = vi.fn(() => { visible = true; });
  const toggle = vi.fn(() => { visible = !visible; });
  const cleanup = registerDJMidiLibraryOpener(open, toggle);
  const disconnect = registerDJMidiLibrary({ browse: vi.fn(), load: vi.fn() });
  try {
    toggleDJMidiLibrary(); expect(visible).toBe(true);
    toggleDJMidiLibrary(); expect(visible).toBe(false);
    browseDJMidiLibrary(1); expect(visible).toBe(true);
    browseDJMidiLibrary(1); expect(visible).toBe(true);
    loadDJMidiLibrary('A'); expect(visible).toBe(true);
    loadDJMidiLibrary('B'); expect(visible).toBe(true);
    expect(toggle).toHaveBeenCalledTimes(2);
  } finally { disconnect(); cleanup(); }
  toggleDJMidiLibrary();
  expect(toggle).toHaveBeenCalledTimes(2);
});
