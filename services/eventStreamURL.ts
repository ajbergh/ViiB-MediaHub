/**
 * Resolves streaming URLs for both the browser build and the native Wails build.
 *
 * Wails proxies ordinary API requests through its asset server, but that
 * response path can buffer Server-Sent Event and audio responses. The
 * backend's loopback HTTP server is therefore used directly for event and audio streams
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

  const resolution = resolveWailsServerURL(getServerURL).then(url => {
    if (!url && serverURLPromise === resolution) serverURLPromise = undefined;
    return url;
  });
  serverURLPromise = resolution;
  return resolution;
}

async function resolveWailsServerURL(getServerURL: () => Promise<string>): Promise<string | null> {
  const read = async () => {
    const address = (await getServerURL()).trim();
    if (!address) return null;
    const url = new URL(address);
    return url.protocol === 'http:' && url.hostname === '127.0.0.1' ? url.origin : null;
  };

  const first = await read().catch(() => null);
  if (first) return first;

  // Give a briefly unavailable native bridge time to recover before creating
  // the audio source or EventSource with the asset-server URL.
  await new Promise(resolve => setTimeout(resolve, 100));
  return read().catch(error => {
    console.warn('Unable to resolve the local API server for event streaming:', error);
    return null;
  });
}

/** Resolves a stream path against the validated loopback origin when the Wails bridge is available. */
export async function getEventStreamURL(path: string): Promise<string> {
  const serverURL = getWailsServerURL();
  const baseURL = await serverURL;
  return baseURL ? new URL(path, `${baseURL}/`).toString() : path;
}

// Test-only reset so a mocked Wails binding cannot leak between test cases.
export function resetEventStreamURLCacheForTests(): void {
  serverURLPromise = undefined;
}
