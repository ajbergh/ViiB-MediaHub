import { requestJSON } from './httpClient';

const JOBS_BASE = '/api/v2/jobs';

export type JobStatus = 'queued' | 'running' | 'succeeded' | 'failed' | 'canceling' | 'canceled' | 'interrupted';
export type JobType = 'full_scan' | 'quick_scan' | 'refresh_genre_stats';

export interface OperationJob {
  id: string;
  type: JobType | string;
  status: JobStatus;
  progressCurrent: number;
  progressTotal: number;
  message?: string;
  parameters?: Record<string, unknown>;
  result?: Record<string, unknown>;
  errorCode?: string;
  errorMessage?: string;
  attempts: number;
  createdAt: number;
  startedAt?: number;
  completedAt?: number;
  updatedAt: number;
}

export const jobsV2 = {
  list(status = '', signal?: AbortSignal): Promise<{ jobs: OperationJob[] }> {
    const params = new URLSearchParams({ limit: '100' });
    if (status) params.set('status', status);
    return requestJSON(`${JOBS_BASE}/?${params.toString()}`, { signal });
  },

  get(id: string, signal?: AbortSignal): Promise<OperationJob> {
    return requestJSON(`${JOBS_BASE}/${encodeURIComponent(id)}`, { signal });
  },

  create(type: JobType, parameters?: Record<string, unknown>, signal?: AbortSignal): Promise<OperationJob> {
    return requestJSON(`${JOBS_BASE}/`, {
      method: 'POST',
      signal,
      retry: false,
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ type, parameters }),
    });
  },

  cancel(id: string, signal?: AbortSignal): Promise<OperationJob> {
    return requestJSON(`${JOBS_BASE}/${encodeURIComponent(id)}/cancel`, { method: 'POST', signal, retry: false });
  },

  retry(id: string, signal?: AbortSignal): Promise<OperationJob> {
    return requestJSON(`${JOBS_BASE}/${encodeURIComponent(id)}/retry`, { method: 'POST', signal, retry: false });
  },

  subscribe(onJobs: (jobs: OperationJob[]) => void): () => void {
    const source = new EventSource(`${JOBS_BASE}/events`);
    source.addEventListener('jobs', event => {
      try {
        const payload = JSON.parse((event as MessageEvent).data) as { jobs: OperationJob[] };
        onJobs(payload.jobs);
      } catch (error) {
        console.warn('Unable to parse jobs event:', error);
      }
    });
    return () => source.close();
  },
};
