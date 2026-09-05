/** Opens external URLs outside the Wails WebView when running natively. */

export interface NavigationRuntime {
  hostname: string;
}

function getRuntime(): NavigationRuntime | null {
  if (typeof window === 'undefined') return null;
  return { hostname: window.location.hostname };
}

export function shouldUseSystemBrowser(runtime: NavigationRuntime | null = getRuntime()): boolean {
  return runtime?.hostname === 'wails.localhost';
}

/**
 * Opens an external URL in the system browser for Wails, preserving the app's
 * origin and avoiding WebKit child-window/popup behavior on macOS.
 */
export async function openExternalURL(url: string): Promise<void> {
  if (shouldUseSystemBrowser()) {
    try {
      const { BrowserOpenURL } = await import('../backend/cmd/wails/frontend/wailsjs/runtime/runtime');
      BrowserOpenURL(url);
      return;
    } catch (error) {
      console.warn('[ExternalNavigation] Wails system-browser request failed', error);
    }
  }

  window.open(url, '_blank', 'noopener,noreferrer');
}
