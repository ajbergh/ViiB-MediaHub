/** Tests and fixtures for webgl Safety behavior. */

import { describe, expect, it, vi } from 'vitest';
import { createPreferredWebGLContext, getPreferredWebGLVersion, isMacOSWails, shouldUseAdvancedWebGL } from './webglSafety';

describe('WebGL safety policy', () => {
  it('detects WebGL2 on the rendering canvas without creating a separate probe', () => {
    const gl = {};
    const canvas = { getContext: vi.fn((_kind: string) => gl) };
    expect(createPreferredWebGLContext(canvas as unknown as HTMLCanvasElement)).toEqual({ gl, webgl2: true });
    expect(canvas.getContext).toHaveBeenCalledTimes(1);
    expect(canvas.getContext.mock.calls[0][0]).toBe('webgl2');
  });

  it('tries WebGL1 when WebGL2 throws and returns null when both profiles fail', () => {
    const gl = {};
    const getContext = vi.fn().mockImplementationOnce(() => { throw new Error('disabled'); }).mockReturnValueOnce(gl);
    expect(createPreferredWebGLContext({ getContext } as unknown as HTMLCanvasElement)).toEqual({ gl, webgl2: false });
    getContext.mockReset().mockReturnValue(null);
    expect(createPreferredWebGLContext({ getContext } as unknown as HTMLCanvasElement)).toBeNull();
    expect(getContext.mock.calls.map(call => call[0])).toEqual(['webgl2', 'webgl']);
  });
  it('recognizes the macOS Wails WebKit shell while preferring WebGL 2', () => {
    const runtime = {
      hostname: 'wails.localhost',
      userAgent: 'Mozilla/5.0 (Macintosh; Intel Mac OS X 14_5) AppleWebKit/605.1.15',
    };

    expect(isMacOSWails(runtime)).toBe(true);
    expect(getPreferredWebGLVersion(runtime)).toBe('webgl2');
    expect(shouldUseAdvancedWebGL(runtime)).toBe(true);
  });

  it('keeps advanced WebGL available in browser and non-macOS Wails builds', () => {
    expect(getPreferredWebGLVersion({
      hostname: 'wails.localhost',
      userAgent: 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36',
    })).toBe('webgl2');
    expect(getPreferredWebGLVersion({
      hostname: 'localhost',
      userAgent: 'Mozilla/5.0 (Macintosh; Intel Mac OS X 14_5) AppleWebKit/605.1.15',
    })).toBe('webgl2');
  });
});
