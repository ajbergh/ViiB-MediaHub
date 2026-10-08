import { createLogger } from './loggerService';

const logger = createLogger('AudioRead');
type AudioReadOperation = 'audio_metadata' | 'local_bands' | 'deck_analysis';
const reported = new WeakSet<object>();
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
