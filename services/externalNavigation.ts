/** Opens external URLs outside the Wails WebView when running natively. */

export interface NavigationRuntime {
  hostname: string;
  protocol?: string;
  hasWailsBridge?: boolean;
}

function getRuntime(): NavigationRuntime | null {
  if (typeof window === 'undefined') return null;
  const wailsWindow = window as Window & {
    runtime?: { BrowserOpenURL?: (url: string) => void };
    go?: unknown;
  };

  return {
    hostname: window.location.hostname,
    protocol: window.location.protocol,
    hasWailsBridge: typeof wailsWindow.runtime?.BrowserOpenURL === 'function'
      || typeof wailsWindow.go !== 'undefined',
  };
}

export function shouldUseSystemBrowser(runtime: NavigationRuntime | null = getRuntime()): boolean {
  return runtime?.hostname === 'wails.localhost'
    || runtime?.hostname === 'wails'
    || runtime?.protocol === 'wails:'
    || runtime?.hasWailsBridge === true;
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
      console.error('[ExternalNavigation] Wails system-browser request failed', error);
      throw new Error('The system browser could not be opened.', { cause: error });
    }
  }

  window.open(url, '_blank', 'noopener,noreferrer');
}
