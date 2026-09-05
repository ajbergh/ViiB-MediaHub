/**
 * Resolves SSE URLs for both the browser build and the native Wails build.
 *
 * Wails proxies ordinary API requests through its asset server, but that
 * response path buffers indefinitely-lived Server-Sent Event responses. The
 * backend's loopback HTTP server is therefore used directly for event streams
 * in the native app. Browser and Vite development builds retain relative URLs.
 */

interface WailsAppBindings {
  GetServerURL?: () => Promise<string>;
}

interface WailsWindow extends Window {
  go?: {
    main?: {
      App?: WailsAppBindings;
    };
  };
}

let serverURLPromise: Promise<string | null> | undefined;

function getWailsServerURL(): Promise<string | null> {
  if (serverURLPromise) return serverURLPromise;

  const runtimeWindow = typeof window === 'undefined' ? undefined : (window as WailsWindow);
  const getServerURL = runtimeWindow?.go?.main?.App?.GetServerURL;
  if (!getServerURL) return Promise.resolve(null);

  serverURLPromise = getServerURL()
    .then(url => url.trim().replace(/\/$/, '') || null)
    .catch(error => {
      console.warn('Unable to resolve the local API server for event streaming:', error);
      return null;
    });
  return serverURLPromise;
}

export async function getEventStreamURL(path: string): Promise<string> {
  const serverURL = getWailsServerURL();
  const baseURL = await serverURL;
  return baseURL ? new URL(path, `${baseURL}/`).toString() : path;
}

// Test-only reset so a mocked Wails binding cannot leak between test cases.
export function resetEventStreamURLCacheForTests(): void {
  serverURLPromise = undefined;
}
