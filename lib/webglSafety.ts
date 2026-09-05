/**
 * WebGL safety policy for the native desktop shell.
 *
 * Wails on macOS is backed by WKWebView. Modern WebKit supports WebGL 2, but
 * Wails can still ship on older macOS releases, so every renderer must retain
 * a valid WebGL 1 fallback.
 */

export interface WebGLRuntime {
  hostname: string;
  userAgent: string;
}

export type PreferredWebGLVersion = 'webgl1' | 'webgl2';

function getRuntime(): WebGLRuntime | null {
  if (typeof window === 'undefined' || typeof navigator === 'undefined') {
    return null;
  }

  return {
    hostname: window.location.hostname,
    userAgent: navigator.userAgent,
  };
}

export function isMacOSWails(runtime: WebGLRuntime | null = getRuntime()): boolean {
  return Boolean(
    runtime
    && runtime.hostname === 'wails.localhost'
    && /Macintosh|Mac OS X/i.test(runtime.userAgent),
  );
}

/**
 * Returns the renderer profile to request before creating a context.
 *
 * WebGL 2 is preferred on every platform. The renderer will fall back to its
 * WebGL 1 shaders if the current system WebKit does not expose WebGL 2.
 */
export function getPreferredWebGLVersion(runtime: WebGLRuntime | null = getRuntime()): PreferredWebGLVersion {
  void runtime;
  return 'webgl2';
}

/** True when the runtime can use the WebGL renderer rather than Canvas 2D. */
export function shouldUseAdvancedWebGL(runtime: WebGLRuntime | null = getRuntime()): boolean {
  return runtime !== null;
}
