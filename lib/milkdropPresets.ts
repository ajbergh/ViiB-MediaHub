/** Shared preset cache. Additional packs are prepared off the UI thread. */
export type MilkdropPresets = Record<string, object>;

let basePromise: Promise<MilkdropPresets> | undefined;
let additionalPromise: Promise<MilkdropPresets> | undefined;
let expandedPresets: MilkdropPresets | undefined;

export function loadMilkdropPresets(): Promise<MilkdropPresets> {
  if (expandedPresets) return Promise.resolve(expandedPresets);
  return basePromise ??= import('butterchurn-presets').then(pack => pack.default.getPresets());
}

/** Never blocks startup; callers keep using the base pack if the worker fails. */
export function loadAdditionalMilkdropPresets(): Promise<MilkdropPresets> {
  return additionalPromise ??= loadMilkdropPresets().then(base => new Promise<MilkdropPresets>((resolve, reject) => {
    const worker = new Worker(new URL('./milkdropPresets.worker.ts', import.meta.url), { type: 'module' });
    const finish = () => { clearTimeout(timeout); worker.terminate(); };
    const timeout = setTimeout(() => {
      finish();
      reject(new Error('Additional Milkdrop presets timed out'));
    }, 60_000);
    worker.onmessage = (event: MessageEvent<MilkdropPresets>) => {
      finish();
      // Existing definitions win when packs share a name.
      expandedPresets = { ...event.data, ...base };
      resolve(expandedPresets);
    };
    const fail = () => {
      finish();
      reject(new Error('Could not load additional Milkdrop presets'));
    };
    worker.onerror = fail;
    worker.onmessageerror = fail;
  }));
}

/** Wait for spare UI time; the fallback gives startup time to settle. */
export function scheduleMilkdropExpansion(callback: () => void): () => void {
  if (typeof window.requestIdleCallback === 'function') {
    const id = window.requestIdleCallback(callback);
    return () => window.cancelIdleCallback(id);
  }
  const id = setTimeout(callback, 1500);
  return () => clearTimeout(id);
}
