import { createLogger } from './loggerService';

const logger = createLogger('AudioRead');
type AudioReadOperation = 'audio_metadata' | 'local_bands' | 'provider_bands' | 'deck_analysis';
const reported = new WeakSet<object>();

/** Retry a narrowly defined transient read once without retrying missing or stale data. */
export async function withTransientReadRetry<T>(read: () => Promise<T>, shouldRetry: () => boolean = () => true): Promise<T> {
  try {
    return await read();
  } catch (error) {
    const detail = error as { name?: unknown; status?: unknown } | null;
    const transient = detail?.name === 'AbortError' ? false
      : error instanceof TypeError
        || (typeof detail?.status === 'number' && detail.status >= 500 && detail.status <= 599);
    if (!transient || !shouldRetry()) throw error;
    await new Promise(resolve => setTimeout(resolve, 250));
    if (!shouldRetry()) throw error;
    return read();
  }
}
type SharedAudioMetadataRead = { promise: Promise<unknown>; owners: Set<() => boolean> };
const sharedAudioMetadataReads = new Map<string, SharedAudioMetadataRead>();

/** Share one in-flight metadata read across Local Bands consumers for the same song/source/session. */
export function withSharedAudioMetadataRead<T>(key: string, read: () => Promise<T>, isCurrent: () => boolean = () => true): Promise<T> {
  const existing = sharedAudioMetadataReads.get(key);
  if (existing) {
    existing.owners.add(isCurrent);
    return existing.promise as Promise<T>;
  }
  const owners = new Set([isCurrent]);
  const hasCurrentOwner = () => [...owners].some(owner => {
    try { return owner(); } catch { return false; }
  });
  const promise = withTransientReadRetry(read, hasCurrentOwner);
  const entry = { promise, owners };
  sharedAudioMetadataReads.set(key, entry);
  void promise.finally(() => {
    if (sharedAudioMetadataReads.get(key) === entry) sharedAudioMetadataReads.delete(key);
  }).catch(() => {});
  return promise;
}

let windowStart = 0;
let windowCount = 0;

/** Bounded normal-mode evidence. Never retain IDs, URLs, messages or stacks. */
export function reportAudioReadFailure(operation: AudioReadOperation, error: unknown): void {
  try {
    const detail = error as { name?: unknown; status?: unknown; category?: unknown } | null;
    if (detail?.name === 'AbortError') return;
    const status = typeof detail?.status === 'number' && Number.isInteger(detail.status)
      && detail.status >= 100 && detail.status <= 599 ? detail.status : undefined;
    // Missing analysis and source-change races are expected fallback outcomes.
    if (status === 404 || status === 412) return;
    if (error !== null && typeof error === 'object' && reported.has(error)) return;
    const now = Date.now();
    if (now < windowStart || now - windowStart >= 60_000) { windowStart = now; windowCount = 0; }
    if (windowCount >= 20) return;
    windowCount++;
    if (error !== null && typeof error === 'object') reported.add(error);
    const category = status !== undefined ? 'http' : detail?.category === 'invalid_payload' ? 'invalid_payload'
      : error instanceof SyntaxError ? 'invalid_json' : error instanceof TypeError ? 'type_error' : 'unknown';
    logger.warn('Audio read failed', { operation, category, ...(status === undefined ? {} : { status }) });
  } catch { /* Diagnostics must never change fallback or recovery behavior. */ }
}
