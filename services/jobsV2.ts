/**
 * Client for persisted library operation jobs. Job mutations disable automatic
 * retries because creating a retry always means creating a distinct job.
 */
import { requestJSON } from './httpClient';
import { getEventStreamURL } from './eventStreamURL';

const JOBS_BASE = '/api/v2/jobs';

export type JobStatus = 'queued' | 'running' | 'paused' | 'succeeded' | 'failed' | 'canceling' | 'canceled' | 'interrupted';
export type JobType = 'full_scan' | 'quick_scan' | 'refresh_genre_stats' | 'analyze_tracks' | 'stem_library_scan' | 'stem_registry_refresh' | 'stem_package_link';

/** Which tracks an analysis job covers. Recorded in the job's parameters so a
 * resumed job re-expands the identical selection. */
export type AnalysisSelectionMode = 'all' | 'missing' | 'stale' | 'ids' | 'playlist';
export type AnalysisSource = 'local' | 'plex' | 'all';

export interface AnalysisSelection {
  mode: AnalysisSelectionMode;
  /** Local is the backwards-compatible default. Plex preparation is explicit. */
  source?: AnalysisSource;
  songIds?: string[];
  playlistId?: string;
}

/** Aggregate counts an analysis job reports when it completes. */
export interface AnalysisJobResult {
  mode?: string;
  total?: number;
  analyzed?: number;
  skipped?: number;
  failed?: number;
}

/** Priority at or above which the backend treats analysis as an explicit
 * "analyze this now" request that ignores DJ playback pressure. */
export const ANALYSIS_FOREGROUND_PRIORITY = 50;

/** Setting key for queueing analysis of tracks a scan just added. */
export const SETTING_AUTO_ANALYZE_NEW_TRACKS = 'analysis_auto_analyze_new';

export type AutomaticCuePointMode = 'off' | 'suggest' | 'fill-empty' | 'replace-generated';
export const SETTING_AUTO_CUE_MODE = 'analysis_auto_cue_mode';
export const AUTOMATIC_CUE_POINT_MODES: { value: AutomaticCuePointMode; label: string }[] = [
  { value: 'off', label: 'Off' },
  { value: 'suggest', label: 'Suggest only' },
  { value: 'fill-empty', label: 'Fill empty slots' },
  { value: 'replace-generated', label: 'Refresh generated cues' },
];

export function normalizeAutomaticCuePointMode(value: string): AutomaticCuePointMode {
  const normalized = value.trim().toLowerCase();
  return normalized === 'off' || normalized === 'suggest' || normalized === 'replace-generated'
    ? normalized
    : 'fill-empty';
}

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

  create(type: JobType, parameters?: Record<string, unknown>, priority?: number, signal?: AbortSignal): Promise<OperationJob> {
    return requestJSON(`${JOBS_BASE}/`, {
      method: 'POST',
      signal,
      retry: false,
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(priority === undefined ? { type, parameters } : { type, parameters, priority }),
    });
  },

  // analyze queues a durable analysis run. The work list is derived from the
  // catalog when the job is claimed, so queueing the same selection twice does
  // not analyze anything twice.
  analyze(selection: AnalysisSelection, priority?: number, signal?: AbortSignal): Promise<OperationJob> {
    return jobsV2.create('analyze_tracks', selection as unknown as Record<string, unknown>, priority, signal);
  },

  // pause and resume act on the whole durable queue, not one job: pausing stops
  // dispatching new work and lets in-flight work finish.
  pauseQueue(signal?: AbortSignal): Promise<{ paused: number }> {
    return requestJSON(`${JOBS_BASE}/pause`, { method: 'POST', signal, retry: false });
  },

  resumeQueue(signal?: AbortSignal): Promise<{ resumed: number }> {
    return requestJSON(`${JOBS_BASE}/resume`, { method: 'POST', signal, retry: false });
  },

  // reportPlaybackPressure tells the backend that DJ playback is active so
  // background analysis yields its worker. The signal carries a TTL and must be
  // renewed while playback continues, so a closed tab cannot park the queue.
  reportPlaybackPressure(active: boolean, ttlSeconds?: number, signal?: AbortSignal): Promise<{ active: boolean; remainingSeconds: number }> {
    return requestJSON(`${JOBS_BASE}/analysis-pressure`, {
      method: 'POST',
      signal,
      retry: false,
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(ttlSeconds === undefined ? { active } : { active, ttlSeconds }),
    });
  },

  cancel(id: string, signal?: AbortSignal): Promise<OperationJob> {
    return requestJSON(`${JOBS_BASE}/${encodeURIComponent(id)}/cancel`, { method: 'POST', signal, retry: false });
  },

  retry(id: string, signal?: AbortSignal): Promise<OperationJob> {
    return requestJSON(`${JOBS_BASE}/${encodeURIComponent(id)}/retry`, { method: 'POST', signal, retry: false });
  },

  // subscribe returns the EventSource cleanup function. It delivers changed
  // job snapshots only; heartbeats are consumed by EventSource internally.
  subscribe(onJobs: (jobs: OperationJob[]) => void): () => void {
    let source: EventSource | null = null;
    let disposed = false;

    void (async () => {
      try {
        const nextSource = new EventSource(await getEventStreamURL(`${JOBS_BASE}/events`));
        if (disposed) {
          nextSource.close();
          return;
        }
        source = nextSource;
        source.addEventListener('jobs', event => {
          try {
            const payload = JSON.parse((event as MessageEvent).data) as { jobs: OperationJob[] };
            onJobs(payload.jobs);
          } catch (error) {
            console.warn('Unable to parse jobs event:', error);
          }
        });
      } catch (error) {
        if (!disposed) console.warn('Unable to open jobs event stream:', error);
      }
    })();

    return () => {
      disposed = true;
      source?.close();
    };
  },
};
