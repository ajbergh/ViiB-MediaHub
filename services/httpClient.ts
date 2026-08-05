export interface APIErrorPayload {
  code: string;
  message: string;
  retryable: boolean;
  requestId?: string;
  details?: Record<string, unknown>;
}

export class APIError extends Error {
  readonly code: string;
  readonly retryable: boolean;
  readonly requestId?: string;
  readonly details?: Record<string, unknown>;
  readonly status: number;

  constructor(status: number, payload: APIErrorPayload) {
    super(payload.message);
    this.name = 'APIError';
    this.status = status;
    this.code = payload.code;
    this.retryable = payload.retryable;
    this.requestId = payload.requestId;
    this.details = payload.details;
  }
}

export interface RequestOptions extends RequestInit {
  timeoutMs?: number;
  retry?: boolean;
}

function combineAbortSignal(parent: AbortSignal | null | undefined, timeoutMs: number) {
  const controller = new AbortController();
  const timer = window.setTimeout(() => controller.abort(new DOMException('Request timed out', 'TimeoutError')), timeoutMs);
  const abort = () => controller.abort(parent?.reason);
  parent?.addEventListener('abort', abort, { once: true });
  return {
    signal: controller.signal,
    cleanup: () => {
      window.clearTimeout(timer);
      parent?.removeEventListener('abort', abort);
    },
  };
}

async function parseError(response: Response): Promise<APIError> {
  const body = await response.json().catch(() => null) as { error?: APIErrorPayload } | null;
  const requestId = response.headers.get('X-Request-ID') || undefined;
  return new APIError(response.status, body?.error || {
    code: `http_${response.status}`,
    message: response.statusText || `HTTP ${response.status}`,
    retryable: response.status >= 500 || response.status === 429,
    requestId,
  });
}

export async function requestJSON<T>(url: string, options: RequestOptions = {}): Promise<T> {
  const method = (options.method || 'GET').toUpperCase();
  const retryAllowed = options.retry !== false && ['GET', 'HEAD', 'OPTIONS'].includes(method);
  const attempts = retryAllowed ? 2 : 1;
  let lastError: unknown;

  for (let attempt = 0; attempt < attempts; attempt += 1) {
    const { signal, cleanup } = combineAbortSignal(options.signal, options.timeoutMs ?? 15_000);
    try {
      const response = await fetch(url, {
        ...options,
        signal,
        headers: { Accept: 'application/json', ...options.headers },
      });
      if (!response.ok) throw await parseError(response);
      if (response.status === 204) return undefined as T;
      return await response.json() as T;
    } catch (error) {
      lastError = error;
      const retryable = error instanceof APIError ? error.retryable : !(error instanceof DOMException && error.name === 'AbortError');
      if (attempt + 1 >= attempts || !retryable || options.signal?.aborted) throw error;
      await new Promise(resolve => window.setTimeout(resolve, 250 * (attempt + 1)));
    } finally {
      cleanup();
    }
  }
  throw lastError;
}
