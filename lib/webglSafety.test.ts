import { describe, expect, it } from 'vitest';
import { getPreferredWebGLVersion, isMacOSWails, shouldUseAdvancedWebGL } from './webglSafety';

describe('WebGL safety policy', () => {
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
