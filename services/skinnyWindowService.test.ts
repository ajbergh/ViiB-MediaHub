import { afterEach, expect, it, vi } from 'vitest';
import { hideNativeWindow } from './skinnyWindowService';

const runtime = vi.hoisted(() => ({ WindowMinimise: vi.fn(), WindowHide: vi.fn() }));
vi.mock('../backend/cmd/wails/frontend/wailsjs/runtime/runtime', () => runtime);
afterEach(() => { vi.unstubAllGlobals(); vi.clearAllMocks(); });

it('minimises the Darwin custom-scheme window to the Dock instead of hiding it', async () => {
  vi.stubGlobal('window', { runtime: {}, location: { protocol: 'wails:', hostname: 'wails' } });
  vi.stubGlobal('navigator', { userAgent: 'Mozilla/5.0 (Macintosh; Intel Mac OS X 12_0)' });
  await hideNativeWindow();
  expect(runtime.WindowMinimise).toHaveBeenCalledOnce();
  expect(runtime.WindowHide).not.toHaveBeenCalled();
});

it('preserves hide-to-tray behavior for Windows', async () => {
  vi.stubGlobal('window', { runtime: {}, location: { protocol: 'http:', hostname: 'wails.localhost' } });
  vi.stubGlobal('navigator', { userAgent: 'Mozilla/5.0 (Windows NT 10.0; Win64; x64)' });
  await hideNativeWindow();
  expect(runtime.WindowHide).toHaveBeenCalledOnce();
  expect(runtime.WindowMinimise).not.toHaveBeenCalled();
});
