/**
 * First typed public-RSS Podcast API client. Subscriptions and episode
 * metadata are backend-owned; do not persist them in the music Zustand store.
 * Streaming, revisioned listening progress and the early-access offline
 * download endpoints are local Go APIs; provider URLs remain backend-only.
 */
const BASE = '/api/v2/podcasts';

export interface PodcastEpisode {
  id: string;
  feedId: string;
  guid: string;
  title: string;
  description: string;
  /** Publisher URLs may carry ephemeral signatures; never expose those in UI JSON. */
  hasAudio: boolean;
  enclosureType: string;
  publishedAt: number;
  durationMs: number;
  isPlayed?: boolean;
  positionMs?: number;
  listenedMs?: number;
  progressRevision?: number;
}
export interface PodcastContinueEpisode extends PodcastEpisode {
  feedTitle: string;
  positionMs: number;
  listenedMs: number;
  progressRevision: number;
}
export interface PodcastStorage {
  usedBytes: number;
  maxBytes: number;
}
export interface PodcastDownload {
  id: string;
  episodeId: string;
  status: 'queued' | 'downloading' | 'completed' | 'failed' | 'canceled' | 'paused';
  bytesReceived: number;
  expectedBytes: number;
  verifiedSize: number;
  errorCode?: string;
  createdAt: number;
  updatedAt: number;
  completedAt: number;
}
export interface PodcastProgress {
  episodeId: string;
  positionMs: number;
  listenedMs: number;
  isPlayed: boolean;
  revision: number;
  completedAt: number;
  lastPlayedAt: number;
}
export interface PodcastRefreshStatus {
  lastCheckedAt: number;
  lastSuccessAt: number;
  nextRefreshAt: number;
  lastErrorCode?: string;
  failures: number;
  refreshing: boolean;
}
export interface PodcastFeed {
  id: string;
  url: string;
  title: string;
  author: string;
  description: string;
  imageUrl: string;
  language: string;
  episodeCount: number;
}
export interface PodcastPreview {
  feed: {
    url: string;
    title: string;
    author?: string;
    description?: string;
    imageUrl?: string;
    language?: string;
    episodes: Array<{
      guid?: string;
      title: string;
      description?: string;
      enclosureUrl?: string;
      enclosureType?: string;
      durationSeconds?: number;
    }>;
  };
  episodeCount: number;
}
export interface PodcastOPMLImportPreview {
  feeds: Array<{ title: string; url: string; alreadyFollowed: boolean }>;
  invalidCount: number;
  duplicateCount: number;
}
export class PodcastAPIError extends Error {
  constructor(readonly status: number, readonly code: string, message: string) {
    super(message);
    this.name = 'PodcastAPIError';
  }
}
async function requestResponse(path: string, options?: RequestInit): Promise<Response> {
  const response = await fetch(BASE + path, {
    ...options,
    headers: { 'Accept': 'application/json', ...(options?.body ? { 'Content-Type': 'application/json' } : {}), ...options?.headers },
  });
  if (!response.ok) {
    // Supported Go handlers return structured v2 JSON errors. HTML/text
    // failures generally indicate a stale backend or a reverse-proxy issue.
    const payload: unknown = await response.json().catch(() => null);
    const error = payload && typeof payload === 'object' && 'error' in payload
      ? (payload as { error?: unknown }).error : null;
    const detail = error && typeof error === 'object'
      ? error as { code?: unknown; message?: unknown } : null;
    const code = typeof detail?.code === 'string' ? detail.code : 'podcast_api_unavailable';
    let message = typeof detail?.message === 'string' && detail.message.trim()
      ? detail.message : '';
    if (!message) {
      switch (response.status) {
        case 404:
        case 405:
          message = `Podcasts API endpoint unavailable (HTTP ${response.status}). Rebuild and restart the Go backend from the same branch as the UI.`;
          break;
        case 502:
        case 503:
        case 504:
          message = `Local podcast backend unavailable (HTTP ${response.status}). Verify that Go is running and the development proxy uses its port.`;
          break;
        case 403:
          message = 'The local API blocked this request (HTTP 403). Check the client origin and backend security settings.';
          break;
        default:
          message = `Podcast API returned HTTP ${response.status} without a structured error. Inspect the Network response and backend logs.`;
      }
    }
    throw new PodcastAPIError(response.status, code, message);
  }
  return response;
}
async function request<T>(path: string, options?: RequestInit): Promise<T> {
  const response = await requestResponse(path, options);
  if (response.status === 204) return undefined as T;
  return response.json() as Promise<T>;
}
export const podcastsAPI = {
  mediaURL(episodeId: string): string {
    return BASE + '/episodes/' + encodeURIComponent(episodeId) + '/media';
  },
  listDownloads(signal?: AbortSignal): Promise<{ downloads: PodcastDownload[]; storage?: PodcastStorage }> {
    return request('/downloads', { signal });
  },
  queueDownload(episodeId: string, signal?: AbortSignal): Promise<{ download: PodcastDownload }> {
    return request('/episodes/' + encodeURIComponent(episodeId) + '/download', { method: 'POST', signal });
  },
  cancelDownload(episodeId: string, signal?: AbortSignal): Promise<{ download: PodcastDownload }> {
    return request('/episodes/' + encodeURIComponent(episodeId) + '/download', { method: 'DELETE', signal });
  },
  // Completed-file DELETE returns 204; never expose local disk paths.
  removeDownload(episodeId: string, signal?: AbortSignal): Promise<void> {
    return request('/episodes/' + encodeURIComponent(episodeId) + '/download', { method: 'DELETE', signal });
  },
  getDownload(episodeId: string, signal?: AbortSignal): Promise<{ download: PodcastDownload }> {
    return request('/episodes/' + encodeURIComponent(episodeId) + '/download', { signal });
  },
  listContinueListening(limit = 12, signal?: AbortSignal): Promise<{ episodes: PodcastContinueEpisode[] }> {
    return request('/episodes/continue?limit=' + encodeURIComponent(String(limit)), { signal });
  },
  getProgress(episodeId: string, signal?: AbortSignal): Promise<PodcastProgress> {
    return request('/episodes/' + encodeURIComponent(episodeId) + '/progress', { signal });
  },
  setProgress(episodeId: string, progress: Pick<PodcastProgress, 'positionMs' | 'listenedMs' | 'isPlayed'> & { expectedRevision: number }, signal?: AbortSignal): Promise<PodcastProgress> {
    return request('/episodes/' + encodeURIComponent(episodeId) + '/progress', {
      method: 'PUT', body: JSON.stringify(progress), signal,
    });
  },
  previewOPML(xml: string, signal?: AbortSignal): Promise<PodcastOPMLImportPreview> {
    return request('/import/opml/preview', {
      method: 'POST', body: xml, headers: { 'Content-Type': 'application/xml' }, signal,
    });
  },
  async exportOPML(signal?: AbortSignal): Promise<Blob> {
    const response = await requestResponse('/export/opml', { signal, headers: { Accept: 'application/xml' } });
    return response.blob();
  },
  getFeed(feedId: string, signal?: AbortSignal): Promise<{ feed: PodcastFeed }> {
    return request('/feeds/' + encodeURIComponent(feedId), { signal });
  },
  listSubscriptions(signal?: AbortSignal): Promise<{ feeds: PodcastFeed[] }> {
    return request('/feeds', { signal });
  },
  previewFeed(url: string, signal?: AbortSignal): Promise<PodcastPreview> {
    return request('/feeds/preview', { method: 'POST', body: JSON.stringify({ url }), signal });
  },
  subscribe(url: string, signal?: AbortSignal): Promise<{ id: string; title: string; episodeCount: number }> {
    return request('/feeds', { method: 'POST', body: JSON.stringify({ url }), signal });
  },
  listEpisodes(feedId: string, signal?: AbortSignal, offset = 0, limit = 50): Promise<{ episodes: PodcastEpisode[]; nextOffset: number | null }> {
    const query = new URLSearchParams({ offset: String(offset), limit: String(limit) });
    return request('/feeds/' + encodeURIComponent(feedId) + '/episodes?' + query.toString(), { signal });
  },
  refresh(feedId: string, signal?: AbortSignal): Promise<{status: 'queued'}> {
    return request('/feeds/' + encodeURIComponent(feedId) + '/refresh', { method: 'POST', signal });
  },
  refreshStatus(feedId: string, signal?: AbortSignal): Promise<PodcastRefreshStatus> {
    return request('/feeds/' + encodeURIComponent(feedId) + '/refresh', { signal });
  },
  unsubscribe(feedId: string, signal?: AbortSignal): Promise<void> {
    return request('/feeds/' + encodeURIComponent(feedId), { method: 'DELETE', signal });
  },
};
